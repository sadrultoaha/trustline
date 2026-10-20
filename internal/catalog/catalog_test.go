package catalog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trustline/trustline/internal/connectors"
	"github.com/trustline/trustline/internal/testengine"
)

func TestCatalogLoads(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Controls) < 80 {
		t.Errorf("expected >= 80 controls, got %d", len(c.Controls))
	}
	if len(c.VendorQuestionnaires) != 1 || len(c.VendorQuestionnaires[0].Questions) != 20 {
		t.Errorf("vendor questionnaire template missing")
	}
	if len(c.Policies) < 20 {
		t.Errorf("expected >= 20 policy templates, got %d", len(c.Policies))
	}
	if len(c.Training) == 0 || len(c.Training[0].Questions) < 5 {
		t.Errorf("training course missing")
	}
	if len(c.Tests) < 110 {
		t.Errorf("expected >= 110 tests, got %d", len(c.Tests))
	}
	want := []string{"soc2", "iso27001", "hipaa", "gdpr", "pci_dss", "nist_ai_rmf", "iso42001"}
	for _, id := range want {
		if c.Framework(id) == nil {
			t.Errorf("framework %s missing", id)
		}
	}
	// Crosswalk: MFA control must satisfy requirements in several frameworks.
	n := 0
	for _, fw := range c.Frameworks {
		for _, r := range fw.Requirements {
			for _, cc := range r.Controls {
				if cc == "IAM-01" {
					n++
				}
			}
		}
	}
	if n < 4 {
		t.Errorf("IAM-01 should be mapped across frameworks, got %d requirements", n)
	}
}

func snapshotFor(t *testing.T, typ string) map[string][]testengine.Resource {
	t.Helper()
	state, err := connectors.ApplyScenarios(typ, "acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := (&connectors.Fake{Type: typ, State: state}).Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]testengine.Resource{}
	for k, v := range snap.Resources {
		for _, r := range v {
			out[k] = append(out[k], r)
		}
	}
	return out
}

// Every sandbox baseline conforms to resources.yaml and every test evaluates
// against it; the baseline must yield a realistic mix of pass and fail.
func TestTestsAgainstSandboxBaselines(t *testing.T) {
	c := MustLoad()
	pass, fail := map[string]int{}, map[string]int{}
	for _, conn := range c.Connectors {
		if conn.Internal {
			continue // platform data is covered by compliance integration tests
		}
		snap := snapshotFor(t, conn.Type)
		for _, rt := range conn.Resources {
			if len(snap[rt.Type]) == 0 {
				t.Errorf("%s baseline has no %s resources", conn.Type, rt.Type)
			}
			check := c.FieldChecker(conn.Type)
			for _, r := range snap[rt.Type] {
				for k := range flatten("", r) {
					if !check(rt.Type, k) {
						t.Errorf("%s.%s baseline field %q not declared in resources.yaml", conn.Type, rt.Type, k)
					}
				}
			}
		}
		for _, tc := range c.Tests {
			if tc.Connector != conn.Type {
				continue
			}
			res := tc.Rule.Evaluate(snap[tc.Resource])
			if res.Status == "pass" {
				pass[conn.Type]++
			} else {
				fail[conn.Type]++
				t.Logf("baseline FAIL %-48s %s", tc.ID, res.Summary)
			}
		}
	}
	for _, conn := range c.Connectors {
		if conn.Internal {
			continue
		}
		t.Logf("%-17s pass=%d fail=%d", conn.Type, pass[conn.Type], fail[conn.Type])
		if pass[conn.Type] == 0 {
			t.Errorf("%s: no tests pass on the baseline", conn.Type)
		}
	}
}

// The acceptance scenario: making the marketing bucket public fails the S3 public access tests.
func TestPublicBucketScenarioFailsDAT01(t *testing.T) {
	c := MustLoad()
	before := snapshotFor(t, "aws")
	state, _ := connectors.ApplyScenarios("aws", "acme", []string{"public_bucket"})
	snap, _ := (&connectors.Fake{Type: "aws", State: state}).Sync(context.Background())
	after := map[string][]testengine.Resource{}
	for k, v := range snap.Resources {
		for _, r := range v {
			after[k] = append(after[k], r)
		}
	}
	flipped := 0
	for _, tc := range c.Tests {
		if tc.Connector != "aws" || tc.Control != "DAT-01" {
			continue
		}
		b := tc.Rule.Evaluate(before[tc.Resource])
		a := tc.Rule.Evaluate(after[tc.Resource])
		if b.Status == "pass" && a.Status == "fail" {
			flipped++
		}
	}
	if flipped == 0 {
		t.Fatal("public_bucket scenario should flip at least one DAT-01 test from pass to fail")
	}
}

func flatten(prefix string, v any) map[string]bool {
	out := map[string]bool{}
	switch m := v.(type) {
	case map[string]any:
		for k, x := range m {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			if _, isMap := x.(map[string]any); isMap {
				for kk := range flatten(p, x) {
					out[kk] = true
				}
				continue
			}
			if arr, ok := x.([]any); ok && len(arr) > 0 {
				if _, objs := arr[0].(map[string]any); objs {
					out[p] = true
					for _, item := range arr {
						for kk := range flatten(p+"[*]", item) {
							out[kk] = true
						}
					}
					continue
				}
			}
			out[p] = true
		}
	}
	return out
}

var _ = json.Marshal
