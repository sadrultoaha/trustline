package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/org"
)

// The platform integration (type "trustline", mode "platform") exposes
// Trustline's own records to the rule engine. It exists for every org that has
// a framework in scope and is synced on the scheduler tick and whenever the
// underlying records change.

func (s *Service) ensurePlatform(ctx context.Context, tx pgx.Tx, orgID domain.ID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE org_id = $1 AND type = 'trustline')`, orgID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	id := domain.NewID()
	creds, err := s.Box.SealJSON(map[string]string{}, id[:])
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO integrations (id, org_id, type, name, mode, credentials_enc, status) VALUES ($1,$2,'trustline','Trustline platform','platform',$3,'pending')`, id, orgID, creds); err != nil {
		return err
	}
	return s.Jobs.EnqueueTx(ctx, tx, jobs.PlatformSyncArgs{OrgID: orgID.String()})
}

// PlatformIntegrationID returns the org's platform integration, if any.
func (s *Service) PlatformIntegrationID(ctx context.Context, orgID domain.ID) (*domain.ID, error) {
	var id domain.ID
	err := s.Pool.QueryRow(ctx, `SELECT id FROM integrations WHERE org_id = $1 AND type = 'trustline'`, orgID).Scan(&id)
	if db.IsNoRows(err) {
		return nil, nil
	}
	return &id, err
}

// touchPlatform re-syncs platform data after a change to policies, people,
// training, devices or access reviews.
func (s *Service) touchPlatform(ctx context.Context, q db.Querier, orgID domain.ID) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE org_id = $1 AND type = 'trustline')`, orgID).Scan(&exists); err != nil || !exists {
		return err
	}
	args := jobs.PlatformSyncArgs{OrgID: orgID.String()}
	if tx, ok := q.(pgx.Tx); ok {
		return s.Jobs.EnqueueTx(ctx, tx, args)
	}
	return s.Jobs.Enqueue(ctx, args)
}

// identity is the access a person has across connected systems.
type identity struct {
	okta, google, github, aws             bool
	hasOkta, hasGoogle, hasGitHub, hasAWS bool
}

type identityIndex struct {
	hasOkta, hasGoogle, hasGitHub, hasAWS bool
	okta                                  map[string]bool // email → active
	google                                map[string]bool // email → not suspended
	github                                map[string]bool // email → member
	aws                                   map[string]bool // login → console user exists
}

func (ix identityIndex) of(email string) identity {
	e := strings.ToLower(email)
	login := e
	if i := strings.Index(e, "@"); i >= 0 {
		login = e[:i]
	}
	return identity{okta: ix.okta[e], google: ix.google[e], github: ix.github[e], aws: ix.aws[login],
		hasOkta: ix.hasOkta, hasGoogle: ix.hasGoogle, hasGitHub: ix.hasGitHub, hasAWS: ix.hasAWS}
}

func (id identity) idpActive() bool {
	if id.hasOkta {
		return id.okta
	}
	return id.google
}

func (s *Service) identityIndex(ctx context.Context, q db.Querier, orgID domain.ID) (identityIndex, error) {
	ix := identityIndex{okta: map[string]bool{}, google: map[string]bool{}, github: map[string]bool{}, aws: map[string]bool{}}
	rows, err := q.Query(ctx, `SELECT DISTINCT ON (connector) connector, data FROM snapshots
		WHERE org_id = $1 AND integration_id IS NOT NULL AND connector IN ('okta','google_workspace','github','aws')
		ORDER BY connector, taken_at DESC`, orgID)
	if err != nil {
		return ix, err
	}
	defer rows.Close()
	for rows.Next() {
		var conn string
		var raw []byte
		if err := rows.Scan(&conn, &raw); err != nil {
			return ix, err
		}
		var res map[string][]map[string]any
		if err := json.Unmarshal(raw, &res); err != nil {
			return ix, err
		}
		switch conn {
		case "okta":
			ix.hasOkta = true
			for _, u := range res["user"] {
				ix.okta[strings.ToLower(str(u["email"]))] = str(u["status"]) == "ACTIVE"
			}
		case "google_workspace":
			ix.hasGoogle = true
			for _, u := range res["user"] {
				ix.google[strings.ToLower(str(u["id"]))] = u["suspended"] != true
			}
		case "github":
			ix.hasGitHub = true
			for _, m := range res["member"] {
				ix.github[strings.ToLower(str(m["email"]))] = true
			}
		case "aws":
			ix.hasAWS = true
			for _, u := range res["iam_user"] {
				if u["console_access"] == true {
					ix.aws[strings.ToLower(str(u["id"]))] = true
				}
			}
		}
	}
	return ix, rows.Err()
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func daysSince(t *time.Time) float64 {
	if t == nil {
		return 9999
	}
	return float64(int(time.Since(*t).Hours() / 24))
}

// personFacts holds the derived per-person compliance state.
type personFacts struct {
	ID              domain.ID
	Email, Name     string
	Status          string
	Department      string
	EmploymentType  string
	HireDate        *time.Time
	TerminatedAt    *time.Time
	Background      string
	UserID          *domain.ID
	MembershipID    *domain.ID
	PoliciesOK      bool
	TrainingOK      bool
	DeviceEnrolled  bool
	DeviceCompliant bool
	Identity        identity
}

func (p personFacts) onboardingComplete() bool {
	return p.PoliciesOK && p.TrainingOK && p.DeviceEnrolled && (p.Background == "completed" || p.Background == "not_required")
}

func (p personFacts) accessRevoked() bool {
	return !p.Identity.okta && !p.Identity.google && !p.Identity.github && !p.Identity.aws && p.MembershipID == nil
}

type policyFacts struct {
	PolicyID, VersionID domain.ID
	Slug, Title         string
	Version             int
	Audience, Accepted  int
	PublishedAt         time.Time
	LastReviewed        *time.Time
	NextReview          *time.Time
}

const deviceCompliantSQL = `d.disk_encrypted AND COALESCE(d.screen_lock_minutes, 999) <= 15 AND d.antivirus_running AND d.last_seen_at > now() - interval '7 days'`

func (s *Service) loadPolicyFacts(ctx context.Context, q db.Querier, orgID domain.ID) ([]policyFacts, error) {
	rows, err := q.Query(ctx, `
		WITH audience AS (
		  SELECT m.user_id FROM org_memberships m WHERE m.org_id = $1 AND m.role IN ('admin','editor','employee')
		)
		SELECT p.id, v.id, p.slug, p.title, v.version, v.published_at, p.last_reviewed_at, p.next_review_at,
		       (SELECT count(*) FROM audience),
		       (SELECT count(*) FROM policy_acceptances a JOIN audience au ON au.user_id = a.user_id WHERE a.policy_version_id = v.id)
		FROM policies p JOIN policy_versions v ON v.id = p.current_version_id
		WHERE p.org_id = $1 AND v.status = 'published' ORDER BY p.title`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []policyFacts
	for rows.Next() {
		var f policyFacts
		if err := rows.Scan(&f.PolicyID, &f.VersionID, &f.Slug, &f.Title, &f.Version, &f.PublishedAt, &f.LastReviewed, &f.NextReview, &f.Audience, &f.Accepted); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// linkPeople connects HR records to Trustline accounts that signed up later,
// and hands their open onboarding items to them.
func (s *Service) linkPeople(ctx context.Context, q db.Querier, orgID domain.ID) error {
	if _, err := q.Exec(ctx, `UPDATE people p SET user_id = u.id, updated_at = now() FROM users u
		WHERE p.org_id = $1 AND p.user_id IS NULL AND lower(u.email) = lower(p.email)`, orgID); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE tasks t SET assignee_membership_id = m.id, updated_at = now()
		FROM people p JOIN org_memberships m ON m.org_id = p.org_id AND m.user_id = p.user_id
		WHERE t.org_id = $1 AND t.person_id = p.id AND t.kind = 'onboarding' AND t.status = 'open'
		  AND t.assignee_membership_id IS NULL AND t.item_key IN ('policies','training','device')`, orgID)
	return err
}

func (s *Service) loadPersonFacts(ctx context.Context, q db.Querier, orgID domain.ID, validDays int) ([]personFacts, error) {
	if err := s.linkPeople(ctx, q, orgID); err != nil {
		return nil, err
	}
	ix, err := s.identityIndex(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT p.id, p.email, p.name, p.status, p.department, p.employment_type, p.hire_date::timestamptz, p.terminated_at, p.background_check, p.user_id, m.id,
		  (m.id IS NOT NULL AND NOT EXISTS (
		     SELECT 1 FROM policies pol JOIN policy_versions v ON v.id = pol.current_version_id AND v.status = 'published'
		     WHERE pol.org_id = p.org_id AND NOT EXISTS (SELECT 1 FROM policy_acceptances a WHERE a.policy_version_id = v.id AND a.user_id = p.user_id))),
		  EXISTS (SELECT 1 FROM training_completions t WHERE t.org_id = p.org_id AND t.user_id = p.user_id AND t.passed AND t.completed_at > now() - make_interval(days => $2)),
		  EXISTS (SELECT 1 FROM devices d WHERE d.org_id = p.org_id AND (d.person_id = p.id OR lower(d.owner_email) = lower(p.email))),
		  NOT EXISTS (SELECT 1 FROM devices d WHERE d.org_id = p.org_id AND (d.person_id = p.id OR lower(d.owner_email) = lower(p.email)) AND NOT (`+deviceCompliantSQL+`))
		FROM people p
		LEFT JOIN org_memberships m ON m.org_id = p.org_id AND m.user_id = p.user_id
		WHERE p.org_id = $1 ORDER BY p.name`, orgID, validDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []personFacts
	for rows.Next() {
		var f personFacts
		if err := rows.Scan(&f.ID, &f.Email, &f.Name, &f.Status, &f.Department, &f.EmploymentType, &f.HireDate, &f.TerminatedAt, &f.Background, &f.UserID, &f.MembershipID,
			&f.PoliciesOK, &f.TrainingOK, &f.DeviceEnrolled, &f.DeviceCompliant); err != nil {
			return nil, err
		}
		f.Identity = ix.of(f.Email)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Service) trainingValidDays() int {
	if len(s.Catalog.Training) > 0 && s.Catalog.Training[0].ValidDays > 0 {
		return s.Catalog.Training[0].ValidDays
	}
	return 365
}

// buildPlatformSnapshot assembles the "trustline" connector resources.
func (s *Service) buildPlatformSnapshot(ctx context.Context, q db.Querier, orgID domain.ID) (map[string][]map[string]any, []personFacts, []policyFacts, error) {
	o, err := org.New(s.Pool).Get(ctx, q, orgID)
	if err != nil {
		return nil, nil, nil, err
	}
	res := map[string][]map[string]any{"policy": {}, "person": {}, "device": {}, "access_review_program": {}, "risk": {}, "risk_program": {}, "vendor": {}}
	if err := s.riskAndVendorResources(ctx, q, orgID, res); err != nil {
		return nil, nil, nil, err
	}
	pols, err := s.loadPolicyFacts(ctx, q, orgID)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, p := range pols {
		rate := 100.0
		if p.Audience > 0 {
			rate = float64(int(float64(p.Accepted)*1000/float64(p.Audience))) / 10
		}
		reviewed := p.PublishedAt
		if p.LastReviewed != nil && p.LastReviewed.After(reviewed) {
			reviewed = *p.LastReviewed
		}
		res["policy"] = append(res["policy"], map[string]any{
			"id": p.Slug, "title": p.Title, "published": true, "version": p.Version, "acceptance_rate": rate,
			"acceptance_threshold_met": rate >= float64(o.Settings.PolicyAcceptanceThreshold), "pending_acceptances": p.Audience - p.Accepted,
			"review_overdue": p.NextReview != nil && p.NextReview.Before(time.Now()), "days_since_review": daysSince(&reviewed),
		})
	}
	people, err := s.loadPersonFacts(ctx, q, orgID, s.trainingValidDays())
	if err != nil {
		return nil, nil, nil, err
	}
	for _, p := range people {
		res["person"] = append(res["person"], map[string]any{
			"id": strings.ToLower(p.Email), "name": p.Name, "status": p.Status, "department": p.Department, "employment_type": p.EmploymentType,
			"hire_days": daysSince(p.HireDate), "terminated_days": daysSince(p.TerminatedAt), "has_account": p.MembershipID != nil,
			"onboarding_complete": p.onboardingComplete(), "policies_accepted": p.PoliciesOK, "training_completed": p.TrainingOK,
			"device_enrolled": p.DeviceEnrolled, "device_compliant": p.DeviceEnrolled && p.DeviceCompliant, "background_check": p.Background,
			"idp_active": p.Identity.idpActive(), "okta_active": p.Identity.okta, "google_active": p.Identity.google,
			"github_member": p.Identity.github, "aws_console_active": p.Identity.aws, "access_revoked": p.accessRevoked(),
		})
	}
	drows, err := q.Query(ctx, `SELECT d.serial, d.hostname, d.owner_email, d.platform, d.disk_encrypted, d.screen_lock_minutes, d.antivirus_running, d.last_seen_at, `+deviceCompliantSQL+`
		FROM devices d WHERE d.org_id = $1 ORDER BY d.serial`, orgID)
	if err != nil {
		return nil, nil, nil, err
	}
	for drows.Next() {
		var serial, host, owner, platform string
		var enc, av, compliant bool
		var lock *int
		var seen time.Time
		if err := drows.Scan(&serial, &host, &owner, &platform, &enc, &lock, &av, &seen, &compliant); err != nil {
			drows.Close()
			return nil, nil, nil, err
		}
		d := map[string]any{"id": serial, "hostname": host, "owner": owner, "platform": platform, "disk_encrypted": enc, "antivirus_running": av,
			"last_seen_days": daysSince(&seen), "compliant": compliant}
		if lock != nil {
			d["screen_lock_minutes"] = *lock
		}
		res["device"] = append(res["device"], d)
	}
	drows.Close()
	// The access review program only applies once an identity system is connected.
	var reviewable bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE org_id = $1 AND type IN ('okta','google_workspace','github','aws'))`, orgID).Scan(&reviewable); err != nil {
		return nil, nil, nil, err
	}
	if !reviewable {
		return res, people, pols, nil
	}
	var lastCompleted *time.Time
	var open, overdue int
	if err := q.QueryRow(ctx, `SELECT max(completed_at) FILTER (WHERE status = 'completed'), count(*) FILTER (WHERE status = 'open'), count(*) FILTER (WHERE status = 'open' AND due_at < now())
		FROM access_review_campaigns WHERE org_id = $1`, orgID).Scan(&lastCompleted, &open, &overdue); err != nil {
		return nil, nil, nil, err
	}
	last := daysSince(lastCompleted)
	res["access_review_program"] = append(res["access_review_program"], map[string]any{
		"id": "access-reviews", "last_completed_days": last, "overdue": last > float64(o.Settings.AccessReviewDays),
		"open_campaigns": open, "overdue_campaigns": overdue,
	})
	return res, people, pols, nil
}

// reconcile closes system-managed tasks whose condition is now met and opens
// acknowledgement tasks for staff who still need to accept a policy version.
func (s *Service) reconcile(ctx context.Context, tx pgx.Tx, orgID domain.ID, people []personFacts, pols []policyFacts) error {
	// Policy acknowledgement tasks for every audience member lacking an acceptance.
	rows, err := tx.Query(ctx, `SELECT m.id, m.user_id FROM org_memberships m WHERE m.org_id = $1 AND m.role IN ('admin','editor','employee')`, orgID)
	if err != nil {
		return err
	}
	type member struct{ id, user domain.ID }
	var members []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.id, &m.user); err != nil {
			rows.Close()
			return err
		}
		members = append(members, m)
	}
	rows.Close()
	current := map[string]bool{}
	for _, p := range pols {
		current[p.VersionID.String()] = true
		accepted := map[domain.ID]bool{}
		arows, err := tx.Query(ctx, `SELECT user_id FROM policy_acceptances WHERE org_id = $1 AND policy_version_id = $2`, orgID, p.VersionID)
		if err != nil {
			return err
		}
		for arows.Next() {
			var u domain.ID
			if err := arows.Scan(&u); err != nil {
				arows.Close()
				return err
			}
			accepted[u] = true
		}
		arows.Close()
		due := p.PublishedAt.Add(14 * 24 * time.Hour)
		var doneKeys []string
		for _, m := range members {
			key := "accept:" + p.VersionID.String() + ":" + m.id.String()
			if accepted[m.user] {
				doneKeys = append(doneKeys, key)
				continue
			}
			mid := m.id
			if _, err := s.openWorkItem(ctx, tx, orgID, workItem{Kind: "policy_acceptance", Title: fmt.Sprintf("Acknowledge %s (v%d)", p.Title, p.Version),
				Description: "Read the policy and confirm you understand and will follow it.", SourceKey: key, Assignee: &mid, Due: &due, Priority: "medium",
				ControlID: controlIDByCode(ctx, tx, orgID, "GOV-05"), Meta: map[string]any{"policy_id": p.PolicyID, "version_id": p.VersionID, "policy_slug": p.Slug}}, false); err != nil {
				return err
			}
		}
		if _, err := s.closeWorkItems(ctx, tx, orgID, doneKeys, "done", "Acknowledged."); err != nil {
			return err
		}
	}
	// Acknowledgement tasks for versions that are no longer current.
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'dismissed', resolution = 'A newer version was published.', updated_at = now()
		WHERE org_id = $1 AND kind = 'policy_acceptance' AND status = 'open' AND NOT (meta->>'version_id' = ANY($2))`, orgID, keys(current)); err != nil {
		return err
	}
	// Onboarding and offboarding items verified from data.
	now := time.Now().UTC().Format("2006-01-02 15:04 MST")
	var done []string
	for _, p := range people {
		id := p.ID.String()
		if p.PoliciesOK {
			done = append(done, "onboard:"+id+":policies")
		}
		if p.TrainingOK {
			done = append(done, "onboard:"+id+":training")
		}
		if p.DeviceEnrolled {
			done = append(done, "onboard:"+id+":device")
		}
		if p.Background == "completed" || p.Background == "not_required" {
			done = append(done, "onboard:"+id+":background")
		}
		if p.MembershipID != nil {
			done = append(done, "onboard:"+id+":account")
		}
		if p.Status == "terminated" {
			ident := p.Identity
			for item, active := range map[string]bool{"okta": ident.okta, "google_workspace": ident.google, "github": ident.github, "aws": ident.aws, "trustline": p.MembershipID != nil} {
				if !active {
					done = append(done, "offboard:"+id+":"+item)
				}
			}
		}
	}
	if _, err := s.closeWorkItems(ctx, tx, orgID, done, "done", "Verified automatically on "+now+"."); err != nil {
		return err
	}
	// Access revocations from reviews: verified when the account no longer has access.
	ix, err := s.identityIndex(ctx, tx, orgID)
	if err != nil {
		return err
	}
	trows, err := tx.Query(ctx, `SELECT source_key, meta FROM tasks WHERE org_id = $1 AND kind = 'access_revocation' AND status = 'open'`, orgID)
	if err != nil {
		return err
	}
	var revoked []string
	for trows.Next() {
		var key string
		var raw []byte
		if err := trows.Scan(&key, &raw); err != nil {
			trows.Close()
			return err
		}
		var meta struct {
			Integration string `json:"integration_type"`
			Account     string `json:"account_id"`
			Email       string `json:"email"`
		}
		_ = json.Unmarshal(raw, &meta)
		if !accountActive(ix, meta.Integration, meta.Account, meta.Email) {
			revoked = append(revoked, key)
		}
	}
	trows.Close()
	_, err = s.closeWorkItems(ctx, tx, orgID, revoked, "done", "Access removal verified automatically on "+now+".")
	return err
}

func accountActive(ix identityIndex, integration, account, email string) bool {
	e := strings.ToLower(email)
	switch integration {
	case "okta":
		return ix.okta[e]
	case "google_workspace":
		return ix.google[e]
	case "github":
		return ix.github[e]
	case "aws":
		return ix.aws[strings.ToLower(account)]
	}
	return true
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (s *Service) riskAndVendorResources(ctx context.Context, q db.Querier, orgID domain.ID, res map[string][]map[string]any) error {
	rows, err := q.Query(ctx, `SELECT r.id, r.title, r.status, r.likelihood * r.impact, COALESCE(r.residual_likelihood * r.residual_impact, r.likelihood * r.impact), r.treatment,
		r.owner_membership_id IS NOT NULL, length(trim(r.treatment_plan)) > 0, COALESCE(r.next_review_at < now(), false),
		(SELECT count(*) FROM risk_controls rc WHERE rc.risk_id = r.id)
		FROM risks r WHERE r.org_id = $1 AND r.status IN ('open','in_treatment','accepted','closed')`, orgID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id domain.ID
		var title, status, treatment string
		var score, residual, linked int
		var owner, plan, overdue bool
		if err := rows.Scan(&id, &title, &status, &score, &residual, &treatment, &owner, &plan, &overdue, &linked); err != nil {
			rows.Close()
			return err
		}
		res["risk"] = append(res["risk"], map[string]any{"id": id.String(), "title": title, "status": status, "score": score, "residual_score": residual, "treatment": treatment,
			"has_owner": owner, "has_treatment_plan": plan, "review_overdue": overdue, "linked_controls": linked})
	}
	rows.Close()
	var last *time.Time
	var open, suggested int
	if err := q.QueryRow(ctx, `SELECT (SELECT max(created_at) FROM risk_assessments WHERE org_id = $1),
		(SELECT count(*) FROM risks WHERE org_id = $1 AND status IN ('open','in_treatment')),
		(SELECT count(*) FROM risks WHERE org_id = $1 AND status = 'suggested')`, orgID).Scan(&last, &open, &suggested); err != nil {
		return err
	}
	res["risk_program"] = append(res["risk_program"], map[string]any{"id": "risk-management", "last_assessment_days": daysSince(last), "open_risks": open, "suggested_risks": suggested})
	vendors, err := s.ListVendors(ctx, orgID, "")
	if err != nil {
		return err
	}
	for _, v := range vendors {
		if v.Status == "offboarded" {
			continue
		}
		assessed := v.LastReviewedAt != nil && v.LastReviewedAt.After(time.Now().Add(-time.Duration(v.ReviewCycleDays)*24*time.Hour))
		res["vendor"] = append(res["vendor"], map[string]any{"id": v.Name, "status": v.Status, "tier": v.Tier, "data_access": v.DataAccess, "handles_pii": v.HandlesPII,
			"has_owner": v.Owner != nil, "has_security_report": v.HasReport, "has_dpa": v.HasDPA, "assessed": assessed, "review_overdue": v.ReviewOverdue,
			"discovered_days": daysSince(v.DiscoveredAt)})
	}
	return nil
}
