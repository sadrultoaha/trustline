package compliance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
)

// customPrefix marks org-authored framework ids so they never collide with catalog ids.
const customPrefix = "custom-"

const defaultSection = "Requirements"

type CustomRequirementInput struct {
	Code        string   `json:"code"`
	Section     string   `json:"section"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Controls    []string `json:"controls"`
}

// CustomFrameworkInput defines a whole custom framework; updates replace it.
type CustomFrameworkInput struct {
	Name         string                   `json:"name"`
	Version      string                   `json:"version"`
	Publisher    string                   `json:"publisher"`
	Description  string                   `json:"description"`
	Requirements []CustomRequirementInput `json:"requirements"`
}

// CustomFrameworkDefinition is the editable form of a custom framework.
type CustomFrameworkDefinition struct {
	ID string `json:"id"`
	CustomFrameworkInput
}

// loadFramework resolves a catalog framework or one of the org's custom ones.
func (s *Service) loadFramework(ctx context.Context, q db.Querier, orgID domain.ID, fwID string) (*catalog.Framework, bool, error) {
	if fw := s.Catalog.Framework(fwID); fw != nil {
		return fw, false, nil
	}
	if !strings.HasPrefix(fwID, customPrefix) {
		return nil, false, domain.ErrNotFound
	}
	fw := &catalog.Framework{ID: fwID}
	if err := q.QueryRow(ctx, `SELECT name, version, publisher, description FROM custom_frameworks WHERE org_id = $1 AND id = $2`, orgID, fwID).
		Scan(&fw.Name, &fw.Version, &fw.Publisher, &fw.Description); err != nil {
		if db.IsNoRows(err) {
			return nil, false, domain.ErrNotFound
		}
		return nil, false, err
	}
	rows, err := q.Query(ctx, `SELECT r.code, r.section, r.title, r.description,
		       COALESCE(array_agg(rc.control_code ORDER BY rc.control_code) FILTER (WHERE rc.control_code IS NOT NULL), '{}')
		FROM custom_requirements r LEFT JOIN custom_requirement_controls rc ON rc.requirement_id = r.id
		WHERE r.org_id = $1 AND r.framework_id = $2
		GROUP BY r.id ORDER BY r.position`, orgID, fwID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var r catalog.Requirement
		if err := rows.Scan(&r.Code, &r.Section, &r.Title, &r.Description, &r.Controls); err != nil {
			return nil, false, err
		}
		fw.Requirements = append(fw.Requirements, r)
	}
	return fw, true, rows.Err()
}

// frameworkControls returns the distinct control codes a framework maps to.
func frameworkControls(fw *catalog.Framework) []string {
	set := map[string]bool{}
	out := []string{}
	for _, r := range fw.Requirements {
		for _, cc := range r.Controls {
			if !set[cc] {
				set[cc] = true
				out = append(out, cc)
			}
		}
	}
	return out
}

// ControlLibrary lists the controls custom requirements can map to.
func (s *Service) ControlLibrary() []catalog.Control { return s.Catalog.Controls }

// GetCustomFramework returns a custom framework in its editable form.
func (s *Service) GetCustomFramework(ctx context.Context, orgID domain.ID, fwID string) (*CustomFrameworkDefinition, error) {
	fw, custom, err := s.loadFramework(ctx, s.Pool, orgID, fwID)
	if err != nil {
		return nil, err
	}
	if !custom {
		return nil, domain.ErrNotFound
	}
	d := &CustomFrameworkDefinition{ID: fw.ID, CustomFrameworkInput: CustomFrameworkInput{
		Name: fw.Name, Version: fw.Version, Publisher: fw.Publisher, Description: fw.Description, Requirements: []CustomRequirementInput{},
	}}
	for _, r := range fw.Requirements {
		d.Requirements = append(d.Requirements, CustomRequirementInput{Code: r.Code, Section: r.Section, Title: r.Title, Description: r.Description, Controls: r.Controls})
	}
	return d, nil
}

// normalize trims the input and checks it against the control library.
func (s *Service) normalizeCustomFramework(in *CustomFrameworkInput) error {
	v := domain.NewValidation()
	in.Name, in.Version, in.Publisher, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Version), strings.TrimSpace(in.Publisher), strings.TrimSpace(in.Description)
	switch {
	case in.Name == "":
		v.Add("name", "Name is required")
	case len(in.Name) > 120:
		v.Add("name", "Name must be at most 120 characters")
	default:
		for _, fw := range s.Catalog.Frameworks {
			if strings.EqualFold(fw.Name, in.Name) {
				v.Add("name", fw.Name+" is a built-in framework; choose another name")
			}
		}
	}
	if len(in.Version) > 120 {
		v.Add("version", "Version must be at most 120 characters")
	}
	if len(in.Publisher) > 120 {
		v.Add("publisher", "Publisher must be at most 120 characters")
	}
	if len(in.Description) > 4000 {
		v.Add("description", "Description must be at most 4000 characters")
	}
	switch {
	case len(in.Requirements) == 0:
		v.Add("requirements", "Add at least one requirement")
	case len(in.Requirements) > 500:
		v.Add("requirements", "A framework can have at most 500 requirements")
	}
	seen := map[string]bool{}
	for i := range in.Requirements {
		r := &in.Requirements[i]
		f := func(name string) string { return fmt.Sprintf("requirements.%d.%s", i, name) }
		r.Code, r.Section, r.Title, r.Description = strings.TrimSpace(r.Code), strings.TrimSpace(r.Section), strings.TrimSpace(r.Title), strings.TrimSpace(r.Description)
		if r.Section == "" {
			r.Section = defaultSection
		}
		switch {
		case r.Code == "":
			v.Add(f("code"), "Code is required")
		case len(r.Code) > 40:
			v.Add(f("code"), "Code must be at most 40 characters")
		case seen[strings.ToLower(r.Code)]:
			v.Add(f("code"), "Code "+r.Code+" is used twice")
		}
		seen[strings.ToLower(r.Code)] = true
		if r.Title == "" {
			v.Add(f("title"), "Title is required")
		} else if len(r.Title) > 300 {
			v.Add(f("title"), "Title must be at most 300 characters")
		}
		if len(r.Section) > 120 {
			v.Add(f("section"), "Section must be at most 120 characters")
		}
		if len(r.Description) > 4000 {
			v.Add(f("description"), "Description must be at most 4000 characters")
		}
		codes := []string{}
		dup := map[string]bool{}
		for _, cc := range r.Controls {
			cc = strings.TrimSpace(cc)
			if dup[cc] {
				continue
			}
			dup[cc] = true
			if s.Catalog.Control(cc) == nil {
				v.Add(f("controls"), "Unknown control "+cc)
			}
			codes = append(codes, cc)
		}
		if len(codes) == 0 {
			v.Add(f("controls"), "Map at least one control")
		}
		r.Controls = codes
	}
	return v.Err()
}

func (in *CustomFrameworkInput) framework(id string) *catalog.Framework {
	fw := &catalog.Framework{ID: id, Name: in.Name, Version: in.Version, Publisher: in.Publisher, Description: in.Description}
	for _, r := range in.Requirements {
		fw.Requirements = append(fw.Requirements, catalog.Requirement{Code: r.Code, Section: r.Section, Title: r.Title, Description: r.Description, Controls: r.Controls})
	}
	return fw
}

func writeCustomRequirements(ctx context.Context, tx pgx.Tx, orgID domain.ID, fwID string, reqs []CustomRequirementInput) error {
	if _, err := tx.Exec(ctx, `DELETE FROM custom_requirements WHERE org_id = $1 AND framework_id = $2`, orgID, fwID); err != nil {
		return err
	}
	b := &pgx.Batch{}
	for i, r := range reqs {
		id := domain.NewID()
		b.Queue(`INSERT INTO custom_requirements (id, org_id, framework_id, code, section, title, description, position) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			id, orgID, fwID, r.Code, r.Section, r.Title, r.Description, i)
		for _, cc := range r.Controls {
			b.Queue(`INSERT INTO custom_requirement_controls (org_id, requirement_id, control_code) VALUES ($1,$2,$3)`, orgID, id, cc)
		}
	}
	return tx.SendBatch(ctx, b).Close()
}

func nameTaken(err error) error {
	if db.IsUniqueViolation(err) {
		return domain.Invalid("name", "A framework with this name already exists")
	}
	return err
}

// CreateCustomFramework adds an org-authored framework. It starts out of
// scope, like any catalog framework, until it is adopted.
func (s *Service) CreateCustomFramework(ctx context.Context, orgID domain.ID, in CustomFrameworkInput, actor domain.Actor) (string, error) {
	if err := s.normalizeCustomFramework(&in); err != nil {
		return "", err
	}
	id := customPrefix + domain.NewID().String()
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO custom_frameworks (id, org_id, name, version, publisher, description, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			id, orgID, in.Name, in.Version, in.Publisher, in.Description, actor.ID); err != nil {
			return nameTaken(err)
		}
		if err := writeCustomRequirements(ctx, tx, orgID, id, in.Requirements); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "framework.created", TargetType: "framework", TargetID: id, TargetLabel: in.Name,
			Metadata: map[string]any{"requirements": len(in.Requirements)}})
	})
	return id, err
}

// UpdateCustomFramework replaces a custom framework's definition. If it is in
// scope, newly mapped controls are brought into scope too.
func (s *Service) UpdateCustomFramework(ctx context.Context, orgID domain.ID, fwID string, in CustomFrameworkInput, actor domain.Actor) error {
	if err := s.normalizeCustomFramework(&in); err != nil {
		return err
	}
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE custom_frameworks SET name = $3, version = $4, publisher = $5, description = $6, updated_at = now() WHERE org_id = $1 AND id = $2`,
			orgID, fwID, in.Name, in.Version, in.Publisher, in.Description)
		if err != nil {
			return nameTaken(err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		if err := writeCustomRequirements(ctx, tx, orgID, fwID, in.Requirements); err != nil {
			return err
		}
		var adopted bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_frameworks WHERE org_id = $1 AND framework_id = $2)`, orgID, fwID).Scan(&adopted); err != nil {
			return err
		}
		meta := map[string]any{"requirements": len(in.Requirements)}
		newTests := 0
		if adopted {
			created, nt, err := s.scopeControls(ctx, tx, orgID, in.framework(fwID))
			if err != nil {
				return err
			}
			meta["controls_created"], newTests = created, nt
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "framework.updated", TargetType: "framework", TargetID: fwID, TargetLabel: in.Name, Metadata: meta}); err != nil {
			return err
		}
		if newTests > 0 {
			return s.Jobs.EnqueueTx(ctx, tx, jobs.RescanOrgArgs{OrgID: orgID.String(), Trigger: "framework_updated"})
		}
		return nil
	})
}

// DeleteCustomFramework removes a custom framework, takes it out of scope and
// unmaps it from vendors. Controls stay, as with RemoveFramework; questionnaires
// already sent keep their questions.
func (s *Service) DeleteCustomFramework(ctx context.Context, orgID domain.ID, fwID string, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `DELETE FROM custom_frameworks WHERE org_id = $1 AND id = $2 RETURNING name`, orgID, fwID).Scan(&name); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM org_frameworks WHERE org_id = $1 AND framework_id = $2`, orgID, fwID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM vendor_frameworks WHERE org_id = $1 AND framework_id = $2`, orgID, fwID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM vendor_questionnaire_templates WHERE org_id = $1 AND template_id = $2`, orgID, generatedPrefix+fwID); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "framework.deleted", TargetType: "framework", TargetID: fwID, TargetLabel: name})
	})
}
