package compliance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

// workItem is a system-managed task (onboarding, offboarding, policy
// acknowledgement, access revocation, policy review, training renewal).
// source_key makes creation idempotent: one open task per key.
type workItem struct {
	Kind, Title, Description, Priority string
	SourceKey, ItemKey                 string
	PersonID                           *domain.ID
	ControlID                          *domain.ID
	Assignee                           *domain.ID // membership
	Steps                              []string
	Due                                *time.Time
	Meta                               map[string]any
}

// openWorkItem creates the task unless an open one with the same key exists
// (or, when onceEver is true, unless any task with that key ever existed).
func (s *Service) openWorkItem(ctx context.Context, tx pgx.Tx, orgID domain.ID, w workItem, onceEver bool) (*domain.ID, error) {
	q := `SELECT id FROM tasks WHERE org_id = $1 AND source_key = $2 AND status = 'open'`
	if onceEver {
		q = `SELECT id FROM tasks WHERE org_id = $1 AND source_key = $2 LIMIT 1`
	}
	var existing domain.ID
	err := tx.QueryRow(ctx, q, orgID, w.SourceKey).Scan(&existing)
	if err == nil {
		return nil, nil
	}
	if !db.IsNoRows(err) {
		return nil, err
	}
	if w.Steps == nil {
		w.Steps = []string{}
	}
	if w.Meta == nil {
		w.Meta = map[string]any{}
	}
	if w.Priority == "" {
		w.Priority = "medium"
	}
	steps, _ := json.Marshal(w.Steps)
	meta, _ := json.Marshal(w.Meta)
	id := domain.NewID()
	if _, err := tx.Exec(ctx, `INSERT INTO tasks (id, org_id, kind, title, description, priority, control_id, person_id, assignee_membership_id, source_key, item_key, steps, meta, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		id, orgID, w.Kind, w.Title, w.Description, w.Priority, w.ControlID, w.PersonID, w.Assignee, w.SourceKey, w.ItemKey, steps, meta, w.Due); err != nil {
		return nil, err
	}
	if err := activity.Record(ctx, tx, orgID, domain.SystemActor("trustline"), activity.Entry{Action: "task.created", TargetType: "task", TargetID: id.String(), TargetLabel: w.Title,
		Metadata: map[string]any{"kind": w.Kind, "source": w.SourceKey}}); err != nil {
		return nil, err
	}
	return &id, nil
}

// closeWorkItems marks open tasks with the given keys done (or dismissed).
func (s *Service) closeWorkItems(ctx context.Context, tx pgx.Tx, orgID domain.ID, keys []string, status, resolution string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `UPDATE tasks SET status = $3, resolution = $4, completed_at = now(), updated_at = now()
		WHERE org_id = $1 AND source_key = ANY($2) AND status = 'open' RETURNING id, title`, orgID, keys, status, resolution)
	if err != nil {
		return 0, err
	}
	type closed struct {
		id    domain.ID
		title string
	}
	var done []closed
	for rows.Next() {
		var c closed
		if err := rows.Scan(&c.id, &c.title); err != nil {
			rows.Close()
			return 0, err
		}
		done = append(done, c)
	}
	rows.Close()
	for _, c := range done {
		if err := activity.Record(ctx, tx, orgID, domain.SystemActor("trustline"), activity.Entry{Action: "task." + status, TargetType: "task", TargetID: c.id.String(), TargetLabel: c.title,
			Metadata: map[string]any{"resolution": resolution}}); err != nil {
			return 0, err
		}
	}
	return len(done), nil
}

// membershipByEmail resolves an org membership id for an email.
func membershipByEmail(ctx context.Context, q db.Querier, orgID domain.ID, email string) (*domain.ID, error) {
	var id domain.ID
	err := q.QueryRow(ctx, `SELECT m.id FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND lower(u.email) = lower($2)`, orgID, email).Scan(&id)
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func controlIDByCode(ctx context.Context, q db.Querier, orgID domain.ID, code string) *domain.ID {
	var id domain.ID
	if err := q.QueryRow(ctx, `SELECT id FROM controls WHERE org_id = $1 AND code = $2`, orgID, code).Scan(&id); err != nil {
		return nil
	}
	return &id
}
