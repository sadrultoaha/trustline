package ai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/pdftext"
)

type Citation struct {
	Page    int    `json:"page"`
	Section string `json:"section"`
	Quote   string `json:"quote"`
}

type TestedControl struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Result      string   `json:"result"` // no_exceptions | exceptions
	Citation    Citation `json:"citation"`
}

type Finding struct {
	Control            string   `json:"control"`
	Description        string   `json:"description"`
	ManagementResponse string   `json:"management_response"`
	Severity           string   `json:"severity"` // low | medium | high
	Citation           Citation `json:"citation"`
}

type Note struct {
	Description string   `json:"description"`
	Citation    Citation `json:"citation"`
}

// VendorAssessment is the structured review of vendor evidence.
type VendorAssessment struct {
	DocumentType       string          `json:"document_type"`
	Auditor            string          `json:"auditor"`
	Opinion            string          `json:"opinion"` // unqualified | qualified | adverse | disclaimer | not_applicable | unknown
	OpinionCitation    *Citation       `json:"opinion_citation"`
	PeriodStart        string          `json:"period_start"`
	PeriodEnd          string          `json:"period_end"`
	Summary            string          `json:"summary"`
	ControlsTested     []TestedControl `json:"controls_tested"`
	Exceptions         []Finding       `json:"exceptions"`
	CUECs              []Note          `json:"cuecs"`
	Subservice         []Note          `json:"subservice_organizations"`
	RiskSummary        string          `json:"risk_summary"`
	RecommendedTier    string          `json:"recommended_tier"`
	RecommendedTierWhy string          `json:"recommended_tier_reason"`
}

type VendorAssessmentInput struct {
	VendorName   string `json:"vendor_name"`
	DocumentKind string `json:"document_kind"`
	DataAccess   string `json:"data_access"`
	HandlesPII   bool   `json:"handles_pii"`
	Answers      string `json:"answers"` // questionnaire answers as text, when assessing a questionnaire
}

func citationSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"page": map[string]any{"type": "integer"}, "section": map[string]any{"type": "string"}, "quote": map[string]any{"type": "string"},
	}, "required": []string{"page", "section", "quote"}, "additionalProperties": false}
}

func noteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"description": map[string]any{"type": "string"}, "citation": citationSchema()},
		"required": []string{"description", "citation"}, "additionalProperties": false}
}

var vendorSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"document_type":    map[string]any{"type": "string"},
		"auditor":          map[string]any{"type": "string"},
		"opinion":          map[string]any{"type": "string", "enum": []string{"unqualified", "qualified", "adverse", "disclaimer", "not_applicable", "unknown"}},
		"opinion_citation": map[string]any{"anyOf": []any{citationSchema(), map[string]any{"type": "null"}}},
		"period_start":     map[string]any{"type": "string"},
		"period_end":       map[string]any{"type": "string"},
		"summary":          map[string]any{"type": "string"},
		"controls_tested": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"id": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
			"result": map[string]any{"type": "string", "enum": []string{"no_exceptions", "exceptions"}}, "citation": citationSchema(),
		}, "required": []string{"id", "description", "result", "citation"}, "additionalProperties": false}},
		"exceptions": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"control": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "management_response": map[string]any{"type": "string"},
			"severity": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}}, "citation": citationSchema(),
		}, "required": []string{"control", "description", "management_response", "severity", "citation"}, "additionalProperties": false}},
		"cuecs":                    map[string]any{"type": "array", "items": noteSchema()},
		"subservice_organizations": map[string]any{"type": "array", "items": noteSchema()},
		"risk_summary":             map[string]any{"type": "string"},
		"recommended_tier":         map[string]any{"type": "string", "enum": []string{"critical", "high", "medium", "low"}},
		"recommended_tier_reason":  map[string]any{"type": "string"},
	},
	"required": []string{"document_type", "auditor", "opinion", "opinion_citation", "period_start", "period_end", "summary", "controls_tested", "exceptions",
		"cuecs", "subservice_organizations", "risk_summary", "recommended_tier", "recommended_tier_reason"},
	"additionalProperties": false,
}

// AssessVendor reviews a vendor document (or questionnaire answers when att is nil).
func (s *Service) AssessVendor(ctx context.Context, orgID domain.ID, in VendorAssessmentInput, att *Attachment) (*VendorAssessment, string, error) {
	sys, usr, err := render("vendor_assessment.v1", in)
	if err != nil {
		return nil, "", err
	}
	req := Request{Task: "vendor_assessment", PromptID: "vendor_assessment.v1", System: sys, Prompt: usr, Schema: vendorSchema, Input: in, MaxTokens: 16000}
	if att != nil {
		req.Attachments = []Attachment{*att}
	}
	var out VendorAssessment
	if err := s.run(ctx, orgID, req, &out); err != nil {
		return nil, "", err
	}
	return &out, s.Provider.Name(), nil
}

// --- fake: parse the document structure deterministically ----------------------

var (
	sectionRe    = regexp.MustCompile(`^Section (\d+)\s*[-–]?\s*(.+)$`)
	periodRe     = regexp.MustCompile(`(?i)(?:for the period|period)\s+([A-Z][a-z]+ \d{1,2}, \d{4})\s+(?:to|through)\s+([A-Z][a-z]+ \d{1,2}, \d{4})`)
	rowRe        = regexp.MustCompile(`^([A-Z]{1,2}\d+\.\d+)\s*\|\s*(.+?)\s*\|\s*(No exceptions noted|Exception noted|Exceptions noted)\s*$`)
	exceptionRe  = regexp.MustCompile(`^Exception ([A-Z]{1,2}\d+\.\d+):\s*(.+)$`)
	responseRe   = regexp.MustCompile(`^Response ([A-Z]{1,2}\d+\.\d+):\s*(.+)$`)
	cuecRe       = regexp.MustCompile(`^(CUEC-\d+):\s*(.+)$`)
	subRe        = regexp.MustCompile(`^Subservice organization:\s*(.+)$`)
	auditorRe    = regexp.MustCompile(`(?i)^Prepared by:\s*(.+)$`)
	pageFooterRe = regexp.MustCompile(`(?i)^page \d+( of \d+)?$`)
)

type docLine struct {
	page    int
	section string
	text    string
}

// joinContinuations merges wrapped paragraph lines into the preceding entry.
func joinContinuations(lines []docLine, starts *regexp.Regexp) []docLine {
	var out []docLine
	for _, l := range lines {
		if len(out) > 0 && !starts.MatchString(l.text) && out[len(out)-1].page == l.page && strings.HasPrefix(out[len(out)-1].text, "\x00") {
			out[len(out)-1].text += " " + l.text
			continue
		}
		if starts.MatchString(l.text) {
			l.text = "\x00" + l.text
		}
		out = append(out, l)
	}
	for i := range out {
		out[i].text = strings.TrimPrefix(out[i].text, "\x00")
	}
	return out
}

func fakeVendorAssessment(in VendorAssessmentInput, atts []Attachment) VendorAssessment {
	out := VendorAssessment{DocumentType: in.DocumentKind, Opinion: "unknown", ControlsTested: []TestedControl{}, Exceptions: []Finding{}, CUECs: []Note{}, Subservice: []Note{}}
	var lines []docLine
	if len(atts) > 0 && atts[0].MediaType == "application/pdf" {
		pages, err := pdftext.Extract(atts[0].Data)
		if err == nil {
			// Running headers repeat at the top of most pages; footers are "Page N".
			firsts := map[string]int{}
			for _, p := range pages {
				if len(p.Lines) > 0 {
					firsts[p.Lines[0]]++
				}
			}
			section := "Front matter"
			for _, p := range pages {
				for i, l := range p.Lines {
					if pageFooterRe.MatchString(l) || (i == 0 && firsts[l] >= 3) {
						continue
					}
					if i == 0 && strings.Contains(l, " - Section ") {
						continue
					}
					if m := sectionRe.FindStringSubmatch(l); m != nil && !strings.Contains(l, "....") {
						section = "Section " + m[1] + " – " + strings.TrimSpace(m[2])
					}
					lines = append(lines, docLine{p.Number, section, l})
				}
			}
		}
	} else if in.Answers != "" {
		for _, l := range strings.Split(in.Answers, "\n") {
			lines = append(lines, docLine{1, "Questionnaire", strings.TrimSpace(l)})
		}
	} else if len(atts) > 0 {
		for _, l := range strings.Split(string(atts[0].Data), "\n") {
			lines = append(lines, docLine{1, "Document", strings.TrimSpace(l)})
		}
	}
	cite := func(l docLine) Citation {
		return Citation{Page: l.page, Section: l.section, Quote: truncateQuote(l.text)}
	}
	starts := regexp.MustCompile(`^(Exception |Response |CUEC-|Subservice organization:|[A-Z]{1,2}\d+\.\d+ \||Section \d)`)
	merged := joinContinuations(lines, starts)
	responses := map[string]string{}
	for _, l := range merged {
		if m := responseRe.FindStringSubmatch(l.text); m != nil {
			responses[m[1]] = m[2]
		}
	}
	for _, l := range merged {
		t := l.text
		switch {
		case auditorRe.MatchString(t) && out.Auditor == "":
			out.Auditor = auditorRe.FindStringSubmatch(t)[1]
		case periodRe.MatchString(t) && out.PeriodStart == "":
			m := periodRe.FindStringSubmatch(t)
			out.PeriodStart, out.PeriodEnd = m[1], m[2]
		case strings.Contains(strings.ToLower(t), "unqualified opinion"):
			out.Opinion = "unqualified"
			c := cite(l)
			out.OpinionCitation = &c
		case strings.Contains(strings.ToLower(t), "qualified opinion") && out.Opinion == "unknown":
			out.Opinion = "qualified"
			c := cite(l)
			out.OpinionCitation = &c
		case rowRe.MatchString(t):
			m := rowRe.FindStringSubmatch(t)
			res := "no_exceptions"
			if !strings.HasPrefix(m[3], "No") {
				res = "exceptions"
			}
			out.ControlsTested = append(out.ControlsTested, TestedControl{ID: m[1], Description: m[2], Result: res, Citation: cite(l)})
		case exceptionRe.MatchString(t):
			m := exceptionRe.FindStringSubmatch(t)
			sev := "medium"
			if strings.HasPrefix(m[1], "CC6") || strings.Contains(strings.ToLower(m[2]), "critical") {
				sev = "high"
			}
			out.Exceptions = append(out.Exceptions, Finding{Control: m[1], Description: m[2], ManagementResponse: responses[m[1]], Severity: sev, Citation: cite(l)})
		case cuecRe.MatchString(t):
			out.CUECs = append(out.CUECs, Note{Description: t, Citation: cite(l)})
		case subRe.MatchString(t):
			out.Subservice = append(out.Subservice, Note{Description: subRe.FindStringSubmatch(t)[1], Citation: cite(l)})
		}
	}
	sort.SliceStable(out.Exceptions, func(i, j int) bool { return out.Exceptions[i].Control < out.Exceptions[j].Control })
	switch {
	case len(lines) == 0:
		out.Summary = "The document could not be read; upload a text-based PDF."
	case len(out.ControlsTested) > 0:
		passed := 0
		for _, c := range out.ControlsTested {
			if c.Result == "no_exceptions" {
				passed++
			}
		}
		out.Summary = fmt.Sprintf("%s report for %s covering %s to %s with a%s opinion. %d of %d controls tested without exceptions; %d exception(s) noted.",
			strings.ToUpper(strings.ReplaceAll(in.DocumentKind, "_", " ")), in.VendorName, orUnknown(out.PeriodStart), orUnknown(out.PeriodEnd), article(out.Opinion), passed, len(out.ControlsTested), len(out.Exceptions))
	default:
		out.Opinion = "not_applicable"
		out.Summary = fmt.Sprintf("%s for %s reviewed; no audited control results were found in the document.", in.DocumentKind, in.VendorName)
	}
	high := 0
	for _, e := range out.Exceptions {
		if e.Severity == "high" {
			high++
		}
	}
	risk := "Low residual risk: controls operated effectively."
	if len(out.Exceptions) > 0 {
		risk = fmt.Sprintf("Moderate residual risk: %d exception(s), %d affecting access or security monitoring. Management has described remediation; confirm it in next year's report.", len(out.Exceptions), high)
	}
	if out.Opinion == "qualified" || out.Opinion == "adverse" {
		risk = "Elevated risk: the auditor modified the opinion. Escalate before relying on this vendor."
	}
	if len(out.CUECs) > 0 {
		risk += fmt.Sprintf(" %d complementary user entity control(s) must be operated by your organization.", len(out.CUECs))
	}
	if in.Answers != "" {
		risk, out.Summary = questionnaireRisk(in.Answers, in.VendorName)
	}
	out.RiskSummary = risk
	out.RecommendedTier, out.RecommendedTierWhy = recommendTier(in.DataAccess, in.HandlesPII, len(out.Exceptions))
	return out
}

func questionnaireRisk(answers, vendor string) (string, string) {
	no := 0
	var gaps []string
	for _, l := range strings.Split(answers, "\n") {
		if strings.Contains(l, ": No") || strings.HasSuffix(strings.TrimSpace(l), "No") {
			no++
			gaps = append(gaps, strings.TrimSpace(strings.SplitN(l, ":", 2)[0]))
		}
	}
	summary := fmt.Sprintf("Questionnaire answers from %s reviewed.", vendor)
	if no == 0 {
		return "Low risk: the vendor reports all baseline security practices in place.", summary
	}
	return fmt.Sprintf("Gaps reported in %d area(s): %s. Follow up before approving.", no, strings.Join(gaps, "; ")), summary
}

func recommendTier(access string, pii bool, exceptions int) (string, string) {
	switch {
	case access == "restricted" || (access == "confidential" && pii && exceptions > 0):
		return "critical", "Processes restricted or personal data with reported exceptions; review at least annually with full evidence."
	case access == "confidential" || pii:
		return "high", "Has access to confidential or personal data; annual SOC 2 review and DPA required."
	case access == "internal":
		return "medium", "Accesses internal data only; review every two years."
	}
	return "low", "No access to sensitive data."
}

func truncateQuote(s string) string {
	if len(s) > 220 {
		return s[:217] + "..."
	}
	return s
}

func orUnknown(s string) string {
	if s == "" {
		return "an unstated period"
	}
	return s
}

func article(op string) string {
	if op == "unqualified" || op == "unknown" || op == "adverse" {
		return "n " + op
	}
	return " " + op
}
