package compliance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/microcosm-cc/bluemonday"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/org"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var (
	md        = goldmark.New(goldmark.WithExtensions(extension.GFM))
	sanitizer = bluemonday.UGCPolicy()
)

// MarkdownToHTML renders and sanitizes policy Markdown.
func MarkdownToHTML(src string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return sanitizer.Sanitize(buf.String()), nil
}

var (
	tagRe   = regexp.MustCompile(`<[^>]+>`)
	blockRe = regexp.MustCompile(`(?i)</(p|h[1-6]|li|tr|div|table|ul|ol)>|<br\s*/?>`)
	spaceRe = regexp.MustCompile(`[ \t]+`)
)

// HTMLToText produces readable plain text (consistency checks, search).
func HTMLToText(h string) string {
	t := blockRe.ReplaceAllString(h, "\n")
	t = tagRe.ReplaceAllString(t, " ")
	t = html.UnescapeString(t)
	lines := strings.Split(t, "\n")
	out := lines[:0]
	for _, l := range lines {
		l = strings.TrimSpace(spaceRe.ReplaceAllString(l, " "))
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

type PolicyVersion struct {
	ID            domain.ID             `json:"id"`
	Version       int                   `json:"version"`
	Status        string                `json:"status"`
	ContentHTML   string                `json:"content_html,omitempty"`
	ChangeSummary string                `json:"change_summary"`
	CreatedBy     string                `json:"created_by"`
	CreatedAt     time.Time             `json:"created_at"`
	SubmittedAt   *time.Time            `json:"submitted_at,omitempty"`
	Approver      *Owner                `json:"approver,omitempty"`
	DecidedBy     string                `json:"decided_by"`
	DecidedAt     *time.Time            `json:"decided_at,omitempty"`
	ReviewComment string                `json:"review_comment"`
	PublishedAt   *time.Time            `json:"published_at,omitempty"`
	PublishedBy   string                `json:"published_by"`
	AIGenerated   bool                  `json:"ai_generated"`
	Consistency   *ai.ConsistencyReport `json:"consistency,omitempty"`
	ConsistencyAt *time.Time            `json:"consistency_checked_at,omitempty"`
}

type PolicyRow struct {
	ID               domain.ID  `json:"id"`
	TemplateID       *string    `json:"template_id,omitempty"`
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	Category         string     `json:"category"`
	Summary          string     `json:"summary"`
	Controls         []string   `json:"controls"`
	Owner            *Owner     `json:"owner,omitempty"`
	PublishedVersion *int       `json:"published_version,omitempty"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	OpenStatus       string     `json:"open_status"` // status of the draft/in-review/approved/rejected version, or ""
	NextReviewAt     *time.Time `json:"next_review_at,omitempty"`
	ReviewOverdue    bool       `json:"review_overdue"`
	Audience         int        `json:"audience"`
	Accepted         int        `json:"accepted"`
	AcceptanceRate   float64    `json:"acceptance_rate"`
	ReviewCycleDays  int        `json:"review_cycle_days"`
}

const policySelect = `SELECT p.id, p.template_id, p.slug, p.title, p.category, p.summary, p.control_codes, m.id, u.name, u.email,
  cv.version, cv.published_at,
  COALESCE((SELECT ov.status FROM policy_versions ov WHERE ov.policy_id = p.id AND ov.status IN ('draft','in_review','approved','rejected')), ''),
  p.next_review_at, p.review_cycle_days,
  (SELECT count(*) FROM org_memberships am WHERE am.org_id = p.org_id AND am.role IN ('admin','editor','employee')),
  COALESCE((SELECT count(*) FROM policy_acceptances a JOIN org_memberships am ON am.user_id = a.user_id AND am.org_id = p.org_id AND am.role IN ('admin','editor','employee')
            WHERE a.policy_version_id = cv.id), 0)
FROM policies p
LEFT JOIN policy_versions cv ON cv.id = p.current_version_id
LEFT JOIN org_memberships m ON m.id = p.owner_membership_id AND m.org_id = p.org_id
LEFT JOIN users u ON u.id = m.user_id`

func scanPolicy(row pgx.Row) (*PolicyRow, error) {
	var p PolicyRow
	var mid *domain.ID
	var mname, memail *string
	if err := row.Scan(&p.ID, &p.TemplateID, &p.Slug, &p.Title, &p.Category, &p.Summary, &p.Controls, &mid, &mname, &memail,
		&p.PublishedVersion, &p.PublishedAt, &p.OpenStatus, &p.NextReviewAt, &p.ReviewCycleDays, &p.Audience, &p.Accepted); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if mid != nil {
		p.Owner = &Owner{MembershipID: *mid, Name: deref(mname), Email: deref(memail)}
	}
	if p.Controls == nil {
		p.Controls = []string{}
	}
	if p.PublishedVersion != nil && p.Audience > 0 {
		p.AcceptanceRate = float64(int(float64(p.Accepted)*1000/float64(p.Audience))) / 10
	}
	p.ReviewOverdue = p.PublishedVersion != nil && p.NextReviewAt != nil && p.NextReviewAt.Before(time.Now())
	return &p, nil
}

func (s *Service) ListPolicies(ctx context.Context, orgID domain.ID, publishedOnly bool) ([]PolicyRow, error) {
	where := " WHERE p.org_id = $1"
	if publishedOnly {
		where += " AND cv.status = 'published'"
	}
	rows, err := s.Pool.Query(ctx, policySelect+where+" ORDER BY p.title", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PolicyRow{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

type LibraryItem struct {
	catalog.PolicyTemplate
	PolicyID *domain.ID `json:"policy_id,omitempty"`
}

func (s *Service) PolicyLibrary(ctx context.Context, orgID domain.ID) ([]LibraryItem, error) {
	have := map[string]domain.ID{}
	rows, err := s.Pool.Query(ctx, `SELECT template_id, id FROM policies WHERE org_id = $1 AND template_id IS NOT NULL`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t string
		var id domain.ID
		if err := rows.Scan(&t, &id); err != nil {
			rows.Close()
			return nil, err
		}
		have[t] = id
	}
	rows.Close()
	out := []LibraryItem{}
	for _, t := range s.Catalog.Policies {
		it := LibraryItem{PolicyTemplate: t}
		if id, ok := have[t.ID]; ok {
			id := id
			it.PolicyID = &id
		}
		out = append(out, it)
	}
	return out, nil
}

// placeholders returns template substitutions for an org.
func (s *Service) placeholders(ctx context.Context, q db.Querier, orgID domain.ID) (map[string]string, *domain.Org, error) {
	o, err := org.New(s.Pool).Get(ctx, q, orgID)
	if err != nil {
		return nil, nil, err
	}
	officer := "the Security Officer"
	var name string
	if err := q.QueryRow(ctx, `SELECT u.name FROM controls c JOIN org_memberships m ON m.id = c.owner_membership_id JOIN users u ON u.id = m.user_id WHERE c.org_id = $1 AND c.code = 'GOV-02'`, orgID).Scan(&name); err == nil {
		officer = name
	} else if err := q.QueryRow(ctx, `SELECT u.name FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND m.role = 'admin' ORDER BY m.created_at LIMIT 1`, orgID).Scan(&name); err == nil {
		officer = name
	}
	return map[string]string{
		"org_name":                    o.Name,
		"effective_date":              time.Now().Format("2006-01-02"),
		"security_officer":            officer,
		"access_review_days":          strconv.Itoa(o.Settings.AccessReviewDays),
		"password_min_length":         strconv.Itoa(o.Settings.PasswordMinLength),
		"policy_acceptance_threshold": strconv.Itoa(o.Settings.PolicyAcceptanceThreshold),
	}, o, nil
}

func fill(src string, vals map[string]string) string {
	for k, v := range vals {
		src = strings.ReplaceAll(src, "{{"+k+"}}", v)
	}
	return src
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// CreatePolicy instantiates a template (or a blank policy) as v1 draft.
func (s *Service) CreatePolicy(ctx context.Context, orgID domain.ID, templateID, title string, actor domain.Actor) (*PolicyDetail, error) {
	var id domain.ID
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		vals, _, err := s.placeholders(ctx, tx, orgID)
		if err != nil {
			return err
		}
		var tpl *catalog.PolicyTemplate
		content := "<h1>" + html.EscapeString(title) + "</h1><h2>Purpose</h2><p></p><h2>Scope</h2><p></p><h2>Policy statements</h2><p></p><h2>Review</h2><p>This policy is reviewed at least annually.</p>"
		category, summary, cycle := "Governance", "", 365
		var controls []string
		if templateID != "" {
			tpl = s.Catalog.PolicyTemplate(templateID)
			if tpl == nil {
				return domain.Invalid("template_id", "unknown template")
			}
			title, category, summary, cycle, controls = tpl.Title, tpl.Category, tpl.Summary, tpl.ReviewCycleDays, tpl.Controls
			if content, err = MarkdownToHTML(fill(tpl.Markdown, vals)); err != nil {
				return err
			}
		}
		if strings.TrimSpace(title) == "" {
			return domain.Invalid("title", "title is required")
		}
		if controls == nil {
			controls = []string{}
		}
		slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
		if tpl != nil {
			slug = tpl.ID
		}
		owner, _ := membershipByEmail(ctx, tx, orgID, deref(&actor.Label))
		id = domain.NewID()
		var tplID *string
		if tpl != nil {
			tplID = &tpl.ID
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policies (id, org_id, template_id, slug, title, category, summary, control_codes, owner_membership_id, review_cycle_days)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, orgID, tplID, slug, title, category, summary, controls, owner, cycle); err != nil {
			if db.IsUniqueViolation(err) {
				return domain.Invalid("template_id", title+" already exists in your policies")
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_versions (id, org_id, policy_id, version, status, content_html, change_summary, created_by) VALUES ($1,$2,$3,1,'draft',$4,$5,$6)`,
			domain.NewID(), orgID, id, content, "Initial version from the template library", actor.ID); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.created", TargetType: "policy", TargetID: id.String(), TargetLabel: title, Metadata: map[string]any{"template": templateID}})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPolicy(ctx, orgID, id, true)
}

type AcceptanceEntry struct {
	MembershipID domain.ID  `json:"membership_id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	AcceptedAt   *time.Time `json:"accepted_at,omitempty"`
}

type PolicyDetail struct {
	PolicyRow
	Versions    []PolicyVersion   `json:"versions"`
	Acceptances []AcceptanceEntry `json:"acceptances"`
}

const versionSelect = `SELECT v.id, v.version, v.status, v.content_html, v.change_summary, COALESCE(cu.email,''), v.created_at, v.submitted_at,
  am.id, au.name, au.email, COALESCE(du.email,''), v.decided_at, v.review_comment, v.published_at, COALESCE(pu.email,''), v.ai_generated, v.consistency, v.consistency_checked_at
FROM policy_versions v
LEFT JOIN users cu ON cu.id = v.created_by
LEFT JOIN org_memberships am ON am.id = v.approver_membership_id
LEFT JOIN users au ON au.id = am.user_id
LEFT JOIN users du ON du.id = v.decided_by
LEFT JOIN users pu ON pu.id = v.published_by`

func scanVersion(row pgx.Row) (*PolicyVersion, error) {
	var v PolicyVersion
	var aid *domain.ID
	var aname, aemail *string
	var cons []byte
	if err := row.Scan(&v.ID, &v.Version, &v.Status, &v.ContentHTML, &v.ChangeSummary, &v.CreatedBy, &v.CreatedAt, &v.SubmittedAt, &aid, &aname, &aemail,
		&v.DecidedBy, &v.DecidedAt, &v.ReviewComment, &v.PublishedAt, &v.PublishedBy, &v.AIGenerated, &cons, &v.ConsistencyAt); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if aid != nil {
		v.Approver = &Owner{MembershipID: *aid, Name: deref(aname), Email: deref(aemail)}
	}
	if len(cons) > 0 {
		var r ai.ConsistencyReport
		if json.Unmarshal(cons, &r) == nil {
			v.Consistency = &r
		}
	}
	return &v, nil
}

// GetPolicy returns the policy; includeDrafts=false shows only published history (employees).
func (s *Service) GetPolicy(ctx context.Context, orgID, id domain.ID, includeDrafts bool) (*PolicyDetail, error) {
	p, err := scanPolicy(s.Pool.QueryRow(ctx, policySelect+" WHERE p.org_id = $1 AND p.id = $2", orgID, id))
	if err != nil {
		return nil, err
	}
	if !includeDrafts && p.PublishedVersion == nil {
		return nil, domain.ErrNotFound
	}
	d := &PolicyDetail{PolicyRow: *p, Versions: []PolicyVersion{}, Acceptances: []AcceptanceEntry{}}
	filter := ""
	if !includeDrafts {
		filter = " AND v.status IN ('published','superseded')"
	}
	rows, err := s.Pool.Query(ctx, versionSelect+" WHERE v.org_id = $1 AND v.policy_id = $2"+filter+" ORDER BY v.version DESC", orgID, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		d.Versions = append(d.Versions, *v)
	}
	rows.Close()
	if !includeDrafts {
		return d, nil
	}
	arows, err := s.Pool.Query(ctx, `SELECT m.id, u.name, u.email, m.role, a.accepted_at FROM org_memberships m JOIN users u ON u.id = m.user_id
		LEFT JOIN policies p ON p.id = $2 AND p.org_id = m.org_id
		LEFT JOIN policy_acceptances a ON a.user_id = m.user_id AND a.policy_version_id = p.current_version_id
		WHERE m.org_id = $1 AND m.role IN ('admin','editor','employee') ORDER BY a.accepted_at IS NOT NULL, u.name`, orgID, id)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var a AcceptanceEntry
		if err := arows.Scan(&a.MembershipID, &a.Name, &a.Email, &a.Role, &a.AcceptedAt); err != nil {
			return nil, err
		}
		d.Acceptances = append(d.Acceptances, a)
	}
	return d, arows.Err()
}

func (s *Service) lockVersion(ctx context.Context, tx pgx.Tx, orgID, versionID domain.ID) (*PolicyVersion, domain.ID, string, error) {
	var policyID domain.ID
	var title string
	if err := tx.QueryRow(ctx, `SELECT v.policy_id, p.title FROM policy_versions v JOIN policies p ON p.id = v.policy_id WHERE v.org_id = $1 AND v.id = $2 FOR UPDATE OF v`, orgID, versionID).Scan(&policyID, &title); err != nil {
		if db.IsNoRows(err) {
			return nil, policyID, "", domain.ErrNotFound
		}
		return nil, policyID, "", err
	}
	v, err := scanVersion(tx.QueryRow(ctx, versionSelect+" WHERE v.org_id = $1 AND v.id = $2", orgID, versionID))
	return v, policyID, title, err
}

// UpdateDraft saves editor content on a draft (or a rejected version, which returns to draft).
func (s *Service) UpdateDraft(ctx context.Context, orgID, versionID domain.ID, content, changeSummary string, actor domain.Actor) error {
	if len(content) > 2<<20 {
		return domain.Invalid("content_html", "policy is too large")
	}
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, policyID, title, err := s.lockVersion(ctx, tx, orgID, versionID)
		if err != nil {
			return err
		}
		if v.Status != "draft" && v.Status != "rejected" {
			return domain.Invalid("status", "only drafts can be edited; create a new version to change a published policy")
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET content_html = $3, change_summary = $4, status = 'draft', updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, versionID, sanitizer.Sanitize(content), strings.TrimSpace(changeSummary)); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.draft_saved", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title, Metadata: map[string]any{"version": v.Version}})
	})
}

// NewVersion starts a draft from the published version.
func (s *Service) NewVersion(ctx context.Context, orgID, policyID domain.ID, actor domain.Actor) (*PolicyVersion, error) {
	var id domain.ID
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title, content string
		var cur int
		if err := tx.QueryRow(ctx, `SELECT p.title, COALESCE(v.content_html, ''), COALESCE((SELECT max(version) FROM policy_versions WHERE policy_id = p.id), 0)
			FROM policies p LEFT JOIN policy_versions v ON v.id = p.current_version_id WHERE p.org_id = $1 AND p.id = $2 FOR UPDATE OF p`, orgID, policyID).Scan(&title, &content, &cur); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		id = domain.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO policy_versions (id, org_id, policy_id, version, status, content_html, created_by) VALUES ($1,$2,$3,$4,'draft',$5,$6)`,
			id, orgID, policyID, cur+1, content, actor.ID); err != nil {
			if db.IsUniqueViolation(err) {
				return domain.Invalid("version", "this policy already has a version in progress")
			}
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.version_started", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title, Metadata: map[string]any{"version": cur + 1}})
	})
	if err != nil {
		return nil, err
	}
	return scanVersion(s.Pool.QueryRow(ctx, versionSelect+" WHERE v.org_id = $1 AND v.id = $2", orgID, id))
}

// SubmitForApproval sends a draft to an approver (an admin or editor).
func (s *Service) SubmitForApproval(ctx context.Context, orgID, versionID, approver domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, policyID, title, err := s.lockVersion(ctx, tx, orgID, versionID)
		if err != nil {
			return err
		}
		if v.Status != "draft" {
			return domain.Invalid("status", "only drafts can be submitted")
		}
		if strings.TrimSpace(HTMLToText(v.ContentHTML)) == "" {
			return domain.Invalid("content_html", "the policy is empty")
		}
		var role string
		var approverUser domain.ID
		if err := tx.QueryRow(ctx, `SELECT role, user_id FROM org_memberships WHERE org_id = $1 AND id = $2`, orgID, approver).Scan(&role, &approverUser); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("approver_membership_id", "choose an approver from this organization")
			}
			return err
		}
		if role != "admin" && role != "editor" {
			return domain.Invalid("approver_membership_id", "approvers must be admins or editors")
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status = 'in_review', submitted_at = now(), approver_membership_id = $3, review_comment = '', updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, versionID, approver); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.submitted", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title, Metadata: map[string]any{"version": v.Version, "approver": approver}}); err != nil {
			return err
		}
		return s.notifyUsers(ctx, tx, orgID, `SELECT id, email FROM users WHERE id = $1`, []any{approverUser}, "policy_review",
			fmt.Sprintf("Approval requested: %s v%d", title, v.Version), actor.Label+" asked you to approve this policy version.", "/app/policies/"+policyID.String())
	})
}

// DecideVersion approves or rejects an in-review version. The approver or any admin may decide.
func (s *Service) DecideVersion(ctx context.Context, orgID, versionID domain.ID, approve bool, comment string, actor domain.Actor, actorRole domain.Role, actorMembership domain.ID) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, policyID, title, err := s.lockVersion(ctx, tx, orgID, versionID)
		if err != nil {
			return err
		}
		if v.Status != "in_review" {
			return domain.Invalid("status", "this version is not awaiting approval")
		}
		if actorRole != domain.RoleAdmin && (v.Approver == nil || v.Approver.MembershipID != actorMembership) {
			return domain.ErrForbidden
		}
		status, action := "approved", "policy.approved"
		if !approve {
			status, action = "rejected", "policy.rejected"
			if strings.TrimSpace(comment) == "" {
				return domain.Invalid("comment", "explain what needs to change")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status = $3, decided_by = $4, decided_at = now(), review_comment = $5, updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, versionID, status, actor.ID, strings.TrimSpace(comment)); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: action, TargetType: "policy", TargetID: policyID.String(), TargetLabel: title, Metadata: map[string]any{"version": v.Version, "comment": comment}}); err != nil {
			return err
		}
		return s.notifyUsers(ctx, tx, orgID, `SELECT u.id, u.email FROM users u WHERE lower(u.email) = lower($1)`, []any{v.CreatedBy}, "policy_review",
			fmt.Sprintf("%s v%d was %s", title, v.Version, status), comment, "/app/policies/"+policyID.String())
	})
}

// Publish makes an approved version current, asks everyone to acknowledge it
// and schedules the next review.
func (s *Service) Publish(ctx context.Context, orgID, versionID domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, policyID, title, err := s.lockVersion(ctx, tx, orgID, versionID)
		if err != nil {
			return err
		}
		if v.Status != "approved" {
			return domain.Invalid("status", "only approved versions can be published")
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status = 'superseded', updated_at = now() WHERE org_id = $1 AND policy_id = $2 AND status = 'published'`, orgID, policyID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET status = 'published', published_at = now(), published_by = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, versionID, actor.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE policies SET current_version_id = $3, last_reviewed_at = now(), next_review_at = now() + make_interval(days => review_cycle_days), updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, policyID, versionID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'done', resolution = 'New version published.', completed_at = now(), updated_at = now()
			WHERE org_id = $1 AND kind = 'policy_review' AND status = 'open' AND meta->>'policy_id' = $2`, orgID, policyID.String()); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.published", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title, Metadata: map[string]any{"version": v.Version}}); err != nil {
			return err
		}
		if err := s.notifyUsers(ctx, tx, orgID, `SELECT u.id, u.email FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND m.role IN ('admin','editor','employee')`, []any{orgID},
			"policy_published", fmt.Sprintf("Please acknowledge: %s v%d", title, v.Version), "A new version of this policy was published. Read it and acknowledge it from My work.", "/app/my-work"); err != nil {
			return err
		}
		if err := s.Jobs.EnqueueTx(ctx, tx, jobs.PolicyConsistencyArgs{OrgID: orgID.String(), VersionID: versionID.String()}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

// MarkReviewed records an annual review without changes.
func (s *Service) MarkReviewed(ctx context.Context, orgID, policyID domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		if err := tx.QueryRow(ctx, `UPDATE policies SET last_reviewed_at = now(), next_review_at = now() + make_interval(days => review_cycle_days), updated_at = now()
			WHERE org_id = $1 AND id = $2 AND current_version_id IS NOT NULL RETURNING title`, orgID, policyID).Scan(&title); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("policy", "only published policies can be marked reviewed")
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'done', resolution = 'Reviewed without changes.', completed_at = now(), updated_at = now()
			WHERE org_id = $1 AND kind = 'policy_review' AND status = 'open' AND meta->>'policy_id' = $2`, orgID, policyID.String()); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.reviewed", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

func (s *Service) SetPolicyOwner(ctx context.Context, orgID, policyID domain.ID, owner *domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if owner != nil {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_memberships WHERE org_id = $1 AND id = $2)`, orgID, *owner).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return domain.Invalid("owner_membership_id", "not a member of this organization")
			}
		}
		var title string
		if err := tx.QueryRow(ctx, `UPDATE policies SET owner_membership_id = $3, updated_at = now() WHERE org_id = $1 AND id = $2 RETURNING title`, orgID, policyID, owner).Scan(&title); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.owner_changed", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title})
	})
}

// Acknowledge records that a user accepted the current published version.
func (s *Service) Acknowledge(ctx context.Context, orgID, versionID, userID domain.ID, ip string, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		var version int
		if err := tx.QueryRow(ctx, `SELECT p.title, v.version FROM policy_versions v JOIN policies p ON p.id = v.policy_id
			WHERE v.org_id = $1 AND v.id = $2 AND p.current_version_id = v.id AND v.status = 'published'`, orgID, versionID).Scan(&title, &version); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("version", "this is not the current published version")
			}
			return err
		}
		var role string
		var mid domain.ID
		if err := tx.QueryRow(ctx, `SELECT role, id FROM org_memberships WHERE org_id = $1 AND user_id = $2`, orgID, userID).Scan(&role, &mid); err != nil {
			return err
		}
		if role == string(domain.RoleAuditor) {
			return domain.ErrForbidden
		}
		tag, err := tx.Exec(ctx, `INSERT INTO policy_acceptances (org_id, policy_version_id, user_id, ip) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, orgID, versionID, userID, ip)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if _, err := s.closeWorkItems(ctx, tx, orgID, []string{"accept:" + versionID.String() + ":" + mid.String()}, "done", "Acknowledged."); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.acknowledged", TargetType: "policy_version", TargetID: versionID.String(), TargetLabel: fmt.Sprintf("%s v%d", title, version)}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

// AIDraftVersion replaces a draft's content with an AI draft tailored to the org.
func (s *Service) AIDraftVersion(ctx context.Context, svc *ai.Service, orgID, versionID domain.ID, instructions string, actor domain.Actor) (*PolicyVersion, error) {
	v, err := scanVersion(s.Pool.QueryRow(ctx, versionSelect+" WHERE v.org_id = $1 AND v.id = $2", orgID, versionID))
	if err != nil {
		return nil, err
	}
	if v.Status != "draft" && v.Status != "rejected" {
		return nil, domain.Invalid("status", "AI drafting works on drafts only")
	}
	var policyID domain.ID
	var title string
	var tplID *string
	if err := s.Pool.QueryRow(ctx, `SELECT p.id, p.title, p.template_id FROM policies p JOIN policy_versions v ON v.policy_id = p.id WHERE v.id = $1 AND p.org_id = $2`, versionID, orgID).Scan(&policyID, &title, &tplID); err != nil {
		return nil, err
	}
	vals, o, err := s.placeholders(ctx, s.Pool, orgID)
	if err != nil {
		return nil, err
	}
	in := ai.PolicyDraftInput{OrgName: o.Name, SecurityOfficer: vals["security_officer"], Title: title, Instructions: strings.TrimSpace(instructions),
		AccessReviewDays: o.Settings.AccessReviewDays, PasswordMinLength: o.Settings.PasswordMinLength, AcceptanceThreshold: o.Settings.PolicyAcceptanceThreshold,
		EffectiveDate: vals["effective_date"]}
	if tplID != nil {
		if t := s.Catalog.PolicyTemplate(*tplID); t != nil {
			in.TemplateMarkdown = fill(t.Markdown, vals)
		}
	}
	if in.TemplateMarkdown == "" {
		in.TemplateMarkdown = HTMLToText(v.ContentHTML)
	}
	rows, err := s.Pool.Query(ctx, `SELECT type FROM integrations WHERE org_id = $1 AND type <> 'trustline' ORDER BY type`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t string
		_ = rows.Scan(&t)
		if def := s.Catalog.ConnectorDef(t); def != nil {
			in.Systems = append(in.Systems, def.Name)
		}
	}
	rows.Close()
	frows, err := s.Pool.Query(ctx, `SELECT f.name FROM org_frameworks o JOIN catalog_frameworks f ON f.id = o.framework_id WHERE o.org_id = $1 ORDER BY f.position`, orgID)
	if err != nil {
		return nil, err
	}
	for frows.Next() {
		var n string
		_ = frows.Scan(&n)
		in.Frameworks = append(in.Frameworks, n)
	}
	frows.Close()
	_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(array_agg(DISTINCT department ORDER BY department) FILTER (WHERE department <> ''), '{}'), count(*) FROM people WHERE org_id = $1 AND status <> 'terminated'`, orgID).Scan(&in.Departments, &in.Headcount)
	draft, model, err := svc.DraftPolicy(ctx, orgID, in)
	if err != nil {
		if err == ai.ErrDisabled {
			return nil, domain.Invalid("ai", "AI features are disabled for this organization")
		}
		return nil, err
	}
	content, err := MarkdownToHTML(draft.Markdown)
	if err != nil {
		return nil, err
	}
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE policy_versions SET content_html = $3, change_summary = $4, ai_generated = true, status = 'draft', updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, versionID, content, draft.ChangeSummary); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.ai_drafted", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title, Metadata: map[string]any{"version": v.Version, "model": model}})
	})
	if err != nil {
		return nil, err
	}
	return scanVersion(s.Pool.QueryRow(ctx, versionSelect+" WHERE v.org_id = $1 AND v.id = $2", orgID, versionID))
}

// CheckConsistency compares a version's text with configured settings and stores the result.
func (s *Service) CheckConsistency(ctx context.Context, svc *ai.Service, orgID, versionID domain.ID) (*ai.ConsistencyReport, error) {
	v, err := scanVersion(s.Pool.QueryRow(ctx, versionSelect+" WHERE v.org_id = $1 AND v.id = $2", orgID, versionID))
	if err != nil {
		return nil, err
	}
	var title string
	if err := s.Pool.QueryRow(ctx, `SELECT p.title FROM policies p JOIN policy_versions v ON v.policy_id = p.id WHERE v.id = $1`, versionID).Scan(&title); err != nil {
		return nil, err
	}
	o, err := org.New(s.Pool).Get(ctx, s.Pool, orgID)
	if err != nil {
		return nil, err
	}
	facts := []ai.Fact{
		{Key: "access_review_days", Label: "User access review cadence in days", Value: strconv.Itoa(o.Settings.AccessReviewDays)},
		{Key: "password_min_length", Label: "Minimum password length in characters", Value: strconv.Itoa(o.Settings.PasswordMinLength)},
		{Key: "policy_acceptance_threshold", Label: "Percentage of staff required to acknowledge policies", Value: strconv.Itoa(o.Settings.PolicyAcceptanceThreshold)},
	}
	rep, _, err := svc.CheckConsistency(ctx, orgID, ai.ConsistencyInput{Title: title, Text: HTMLToText(v.ContentHTML), Facts: facts})
	if err != nil {
		if err == ai.ErrDisabled {
			return nil, domain.Invalid("ai", "AI features are disabled for this organization")
		}
		return nil, err
	}
	raw, _ := json.Marshal(rep)
	if _, err := s.Pool.Exec(ctx, `UPDATE policy_versions SET consistency = $3, consistency_checked_at = now() WHERE org_id = $1 AND id = $2`, orgID, versionID, raw); err != nil {
		return nil, err
	}
	if len(rep.Issues) > 0 && v.Status == "published" {
		_ = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
			return s.notifyAdmins(ctx, tx, orgID, "policy_inconsistency", fmt.Sprintf("%s contradicts your settings", title),
				rep.Issues[0].Explanation, "/app/policies")
		})
	}
	return rep, nil
}

// PolicyReviewReminders opens review tasks 30 days before a policy's review date (daily job).
func (s *Service) PolicyReviewReminders(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT org_id, id, title, owner_membership_id, next_review_at FROM policies WHERE current_version_id IS NOT NULL AND next_review_at < now() + interval '30 days'`)
	if err != nil {
		return err
	}
	type pol struct {
		org, id domain.ID
		title   string
		owner   *domain.ID
		due     time.Time
	}
	var list []pol
	for rows.Next() {
		var p pol
		if err := rows.Scan(&p.org, &p.id, &p.title, &p.owner, &p.due); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	for _, p := range list {
		if err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
			due := p.due
			created, err := s.openWorkItem(ctx, tx, p.org, workItem{Kind: "policy_review", Title: "Annual review: " + p.title, Priority: "medium",
				Description: "Review this policy against current practice. Publish a new version, or mark it reviewed if nothing changed.",
				SourceKey:   "review:" + p.id.String() + ":" + p.due.Format("2006-01-02"), Assignee: p.owner, Due: &due, ControlID: controlIDByCode(ctx, tx, p.org, "GOV-04"),
				Meta: map[string]any{"policy_id": p.id.String()}}, true)
			if err != nil || created == nil {
				return err
			}
			return s.notifyAdmins(ctx, tx, p.org, "policy_review_due", "Policy review due: "+p.title, "Due "+p.due.Format("2006-01-02")+".", "/app/policies/"+p.id.String())
		}); err != nil {
			return err
		}
	}
	return nil
}
