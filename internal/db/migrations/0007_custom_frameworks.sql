-- +goose Up

-- Frameworks an org writes itself (internal policies, customer contracts,
-- regional rules). Same shape as the catalog, but tenant data: requirements
-- map onto the shared control library, so readiness and the crosswalk work
-- exactly as for built-in frameworks.
CREATE TABLE custom_frameworks (
  id           text PRIMARY KEY, -- 'custom-<uuid>', never collides with catalog ids
  org_id       uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  name         text NOT NULL,
  version      text NOT NULL DEFAULT '',
  publisher    text NOT NULL DEFAULT '',
  description  text NOT NULL DEFAULT '',
  created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX custom_frameworks_org_name_idx ON custom_frameworks (org_id, lower(name));

CREATE TABLE custom_requirements (
  id            uuid PRIMARY KEY,
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  framework_id  text NOT NULL REFERENCES custom_frameworks(id) ON DELETE CASCADE,
  code          text NOT NULL,
  section       text NOT NULL DEFAULT '',
  title         text NOT NULL,
  description   text NOT NULL DEFAULT '',
  position      int NOT NULL DEFAULT 0,
  UNIQUE (framework_id, code)
);

CREATE TABLE custom_requirement_controls (
  org_id          uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  requirement_id  uuid NOT NULL REFERENCES custom_requirements(id) ON DELETE CASCADE,
  control_code    text NOT NULL REFERENCES catalog_controls(code) ON DELETE CASCADE,
  PRIMARY KEY (requirement_id, control_code)
);
CREATE INDEX custom_requirement_controls_control_idx ON custom_requirement_controls (control_code);

-- Adoption now points at either kind of framework. Catalog sync already keeps
-- adopted catalog frameworks; custom ones are removed from scope on delete.
ALTER TABLE org_frameworks DROP CONSTRAINT org_frameworks_framework_id_fkey;

-- Catalog and custom content side by side. org_id is NULL for catalog rows;
-- every query must filter on (org_id IS NULL OR org_id = <tenant>).
CREATE VIEW framework_defs AS
  SELECT id, NULL::uuid AS org_id, name, version, publisher, description, position, false AS custom FROM catalog_frameworks
  UNION ALL
  SELECT id, org_id, name, version, publisher, description, 100000, true FROM custom_frameworks;

CREATE VIEW requirement_defs AS
  SELECT id, NULL::uuid AS org_id, framework_id, code, section, title, description, position FROM catalog_requirements
  UNION ALL
  SELECT id, org_id, framework_id, code, section, title, description, position FROM custom_requirements;

CREATE VIEW requirement_control_defs AS
  SELECT requirement_id, NULL::uuid AS org_id, control_code FROM catalog_requirement_controls
  UNION ALL
  SELECT requirement_id, org_id, control_code FROM custom_requirement_controls;

-- +goose Down
DROP VIEW requirement_control_defs;
DROP VIEW requirement_defs;
DROP VIEW framework_defs;
DELETE FROM org_frameworks WHERE framework_id NOT IN (SELECT id FROM catalog_frameworks);
ALTER TABLE org_frameworks ADD CONSTRAINT org_frameworks_framework_id_fkey FOREIGN KEY (framework_id) REFERENCES catalog_frameworks(id);
DROP TABLE custom_requirement_controls;
DROP TABLE custom_requirements;
DROP TABLE custom_frameworks;
