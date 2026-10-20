-- +goose Up

-- Global, read-only catalog (synced from embedded YAML on boot). These are the
-- only tables without org_id: they hold reference content shared by all
-- tenants and are never written by request handlers (see DECISIONS.md).
CREATE TABLE catalog_frameworks (
  id           text PRIMARY KEY,
  name         text NOT NULL,
  version      text NOT NULL DEFAULT '',
  publisher    text NOT NULL DEFAULT '',
  description  text NOT NULL DEFAULT '',
  sections     jsonb NOT NULL DEFAULT '[]'::jsonb,
  position     int NOT NULL DEFAULT 0
);

CREATE TABLE catalog_controls (
  code               text PRIMARY KEY,
  name               text NOT NULL,
  domain             text NOT NULL,
  description        text NOT NULL,
  evidence_mode      text NOT NULL CHECK (evidence_mode IN ('automated','manual','hybrid')),
  evidence_hint      text NOT NULL DEFAULT '',
  keywords           text[] NOT NULL DEFAULT '{}',
  evidence_ttl_days  int NOT NULL DEFAULT 365
);

CREATE TABLE catalog_requirements (
  id            uuid PRIMARY KEY,
  framework_id  text NOT NULL REFERENCES catalog_frameworks(id) ON DELETE CASCADE,
  code          text NOT NULL,
  section       text NOT NULL DEFAULT '',
  title         text NOT NULL,
  description   text NOT NULL DEFAULT '',
  position      int NOT NULL DEFAULT 0,
  UNIQUE (framework_id, code)
);

-- The crosswalk: requirement → control.
CREATE TABLE catalog_requirement_controls (
  requirement_id  uuid NOT NULL REFERENCES catalog_requirements(id) ON DELETE CASCADE,
  control_code    text NOT NULL REFERENCES catalog_controls(code) ON DELETE CASCADE,
  PRIMARY KEY (requirement_id, control_code)
);
CREATE INDEX catalog_requirement_controls_control_idx ON catalog_requirement_controls (control_code);

CREATE TABLE catalog_tests (
  id            text PRIMARY KEY,
  name          text NOT NULL,
  description   text NOT NULL DEFAULT '',
  connector     text NOT NULL,
  resource      text NOT NULL,
  control_code  text NOT NULL REFERENCES catalog_controls(code),
  severity      text NOT NULL,
  rule          jsonb NOT NULL,
  remediation   jsonb NOT NULL
);

-- Org-scoped ---------------------------------------------------------------

CREATE TABLE org_frameworks (
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  framework_id  text NOT NULL REFERENCES catalog_frameworks(id),
  adopted_at    timestamptz NOT NULL DEFAULT now(),
  adopted_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  PRIMARY KEY (org_id, framework_id)
);

CREATE TABLE controls (
  id                   uuid PRIMARY KEY,
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  code                 text NOT NULL,
  name                 text NOT NULL,
  domain               text NOT NULL,
  description          text NOT NULL,
  evidence_mode        text NOT NULL,
  custom               boolean NOT NULL DEFAULT false,
  owner_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  status               text NOT NULL DEFAULT 'needs_evidence' CHECK (status IN ('passing','failing','needs_evidence')),
  status_reason        text NOT NULL DEFAULT '',
  status_changed_at    timestamptz NOT NULL DEFAULT now(),
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, code)
);
CREATE INDEX controls_org_status_idx ON controls (org_id, status);

CREATE TABLE integrations (
  id                uuid PRIMARY KEY,
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  type              text NOT NULL,
  name              text NOT NULL,
  mode              text NOT NULL CHECK (mode IN ('sandbox','live')),
  credentials_enc   bytea NOT NULL,
  credentials_hint  jsonb NOT NULL DEFAULT '{}'::jsonb,
  sandbox_state     jsonb,
  enabled           boolean NOT NULL DEFAULT true,
  status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','syncing','connected','error')),
  last_sync_at      timestamptz,
  last_error        text NOT NULL DEFAULT '',
  resource_count    int NOT NULL DEFAULT 0,
  created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, type)
);

CREATE TABLE snapshots (
  id              uuid PRIMARY KEY,
  org_id          uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  integration_id  uuid REFERENCES integrations(id) ON DELETE SET NULL, -- kept after disconnect: snapshots are audit evidence
  connector       text NOT NULL,
  taken_at        timestamptz NOT NULL,
  resource_count  int NOT NULL,
  sha256          text NOT NULL,
  data            jsonb NOT NULL
);
CREATE INDEX snapshots_integration_idx ON snapshots (org_id, integration_id, taken_at DESC);

CREATE TABLE tests (
  id                uuid PRIMARY KEY,
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  catalog_id        text REFERENCES catalog_tests(id),
  control_id        uuid NOT NULL REFERENCES controls(id) ON DELETE CASCADE,
  integration_type  text NOT NULL,
  name              text NOT NULL,
  description       text NOT NULL DEFAULT '',
  severity          text NOT NULL CHECK (severity IN ('low','medium','high','critical')),
  rule              jsonb NOT NULL,
  remediation       jsonb NOT NULL DEFAULT '{}'::jsonb,
  custom            boolean NOT NULL DEFAULT false,
  enabled           boolean NOT NULL DEFAULT true,
  last_status       text NOT NULL DEFAULT 'pending' CHECK (last_status IN ('pending','pass','fail','error')),
  last_run_at       timestamptz,
  last_run_id       uuid,
  last_summary      text NOT NULL DEFAULT '',
  created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, catalog_id)
);
CREATE INDEX tests_org_integration_idx ON tests (org_id, integration_type);
CREATE INDEX tests_control_idx ON tests (org_id, control_id);

CREATE TABLE test_runs (
  id              uuid PRIMARY KEY,
  org_id          uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  test_id         uuid NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
  snapshot_id     uuid REFERENCES snapshots(id) ON DELETE SET NULL,
  status          text NOT NULL CHECK (status IN ('pass','fail','error')),
  evaluated       int NOT NULL DEFAULT 0,
  failing         jsonb NOT NULL DEFAULT '[]'::jsonb,
  summary         text NOT NULL DEFAULT '',
  error           text NOT NULL DEFAULT '',
  trigger         text NOT NULL DEFAULT 'scheduled',
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX test_runs_test_idx ON test_runs (org_id, test_id, created_at DESC);

CREATE TABLE evidence (
  id                uuid PRIMARY KEY,
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  control_id        uuid NOT NULL REFERENCES controls(id) ON DELETE CASCADE,
  title             text NOT NULL,
  description       text NOT NULL DEFAULT '',
  blob_key          text NOT NULL,
  filename          text NOT NULL,
  content_type      text NOT NULL,
  size_bytes        bigint NOT NULL,
  sha256            text NOT NULL,
  collected_at      timestamptz NOT NULL DEFAULT now(),
  expires_at        timestamptz,
  uploaded_by       uuid REFERENCES users(id) ON DELETE SET NULL,
  ai_status         text NOT NULL DEFAULT 'pending' CHECK (ai_status IN ('pending','done','skipped','error')),
  ai_verdict        text CHECK (ai_verdict IN ('pass','fail')),
  ai_confidence     double precision,
  ai_reasoning      text NOT NULL DEFAULT '',
  ai_missing        jsonb NOT NULL DEFAULT '[]'::jsonb,
  ai_model          text NOT NULL DEFAULT '',
  override_verdict  text CHECK (override_verdict IN ('pass','fail')),
  override_reason   text NOT NULL DEFAULT '',
  override_by       uuid REFERENCES users(id) ON DELETE SET NULL,
  override_at       timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX evidence_control_idx ON evidence (org_id, control_id, created_at DESC);

CREATE TABLE tasks (
  id                      uuid PRIMARY KEY,
  org_id                  uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  kind                    text NOT NULL,
  title                   text NOT NULL,
  description             text NOT NULL DEFAULT '',
  status                  text NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','dismissed')),
  priority                text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high','critical')),
  control_id              uuid REFERENCES controls(id) ON DELETE SET NULL,
  test_id                 uuid REFERENCES tests(id) ON DELETE SET NULL,
  assignee_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  source_key              text,
  steps                   jsonb NOT NULL DEFAULT '[]'::jsonb,
  terraform               text NOT NULL DEFAULT '',
  ai_status               text NOT NULL DEFAULT 'none' CHECK (ai_status IN ('none','pending','done','skipped','error')),
  ai_model                text NOT NULL DEFAULT '',
  ai_notes                text NOT NULL DEFAULT '',
  resolution              text NOT NULL DEFAULT '',
  due_at                  timestamptz,
  completed_at            timestamptz,
  completed_by            uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_org_status_idx ON tasks (org_id, status, created_at DESC);
CREATE UNIQUE INDEX tasks_open_source_idx ON tasks (org_id, source_key) WHERE status = 'open' AND source_key IS NOT NULL;

CREATE TABLE notifications (
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind        text NOT NULL,
  title       text NOT NULL,
  body        text NOT NULL DEFAULT '',
  link        text NOT NULL DEFAULT '',
  read_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (org_id, user_id, created_at DESC);

CREATE TABLE ai_calls (
  id             uuid PRIMARY KEY,
  org_id         uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  task           text NOT NULL,
  prompt_id      text NOT NULL,
  provider       text NOT NULL,
  model          text NOT NULL,
  input_tokens   bigint NOT NULL DEFAULT 0,
  output_tokens  bigint NOT NULL DEFAULT 0,
  cost_usd       numeric(12,6) NOT NULL DEFAULT 0,
  latency_ms     bigint NOT NULL DEFAULT 0,
  status         text NOT NULL,
  error          text NOT NULL DEFAULT '',
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_calls_org_idx ON ai_calls (org_id, created_at DESC);

-- +goose Down
DROP TABLE ai_calls;
DROP TABLE notifications;
DROP TABLE tasks;
DROP TABLE evidence;
DROP TABLE test_runs;
DROP TABLE tests;
DROP TABLE snapshots;
DROP TABLE integrations;
DROP TABLE controls;
DROP TABLE org_frameworks;
DROP TABLE catalog_tests;
DROP TABLE catalog_requirement_controls;
DROP TABLE catalog_requirements;
DROP TABLE catalog_controls;
DROP TABLE catalog_frameworks;
