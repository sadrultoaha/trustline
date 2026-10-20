-- +goose Up

-- Policies -----------------------------------------------------------------
CREATE TABLE policies (
  id                   uuid PRIMARY KEY,
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  template_id          text,
  slug                 text NOT NULL,
  title                text NOT NULL,
  category             text NOT NULL DEFAULT 'Governance',
  summary              text NOT NULL DEFAULT '',
  control_codes        text[] NOT NULL DEFAULT '{}',
  owner_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  current_version_id   uuid,
  review_cycle_days    int NOT NULL DEFAULT 365,
  next_review_at       timestamptz,
  last_reviewed_at     timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, slug)
);

CREATE TABLE policy_versions (
  id                      uuid PRIMARY KEY,
  org_id                  uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  policy_id               uuid NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
  version                 int NOT NULL,
  status                  text NOT NULL CHECK (status IN ('draft','in_review','approved','rejected','published','superseded')),
  content_html            text NOT NULL DEFAULT '',
  change_summary          text NOT NULL DEFAULT '',
  created_by              uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now(),
  submitted_at            timestamptz,
  approver_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  decided_by              uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at              timestamptz,
  review_comment          text NOT NULL DEFAULT '',
  published_at            timestamptz,
  published_by            uuid REFERENCES users(id) ON DELETE SET NULL,
  ai_generated            boolean NOT NULL DEFAULT false,
  consistency             jsonb,
  consistency_checked_at  timestamptz,
  UNIQUE (policy_id, version)
);
CREATE INDEX policy_versions_org_idx ON policy_versions (org_id, policy_id, version DESC);
-- At most one editable draft/in-review version per policy.
CREATE UNIQUE INDEX policy_versions_open_idx ON policy_versions (policy_id) WHERE status IN ('draft','in_review','approved','rejected');
ALTER TABLE policies ADD CONSTRAINT policies_current_version_fk FOREIGN KEY (current_version_id) REFERENCES policy_versions(id) ON DELETE SET NULL;

CREATE TABLE policy_acceptances (
  org_id             uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  policy_version_id  uuid NOT NULL REFERENCES policy_versions(id) ON DELETE CASCADE,
  user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  accepted_at        timestamptz NOT NULL DEFAULT now(),
  ip                 text NOT NULL DEFAULT '',
  PRIMARY KEY (policy_version_id, user_id)
);
CREATE INDEX policy_acceptances_org_idx ON policy_acceptances (org_id, user_id);

-- People (HRIS-sourced workforce) -------------------------------------------
CREATE TABLE people (
  id                uuid PRIMARY KEY,
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  email             text NOT NULL,
  name              text NOT NULL,
  department        text NOT NULL DEFAULT '',
  title             text NOT NULL DEFAULT '',
  manager_email     text NOT NULL DEFAULT '',
  status            text NOT NULL CHECK (status IN ('active','on_leave','terminated')),
  employment_type   text NOT NULL DEFAULT 'full_time',
  hire_date         date,
  termination_date  date,
  location          text NOT NULL DEFAULT '',
  background_check  text NOT NULL DEFAULT 'pending',
  background_attested_by uuid REFERENCES users(id) ON DELETE SET NULL,
  source            text NOT NULL DEFAULT 'hris' CHECK (source IN ('hris','manual')),
  external_id       text NOT NULL DEFAULT '',
  user_id           uuid REFERENCES users(id) ON DELETE SET NULL,
  terminated_at     timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX people_org_email_idx ON people (org_id, lower(email));

-- Device agent reports -------------------------------------------------------
CREATE TABLE device_tokens (
  org_id        uuid PRIMARY KEY REFERENCES orgs(id) ON DELETE RESTRICT,
  token_hash    bytea NOT NULL UNIQUE,
  token_hint    text NOT NULL,
  created_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_used_at  timestamptz
);

CREATE TABLE devices (
  id                   uuid PRIMARY KEY,
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  serial               text NOT NULL,
  hostname             text NOT NULL DEFAULT '',
  owner_email          text NOT NULL DEFAULT '',
  person_id            uuid REFERENCES people(id) ON DELETE SET NULL,
  platform             text NOT NULL DEFAULT '',
  os_version           text NOT NULL DEFAULT '',
  disk_encrypted       boolean NOT NULL,
  screen_lock_minutes  int,
  antivirus_running    boolean NOT NULL DEFAULT false,
  antivirus_name       text NOT NULL DEFAULT '',
  agent_version        text NOT NULL DEFAULT '',
  last_seen_at         timestamptz NOT NULL DEFAULT now(),
  created_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, serial)
);

-- Training -------------------------------------------------------------------
CREATE TABLE training_completions (
  id            uuid PRIMARY KEY,
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  course_id     text NOT NULL,
  score         int NOT NULL,
  passed        boolean NOT NULL,
  answers       jsonb NOT NULL DEFAULT '{}'::jsonb,
  completed_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX training_completions_user_idx ON training_completions (org_id, user_id, completed_at DESC);

-- Access reviews -------------------------------------------------------------
CREATE TABLE access_review_campaigns (
  id            uuid PRIMARY KEY,
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  name          text NOT NULL,
  status        text NOT NULL DEFAULT 'open' CHECK (status IN ('open','completed','cancelled')),
  systems       text[] NOT NULL,
  due_at        timestamptz NOT NULL,
  created_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  completed_at  timestamptz,
  evidence_id   uuid REFERENCES evidence(id) ON DELETE SET NULL
);
CREATE INDEX access_review_campaigns_org_idx ON access_review_campaigns (org_id, created_at DESC);

CREATE TABLE access_review_items (
  id                      uuid PRIMARY KEY,
  org_id                  uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  campaign_id             uuid NOT NULL REFERENCES access_review_campaigns(id) ON DELETE CASCADE,
  integration_type        text NOT NULL,
  account_id              text NOT NULL,
  account_label           text NOT NULL,
  email                   text NOT NULL DEFAULT '',
  person_id               uuid REFERENCES people(id) ON DELETE SET NULL,
  person_status           text NOT NULL DEFAULT '',
  entitlements            text[] NOT NULL DEFAULT '{}',
  is_admin                boolean NOT NULL DEFAULT false,
  reviewer_membership_id  uuid REFERENCES org_memberships(id) ON DELETE SET NULL,
  decision                text NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending','approve','revoke')),
  comment                 text NOT NULL DEFAULT '',
  decided_by              uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at              timestamptz,
  UNIQUE (campaign_id, integration_type, account_id)
);
CREATE INDEX access_review_items_reviewer_idx ON access_review_items (org_id, reviewer_membership_id, decision);

-- Tasks gain a person link and structured metadata (onboarding/offboarding,
-- policy acceptance, access revocation, policy review).
ALTER TABLE tasks ADD COLUMN person_id uuid REFERENCES people(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN item_key text NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN meta jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX tasks_person_idx ON tasks (org_id, person_id);
CREATE INDEX tasks_assignee_idx ON tasks (org_id, assignee_membership_id, status);

-- Integrations: the built-in platform connector has no external credentials.
ALTER TABLE integrations DROP CONSTRAINT integrations_mode_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_mode_check CHECK (mode IN ('sandbox','live','platform'));

-- +goose Down
ALTER TABLE integrations DROP CONSTRAINT integrations_mode_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_mode_check CHECK (mode IN ('sandbox','live'));
DROP INDEX tasks_assignee_idx;
DROP INDEX tasks_person_idx;
ALTER TABLE tasks DROP COLUMN meta;
ALTER TABLE tasks DROP COLUMN item_key;
ALTER TABLE tasks DROP COLUMN person_id;
DROP TABLE access_review_items;
DROP TABLE access_review_campaigns;
DROP TABLE training_completions;
DROP TABLE devices;
DROP TABLE device_tokens;
DROP TABLE people;
DROP TABLE policy_acceptances;
ALTER TABLE policies DROP CONSTRAINT policies_current_version_fk;
DROP TABLE policy_versions;
DROP TABLE policies;
