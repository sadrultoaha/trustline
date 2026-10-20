package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/trustline/trustline/internal/domain"
)

type FailingTestInfo struct {
	Name     string `json:"name"`
	Control  string `json:"control"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

type RiskAssessmentInput struct {
	OrgName       string            `json:"org_name"`
	Headcount     int               `json:"headcount"`
	Departments   []string          `json:"departments"`
	Frameworks    []string          `json:"frameworks"`
	Systems       []string          `json:"systems"`
	Vendors       []string          `json:"vendors"`
	FailingTests  []FailingTestInfo `json:"failing_tests"`
	ExistingRisks []string          `json:"existing_risks"`
	ControlCodes  []string          `json:"control_codes"`
}

type SuggestedRisk struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Likelihood    int      `json:"likelihood"`
	Impact        int      `json:"impact"`
	Treatment     string   `json:"treatment"`
	TreatmentPlan string   `json:"treatment_plan"`
	Controls      []string `json:"controls"`
	Rationale     string   `json:"rationale"`
}

type RiskAssessment struct {
	Summary string          `json:"summary"`
	Risks   []SuggestedRisk `json:"risks"`
}

var riskSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{"type": "string"},
		"risks": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
			"category":   map[string]any{"type": "string", "enum": []string{"Security", "Privacy", "Operational", "Third party", "Compliance", "People", "AI"}},
			"likelihood": map[string]any{"type": "integer", "minimum": 1, "maximum": 5}, "impact": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
			"treatment":      map[string]any{"type": "string", "enum": []string{"mitigate", "accept", "transfer", "avoid"}},
			"treatment_plan": map[string]any{"type": "string"}, "controls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"rationale": map[string]any{"type": "string"},
		}, "required": []string{"title", "description", "category", "likelihood", "impact", "treatment", "treatment_plan", "controls", "rationale"}, "additionalProperties": false}},
	},
	"required":             []string{"summary", "risks"},
	"additionalProperties": false,
}

// AssessRisks drafts a risk assessment from the org's profile and gaps.
func (s *Service) AssessRisks(ctx context.Context, orgID domain.ID, in RiskAssessmentInput) (*RiskAssessment, string, error) {
	sys, usr, err := render("risk_assessment.v1", in)
	if err != nil {
		return nil, "", err
	}
	var out RiskAssessment
	if err := s.run(ctx, orgID, Request{Task: "risk_assessment", PromptID: "risk_assessment.v1", System: sys, Prompt: usr, Schema: riskSchema, Input: in, MaxTokens: 16000}, &out); err != nil {
		return nil, "", err
	}
	valid := map[string]bool{}
	for _, c := range in.ControlCodes {
		valid[c] = true
	}
	for i := range out.Risks {
		r := &out.Risks[i]
		r.Likelihood, r.Impact = clamp(r.Likelihood), clamp(r.Impact)
		var keep []string
		for _, c := range r.Controls {
			if valid[c] {
				keep = append(keep, c)
			}
		}
		if keep == nil {
			keep = []string{}
		}
		r.Controls = keep
	}
	return &out, s.Provider.Name(), nil
}

func clamp(n int) int {
	if n < 1 {
		return 1
	}
	if n > 5 {
		return 5
	}
	return n
}

// riskTemplates maps a failing control's domain to a risk scenario (fake).
var riskTemplates = map[string]SuggestedRisk{
	"IAM": {Title: "Account takeover through weak authentication", Category: "Security", Likelihood: 4, Impact: 4, Treatment: "mitigate",
		Description:   "Accounts without enforced MFA or with stale credentials can be taken over by phishing or credential stuffing, exposing production systems and customer data.",
		TreatmentPlan: "Enforce MFA for every workforce identity, disable inactive accounts after 90 days and rotate long-lived keys."},
	"DAT": {Title: "Exposure of customer data through misconfigured storage", Category: "Privacy", Likelihood: 3, Impact: 5, Treatment: "mitigate",
		Description:   "Publicly accessible or unencrypted storage could leak customer data, triggering breach notification and contractual penalties.",
		TreatmentPlan: "Block public access account-wide, enforce encryption by default and alert on any public bucket."},
	"LOG": {Title: "Undetected intrusion due to monitoring gaps", Category: "Security", Likelihood: 3, Impact: 4, Treatment: "mitigate",
		Description:   "Gaps in audit logging or threat detection delay the detection of an attacker, increasing the blast radius of an incident.",
		TreatmentPlan: "Enable organization-wide audit logging and threat detection, and route high-severity findings to on-call."},
	"BCP": {Title: "Extended outage of the production database", Category: "Operational", Likelihood: 2, Impact: 5, Treatment: "mitigate",
		Description:   "A single-zone database failure would cause a prolonged outage and could breach availability commitments.",
		TreatmentPlan: "Run production databases Multi-AZ, test failover, and document recovery objectives."},
	"VUL": {Title: "Exploitation of known vulnerabilities", Category: "Security", Likelihood: 4, Impact: 4, Treatment: "mitigate",
		Description:   "Unpatched critical vulnerabilities in dependencies or hosts give attackers a known path in.",
		TreatmentPlan: "Fix critical findings within 30 days and track overdue items weekly."},
	"HR": {Title: "Former employees retain access", Category: "People", Likelihood: 3, Impact: 4, Treatment: "mitigate",
		Description:   "Delayed offboarding leaves departed staff with access to systems and data.",
		TreatmentPlan: "Drive offboarding from the HR system and verify access removal in every connected system within one business day."},
	"AST": {Title: "Data loss from lost or stolen laptops", Category: "Security", Likelihood: 3, Impact: 3, Treatment: "mitigate",
		Description:   "An unencrypted or unlocked laptop exposes company and customer data if lost.",
		TreatmentPlan: "Enforce disk encryption and screen lock through the device agent and wipe lost devices."},
	"CHG": {Title: "Unreviewed changes reach production", Category: "Operational", Likelihood: 3, Impact: 4, Treatment: "mitigate",
		Description:   "Without enforced review, a defective or malicious change can reach customers.",
		TreatmentPlan: "Protect default branches, require approvals and passing checks before merge."},
}

func fakeRiskAssessment(in RiskAssessmentInput) RiskAssessment {
	existing := map[string]bool{}
	for _, t := range in.ExistingRisks {
		existing[strings.ToLower(t)] = true
	}
	byDomain := map[string][]FailingTestInfo{}
	for _, t := range in.FailingTests {
		d := strings.SplitN(t.Control, "-", 2)[0]
		byDomain[d] = append(byDomain[d], t)
	}
	domains := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	valid := map[string]bool{}
	for _, c := range in.ControlCodes {
		valid[c] = true
	}
	out := RiskAssessment{Risks: []SuggestedRisk{}}
	for _, d := range domains {
		tpl, ok := riskTemplates[d]
		if !ok || existing[strings.ToLower(tpl.Title)] {
			continue
		}
		r := tpl
		seen := map[string]bool{}
		var names []string
		for _, t := range byDomain[d] {
			if valid[t.Control] && !seen[t.Control] {
				r.Controls = append(r.Controls, t.Control)
				seen[t.Control] = true
			}
			names = append(names, t.Name)
		}
		if len(names) > 3 {
			names = names[:3]
		}
		r.Rationale = fmt.Sprintf("%d failing test(s) show this gap today: %s.", len(byDomain[d]), strings.Join(names, "; "))
		for _, t := range byDomain[d] {
			if t.Severity == "critical" && r.Likelihood < 5 {
				r.Likelihood++
				break
			}
		}
		out.Risks = append(out.Risks, r)
	}
	if len(in.Vendors) > 0 && !existing["third-party compromise of customer data"] {
		out.Risks = append(out.Risks, SuggestedRisk{Title: "Third-party compromise of customer data", Category: "Third party", Likelihood: 2, Impact: 4, Treatment: "transfer",
			Description:   fmt.Sprintf("Vendors with access to sensitive data (%s) could suffer a breach that exposes customer data.", strings.Join(in.Vendors, ", ")),
			TreatmentPlan: "Review SOC 2 reports annually, require DPAs and breach notification clauses, and maintain cyber insurance.",
			Controls:      filterValid([]string{"VEN-02", "VEN-03"}, valid), Rationale: "Vendors in the inventory process confidential data."})
	}
	if !existing["ransomware disrupts operations"] {
		out.Risks = append(out.Risks, SuggestedRisk{Title: "Ransomware disrupts operations", Category: "Operational", Likelihood: 2, Impact: 5, Treatment: "mitigate",
			Description:   "Ransomware on endpoints or servers could encrypt production data and halt operations.",
			TreatmentPlan: "Keep immutable, tested backups; enforce endpoint protection; rehearse the incident response plan.",
			Controls:      filterValid([]string{"DAT-03", "DAT-04", "AST-05", "IR-02"}, valid), Rationale: fmt.Sprintf("Baseline risk for a %d-person company running %s.", in.Headcount, strings.Join(in.Systems, ", "))})
	}
	out.Summary = fmt.Sprintf("Assessment of %s: %d risk(s) proposed from %d failing test(s), %d system(s) and %d sensitive vendor(s).", in.OrgName, len(out.Risks), len(in.FailingTests), len(in.Systems), len(in.Vendors))
	return out
}

func filterValid(codes []string, valid map[string]bool) []string {
	out := []string{}
	for _, c := range codes {
		if valid[c] {
			out = append(out, c)
		}
	}
	return out
}
