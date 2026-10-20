package compliance

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
)

type Readiness struct {
	RequirementsTotal int     `json:"requirements_total"`
	RequirementsMet   int     `json:"requirements_met"`
	ControlsTotal     int     `json:"controls_total"`
	ControlsPassing   int     `json:"controls_passing"`
	ControlsFailing   int     `json:"controls_failing"`
	Percent           float64 `json:"percent"` // requirements met / total, 0-100
}

type FrameworkSummary struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Publisher   string     `json:"publisher"`
	Description string     `json:"description"`
	Adopted     bool       `json:"adopted"`
	AdoptedAt   *time.Time `json:"adopted_at,omitempty"`
	Readiness   Readiness  `json:"readiness"`
}

// readiness computes requirement- and control-level readiness for the given frameworks.
func (s *Service) readiness(ctx context.Context, q db.Querier, orgID domain.ID, fwIDs []string) (map[string]Readiness, error) {
	out := map[string]Readiness{}
	rows, err := q.Query(ctx, `
		WITH req AS (
		  SELECT r.framework_id, r.id,
		         bool_and(COALESCE(c.status = 'passing', false)) AS met
		  FROM catalog_requirements r
		  JOIN catalog_requirement_controls rc ON rc.requirement_id = r.id
		  LEFT JOIN controls c ON c.org_id = $1 AND c.code = rc.control_code
		  WHERE r.framework_id = ANY($2)
		  GROUP BY r.framework_id, r.id
		)
		SELECT framework_id, count(*), count(*) FILTER (WHERE met) FROM req GROUP BY framework_id`, orgID, fwIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var r Readiness
		if err := rows.Scan(&id, &r.RequirementsTotal, &r.RequirementsMet); err != nil {
			rows.Close()
			return nil, err
		}
		if r.RequirementsTotal > 0 {
			r.Percent = float64(int(float64(r.RequirementsMet)*1000/float64(r.RequirementsTotal))) / 10
		}
		out[id] = r
	}
	rows.Close()
	rows, err = q.Query(ctx, `
		SELECT r.framework_id, count(DISTINCT rc.control_code),
		       count(DISTINCT rc.control_code) FILTER (WHERE c.status = 'passing'),
		       count(DISTINCT rc.control_code) FILTER (WHERE c.status = 'failing')
		FROM catalog_requirements r
		JOIN catalog_requirement_controls rc ON rc.requirement_id = r.id
		LEFT JOIN controls c ON c.org_id = $1 AND c.code = rc.control_code
		WHERE r.framework_id = ANY($2)
		GROUP BY r.framework_id`, orgID, fwIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var total, passing, failing int
		if err := rows.Scan(&id, &total, &passing, &failing); err != nil {
			return nil, err
		}
		r := out[id]
		r.ControlsTotal, r.ControlsPassing, r.ControlsFailing = total, passing, failing
		out[id] = r
	}
	return out, rows.Err()
}

// ListFrameworks returns every catalog framework with adoption state and readiness.
func (s *Service) ListFrameworks(ctx context.Context, orgID domain.ID) ([]FrameworkSummary, error) {
	rows, err := s.Pool.Query(ctx, `SELECT f.id, f.name, f.version, f.publisher, f.description, of.adopted_at
		FROM catalog_frameworks f LEFT JOIN org_frameworks of ON of.framework_id = f.id AND of.org_id = $1
		ORDER BY of.adopted_at IS NULL, f.position`, orgID)
	if err != nil {
		return nil, err
	}
	var out []FrameworkSummary
	var ids []string
	for rows.Next() {
		var f FrameworkSummary
		if err := rows.Scan(&f.ID, &f.Name, &f.Version, &f.Publisher, &f.Description, &f.AdoptedAt); err != nil {
			rows.Close()
			return nil, err
		}
		f.Adopted = f.AdoptedAt != nil
		out = append(out, f)
		ids = append(ids, f.ID)
	}
	rows.Close()
	rd, err := s.readiness(ctx, s.Pool, orgID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Readiness = rd[out[i].ID]
	}
	if out == nil {
		out = []FrameworkSummary{}
	}
	return out, nil
}

func (s *Service) adoptedFrameworks(ctx context.Context, q db.Querier, orgID domain.ID) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT framework_id FROM org_frameworks WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AdoptFramework puts a framework in scope. Controls it maps to are created if
// missing; controls that already exist (and may already pass) count at once —
// that is the crosswalk paying off.
func (s *Service) AdoptFramework(ctx context.Context, orgID domain.ID, fwID string, actor domain.Actor) error {
	fw := s.Catalog.Framework(fwID)
	if fw == nil {
		return domain.ErrNotFound
	}
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO org_frameworks (org_id, framework_id, adopted_by) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, orgID, fwID, actor.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.Invalid("framework", fw.Name+" is already in scope")
		}
		created, err := s.ensureControls(ctx, tx, orgID, s.Catalog.ControlsForFrameworks([]string{fwID}))
		if err != nil {
			return err
		}
		newTests, err := s.ensureTests(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if _, err := s.recompute(ctx, tx, orgID, nil); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "framework.adopted", TargetType: "framework", TargetID: fwID, TargetLabel: fw.Name,
			Metadata: map[string]any{"controls_created": created, "tests_created": newTests}}); err != nil {
			return err
		}
		if newTests > 0 {
			return s.Jobs.EnqueueTx(ctx, tx, jobs.RescanOrgArgs{OrgID: orgID.String(), Trigger: "framework_adopted"})
		}
		return nil
	})
}

// RemoveFramework takes a framework out of scope. Controls stay (other
// frameworks or evidence may rely on them).
func (s *Service) RemoveFramework(ctx context.Context, orgID domain.ID, fwID string, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM org_frameworks WHERE org_id = $1 AND framework_id = $2`, orgID, fwID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		name := fwID
		if fw := s.Catalog.Framework(fwID); fw != nil {
			name = fw.Name
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "framework.removed", TargetType: "framework", TargetID: fwID, TargetLabel: name})
	})
}

type RequirementControl struct {
	ID     *domain.ID `json:"id,omitempty"` // nil when the control is not in the org
	Code   string     `json:"code"`
	Name   string     `json:"name"`
	Status string     `json:"status"` // passing | failing | needs_evidence | not_in_scope
}

type RequirementView struct {
	Code        string               `json:"code"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Met         bool                 `json:"met"`
	Controls    []RequirementControl `json:"controls"`
}

type SectionView struct {
	Name         string            `json:"name"`
	Met          int               `json:"met"`
	Total        int               `json:"total"`
	Requirements []RequirementView `json:"requirements"`
}

type FrameworkDetail struct {
	FrameworkSummary
	Sections []SectionView `json:"sections"`
}

// GetFramework returns readiness with a requirement-by-requirement drilldown.
func (s *Service) GetFramework(ctx context.Context, orgID domain.ID, fwID string) (*FrameworkDetail, error) {
	fw := s.Catalog.Framework(fwID)
	if fw == nil {
		return nil, domain.ErrNotFound
	}
	d := &FrameworkDetail{FrameworkSummary: FrameworkSummary{ID: fw.ID, Name: fw.Name, Version: fw.Version, Publisher: fw.Publisher, Description: fw.Description}}
	if err := s.Pool.QueryRow(ctx, `SELECT adopted_at FROM org_frameworks WHERE org_id = $1 AND framework_id = $2`, orgID, fwID).Scan(&d.AdoptedAt); err != nil && !db.IsNoRows(err) {
		return nil, err
	}
	d.Adopted = d.AdoptedAt != nil
	rd, err := s.readiness(ctx, s.Pool, orgID, []string{fwID})
	if err != nil {
		return nil, err
	}
	d.Readiness = rd[fwID]
	// Org control status by code.
	type ctl struct {
		id     domain.ID
		status string
	}
	byCode := map[string]ctl{}
	rows, err := s.Pool.Query(ctx, `SELECT id, code, status FROM controls WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c ctl
		var code string
		if err := rows.Scan(&c.id, &code, &c.status); err != nil {
			rows.Close()
			return nil, err
		}
		byCode[code] = c
	}
	rows.Close()
	sectionIdx := map[string]int{}
	order := append([]string{}, fw.Sections...)
	for _, r := range fw.Requirements {
		if _, ok := sectionIdx[r.Section]; !ok {
			found := false
			for _, o := range order {
				if o == r.Section {
					found = true
				}
			}
			if !found {
				order = append(order, r.Section)
			}
			sectionIdx[r.Section] = -1
		}
	}
	sections := map[string]*SectionView{}
	for _, name := range order {
		sections[name] = &SectionView{Name: name, Requirements: []RequirementView{}}
	}
	for _, r := range fw.Requirements {
		rv := RequirementView{Code: r.Code, Title: r.Title, Description: r.Description, Met: true}
		for _, cc := range r.Controls {
			cat := s.Catalog.Control(cc)
			rc := RequirementControl{Code: cc, Name: cat.Name, Status: "not_in_scope"}
			if c, ok := byCode[cc]; ok {
				id := c.id
				rc.ID, rc.Status = &id, c.status
			}
			if rc.Status != "passing" {
				rv.Met = false
			}
			rv.Controls = append(rv.Controls, rc)
		}
		sec := sections[r.Section]
		sec.Requirements = append(sec.Requirements, rv)
		sec.Total++
		if rv.Met {
			sec.Met++
		}
	}
	for _, name := range order {
		if sec := sections[name]; sec != nil && sec.Total > 0 {
			d.Sections = append(d.Sections, *sec)
		}
	}
	return d, nil
}

// ensureControls creates org controls for catalog codes that are missing.
func (s *Service) ensureControls(ctx context.Context, tx pgx.Tx, orgID domain.ID, codes []string) (int, error) {
	sort.Strings(codes)
	created := 0
	for _, code := range codes {
		c := s.Catalog.Control(code)
		if c == nil {
			continue
		}
		tag, err := tx.Exec(ctx, `INSERT INTO controls (id, org_id, code, name, domain, description, evidence_mode, status_reason)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (org_id, code) DO NOTHING`,
			domain.NewID(), orgID, c.Code, c.Name, c.Domain, c.Description, c.Evidence, initialReason(c.Evidence))
		if err != nil {
			return created, err
		}
		created += int(tag.RowsAffected())
	}
	return created, nil
}

func initialReason(mode string) string {
	if mode == "automated" {
		return "Connect an integration that monitors this control, or upload evidence."
	}
	return "Upload evidence for this control."
}
