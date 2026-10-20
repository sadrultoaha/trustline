package compliance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/connectors"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
)

type Integration struct {
	ID              domain.ID         `json:"id"`
	Type            string            `json:"type"`
	Name            string            `json:"name"`
	Mode            string            `json:"mode"`
	CredentialsHint map[string]string `json:"credentials_hint"`
	Enabled         bool              `json:"enabled"`
	Status          string            `json:"status"`
	LastSyncAt      *time.Time        `json:"last_sync_at,omitempty"`
	LastError       string            `json:"last_error"`
	ResourceCount   int               `json:"resource_count"`
	CreatedAt       time.Time         `json:"created_at"`
	TestsTotal      int               `json:"tests_total"`
	TestsFailing    int               `json:"tests_failing"`
	Scenarios       []string          `json:"active_scenarios"`
}

type IntegrationType struct {
	catalog.Connector
	Credentials []connectors.CredentialField `json:"credentials"`
	LiveReady   bool                         `json:"live_ready"`
	Scenarios   []connectors.ScenarioInfo    `json:"scenarios"`
	TestCount   int                          `json:"test_count"`
	Connected   *domain.ID                   `json:"connected_integration_id,omitempty"`
}

// ListTypes returns the connector catalog with connection state.
func (s *Service) ListTypes(ctx context.Context, orgID domain.ID) ([]IntegrationType, error) {
	connected := map[string]domain.ID{}
	rows, err := s.Pool.Query(ctx, `SELECT type, id FROM integrations WHERE org_id = $1`, orgID)
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
		connected[t] = id
	}
	rows.Close()
	counts := map[string]int{}
	for _, t := range s.Catalog.Tests {
		counts[t.Connector]++
	}
	var out []IntegrationType
	for _, c := range s.Catalog.Connectors {
		if c.Internal {
			continue
		}
		reg := connectors.Types[c.Type]
		it := IntegrationType{Connector: c, Credentials: reg.Credentials, LiveReady: reg.LiveReady, Scenarios: reg.Scenarios, TestCount: counts[c.Type]}
		if id, ok := connected[c.Type]; ok {
			id := id
			it.Connected = &id
		}
		out = append(out, it)
	}
	return out, nil
}

const integrationSelect = `SELECT i.id, i.type, i.name, i.mode, i.credentials_hint, i.enabled, i.status, i.last_sync_at, i.last_error, i.resource_count, i.created_at, i.sandbox_state,
  (SELECT count(*) FROM tests t WHERE t.org_id = i.org_id AND t.integration_type = i.type AND t.enabled),
  (SELECT count(*) FROM tests t WHERE t.org_id = i.org_id AND t.integration_type = i.type AND t.enabled AND t.last_status IN ('fail','error'))
FROM integrations i`

func scanIntegration(row pgx.Row) (*Integration, error) {
	var in Integration
	var hint, state []byte
	if err := row.Scan(&in.ID, &in.Type, &in.Name, &in.Mode, &hint, &in.Enabled, &in.Status, &in.LastSyncAt, &in.LastError, &in.ResourceCount, &in.CreatedAt, &state, &in.TestsTotal, &in.TestsFailing); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(hint, &in.CredentialsHint)
	in.Scenarios = []string{}
	if len(state) > 0 {
		var st map[string]any
		_ = json.Unmarshal(state, &st)
		in.Scenarios = append(in.Scenarios, connectors.ActiveScenarios(st)...)
	}
	return &in, nil
}

func (s *Service) ListIntegrations(ctx context.Context, orgID domain.ID) ([]Integration, error) {
	rows, err := s.Pool.Query(ctx, integrationSelect+` WHERE i.org_id = $1 ORDER BY i.mode = 'platform', i.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Integration{}
	for rows.Next() {
		in, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

func (s *Service) GetIntegration(ctx context.Context, orgID, id domain.ID) (*Integration, error) {
	return scanIntegration(s.Pool.QueryRow(ctx, integrationSelect+` WHERE i.org_id = $1 AND i.id = $2`, orgID, id))
}

type ConnectInput struct {
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Mode        string            `json:"mode"`
	Credentials map[string]string `json:"credentials"`
}

// Connect stores an integration (credentials encrypted), creates the tests
// for the controls in scope and enqueues the first sync.
func (s *Service) Connect(ctx context.Context, orgID domain.ID, in ConnectInput, actor domain.Actor) (*Integration, error) {
	v := domain.NewValidation()
	if def := s.Catalog.ConnectorDef(in.Type); def != nil && def.Internal {
		return nil, domain.Invalid("type", "this integration is built in and always connected")
	}
	for k, msg := range connectors.ValidateCredentials(in.Type, in.Credentials) {
		v.Add(k, msg)
	}
	if in.Mode == "" {
		in.Mode = string(connectors.ModeSandbox)
	}
	if in.Mode != string(connectors.ModeSandbox) && in.Mode != string(connectors.ModeLive) {
		v.Add("mode", "mode must be sandbox or live")
	}
	if in.Mode == string(connectors.ModeLive) && !connectors.Types[in.Type].LiveReady {
		v.Add("mode", connectors.ErrLiveUnavailable.Error())
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	def := s.Catalog.ConnectorDef(in.Type)
	if strings.TrimSpace(in.Name) == "" {
		in.Name = def.Name
	}
	var id domain.ID
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var slug string
		if err := tx.QueryRow(ctx, `SELECT slug FROM orgs WHERE id = $1`, orgID).Scan(&slug); err != nil {
			return err
		}
		id = domain.NewID()
		creds, err := s.Box.SealJSON(in.Credentials, id[:])
		if err != nil {
			return err
		}
		hint := map[string]string{}
		for _, f := range connectors.Types[in.Type].Credentials {
			val := in.Credentials[f.Key]
			if f.Secret {
				if len(val) > 4 {
					val = "••••" + val[len(val)-4:]
				} else if val != "" {
					val = "••••"
				}
			}
			hint[f.Key] = val
		}
		hintJSON, _ := json.Marshal(hint)
		var state []byte
		if in.Mode == string(connectors.ModeSandbox) {
			st, err := connectors.ApplyScenarios(in.Type, slug, nil)
			if err != nil {
				return err
			}
			state, _ = json.Marshal(st)
		}
		_, err = tx.Exec(ctx, `INSERT INTO integrations (id, org_id, type, name, mode, credentials_enc, credentials_hint, sandbox_state, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, orgID, in.Type, strings.TrimSpace(in.Name), in.Mode, creds, hintJSON, state, actor.ID)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return domain.Invalid("type", def.Name+" is already connected")
			}
			return err
		}
		if err := s.ensurePlatform(ctx, tx, orgID); err != nil {
			return err
		}
		created, err := s.ensureTests(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "integration.connected", TargetType: "integration", TargetID: id.String(), TargetLabel: in.Name,
			Metadata: map[string]any{"type": in.Type, "mode": in.Mode, "tests_created": created}}); err != nil {
			return err
		}
		return s.Jobs.EnqueueTx(ctx, tx, jobs.SyncIntegrationArgs{OrgID: orgID.String(), IntegrationID: id.String(), Trigger: "connect"})
	})
	if err != nil {
		return nil, err
	}
	return s.GetIntegration(ctx, orgID, id)
}

// Disconnect removes the integration and disables its tests. Snapshots and
// test runs are kept as historical evidence.
func (s *Service) Disconnect(ctx context.Context, orgID, id domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var typ, name string
		if err := tx.QueryRow(ctx, `SELECT type, name FROM integrations WHERE org_id = $1 AND id = $2`, orgID, id).Scan(&typ, &name); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if typ == "trustline" {
			return domain.Invalid("integration", "the Trustline platform integration is built in and cannot be disconnected")
		}
		var affected []domain.ID
		rows, err := tx.Query(ctx, `UPDATE tests SET enabled = false, last_status = 'pending', updated_at = now() WHERE org_id = $1 AND integration_type = $2 AND enabled RETURNING control_id`, orgID, typ)
		if err != nil {
			return err
		}
		for rows.Next() {
			var c domain.ID
			if err := rows.Scan(&c); err != nil {
				rows.Close()
				return err
			}
			affected = append(affected, c)
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `DELETE FROM integrations WHERE org_id = $1 AND id = $2`, orgID, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'dismissed', resolution = 'Integration disconnected', updated_at = now()
			WHERE org_id = $1 AND status = 'open' AND test_id IN (SELECT id FROM tests WHERE org_id = $1 AND integration_type = $2)`, orgID, typ); err != nil {
			return err
		}
		if _, err := s.recompute(ctx, tx, orgID, affected); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "integration.disconnected", TargetType: "integration", TargetID: id.String(), TargetLabel: name, Metadata: map[string]any{"type": typ}})
	})
}

// RequestSync enqueues an on-demand sync.
func (s *Service) RequestSync(ctx context.Context, orgID, id domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM integrations WHERE org_id = $1 AND id = $2`, orgID, id).Scan(&name); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "integration.sync_requested", TargetType: "integration", TargetID: id.String(), TargetLabel: name}); err != nil {
			return err
		}
		return s.Jobs.EnqueueTx(ctx, tx, jobs.SyncIntegrationArgs{OrgID: orgID.String(), IntegrationID: id.String(), Trigger: "manual"})
	})
}

// SyncIntegration runs a connector (or builds the platform snapshot), stores
// the snapshot, applies side effects (people sync, task reconciliation) and
// enqueues test runs. Called by the sync worker.
func (s *Service) SyncIntegration(ctx context.Context, orgID, id domain.ID, trigger string) (*domain.ID, error) {
	var typ, mode string
	var credsEnc, state []byte
	err := s.Pool.QueryRow(ctx, `UPDATE integrations SET status = 'syncing', updated_at = now() WHERE org_id = $1 AND id = $2 AND enabled
		RETURNING type, mode, credentials_enc, sandbox_state`, orgID, id).Scan(&typ, &mode, &credsEnc, &state)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil // deleted or disabled since the job was queued
		}
		return nil, err
	}
	fail := func(err error) (*domain.ID, error) {
		_, _ = s.Pool.Exec(ctx, `UPDATE integrations SET status = 'error', last_error = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, err.Error())
		return nil, err
	}
	var resources map[string][]map[string]any
	var people []personFacts
	var pols []policyFacts
	takenAt := time.Now().UTC()
	if mode == string(connectors.ModePlatform) {
		resources, people, pols, err = s.buildPlatformSnapshot(ctx, s.Pool, orgID)
		if err != nil {
			return fail(err)
		}
	} else {
		var creds map[string]string
		if err := s.Box.OpenJSON(credsEnc, id[:], &creds); err != nil {
			return fail(fmt.Errorf("credentials unreadable: %w", err))
		}
		var st map[string]any
		if len(state) > 0 {
			_ = json.Unmarshal(state, &st)
		}
		conn, err := connectors.Build(typ, connectors.Mode(mode), creds, st)
		if err != nil {
			return fail(err)
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		snap, err := conn.Sync(cctx)
		if err != nil {
			return fail(err)
		}
		resources, takenAt = snap.Resources, snap.TakenAt
	}
	data, err := json.Marshal(resources)
	if err != nil {
		return fail(err)
	}
	count := 0
	for _, r := range resources {
		count += len(r)
	}
	sum := sha256.Sum256(data)
	snapID := domain.NewID()
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO snapshots (id, org_id, integration_id, connector, taken_at, resource_count, sha256, data) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			snapID, orgID, id, typ, takenAt, count, hex.EncodeToString(sum[:]), data); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE integrations SET status = 'connected', last_sync_at = $3, last_error = '', resource_count = $4, updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, id, takenAt, count); err != nil {
			return err
		}
		switch typ {
		case "hris":
			if err := s.syncPeople(ctx, tx, orgID, resources["employee"]); err != nil {
				return fmt.Errorf("people sync: %w", err)
			}
		case "trustline":
			if err := s.reconcile(ctx, tx, orgID, people, pols); err != nil {
				return fmt.Errorf("reconcile: %w", err)
			}
		case "google_workspace", "okta":
			if err := s.discoverVendors(ctx, tx, orgID, typ, resources); err != nil {
				return fmt.Errorf("vendor discovery: %w", err)
			}
		}
		return s.Jobs.EnqueueTx(ctx, tx, jobs.RunTestsArgs{OrgID: orgID.String(), SnapshotID: snapID.String(), Trigger: trigger})
	})
	if err != nil {
		return fail(err)
	}
	return &snapID, nil
}

type SnapshotView struct {
	ID            domain.ID                   `json:"id"`
	IntegrationID *domain.ID                  `json:"integration_id,omitempty"`
	Connector     string                      `json:"connector"`
	TakenAt       time.Time                   `json:"taken_at"`
	ResourceCount int                         `json:"resource_count"`
	SHA256        string                      `json:"sha256"`
	Resources     map[string][]map[string]any `json:"resources"`
}

func (s *Service) LatestSnapshot(ctx context.Context, orgID, integrationID domain.ID) (*SnapshotView, error) {
	return s.snapshot(ctx, `WHERE org_id = $1 AND integration_id = $2 ORDER BY taken_at DESC LIMIT 1`, orgID, integrationID)
}

func (s *Service) GetSnapshot(ctx context.Context, orgID, snapshotID domain.ID) (*SnapshotView, error) {
	return s.snapshot(ctx, `WHERE org_id = $1 AND id = $2`, orgID, snapshotID)
}

func (s *Service) snapshot(ctx context.Context, where string, args ...any) (*SnapshotView, error) {
	var v SnapshotView
	var data []byte
	err := s.Pool.QueryRow(ctx, `SELECT id, integration_id, connector, taken_at, resource_count, sha256, data FROM snapshots `+where, args...).
		Scan(&v.ID, &v.IntegrationID, &v.Connector, &v.TakenAt, &v.ResourceCount, &v.SHA256, &data)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &v.Resources); err != nil {
		return nil, err
	}
	return &v, nil
}

// SetScenario toggles a sandbox scenario. State is rebuilt from the baseline
// plus the active scenarios; the next sync (scheduled or manual) picks it up.
func (s *Service) SetScenario(ctx context.Context, orgID, id domain.ID, key string, active bool, actor domain.Actor) (*Integration, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var typ, mode, name, slug string
		var state []byte
		err := tx.QueryRow(ctx, `SELECT i.type, i.mode, i.name, i.sandbox_state, o.slug FROM integrations i JOIN orgs o ON o.id = i.org_id
			WHERE i.org_id = $1 AND i.id = $2 FOR UPDATE OF i`, orgID, id).Scan(&typ, &mode, &name, &state, &slug)
		if err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if mode != string(connectors.ModeSandbox) {
			return domain.Invalid("mode", "scenarios apply to sandbox integrations only")
		}
		var st map[string]any
		_ = json.Unmarshal(state, &st)
		set := map[string]bool{}
		for _, k := range connectors.ActiveScenarios(st) {
			set[k] = true
		}
		if active {
			set[key] = true
		} else {
			delete(set, key)
		}
		var keys []string
		for k := range set {
			keys = append(keys, k)
		}
		next, err := connectors.ApplyScenarios(typ, slug, keys)
		if err != nil {
			return domain.Invalid("key", err.Error())
		}
		raw, _ := json.Marshal(next)
		if _, err := tx.Exec(ctx, `UPDATE integrations SET sandbox_state = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, raw); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "integration.sandbox_scenario", TargetType: "integration", TargetID: id.String(), TargetLabel: name,
			Metadata: map[string]any{"scenario": key, "active": active}})
	})
	if err != nil {
		return nil, err
	}
	return s.GetIntegration(ctx, orgID, id)
}

// PutSandboxState replaces the sandbox resources wholesale (advanced editing).
func (s *Service) PutSandboxState(ctx context.Context, orgID, id domain.ID, resources map[string][]map[string]any, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var typ, mode, name string
		if err := tx.QueryRow(ctx, `SELECT type, mode, name FROM integrations WHERE org_id = $1 AND id = $2 FOR UPDATE`, orgID, id).Scan(&typ, &mode, &name); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if mode != string(connectors.ModeSandbox) {
			return domain.Invalid("mode", "only sandbox integrations can be edited")
		}
		def := s.Catalog.ConnectorDef(typ)
		known := map[string]bool{}
		for _, r := range def.Resources {
			known[r.Type] = true
		}
		for rt, list := range resources {
			if !known[rt] {
				return domain.Invalid("resources", fmt.Sprintf("unknown resource type %q for %s", rt, typ))
			}
			for i, r := range list {
				if _, ok := r["id"]; !ok {
					return domain.Invalid("resources", fmt.Sprintf("%s[%d] needs an id", rt, i))
				}
			}
		}
		raw, _ := json.Marshal(map[string]any{"resources": resources, "scenarios": []string{"custom"}})
		if _, err := tx.Exec(ctx, `UPDATE integrations SET sandbox_state = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, raw); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "integration.sandbox_edited", TargetType: "integration", TargetID: id.String(), TargetLabel: name})
	})
}

// IntegrationsForTick lists enabled integrations of all orgs (scheduler).
func (s *Service) IntegrationsForTick(ctx context.Context) ([][2]domain.ID, error) {
	rows, err := s.Pool.Query(ctx, `SELECT org_id, id FROM integrations WHERE enabled`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]domain.ID
	for rows.Next() {
		var o, i domain.ID
		if err := rows.Scan(&o, &i); err != nil {
			return nil, err
		}
		out = append(out, [2]domain.ID{o, i})
	}
	return out, rows.Err()
}

var errNoSnapshot = errors.New("no snapshot yet")
