package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/testengine"
)

type TestRow struct {
	ID              domain.ID           `json:"id"`
	CatalogID       *string             `json:"catalog_id,omitempty"`
	ControlID       domain.ID           `json:"control_id"`
	ControlCode     string              `json:"control_code"`
	ControlName     string              `json:"control_name"`
	IntegrationType string              `json:"integration_type"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Severity        string              `json:"severity"`
	Rule            json.RawMessage     `json:"rule"`
	Remediation     catalog.Remediation `json:"remediation"`
	Custom          bool                `json:"custom"`
	Enabled         bool                `json:"enabled"`
	LastStatus      string              `json:"last_status"`
	LastRunAt       *time.Time          `json:"last_run_at,omitempty"`
	LastRunID       *domain.ID          `json:"last_run_id,omitempty"`
	LastSummary     string              `json:"last_summary"`
}

type TestRun struct {
	ID         domain.ID            `json:"id"`
	TestID     domain.ID            `json:"test_id"`
	SnapshotID *domain.ID           `json:"snapshot_id,omitempty"`
	Status     string               `json:"status"`
	Evaluated  int                  `json:"evaluated"`
	Failing    []testengine.Failure `json:"failing"`
	Summary    string               `json:"summary"`
	Error      string               `json:"error"`
	Trigger    string               `json:"trigger"`
	CreatedAt  time.Time            `json:"created_at"`
}

const testSelect = `SELECT t.id, t.catalog_id, t.control_id, c.code, c.name, t.integration_type, t.name, t.description, t.severity, t.rule, t.remediation,
  t.custom, t.enabled, t.last_status, t.last_run_at, t.last_run_id, t.last_summary
FROM tests t JOIN controls c ON c.id = t.control_id AND c.org_id = t.org_id`

func scanTest(row pgx.Row) (*TestRow, error) {
	var t TestRow
	var rem []byte
	if err := row.Scan(&t.ID, &t.CatalogID, &t.ControlID, &t.ControlCode, &t.ControlName, &t.IntegrationType, &t.Name, &t.Description, &t.Severity, &t.Rule, &rem,
		&t.Custom, &t.Enabled, &t.LastStatus, &t.LastRunAt, &t.LastRunID, &t.LastSummary); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(rem, &t.Remediation)
	if t.Remediation.Steps == nil {
		t.Remediation.Steps = []string{}
	}
	return &t, nil
}

type TestFilter struct {
	ControlID       *domain.ID
	IntegrationType string
	Status          string
	Query           string
	IncludeDisabled bool
}

func (s *Service) ListTests(ctx context.Context, orgID domain.ID, f TestFilter) ([]TestRow, error) {
	args := []any{orgID}
	where := []string{"t.org_id = $1"}
	if !f.IncludeDisabled {
		where = append(where, "t.enabled")
	}
	if f.ControlID != nil {
		args = append(args, *f.ControlID)
		where = append(where, fmt.Sprintf("t.control_id = $%d", len(args)))
	}
	if f.IntegrationType != "" {
		args = append(args, f.IntegrationType)
		where = append(where, fmt.Sprintf("t.integration_type = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("t.last_status = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf("(lower(t.name) LIKE $%d OR lower(c.code) LIKE $%d OR lower(coalesce(t.catalog_id,'')) LIKE $%d)", len(args), len(args), len(args)))
	}
	rows, err := s.Pool.Query(ctx, testSelect+" WHERE "+strings.Join(where, " AND ")+
		` ORDER BY CASE t.last_status WHEN 'fail' THEN 0 WHEN 'error' THEN 1 WHEN 'pending' THEN 2 ELSE 3 END,
		  CASE t.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, t.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TestRow{}
	for rows.Next() {
		t, err := scanTest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

type TestDetail struct {
	TestRow
	Runs []TestRun `json:"runs"`
}

func (s *Service) GetTest(ctx context.Context, orgID, id domain.ID) (*TestDetail, error) {
	t, err := scanTest(s.Pool.QueryRow(ctx, testSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, id))
	if err != nil {
		return nil, err
	}
	d := &TestDetail{TestRow: *t, Runs: []TestRun{}}
	rows, err := s.Pool.Query(ctx, `SELECT id, test_id, snapshot_id, status, evaluated, failing, summary, error, trigger, created_at
		FROM test_runs WHERE org_id = $1 AND test_id = $2 ORDER BY created_at DESC LIMIT 30`, orgID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		d.Runs = append(d.Runs, *r)
	}
	return d, rows.Err()
}

func scanRun(row pgx.Row) (*TestRun, error) {
	var r TestRun
	var failing []byte
	if err := row.Scan(&r.ID, &r.TestID, &r.SnapshotID, &r.Status, &r.Evaluated, &failing, &r.Summary, &r.Error, &r.Trigger, &r.CreatedAt); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(failing, &r.Failing)
	if r.Failing == nil {
		r.Failing = []testengine.Failure{}
	}
	return &r, nil
}

// RunDetail is a run plus the frozen evidence snapshot of the evaluated resources.
type RunDetail struct {
	TestRun
	TestName  string           `json:"test_name"`
	Resource  string           `json:"resource"`
	Resources []map[string]any `json:"resources"`
	TakenAt   *time.Time       `json:"snapshot_taken_at,omitempty"`
	SHA256    string           `json:"snapshot_sha256,omitempty"`
}

func (s *Service) GetRun(ctx context.Context, orgID, runID domain.ID) (*RunDetail, error) {
	r, err := scanRun(s.Pool.QueryRow(ctx, `SELECT id, test_id, snapshot_id, status, evaluated, failing, summary, error, trigger, created_at
		FROM test_runs WHERE org_id = $1 AND id = $2`, orgID, runID))
	if err != nil {
		return nil, err
	}
	d := &RunDetail{TestRun: *r, Resources: []map[string]any{}}
	var rule []byte
	if err := s.Pool.QueryRow(ctx, `SELECT name, rule FROM tests WHERE org_id = $1 AND id = $2`, orgID, r.TestID).Scan(&d.TestName, &rule); err != nil {
		return nil, err
	}
	var rr testengine.Rule
	_ = json.Unmarshal(rule, &rr)
	d.Resource = rr.Resource
	if r.SnapshotID != nil {
		if snap, err := s.GetSnapshot(ctx, orgID, *r.SnapshotID); err == nil {
			d.Resources = snap.Resources[rr.Resource]
			d.TakenAt, d.SHA256 = &snap.TakenAt, snap.SHA256
		}
	}
	return d, nil
}

// ensureTests instantiates catalog tests for every (control in scope ×
// connected integration type) and re-enables tests of reconnected integrations.
func (s *Service) ensureTests(ctx context.Context, tx pgx.Tx, orgID domain.ID) (int, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO tests (id, org_id, catalog_id, control_id, integration_type, name, description, severity, rule, remediation)
		SELECT gen_random_uuid(), $1, ct.id, c.id, ct.connector, ct.name, ct.description, ct.severity, ct.rule, ct.remediation
		FROM catalog_tests ct
		JOIN controls c ON c.org_id = $1 AND c.code = ct.control_code
		JOIN integrations i ON i.org_id = $1 AND i.type = ct.connector
		ON CONFLICT (org_id, catalog_id) DO NOTHING`, orgID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tests t SET enabled = true, updated_at = now()
		WHERE t.org_id = $1 AND NOT t.enabled AND t.catalog_id IS NOT NULL
		  AND EXISTS (SELECT 1 FROM integrations i WHERE i.org_id = $1 AND i.type = t.integration_type)`, orgID); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

type testTransition struct {
	TestID   domain.ID
	Name     string
	Control  domain.ID
	Severity string
	From, To string
	RunID    domain.ID
	Failing  []testengine.Failure
}

// RunTests evaluates the org's enabled tests for the snapshot's connector.
func (s *Service) RunTests(ctx context.Context, orgID, snapshotID domain.ID, trigger string) (int, error) {
	snap, err := s.GetSnapshot(ctx, orgID, snapshotID)
	if err != nil {
		return 0, err
	}
	// Jobs may run out of order; never let an older snapshot overwrite newer results.
	if snap.IntegrationID != nil {
		var newer bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM snapshots WHERE org_id = $1 AND integration_id = $2 AND taken_at > $3)`, orgID, *snap.IntegrationID, snap.TakenAt).Scan(&newer); err != nil {
			return 0, err
		}
		if newer {
			return 0, nil
		}
	}
	return s.runOver(ctx, orgID, snap, nil, trigger)
}

// RunTestNow evaluates one test against the latest snapshot of its integration.
func (s *Service) RunTestNow(ctx context.Context, orgID, testID domain.ID, actor domain.Actor) (*TestRow, error) {
	t, err := scanTest(s.Pool.QueryRow(ctx, testSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, testID))
	if err != nil {
		return nil, err
	}
	snap, err := s.snapshot(ctx, `WHERE org_id = $1 AND connector = $2 AND integration_id IS NOT NULL ORDER BY taken_at DESC LIMIT 1`, orgID, t.IntegrationType)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, domain.Invalid("integration", "no data has been synced from "+t.IntegrationType+" yet; connect it and wait for the first sync")
		}
		return nil, err
	}
	if _, err := s.runOver(ctx, orgID, snap, []domain.ID{testID}, "manual"); err != nil {
		return nil, err
	}
	_ = activity.Record(ctx, s.Pool, orgID, actor, activity.Entry{Action: "test.run_requested", TargetType: "test", TargetID: testID.String(), TargetLabel: t.Name})
	return scanTest(s.Pool.QueryRow(ctx, testSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, testID))
}

// RescanOrg runs every enabled test on the latest snapshot of each integration.
func (s *Service) RescanOrg(ctx context.Context, orgID domain.ID, trigger string) error {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT ON (integration_id) id FROM snapshots
		WHERE org_id = $1 AND integration_id IS NOT NULL ORDER BY integration_id, taken_at DESC`, orgID)
	if err != nil {
		return err
	}
	var ids []domain.ID
	for rows.Next() {
		var id domain.ID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.RunTests(ctx, orgID, id, trigger); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runOver(ctx context.Context, orgID domain.ID, snap *SnapshotView, only []domain.ID, trigger string) (int, error) {
	var n int
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		args := []any{orgID, snap.Connector}
		filter := ""
		if only != nil {
			args = append(args, only)
			filter = " AND t.id = ANY($3)"
		}
		rows, err := tx.Query(ctx, testSelect+" WHERE t.org_id = $1 AND t.integration_type = $2 AND t.enabled"+filter+" FOR UPDATE OF t", args...)
		if err != nil {
			return err
		}
		var tests []TestRow
		for rows.Next() {
			t, err := scanTest(rows)
			if err != nil {
				rows.Close()
				return err
			}
			tests = append(tests, *t)
		}
		rows.Close()
		var transitions []testTransition
		controls := map[domain.ID]bool{}
		for _, t := range tests {
			status, evaluated, summary, errMsg := "error", 0, "", ""
			failing := []testengine.Failure{}
			rule, perr := testengine.Parse(t.Rule, nil)
			if perr != nil {
				errMsg = perr.Error()
				summary = "Rule is invalid"
			} else {
				var res []testengine.Resource
				for _, r := range snap.Resources[rule.Resource] {
					res = append(res, r)
				}
				out := rule.Evaluate(res)
				status, evaluated, summary, failing = out.Status, out.Evaluated, out.Summary, out.Failing
			}
			runID := domain.NewID()
			fj, _ := json.Marshal(failing)
			snapID := snap.ID
			if _, err := tx.Exec(ctx, `INSERT INTO test_runs (id, org_id, test_id, snapshot_id, status, evaluated, failing, summary, error, trigger)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, runID, orgID, t.ID, snapID, status, evaluated, fj, summary, errMsg, trigger); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE tests SET last_status = $3, last_run_at = now(), last_run_id = $4, last_summary = $5, updated_at = now() WHERE org_id = $1 AND id = $2`,
				orgID, t.ID, status, runID, summary); err != nil {
				return err
			}
			n++
			controls[t.ControlID] = true
			if status != t.LastStatus {
				transitions = append(transitions, testTransition{TestID: t.ID, Name: t.Name, Control: t.ControlID, Severity: t.Severity, From: t.LastStatus, To: status, RunID: runID, Failing: failing})
			}
		}
		var ids []domain.ID
		for id := range controls {
			ids = append(ids, id)
		}
		ctlTransitions, err := s.recompute(ctx, tx, orgID, ids)
		if err != nil {
			return err
		}
		if err := s.handleTransitions(ctx, tx, orgID, transitions, ctlTransitions); err != nil {
			return err
		}
		// Identity and HR data feed the platform's people view; refresh it.
		if snap.Connector != "trustline" && only == nil {
			return s.touchPlatform(ctx, tx, orgID)
		}
		return nil
	})
	return n, err
}

// handleTransitions turns status changes into remediation tasks and notifications.
func (s *Service) handleTransitions(ctx context.Context, tx pgx.Tx, orgID domain.ID, tests []testTransition, ctls []Transition) error {
	for _, t := range tests {
		key := "test:" + t.TestID.String()
		switch {
		case t.To == "fail":
			taskID, created, err := s.openRemediationTask(ctx, tx, orgID, t)
			if err != nil {
				return err
			}
			if created {
				if err := s.Jobs.EnqueueTx(ctx, tx, jobs.GenerateRemediationArgs{OrgID: orgID.String(), TaskID: taskID.String()}); err != nil {
					return err
				}
			}
			if t.Severity == "critical" {
				if err := s.suggestRiskForTest(ctx, tx, orgID, t); err != nil {
					return err
				}
			}
		case (t.To == "pass" || t.To == "na") && (t.From == "fail" || t.From == "error"):
			why := "Resolved automatically: the test passed on "
			if t.To == "na" {
				why = "Closed automatically: the test stopped applying on "
			}
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'done', resolution = $3, completed_at = now(), updated_at = now()
				WHERE org_id = $1 AND source_key = $2 AND status = 'open'`, orgID, key, why+time.Now().UTC().Format("2006-01-02 15:04 MST")+"."); err != nil {
				return err
			}
		}
	}
	for _, c := range ctls {
		switch {
		case c.To == "failing":
			if err := s.notifyControl(ctx, tx, orgID, c, "control_failing",
				fmt.Sprintf("%s %s is failing", c.Code, c.Name), c.Reason+" A remediation task has been created."); err != nil {
				return err
			}
		case c.From == "failing":
			if err := s.notifyControl(ctx, tx, orgID, c, "control_recovered",
				fmt.Sprintf("%s %s is %s again", c.Code, c.Name, strings.ReplaceAll(c.To, "_", " ")), c.Reason); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) openRemediationTask(ctx context.Context, tx pgx.Tx, orgID domain.ID, t testTransition) (domain.ID, bool, error) {
	key := "test:" + t.TestID.String()
	var existing domain.ID
	err := tx.QueryRow(ctx, `SELECT id FROM tasks WHERE org_id = $1 AND source_key = $2 AND status = 'open'`, orgID, key).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !db.IsNoRows(err) {
		return domain.ID{}, false, err
	}
	var rem []byte
	var owner *domain.ID
	if err := tx.QueryRow(ctx, `SELECT t.remediation, c.owner_membership_id FROM tests t JOIN controls c ON c.id = t.control_id WHERE t.org_id = $1 AND t.id = $2`, orgID, t.TestID).Scan(&rem, &owner); err != nil {
		return domain.ID{}, false, err
	}
	var r catalog.Remediation
	_ = json.Unmarshal(rem, &r)
	steps := r.Steps
	if steps == nil {
		steps = []string{}
	}
	stepsJSON, _ := json.Marshal(steps)
	var ids []string
	for _, f := range t.Failing {
		ids = append(ids, f.ID)
	}
	desc := r.Summary
	if len(ids) > 0 {
		desc += "\n\nFailing resources: " + strings.Join(ids, ", ")
	}
	id := domain.NewID()
	due := time.Now().Add(slaFor(t.Severity))
	if _, err := tx.Exec(ctx, `INSERT INTO tasks (id, org_id, kind, title, description, priority, control_id, test_id, assignee_membership_id, source_key, steps, ai_status, due_at)
		VALUES ($1,$2,'remediation',$3,$4,$5,$6,$7,$8,$9,$10,'pending',$11)`,
		id, orgID, "Fix: "+t.Name, desc, t.Severity, t.Control, t.TestID, owner, key, stepsJSON, due); err != nil {
		return domain.ID{}, false, err
	}
	if err := activity.Record(ctx, tx, orgID, domain.SystemActor("monitor"), activity.Entry{Action: "task.created", TargetType: "task", TargetID: id.String(), TargetLabel: "Fix: " + t.Name,
		Metadata: map[string]any{"test_id": t.TestID, "failing": ids}}); err != nil {
		return domain.ID{}, false, err
	}
	return id, true, nil
}

func slaFor(severity string) time.Duration {
	switch severity {
	case "critical":
		return 24 * time.Hour
	case "high":
		return 7 * 24 * time.Hour
	case "medium":
		return 30 * 24 * time.Hour
	}
	return 90 * 24 * time.Hour
}

// --- custom tests -----------------------------------------------------------

type CustomTestInput struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ControlID       string          `json:"control_id"`
	IntegrationType string          `json:"integration_type"`
	Severity        string          `json:"severity"`
	Rule            json.RawMessage `json:"rule"`
	Remediation     struct {
		Summary string   `json:"summary"`
		Steps   []string `json:"steps"`
	} `json:"remediation"`
}

// ValidateRule checks a rule against a connector's resource schema.
func (s *Service) ValidateRule(integrationType string, raw json.RawMessage) (*testengine.Rule, error) {
	if s.Catalog.ConnectorDef(integrationType) == nil {
		return nil, domain.Invalid("integration_type", "unknown integration type")
	}
	rule, err := testengine.Parse(raw, s.Catalog.FieldChecker(integrationType))
	if err != nil {
		return nil, domain.Invalid("rule", err.Error())
	}
	return rule, nil
}

// PreviewRule evaluates an unsaved rule against the latest snapshot (builder "Try it").
func (s *Service) PreviewRule(ctx context.Context, orgID domain.ID, integrationType string, raw json.RawMessage) (*testengine.Result, error) {
	rule, err := s.ValidateRule(integrationType, raw)
	if err != nil {
		return nil, err
	}
	snap, err := s.snapshot(ctx, `WHERE org_id = $1 AND connector = $2 AND integration_id IS NOT NULL ORDER BY taken_at DESC LIMIT 1`, orgID, integrationType)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, domain.Invalid("integration_type", "connect "+integrationType+" first to preview against real data")
		}
		return nil, err
	}
	var res []testengine.Resource
	for _, r := range snap.Resources[rule.Resource] {
		res = append(res, r)
	}
	out := rule.Evaluate(res)
	return &out, nil
}

func (s *Service) CreateCustomTest(ctx context.Context, orgID domain.ID, in CustomTestInput, actor domain.Actor) (*TestRow, error) {
	v := domain.NewValidation()
	if strings.TrimSpace(in.Name) == "" {
		v.Add("name", "name is required")
	}
	switch in.Severity {
	case "low", "medium", "high", "critical":
	default:
		v.Add("severity", "choose low, medium, high or critical")
	}
	controlID, err := domain.ParseID(in.ControlID)
	if err != nil {
		v.Add("control_id", "choose a control")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	rule, err := s.ValidateRule(in.IntegrationType, in.Rule)
	if err != nil {
		return nil, err
	}
	ruleJSON, _ := json.Marshal(rule)
	steps := in.Remediation.Steps
	if steps == nil {
		steps = []string{}
	}
	rem, _ := json.Marshal(catalog.Remediation{Summary: in.Remediation.Summary, Steps: steps})
	id := domain.NewID()
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var code string
		if err := tx.QueryRow(ctx, `SELECT code FROM controls WHERE org_id = $1 AND id = $2`, orgID, controlID).Scan(&code); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("control_id", "control not found")
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tests (id, org_id, control_id, integration_type, name, description, severity, rule, remediation, custom, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,true,$10)`, id, orgID, controlID, in.IntegrationType, strings.TrimSpace(in.Name), in.Description, in.Severity, ruleJSON, rem, actor.ID); err != nil {
			return err
		}
		if _, err := s.recompute(ctx, tx, orgID, []domain.ID{controlID}); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "test.created", TargetType: "test", TargetID: id.String(), TargetLabel: in.Name,
			Metadata: map[string]any{"control": code, "integration_type": in.IntegrationType, "rule": json.RawMessage(ruleJSON)}}); err != nil {
			return err
		}
		return s.Jobs.EnqueueTx(ctx, tx, jobs.RescanOrgArgs{OrgID: orgID.String(), Trigger: "test_created"})
	})
	if err != nil {
		return nil, err
	}
	return scanTest(s.Pool.QueryRow(ctx, testSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, id))
}

// SetTestEnabled turns a test on or off (e.g. accepted risk).
func (s *Service) SetTestEnabled(ctx context.Context, orgID, id domain.ID, enabled bool, actor domain.Actor) (*TestRow, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		var control domain.ID
		if err := tx.QueryRow(ctx, `UPDATE tests SET enabled = $3, updated_at = now() WHERE org_id = $1 AND id = $2 RETURNING name, control_id`, orgID, id, enabled).Scan(&name, &control); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if !enabled {
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'dismissed', resolution = 'Test disabled', updated_at = now() WHERE org_id = $1 AND source_key = $2 AND status = 'open'`, orgID, "test:"+id.String()); err != nil {
				return err
			}
		}
		if _, err := s.recompute(ctx, tx, orgID, []domain.ID{control}); err != nil {
			return err
		}
		action := "test.disabled"
		if enabled {
			action = "test.enabled"
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: action, TargetType: "test", TargetID: id.String(), TargetLabel: name})
	})
	if err != nil {
		return nil, err
	}
	return scanTest(s.Pool.QueryRow(ctx, testSelect+" WHERE t.org_id = $1 AND t.id = $2", orgID, id))
}

// ResourceSchema is exposed to the builder UI.
func (s *Service) ResourceSchema() []catalog.Connector { return s.Catalog.Connectors }
