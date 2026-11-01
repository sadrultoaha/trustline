-- +goose Up

-- Frameworks a vendor supports for us. framework_id is a catalog id or a
-- custom framework id (like org_frameworks, no foreign key); deleting a custom
-- framework removes its rows here.
CREATE TABLE vendor_frameworks (
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  vendor_id     uuid NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  framework_id  text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (vendor_id, framework_id)
);
CREATE INDEX vendor_frameworks_org_framework_idx ON vendor_frameworks (org_id, framework_id);

-- Questionnaires attached to a vendor: catalog template ids, or
-- 'framework:<custom framework id>' for a questionnaire generated from a custom
-- framework's requirements. A vendor with no rows uses the suggested set for
-- its frameworks.
CREATE TABLE vendor_questionnaire_templates (
  org_id       uuid NOT NULL REFERENCES orgs(id) ON DELETE RESTRICT,
  vendor_id    uuid NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
  template_id  text NOT NULL,
  position     int NOT NULL DEFAULT 0,
  PRIMARY KEY (vendor_id, template_id)
);
CREATE INDEX vendor_questionnaire_templates_org_idx ON vendor_questionnaire_templates (org_id, template_id);

-- The questions as sent, so answers stay tied to what the vendor saw. NULL for
-- questionnaires sent before this migration (template_id names a catalog template).
ALTER TABLE vendor_questionnaires ADD COLUMN questions jsonb;

-- +goose Down
ALTER TABLE vendor_questionnaires DROP COLUMN questions;
DROP TABLE vendor_questionnaire_templates;
DROP TABLE vendor_frameworks;
