package compliance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

type RiskControl struct {
	ID     domain.ID `json:"id"`
	Code   string    `json:"code"`
	Name   string    `json:"name"`
	Status string    `json:"status"`
}

type Risk struct {
	ID                 domain.ID     `json:"id"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	Category           string        `json:"category"`
	Likelihood         int           `json:"likelihood"`
	Impact             int           `json:"impact"`
	Score              int           `json:"score"`
	ResidualLikelihood *int          `json:"residual_likelihood,omitempty"`
	ResidualImpact     *int          `json:"residual_impact,omitempty"`
	ResidualScore      *int          `json:"residual_score,omitempty"`
	Level              string        `json:"level"` // low | medium | high | critical
	Treatment          string        `json:"treatment"`
	TreatmentPlan      string        `json:"treatment_plan"`
	Status             string        `json:"status"`
	Owner              *Owner        `json:"owner,omitempty"`
	Source             string        `json:"source"`
	TestID             *domain.ID    `json:"test_id,omitempty"`
	AIRationale        string        `json:"ai_rationale"`
	ReviewCycleDays    int           `json:"review_cycle_days"`
	NextReviewAt       *time.Time    `json:"next_review_at,omitempty"`
	LastReviewedAt     *time.Time    `json:"last_reviewed_at,omitempty"`
	ReviewOverdue      bool          `json:"review_overdue"`
	Controls           []RiskControl `json:"controls"`
	CreatedAt          time.Time     `json:"created_at"`
}

func riskLevel(score int) string {
	switch {
	case score >= 20:
		return "critical"
	case score >= 12:
		return "high"
	case score >= 6:
		return "medium"
	}
	return "low"
}

const riskSelect = `SELECT r.id, r.title, r.description, r.category, r.likelihood, r.impact, r.residual_likelihood, r.residual_impact, r.treatment, r.treatment_plan,
  r.status, m.id, u.name, u.email, r.source, r.test_id, r.ai_rationale, r.review_cycle_days, r.next_review_at, r.last_reviewed_at, r.created_at
FROM risks r
LEFT JOIN org_memberships m ON m.id = r.owner_membership_id AND m.org_id = r.org_id
LEFT JOIN users u ON u.id = m.user_id`

func scanRisk(row pgx.Row) (*Risk, error) {
	var r Risk
	var mid *domain.ID
	var mname, memail *string
	if err := row.Scan(&r.ID, &r.Title, &r.Description, &r.Category, &r.Likelihood, &r.Impact, &r.ResidualLikelihood, &r.ResidualImpact, &r.Treatment, &r.TreatmentPlan,
		&r.Status, &mid, &mname, &memail, &r.Source, &r.TestID, &r.AIRationale, &r.ReviewCycleDays, &r.NextReviewAt, &r.LastReviewedAt, &r.CreatedAt); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if mid != nil {
		r.Owner = &Owner{MembershipID: *mid, Name: deref(mname), Email: deref(memail)}
	}
	r.Score = r.Likelihood * r.Impact
	if r.ResidualLikelihood != nil && r.ResidualImpact != nil {
		rs := *r.ResidualLikelihood * *r.ResidualImpact
		r.ResidualScore = &rs
	}
	r.Level = riskLevel(r.Score)
	r.ReviewOverdue = r.NextReviewAt != nil && r.NextReviewAt.Before(time.Now()) && r.Status != "closed" && r.Status != "dismissed" && r.Status != "suggested"
	r.Controls = []RiskControl{}
	return &r, nil
}

func (s *Service) loadRiskControls(ctx context.Context, q db.Querier, orgID domain.ID, risks []Risk) error {
	if len(risks) == 0 {
		return nil
	}
	idx := map[domain.ID]int{}
	var ids []domain.ID
	for i, r := range risks {
		idx[r.ID] = i
		ids = append(ids, r.ID)
	}
	rows, err := q.Query(ctx, `SELECT rc.risk_id, c.id, c.code, c.name, c.status FROM risk_controls rc JOIN controls c ON c.id = rc.control_id
		WHERE rc.org_id = $1 AND rc.risk_id = ANY($2) ORDER BY c.code`, orgID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rid domain.ID
		var c RiskControl
		if err := rows.Scan(&rid, &c.ID, &c.Code, &c.Name, &c.Status); err != nil {
			return err
		}
		risks[idx[rid]].Controls = append(risks[idx[rid]].Controls, c)
	}
	return rows.Err()
}

type RiskFilter struct {
	Status    string // "" = all except dismissed; "register" = open, in_treatment, accepted, closed
	ControlID *domain.ID
}

func (s *Service) ListRisks(ctx context.Context, orgID domain.ID, f RiskFilter) ([]Risk, error) {
	args := []any{orgID}
	where := "r.org_id = $1"
	switch f.Status {
	case "":
		where += " AND r.status <> 'dismissed'"
	case "register":
		where += " AND r.status IN ('open','in_treatment','accepted','closed')"
	default:
		args = append(args, f.Status)
		where += fmt.Sprintf(" AND r.status = $%d", len(args))
	}
	if f.ControlID != nil {
		args = append(args, *f.ControlID)
		where += fmt.Sprintf(" AND r.id IN (SELECT risk_id FROM risk_controls WHERE control_id = $%d)", len(args))
	}
	rows, err := s.Pool.Query(ctx, riskSelect+" WHERE "+where+" ORDER BY r.likelihood * r.impact DESC, r.created_at", args...)
	if err != nil {
		return nil, err
	}
	out := []Risk{}
	for rows.Next() {
		r, err := scanRisk(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *r)
	}
	rows.Close()
	return out, s.loadRiskControls(ctx, s.Pool, orgID, out)
}

func (s *Service) GetRisk(ctx context.Context, orgID, id domain.ID) (*Risk, error) {
	r, err := scanRisk(s.Pool.QueryRow(ctx, riskSelect+" WHERE r.org_id = $1 AND r.id = $2", orgID, id))
	if err != nil {
		return nil, err
	}
	list := []Risk{*r}
	if err := s.loadRiskControls(ctx, s.Pool, orgID, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// Heatmap counts register risks by likelihood (rows) and impact (cols).
type Heatmap struct {
	Inherent [5][5]int `json:"inherent"`
	Residual [5][5]int `json:"residual"`
}

func (s *Service) RiskHeatmap(ctx context.Context, orgID domain.ID) (*Heatmap, error) {
	risks, err := s.ListRisks(ctx, orgID, RiskFilter{Status: "register"})
	if err != nil {
		return nil, err
	}
	h := &Heatmap{}
	for _, r := range risks {
		if r.Status == "closed" {
			continue
		}
		h.Inherent[r.Likelihood-1][r.Impact-1]++
		l, i := r.Likelihood, r.Impact
		if r.ResidualLikelihood != nil && r.ResidualImpact != nil {
			l, i = *r.ResidualLikelihood, *r.ResidualImpact
		}
		h.Residual[l-1][i-1]++
	}
	return h, nil
}

type RiskInput struct {
	Title              *string  `json:"title"`
	Description        *string  `json:"description"`
	Category           *string  `json:"category"`
	Likelihood         *int     `json:"likelihood"`
	Impact             *int     `json:"impact"`
	ResidualLikelihood *int     `json:"residual_likelihood"`
	ResidualImpact     *int     `json:"residual_impact"`
	Treatment          *string  `json:"treatment"`
	TreatmentPlan      *string  `json:"treatment_plan"`
	Status             *string  `json:"status"`
	OwnerMembershipID  *string  `json:"owner_membership_id"`
	ReviewCycleDays    *int     `json:"review_cycle_days"`
	ControlIDs         []string `json:"control_ids"`
}

func validScale(v *int) bool { return v == nil || (*v >= 1 && *v <= 5) }

func (in RiskInput) validate(creating bool) error {
	v := domain.NewValidation()
	if creating && (in.Title == nil || strings.TrimSpace(*in.Title) == "") {
		v.Add("title", "title is required")
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		v.Add("title", "title is required")
	}
	if creating && (in.Likelihood == nil || in.Impact == nil) {
		v.Add("likelihood", "likelihood and impact are required")
	}
	if !validScale(in.Likelihood) || !validScale(in.Impact) || !validScale(in.ResidualLikelihood) || !validScale(in.ResidualImpact) {
		v.Add("likelihood", "likelihood and impact are 1 to 5")
	}
	if in.Treatment != nil {
		switch *in.Treatment {
		case "mitigate", "accept", "transfer", "avoid":
		default:
			v.Add("treatment", "treatment must be mitigate, accept, transfer or avoid")
		}
	}
	if in.Status != nil {
		switch *in.Status {
		case "open", "in_treatment", "accepted", "closed":
		default:
			v.Add("status", "status must be open, in_treatment, accepted or closed")
		}
	}
	if in.ReviewCycleDays != nil && (*in.ReviewCycleDays < 7 || *in.ReviewCycleDays > 730) {
		v.Add("review_cycle_days", "between 7 and 730 days")
	}
	return v.Err()
}

func (s *Service) setRiskControls(ctx context.Context, tx pgx.Tx, orgID, riskID domain.ID, ids []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM risk_controls WHERE org_id = $1 AND risk_id = $2`, orgID, riskID); err != nil {
		return err
	}
	for _, raw := range ids {
		cid, err := domain.ParseID(raw)
		if err != nil {
			return domain.Invalid("control_ids", "invalid control id")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO risk_controls (org_id, risk_id, control_id) SELECT $1, $2, id FROM controls WHERE org_id = $1 AND id = $3 ON CONFLICT DO NOTHING`, orgID, riskID, cid)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var dup bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_controls WHERE risk_id = $1 AND control_id = $2)`, riskID, cid).Scan(&dup)
			if !dup {
				return domain.Invalid("control_ids", "control not found")
			}
		}
	}
	return nil
}

func parseOwner(raw *string) (*domain.ID, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if *raw == "" {
		return nil, true, nil
	}
	id, err := domain.ParseID(*raw)
	if err != nil {
		return nil, true, domain.Invalid("owner_membership_id", "invalid id")
	}
	return &id, true, nil
}

func (s *Service) CreateRisk(ctx context.Context, orgID domain.ID, in RiskInput, actor domain.Actor) (*Risk, error) {
	if err := in.validate(true); err != nil {
		return nil, err
	}
	owner, _, err := parseOwner(in.OwnerMembershipID)
	if err != nil {
		return nil, err
	}
	id := domain.NewID()
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		cycle := 90
		if in.ReviewCycleDays != nil {
			cycle = *in.ReviewCycleDays
		}
		if _, err := tx.Exec(ctx, `INSERT INTO risks (id, org_id, title, description, category, likelihood, impact, residual_likelihood, residual_impact, treatment, treatment_plan,
			status, owner_membership_id, source, review_cycle_days, next_review_at, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'open',$12,'manual',$13, now() + make_interval(days => $13), $14)`,
			id, orgID, strings.TrimSpace(*in.Title), deref(in.Description), orDefault(in.Category, "Security"), *in.Likelihood, *in.Impact, in.ResidualLikelihood, in.ResidualImpact,
			orDefault(in.Treatment, "mitigate"), deref(in.TreatmentPlan), owner, cycle, actor.ID); err != nil {
			return err
		}
		if err := s.setRiskControls(ctx, tx, orgID, id, in.ControlIDs); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.created", TargetType: "risk", TargetID: id.String(), TargetLabel: *in.Title,
			Metadata: map[string]any{"score": *in.Likelihood * *in.Impact}}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetRisk(ctx, orgID, id)
}

func orDefault(p *string, def string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return def
	}
	return strings.TrimSpace(*p)
}

func (s *Service) UpdateRisk(ctx context.Context, orgID, id domain.ID, in RiskInput, actor domain.Actor) (*Risk, error) {
	if err := in.validate(false); err != nil {
		return nil, err
	}
	owner, setOwner, err := parseOwner(in.OwnerMembershipID)
	if err != nil {
		return nil, err
	}
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		cur, err := scanRisk(tx.QueryRow(ctx, riskSelect+" WHERE r.org_id = $1 AND r.id = $2 FOR UPDATE OF r", orgID, id))
		if err != nil {
			return err
		}
		if cur.Status == "suggested" && in.Status != nil {
			return domain.Invalid("status", "accept or dismiss the suggestion first")
		}
		set := func(col string, v any) error {
			_, err := tx.Exec(ctx, `UPDATE risks SET `+col+` = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, v)
			return err
		}
		changes := map[string]any{}
		apply := func(name string, ok bool, v any) error {
			if !ok {
				return nil
			}
			changes[name] = v
			return set(name, v)
		}
		if err := apply("title", in.Title != nil, strings.TrimSpace(deref(in.Title))); err != nil {
			return err
		}
		if err := apply("description", in.Description != nil, deref(in.Description)); err != nil {
			return err
		}
		if err := apply("category", in.Category != nil, deref(in.Category)); err != nil {
			return err
		}
		if in.Likelihood != nil {
			if err := apply("likelihood", true, *in.Likelihood); err != nil {
				return err
			}
		}
		if in.Impact != nil {
			if err := apply("impact", true, *in.Impact); err != nil {
				return err
			}
		}
		if err := apply("residual_likelihood", in.ResidualLikelihood != nil, in.ResidualLikelihood); err != nil {
			return err
		}
		if err := apply("residual_impact", in.ResidualImpact != nil, in.ResidualImpact); err != nil {
			return err
		}
		if err := apply("treatment", in.Treatment != nil, deref(in.Treatment)); err != nil {
			return err
		}
		if err := apply("treatment_plan", in.TreatmentPlan != nil, deref(in.TreatmentPlan)); err != nil {
			return err
		}
		if err := apply("status", in.Status != nil, deref(in.Status)); err != nil {
			return err
		}
		if err := apply("owner_membership_id", setOwner, owner); err != nil {
			return err
		}
		if in.ReviewCycleDays != nil {
			if err := apply("review_cycle_days", true, *in.ReviewCycleDays); err != nil {
				return err
			}
		}
		if in.ControlIDs != nil {
			if err := s.setRiskControls(ctx, tx, orgID, id, in.ControlIDs); err != nil {
				return err
			}
			changes["controls"] = len(in.ControlIDs)
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.updated", TargetType: "risk", TargetID: id.String(), TargetLabel: cur.Title, Metadata: changes}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetRisk(ctx, orgID, id)
}

// AcceptSuggestion moves a suggested risk into the register (keeping its linked controls).
func (s *Service) AcceptSuggestion(ctx context.Context, orgID, id domain.ID, actor domain.Actor, membership domain.ID) (*Risk, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		if err := tx.QueryRow(ctx, `UPDATE risks SET status = 'open', owner_membership_id = COALESCE(owner_membership_id, $3), next_review_at = now() + make_interval(days => review_cycle_days), updated_at = now()
			WHERE org_id = $1 AND id = $2 AND status = 'suggested' RETURNING title`, orgID, id, membership).Scan(&title); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("status", "only suggested risks can be accepted")
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.suggestion_accepted", TargetType: "risk", TargetID: id.String(), TargetLabel: title}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetRisk(ctx, orgID, id)
}

func (s *Service) DismissSuggestion(ctx context.Context, orgID, id domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		if err := tx.QueryRow(ctx, `UPDATE risks SET status = 'dismissed', updated_at = now() WHERE org_id = $1 AND id = $2 AND status = 'suggested' RETURNING title`, orgID, id).Scan(&title); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("status", "only suggested risks can be dismissed")
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.suggestion_dismissed", TargetType: "risk", TargetID: id.String(), TargetLabel: title}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

func (s *Service) MarkRiskReviewed(ctx context.Context, orgID, id domain.ID, actor domain.Actor) (*Risk, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var title string
		if err := tx.QueryRow(ctx, `UPDATE risks SET last_reviewed_at = now(), next_review_at = now() + make_interval(days => review_cycle_days), updated_at = now()
			WHERE org_id = $1 AND id = $2 AND status IN ('open','in_treatment','accepted') RETURNING title`, orgID, id).Scan(&title); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("status", "only risks in the register can be reviewed")
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'done', resolution = 'Risk reviewed.', completed_at = now(), updated_at = now()
			WHERE org_id = $1 AND kind = 'risk_review' AND status = 'open' AND meta->>'risk_id' = $2`, orgID, id.String()); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.reviewed", TargetType: "risk", TargetID: id.String(), TargetLabel: title}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetRisk(ctx, orgID, id)
}

// suggestRiskForTest raises a suggested risk when a critical test starts failing
// (called inside the test-run transaction).
func (s *Service) suggestRiskForTest(ctx context.Context, tx pgx.Tx, orgID domain.ID, t testTransition) error {
	key := "test:" + t.TestID.String()
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risks WHERE org_id = $1 AND source_key = $2 AND status IN ('suggested','open','in_treatment','accepted'))`, orgID, key).Scan(&exists); err != nil || exists {
		return err
	}
	var code, cname, desc string
	if err := tx.QueryRow(ctx, `SELECT c.code, c.name, t.description FROM tests t JOIN controls c ON c.id = t.control_id WHERE t.org_id = $1 AND t.id = $2`, orgID, t.TestID).Scan(&code, &cname, &desc); err != nil {
		return err
	}
	var ids []string
	for _, f := range t.Failing {
		if f.ID != "-" {
			ids = append(ids, f.ID)
		}
	}
	id := domain.NewID()
	title := fmt.Sprintf("%s: %s", cname, strings.TrimSuffix(t.Name, "."))
	description := fmt.Sprintf("The critical test \"%s\" is failing for %s. %s If exploited before remediation, this gap could lead to unauthorized access or data exposure.", t.Name, strings.Join(ids, ", "), desc)
	if _, err := tx.Exec(ctx, `INSERT INTO risks (id, org_id, title, description, category, likelihood, impact, treatment, treatment_plan, status, source, source_key, test_id, ai_rationale)
		VALUES ($1,$2,$3,$4,'Security',4,5,'mitigate',$5,'suggested','test',$6,$7,$8)`,
		id, orgID, title, description, "Complete the remediation task for the failing test and keep it passing.", key, t.TestID,
		fmt.Sprintf("Raised automatically because a critical test on control %s failed on %s.", code, time.Now().UTC().Format("2006-01-02 15:04 MST"))); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO risk_controls (org_id, risk_id, control_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, orgID, id, t.Control); err != nil {
		return err
	}
	if err := activity.Record(ctx, tx, orgID, domain.SystemActor("monitor"), activity.Entry{Action: "risk.suggested", TargetType: "risk", TargetID: id.String(), TargetLabel: title,
		Metadata: map[string]any{"test_id": t.TestID, "control": code}}); err != nil {
		return err
	}
	return s.notifyAdmins(ctx, tx, orgID, "risk_suggested", "Suggested risk: "+title, "A critical test failed on "+code+". Review the suggested risk and accept it into the register.", "/app/risks/"+id.String())
}

// CompleteWithAI drafts a risk assessment and stores the results as suggestions.
func (s *Service) CompleteWithAI(ctx context.Context, svc *ai.Service, orgID domain.ID, actor domain.Actor) ([]Risk, string, error) {
	in := ai.RiskAssessmentInput{}
	if err := s.Pool.QueryRow(ctx, `SELECT name FROM orgs WHERE id = $1`, orgID).Scan(&in.OrgName); err != nil {
		return nil, "", err
	}
	_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(array_agg(DISTINCT department ORDER BY department) FILTER (WHERE department <> ''), '{}'), count(*) FROM people WHERE org_id = $1 AND status <> 'terminated'`, orgID).Scan(&in.Departments, &in.Headcount)
	collect := func(q string, dst *[]string) error {
		rows, err := s.Pool.Query(ctx, q, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				return err
			}
			*dst = append(*dst, v)
		}
		return rows.Err()
	}
	if err := collect(`SELECT f.name FROM org_frameworks o JOIN catalog_frameworks f ON f.id = o.framework_id WHERE o.org_id = $1 ORDER BY f.position`, &in.Frameworks); err != nil {
		return nil, "", err
	}
	if err := collect(`SELECT name FROM integrations WHERE org_id = $1 AND type <> 'trustline' ORDER BY name`, &in.Systems); err != nil {
		return nil, "", err
	}
	if err := collect(`SELECT name FROM vendors WHERE org_id = $1 AND status = 'active' AND (data_access IN ('confidential','restricted') OR handles_pii) ORDER BY name`, &in.Vendors); err != nil {
		return nil, "", err
	}
	if err := collect(`SELECT title FROM risks WHERE org_id = $1 AND status <> 'dismissed'`, &in.ExistingRisks); err != nil {
		return nil, "", err
	}
	if err := collect(`SELECT code FROM controls WHERE org_id = $1 ORDER BY code`, &in.ControlCodes); err != nil {
		return nil, "", err
	}
	rows, err := s.Pool.Query(ctx, `SELECT t.name, c.code, t.severity, t.last_summary FROM tests t JOIN controls c ON c.id = t.control_id
		WHERE t.org_id = $1 AND t.enabled AND t.last_status IN ('fail','error') ORDER BY CASE t.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 ELSE 2 END LIMIT 40`, orgID)
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var f ai.FailingTestInfo
		if err := rows.Scan(&f.Name, &f.Control, &f.Severity, &f.Summary); err != nil {
			rows.Close()
			return nil, "", err
		}
		in.FailingTests = append(in.FailingTests, f)
	}
	rows.Close()
	res, model, err := svc.AssessRisks(ctx, orgID, in)
	if err != nil {
		if err == ai.ErrDisabled {
			return nil, "", domain.Invalid("ai", "AI features are disabled for this organization")
		}
		return nil, "", err
	}
	var created []domain.ID
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		for _, r := range res.Risks {
			id := domain.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO risks (id, org_id, title, description, category, likelihood, impact, treatment, treatment_plan, status, source, ai_rationale, created_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'suggested','ai',$10,$11)`, id, orgID, r.Title, r.Description, r.Category, r.Likelihood, r.Impact, r.Treatment, r.TreatmentPlan, r.Rationale, actor.ID); err != nil {
				return err
			}
			for _, code := range r.Controls {
				if _, err := tx.Exec(ctx, `INSERT INTO risk_controls (org_id, risk_id, control_id) SELECT $1, $2, id FROM controls WHERE org_id = $1 AND code = $3 ON CONFLICT DO NOTHING`, orgID, id, code); err != nil {
					return err
				}
			}
			created = append(created, id)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO risk_assessments (id, org_id, kind, summary, suggested, model, created_by) VALUES ($1,$2,'ai',$3,$4,$5,$6)`,
			domain.NewID(), orgID, res.Summary, len(created), model, actor.ID); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.ai_assessment", TargetType: "risk_assessment", TargetLabel: res.Summary, Metadata: map[string]any{"suggested": len(created), "model": model}}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, "", err
	}
	all, err := s.ListRisks(ctx, orgID, RiskFilter{Status: "suggested"})
	if err != nil {
		return nil, "", err
	}
	var out []Risk
	for _, r := range all {
		for _, id := range created {
			if r.ID == id {
				out = append(out, r)
			}
		}
	}
	return out, res.Summary, nil
}

// RecordManualAssessment marks a completed (non-AI) risk assessment.
func (s *Service) RecordManualAssessment(ctx context.Context, orgID domain.ID, summary string, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO risk_assessments (id, org_id, kind, summary, created_by) VALUES ($1,$2,'manual',$3,$4)`, domain.NewID(), orgID, summary, actor.ID); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "risk.assessment_recorded", TargetType: "risk_assessment", TargetLabel: summary}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

type RiskAssessmentRow struct {
	ID        domain.ID `json:"id"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Suggested int       `json:"suggested"`
	Model     string    `json:"model"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) ListRiskAssessments(ctx context.Context, orgID domain.ID) ([]RiskAssessmentRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.id, a.kind, a.summary, a.suggested, a.model, COALESCE(u.email,''), a.created_at FROM risk_assessments a
		LEFT JOIN users u ON u.id = a.created_by WHERE a.org_id = $1 ORDER BY a.created_at DESC LIMIT 20`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RiskAssessmentRow{}
	for rows.Next() {
		var r RiskAssessmentRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.Summary, &r.Suggested, &r.Model, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
