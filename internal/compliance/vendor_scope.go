package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

// generatedPrefix marks a questionnaire generated from a custom framework's
// requirements: "framework:<custom framework id>".
const generatedPrefix = "framework:"

const maxVendorQuestionnaires = 20

// QuestionnaireSummary describes a questionnaire that can be attached to a vendor.
type QuestionnaireSummary struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Frameworks    []string `json:"frameworks"`
	QuestionCount int      `json:"question_count"`
	Generated     bool     `json:"generated"`
}

func summarize(t catalog.QuestionnaireTemplate, generated bool) QuestionnaireSummary {
	return QuestionnaireSummary{ID: t.ID, Title: t.Title, Description: t.Description, Frameworks: t.Frameworks, QuestionCount: len(t.Questions), Generated: generated}
}

// QuestionnaireLibrary lists the catalog questionnaires plus one generated
// from each of the org's custom frameworks.
func (s *Service) QuestionnaireLibrary(ctx context.Context, orgID domain.ID) ([]QuestionnaireSummary, error) {
	out := []QuestionnaireSummary{}
	for _, t := range s.Catalog.VendorQuestionnaires {
		out = append(out, summarize(t, false))
	}
	rows, err := s.Pool.Query(ctx, `SELECT f.id, f.name, count(r.id) FROM custom_frameworks f LEFT JOIN custom_requirements r ON r.framework_id = f.id
		WHERE f.org_id = $1 GROUP BY f.id ORDER BY lower(f.name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		var n int
		if err := rows.Scan(&id, &name, &n); err != nil {
			return nil, err
		}
		g := generatedTemplate(&catalog.Framework{ID: id, Name: name})
		out = append(out, QuestionnaireSummary{ID: g.ID, Title: g.Title, Description: g.Description, Frameworks: g.Frameworks, QuestionCount: n + 1, Generated: true})
	}
	return out, rows.Err()
}

// generatedTemplate asks the vendor to confirm each requirement of a custom
// framework. Question ids are namespaced by framework so several generated
// questionnaires can be sent together.
func generatedTemplate(fw *catalog.Framework) catalog.QuestionnaireTemplate {
	t := catalog.QuestionnaireTemplate{
		ID:          generatedPrefix + fw.ID,
		Title:       fw.Name + " requirements",
		Description: "Confirm whether your service meets each requirement of " + fw.Name + ".",
		Frameworks:  []string{fw.ID},
		Questions:   []catalog.QuestionnaireQuestion{},
	}
	for i, r := range fw.Requirements {
		q := r.Code + ": " + strings.TrimRight(r.Title, ". ") + "."
		if r.Description != "" {
			q += " " + r.Description
		}
		t.Questions = append(t.Questions, catalog.QuestionnaireQuestion{ID: fmt.Sprintf("%s/%d", fw.ID, i+1), Section: r.Section, Type: "yes_no",
			Question: q + " Does your service meet this requirement?"})
	}
	t.Questions = append(t.Questions, catalog.QuestionnaireQuestion{ID: fw.ID + "/exceptions", Section: "Exceptions", Type: "text",
		Question: "Describe any exceptions to the requirements above, or write None."})
	return t
}

// resolveQuestionnaires turns questionnaire ids into the questions to send.
func (s *Service) resolveQuestionnaires(ctx context.Context, q db.Querier, orgID domain.ID, ids []string) ([]catalog.QuestionnaireTemplate, error) {
	out := []catalog.QuestionnaireTemplate{}
	for _, id := range ids {
		if fwID, ok := strings.CutPrefix(id, generatedPrefix); ok {
			fw, custom, err := s.loadFramework(ctx, q, orgID, fwID)
			if errors.Is(err, domain.ErrNotFound) || (err == nil && !custom) {
				return nil, domain.Invalid("questionnaires", "unknown questionnaire "+id)
			}
			if err != nil {
				return nil, err
			}
			out = append(out, generatedTemplate(fw))
			continue
		}
		t := s.Catalog.Questionnaire(id)
		if t == nil {
			return nil, domain.Invalid("questionnaires", "unknown questionnaire "+id)
		}
		out = append(out, *t)
	}
	return out, nil
}

// suggestedQuestionnaires returns the general questionnaires plus those tied
// to the given frameworks.
func (s *Service) suggestedQuestionnaires(frameworkIDs []string) []string {
	return s.questionnairesFor(frameworkIDs, true)
}

func (s *Service) questionnairesFor(frameworkIDs []string, general bool) []string {
	mapped := map[string]bool{}
	for _, id := range frameworkIDs {
		mapped[id] = true
	}
	out := []string{}
	for _, t := range s.Catalog.VendorQuestionnaires {
		match := general && len(t.Frameworks) == 0
		for _, f := range t.Frameworks {
			match = match || mapped[f]
		}
		if match {
			out = append(out, t.ID)
		}
	}
	for _, id := range frameworkIDs {
		if strings.HasPrefix(id, customPrefix) {
			out = append(out, generatedPrefix+id)
		}
	}
	return out
}

func queryStrings(ctx context.Context, q db.Querier, sql string, args ...any) ([]string, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) vendorFrameworks(ctx context.Context, q db.Querier, orgID, vendorID domain.ID) ([]string, error) {
	return queryStrings(ctx, q, `SELECT framework_id FROM vendor_frameworks WHERE org_id = $1 AND vendor_id = $2 ORDER BY created_at, framework_id`, orgID, vendorID)
}

func (s *Service) attachedQuestionnaires(ctx context.Context, q db.Querier, orgID, vendorID domain.ID) ([]string, error) {
	return queryStrings(ctx, q, `SELECT template_id FROM vendor_questionnaire_templates WHERE org_id = $1 AND vendor_id = $2 ORDER BY position, template_id`, orgID, vendorID)
}

// effectiveQuestionnaires is what a questionnaire sent now would contain: the
// attached set, or the suggestions for the vendor's frameworks if none is attached.
func (s *Service) effectiveQuestionnaires(ctx context.Context, q db.Querier, orgID, vendorID domain.ID, frameworks []string) ([]string, bool, error) {
	attached, err := s.attachedQuestionnaires(ctx, q, orgID, vendorID)
	if err != nil || len(attached) > 0 {
		return attached, false, err
	}
	return s.suggestedQuestionnaires(frameworks), true, nil
}

func cleanIDs(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// setVendorScope replaces the vendor's frameworks and/or attached
// questionnaires (nil leaves either unchanged). Newly mapped frameworks must be
// in scope; existing mappings are kept even if their framework left scope.
// Mapping a framework while questionnaires are attached explicitly also
// attaches the questionnaires tied to it.
func (s *Service) setVendorScope(ctx context.Context, tx pgx.Tx, orgID, vendorID domain.ID, frameworks, questionnaires *[]string, changes map[string]any) error {
	if frameworks != nil {
		want := cleanIDs(*frameworks)
		cur, err := s.vendorFrameworks(ctx, tx, orgID, vendorID)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, id := range cur {
			have[id] = true
		}
		var added []string
		for _, id := range want {
			if !have[id] {
				added = append(added, id)
			}
		}
		if len(added) > 0 {
			inScope, err := queryStrings(ctx, tx, `SELECT framework_id FROM org_frameworks WHERE org_id = $1 AND framework_id = ANY($2)`, orgID, added)
			if err != nil {
				return err
			}
			ok := map[string]bool{}
			for _, id := range inScope {
				ok[id] = true
			}
			for _, id := range added {
				if ok[id] {
					continue
				}
				fw, _, err := s.loadFramework(ctx, tx, orgID, id)
				if err != nil {
					return domain.Invalid("frameworks", "unknown framework "+id)
				}
				return domain.Invalid("frameworks", fw.Name+" is not in scope; add it on the Frameworks page first")
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM vendor_frameworks WHERE org_id = $1 AND vendor_id = $2 AND NOT (framework_id = ANY($3))`, orgID, vendorID, want); err != nil {
			return err
		}
		for _, id := range added {
			if _, err := tx.Exec(ctx, `INSERT INTO vendor_frameworks (org_id, vendor_id, framework_id) VALUES ($1,$2,$3)`, orgID, vendorID, id); err != nil {
				return err
			}
		}
		changes["frameworks"] = want
		if questionnaires == nil && len(added) > 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO vendor_questionnaire_templates (org_id, vendor_id, template_id, position)
				SELECT $1, $2, t.id, (SELECT COALESCE(max(position), -1) FROM vendor_questionnaire_templates WHERE vendor_id = $2) + t.n
				FROM unnest($3::text[]) WITH ORDINALITY AS t(id, n)
				WHERE EXISTS (SELECT 1 FROM vendor_questionnaire_templates WHERE vendor_id = $2)
				ON CONFLICT DO NOTHING`, orgID, vendorID, s.questionnairesFor(added, false)); err != nil {
				return err
			}
		}
	}
	if questionnaires != nil {
		want := cleanIDs(*questionnaires)
		switch {
		case len(want) == 0:
			return domain.Invalid("questionnaires", "attach at least one questionnaire")
		case len(want) > maxVendorQuestionnaires:
			return domain.Invalid("questionnaires", fmt.Sprintf("attach at most %d questionnaires", maxVendorQuestionnaires))
		}
		if _, err := s.resolveQuestionnaires(ctx, tx, orgID, want); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM vendor_questionnaire_templates WHERE org_id = $1 AND vendor_id = $2`, orgID, vendorID); err != nil {
			return err
		}
		for i, id := range want {
			if _, err := tx.Exec(ctx, `INSERT INTO vendor_questionnaire_templates (org_id, vendor_id, template_id, position) VALUES ($1,$2,$3,$4)`, orgID, vendorID, id, i); err != nil {
				return err
			}
		}
		changes["questionnaires"] = want
	}
	return nil
}

// sentTemplates returns the questions a questionnaire was sent with. Rows sent
// before snapshots existed name a single catalog template.
func (s *Service) sentTemplates(raw []byte, templateID string) []catalog.QuestionnaireTemplate {
	var out []catalog.QuestionnaireTemplate
	if len(raw) > 0 && json.Unmarshal(raw, &out) == nil && len(out) > 0 {
		return out
	}
	if t := s.Catalog.Questionnaire(templateID); t != nil {
		return []catalog.QuestionnaireTemplate{*t}
	}
	return []catalog.QuestionnaireTemplate{}
}

func countQuestions(tpls []catalog.QuestionnaireTemplate) int {
	n := 0
	for _, t := range tpls {
		n += len(t.Questions)
	}
	return n
}

// frameworkNames resolves framework ids for display and AI context.
func (s *Service) frameworkNames(ctx context.Context, orgID domain.ID, ids []string) []string {
	out := []string{}
	for _, id := range ids {
		if fw, _, err := s.loadFramework(ctx, s.Pool, orgID, id); err == nil {
			out = append(out, fw.Name)
		}
	}
	return out
}
