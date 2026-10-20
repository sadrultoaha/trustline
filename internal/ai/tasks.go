package ai

// RemediationInput is the structured input for a remediation plan.
type RemediationInput struct {
	ControlCode string    `json:"control_code"`
	ControlName string    `json:"control_name"`
	TestName    string    `json:"test_name"`
	Description string    `json:"description"`
	Connector   string    `json:"connector"`
	Resource    string    `json:"resource"`
	Severity    string    `json:"severity"`
	RuleJSON    string    `json:"rule_json"`
	Failing     []Failing `json:"failing"`
	Summary     string    `json:"summary"`
	Steps       []string  `json:"steps"`
	IaCHint     string    `json:"iac_hint"`
}

type Failing struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// RemediationPlan is the structured output.
type RemediationPlan struct {
	Summary   string   `json:"summary"`
	Steps     []string `json:"steps"`
	Terraform string   `json:"terraform"`
	Notes     string   `json:"notes"`
}

var remediationSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary":   map[string]any{"type": "string"},
		"steps":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"terraform": map[string]any{"type": "string", "description": "Terraform HCL fixing the failing resources, or empty string"},
		"notes":     map[string]any{"type": "string", "description": "Caveats, blast radius, or verification advice"},
	},
	"required":             []string{"summary", "steps", "terraform", "notes"},
	"additionalProperties": false,
}

// EvidenceInput is the structured input for evidence validation.
type EvidenceInput struct {
	ControlCode        string   `json:"control_code"`
	ControlName        string   `json:"control_name"`
	ControlDescription string   `json:"control_description"`
	EvidenceHint       string   `json:"evidence_hint"`
	Keywords           []string `json:"keywords"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Filename           string   `json:"filename"`
	Today              string   `json:"today"`
}

// EvidenceVerdict is the structured output.
type EvidenceVerdict struct {
	Verdict    string   `json:"verdict"` // pass | fail
	Confidence float64  `json:"confidence"`
	Reasoning  string   `json:"reasoning"`
	Missing    []string `json:"missing"`
}

var evidenceSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verdict":    map[string]any{"type": "string", "enum": []string{"pass", "fail"}},
		"confidence": map[string]any{"type": "number", "description": "0 to 1"},
		"reasoning":  map[string]any{"type": "string"},
		"missing":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "What the evidence lacks, empty when it passes"},
	},
	"required":             []string{"verdict", "confidence", "reasoning", "missing"},
	"additionalProperties": false,
}
