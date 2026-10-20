-- +goose Up

CREATE TABLE risks (
  id                   uuid PRIMARY KEY,
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  title                text NOT NULL,
  description          text NOT NULL DEFAULT '',
  category             text NOT NULL DEFAULT 'Security',
  likelihood           int NOT NULL CHECK (likelihood BETWEEN 1 AND 5),
  impact               int NOT NULL CHECK (impact BETWEEN 1 AND 5),
  residual_likelihood  int CHECK (residual_likelihood BETWEEN 1 AND 5),
  residual_impact      int CHECK (residual_impact BETWEEN 1 AND 5),
  treatment            text NOT NULL DEFAULT 'mitigate' CHECK (treatment IN ('mitigate','accept','transfer','avoid')),
  treatment_plan       text NOT NULL DEFAULT '',
  status               text NOT NULL DEFAULT 'open' CHECK (status IN ('suggested','open','in_treatment','accepted','closed','dismissed')),
  owner_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  source               text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','ai','test')),
  source_key           text,
  test_id              uuid REFERENCES tests(id) ON DELETE SET NULL,
  ai_rationale         text NOT NULL DEFAULT '',
  review_cycle_days    int NOT NULL DEFAULT 90,
  next_review_at       timestamptz,
  last_reviewed_at     timestamptz,
  created_by           uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX risks_org_idx ON risks (org_id, status);
CREATE UNIQUE INDEX risks_source_idx ON risks (org_id, source_key) WHERE source_key IS NOT NULL AND status IN ('suggested','open','in_treatment','accepted');

CREATE TABLE risk_controls (
  org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  risk_id     uuid NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
  control_id  uuid NOT NULL REFERENCES controls(id) ON DELETE CASCADE,
  PRIMARY KEY (risk_id, control_id)
);
CREATE INDEX risk_controls_control_idx ON risk_controls (org_id, control_id);

CREATE TABLE risk_assessments (
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  kind        text NOT NULL CHECK (kind IN ('ai','manual')),
  summary     text NOT NULL DEFAULT '',
  suggested   int NOT NULL DEFAULT 0,
  model       text NOT NULL DEFAULT '',
  created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX risk_assessments_org_idx ON risk_assessments (org_id, created_at DESC);

CREATE TABLE vendors (
  id                   uuid PRIMARY KEY,
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  name                 text NOT NULL,
  domain               text NOT NULL DEFAULT '',
  description          text NOT NULL DEFAULT '',
  category             text NOT NULL DEFAULT '',
  status               text NOT NULL DEFAULT 'active' CHECK (status IN ('discovered','active','offboarded','dismissed')),
  tier                 text NOT NULL DEFAULT 'unassessed' CHECK (tier IN ('critical','high','medium','low','unassessed')),
  data_access          text NOT NULL DEFAULT 'none' CHECK (data_access IN ('none','internal','confidential','restricted')),
  handles_pii          boolean NOT NULL DEFAULT false,
  owner_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  source               text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','google_workspace','okta')),
  discovery_ref        text NOT NULL DEFAULT '',
  discovery_users      int NOT NULL DEFAULT 0,
  discovery_note       text NOT NULL DEFAULT '',
  recommended_tier     text NOT NULL DEFAULT '',
  risk_summary         text NOT NULL DEFAULT '',
  review_cycle_days    int NOT NULL DEFAULT 365,
  next_review_at       timestamptz,
  last_reviewed_at     timestamptz,
  discovered_at        timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX vendors_org_name_idx ON vendors (org_id, lower(name));

CREATE TABLE vendor_documents (
  id            uuid PRIMARY KEY,
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  vendor_id     uuid NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  kind          text NOT NULL CHECK (kind IN ('soc2_type2','soc2_type1','iso27001','pentest','dpa','baa','insurance','other')),
  title         text NOT NULL,
  blob_key      text NOT NULL,
  filename      text NOT NULL,
  content_type  text NOT NULL,
  size_bytes    bigint NOT NULL,
  sha256        text NOT NULL,
  expires_at    timestamptz,
  uploaded_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX vendor_documents_vendor_idx ON vendor_documents (org_id, vendor_id);

CREATE TABLE vendor_questionnaires (
  id               uuid PRIMARY KEY,
  org_id           uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  vendor_id        uuid NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  template_id      text NOT NULL,
  token_hash       bytea NOT NULL UNIQUE,
  recipient_email  text NOT NULL,
  status           text NOT NULL DEFAULT 'sent' CHECK (status IN ('sent','in_progress','submitted','expired')),
  answers          jsonb NOT NULL DEFAULT '{}'::jsonb,
  sent_by          uuid REFERENCES users(id) ON DELETE SET NULL,
  sent_at          timestamptz NOT NULL DEFAULT now(),
  submitted_at     timestamptz,
  expires_at       timestamptz NOT NULL
);
CREATE INDEX vendor_questionnaires_vendor_idx ON vendor_questionnaires (org_id, vendor_id);

CREATE TABLE vendor_assessments (
  id                uuid PRIMARY KEY,
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  vendor_id         uuid NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  document_id       uuid REFERENCES vendor_documents(id) ON DELETE SET NULL,
  questionnaire_id  uuid REFERENCES vendor_questionnaires(id) ON DELETE SET NULL,
  status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','done','error','skipped')),
  result            jsonb,
  model             text NOT NULL DEFAULT '',
  error             text NOT NULL DEFAULT '',
  requested_by      uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  completed_at      timestamptz
);
CREATE INDEX vendor_assessments_vendor_idx ON vendor_assessments (org_id, vendor_id, created_at DESC);

-- +goose Down
DROP TABLE vendor_assessments;
DROP TABLE vendor_questionnaires;
DROP TABLE vendor_documents;
DROP TABLE vendors;
DROP TABLE risk_assessments;
DROP TABLE risk_controls;
DROP TABLE risks;
