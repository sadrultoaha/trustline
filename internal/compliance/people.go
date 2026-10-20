package compliance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/org"
)

var systemNames = map[string]string{"okta": "Okta", "google_workspace": "Google Workspace", "github": "GitHub", "aws": "AWS", "trustline": "Trustline"}

func parseDate(v any) *time.Time {
	s := str(v)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// syncPeople upserts the workforce from an HRIS snapshot and starts onboarding
// for new hires and offboarding for terminations. It runs inside the sync
// transaction that stores the snapshot.
func (s *Service) syncPeople(ctx context.Context, tx pgx.Tx, orgID domain.ID, employees []map[string]any) error {
	var existing int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM people WHERE org_id = $1`, orgID).Scan(&existing); err != nil {
		return err
	}
	initial := existing == 0
	for _, e := range employees {
		email := strings.ToLower(strings.TrimSpace(str(e["email"])))
		if email == "" {
			continue
		}
		status := str(e["status"])
		switch status {
		case "active", "on_leave", "terminated":
		default:
			status = "active"
		}
		hire, term := parseDate(e["hire_date"]), parseDate(e["termination_date"])
		var userID *domain.ID
		var uid domain.ID
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = $1`, email).Scan(&uid); err == nil {
			userID = &uid
		}
		var prev struct {
			id       domain.ID
			status   string
			attested *domain.ID
		}
		err := tx.QueryRow(ctx, `SELECT id, status, background_attested_by FROM people WHERE org_id = $1 AND lower(email) = $2 FOR UPDATE`, orgID, email).Scan(&prev.id, &prev.status, &prev.attested)
		isNew := db.IsNoRows(err)
		if err != nil && !isNew {
			return err
		}
		bg := str(e["background_check"])
		if bg == "" {
			bg = "pending"
		}
		var personID domain.ID
		if isNew {
			personID = domain.NewID()
			var terminatedAt any
			if status == "terminated" {
				terminatedAt = time.Now()
			}
			if _, err := tx.Exec(ctx, `INSERT INTO people (id, org_id, email, name, department, title, manager_email, status, employment_type, hire_date, termination_date, location, background_check, source, external_id, user_id, terminated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'hris',$14,$15,$16)`,
				personID, orgID, email, str(e["name"]), str(e["department"]), str(e["title"]), strings.ToLower(str(e["manager_email"])), status, str(e["employment_type"]),
				hire, term, str(e["location"]), bg, str(e["id"]), userID, terminatedAt); err != nil {
				return err
			}
			if err := activity.Record(ctx, tx, orgID, domain.SystemActor("hris-sync"), activity.Entry{Action: "person.created", TargetType: "person", TargetID: personID.String(), TargetLabel: email,
				Metadata: map[string]any{"status": status, "initial_import": initial}}); err != nil {
				return err
			}
		} else {
			personID = prev.id
			bgExpr := "$13"
			if prev.attested != nil {
				bgExpr = "CASE WHEN $13 = 'completed' THEN $13 ELSE background_check END" // an attestation survives a stale HRIS value
			}
			if _, err := tx.Exec(ctx, `UPDATE people SET name = $3, department = $4, title = $5, manager_email = $6, status = $7, employment_type = $8, hire_date = $9,
				termination_date = $10, location = $11, user_id = COALESCE($12, user_id), background_check = `+bgExpr+`,
				terminated_at = CASE WHEN $7 = 'terminated' THEN COALESCE(terminated_at, now()) ELSE NULL END, updated_at = now()
				WHERE org_id = $1 AND id = $2`, orgID, personID, str(e["name"]), str(e["department"]), str(e["title"]), strings.ToLower(str(e["manager_email"])), status,
				str(e["employment_type"]), hire, term, str(e["location"]), userID, bg); err != nil {
				return err
			}
		}
		recentHire := hire != nil && time.Since(*hire) < 30*24*time.Hour
		switch {
		case status == "active" && isNew && (!initial || recentHire):
			if err := s.startOnboarding(ctx, tx, orgID, personID, email, str(e["name"]), hire); err != nil {
				return err
			}
		case status == "terminated" && (isNew && !initial || !isNew && prev.status != "terminated"):
			if err := s.startOffboarding(ctx, tx, orgID, personID, email, str(e["name"])); err != nil {
				return err
			}
		case status != "terminated" && !isNew && prev.status == "terminated":
			if err := s.reactivate(ctx, tx, orgID, personID, email); err != nil {
				return err
			}
		}
	}
	return nil
}

// startOnboarding creates the onboarding checklist and invites the new hire.
func (s *Service) startOnboarding(ctx context.Context, tx pgx.Tx, orgID, personID domain.ID, email, name string, hire *time.Time) error {
	due := time.Now().Add(30 * 24 * time.Hour)
	if hire != nil {
		due = hire.Add(30 * 24 * time.Hour)
	}
	member, err := membershipByEmail(ctx, tx, orgID, email)
	if err != nil {
		return err
	}
	pid := personID
	items := []workItem{
		{ItemKey: "account", Title: "Create a Trustline account", Description: name + " has been invited to Trustline to complete onboarding.", Priority: "medium"},
		{ItemKey: "policies", Title: "Acknowledge security policies", Description: "Read and acknowledge every published policy.", Priority: "medium", ControlID: controlIDByCode(ctx, tx, orgID, "GOV-05")},
		{ItemKey: "training", Title: "Complete security awareness training", Description: "Pass the Security Awareness Essentials course (80% to pass).", Priority: "medium", ControlID: controlIDByCode(ctx, tx, orgID, "HR-02")},
		{ItemKey: "device", Title: "Enroll your laptop", Description: "Install the Trustline device agent so disk encryption, screen lock and anti-malware are verified.", Priority: "medium", ControlID: controlIDByCode(ctx, tx, orgID, "AST-03")},
		{ItemKey: "background", Title: "Background check", Description: "HR confirms the background check is complete (or not required for this role).", Priority: "medium", ControlID: controlIDByCode(ctx, tx, orgID, "HR-01")},
	}
	for _, it := range items {
		it.Kind = "onboarding"
		it.Title = it.Title + " — " + name
		it.SourceKey = "onboard:" + personID.String() + ":" + it.ItemKey
		it.PersonID = &pid
		it.Due = &due
		if it.ItemKey != "background" && it.ItemKey != "account" {
			it.Assignee = member
		}
		if _, err := s.openWorkItem(ctx, tx, orgID, it, true); err != nil {
			return err
		}
	}
	if member == nil {
		if err := s.inviteNewHire(ctx, tx, orgID, email); err != nil {
			return err
		}
	}
	return activity.Record(ctx, tx, orgID, domain.SystemActor("hris-sync"), activity.Entry{Action: "person.onboarding_started", TargetType: "person", TargetID: personID.String(), TargetLabel: email})
}

func (s *Service) inviteNewHire(ctx context.Context, tx pgx.Tx, orgID domain.ID, email string) error {
	var pending bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_invites WHERE org_id = $1 AND lower(email) = $2 AND accepted_at IS NULL AND expires_at > now())`, orgID, email).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return nil
	}
	svc := org.New(s.Pool)
	o, err := svc.Get(ctx, tx, orgID)
	if err != nil {
		return err
	}
	inv, token, err := svc.CreateInvite(ctx, tx, orgID, email, domain.RoleEmployee, domain.SystemActor("hris-sync"))
	if err != nil {
		return err
	}
	link := s.BaseURL + "/invite/" + token
	html, text := mail.Render(mail.TemplateData{
		Title: "Welcome to " + o.Name, Body: "Your HR record was created, so you have been invited to Trustline to complete your security onboarding: acknowledge policies, take a short training and enroll your laptop.",
		ActionURL: link, ActionLabel: "Start onboarding", Footer: "This invitation expires in 7 days.",
	})
	return s.Jobs.EnqueueTx(ctx, tx, jobs.SendEmailArgs{To: inv.Email, Subject: "Start your security onboarding at " + o.Name, HTML: html, Text: text})
}

// startOffboarding opens one verified removal task per connected system,
// removes Trustline access immediately and alerts admins.
func (s *Service) startOffboarding(ctx context.Context, tx pgx.Tx, orgID, personID domain.ID, email, name string) error {
	due := time.Now().Add(24 * time.Hour)
	pid := personID
	ctl := controlIDByCode(ctx, tx, orgID, "HR-05")
	rows, err := tx.Query(ctx, `SELECT type FROM integrations WHERE org_id = $1 AND type IN ('okta','google_workspace','github','aws') ORDER BY type`, orgID)
	if err != nil {
		return err
	}
	var systems []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		systems = append(systems, t)
	}
	rows.Close()
	steps := map[string][]string{
		"okta":             {"Okta Admin → Directory → People → " + email + " → More actions → Deactivate."},
		"google_workspace": {"Admin console → Directory → Users → " + email + " → Suspend user.", "Transfer Drive and Calendar data to the manager before deleting."},
		"github":           {"GitHub → organization → People → find the member → Remove from organization."},
		"aws":              {"IAM → Users → the user → delete console password, access keys and MFA device, then delete the user."},
	}
	for _, sys := range systems {
		priority := "high"
		if sys == "okta" || (sys == "google_workspace" && !contains(systems, "okta")) {
			priority = "critical"
		}
		if _, err := s.openWorkItem(ctx, tx, orgID, workItem{Kind: "offboarding", ItemKey: sys, Title: fmt.Sprintf("Remove %s's access in %s", name, systemNames[sys]),
			Description: "Trustline verifies removal automatically on the next sync of " + systemNames[sys] + ".", Priority: priority,
			SourceKey: "offboard:" + personID.String() + ":" + sys, PersonID: &pid, ControlID: ctl, Due: &due, Steps: steps[sys],
			Meta: map[string]any{"system": sys, "email": email}}, false); err != nil {
			return err
		}
	}
	var devices int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM devices WHERE org_id = $1 AND (person_id = $2 OR lower(owner_email) = $3)`, orgID, personID, email).Scan(&devices); err != nil {
		return err
	}
	if devices > 0 {
		if _, err := s.openWorkItem(ctx, tx, orgID, workItem{Kind: "offboarding", ItemKey: "devices", Title: fmt.Sprintf("Recover %s's devices", name),
			Description: fmt.Sprintf("Collect %d company device(s) and wipe them before reuse.", devices), Priority: "medium",
			SourceKey: "offboard:" + personID.String() + ":devices", PersonID: &pid, ControlID: controlIDByCode(ctx, tx, orgID, "AST-06"), Due: &due}, false); err != nil {
			return err
		}
	}
	// Trustline removes its own access immediately.
	var mid domain.ID
	var role string
	err = tx.QueryRow(ctx, `SELECT m.id, m.role FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND lower(u.email) = $2`, orgID, email).Scan(&mid, &role)
	switch {
	case db.IsNoRows(err):
	case err != nil:
		return err
	default:
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_memberships WHERE org_id = $1 AND role = 'admin'`, orgID).Scan(&admins); err != nil {
			return err
		}
		if role != "admin" || admins > 1 {
			if _, err := tx.Exec(ctx, `DELETE FROM org_memberships WHERE org_id = $1 AND id = $2`, orgID, mid); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE sessions SET org_id = NULL WHERE org_id = $1 AND user_id = (SELECT id FROM users WHERE lower(email) = $2)`, orgID, email); err != nil {
				return err
			}
			if err := activity.Record(ctx, tx, orgID, domain.SystemActor("hris-sync"), activity.Entry{Action: "member.removed", TargetType: "member", TargetID: mid.String(), TargetLabel: email,
				Metadata: map[string]any{"reason": "terminated in HRIS"}}); err != nil {
				return err
			}
		}
	}
	// Dismiss onboarding that will never finish.
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'dismissed', resolution = 'Employee terminated.', updated_at = now() WHERE org_id = $1 AND person_id = $2 AND kind IN ('onboarding','policy_acceptance') AND status = 'open'`, orgID, personID); err != nil {
		return err
	}
	if err := activity.Record(ctx, tx, orgID, domain.SystemActor("hris-sync"), activity.Entry{Action: "person.offboarding_started", TargetType: "person", TargetID: personID.String(), TargetLabel: email,
		Metadata: map[string]any{"systems": systems}}); err != nil {
		return err
	}
	return s.notifyAdmins(ctx, tx, orgID, "offboarding", name+" was terminated — remove access within 1 business day",
		fmt.Sprintf("Offboarding tasks were created for %d connected system(s). Trustline access was removed automatically.", len(systems)), "/app/people/"+personID.String())
}

func (s *Service) reactivate(ctx context.Context, tx pgx.Tx, orgID, personID domain.ID, email string) error {
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'dismissed', resolution = 'Employee reactivated in the HR system.', updated_at = now()
		WHERE org_id = $1 AND person_id = $2 AND kind = 'offboarding' AND status = 'open'`, orgID, personID); err != nil {
		return err
	}
	return activity.Record(ctx, tx, orgID, domain.SystemActor("hris-sync"), activity.Entry{Action: "person.reactivated", TargetType: "person", TargetID: personID.String(), TargetLabel: email})
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// notifyAdmins sends an in-app notification (and email, if enabled) to admins and editors.
func (s *Service) notifyAdmins(ctx context.Context, tx pgx.Tx, orgID domain.ID, kind, title, body, link string) error {
	return s.notifyUsers(ctx, tx, orgID, `SELECT u.id, u.email FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND m.role IN ('admin','editor')`, []any{orgID}, kind, title, body, link)
}

func (s *Service) notifyUsers(ctx context.Context, tx pgx.Tx, orgID domain.ID, query string, args []any, kind, title, body, link string) error {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	type r struct {
		id    domain.ID
		email string
	}
	var rs []r
	for rows.Next() {
		var x r
		if err := rows.Scan(&x.id, &x.email); err != nil {
			rows.Close()
			return err
		}
		rs = append(rs, x)
	}
	rows.Close()
	o, err := org.New(s.Pool).Get(ctx, tx, orgID)
	if err != nil {
		return err
	}
	for _, x := range rs {
		if _, err := tx.Exec(ctx, `INSERT INTO notifications (id, org_id, user_id, kind, title, body, link) VALUES ($1,$2,$3,$4,$5,$6,$7)`, domain.NewID(), orgID, x.id, kind, title, body, link); err != nil {
			return err
		}
		if o.Settings.EmailNotifications {
			html, text := mail.Render(mail.TemplateData{Title: title, Body: body, ActionURL: s.BaseURL + link, ActionLabel: "Open in Trustline", Footer: o.Name + " · Trustline"})
			if err := s.Jobs.EnqueueTx(ctx, tx, jobs.SendEmailArgs{To: x.email, Subject: "[" + o.Name + "] " + title, HTML: html, Text: text}); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- people API -------------------------------------------------------------

type PersonRow struct {
	ID               domain.ID       `json:"id"`
	Email            string          `json:"email"`
	Name             string          `json:"name"`
	Department       string          `json:"department"`
	Title            string          `json:"title"`
	ManagerEmail     string          `json:"manager_email"`
	Status           string          `json:"status"`
	EmploymentType   string          `json:"employment_type"`
	HireDate         *time.Time      `json:"hire_date,omitempty"`
	TerminationDate  *time.Time      `json:"termination_date,omitempty"`
	TerminatedAt     *time.Time      `json:"terminated_at,omitempty"`
	BackgroundCheck  string          `json:"background_check"`
	HasAccount       bool            `json:"has_account"`
	PoliciesAccepted bool            `json:"policies_accepted"`
	TrainingDone     bool            `json:"training_completed"`
	DeviceEnrolled   bool            `json:"device_enrolled"`
	DeviceCompliant  bool            `json:"device_compliant"`
	OnboardingDone   bool            `json:"onboarding_complete"`
	OpenTasks        int             `json:"open_tasks"`
	Access           map[string]bool `json:"access"`
}

func (s *Service) ListPeople(ctx context.Context, orgID domain.ID, status string) ([]PersonRow, error) {
	facts, err := s.loadPersonFacts(ctx, s.Pool, orgID, s.trainingValidDays())
	if err != nil {
		return nil, err
	}
	out := []PersonRow{}
	for _, f := range facts {
		if status != "" && f.Status != status {
			continue
		}
		row, err := s.personRow(ctx, orgID, f)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, nil
}

func (s *Service) personRow(ctx context.Context, orgID domain.ID, f personFacts) (*PersonRow, error) {
	r := &PersonRow{ID: f.ID, Email: f.Email, Name: f.Name, Status: f.Status, Department: f.Department, EmploymentType: f.EmploymentType, BackgroundCheck: f.Background,
		HasAccount: f.MembershipID != nil, PoliciesAccepted: f.PoliciesOK, TrainingDone: f.TrainingOK, DeviceEnrolled: f.DeviceEnrolled, DeviceCompliant: f.DeviceEnrolled && f.DeviceCompliant,
		OnboardingDone: f.onboardingComplete(), Access: map[string]bool{}}
	if f.Identity.hasOkta {
		r.Access["okta"] = f.Identity.okta
	}
	if f.Identity.hasGoogle {
		r.Access["google_workspace"] = f.Identity.google
	}
	if f.Identity.hasGitHub {
		r.Access["github"] = f.Identity.github
	}
	if f.Identity.hasAWS {
		r.Access["aws"] = f.Identity.aws
	}
	r.Access["trustline"] = f.MembershipID != nil
	if err := s.Pool.QueryRow(ctx, `SELECT title, manager_email, hire_date::timestamptz, termination_date::timestamptz, terminated_at,
		(SELECT count(*) FROM tasks t WHERE t.org_id = p.org_id AND t.person_id = p.id AND t.status = 'open')
		FROM people p WHERE p.org_id = $1 AND p.id = $2`, orgID, f.ID).Scan(&r.Title, &r.ManagerEmail, &r.HireDate, &r.TerminationDate, &r.TerminatedAt, &r.OpenTasks); err != nil {
		return nil, err
	}
	return r, nil
}

type PersonDetail struct {
	PersonRow
	Tasks   []TaskRow `json:"tasks"`
	Devices []Device  `json:"devices"`
}

func (s *Service) GetPerson(ctx context.Context, orgID, id domain.ID) (*PersonDetail, error) {
	facts, err := s.loadPersonFacts(ctx, s.Pool, orgID, s.trainingValidDays())
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		if f.ID != id {
			continue
		}
		row, err := s.personRow(ctx, orgID, f)
		if err != nil {
			return nil, err
		}
		d := &PersonDetail{PersonRow: *row}
		pid := id
		if d.Tasks, err = s.ListTasks(ctx, orgID, TaskFilter{PersonID: &pid}); err != nil {
			return nil, err
		}
		if d.Devices, err = s.ListDevices(ctx, orgID, &f.Email); err != nil {
			return nil, err
		}
		return d, nil
	}
	return nil, domain.ErrNotFound
}

// AttestBackgroundCheck records an HR attestation for a person.
func (s *Service) AttestBackgroundCheck(ctx context.Context, orgID, id domain.ID, status string, actor domain.Actor) error {
	if status != "completed" && status != "not_required" && status != "pending" {
		return domain.Invalid("status", "status must be completed, not_required or pending")
	}
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(ctx, `UPDATE people SET background_check = $3, background_attested_by = $4, updated_at = now() WHERE org_id = $1 AND id = $2 RETURNING email`, orgID, id, status, actor.ID).Scan(&email); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "person.background_check_attested", TargetType: "person", TargetID: id.String(), TargetLabel: email, Metadata: map[string]any{"status": status}}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

// CompleteManualItem lets an admin close a manual offboarding/onboarding item (e.g. device recovery).
func (s *Service) CompleteManualItem(ctx context.Context, orgID, taskID domain.ID, note string, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title, item string
		if err := tx.QueryRow(ctx, `UPDATE tasks SET status = 'done', resolution = $3, completed_at = now(), completed_by = $4, updated_at = now()
			WHERE org_id = $1 AND id = $2 AND status = 'open' RETURNING title, item_key`, orgID, taskID, "Completed manually: "+note, actor.ID).Scan(&title, &item); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "task.completed_manually", TargetType: "task", TargetID: taskID.String(), TargetLabel: title, Metadata: map[string]any{"item": item, "note": note}})
	})
}
