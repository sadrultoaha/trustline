package httpapi_test

import (
	"testing"

	"github.com/trustline/trustline/internal/testutil"
)

func fieldErrors(r testutil.Resp) obj {
	e, _ := r.Body["error"].(obj)
	f, _ := e["fields"].(obj)
	return f
}

// A custom framework maps onto the control library, so controls that already
// pass count toward it the moment it is adopted, and it stays private to its org.
func TestCustomFramework(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("cf")+"@example.test", testutil.Unique("Custom Co"))
	other := app.Signup(t, testutil.Unique("cfo")+"@example.test", testutil.Unique("Other Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	connectAWS(t, c)
	if s := controlByCode(t, c, "DAT-01")["status"]; s != "passing" {
		t.Fatalf("DAT-01 should pass with AWS connected, got %v", s)
	}

	// Validation.
	r := c.Post("/api/v1/frameworks", obj{"name": " ", "requirements": []obj{}})
	if r.Status != 422 || fieldErrors(r)["name"] == nil || fieldErrors(r)["requirements"] == nil {
		t.Fatalf("empty framework should be 422 with name and requirements errors: %d %v", r.Status, r.Body)
	}
	r = c.Post("/api/v1/frameworks", obj{"name": "Acme", "requirements": []obj{{"code": "A1", "title": "x", "controls": []string{"NOPE-99"}}, {"code": "a1", "title": "", "controls": []string{}}}})
	f := fieldErrors(r)
	if r.Status != 422 || f["requirements.0.controls"] == nil || f["requirements.1.code"] == nil || f["requirements.1.title"] == nil || f["requirements.1.controls"] == nil {
		t.Fatalf("bad requirements should be rejected per row: %d %v", r.Status, r.Body)
	}
	if r := c.Post("/api/v1/frameworks", obj{"name": "soc 2", "requirements": []obj{{"code": "A1", "title": "x", "controls": []string{"DAT-01"}}}}); r.Status != 422 {
		t.Fatalf("built-in name should be rejected, got %d", r.Status)
	}

	def := obj{"name": "Acme Customer Contract", "version": "2026", "publisher": "Acme Corp", "description": "Security schedule from the Acme MSA.",
		"requirements": []obj{
			{"code": "ACME-1", "section": "Data", "title": "Encrypt customer data at rest", "controls": []string{"DAT-01"}},
			{"code": "ACME-2", "title": "Maintain a security policy", "controls": []string{"GOV-01", "GOV-01"}},
		}}
	fw := c.Must(c.Post("/api/v1/frameworks", def), 201).Body
	id := fw["id"].(string)
	if fw["custom"] != true || fw["adopted"] != false || len(fw["sections"].([]any)) != 2 {
		t.Fatalf("new custom framework should be custom, not adopted, with 2 sections: %v", fw)
	}
	if r := c.Post("/api/v1/frameworks", def); r.Status != 422 || fieldErrors(r)["name"] == nil {
		t.Fatalf("duplicate name should be 422, got %d", r.Status)
	}
	if got := findBy(items(c.Get("/api/v1/frameworks")), "id", id); got == nil || got["custom"] != true {
		t.Fatalf("custom framework should be listed: %v", got)
	}

	// Adopting reuses the passing control through the crosswalk.
	ad := c.Must(c.Post("/api/v1/frameworks/"+id+"/adopt", obj{}), 200).Body
	rd := ad["readiness"].(obj)
	if rd["requirements_total"].(float64) != 2 || rd["requirements_met"].(float64) != 1 || rd["percent"].(float64) != 50 {
		t.Fatalf("expected 1 of 2 requirements met at once: %v", rd)
	}
	ctls := items(c.Get("/api/v1/controls?framework=" + id))
	if len(ctls) != 2 || findBy(ctls, "code", "DAT-01") == nil {
		t.Fatalf("framework filter should return the 2 mapped controls: %v", ctls)
	}

	// Edit: add a requirement mapping a control SOC 2 already scoped.
	got := c.Must(c.Get("/api/v1/frameworks/"+id+"/definition"), 200).Body
	reqs := got["requirements"].([]any)
	if len(reqs) != 2 || reqs[1].(obj)["section"] != "Requirements" || len(reqs[1].(obj)["controls"].([]any)) != 1 {
		t.Fatalf("definition should round-trip normalised: %v", got)
	}
	def["requirements"] = append(def["requirements"].([]obj), obj{"code": "ACME-3", "title": "MFA for admins", "controls": []string{"IAM-01"}})
	up := c.Must(c.Do("PUT", "/api/v1/frameworks/"+id+"/definition", def), 200).Body
	if up["readiness"].(obj)["requirements_total"].(float64) != 3 {
		t.Fatalf("update should be reflected in readiness: %v", up["readiness"])
	}

	// Private to the org.
	if findBy(items(other.Get("/api/v1/frameworks")), "id", id) != nil {
		t.Fatal("custom framework leaked into another org's list")
	}
	for _, r := range []testutil.Resp{
		other.Get("/api/v1/frameworks/" + id),
		other.Get("/api/v1/frameworks/" + id + "/definition"),
		other.Post("/api/v1/frameworks/"+id+"/adopt", obj{}),
		other.Do("PUT", "/api/v1/frameworks/"+id+"/definition", def),
		other.Delete("/api/v1/frameworks/" + id + "/definition"),
	} {
		if r.Status != 404 {
			t.Errorf("cross-tenant access should be 404, got %d", r.Status)
		}
	}
	// Same name is fine in another org.
	other.Must(other.Post("/api/v1/frameworks", def), 201)

	// Built-in frameworks have no editable definition.
	if r := c.Delete("/api/v1/frameworks/soc2/definition"); r.Status != 404 {
		t.Fatalf("deleting a catalog framework should be 404, got %d", r.Status)
	}

	c.Must(c.Delete("/api/v1/frameworks/"+id+"/definition"), 200)
	if r := c.Get("/api/v1/frameworks/" + id); r.Status != 404 {
		t.Fatalf("deleted framework should be 404, got %d", r.Status)
	}
	if s := controlByCode(t, c, "DAT-01")["status"]; s != "passing" {
		t.Fatalf("controls stay after deleting a framework, got %v", s)
	}
	act := items(c.Get("/api/v1/activity?limit=200"))
	for _, want := range []string{"framework.created", "framework.updated", "framework.deleted"} {
		if findBy(act, "action", want) == nil {
			t.Errorf("activity missing %s", want)
		}
	}
}
