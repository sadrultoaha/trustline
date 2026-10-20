// Package connectors defines the integration interface, the registry of
// supported connector types and their credential schemas, and the sandbox
// (fake) implementations whose state the user and tests can manipulate.
package connectors

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Snapshot is the normalized output of a sync: resource type → resources.
type Snapshot struct {
	Connector string                      `json:"connector"`
	TakenAt   time.Time                   `json:"taken_at"`
	Resources map[string][]map[string]any `json:"resources"`
}

// Count returns the total number of resources.
func (s *Snapshot) Count() int {
	n := 0
	for _, r := range s.Resources {
		n += len(r)
	}
	return n
}

// Connector is implemented by every integration (real and sandbox).
type Connector interface {
	Sync(ctx context.Context) (*Snapshot, error)
}

// Mode selects the implementation.
type Mode string

const (
	ModeSandbox  Mode = "sandbox"  // fake implementation over controllable state
	ModeLive     Mode = "live"     // real API client
	ModePlatform Mode = "platform" // built-in Trustline data (connector "trustline")
)

// CredentialField describes one input on the connect form.
type CredentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
	Pattern     string `json:"pattern,omitempty"`
	Help        string `json:"help,omitempty"`
}

// Type describes a connector type.
type Type struct {
	Type        string            `json:"type"`
	Credentials []CredentialField `json:"credentials"`
	LiveReady   bool              `json:"live_ready"` // a real client is available in this build
	Scenarios   []ScenarioInfo    `json:"scenarios"`
}

// Types is the registry of supported connector types.
var Types = map[string]Type{
	"aws": {Type: "aws", Credentials: []CredentialField{
		{Key: "role_arn", Label: "IAM role ARN", Placeholder: "arn:aws:iam::123456789012:role/TrustlineAudit", Required: true, Pattern: `^arn:aws:iam::\d{12}:role/[\w+=,.@/-]+$`, Help: "Read-only role with the SecurityAudit managed policy, trusting Trustline's account."},
		{Key: "external_id", Label: "External ID", Required: true, Secret: true, Help: "The external ID configured on the role's trust policy."},
	}},
	"gcp": {Type: "gcp", Credentials: []CredentialField{
		{Key: "project_id", Label: "Project ID", Placeholder: "acme-prod", Required: true, Pattern: `^[a-z][a-z0-9-]{4,28}[a-z0-9]$`},
		{Key: "service_account_json", Label: "Service account key (JSON)", Required: true, Secret: true, Help: "Key for a service account with roles/iam.securityReviewer and roles/cloudasset.viewer."},
	}},
	"github": {Type: "github", Credentials: []CredentialField{
		{Key: "org", Label: "Organization", Placeholder: "acme", Required: true, Pattern: `^[A-Za-z0-9-]+$`},
		{Key: "token", Label: "Access token", Required: true, Secret: true, Help: "Fine-grained token with read access to administration, members and security events."},
	}},
	"okta": {Type: "okta", Credentials: []CredentialField{
		{Key: "domain", Label: "Okta domain", Placeholder: "acme.okta.com", Required: true, Pattern: `^[a-z0-9-]+\.(okta|oktapreview|okta-emea)\.com$`},
		{Key: "api_token", Label: "API token", Required: true, Secret: true, Help: "Token of a read-only admin."},
	}},
	"google_workspace": {Type: "google_workspace", Credentials: []CredentialField{
		{Key: "domain", Label: "Primary domain", Placeholder: "acme.com", Required: true},
		{Key: "admin_email", Label: "Admin email to impersonate", Required: true},
		{Key: "service_account_json", Label: "Service account key (JSON)", Required: true, Secret: true, Help: "Domain-wide delegation with read-only directory and reports scopes."},
	}},
	// trustline is the built-in platform connector: its snapshot is built from
	// Trustline's own records by the compliance service, never by Build.
	"trustline": {Type: "trustline", Credentials: []CredentialField{}},
	"hris": {Type: "hris", Credentials: []CredentialField{
		{Key: "subdomain", Label: "BambooHR subdomain", Placeholder: "acme", Required: true},
		{Key: "api_key", Label: "API key", Required: true, Secret: true},
	}},
}

func init() {
	for k, t := range Types {
		t.Scenarios = scenarioInfo(k)
		Types[k] = t
	}
}

// ValidateCredentials checks required fields and patterns.
func ValidateCredentials(typ string, creds map[string]string) map[string]string {
	t, ok := Types[typ]
	errs := map[string]string{}
	if !ok {
		errs["type"] = "unsupported integration type"
		return errs
	}
	for _, f := range t.Credentials {
		v := strings.TrimSpace(creds[f.Key])
		if v == "" {
			if f.Required {
				errs["credentials."+f.Key] = f.Label + " is required"
			}
			continue
		}
		if f.Pattern != "" && !regexp.MustCompile(f.Pattern).MatchString(v) {
			errs["credentials."+f.Key] = f.Label + " has an invalid format"
		}
	}
	return errs
}

// ErrLiveUnavailable is returned when live mode is requested for a type without a real client.
var ErrLiveUnavailable = errors.New("live connections for this integration type are not available in this build; use sandbox mode")

// Build returns the connector implementation for an integration.
func Build(typ string, mode Mode, creds map[string]string, state map[string]any) (Connector, error) {
	if _, ok := Types[typ]; !ok {
		return nil, fmt.Errorf("unknown connector type %q", typ)
	}
	switch mode {
	case ModeSandbox:
		return &Fake{Type: typ, State: state}, nil
	case ModeLive:
		return nil, ErrLiveUnavailable
	case ModePlatform:
		return nil, errors.New("platform snapshots are built by the compliance service")
	}
	return nil, fmt.Errorf("unknown mode %q", mode)
}
