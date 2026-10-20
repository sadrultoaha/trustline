package ai

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/domain"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

var funcs = template.FuncMap{"join": strings.Join}

var prompts = template.Must(template.New("").Funcs(funcs).ParseFS(promptFS, "prompts/*.tmpl"))

// pricing per million tokens (input, output) for cost logging.
var pricing = map[string][2]float64{
	"claude-sonnet-4-6": {3, 15},
	"claude-opus-5-5":   {4, 20},
	"claude-sonnet-5-5": {2, 10},
	"fake-1":            {0, 0},
}

// Service wraps a provider with org opt-out, redaction, prompts and logging.
type Service struct {
	Provider Provider
	Pool     *pgxpool.Pool
	Log      *slog.Logger
}

func render(name string, data any) (string, string, error) {
	// Each template file defines "system" and "user"; templates are looked up
	// by file so versions can coexist.
	t := prompts.Lookup(name + ".tmpl")
	if t == nil {
		return "", "", fmt.Errorf("prompt %s not found", name)
	}
	clone, err := t.Clone()
	if err != nil {
		return "", "", err
	}
	if _, err := clone.ParseFS(promptFS, "prompts/"+name+".tmpl"); err != nil {
		return "", "", err
	}
	var sys, usr bytes.Buffer
	if err := clone.ExecuteTemplate(&sys, "system", data); err != nil {
		return "", "", err
	}
	if err := clone.ExecuteTemplate(&usr, "user", data); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(sys.String()), strings.TrimSpace(usr.String()), nil
}

// Enabled reports whether the org allows AI features.
func (s *Service) Enabled(ctx context.Context, orgID domain.ID) (bool, error) {
	var raw []byte
	if err := s.Pool.QueryRow(ctx, `SELECT settings FROM orgs WHERE id = $1`, orgID).Scan(&raw); err != nil {
		return false, err
	}
	st := domain.DefaultOrgSettings()
	_ = json.Unmarshal(raw, &st)
	return st.AIEnabled, nil
}

func (s *Service) run(ctx context.Context, orgID domain.ID, req Request, out any) error {
	return s.runRaw(ctx, orgID, req, out)
}

func (s *Service) runRaw(ctx context.Context, orgID domain.ID, req Request, out any) error {
	ok, err := s.Enabled(ctx, orgID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrDisabled
	}
	req.Prompt = Redact(req.Prompt)
	start := time.Now()
	resp, err := s.Provider.Generate(ctx, req)
	status := "ok"
	errMsg := ""
	if err == nil {
		if jerr := json.Unmarshal(resp.JSON, out); jerr != nil {
			err = fmt.Errorf("ai: response did not match schema: %w", jerr)
		}
	}
	if err != nil {
		status, errMsg = "error", err.Error()
	}
	var model string
	var in, outTok int64
	if resp != nil {
		model, in, outTok = resp.Model, resp.InputTokens, resp.OutputTokens
	}
	if model == "" {
		model = s.Provider.Name()
	}
	p := pricing[model]
	cost := float64(in)/1e6*p[0] + float64(outTok)/1e6*p[1]
	if _, lerr := s.Pool.Exec(ctx, `INSERT INTO ai_calls (id, org_id, task, prompt_id, provider, model, input_tokens, output_tokens, cost_usd, latency_ms, status, error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		domain.NewID(), orgID, req.Task, req.PromptID, s.Provider.Name(), model, in, outTok, cost, time.Since(start).Milliseconds(), status, errMsg); lerr != nil {
		s.Log.Warn("ai call log failed", "err", lerr)
	}
	return err
}

// RemediationPlan drafts fix steps and a Terraform snippet for a failing test.
func (s *Service) RemediationPlan(ctx context.Context, orgID domain.ID, in RemediationInput) (*RemediationPlan, string, error) {
	sys, usr, err := render("remediation_plan.v1", in)
	if err != nil {
		return nil, "", err
	}
	var plan RemediationPlan
	err = s.run(ctx, orgID, Request{Task: "remediation_plan", PromptID: "remediation_plan.v1", System: sys, Prompt: usr, Schema: remediationSchema, Input: in, MaxTokens: 8000}, &plan)
	if err != nil {
		return nil, "", err
	}
	return &plan, s.Provider.Name(), nil
}

// ValidateEvidence judges whether an uploaded file satisfies a control.
func (s *Service) ValidateEvidence(ctx context.Context, orgID domain.ID, in EvidenceInput, att *Attachment) (*EvidenceVerdict, string, error) {
	sys, usr, err := render("evidence_validation.v1", in)
	if err != nil {
		return nil, "", err
	}
	req := Request{Task: "evidence_validation", PromptID: "evidence_validation.v1", System: sys, Prompt: usr, Schema: evidenceSchema, Input: in, MaxTokens: 4000}
	if att != nil {
		req.Attachments = []Attachment{*att}
	}
	var v EvidenceVerdict
	if err := s.run(ctx, orgID, req, &v); err != nil {
		return nil, "", err
	}
	if v.Verdict != "pass" && v.Verdict != "fail" {
		return nil, "", fmt.Errorf("ai: invalid verdict %q", v.Verdict)
	}
	return &v, s.Provider.Name(), nil
}
