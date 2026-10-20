-- +goose Up

CREATE TABLE orgs (
  id          uuid PRIMARY KEY,
  name        text NOT NULL,
  slug        text NOT NULL UNIQUE,
  settings    jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id             uuid PRIMARY KEY,
  email          text NOT NULL,
  name           text NOT NULL,
  password_hash  text,
  google_sub     text UNIQUE,
  avatar_url     text NOT NULL DEFAULT '',
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  last_login_at  timestamptz
);
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(email));

CREATE TABLE org_memberships (
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role        text NOT NULL CHECK (role IN ('admin','editor','auditor','employee')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, user_id)
);
CREATE INDEX org_memberships_user_idx ON org_memberships (user_id);

CREATE TABLE sessions (
  id            uuid PRIMARY KEY,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id        uuid REFERENCES orgs(id) ON DELETE SET NULL,
  token_hash    bytea NOT NULL UNIQUE,
  ip            text NOT NULL DEFAULT '',
  user_agent    text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  expires_at    timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE org_invites (
  id           uuid PRIMARY KEY,
  org_id       uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  email        text NOT NULL,
  role         text NOT NULL CHECK (role IN ('admin','editor','auditor','employee')),
  token_hash   bytea NOT NULL UNIQUE,
  invited_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  expires_at   timestamptz NOT NULL,
  accepted_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX org_invites_org_idx ON org_invites (org_id, created_at DESC);
CREATE UNIQUE INDEX org_invites_pending_idx ON org_invites (org_id, lower(email)) WHERE accepted_at IS NULL;

CREATE TABLE password_resets (
  id          uuid PRIMARY KEY,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE activity_log (
  id            uuid PRIMARY KEY,
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  actor_type    text NOT NULL CHECK (actor_type IN ('user','system','api_key','agent')),
  actor_id      uuid,
  actor_label   text NOT NULL,
  action        text NOT NULL,
  target_type   text NOT NULL,
  target_id     text NOT NULL DEFAULT '',
  target_label  text NOT NULL DEFAULT '',
  metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip            text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_log_org_idx ON activity_log (org_id, created_at DESC, id DESC);
CREATE INDEX activity_log_target_idx ON activity_log (org_id, target_type, target_id);

-- +goose StatementBegin
CREATE FUNCTION activity_log_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'activity_log is append-only';
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER activity_log_append_only
  BEFORE UPDATE OR DELETE ON activity_log
  FOR EACH ROW EXECUTE FUNCTION activity_log_immutable();

-- +goose Down
DROP TABLE activity_log;
DROP FUNCTION activity_log_immutable;
DROP TABLE password_resets;
DROP TABLE org_invites;
DROP TABLE sessions;
DROP TABLE org_memberships;
DROP TABLE users;
DROP TABLE orgs;
