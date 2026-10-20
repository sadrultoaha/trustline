package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic calls the Claude Messages API with structured outputs.
type Anthropic struct {
	client anthropic.Client
	model  string
}

func NewAnthropic(apiKey, model string) *Anthropic {
	opts := []option.RequestOption{option.WithMaxRetries(3), option.WithRequestTimeout(120 * time.Second)}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &Anthropic{client: anthropic.NewClient(opts...), model: model}
}

func (a *Anthropic) Name() string { return "anthropic:" + a.model }

func (a *Anthropic) Generate(ctx context.Context, req Request) (*Response, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, att := range req.Attachments {
		switch {
		case strings.HasPrefix(att.MediaType, "image/"):
			blocks = append(blocks, anthropic.NewImageBlockBase64(att.MediaType, base64.StdEncoding.EncodeToString(att.Data)))
		case att.MediaType == "application/pdf":
			blocks = append(blocks, anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(att.Data)}))
		default:
			blocks = append(blocks, anthropic.NewDocumentBlock(anthropic.PlainTextSourceParam{Data: string(att.Data)}))
		}
	}
	blocks = append(blocks, anthropic.NewTextBlock(req.Prompt))
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 16000
	}
	adaptive := anthropic.ThinkingConfigAdaptiveParam{}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(maxTokens),
		System:    []anthropic.TextBlockParam{{Text: req.System}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: req.Schema},
		},
	}
	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return nil, fmt.Errorf("anthropic %d: %w", apiErr.StatusCode, err)
		}
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("anthropic: request declined (%s)", msg.StopDetails.Category)
	}
	if msg.StopReason == anthropic.StopReasonMaxTokens {
		return nil, errors.New("anthropic: response truncated at max_tokens")
	}
	var text strings.Builder
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	return &Response{JSON: []byte(text.String()), Model: string(msg.Model), InputTokens: msg.Usage.InputTokens, OutputTokens: msg.Usage.OutputTokens}, nil
}
