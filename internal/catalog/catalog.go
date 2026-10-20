// Package catalog loads the built-in compliance content (controls, frameworks,
// crosswalk, connector resource schemas and automated tests) from embedded YAML
// and syncs it into global, read-only catalog tables.
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/trustline/trustline/internal/testengine"
	"gopkg.in/yaml.v3"
)

//go:embed data
var dataFS embed.FS

type Control struct {
	Code            string   `yaml:"code" json:"code"`
	Name            string   `yaml:"name" json:"name"`
	Domain          string   `yaml:"domain" json:"domain"`
	Description     string   `yaml:"description" json:"description"`
	Evidence        string   `yaml:"evidence" json:"evidence"` // automated | manual | hybrid
	EvidenceHint    string   `yaml:"evidence_hint" json:"evidence_hint"`
	Keywords        []string `yaml:"keywords" json:"keywords"`
	EvidenceTTLDays int      `yaml:"evidence_ttl_days" json:"evidence_ttl_days"`
}

type Requirement struct {
	Code        string   `yaml:"code" json:"code"`
	Section     string   `yaml:"section" json:"section"`
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description"`
	Controls    []string `yaml:"controls" json:"controls"`
}

type Framework struct {
	ID           string        `yaml:"id" json:"id"`
	Name         string        `yaml:"name" json:"name"`
	Version      string        `yaml:"version" json:"version"`
	Publisher    string        `yaml:"publisher" json:"publisher"`
	Description  string        `yaml:"description" json:"description"`
	Sections     []string      `yaml:"sections" json:"sections"`
	Requirements []Requirement `yaml:"requirements" json:"requirements"`
}

type Remediation struct {
	Summary string   `yaml:"summary" json:"summary"`
	Steps   []string `yaml:"steps" json:"steps"`
	IaCHint string   `yaml:"iac_hint" json:"iac_hint,omitempty"`
}

type Test struct {
	ID               string           `yaml:"id" json:"id"`
	Name             string           `yaml:"name" json:"name"`
	Description      string           `yaml:"description" json:"description"`
	Connector        string           `yaml:"connector" json:"connector"`
	Resource         string           `yaml:"resource" json:"resource"`
	Control          string           `yaml:"control" json:"control"`
	Severity         string           `yaml:"severity" json:"severity"`
	Quantifier       string           `yaml:"quantifier" json:"quantifier"`
	Min              *int             `yaml:"min" json:"min,omitempty"`
	Max              *int             `yaml:"max" json:"max,omitempty"`
	Where            any              `yaml:"where" json:"where,omitempty"`
	RequireResources bool             `yaml:"require_resources" json:"require_resources,omitempty"`
	Assert           any              `yaml:"assert" json:"assert"`
	Remediation      Remediation      `yaml:"remediation" json:"remediation"`
	Rule             *testengine.Rule `yaml:"-" json:"-"`
	RuleJSON         json.RawMessage  `yaml:"-" json:"rule"`
}

type Field struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type ResourceType struct {
	Type   string  `json:"type"`
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
}

type Connector struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Category    string         `json:"category"`
	Description string         `json:"description"`
	Internal    bool           `json:"internal"` // built-in platform connector, not user-connectable
	Resources   []ResourceType `json:"resources"`
}

// PolicyTemplate is one document of the policy library.
type PolicyTemplate struct {
	ID              string   `yaml:"id" json:"id"`
	Title           string   `yaml:"title" json:"title"`
	Category        string   `yaml:"category" json:"category"`
	Summary         string   `yaml:"summary" json:"summary"`
	Controls        []string `yaml:"controls" json:"controls"`
	ReviewCycleDays int      `yaml:"review_cycle_days" json:"review_cycle_days"`
	Markdown        string   `yaml:"-" json:"-"`
	Words           int      `yaml:"-" json:"words"`
}

type TrainingModule struct {
	Title string `yaml:"title" json:"title"`
	Body  string `yaml:"body" json:"body"`
}

type TrainingQuestion struct {
	ID          string   `yaml:"id" json:"id"`
	Question    string   `yaml:"question" json:"question"`
	Options     []string `yaml:"options" json:"options"`
	Answer      int      `yaml:"answer" json:"-"`
	Explanation string   `yaml:"explanation" json:"-"`
}

type QuestionnaireQuestion struct {
	ID       string   `yaml:"id" json:"id"`
	Section  string   `yaml:"section" json:"section"`
	Type     string   `yaml:"type" json:"type"` // yes_no | text | choice
	Options  []string `yaml:"options" json:"options,omitempty"`
	Question string   `yaml:"question" json:"question"`
}

// QuestionnaireTemplate is a questionnaire sent to vendors.
type QuestionnaireTemplate struct {
	ID          string                  `yaml:"id" json:"id"`
	Title       string                  `yaml:"title" json:"title"`
	Description string                  `yaml:"description" json:"description"`
	Questions   []QuestionnaireQuestion `yaml:"questions" json:"questions"`
}

// TrainingCourse is the built-in security awareness course.
type TrainingCourse struct {
	ID           string             `yaml:"id" json:"id"`
	Title        string             `yaml:"title" json:"title"`
	Description  string             `yaml:"description" json:"description"`
	PassingScore int                `yaml:"passing_score" json:"passing_score"`
	ValidDays    int                `yaml:"valid_days" json:"valid_days"`
	Modules      []TrainingModule   `yaml:"modules" json:"modules"`
	Questions    []TrainingQuestion `yaml:"questions" json:"questions"`
}

// Catalog is the parsed, validated content.
type Catalog struct {
	Controls             []Control
	Frameworks           []Framework
	Tests                []Test
	Connectors           []Connector
	Policies             []PolicyTemplate
	Training             []TrainingCourse
	VendorQuestionnaires []QuestionnaireTemplate

	controlByCode map[string]*Control
	fieldIndex    map[string]map[string]map[string]Field // connector → resource → path → field
}

var (
	loadOnce sync.Once
	loaded   *Catalog
	loadErr  error
)

// Load parses the embedded catalog once.
func Load() (*Catalog, error) {
	loadOnce.Do(func() { loaded, loadErr = parse(dataFS) })
	return loaded, loadErr
}

// MustLoad panics on an invalid catalog (it is compiled into the binary and covered by tests).
func MustLoad() *Catalog {
	c, err := Load()
	if err != nil {
		panic(err)
	}
	return c
}

func parse(fsys fs.FS) (*Catalog, error) {
	c := &Catalog{controlByCode: map[string]*Control{}, fieldIndex: map[string]map[string]map[string]Field{}}
	if err := readYAML(fsys, "data/controls.yaml", &c.Controls); err != nil {
		return nil, err
	}
	for i := range c.Controls {
		ctl := &c.Controls[i]
		if _, dup := c.controlByCode[ctl.Code]; dup {
			return nil, fmt.Errorf("controls: duplicate code %s", ctl.Code)
		}
		switch ctl.Evidence {
		case "automated", "manual", "hybrid":
		default:
			return nil, fmt.Errorf("controls: %s: invalid evidence mode %q", ctl.Code, ctl.Evidence)
		}
		c.controlByCode[ctl.Code] = ctl
	}

	// Resource schemas.
	var raw map[string]struct {
		Name        string `yaml:"name"`
		Category    string `yaml:"category"`
		Description string `yaml:"description"`
		Internal    bool   `yaml:"internal"`
		Resources   map[string]struct {
			Label  string    `yaml:"label"`
			Fields yaml.Node `yaml:"fields"`
		} `yaml:"resources"`
	}
	if err := readYAML(fsys, "data/resources.yaml", &raw); err != nil {
		return nil, err
	}
	for typ, conn := range raw {
		cn := Connector{Type: typ, Name: conn.Name, Category: conn.Category, Description: conn.Description, Internal: conn.Internal}
		c.fieldIndex[typ] = map[string]map[string]Field{}
		for rtype, r := range conn.Resources {
			rt := ResourceType{Type: rtype, Label: r.Label}
			idx := map[string]Field{}
			// Preserve declaration order of fields.
			for i := 0; i+1 < len(r.Fields.Content); i += 2 {
				var f struct {
					Type  string `yaml:"type"`
					Label string `yaml:"label"`
				}
				if err := r.Fields.Content[i+1].Decode(&f); err != nil {
					return nil, fmt.Errorf("resources %s.%s: %w", typ, rtype, err)
				}
				fd := Field{Path: r.Fields.Content[i].Value, Type: f.Type, Label: f.Label}
				rt.Fields = append(rt.Fields, fd)
				idx[fd.Path] = fd
			}
			c.fieldIndex[typ][rtype] = idx
			cn.Resources = append(cn.Resources, rt)
		}
		sort.Slice(cn.Resources, func(i, j int) bool { return cn.Resources[i].Type < cn.Resources[j].Type })
		c.Connectors = append(c.Connectors, cn)
	}
	sort.Slice(c.Connectors, func(i, j int) bool {
		return connectorOrder(c.Connectors[i].Type) < connectorOrder(c.Connectors[j].Type)
	})

	// Frameworks.
	fwFiles, _ := fs.Glob(fsys, "data/frameworks/*.yaml")
	sort.Strings(fwFiles)
	for _, f := range fwFiles {
		var fw Framework
		if err := readYAML(fsys, f, &fw); err != nil {
			return nil, err
		}
		if fw.ID == "" || fw.Name == "" {
			return nil, fmt.Errorf("%s: id and name are required", f)
		}
		seen := map[string]bool{}
		for _, r := range fw.Requirements {
			if seen[r.Code] {
				return nil, fmt.Errorf("%s: duplicate requirement %s", f, r.Code)
			}
			seen[r.Code] = true
			if len(r.Controls) == 0 {
				return nil, fmt.Errorf("%s: %s maps to no controls", f, r.Code)
			}
			for _, cc := range r.Controls {
				if c.controlByCode[cc] == nil {
					return nil, fmt.Errorf("%s: %s maps to unknown control %s", f, r.Code, cc)
				}
			}
		}
		c.Frameworks = append(c.Frameworks, fw)
	}

	// Tests.
	testFiles, _ := fs.Glob(fsys, "data/tests/*.yaml")
	sort.Strings(testFiles)
	ids := map[string]bool{}
	for _, f := range testFiles {
		var tests []Test
		if err := readYAML(fsys, f, &tests); err != nil {
			return nil, err
		}
		for _, t := range tests {
			if ids[t.ID] {
				return nil, fmt.Errorf("%s: duplicate test id %s", f, t.ID)
			}
			ids[t.ID] = true
			if c.controlByCode[t.Control] == nil {
				return nil, fmt.Errorf("%s: %s: unknown control %s", f, t.ID, t.Control)
			}
			if _, ok := c.fieldIndex[t.Connector][t.Resource]; !ok {
				return nil, fmt.Errorf("%s: %s: unknown resource %s.%s", f, t.ID, t.Connector, t.Resource)
			}
			ruleSrc := map[string]any{"resource": t.Resource, "quantifier": t.Quantifier, "assert": t.Assert}
			if t.Where != nil {
				ruleSrc["where"] = t.Where
			}
			if t.Min != nil {
				ruleSrc["min"] = *t.Min
			}
			if t.Max != nil {
				ruleSrc["max"] = *t.Max
			}
			if t.RequireResources {
				ruleSrc["require_resources"] = true
			}
			rule, err := testengine.FromAny(ruleSrc)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", f, t.ID, err)
			}
			if err := rule.Validate(c.FieldChecker(t.Connector)); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", f, t.ID, err)
			}
			t.Rule = rule
			t.RuleJSON, _ = json.Marshal(rule)
			c.Tests = append(c.Tests, t)
		}
	}

	// Policy templates.
	policyFiles, _ := fs.Glob(fsys, "data/policies/*.md")
	sort.Strings(policyFiles)
	seenPolicy := map[string]bool{}
	for _, f := range policyFiles {
		if strings.HasSuffix(f, "README.md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		if !strings.HasPrefix(text, "---\n") {
			return nil, fmt.Errorf("%s: missing front matter", f)
		}
		end := strings.Index(text[4:], "\n---\n")
		if end < 0 {
			return nil, fmt.Errorf("%s: unterminated front matter", f)
		}
		var pt PolicyTemplate
		if err := yaml.Unmarshal([]byte(text[4:4+end]), &pt); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		pt.Markdown = strings.TrimSpace(text[4+end+5:])
		pt.Words = len(strings.Fields(pt.Markdown))
		if pt.ID == "" || pt.Title == "" || pt.Markdown == "" {
			return nil, fmt.Errorf("%s: id, title and body are required", f)
		}
		if seenPolicy[pt.ID] {
			return nil, fmt.Errorf("%s: duplicate policy id %s", f, pt.ID)
		}
		seenPolicy[pt.ID] = true
		if pt.ReviewCycleDays <= 0 {
			pt.ReviewCycleDays = 365
		}
		for _, cc := range pt.Controls {
			if c.controlByCode[cc] == nil {
				return nil, fmt.Errorf("%s: unknown control %s", f, cc)
			}
		}
		c.Policies = append(c.Policies, pt)
	}

	// Training courses.
	courseFiles, _ := fs.Glob(fsys, "data/training/*.yaml")
	for _, f := range courseFiles {
		var tc TrainingCourse
		if err := readYAML(fsys, f, &tc); err != nil {
			return nil, err
		}
		for _, q := range tc.Questions {
			if q.Answer < 0 || q.Answer >= len(q.Options) {
				return nil, fmt.Errorf("%s: question %s has no valid answer", f, q.ID)
			}
		}
		c.Training = append(c.Training, tc)
	}
	qFiles, _ := fs.Glob(fsys, "data/questionnaires/*.yaml")
	for _, f := range qFiles {
		var qt QuestionnaireTemplate
		if err := readYAML(fsys, f, &qt); err != nil {
			return nil, err
		}
		for _, q := range qt.Questions {
			switch q.Type {
			case "yes_no", "text":
			case "choice":
				if len(q.Options) < 2 {
					return nil, fmt.Errorf("%s: %s needs options", f, q.ID)
				}
			default:
				return nil, fmt.Errorf("%s: %s has unknown type %q", f, q.ID, q.Type)
			}
		}
		c.VendorQuestionnaires = append(c.VendorQuestionnaires, qt)
	}
	return c, nil
}

func (c *Catalog) PolicyTemplate(id string) *PolicyTemplate {
	for i := range c.Policies {
		if c.Policies[i].ID == id {
			return &c.Policies[i]
		}
	}
	return nil
}

func (c *Catalog) Course(id string) *TrainingCourse {
	for i := range c.Training {
		if c.Training[i].ID == id {
			return &c.Training[i]
		}
	}
	return nil
}

func connectorOrder(t string) string {
	order := map[string]string{"aws": "0", "gcp": "1", "github": "2", "okta": "3", "google_workspace": "4", "hris": "5", "trustline": "8"}
	if o, ok := order[t]; ok {
		return o
	}
	return "9" + t
}

func readYAML(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", path.Base(name), err)
	}
	return nil
}

// FieldChecker validates field paths for a connector's resources. Paths with a
// [*] segment are accepted if declared; parent object paths of declared nested
// fields are also accepted.
func (c *Catalog) FieldChecker(connector string) testengine.FieldChecker {
	return func(resource, field string) bool {
		idx := c.fieldIndex[connector][resource]
		if idx == nil {
			return false
		}
		if _, ok := idx[field]; ok {
			return true
		}
		for p := range idx {
			if strings.HasPrefix(p, field+".") || strings.HasPrefix(p, field+"[*].") {
				return true
			}
		}
		return false
	}
}

func (c *Catalog) Control(code string) *Control { return c.controlByCode[code] }

func (c *Catalog) Framework(id string) *Framework {
	for i := range c.Frameworks {
		if c.Frameworks[i].ID == id {
			return &c.Frameworks[i]
		}
	}
	return nil
}

func (c *Catalog) ConnectorDef(typ string) *Connector {
	for i := range c.Connectors {
		if c.Connectors[i].Type == typ {
			return &c.Connectors[i]
		}
	}
	return nil
}

func (c *Catalog) Test(id string) *Test {
	for i := range c.Tests {
		if c.Tests[i].ID == id {
			return &c.Tests[i]
		}
	}
	return nil
}

// ControlsForFrameworks returns the distinct control codes mapped by the given frameworks.
func (c *Catalog) ControlsForFrameworks(ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		if fw := c.Framework(id); fw != nil {
			for _, r := range fw.Requirements {
				for _, cc := range r.Controls {
					set[cc] = true
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
