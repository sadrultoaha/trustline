package testengine

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Resource is one item from a snapshot (decoded JSON object).
type Resource = map[string]any

// Failure explains why one resource failed.
type Failure struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Result is the outcome of evaluating a rule.
type Result struct {
	Status    string    `json:"status"` // pass | fail
	Evaluated int       `json:"evaluated"`
	Matched   int       `json:"matched"` // resources satisfying assert
	Failing   []Failure `json:"failing"`
	Summary   string    `json:"summary"`
}

// Evaluate runs the rule over resources (already selected by resource type).
func (r *Rule) Evaluate(resources []Resource) Result {
	if r.Assert != nil && r.Assert.re == nil {
		// Ensure regexes are compiled even when the rule was not validated.
		_ = r.Validate(nil)
	}
	var scoped []Resource
	for _, res := range resources {
		if r.Where == nil || r.Where.eval(res).ok {
			scoped = append(scoped, res)
		}
	}
	out := Result{Evaluated: len(scoped), Failing: []Failure{}}
	var passing []Resource
	var failing []Failure
	for _, res := range scoped {
		ev := r.Assert.eval(res)
		if ev.ok {
			out.Matched++
			passing = append(passing, res)
		} else {
			failing = append(failing, Failure{ID: idOf(res), Reason: ev.reason})
		}
	}
	switch r.Quantifier {
	case "all":
		out.Failing = failing
		if len(failing) == 0 {
			out.Status = "pass"
			out.Summary = fmt.Sprintf("All %d resources pass", len(scoped))
		} else {
			out.Status = "fail"
			out.Summary = fmt.Sprintf("%d of %d resources fail", len(failing), len(scoped))
		}
	case "any":
		if out.Matched > 0 {
			out.Status = "pass"
			out.Summary = fmt.Sprintf("%d of %d resources satisfy the condition", out.Matched, len(scoped))
		} else {
			out.Status = "fail"
			out.Failing = failing
			if len(scoped) == 0 {
				out.Summary = "No resources found; at least one is required"
				out.Failing = []Failure{{ID: "-", Reason: "no matching resources exist"}}
			} else {
				out.Summary = fmt.Sprintf("None of %d resources satisfy the condition", len(scoped))
			}
		}
	case "none":
		if out.Matched == 0 {
			out.Status = "pass"
			out.Summary = fmt.Sprintf("No offending resources among %d", len(scoped))
		} else {
			out.Status = "fail"
			for _, p := range passing {
				out.Failing = append(out.Failing, Failure{ID: idOf(p), Reason: "matches a prohibited condition"})
			}
			out.Summary = fmt.Sprintf("%d of %d resources match a prohibited condition", out.Matched, len(scoped))
		}
	case "count":
		ok := true
		if r.Min != nil && out.Matched < *r.Min {
			ok = false
		}
		if r.Max != nil && out.Matched > *r.Max {
			ok = false
		}
		bounds := ""
		if r.Min != nil {
			bounds += fmt.Sprintf(" min %d", *r.Min)
		}
		if r.Max != nil {
			bounds += fmt.Sprintf(" max %d", *r.Max)
		}
		out.Summary = fmt.Sprintf("%d matching resources (expected%s)", out.Matched, bounds)
		if ok {
			out.Status = "pass"
		} else {
			out.Status = "fail"
			if r.Max != nil && out.Matched > *r.Max {
				for _, p := range passing {
					out.Failing = append(out.Failing, Failure{ID: idOf(p), Reason: "counted toward limit" + bounds})
				}
			} else {
				out.Failing = []Failure{{ID: "-", Reason: out.Summary}}
			}
		}
	}
	if out.Failing == nil {
		out.Failing = []Failure{}
	}
	sort.SliceStable(out.Failing, func(i, j int) bool { return out.Failing[i].ID < out.Failing[j].ID })
	return out
}

func idOf(r Resource) string {
	if v, ok := r["id"]; ok {
		return fmt.Sprint(v)
	}
	return "?"
}

type evalResult struct {
	ok     bool
	reason string
}

func (e *Expr) eval(res Resource) evalResult {
	switch {
	case e.And != nil:
		for _, x := range e.And {
			if r := x.eval(res); !r.ok {
				return r
			}
		}
		return evalResult{ok: true}
	case e.Or != nil:
		var reasons []string
		for _, x := range e.Or {
			r := x.eval(res)
			if r.ok {
				return r
			}
			reasons = append(reasons, r.reason)
		}
		return evalResult{reason: strings.Join(reasons, " and ")}
	case e.Not != nil:
		r := e.Not.eval(res)
		if r.ok {
			return evalResult{reason: "NOT (" + e.Not.describe() + ") was violated"}
		}
		return evalResult{ok: true}
	}
	return e.evalCond(res)
}

func (e *Expr) describe() string {
	switch {
	case e.And != nil:
		return "and(...)"
	case e.Or != nil:
		return "or(...)"
	case e.Not != nil:
		return "not(" + e.Not.describe() + ")"
	}
	return fmt.Sprintf("%s %s %v", e.Field, e.Op, fmtVal(e.Value))
}

func (e *Expr) evalCond(res Resource) evalResult {
	values, fanout := resolve(res, e.Field)
	if e.Op == "exists" || e.Op == "not_exists" {
		present := false
		for _, v := range values {
			if v != nil {
				present = true
			}
		}
		if (e.Op == "exists") == present {
			return evalResult{ok: true}
		}
		if present {
			return evalResult{reason: e.Field + " is set"}
		}
		return evalResult{reason: e.Field + " is missing"}
	}
	if !fanout {
		v := any(nil)
		if len(values) > 0 {
			v = values[0]
		}
		if compare(e, v) {
			return evalResult{ok: true}
		}
		return evalResult{reason: fmt.Sprintf("%s = %s (expected %s %s)", e.Field, fmtVal(v), opWord(e.Op), fmtVal(e.Value))}
	}
	if e.Match == "any" {
		for _, v := range values {
			if compare(e, v) {
				return evalResult{ok: true}
			}
		}
		return evalResult{reason: fmt.Sprintf("no %s %s %s", e.Field, opWord(e.Op), fmtVal(e.Value))}
	}
	for i, v := range values {
		if !compare(e, v) {
			return evalResult{reason: fmt.Sprintf("%s (item %d) = %s (expected %s %s)", e.Field, i, fmtVal(v), opWord(e.Op), fmtVal(e.Value))}
		}
	}
	return evalResult{ok: true}
}

func opWord(op string) string {
	switch op {
	case "eq":
		return "="
	case "neq":
		return "≠"
	case "gt":
		return ">"
	case "gte":
		return "≥"
	case "lt":
		return "<"
	case "lte":
		return "≤"
	}
	return strings.ReplaceAll(op, "_", " ")
}

func fmtVal(v any) string {
	if v == nil {
		return "missing"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// resolve walks a dotted path; segments ending in [*] fan out over arrays.
func resolve(res Resource, path string) ([]any, bool) {
	cur := []any{map[string]any(res)}
	fanout := false
	for _, seg := range strings.Split(path, ".") {
		star := strings.HasSuffix(seg, "[*]")
		key := strings.TrimSuffix(seg, "[*]")
		var next []any
		for _, c := range cur {
			m, ok := c.(map[string]any)
			if !ok {
				next = append(next, nil)
				continue
			}
			v, exists := m[key]
			if !exists {
				if !star {
					next = append(next, nil)
				}
				continue
			}
			if star {
				fanout = true
				if arr, ok := v.([]any); ok {
					next = append(next, arr...)
				}
				continue
			}
			next = append(next, v)
		}
		cur = next
	}
	return cur, fanout
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func toTime(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func equal(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			return math.Abs(fa-fb) < 1e-9
		}
		return false
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func compare(e *Expr, v any) bool {
	want := e.Value
	switch e.Op {
	case "eq":
		return equal(v, want)
	case "neq":
		return !equal(v, want)
	case "gt", "gte", "lt", "lte":
		if a, ok := toFloat(v); ok {
			if b, ok := toFloat(want); ok {
				switch e.Op {
				case "gt":
					return a > b
				case "gte":
					return a >= b
				case "lt":
					return a < b
				default:
					return a <= b
				}
			}
			return false
		}
		if a, ok := toTime(v); ok {
			if b, ok := toTime(want); ok {
				switch e.Op {
				case "gt":
					return a.After(b)
				case "gte":
					return !a.Before(b)
				case "lt":
					return a.Before(b)
				default:
					return !a.After(b)
				}
			}
		}
		return false
	case "in", "nin":
		list, _ := want.([]any)
		found := false
		for _, x := range list {
			if equal(v, x) {
				found = true
				break
			}
		}
		return found == (e.Op == "in")
	case "contains", "not_contains":
		has := false
		switch x := v.(type) {
		case string:
			if s, ok := want.(string); ok {
				has = strings.Contains(strings.ToLower(x), strings.ToLower(s))
			}
		case []any:
			for _, item := range x {
				if equal(item, want) {
					has = true
					break
				}
			}
		}
		return has == (e.Op == "contains")
	case "matches":
		s, ok := v.(string)
		if !ok {
			return false
		}
		re := e.re
		if re == nil {
			var err error
			re, err = regexp.Compile(fmt.Sprint(want))
			if err != nil {
				return false
			}
		}
		return re.MatchString(s)
	case "len_gt", "len_lt":
		n := -1
		switch x := v.(type) {
		case string:
			n = len(x)
		case []any:
			n = len(x)
		}
		if n < 0 {
			return false
		}
		b, ok := toFloat(want)
		if !ok {
			return false
		}
		if e.Op == "len_gt" {
			return float64(n) > b
		}
		return float64(n) < b
	}
	return false
}
