package catalog

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var reqNamespace = uuid.MustParse("6f1b8c1e-3d6a-4c41-9b8e-2a3b7d1f0c55")

// RequirementID is deterministic so requirement ids are stable across syncs.
func RequirementID(frameworkID, code string) uuid.UUID {
	return uuid.NewSHA1(reqNamespace, []byte(frameworkID+"\x00"+code))
}

// Sync upserts the embedded catalog into the database in one transaction and
// removes rows that no longer exist in the catalog.
func Sync(ctx context.Context, pool *pgxpool.Pool) error {
	c, err := Load()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialise concurrent syncs (api + worker boot together).
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7421337002)`); err != nil {
		return err
	}
	b := &pgx.Batch{}
	codes := []string{}
	for _, ctl := range c.Controls {
		codes = append(codes, ctl.Code)
		kw := ctl.Keywords
		if kw == nil {
			kw = []string{}
		}
		b.Queue(`INSERT INTO catalog_controls (code, name, domain, description, evidence_mode, evidence_hint, keywords, evidence_ttl_days)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, domain=EXCLUDED.domain, description=EXCLUDED.description,
			  evidence_mode=EXCLUDED.evidence_mode, evidence_hint=EXCLUDED.evidence_hint, keywords=EXCLUDED.keywords, evidence_ttl_days=EXCLUDED.evidence_ttl_days`,
			ctl.Code, ctl.Name, ctl.Domain, ctl.Description, ctl.Evidence, ctl.EvidenceHint, kw, ctl.EvidenceTTLDays)
	}
	fwIDs := []string{}
	reqIDs := []uuid.UUID{}
	for i, fw := range c.Frameworks {
		fwIDs = append(fwIDs, fw.ID)
		sections, _ := json.Marshal(fw.Sections)
		if fw.Sections == nil {
			sections = []byte("[]")
		}
		b.Queue(`INSERT INTO catalog_frameworks (id, name, version, publisher, description, sections, position) VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, version=EXCLUDED.version, publisher=EXCLUDED.publisher, description=EXCLUDED.description, sections=EXCLUDED.sections, position=EXCLUDED.position`,
			fw.ID, fw.Name, fw.Version, fw.Publisher, fw.Description, sections, i)
		for j, r := range fw.Requirements {
			id := RequirementID(fw.ID, r.Code)
			reqIDs = append(reqIDs, id)
			b.Queue(`INSERT INTO catalog_requirements (id, framework_id, code, section, title, description, position) VALUES ($1,$2,$3,$4,$5,$6,$7)
				ON CONFLICT (id) DO UPDATE SET section=EXCLUDED.section, title=EXCLUDED.title, description=EXCLUDED.description, position=EXCLUDED.position`,
				id, fw.ID, r.Code, r.Section, r.Title, r.Description, j)
			b.Queue(`DELETE FROM catalog_requirement_controls WHERE requirement_id = $1 AND NOT (control_code = ANY($2))`, id, r.Controls)
			for _, cc := range r.Controls {
				b.Queue(`INSERT INTO catalog_requirement_controls (requirement_id, control_code) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, cc)
			}
		}
	}
	testIDs := []string{}
	for _, t := range c.Tests {
		testIDs = append(testIDs, t.ID)
		rem, _ := json.Marshal(t.Remediation)
		b.Queue(`INSERT INTO catalog_tests (id, name, description, connector, resource, control_code, severity, rule, remediation) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description, connector=EXCLUDED.connector, resource=EXCLUDED.resource,
			  control_code=EXCLUDED.control_code, severity=EXCLUDED.severity, rule=EXCLUDED.rule, remediation=EXCLUDED.remediation`,
			t.ID, t.Name, t.Description, t.Connector, t.Resource, t.Control, t.Severity, []byte(t.RuleJSON), rem)
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return err
	}
	// Remove catalog rows that disappeared. Org tests referencing a removed
	// catalog test keep their own copy of the rule, so they are detached.
	if _, err := tx.Exec(ctx, `UPDATE tests SET catalog_id = NULL, custom = true WHERE catalog_id IS NOT NULL AND NOT (catalog_id = ANY($1))`, testIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM catalog_tests WHERE NOT (id = ANY($1))`, testIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM catalog_requirements WHERE NOT (id = ANY($1))`, reqIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM catalog_frameworks WHERE NOT (id = ANY($1)) AND id NOT IN (SELECT framework_id FROM org_frameworks)`, fwIDs); err != nil {
		return err
	}
	// Refresh org test definitions from the catalog (rules and remediation evolve).
	if _, err := tx.Exec(ctx, `UPDATE tests t SET name = c.name, description = c.description, severity = c.severity, rule = c.rule, remediation = c.remediation, updated_at = now()
		FROM catalog_tests c WHERE t.catalog_id = c.id AND (t.rule <> c.rule OR t.remediation <> c.remediation OR t.name <> c.name)`); err != nil {
		return err
	}
	_ = codes
	return tx.Commit(ctx)
}
