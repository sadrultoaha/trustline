package httpapi_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trustline/trustline/internal/testutil"
)

// Module 4 acceptance: a new failing test on a critical control raises a
// suggested risk; accepting it links them.
func TestCriticalFailureRaisesSuggestedRisk(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("rk")+"@example.test", testutil.Unique("Risk Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	integ := connectAWS(t, c)
	if n := len(items(c.Get("/api/v1/risks?status=suggested"))); n != 0 {
		t.Fatalf("no suggestions expected on a passing baseline, got %d", n)
	}
	c.Must(c.Post("/api/v1/integrations/"+integ["id"].(string)+"/sandbox/scenarios", obj{"key": "public_bucket", "active": true}), 200)
	syncAndWait(t, c, integ["id"].(string))
	var risk obj
	testutil.Eventually(t, 30*time.Second, "suggested risk", func() bool {
		for _, r := range items(c.Get("/api/v1/risks?status=suggested")) {
			if strings.Contains(r["title"].(string), "Storage not publicly accessible") {
				risk = r
				return true
			}
		}
		return false
	})
	ctls := risk["controls"].([]any)
	if len(ctls) != 1 || ctls[0].(obj)["code"] != "DAT-01" || risk["source"] != "test" || risk["score"].(float64) != 20 {
		t.Fatalf("suggested risk not linked to DAT-01: %v", risk)
	}
	// Suggestions are idempotent: another failing sync does not duplicate it.
	syncAndWait(t, c, integ["id"].(string))
	time.Sleep(time.Second)
	if n := len(items(c.Get("/api/v1/risks?status=suggested"))); n != 1 {
		t.Fatalf("expected exactly one suggestion, got %d", n)
	}
	accepted := c.Must(c.Post("/api/v1/risks/"+risk["id"].(string)+"/accept", obj{}), 200).Body
	if accepted["status"] != "open" || accepted["owner"] == nil || len(accepted["controls"].([]any)) != 1 {
		t.Fatalf("accepted risk wrong: %v", accepted)
	}
	// The control shows the linked risk.
	ctl := c.Must(c.Get("/api/v1/controls/"+ctls[0].(obj)["id"].(string)), 200).Body
	if len(ctl["risks"].([]any)) != 1 {
		t.Fatalf("control should list the risk: %v", ctl["risks"])
	}
	// Heatmap counts it at likelihood 4 × impact 5.
	h := c.Must(c.Get("/api/v1/risks/heatmap"), 200).Body
	if h["inherent"].([]any)[3].([]any)[4].(float64) != 1 {
		t.Fatalf("heatmap: %v", h["inherent"])
	}
	// Edit residual and treatment, link another control.
	iam := controlByCode(t, c, "IAM-01")
	up := c.Must(c.Patch("/api/v1/risks/"+risk["id"].(string), obj{"residual_likelihood": 2, "residual_impact": 3, "status": "in_treatment",
		"control_ids": []string{ctls[0].(obj)["id"].(string), iam["id"].(string)}}), 200).Body
	if up["residual_score"].(float64) != 6 || len(up["controls"].([]any)) != 2 {
		t.Fatalf("update failed: %v", up)
	}
	if r := c.Patch("/api/v1/risks/"+risk["id"].(string), obj{"likelihood": 9}); r.Status != 422 {
		t.Fatalf("likelihood 9 should be 422, got %d", r.Status)
	}
}

func TestAIRiskAssessmentAndPlatformTests(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("ra")+"@example.test", testutil.Unique("Assess Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	connectAWS(t, c)
	testutil.Eventually(t, 30*time.Second, "risk assessment test fails initially", func() bool {
		tt := platformTest(t, c, "trustline.risks.assessment_current")
		return tt != nil && tt["last_status"] == "fail"
	})
	res := c.Must(c.Post("/api/v1/risks/ai-assessment", obj{}), 200).Body
	sugg := res["items"].([]any)
	if len(sugg) < 3 || !strings.Contains(res["summary"].(string), "risk(s) proposed") {
		t.Fatalf("expected AI suggestions: %v", res)
	}
	for _, x := range sugg {
		if x.(obj)["status"] != "suggested" || x.(obj)["ai_rationale"] == "" {
			t.Fatalf("suggestion malformed: %v", x)
		}
	}
	testutil.Eventually(t, 30*time.Second, "risk assessment test passes", func() bool {
		tt := platformTest(t, c, "trustline.risks.assessment_current")
		return tt != nil && tt["last_status"] == "pass"
	})
	// Accept one without a plan or owner → owned_and_planned evaluates it.
	first := sugg[0].(obj)
	c.Must(c.Post("/api/v1/risks/"+first["id"].(string)+"/accept", obj{}), 200)
	c.Must(c.Patch("/api/v1/risks/"+first["id"].(string), obj{"treatment_plan": "", "owner_membership_id": ""}), 200)
	testutil.Eventually(t, 30*time.Second, "owner/plan test fails", func() bool {
		tt := platformTest(t, c, "trustline.risks.owned_and_planned")
		return tt != nil && tt["last_status"] == "fail"
	})
	c.Must(c.Post("/api/v1/risks/"+sugg[1].(obj)["id"].(string)+"/dismiss", obj{}), 200)
	if len(items(c.Get("/api/v1/risk-assessments"))) != 1 {
		t.Fatal("assessment record missing")
	}
}

// Module 5 acceptance: upload the sample SOC 2 PDF → assessment with citations.
func TestVendorSOC2AssessmentWithCitations(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("vd")+"@example.test", testutil.Unique("Vendor Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	v := c.Must(c.Post("/api/v1/vendors", obj{"name": "Northwind Cloud Storage", "domain": "northwind.example", "category": "Storage", "data_access": "confidential", "handles_pii": true}), 201).Body
	vid := v["id"].(string)
	if r := c.Post("/api/v1/vendors", obj{"name": "northwind cloud storage"}); r.Status != 422 {
		t.Fatalf("duplicate vendor should be 422, got %d", r.Status)
	}
	pdf, err := os.ReadFile("../../testdata/vendors/northwind-soc2-type2.pdf")
	if err != nil {
		t.Fatal(err)
	}
	c.Must(c.Upload("/api/v1/vendors/"+vid+"/documents", "file", "northwind-soc2-type2.pdf", pdf, map[string]string{"kind": "soc2_type2"}), 201)
	var a obj
	testutil.Eventually(t, 30*time.Second, "assessment done", func() bool {
		as := c.Get("/api/v1/vendors/" + vid).Body["assessments"].([]any)
		if len(as) > 0 && as[0].(obj)["status"] == "done" {
			a = as[0].(obj)["result"].(obj)
			return true
		}
		return false
	})
	if a["opinion"] != "unqualified" || a["period_end"] != "June 30, 2026" {
		t.Fatalf("opinion/period: %v %v", a["opinion"], a["period_end"])
	}
	oc := a["opinion_citation"].(obj)
	if oc["page"].(float64) != 3 || !strings.HasPrefix(oc["section"].(string), "Section 1") {
		t.Fatalf("opinion citation: %v", oc)
	}
	ex := a["exceptions"].([]any)
	if len(ex) != 2 {
		t.Fatalf("expected 2 exceptions, got %v", ex)
	}
	e0 := ex[0].(obj)
	cit := e0["citation"].(obj)
	if e0["control"] != "CC6.2" || cit["page"].(float64) != 8 || !strings.HasPrefix(cit["section"].(string), "Section 4") || e0["management_response"] == "" {
		t.Fatalf("exception citation: %v", e0)
	}
	if len(a["controls_tested"].([]any)) != 18 || len(a["cuecs"].([]any)) != 3 {
		t.Fatalf("controls/cuecs: %d %d", len(a["controls_tested"].([]any)), len(a["cuecs"].([]any)))
	}
	got := c.Get("/api/v1/vendors/" + vid).Body
	if got["recommended_tier"] != "critical" || got["has_security_report"] != true || got["last_reviewed_at"] == nil {
		t.Fatalf("vendor not updated from assessment: %v", got)
	}
	applied := c.Must(c.Post("/api/v1/vendors/"+vid+"/apply-tier", obj{}), 200).Body
	if applied["tier"] != "critical" {
		t.Fatalf("tier not applied: %v", applied["tier"])
	}
	dl := c.Get("/api/v1/vendors/" + vid + "/documents/" + got["documents_list"].([]any)[0].(obj)["id"].(string) + "/download")
	if dl.Status != 200 || len(dl.Raw) != len(pdf) {
		t.Fatalf("download failed: %d", dl.Status)
	}
	// DPA test fails until a DPA is uploaded.
	testutil.Eventually(t, 30*time.Second, "dpa test fails", func() bool {
		tt := platformTest(t, c, "trustline.vendors.dpa")
		return tt != nil && tt["last_status"] == "fail"
	})
	c.Must(c.Upload("/api/v1/vendors/"+vid+"/documents", "file", "northwind-dpa.txt", []byte("Data Processing Agreement between Vendor Co and Northwind"), map[string]string{"kind": "dpa"}), 201)
	testutil.Eventually(t, 30*time.Second, "dpa test passes", func() bool {
		tt := platformTest(t, c, "trustline.vendors.dpa")
		return tt != nil && tt["last_status"] == "pass"
	})
}

func TestVendorDiscoveryAndQuestionnairePortal(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("vq")+"@example.test", testutil.Unique("Portal Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	connectSandbox(t, c, "google_workspace", obj{"domain": "acme.test", "admin_email": "ada@acme.test", "service_account_json": "{}"})
	var discovered []obj
	testutil.Eventually(t, 30*time.Second, "vendors discovered", func() bool {
		discovered = items(c.Get("/api/v1/vendors?status=discovered"))
		return len(discovered) == 4
	})
	pdf := findBy(discovered, "name", "PDF Magic Converter")
	if pdf == nil || !strings.Contains(pdf["discovery_note"].(string), "high-risk scopes") || pdf["source"] != "google_workspace" {
		t.Fatalf("discovery details missing: %v", pdf)
	}
	c.Must(c.Post("/api/v1/vendors/"+pdf["id"].(string)+"/triage", obj{"add": false}), 200)
	slack := findBy(discovered, "name", "Slack")
	added := c.Must(c.Post("/api/v1/vendors/"+slack["id"].(string)+"/triage", obj{"add": true}), 200).Body
	if added["status"] != "active" || added["domain"] != "slack.com" {
		t.Fatalf("triage add failed: %v", added)
	}
	// Re-sync must not resurrect dismissed vendors or duplicate.
	if n := len(items(c.Get("/api/v1/vendors?status=discovered"))); n != 2 {
		t.Fatalf("expected 2 remaining discovered, got %d", n)
	}
	// A dismissal can be undone; only dismissed vendors can be restored.
	if r := c.Must(c.Post("/api/v1/vendors/"+pdf["id"].(string)+"/restore", obj{}), 200).Body; r["status"] != "discovered" {
		t.Fatalf("restore: %v", r)
	}
	c.Must(c.Post("/api/v1/vendors/"+slack["id"].(string)+"/restore", obj{}), 422)
	c.Must(c.Post("/api/v1/vendors/"+pdf["id"].(string)+"/triage", obj{"add": false}), 200)

	// Questionnaire by magic link.
	q := c.Must(c.Post("/api/v1/vendors/"+slack["id"].(string)+"/questionnaires", obj{"recipient_email": "security@slack.example"}), 201).Body
	link := q["link"].(string)
	token := link[strings.LastIndex(link, "/")+1:]
	portal := app.Client(t) // unauthenticated vendor
	pv := portal.Must(portal.Get("/api/v1/vendor-portal/"+token), 200).Body
	tpls := pv["templates"].([]any)
	questions := tpls[0].(obj)["questions"].([]any)
	if pv["vendor_name"] != "Slack" || len(tpls) != 1 || len(questions) != 20 {
		t.Fatalf("portal view: %v", pv)
	}
	if r := portal.Get("/api/v1/vendor-portal/not-a-token"); r.Status != 404 {
		t.Fatalf("bad token should 404, got %d", r.Status)
	}
	answers := obj{}
	for _, x := range questions {
		qq := x.(obj)
		switch qq["type"] {
		case "yes_no":
			answers[qq["id"].(string)] = "Yes"
		case "choice":
			answers[qq["id"].(string)] = "SOC 2 Type II"
		default:
			answers[qq["id"].(string)] = "See our trust center"
		}
	}
	answers["pentest"] = "No"
	portal.Must(portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": obj{"mfa": "Yes"}, "submit": false}), 200)
	if r := portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": obj{"mfa": "Yes"}, "submit": true}); r.Status != 422 {
		t.Fatalf("incomplete submit should be 422, got %d", r.Status)
	}
	if r := portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": obj{"mfa": "Maybe"}}); r.Status != 422 {
		t.Fatalf("invalid yes/no should be 422, got %d", r.Status)
	}
	done := portal.Must(portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": answers, "submit": true}), 200).Body
	if done["status"] != "submitted" {
		t.Fatalf("not submitted: %v", done["status"])
	}
	if r := portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": answers}); r.Status != 422 {
		t.Fatalf("editing after submit should be 422, got %d", r.Status)
	}
	testutil.Eventually(t, 30*time.Second, "questionnaire assessed", func() bool {
		as := c.Get("/api/v1/vendors/" + slack["id"].(string)).Body["assessments"].([]any)
		return len(as) > 0 && as[0].(obj)["status"] == "done" && strings.Contains(as[0].(obj)["result"].(obj)["risk_summary"].(string), "penetration test")
	})
}

func TestPhase3TenantIsolation(t *testing.T) {
	app := testutil.NewApp(t)
	a := app.Signup(t, testutil.Unique("ra")+"@example.test", testutil.Unique("Iso3 A"))
	b := app.Signup(t, testutil.Unique("rb")+"@example.test", testutil.Unique("Iso3 B"))
	b.Must(b.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	risk := b.Must(b.Post("/api/v1/risks", obj{"title": "B's risk", "likelihood": 3, "impact": 3}), 201).Body
	v := b.Must(b.Post("/api/v1/vendors", obj{"name": "B's vendor"}), 201).Body
	for _, path := range []string{"/api/v1/risks/" + risk["id"].(string), "/api/v1/vendors/" + v["id"].(string)} {
		if r := a.Get(path); r.Status != 404 {
			t.Errorf("cross-tenant GET %s should be 404, got %d", path, r.Status)
		}
	}
	for _, r := range []testutil.Resp{
		a.Patch("/api/v1/risks/"+risk["id"].(string), obj{"title": "x"}),
		a.Post("/api/v1/vendors/"+v["id"].(string)+"/questionnaires", obj{"recipient_email": "x@y.z"}),
		a.Patch("/api/v1/vendors/"+v["id"].(string), obj{"tier": "low"}),
		a.Upload("/api/v1/vendors/"+v["id"].(string)+"/documents", "file", "x.txt", []byte("x"), map[string]string{"kind": "dpa"}),
	} {
		if r.Status != 404 {
			t.Errorf("cross-tenant mutation should be 404, got %d %s", r.Status, r.Raw)
		}
	}
	// Linking B's control to A's risk is refused.
	ctl := controlByCode(t, b, "RSK-01")
	ar := a.Must(a.Post("/api/v1/risks", obj{"title": "A's risk", "likelihood": 2, "impact": 2}), 201).Body
	if r := a.Patch("/api/v1/risks/"+ar["id"].(string), obj{"control_ids": []string{ctl["id"].(string)}}); r.Status != 422 {
		t.Errorf("linking a foreign control should be 422, got %d", r.Status)
	}
	if len(items(a.Get("/api/v1/vendors"))) != 0 {
		t.Error("A should see no vendors")
	}
}
