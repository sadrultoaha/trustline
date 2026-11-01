package httpapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/trustline/trustline/internal/testutil"
)

func strs(v any) []string {
	out := []string{}
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func templateIDs(v obj) []string {
	out := []string{}
	for _, x := range v["questionnaire_templates"].([]any) {
		out = append(out, x.(obj)["id"].(string))
	}
	return out
}

func portalToken(q obj) string {
	link := q["link"].(string)
	return link[strings.LastIndex(link, "/")+1:]
}

// Vendors map to in-scope frameworks; questionnaires follow the mapping (or an
// explicit attached set) and are frozen when sent.
func TestVendorFrameworksAndQuestionnaires(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("vf")+"@example.test", testutil.Unique("Vendor Scope Co"))
	other := app.Signup(t, testutil.Unique("vfo")+"@example.test", testutil.Unique("Other Scope Co"))
	for _, fw := range []string{"soc2", "hipaa"} {
		c.Must(c.Post("/api/v1/frameworks/"+fw+"/adopt", obj{}), 200)
	}
	custom := c.Must(c.Post("/api/v1/frameworks", obj{"name": "Acme Schedule", "requirements": []obj{
		{"code": "A-1", "title": "Encrypt data", "controls": []string{"DAT-01"}},
		{"code": "A-2", "title": "Report incidents", "description": "Within 24 hours.", "controls": []string{"GOV-01"}},
	}}), 201).Body["id"].(string)
	c.Must(c.Post("/api/v1/frameworks/"+custom+"/adopt", obj{}), 200)
	gen := "framework:" + custom

	lib := items(c.Must(c.Get("/api/v1/questionnaire-library"), 200))
	if g := findBy(lib, "id", gen); g == nil || g["question_count"].(float64) != 3 || g["generated"] != true {
		t.Fatalf("library should include the generated questionnaire: %v", lib)
	}
	if findBy(lib, "id", "hipaa-vendor-v1") == nil || lib[0]["id"] != "vendor-security-v1" {
		t.Fatalf("library should list the general questionnaire first and framework ones: %v", lib)
	}
	if findBy(items(other.Get("/api/v1/questionnaire-library")), "id", gen) != nil {
		t.Fatal("generated questionnaire leaked into another org's library")
	}

	// Only in-scope frameworks can be mapped; attached questionnaires must exist.
	for _, body := range []obj{
		{"name": "Bad 1", "frameworks": []string{"gdpr"}},
		{"name": "Bad 2", "frameworks": []string{"nope"}},
		{"name": "Bad 3", "questionnaires": []string{"nope"}},
		{"name": "Bad 4", "questionnaires": []string{}},
	} {
		if r := c.Post("/api/v1/vendors", body); r.Status != 422 {
			t.Fatalf("%v should be 422, got %d", body, r.Status)
		}
	}
	if len(items(c.Get("/api/v1/vendors"))) != 0 {
		t.Fatal("rejected vendors must not be created")
	}

	// Without an explicit set, questionnaires follow the frameworks.
	v := c.Must(c.Post("/api/v1/vendors", obj{"name": "MedCloud", "frameworks": []string{"hipaa", custom, "hipaa"}}), 201).Body
	id := v["id"].(string)
	if got := strs(v["frameworks"]); len(got) != 2 || v["questionnaires_default"] != true {
		t.Fatalf("frameworks should be deduplicated and questionnaires default: %v", v)
	}
	if got := templateIDs(v); strings.Join(got, ",") != "vendor-security-v1,hipaa-vendor-v1,"+gen {
		t.Fatalf("suggested questionnaires: %v", got)
	}

	// An explicit set; mapping another framework adds its questionnaire to it.
	v = c.Must(c.Patch("/api/v1/vendors/"+id, obj{"questionnaires": []string{"vendor-security-v1"}}), 200).Body
	if v["questionnaires_default"] != false || len(templateIDs(v)) != 1 {
		t.Fatalf("explicit set: %v", templateIDs(v))
	}
	c.Must(c.Post("/api/v1/frameworks/pci_dss/adopt", obj{}), 200)
	v = c.Must(c.Patch("/api/v1/vendors/"+id, obj{"frameworks": []string{"hipaa", custom, "pci_dss"}}), 200).Body
	if got := templateIDs(v); strings.Join(got, ",") != "vendor-security-v1,pci-vendor-v1" {
		t.Fatalf("mapping PCI DSS should attach its questionnaire: %v", got)
	}

	// A mapping survives its framework leaving scope, but new ones must be in scope.
	c.Must(c.Delete("/api/v1/frameworks/hipaa"), 200)
	c.Must(c.Patch("/api/v1/vendors/"+id, obj{"frameworks": []string{"hipaa", custom, "pci_dss"}, "tier": "high"}), 200)
	if r := c.Patch("/api/v1/vendors/"+id, obj{"frameworks": []string{"hipaa", "gdpr"}}); r.Status != 422 {
		t.Fatalf("mapping an out-of-scope framework should be 422, got %d", r.Status)
	}
	if r := c.Get("/api/v1/vendors/" + id).Body; len(strs(r["frameworks"])) != 3 || r["tier"] != "high" {
		t.Fatalf("rejected update must not change the vendor: %v", r)
	}
	if len(items(c.Get("/api/v1/vendors?framework=pci_dss"))) != 1 || len(items(c.Get("/api/v1/vendors?framework=gdpr"))) != 0 {
		t.Fatal("framework filter")
	}

	// Send a bundle: general + generated + HIPAA.
	c.Must(c.Patch("/api/v1/vendors/"+id, obj{"questionnaires": []string{"vendor-security-v1", gen, "hipaa-vendor-v1"}}), 200)
	q := c.Must(c.Post("/api/v1/vendors/"+id+"/questionnaires", obj{"recipient_email": "sec@medcloud.example"}), 201).Body
	token := portalToken(q)
	portal := app.Client(t)
	pv := portal.Must(portal.Get("/api/v1/vendor-portal/"+token), 200).Body
	tpls := pv["templates"].([]any)
	if len(tpls) != 3 || tpls[1].(obj)["title"] != "Acme Schedule requirements" {
		t.Fatalf("portal should show 3 questionnaires: %v", tpls)
	}
	answers := obj{}
	total := 0
	for _, tp := range tpls {
		for _, x := range tp.(obj)["questions"].([]any) {
			qq := x.(obj)
			total++
			switch qq["type"] {
			case "yes_no":
				answers[qq["id"].(string)] = "Yes"
			case "choice":
				answers[qq["id"].(string)] = qq["options"].([]any)[0]
			default:
				answers[qq["id"].(string)] = "None"
			}
		}
	}
	if total != 20+3+10 || answers[custom+"/1"] != "Yes" {
		t.Fatalf("expected 33 questions with namespaced generated ids, got %d", total)
	}
	answers[custom+"/2"] = "No"
	if r := portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": obj{"certifications": "Maybe"}}); r.Status != 422 {
		t.Fatalf("an answer outside the options should be 422, got %d", r.Status)
	}

	// Editing the custom framework after sending does not change what was sent.
	c.Must(c.Do("PUT", "/api/v1/frameworks/"+custom+"/definition", obj{"name": "Acme Schedule", "requirements": []obj{
		{"code": "A-1", "title": "Encrypt data", "controls": []string{"DAT-01"}},
		{"code": "A-2", "title": "Report incidents", "controls": []string{"GOV-01"}},
		{"code": "A-3", "title": "MFA", "controls": []string{"IAM-01"}},
	}}), 200)
	pv = portal.Must(portal.Get("/api/v1/vendor-portal/"+token), 200).Body
	if n := len(pv["templates"].([]any)[1].(obj)["questions"].([]any)); n != 3 {
		t.Fatalf("sent questionnaire should keep 3 questions, got %d", n)
	}
	done := portal.Must(portal.Do("PUT", "/api/v1/vendor-portal/"+token, obj{"answers": answers, "submit": true}), 200).Body
	if done["status"] != "submitted" {
		t.Fatalf("not submitted: %v", done["status"])
	}
	testutil.Eventually(t, 30*time.Second, "bundle assessed", func() bool {
		as := c.Get("/api/v1/vendors/" + id).Body["assessments"].([]any)
		return len(as) > 0 && as[0].(obj)["status"] == "done" && strings.Contains(as[0].(obj)["result"].(obj)["risk_summary"].(string), "A-2")
	})

	// Deleting the custom framework unmaps it; the sent questionnaire is intact.
	c.Must(c.Delete("/api/v1/frameworks/"+custom+"/definition"), 200)
	d := c.Get("/api/v1/vendors/" + id).Body
	if got := strs(d["frameworks"]); strings.Contains(strings.Join(got, ","), custom) {
		t.Fatalf("deleted framework still mapped: %v", got)
	}
	if got := templateIDs(d); strings.Join(got, ",") != "vendor-security-v1,hipaa-vendor-v1" {
		t.Fatalf("generated questionnaire should be detached: %v", got)
	}
	sent := d["questionnaires"].([]any)[0].(obj)
	if len(sent["templates"].([]any)) != 3 || sent["answers"].(obj)[custom+"/2"] != "No" {
		t.Fatalf("sent questionnaire should keep its questions and answers: %v", sent)
	}

	// Another org can't map or attach this org's frameworks, or touch the vendor.
	ov := other.Must(other.Post("/api/v1/vendors", obj{"name": "Theirs"}), 201).Body["id"].(string)
	c.Must(c.Post("/api/v1/frameworks", obj{"name": "Private", "requirements": []obj{{"code": "P", "title": "P", "controls": []string{"DAT-01"}}}}), 201)
	priv := findBy(items(c.Get("/api/v1/frameworks")), "name", "Private")["id"].(string)
	c.Must(c.Post("/api/v1/frameworks/"+priv+"/adopt", obj{}), 200)
	for _, body := range []obj{{"frameworks": []string{priv}}, {"questionnaires": []string{"framework:" + priv}}} {
		if r := other.Patch("/api/v1/vendors/"+ov, body); r.Status != 422 {
			t.Errorf("foreign framework %v should be 422, got %d", body, r.Status)
		}
	}
	if r := other.Patch("/api/v1/vendors/"+id, obj{"frameworks": []string{}}); r.Status != 404 {
		t.Errorf("cross-tenant vendor update should be 404, got %d", r.Status)
	}
}

// Questionnaires sent before questions were frozen still render from the catalog.
func TestLegacyQuestionnaireWithoutSnapshot(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.Signup(t, testutil.Unique("lq")+"@example.test", testutil.Unique("Legacy Co"))
	id := c.Must(c.Post("/api/v1/vendors", obj{"name": "Old Vendor"}), 201).Body["id"].(string)
	q := c.Must(c.Post("/api/v1/vendors/"+id+"/questionnaires", obj{"recipient_email": "a@old.example"}), 201).Body
	if _, err := app.Pool.Exec(context.Background(), `UPDATE vendor_questionnaires SET questions = NULL, template_id = 'vendor-security-v1' WHERE id = $1`, q["id"]); err != nil {
		t.Fatal(err)
	}
	portal := app.Client(t)
	pv := portal.Must(portal.Get("/api/v1/vendor-portal/"+portalToken(q)), 200).Body
	if tpls := pv["templates"].([]any); len(tpls) != 1 || len(tpls[0].(obj)["questions"].([]any)) != 20 {
		t.Fatalf("legacy questionnaire should render the catalog template: %v", pv["templates"])
	}
	portal.Must(portal.Do("PUT", "/api/v1/vendor-portal/"+portalToken(q), obj{"answers": obj{"mfa": "Yes"}}), 200)
}
