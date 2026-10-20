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
)

type Owner struct {
	MembershipID domain.ID `json:"membership_id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
}

type ControlRow struct {
	ID              domain.ID `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Domain          string    `json:"domain"`
	Description     string    `json:"description"`
	EvidenceMode    string    `json:"evidence_mode"`
	Status          string    `json:"status"`
	StatusReason    string    `json:"status_reason"`
	StatusChangedAt time.Time `json:"status_changed_at"`
	Owner           *Owner    `json:"owner,omitempty"`
	Frameworks      []string  `json:"frameworks"`
	TestsTotal      int       `json:"tests_total"`
	TestsFailing    int       `json:"tests_failing"`
	EvidenceValid   int       `json:"evidence_valid"`
}

type ControlFilter struct {
	FrameworkID string
	Status      string
	Query       string
	OwnerID     *domain.ID
}

const controlSelect = `
SELECT c.id, c.code, c.name, c.domain, c.description, c.evidence_mode, c.status, c.status_reason, c.status_changed_at,
       m.id, u.name, u.email,
       COALESCE((SELECT array_agg(DISTINCT r.framework_id ORDER BY r.framework_id)
                 FROM catalog_requirement_controls rc JOIN catalog_requirements r ON r.id = rc.requirement_id
                 JOIN org_frameworks ofw ON ofw.framework_id = r.framework_id AND ofw.org_id = c.org_id
                 WHERE rc.control_code = c.code), '{}'),
       (SELECT count(*) FROM tests t WHERE t.org_id = c.org_id AND t.control_id = c.id AND t.enabled AND t.last_status <> 'na'),
       (SELECT count(*) FROM tests t WHERE t.org_id = c.org_id AND t.control_id = c.id AND t.enabled AND t.last_status IN ('fail','error')),
       (SELECT count(*) FROM evidence e WHERE e.org_id = c.org_id AND e.control_id = c.id AND ` + validEvidence + `)
FROM controls c
LEFT JOIN org_memberships m ON m.id = c.owner_membership_id AND m.org_id = c.org_id
LEFT JOIN users u ON u.id = m.user_id`

// validEvidence is the SQL predicate for evidence that currently satisfies its control.
const validEvidence = `COALESCE(e.override_verdict, e.ai_verdict) = 'pass' AND (e.expires_at IS NULL OR e.expires_at > now())`

func scanControl(row pgx.Row) (*ControlRow, error) {
	var c ControlRow
	var ownerID *domain.ID
	var ownerName, ownerEmail *string
	if err := row.Scan(&c.ID, &c.Code, &c.Name, &c.Domain, &c.Description, &c.EvidenceMode, &c.Status, &c.StatusReason, &c.StatusChangedAt,
		&ownerID, &ownerName, &ownerEmail, &c.Frameworks, &c.TestsTotal, &c.TestsFailing, &c.EvidenceValid); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if ownerID != nil {
		c.Owner = &Owner{MembershipID: *ownerID, Name: deref(ownerName), Email: deref(ownerEmail)}
	}
	if c.Frameworks == nil {
		c.Frameworks = []string{}
	}
	return &c, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) ListControls(ctx context.Context, orgID domain.ID, f ControlFilter) ([]ControlRow, error) {
	args := []any{orgID}
	where := []string{"c.org_id = $1"}
	if f.FrameworkID != "" {
		args = append(args, f.FrameworkID)
		where = append(where, fmt.Sprintf(`c.code IN (SELECT rc.control_code FROM catalog_requirement_controls rc JOIN catalog_requirements r ON r.id = rc.requirement_id WHERE r.framework_id = $%d)`, len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("c.status = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf("(lower(c.code) LIKE $%d OR lower(c.name) LIKE $%d OR lower(c.domain) LIKE $%d)", len(args), len(args), len(args)))
	}
	if f.OwnerID != nil {
		args = append(args, *f.OwnerID)
		where = append(where, fmt.Sprintf("c.owner_membership_id = $%d", len(args)))
	}
	rows, err := s.Pool.Query(ctx, controlSelect+" WHERE "+strings.Join(where, " AND ")+
		" ORDER BY CASE c.status WHEN 'failing' THEN 0 WHEN 'needs_evidence' THEN 1 ELSE 2 END, c.code", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ControlRow{}
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

type MappedRequirement struct {
	FrameworkID   string `json:"framework_id"`
	FrameworkName string `json:"framework_name"`
	Adopted       bool   `json:"adopted"`
	Code          string `json:"code"`
	Title         string `json:"title"`
}

type ControlDetail struct {
	ControlRow
	EvidenceHint    string              `json:"evidence_hint"`
	EvidenceTTLDays int                 `json:"evidence_ttl_days"`
	Requirements    []MappedRequirement `json:"requirements"`
	Tests           []TestRow           `json:"tests"`
	Evidence        []Evidence          `json:"evidence"`
	OpenTasks       []TaskRow           `json:"open_tasks"`
	Policies        []LinkedPolicy      `json:"policies"`
	Risks           []Risk              `json:"risks"`
}

func (s *Service) GetControl(ctx context.Context, orgID, id domain.ID) (*ControlDetail, error) {
	c, err := scanControl(s.Pool.QueryRow(ctx, controlSelect+" WHERE c.org_id = $1 AND c.id = $2", orgID, id))
	if err != nil {
		return nil, err
	}
	d := &ControlDetail{ControlRow: *c, Requirements: []MappedRequirement{}}
	if cat := s.Catalog.Control(c.Code); cat != nil {
		d.EvidenceHint, d.EvidenceTTLDays = cat.EvidenceHint, cat.EvidenceTTLDays
	}
	rows, err := s.Pool.Query(ctx, `SELECT f.id, f.name, (ofw.org_id IS NOT NULL), r.code, r.title
		FROM catalog_requirement_controls rc
		JOIN catalog_requirements r ON r.id = rc.requirement_id
		JOIN catalog_frameworks f ON f.id = r.framework_id
		LEFT JOIN org_frameworks ofw ON ofw.framework_id = f.id AND ofw.org_id = $1
		WHERE rc.control_code = $2
		ORDER BY (ofw.org_id IS NULL), f.position, r.position`, orgID, c.Code)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m MappedRequirement
		if err := rows.Scan(&m.FrameworkID, &m.FrameworkName, &m.Adopted, &m.Code, &m.Title); err != nil {
			rows.Close()
			return nil, err
		}
		d.Requirements = append(d.Requirements, m)
	}
	rows.Close()
	if d.Tests, err = s.ListTests(ctx, orgID, TestFilter{ControlID: &id}); err != nil {
		return nil, err
	}
	if d.Evidence, err = s.ListEvidence(ctx, orgID, id); err != nil {
		return nil, err
	}
	if d.OpenTasks, err = s.ListTasks(ctx, orgID, TaskFilter{ControlID: &id, Status: "open"}); err != nil {
		return nil, err
	}
	if d.Policies, err = s.policiesForControl(ctx, orgID, c.Code); err != nil {
		return nil, err
	}
	cid := id
	if d.Risks, err = s.ListRisks(ctx, orgID, RiskFilter{ControlID: &cid}); err != nil {
		return nil, err
	}
	return d, nil
}

// SetOwner assigns (or clears) the control owner.
func (s *Service) SetOwner(ctx context.Context, orgID, controlID domain.ID, membershipID *domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var code string
		if err := tx.QueryRow(ctx, `SELECT code FROM controls WHERE org_id = $1 AND id = $2`, orgID, controlID).Scan(&code); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		label := "nobody"
		if membershipID != nil {
			if err := tx.QueryRow(ctx, `SELECT u.email FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND m.id = $2`, orgID, *membershipID).Scan(&label); err != nil {
				if db.IsNoRows(err) {
					return domain.Invalid("owner_membership_id", "not a member of this organization")
				}
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE controls SET owner_membership_id = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, controlID, membershipID); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "control.owner_changed", TargetType: "control", TargetID: controlID.String(), TargetLabel: code, Metadata: map[string]any{"owner": label}})
	})
}

// Transition is a control status change produced by recompute.
type Transition struct {
	ControlID domain.ID
	Code      string
	Name      string
	From, To  string
	Reason    string
	OwnerID   *domain.ID // membership id
}

// recompute derives each control's status from its tests and evidence and
// returns the controls whose status changed. controlIDs nil = all controls.
func (s *Service) recompute(ctx context.Context, tx pgx.Tx, orgID domain.ID, controlIDs []domain.ID) ([]Transition, error) {
	args := []any{orgID}
	filter := ""
	if controlIDs != nil {
		if len(controlIDs) == 0 {
			return nil, nil
		}
		args = append(args, controlIDs)
		filter = " AND c.id = ANY($2)"
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.code, c.name, c.status, c.owner_membership_id, c.evidence_mode,
		  (SELECT count(*) FROM tests t WHERE t.org_id = c.org_id AND t.control_id = c.id AND t.enabled AND t.last_status <> 'na'),
		  (SELECT count(*) FROM tests t WHERE t.org_id = c.org_id AND t.control_id = c.id AND t.enabled AND t.last_status = 'pass'),
		  (SELECT count(*) FROM tests t WHERE t.org_id = c.org_id AND t.control_id = c.id AND t.enabled AND t.last_status IN ('fail','error')),
		  (SELECT count(*) FROM evidence e WHERE e.org_id = c.org_id AND e.control_id = c.id AND `+validEvidence+`),
		  (SELECT count(*) FROM evidence e WHERE e.org_id = c.org_id AND e.control_id = c.id AND e.expires_at IS NOT NULL AND e.expires_at <= now()
		     AND COALESCE(e.override_verdict, e.ai_verdict) = 'pass')
		FROM controls c WHERE c.org_id = $1`+filter+` FOR UPDATE OF c`, args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		t                                          Transition
		mode                                       string
		total, passing, failing, evidence, expired int
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.t.ControlID, &r.t.Code, &r.t.Name, &r.t.From, &r.t.OwnerID, &r.mode, &r.total, &r.passing, &r.failing, &r.evidence, &r.expired); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	var changed []Transition
	for _, r := range all {
		status, reason := deriveStatus(r.total, r.passing, r.failing, r.evidence, r.expired, r.mode)
		var cur string
		if err := tx.QueryRow(ctx, `SELECT status_reason FROM controls WHERE org_id = $1 AND id = $2`, orgID, r.t.ControlID).Scan(&cur); err != nil {
			return nil, err
		}
		if status == r.t.From && reason == cur {
			continue
		}
		if status != r.t.From {
			_, err = tx.Exec(ctx, `UPDATE controls SET status = $3, status_reason = $4, status_changed_at = now(), updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, r.t.ControlID, status, reason)
		} else {
			_, err = tx.Exec(ctx, `UPDATE controls SET status_reason = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, r.t.ControlID, reason)
		}
		if err != nil {
			return nil, err
		}
		if status != r.t.From {
			t := r.t
			t.To, t.Reason = status, reason
			changed = append(changed, t)
			if err := activity.Record(ctx, tx, orgID, domain.SystemActor("monitor"), activity.Entry{Action: "control.status_changed", TargetType: "control", TargetID: t.ControlID.String(), TargetLabel: t.Code,
				Metadata: map[string]any{"from": t.From, "to": t.To, "reason": reason}}); err != nil {
				return nil, err
			}
		}
	}
	return changed, nil
}

// deriveStatus is the control status rule:
//   - any enabled test failing or erroring → failing
//   - all enabled tests passing (at least one) → passing
//   - no decisive tests: valid (accepted, unexpired) evidence → passing
//   - otherwise needs_evidence
func deriveStatus(total, passing, failing, evidence, expired int, mode string) (string, string) {
	switch {
	case failing > 0:
		return "failing", plural(failing, "automated test is", "automated tests are") + " failing."
	case total > 0 && passing == total:
		return "passing", plural(passing, "automated test passes", "automated tests pass") + "."
	case evidence > 0:
		if total > 0 {
			return "passing", fmt.Sprintf("Accepted evidence on file; %d test(s) awaiting their first run.", total-passing)
		}
		return "passing", plural(evidence, "accepted evidence item is", "accepted evidence items are") + " current."
	case expired > 0:
		return "needs_evidence", "Evidence has expired. Upload current evidence."
	case total > 0:
		return "needs_evidence", "Awaiting the first test run."
	case mode == "automated":
		return "needs_evidence", initialReason(mode)
	}
	return "needs_evidence", "Upload evidence for this control."
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
