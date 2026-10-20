// Package ai is Trustline's single entry point to language models. It owns the
// Provider interface (Anthropic + deterministic fake), versioned prompt
// templates, structured-output parsing, PII redaction, per-org opt-out and
// token/cost logging. Phase 1 uses it for remediation plans and evidence
// validation; Phase 5 adds the assistant and agent tools on the same package.
package ai

import (
	"context"
	"errors"
)

// Attachment is a file passed to the model.
type Attachment struct {
	Filename  string
	MediaType string
	Data      []byte
}

// Request is a single structured generation.
type Request struct {
	Task        string         // e.g. "remediation_plan"; selects fake behaviour and is logged
	PromptID    string         // template name + version, e.g. "remediation_plan.v1"
	System      string         // rendered system prompt
	Prompt      string         // rendered user prompt
	Attachments []Attachment   // images, PDFs, text files
	Schema      map[string]any // JSON schema the response must match
	Input       any            // structured input (the fake works from this)
	MaxTokens   int
}

// Response carries raw JSON that matches Request.Schema plus usage.
type Response struct {
	JSON         []byte
	Model        string
	InputTokens  int64
	OutputTokens int64
}

// Provider is implemented by the Anthropic client and the fake.
type Provider interface {
	Name() string
	Generate(ctx context.Context, req Request) (*Response, error)
}

// ErrDisabled is returned when the org has opted out of AI features.
var ErrDisabled = errors.New("AI features are disabled for this organization")
