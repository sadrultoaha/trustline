// Package testengine evaluates declarative test rules against connector
// snapshots. The rule format is documented in docs/ARCHITECTURE.md §6 and
// internal/catalog/data/README.md; the UI's no-code builder emits the same JSON.
package testengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Rule is the compiled-from-JSON form of a test.
type Rule struct {
	Resource   string `json:"resource" yaml:"resource"`
	Quantifier string `json:"quantifier" yaml:"quantifier"`
	Min        *int   `json:"min,omitempty" yaml:"min,omitempty"`
	Max        *int   `json:"max,omitempty" yaml:"max,omitempty"`
	Where      *Expr  `json:"where,omitempty" yaml:"where,omitempty"`
	Assert     *Expr  `json:"assert" yaml:"assert"`
}

// Expr is a boolean expression: exactly one of And, Or, Not, or a condition.
type Expr struct {
	And   []*Expr `json:"and,omitempty" yaml:"and,omitempty"`
	Or    []*Expr `json:"or,omitempty" yaml:"or,omitempty"`
	Not   *Expr   `json:"not,omitempty" yaml:"not,omitempty"`
	Field string  `json:"field,omitempty" yaml:"field,omitempty"`
	Op    string  `json:"op,omitempty" yaml:"op,omitempty"`
	Value any     `json:"value,omitempty" yaml:"value,omitempty"`
	Match string  `json:"match,omitempty" yaml:"match,omitempty"` // "" (all) | "any"

	re *regexp.Regexp
}

var ops = map[string]bool{
	"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true,
	"in": true, "nin": true, "contains": true, "not_contains": true,
	"exists": true, "not_exists": true, "matches": true, "len_gt": true, "len_lt": true,
}

// Operators lists supported operators (for the builder UI).
func Operators() []string {
	return []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "nin", "contains", "not_contains", "exists", "not_exists", "matches", "len_gt", "len_lt"}
}

// FieldChecker reports whether a field path exists for the rule's resource.
type FieldChecker func(resource, field string) bool

// Parse decodes and validates a rule. fields may be nil to skip field checks.
func Parse(raw []byte, fields FieldChecker) (*Rule, error) {
	var r Rule
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("rule: %w", err)
	}
	if err := r.Validate(fields); err != nil {
		return nil, err
	}
	return &r, nil
}

// Validate checks structure, operators, regexes and (optionally) field paths.
func (r *Rule) Validate(fields FieldChecker) error {
	if r.Resource == "" {
		return errors.New("rule: resource is required")
	}
	switch r.Quantifier {
	case "all", "any", "none":
	case "count":
		if r.Min == nil && r.Max == nil {
			return errors.New("rule: count quantifier needs min and/or max")
		}
	default:
		return fmt.Errorf("rule: unknown quantifier %q", r.Quantifier)
	}
	if r.Assert == nil {
		return errors.New("rule: assert is required")
	}
	if r.Where != nil {
		if err := r.Where.validate(r.Resource, fields, "where"); err != nil {
			return err
		}
	}
	return r.Assert.validate(r.Resource, fields, "assert")
}

func (e *Expr) validate(resource string, fields FieldChecker, path string) error {
	if e == nil {
		return fmt.Errorf("rule: %s: empty expression", path)
	}
	kinds := 0
	if e.And != nil {
		kinds++
	}
	if e.Or != nil {
		kinds++
	}
	if e.Not != nil {
		kinds++
	}
	if e.Field != "" || e.Op != "" {
		kinds++
	}
	if kinds != 1 {
		return fmt.Errorf("rule: %s: expression must be exactly one of and/or/not/condition", path)
	}
	switch {
	case e.And != nil || e.Or != nil:
		list := e.And
		name := "and"
		if e.Or != nil {
			list, name = e.Or, "or"
		}
		if len(list) == 0 {
			return fmt.Errorf("rule: %s.%s: needs at least one expression", path, name)
		}
		for i, x := range list {
			if err := x.validate(resource, fields, fmt.Sprintf("%s.%s[%d]", path, name, i)); err != nil {
				return err
			}
		}
		return nil
	case e.Not != nil:
		return e.Not.validate(resource, fields, path+".not")
	}
	if e.Field == "" {
		return fmt.Errorf("rule: %s: field is required", path)
	}
	if !ops[e.Op] {
		return fmt.Errorf("rule: %s: unknown operator %q", path, e.Op)
	}
	if e.Match != "" && e.Match != "any" && e.Match != "all" {
		return fmt.Errorf("rule: %s: match must be any or all", path)
	}
	if fields != nil && !fields(resource, e.Field) {
		return fmt.Errorf("rule: %s: unknown field %q for resource %q", path, e.Field, resource)
	}
	switch e.Op {
	case "exists", "not_exists":
	case "in", "nin":
		if _, ok := e.Value.([]any); !ok {
			return fmt.Errorf("rule: %s: %s needs a list value", path, e.Op)
		}
	case "matches":
		s, ok := e.Value.(string)
		if !ok {
			return fmt.Errorf("rule: %s: matches needs a string regex", path)
		}
		re, err := regexp.Compile(s)
		if err != nil {
			return fmt.Errorf("rule: %s: invalid regex: %v", path, err)
		}
		e.re = re
	default:
		if e.Value == nil {
			return fmt.Errorf("rule: %s: value is required for %s", path, e.Op)
		}
	}
	return nil
}

// FromAny converts a YAML/JSON-decoded value (maps with any keys) to a Rule by
// round-tripping through JSON, normalising YAML integer types.
func FromAny(v any) (*Rule, error) {
	b, err := json.Marshal(normalize(v))
	if err != nil {
		return nil, err
	}
	var r Rule
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range t {
			out[k] = normalize(x)
		}
		return out
	case map[any]any:
		out := map[string]any{}
		for k, x := range t {
			out[fmt.Sprint(k)] = normalize(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = normalize(x)
		}
		return out
	}
	return v
}
