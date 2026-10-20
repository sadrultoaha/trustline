package compliance

import (
	"context"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

// LinkedPolicy is a policy that supports a control.
type LinkedPolicy struct {
	ID               domain.ID `json:"id"`
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	PublishedVersion *int      `json:"published_version,omitempty"`
	OpenStatus       string    `json:"open_status"`
	AcceptanceRate   float64   `json:"acceptance_rate"`
}

func (s *Service) policiesForControl(ctx context.Context, orgID domain.ID, code string) ([]LinkedPolicy, error) {
	rows, err := s.Pool.Query(ctx, policySelect+` WHERE p.org_id = $1 AND $2 = ANY(p.control_codes) ORDER BY p.title`, orgID, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LinkedPolicy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, LinkedPolicy{ID: p.ID, Slug: p.Slug, Title: p.Title, PublishedVersion: p.PublishedVersion, OpenStatus: p.OpenStatus, AcceptanceRate: p.AcceptanceRate})
	}
	return out, rows.Err()
}

// SetPolicyControls replaces the controls a policy supports.
func (s *Service) SetPolicyControls(ctx context.Context, orgID, policyID domain.ID, codes []string, actor domain.Actor) error {
	clean := []string{}
	seen := map[string]bool{}
	v := domain.NewValidation()
	for _, c := range codes {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		var exists bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM controls WHERE org_id = $1 AND code = $2)`, orgID, c).Scan(&exists); err != nil {
			return err
		}
		if !exists && s.Catalog.Control(c) == nil {
			v.Add("control_codes", "unknown control "+c)
		}
		clean = append(clean, c)
	}
	if err := v.Err(); err != nil {
		return err
	}
	sort.Strings(clean)
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		var before []string
		if err := tx.QueryRow(ctx, `SELECT title, control_codes FROM policies WHERE org_id = $1 AND id = $2 FOR UPDATE`, orgID, policyID).Scan(&title, &before); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE policies SET control_codes = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, policyID, clean); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "policy.controls_changed", TargetType: "policy", TargetID: policyID.String(), TargetLabel: title,
			Metadata: map[string]any{"from": before, "to": clean}})
	})
}

// LinkPolicyToControl adds or removes one control on a policy (control page).
func (s *Service) LinkPolicyToControl(ctx context.Context, orgID, controlID, policyID domain.ID, linked bool, actor domain.Actor) error {
	var code string
	if err := s.Pool.QueryRow(ctx, `SELECT code FROM controls WHERE org_id = $1 AND id = $2`, orgID, controlID).Scan(&code); err != nil {
		if db.IsNoRows(err) {
			return domain.ErrNotFound
		}
		return err
	}
	var codes []string
	if err := s.Pool.QueryRow(ctx, `SELECT control_codes FROM policies WHERE org_id = $1 AND id = $2`, orgID, policyID).Scan(&codes); err != nil {
		if db.IsNoRows(err) {
			return domain.Invalid("policy_id", "policy not found")
		}
		return err
	}
	next := []string{}
	for _, c := range codes {
		if c != code {
			next = append(next, c)
		}
	}
	if linked {
		next = append(next, code)
	}
	return s.SetPolicyControls(ctx, orgID, policyID, next, actor)
}
