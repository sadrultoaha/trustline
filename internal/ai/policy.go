package ai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/trustline/trustline/internal/domain"
)

// PolicyDraftInput gives the model the org context for a policy draft.
type PolicyDraftInput struct {
	OrgName             string   `json:"org_name"`
	SecurityOfficer     string   `json:"security_officer"`
	Frameworks          []string `json:"frameworks"`
	Systems             []string `json:"systems"`
	Departments         []string `json:"departments"`
	Headcount           int      `json:"headcount"`
	AccessReviewDays    int      `json:"access_review_days"`
	PasswordMinLength   int      `json:"password_min_length"`
	AcceptanceThreshold int      `json:"acceptance_threshold"`
	EffectiveDate       string   `json:"effective_date"`
	Title               string   `json:"title"`
	Instructions        string   `json:"instructions"`
	TemplateMarkdown    string   `json:"template_markdown"` // placeholders already filled
}

type PolicyDraft struct {
	Markdown      string `json:"markdown"`
	ChangeSummary string `json:"change_summary"`
}

var policyDraftSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"markdown":       map[string]any{"type": "string"},
		"change_summary": map[string]any{"type": "string"},
	},
	"required":             []string{"markdown", "change_summary"},
	"additionalProperties": false,
}

// Fact is one configured setting the policy text is checked against.
type Fact struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type ConsistencyInput struct {
	Title string `json:"title"`
	Text  string `json:"text"` // plain text of the policy
	Facts []Fact `json:"facts"`
}

type Inconsistency struct {
	Setting      string `json:"setting"`
	SettingValue string `json:"setting_value"`
	PolicyValue  string `json:"policy_value"`
	Quote        string `json:"quote"`
	Explanation  string `json:"explanation"`
}

type ConsistencyReport struct {
	Issues []Inconsistency `json:"issues"`
}

var consistencySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"issues": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"setting":       map[string]any{"type": "string", "description": "The fact key"},
				"setting_value": map[string]any{"type": "string"},
				"policy_value":  map[string]any{"type": "string"},
				"quote":         map[string]any{"type": "string"},
				"explanation":   map[string]any{"type": "string"},
			},
			"required":             []string{"setting", "setting_value", "policy_value", "quote", "explanation"},
			"additionalProperties": false,
		}},
	},
	"required":             []string{"issues"},
	"additionalProperties": false,
}

// DraftPolicy produces a tailored draft from a template and org context.
func (s *Service) DraftPolicy(ctx context.Context, orgID domain.ID, in PolicyDraftInput) (*PolicyDraft, string, error) {
	sys, usr, err := render("policy_draft.v1", in)
	if err != nil {
		return nil, "", err
	}
	var out PolicyDraft
	if err := s.run(ctx, orgID, Request{Task: "policy_draft", PromptID: "policy_draft.v1", System: sys, Prompt: usr, Schema: policyDraftSchema, Input: in, MaxTokens: 16000}, &out); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(out.Markdown) == "" {
		return nil, "", fmt.Errorf("ai: empty policy draft")
	}
	return &out, s.Provider.Name(), nil
}

// CheckConsistency compares policy text with configured settings.
func (s *Service) CheckConsistency(ctx context.Context, orgID domain.ID, in ConsistencyInput) (*ConsistencyReport, string, error) {
	sys, usr, err := render("policy_consistency.v1", in)
	if err != nil {
		return nil, "", err
	}
	var out ConsistencyReport
	if err := s.run(ctx, orgID, Request{Task: "policy_consistency", PromptID: "policy_consistency.v1", System: sys, Prompt: usr, Schema: consistencySchema, Input: in, MaxTokens: 8000}, &out); err != nil {
		return nil, "", err
	}
	if out.Issues == nil {
		out.Issues = []Inconsistency{}
	}
	return &out, s.Provider.Name(), nil
}

// --- fake behaviour -----------------------------------------------------------

func fakePolicyDraft(in PolicyDraftInput) PolicyDraft {
	md := in.TemplateMarkdown
	var b strings.Builder
	b.WriteString("\n\n## Systems in scope\n\n")
	fmt.Fprintf(&b, "This policy applies to the systems %s uses to store or process company and customer data", in.OrgName)
	if len(in.Systems) > 0 {
		b.WriteString(", including:\n\n")
		for _, sys := range in.Systems {
			fmt.Fprintf(&b, "- %s\n", sys)
		}
	} else {
		b.WriteString(".\n")
	}
	if len(in.Frameworks) > 0 {
		fmt.Fprintf(&b, "\nIt supports %s's commitments under %s.\n", in.OrgName, strings.Join(in.Frameworks, ", "))
	}
	if in.Instructions != "" {
		fmt.Fprintf(&b, "\n## Additional requirements\n\n%s\n", in.Instructions)
	}
	// Insert the tailored section before the Review section when present.
	if i := strings.LastIndex(md, "\n## Review"); i >= 0 {
		md = md[:i] + b.String() + md[i:]
	} else {
		md += b.String()
	}
	return PolicyDraft{Markdown: md, ChangeSummary: fmt.Sprintf("Drafted %s for %s from the template, tailored to %d connected systems.", in.Title, in.OrgName, len(in.Systems))}
}

type numberRule struct {
	key   string
	re    *regexp.Regexp
	unit  string
	label string
}

// Patterns for the commitments the platform also configures. Each captures the number.
var numberRules = []numberRule{
	{"access_review_days", regexp.MustCompile(`(?i)(?:access|accounts?|permissions?|entitlements?)[^.]{0,120}?review(?:ed)?[^.]{0,60}?(?:every|each|at least every|within)\s+(\d{1,3})\s+days`), "days", "access review cadence"},
	{"access_review_days", regexp.MustCompile(`(?i)review(?:ed|s)?[^.]{0,40}?(?:access|accounts?|permissions?)[^.]{0,60}?(?:every|each|at least every)\s+(\d{1,3})\s+days`), "days", "access review cadence"},
	{"access_review_days", regexp.MustCompile(`(?i)(?:every|each|at least every)\s+(\d{1,3})\s+days[^.]{0,80}?access\s+review`), "days", "access review cadence"},
	{"password_min_length", regexp.MustCompile(`(?i)(?:at least|minimum(?: of)?|no fewer than)\s+(\d{1,3})\s+characters`), "characters", "minimum password length"},
	{"policy_acceptance_threshold", regexp.MustCompile(`(?i)(?:at least\s+)?(\d{1,3})\s*%[^.]{0,80}?acknowledg`), "%", "policy acknowledgement threshold"},
}

var sentenceSplit = regexp.MustCompile(`[^.!?\n]*[.!?]`)

func sentenceAround(text string, idx int) string {
	for _, loc := range sentenceSplit.FindAllStringIndex(text, -1) {
		if idx >= loc[0] && idx < loc[1] {
			return strings.TrimSpace(text[loc[0]:loc[1]])
		}
	}
	end := idx + 160
	if end > len(text) {
		end = len(text)
	}
	return strings.TrimSpace(text[idx:end])
}

func fakeConsistency(in ConsistencyInput) ConsistencyReport {
	facts := map[string]Fact{}
	for _, f := range in.Facts {
		facts[f.Key] = f
	}
	seen := map[string]bool{}
	out := ConsistencyReport{Issues: []Inconsistency{}}
	for _, r := range numberRules {
		f, ok := facts[r.key]
		if !ok {
			continue
		}
		want, err := strconv.Atoi(f.Value)
		if err != nil {
			continue
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(in.Text, -1) {
			got, _ := strconv.Atoi(in.Text[m[2]:m[3]])
			if got == want {
				continue
			}
			quote := sentenceAround(in.Text, m[2])
			if seen[r.key+quote] {
				continue
			}
			seen[r.key+quote] = true
			out.Issues = append(out.Issues, Inconsistency{
				Setting: r.key, SettingValue: f.Value, PolicyValue: strconv.Itoa(got), Quote: quote,
				Explanation: fmt.Sprintf("The policy states a %s of %d %s, but Trustline is configured with %s %s. Update the policy or the setting so they agree.", r.label, got, r.unit, f.Value, r.unit),
			})
		}
	}
	sort.SliceStable(out.Issues, func(i, j int) bool { return out.Issues[i].Setting < out.Issues[j].Setting })
	return out
}
