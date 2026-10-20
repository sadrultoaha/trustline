package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/testengine"
)

type TaskRow struct {
	ID          domain.ID      `json:"id"`
	Kind        string         `json:"kind"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Priority    string         `json:"priority"`
	ControlID   *domain.ID     `json:"control_id,omitempty"`
	ControlCode string         `json:"control_code"`
	TestID      *domain.ID     `json:"test_id,omitempty"`
	Assignee    *Owner         `json:"assignee,omitempty"`
	Steps       []string       `json:"steps"`
	Terraform   string         `json:"terraform"`
	AIStatus    string         `json:"ai_status"`
	AIModel     string         `json:"ai_model"`
	AINotes     string         `json:"ai_notes"`
	Resolution  string         `json:"resolution"`
	DueAt       *time.Time     `json:"due_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	Overdue     bool           `json:"overdue"`
	PersonID    *domain.ID     `json:"person_id,omitempty"`
	PersonName  string         `json:"person_name"`
	ItemKey     string         `json:"item_key"`
	Meta        map[string]any `json:"meta"`
}

const taskSelect = `SELECT t.id, t.kind, t.title, t.description, t.status, t.priority, t.control_id, COALESCE(c.code, ''), t.test_id,
  m.id, u.name, u.email, t.steps, t.terraform, t.ai_status, t.ai_model, t.ai_notes, t.resolution, t.due_at, t.completed_at, t.created_at,
  t.person_id, COALESCE(p.name, ''), t.item_key, t.meta
FROM tasks t
LEFT JOIN controls c ON c.id = t.control_id AND c.org_id = t.org_id
LEFT JOIN people p ON p.id = t.person_id AND p.org_id = t.org_id
LEFT JOIN org_memberships m ON m.id = t.assignee_membership_id AND m.org_id = t.org_id
LEFT JOIN users u ON u.id = m.user_id`

func scanTask(row pgx.Row) (*TaskRow, error) {
	var t TaskRow
	var mID *domain.ID
	var mName, mEmail *string
	var steps, meta []byte
	if err := row.Scan(&t.ID, &t.Kind, &t.Title, &t.Description, &t.Status, &t.Priority, &t.ControlID, &t.ControlCode, &t.TestID,
		&mID, &mName, &mEmail, &steps, &t.Terraform, &t.AIStatus, &t.AIModel, &t.AINotes, &t.Resolution, &t.DueAt, &t.CompletedAt, &t.CreatedAt,
		&t.PersonID, &t.PersonName, &t.ItemKey, &meta); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if mID != nil {
		t.Assignee = &Owner{MembershipID: *mID, Name: deref(mName), Email: deref(mEmail)}
	}
	_ = json.Unmarshal(steps, &t.Steps)
	_ = json.Unmarshal(meta, &t.Meta)
	if t.Meta == nil {
		t.Meta = map[string]any{}
	}
	if t.Steps == nil {
		t.Steps = []string{}
	}
	t.Overdue = t.Status == "open" && t.DueAt != nil && t.DueAt.Before(time.Now())
	return &t, nil
}

type TaskFilter struct {
	Status     string
	ControlID  *domain.ID
	AssigneeID *domain.ID // membership
	PersonID   *domain.ID
	Kinds      []string
}

func (s *Service) ListTasks(ctx context.Context, orgID domain.ID, f TaskFilter) ([]TaskRow, error) {
	args := []any{orgID}
	where := []string{"t.org_id = $1"}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("t.status = $%d", len(args)))
	}
	if f.ControlID != nil {
		args = append(args, *f.ControlID)
		where = append(where, fmt.Sprintf("t.control_id = $%d", len(args)))
	}
	if f.AssigneeID != nil {
		args = append(args, *f.AssigneeID)
		where = append(where, fmt.Sprintf("t.assignee_membership_id = $%d", len(args)))
	}
	if f.PersonID != nil {
		args = append(args, *f.PersonID)
		where = append(where, fmt.Sprintf("t.person_id = $%d", len(args)))
	}
	if len(f.Kinds) > 0 {
		args = append(args, f.Kinds)
		where = append(where, fmt.Sprintf("t.kind = ANY($%d)", len(args)))
	}
	rows, err := s.Pool.Query(ctx, taskSelect+" WHERE "+strings.Join(where, " AND ")+
		` ORDER BY CASE t.status WHEN 'open' THEN 0 ELSE 1 END, CASE t.priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, t.created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskRow{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Service) GetTask(ctx context.Context, orgID, id domain.ID) (*TaskRow, error) {
	return scanTask(s.Pool.QueryRow(ctx, taskSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, id))
}

type TaskUpdate struct {
	Status     *string
	Assignee   *domain.ID
	ClearOwner bool
	Resolution *string
}

func (s *Service) UpdateTask(ctx context.Context, orgID, id domain.ID, in TaskUpdate, actor domain.Actor) (*TaskRow, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		cur, err := scanTask(tx.QueryRow(ctx, taskSelect+" WHERE t.org_id = $1 AND t.id = $2 FOR UPDATE OF t", orgID, id))
		if err != nil {
			return err
		}
		meta := map[string]any{}
		if in.Status != nil {
			switch *in.Status {
			case "open", "done", "dismissed":
			default:
				return domain.Invalid("status", "status must be open, done or dismissed")
			}
			if *in.Status != cur.Status {
				res := cur.Resolution
				if in.Resolution != nil {
					res = *in.Resolution
				}
				var completed any
				if *in.Status != "open" {
					completed = time.Now()
				}
				if _, err := tx.Exec(ctx, `UPDATE tasks SET status = $3, resolution = $4, completed_at = $5, completed_by = $6, updated_at = now() WHERE org_id = $1 AND id = $2`,
					orgID, id, *in.Status, res, completed, actor.ID); err != nil {
					if db.IsUniqueViolation(err) {
						return domain.Invalid("status", "another open task already tracks this issue")
					}
					return err
				}
				meta["status"] = map[string]string{"from": cur.Status, "to": *in.Status}
			}
		}
		if in.Assignee != nil || in.ClearOwner {
			if in.Assignee != nil {
				var ok bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_memberships WHERE org_id = $1 AND id = $2)`, orgID, *in.Assignee).Scan(&ok); err != nil {
					return err
				}
				if !ok {
					return domain.Invalid("assignee_membership_id", "not a member of this organization")
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE tasks SET assignee_membership_id = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, in.Assignee); err != nil {
				return err
			}
			meta["assignee"] = in.Assignee
		}
		if len(meta) == 0 {
			return nil
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "task.updated", TargetType: "task", TargetID: id.String(), TargetLabel: cur.Title, Metadata: meta})
	})
	if err != nil {
		return nil, err
	}
	return s.GetTask(ctx, orgID, id)
}

// RegenerateRemediation re-queues AI generation for a remediation task.
func (s *Service) RegenerateRemediation(ctx context.Context, orgID, id domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		if err := tx.QueryRow(ctx, `UPDATE tasks SET ai_status = 'pending', updated_at = now() WHERE org_id = $1 AND id = $2 AND kind = 'remediation' RETURNING title`, orgID, id).Scan(&title); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "task.remediation_regenerated", TargetType: "task", TargetID: id.String(), TargetLabel: title}); err != nil {
			return err
		}
		return s.Jobs.EnqueueTx(ctx, tx, jobs.GenerateRemediationArgs{OrgID: orgID.String(), TaskID: id.String()})
	})
}

// GenerateRemediation fills a remediation task with an AI plan (worker).
func (s *Service) GenerateRemediation(ctx context.Context, svc *ai.Service, orgID, taskID domain.ID) error {
	var testID *domain.ID
	if err := s.Pool.QueryRow(ctx, `SELECT test_id FROM tasks WHERE org_id = $1 AND id = $2`, orgID, taskID).Scan(&testID); err != nil {
		if db.IsNoRows(err) {
			return nil
		}
		return err
	}
	if testID == nil {
		return nil
	}
	t, err := scanTest(s.Pool.QueryRow(ctx, testSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, *testID))
	if err != nil {
		return err
	}
	var failing []testengine.Failure
	if t.LastRunID != nil {
		if run, err := scanRun(s.Pool.QueryRow(ctx, `SELECT id, test_id, snapshot_id, status, evaluated, failing, summary, error, trigger, created_at FROM test_runs WHERE org_id = $1 AND id = $2`, orgID, *t.LastRunID)); err == nil {
			failing = run.Failing
		}
	}
	var rule testengine.Rule
	_ = json.Unmarshal(t.Rule, &rule)
	in := ai.RemediationInput{ControlCode: t.ControlCode, ControlName: t.ControlName, TestName: t.Name, Description: t.Description, Connector: t.IntegrationType,
		Resource: rule.Resource, Severity: t.Severity, RuleJSON: string(t.Rule), Summary: t.Remediation.Summary, Steps: t.Remediation.Steps, IaCHint: t.Remediation.IaCHint}
	for _, f := range failing {
		in.Failing = append(in.Failing, ai.Failing{ID: f.ID, Reason: f.Reason})
	}
	plan, model, err := svc.RemediationPlan(ctx, orgID, in)
	switch {
	case errors.Is(err, ai.ErrDisabled):
		_, err = s.Pool.Exec(ctx, `UPDATE tasks SET ai_status = 'skipped', ai_notes = 'AI is disabled for this organization; steps come from the test library.', updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, taskID)
		return err
	case err != nil:
		_, _ = s.Pool.Exec(ctx, `UPDATE tasks SET ai_status = 'error', ai_notes = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, taskID, "Remediation generation failed: "+err.Error())
		return err
	}
	steps, _ := json.Marshal(plan.Steps)
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE tasks SET steps = $3, terraform = $4, ai_status = 'done', ai_model = $5, ai_notes = $6, description = $7, updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, taskID, steps, plan.Terraform, model, plan.Notes, plan.Summary+failingSuffix(failing)); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, domain.Actor{Type: domain.ActorAgent, Label: "remediation-planner"}, activity.Entry{Action: "task.remediation_generated", TargetType: "task", TargetID: taskID.String(),
			TargetLabel: "Fix: " + t.Name, Metadata: map[string]any{"model": model, "has_terraform": plan.Terraform != ""}})
	})
}

func failingSuffix(f []testengine.Failure) string {
	var ids []string
	for _, x := range f {
		if x.ID != "-" {
			ids = append(ids, x.ID)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	return "\n\nFailing resources: " + strings.Join(ids, ", ")
}

var _ = catalog.Remediation{}
