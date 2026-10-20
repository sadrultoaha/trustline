package compliance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/org"
)

var reviewableSystems = []string{"okta", "google_workspace", "github", "aws"}

type Campaign struct {
	ID          domain.ID  `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Systems     []string   `json:"systems"`
	DueAt       time.Time  `json:"due_at"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	EvidenceID  *domain.ID `json:"evidence_id,omitempty"`
	Total       int        `json:"total"`
	Decided     int        `json:"decided"`
	Revoked     int        `json:"revoked"`
	Overdue     bool       `json:"overdue"`
}

type ReviewItem struct {
	ID              domain.ID  `json:"id"`
	CampaignID      domain.ID  `json:"campaign_id"`
	CampaignName    string     `json:"campaign_name"`
	IntegrationType string     `json:"integration_type"`
	AccountID       string     `json:"account_id"`
	AccountLabel    string     `json:"account_label"`
	Email           string     `json:"email"`
	PersonID        *domain.ID `json:"person_id,omitempty"`
	PersonStatus    string     `json:"person_status"`
	Entitlements    []string   `json:"entitlements"`
	IsAdmin         bool       `json:"is_admin"`
	Reviewer        *Owner     `json:"reviewer,omitempty"`
	Decision        string     `json:"decision"`
	Comment         string     `json:"comment"`
	DecidedBy       string     `json:"decided_by"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	TaskID          *domain.ID `json:"revocation_task_id,omitempty"`
}

const campaignSelect = `SELECT c.id, c.name, c.status, c.systems, c.due_at, COALESCE(u.email,''), c.created_at, c.completed_at, c.evidence_id,
  (SELECT count(*) FROM access_review_items i WHERE i.campaign_id = c.id),
  (SELECT count(*) FROM access_review_items i WHERE i.campaign_id = c.id AND i.decision <> 'pending'),
  (SELECT count(*) FROM access_review_items i WHERE i.campaign_id = c.id AND i.decision = 'revoke')
FROM access_review_campaigns c LEFT JOIN users u ON u.id = c.created_by`

func scanCampaign(row pgx.Row) (*Campaign, error) {
	var c Campaign
	if err := row.Scan(&c.ID, &c.Name, &c.Status, &c.Systems, &c.DueAt, &c.CreatedBy, &c.CreatedAt, &c.CompletedAt, &c.EvidenceID, &c.Total, &c.Decided, &c.Revoked); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	c.Overdue = c.Status == "open" && c.DueAt.Before(time.Now())
	return &c, nil
}

func (s *Service) ListCampaigns(ctx context.Context, orgID domain.ID) ([]Campaign, error) {
	rows, err := s.Pool.Query(ctx, campaignSelect+` WHERE c.org_id = $1 ORDER BY c.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

const itemSelect = `SELECT i.id, i.campaign_id, c.name, i.integration_type, i.account_id, i.account_label, i.email, i.person_id, i.person_status, i.entitlements, i.is_admin,
  m.id, u.name, u.email, i.decision, i.comment, COALESCE(du.email,''), i.decided_at,
  (SELECT t.id FROM tasks t WHERE t.org_id = i.org_id AND t.source_key = 'revoke:' || i.id::text ORDER BY t.created_at DESC LIMIT 1)
FROM access_review_items i
JOIN access_review_campaigns c ON c.id = i.campaign_id
LEFT JOIN org_memberships m ON m.id = i.reviewer_membership_id
LEFT JOIN users u ON u.id = m.user_id
LEFT JOIN users du ON du.id = i.decided_by`

func scanItem(row pgx.Row) (*ReviewItem, error) {
	var it ReviewItem
	var mid *domain.ID
	var mname, memail *string
	if err := row.Scan(&it.ID, &it.CampaignID, &it.CampaignName, &it.IntegrationType, &it.AccountID, &it.AccountLabel, &it.Email, &it.PersonID, &it.PersonStatus, &it.Entitlements, &it.IsAdmin,
		&mid, &mname, &memail, &it.Decision, &it.Comment, &it.DecidedBy, &it.DecidedAt, &it.TaskID); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if mid != nil {
		it.Reviewer = &Owner{MembershipID: *mid, Name: deref(mname), Email: deref(memail)}
	}
	if it.Entitlements == nil {
		it.Entitlements = []string{}
	}
	return &it, nil
}

func (s *Service) listItems(ctx context.Context, where string, args ...any) ([]ReviewItem, error) {
	rows, err := s.Pool.Query(ctx, itemSelect+where+` ORDER BY i.decision <> 'pending', i.integration_type, i.account_label`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

type CampaignDetail struct {
	Campaign
	Items []ReviewItem `json:"items"`
}

func (s *Service) GetCampaign(ctx context.Context, orgID, id domain.ID) (*CampaignDetail, error) {
	c, err := scanCampaign(s.Pool.QueryRow(ctx, campaignSelect+` WHERE c.org_id = $1 AND c.id = $2`, orgID, id))
	if err != nil {
		return nil, err
	}
	items, err := s.listItems(ctx, ` WHERE i.org_id = $1 AND i.campaign_id = $2`, orgID, id)
	if err != nil {
		return nil, err
	}
	return &CampaignDetail{Campaign: *c, Items: items}, nil
}

// ItemsForReviewer lists pending items assigned to a membership.
func (s *Service) ItemsForReviewer(ctx context.Context, orgID, membershipID domain.ID) ([]ReviewItem, error) {
	return s.listItems(ctx, ` WHERE i.org_id = $1 AND i.reviewer_membership_id = $2 AND c.status = 'open'`, orgID, membershipID)
}

type CampaignInput struct {
	Name    string   `json:"name"`
	Systems []string `json:"systems"`
	DueDays int      `json:"due_days"`
}

type account struct {
	integration, id, label, email string
	entitlements                  []string
	admin                         bool
}

// accountsFromSnapshots pulls users and roles per system from the latest snapshots.
func (s *Service) accountsFromSnapshots(ctx context.Context, q db.Querier, orgID domain.ID, systems []string) ([]account, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT ON (connector) connector, data FROM snapshots WHERE org_id = $1 AND integration_id IS NOT NULL AND connector = ANY($2) ORDER BY connector, taken_at DESC`, orgID, systems)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []account
	for rows.Next() {
		var conn string
		var raw []byte
		if err := rows.Scan(&conn, &raw); err != nil {
			return nil, err
		}
		var res map[string][]map[string]any
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, err
		}
		switch conn {
		case "okta":
			for _, u := range res["user"] {
				if str(u["status"]) != "ACTIVE" {
					continue
				}
				var ents []string
				for _, g := range asList(u["groups"]) {
					ents = append(ents, "group:"+g)
				}
				if u["admin"] == true {
					ents = append(ents, "Super Administrator")
				}
				out = append(out, account{conn, str(u["id"]), str(u["name"]), strings.ToLower(str(u["email"])), ents, u["admin"] == true})
			}
		case "google_workspace":
			for _, u := range res["user"] {
				if u["suspended"] == true {
					continue
				}
				ents := []string{"org unit " + str(u["org_unit"])}
				if u["is_admin"] == true {
					ents = append(ents, "Super Admin")
				}
				out = append(out, account{conn, str(u["id"]), str(u["name"]), strings.ToLower(str(u["id"])), ents, u["is_admin"] == true})
			}
		case "github":
			for _, m := range res["member"] {
				role := str(m["role"])
				out = append(out, account{conn, str(m["id"]), str(m["id"]), strings.ToLower(str(m["email"])), []string{"org " + role}, role == "admin"})
			}
		case "aws":
			for _, u := range res["iam_user"] {
				var ents []string
				if u["console_access"] == true {
					ents = append(ents, "console")
				}
				if len(asList(u["access_keys"])) > 0 || len(anyList(u["access_keys"])) > 0 {
					ents = append(ents, "access keys")
				}
				if u["admin"] == true {
					ents = append(ents, "AdministratorAccess")
				}
				email := ""
				if u["is_service_account"] != true {
					email = strings.ToLower(str(u["id"]))
				}
				out = append(out, account{conn, str(u["id"]), str(u["id"]), email, ents, u["admin"] == true})
			}
		}
	}
	return out, rows.Err()
}

func asList(v any) []string {
	arr, _ := v.([]any)
	var out []string
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func anyList(v any) []any {
	arr, _ := v.([]any)
	return arr
}

// CreateCampaign snapshots the current accounts of the chosen systems and
// assigns each to the account owner's manager (falling back to the creator).
func (s *Service) CreateCampaign(ctx context.Context, orgID domain.ID, in CampaignInput, actor domain.Actor) (*CampaignDetail, error) {
	v := domain.NewValidation()
	if strings.TrimSpace(in.Name) == "" {
		in.Name = "Access review " + time.Now().Format("2006-01-02")
	}
	if in.DueDays <= 0 {
		in.DueDays = 14
	}
	if in.DueDays > 90 {
		v.Add("due_days", "due within 90 days at most")
	}
	if len(in.Systems) == 0 {
		v.Add("systems", "choose at least one system")
	}
	for _, sys := range in.Systems {
		if !contains(reviewableSystems, sys) {
			v.Add("systems", "unsupported system "+sys)
		}
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	sort.Strings(in.Systems)
	id := domain.NewID()
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var connected []string
		rows, err := tx.Query(ctx, `SELECT type FROM integrations WHERE org_id = $1 AND type = ANY($2)`, orgID, in.Systems)
		if err != nil {
			return err
		}
		for rows.Next() {
			var t string
			_ = rows.Scan(&t)
			connected = append(connected, t)
		}
		rows.Close()
		for _, sys := range in.Systems {
			if !contains(connected, sys) {
				return domain.Invalid("systems", systemNames[sys]+" is not connected")
			}
		}
		accounts, err := s.accountsFromSnapshots(ctx, tx, orgID, in.Systems)
		if err != nil {
			return err
		}
		if len(accounts) == 0 {
			return domain.Invalid("systems", "no accounts have been synced from these systems yet")
		}
		var creatorMembership *domain.ID
		if actor.ID != nil {
			var m domain.ID
			if err := tx.QueryRow(ctx, `SELECT id FROM org_memberships WHERE org_id = $1 AND user_id = $2`, orgID, *actor.ID).Scan(&m); err == nil {
				creatorMembership = &m
			}
		}
		if creatorMembership == nil {
			var m domain.ID
			if err := tx.QueryRow(ctx, `SELECT id FROM org_memberships WHERE org_id = $1 AND role = 'admin' ORDER BY created_at LIMIT 1`, orgID).Scan(&m); err == nil {
				creatorMembership = &m
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO access_review_campaigns (id, org_id, name, systems, due_at, created_by) VALUES ($1,$2,$3,$4, now() + make_interval(days => $5), $6)`,
			id, orgID, strings.TrimSpace(in.Name), in.Systems, in.DueDays, actor.ID); err != nil {
			return err
		}
		reviewers := map[domain.ID]bool{}
		for _, a := range accounts {
			var personID *domain.ID
			var status, manager string
			if a.email != "" {
				var pid domain.ID
				if err := tx.QueryRow(ctx, `SELECT id, status, manager_email FROM people WHERE org_id = $1 AND (lower(email) = $2 OR split_part(lower(email), '@', 1) = $2)`, orgID, a.email).Scan(&pid, &status, &manager); err == nil {
					personID = &pid
				}
			}
			reviewer := creatorMembership
			if manager != "" {
				if m, err := membershipByEmail(ctx, tx, orgID, manager); err == nil && m != nil {
					reviewer = m
				}
			}
			if reviewer != nil {
				reviewers[*reviewer] = true
			}
			ents := a.entitlements
			if ents == nil {
				ents = []string{}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO access_review_items (id, org_id, campaign_id, integration_type, account_id, account_label, email, person_id, person_status, entitlements, is_admin, reviewer_membership_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, domain.NewID(), orgID, id, a.integration, a.id, a.label, a.email, personID, status, ents, a.admin, reviewer); err != nil {
				return err
			}
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "access_review.created", TargetType: "access_review", TargetID: id.String(), TargetLabel: in.Name,
			Metadata: map[string]any{"systems": in.Systems, "accounts": len(accounts), "reviewers": len(reviewers)}}); err != nil {
			return err
		}
		var ids []any
		for r := range reviewers {
			ids = append(ids, r)
		}
		for _, r := range ids {
			if err := s.notifyUsers(ctx, tx, orgID, `SELECT u.id, u.email FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.id = $1`, []any{r},
				"access_review", "Access review assigned: "+in.Name, fmt.Sprintf("Approve or revoke the accounts assigned to you within %d days.", in.DueDays), "/app/my-work"); err != nil {
				return err
			}
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetCampaign(ctx, orgID, id)
}

// Decide records a reviewer decision. Revocations open a verified removal task.
func (s *Service) Decide(ctx context.Context, orgID, itemID domain.ID, decision, comment string, actor domain.Actor, role domain.Role, membership domain.ID) (*ReviewItem, error) {
	if decision != "approve" && decision != "revoke" {
		return nil, domain.Invalid("decision", "decision must be approve or revoke")
	}
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		it, err := scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.org_id = $1 AND i.id = $2 FOR UPDATE OF i`, orgID, itemID))
		if err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM access_review_campaigns WHERE id = $1`, it.CampaignID).Scan(&status); err != nil {
			return err
		}
		if status != "open" {
			return domain.Invalid("campaign", "this campaign is closed")
		}
		if role != domain.RoleAdmin && role != domain.RoleEditor && (it.Reviewer == nil || it.Reviewer.MembershipID != membership) {
			return domain.ErrForbidden
		}
		if decision == "revoke" && strings.TrimSpace(comment) == "" {
			comment = "Access no longer required"
		}
		if _, err := tx.Exec(ctx, `UPDATE access_review_items SET decision = $3, comment = $4, decided_by = $5, decided_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, itemID, decision, strings.TrimSpace(comment), actor.ID); err != nil {
			return err
		}
		if decision == "revoke" {
			due := time.Now().Add(72 * time.Hour)
			if _, err := s.openWorkItem(ctx, tx, orgID, workItem{Kind: "access_revocation", Title: fmt.Sprintf("Revoke %s's access in %s", it.AccountLabel, systemNames[it.IntegrationType]),
				Description: fmt.Sprintf("Revoked in access review \"%s\": %s. Trustline verifies removal on the next sync.", it.CampaignName, comment), Priority: "high",
				SourceKey: "revoke:" + it.ID.String(), PersonID: it.PersonID, ControlID: controlIDByCode(ctx, tx, orgID, "IAM-04"), Due: &due,
				Meta: map[string]any{"integration_type": it.IntegrationType, "account_id": it.AccountID, "email": it.Email, "campaign_id": it.CampaignID}}, false); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'dismissed', resolution = 'Decision changed to approve.', updated_at = now() WHERE org_id = $1 AND source_key = $2 AND status = 'open'`,
				orgID, "revoke:"+it.ID.String()); err != nil {
				return err
			}
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "access_review." + decision, TargetType: "access_review_item", TargetID: itemID.String(),
			TargetLabel: it.AccountLabel + " @ " + systemNames[it.IntegrationType], Metadata: map[string]any{"campaign": it.CampaignName, "comment": comment}})
	})
	if err != nil {
		return nil, err
	}
	return scanItem(s.Pool.QueryRow(ctx, itemSelect+` WHERE i.org_id = $1 AND i.id = $2`, orgID, itemID))
}

// CompleteCampaign closes a fully decided campaign and files the CSV export as
// evidence on the access review control.
func (s *Service) CompleteCampaign(ctx context.Context, orgID, id domain.ID, actor domain.Actor) (*CampaignDetail, error) {
	d, err := s.GetCampaign(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if d.Status != "open" {
		return nil, domain.Invalid("campaign", "this campaign is already closed")
	}
	if d.Decided < d.Total {
		return nil, domain.Invalid("campaign", fmt.Sprintf("%d account(s) still need a decision", d.Total-d.Decided))
	}
	csvData, err := campaignCSV(d)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(csvData)
	var evID *domain.ID
	controlID := controlIDByCode(ctx, s.Pool, orgID, "IAM-04")
	var key string
	if controlID != nil {
		eid := domain.NewID()
		evID = &eid
		key = fmt.Sprintf("orgs/%s/evidence/%s/access-review-%s.csv", orgID, eid, d.CreatedAt.Format("2006-01-02"))
		if err := s.Blob.Put(ctx, key, csvData, "text/csv"); err != nil {
			return nil, err
		}
	}
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if evID != nil {
			o, err := org.New(s.Pool).Get(ctx, tx, orgID)
			if err != nil {
				return err
			}
			exp := time.Now().Add(time.Duration(o.Settings.AccessReviewDays) * 24 * time.Hour)
			if _, err := tx.Exec(ctx, `INSERT INTO evidence (id, org_id, control_id, title, description, blob_key, filename, content_type, size_bytes, sha256, expires_at, uploaded_by,
				ai_status, ai_reasoning, override_verdict, override_reason, override_by, override_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'text/csv',$8,$9,$10,$11,'skipped','Generated by Trustline from a completed access review.','pass','System-generated record of a completed access review',$11, now())`,
				*evID, orgID, *controlID, "Access review: "+d.Name, fmt.Sprintf("%d accounts reviewed across %s; %d revoked.", d.Total, strings.Join(d.Systems, ", "), d.Revoked),
				key, "access-review-"+d.CreatedAt.Format("2006-01-02")+".csv", len(csvData), hex.EncodeToString(sum[:]), exp, actor.ID); err != nil {
				return err
			}
			ts, err := s.recompute(ctx, tx, orgID, []domain.ID{*controlID})
			if err != nil {
				return err
			}
			if err := s.handleTransitions(ctx, tx, orgID, nil, ts); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE access_review_campaigns SET status = 'completed', completed_at = now(), evidence_id = $3 WHERE org_id = $1 AND id = $2`, orgID, id, evID); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "access_review.completed", TargetType: "access_review", TargetID: id.String(), TargetLabel: d.Name,
			Metadata: map[string]any{"accounts": d.Total, "revoked": d.Revoked, "evidence_sha256": hex.EncodeToString(sum[:])}}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetCampaign(ctx, orgID, id)
}

func campaignCSV(d *CampaignDetail) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"campaign", "system", "account", "email", "hr_status", "entitlements", "admin", "reviewer", "decision", "comment", "decided_by", "decided_at"})
	for _, it := range d.Items {
		reviewer := ""
		if it.Reviewer != nil {
			reviewer = it.Reviewer.Email
		}
		decided := ""
		if it.DecidedAt != nil {
			decided = it.DecidedAt.UTC().Format(time.RFC3339)
		}
		if err := w.Write([]string{d.Name, systemNames[it.IntegrationType], it.AccountLabel, it.Email, it.PersonStatus, strings.Join(it.Entitlements, "; "),
			fmt.Sprint(it.IsAdmin), reviewer, it.Decision, it.Comment, it.DecidedBy, decided}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// ExportCampaign returns the CSV for download.
func (s *Service) ExportCampaign(ctx context.Context, orgID, id domain.ID) ([]byte, string, error) {
	d, err := s.GetCampaign(ctx, orgID, id)
	if err != nil {
		return nil, "", err
	}
	data, err := campaignCSV(d)
	return data, "access-review-" + d.CreatedAt.Format("2006-01-02") + ".csv", err
}

// ScheduleAccessReviews opens a campaign when the configured cadence has
// elapsed since the last one (daily job).
func (s *Service) ScheduleAccessReviews(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT o.id, (o.settings->>'access_review_days')::int,
		  (SELECT max(created_at) FROM access_review_campaigns c WHERE c.org_id = o.id),
		  ARRAY(SELECT type FROM integrations i WHERE i.org_id = o.id AND i.type = ANY($1) ORDER BY type)
		FROM orgs o`, reviewableSystems)
	if err != nil {
		return err
	}
	type due struct {
		org     domain.ID
		systems []string
	}
	var list []due
	for rows.Next() {
		var id domain.ID
		var days *int
		var last *time.Time
		var systems []string
		if err := rows.Scan(&id, &days, &last, &systems); err != nil {
			rows.Close()
			return err
		}
		cadence := 90
		if days != nil && *days > 0 {
			cadence = *days
		}
		if len(systems) == 0 || (last != nil && time.Since(*last) < time.Duration(cadence)*24*time.Hour) {
			continue
		}
		list = append(list, due{id, systems})
	}
	rows.Close()
	for _, d := range list {
		if _, err := s.CreateCampaign(ctx, d.org, CampaignInput{Name: "Scheduled access review " + time.Now().Format("2006-01-02"), Systems: d.systems, DueDays: 14}, domain.SystemActor("scheduler")); err != nil {
			s.Log.Warn("scheduled access review failed", "org", d.org, "err", err)
		}
	}
	return nil
}

// TrainingRenewals opens a renewal task 30 days before training expires (daily job).
func (s *Service) TrainingRenewals(ctx context.Context) error {
	c := s.course()
	if c == nil {
		return nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT m.org_id, m.id, max(t.completed_at) FROM org_memberships m
		JOIN training_completions t ON t.org_id = m.org_id AND t.user_id = m.user_id AND t.passed AND t.course_id = $1
		WHERE m.role IN ('admin','editor','employee') GROUP BY m.org_id, m.id
		HAVING max(t.completed_at) < now() - make_interval(days => $2)`, c.ID, c.ValidDays-30)
	if err != nil {
		return err
	}
	type r struct {
		org, member domain.ID
		last        time.Time
	}
	var list []r
	for rows.Next() {
		var x r
		if err := rows.Scan(&x.org, &x.member, &x.last); err != nil {
			rows.Close()
			return err
		}
		list = append(list, x)
	}
	rows.Close()
	for _, x := range list {
		due := x.last.Add(time.Duration(c.ValidDays) * 24 * time.Hour)
		m := x.member
		if err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
			_, err := s.openWorkItem(ctx, tx, x.org, workItem{Kind: "training_renewal", Title: "Renew security awareness training", Description: "Your annual training expires soon.",
				SourceKey: "training:" + x.member.String() + ":" + due.Format("2006-01-02"), Assignee: &m, Due: &due, ControlID: controlIDByCode(ctx, tx, x.org, "HR-02")}, true)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
