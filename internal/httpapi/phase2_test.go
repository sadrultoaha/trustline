package httpapi_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/trustline/trustline/internal/testutil"
)

// orgWithTeam creates an org (admin) plus an editor and an employee via invites.
func orgWithTeam(t *testing.T, app *testutil.App, name string) (admin, editor, employee *testutil.Client, editorMID, employeeEmail string) {
	t.Helper()
	admin = app.Signup(t, testutil.Unique("adm")+"@example.test", testutil.Unique(name))
	join := func(role string) (*testutil.Client, string) {
		email := testutil.Unique(role) + "@example.test"
		admin.Must(admin.Post("/api/v1/org/invites", obj{"email": email, "role": role}), 201)
		c := app.Client(t)
		c.Must(c.Post("/api/v1/auth/signup", obj{"email": email, "password": "correct-horse-battery", "name": strings.ToUpper(role[:1]) + role[1:] + " Person", "invite_token": inviteTokens(t, app)[email]}), 201)
		return c, email
	}
	editor, editorEmail := join("editor")
	employee, employeeEmail = join("employee")
	for _, m := range items(admin.Get("/api/v1/org/members")) {
		if m["email"] == editorEmail {
			editorMID = m["id"].(string)
		}
	}
	return
}

func platformTest(t *testing.T, c *testutil.Client, catalogID string) obj {
	t.Helper()
	for _, tt := range items(c.Get("/api/v1/tests?integration=trustline")) {
		if tt["catalog_id"] == catalogID {
			return tt
		}
	}
	return nil
}

// Module 3 acceptance: publish policy v2 → all employees get an acceptance
// task → "policies acknowledged" fails until the threshold accepts.
func TestPolicyPublishAcknowledgementFlow(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	admin, editor, employee, editorMID, _ := orgWithTeam(t, app, "Policy Co")
	admin.Must(admin.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)

	// Library has at least 20 templates.
	lib := items(admin.Must(admin.Get("/api/v1/policy-library"), 200))
	if len(lib) < 20 {
		t.Fatalf("expected >= 20 policy templates, got %d", len(lib))
	}
	p := admin.Must(admin.Post("/api/v1/policies", obj{"template_id": "information-security"}), 201).Body
	pid := p["id"].(string)
	v1 := p["versions"].([]any)[0].(obj)
	if !strings.Contains(v1["content_html"].(string), "Information Security Policy") || strings.Contains(v1["content_html"].(string), "{{") {
		t.Fatalf("template not instantiated: %.300s", v1["content_html"])
	}
	publish := func(vid string) {
		admin.Must(admin.Post("/api/v1/policy-versions/"+vid+"/submit", obj{"approver_membership_id": editorMID}), 200)
		// Only the approver (or an admin) decides; the employee cannot.
		if r := employee.Post("/api/v1/policy-versions/"+vid+"/decide", obj{"approve": true}); r.Status != 403 {
			t.Fatalf("employee must not approve, got %d", r.Status)
		}
		editor.Must(editor.Post("/api/v1/policy-versions/"+vid+"/decide", obj{"approve": true, "comment": "LGTM"}), 200)
		admin.Must(admin.Post("/api/v1/policy-versions/"+vid+"/publish", obj{}), 200)
	}
	// Drafts cannot be published directly.
	if r := admin.Post("/api/v1/policy-versions/"+v1["id"].(string)+"/publish", obj{}); r.Status != 422 {
		t.Fatalf("publishing a draft should be 422, got %d", r.Status)
	}
	publish(v1["id"].(string))
	// Everyone acknowledges v1.
	for _, c := range []*testutil.Client{admin, editor, employee} {
		c.Must(c.Post("/api/v1/policy-versions/"+v1["id"].(string)+"/acknowledge", obj{}), 200)
	}
	testutil.Eventually(t, 30*time.Second, "acknowledged test passes for v1", func() bool {
		tt := platformTest(t, admin, "trustline.policies.acknowledged")
		return tt != nil && tt["last_status"] == "pass"
	})

	// v2: edit, approve, publish → acknowledgements reset.
	v2 := admin.Must(admin.Post("/api/v1/policies/"+pid+"/versions", obj{}), 201).Body
	admin.Must(admin.Do("PUT", "/api/v1/policy-versions/"+v2["id"].(string), obj{"content_html": "<h1>Information Security Policy</h1><p>Updated: all laptops must use a hardware security key.</p><h2>Review</h2><p>Annual.</p>", "change_summary": "Require security keys"}), 200)
	publish(v2["id"].(string))
	testutil.Eventually(t, 30*time.Second, "GOV-05 fails after v2 publish", func() bool {
		return controlByCode(t, admin, "GOV-05")["status"] == "failing"
	})
	// Each employee has an acknowledgement in My work.
	work := employee.Must(employee.Get("/api/v1/me/work"), 200).Body
	var pending obj
	for _, x := range work["policies"].([]any) {
		if x.(obj)["version_id"] == v2["id"] && x.(obj)["accepted_at"] == nil {
			pending = x.(obj)
		}
	}
	if pending == nil {
		t.Fatalf("employee should have v2 pending: %v", work["policies"])
	}
	testutil.Eventually(t, 20*time.Second, "acceptance task for employee", func() bool {
		for _, tk := range items(admin.Get("/api/v1/tasks?status=open")) {
			if tk["kind"] == "policy_acceptance" && strings.Contains(tk["title"].(string), "v2") {
				return true
			}
		}
		return false
	})
	// Two of three (67%) < 90% threshold → still failing.
	admin.Must(admin.Post("/api/v1/policy-versions/"+v2["id"].(string)+"/acknowledge", obj{}), 200)
	editor.Must(editor.Post("/api/v1/policy-versions/"+v2["id"].(string)+"/acknowledge", obj{}), 200)
	time.Sleep(2 * time.Second)
	if s := controlByCode(t, admin, "GOV-05")["status"]; s != "failing" {
		t.Fatalf("GOV-05 should still fail at 67%%, got %v", s)
	}
	employee.Must(employee.Post("/api/v1/policy-versions/"+v2["id"].(string)+"/acknowledge", obj{}), 200)
	testutil.Eventually(t, 30*time.Second, "GOV-05 passes once threshold met", func() bool {
		tt := platformTest(t, admin, "trustline.policies.acknowledged")
		return tt != nil && tt["last_status"] == "pass"
	})
	// Old v1 acknowledgement can no longer be given (superseded).
	if r := employee.Post("/api/v1/policy-versions/"+v1["id"].(string)+"/acknowledge", obj{}); r.Status != 422 {
		t.Fatalf("acknowledging a superseded version should be 422, got %d", r.Status)
	}
	// Employee sees only published versions, never drafts.
	admin.Must(admin.Post("/api/v1/policies/"+pid+"/versions", obj{}), 201)
	view := employee.Must(employee.Get("/api/v1/policies/"+pid), 200).Body
	for _, v := range view["versions"].([]any) {
		if st := v.(obj)["status"]; st != "published" && st != "superseded" {
			t.Fatalf("employee saw a %v version", st)
		}
	}
	if r := employee.Post("/api/v1/policies", obj{"template_id": "password"}); r.Status != 403 {
		t.Fatalf("employee must not create policies, got %d", r.Status)
	}
	// Activity captured the workflow.
	act := items(admin.Get("/api/v1/activity?limit=200&action=policy."))
	for _, want := range []string{"policy.created", "policy.submitted", "policy.approved", "policy.published", "policy.acknowledged", "policy.version_started"} {
		if findBy(act, "action", want) == nil {
			t.Errorf("activity missing %s", want)
		}
	}
}

func TestPolicyAIDraftAndConsistency(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	admin := app.Signup(t, testutil.Unique("ai")+"@example.test", testutil.Unique("Consistency Co"))
	admin.Must(admin.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	admin.Must(admin.Post("/api/v1/integrations", obj{"type": "aws", "mode": "sandbox", "credentials": awsCreds}), 201)
	p := admin.Must(admin.Post("/api/v1/policies", obj{"template_id": "access-control"}), 201).Body
	vid := p["versions"].([]any)[0].(obj)["id"].(string)
	// AI draft tailors the template to connected systems.
	d := admin.Must(admin.Post("/api/v1/policy-versions/"+vid+"/ai-draft", obj{"instructions": "Contractors must use company laptops."}), 200).Body
	if !strings.Contains(d["content_html"].(string), "Systems in scope") || !strings.Contains(d["content_html"].(string), "Amazon Web Services") || d["ai_generated"] != true {
		t.Fatalf("AI draft not tailored: %.400s", d["content_html"])
	}
	// Consistent with settings at 90 days → no issues.
	rep := admin.Must(admin.Post("/api/v1/policy-versions/"+vid+"/consistency", obj{}), 200).Body
	if n := len(rep["issues"].([]any)); n != 0 {
		t.Fatalf("expected no issues, got %v", rep["issues"])
	}
	// Change the setting to 180 → the policy (90 days) is flagged.
	admin.Must(admin.Patch("/api/v1/org", obj{"settings": obj{"access_review_days": 180, "password_min_length": 12, "policy_acceptance_threshold": 90, "ai_enabled": true, "timezone": "UTC", "slack_webhook_url": "", "email_notifications": false}}), 200)
	rep = admin.Must(admin.Post("/api/v1/policy-versions/"+vid+"/consistency", obj{}), 200).Body
	issues := rep["issues"].([]any)
	if len(issues) == 0 {
		t.Fatal("expected an access review inconsistency")
	}
	first := issues[0].(obj)
	if first["setting"] != "access_review_days" || first["policy_value"] != "90" || first["setting_value"] != "180" {
		t.Fatalf("unexpected issue: %v", first)
	}
	got := admin.Must(admin.Get("/api/v1/policies/"+p["id"].(string)), 200).Body
	if got["versions"].([]any)[0].(obj)["consistency"] == nil {
		t.Fatal("consistency result should be stored on the version")
	}
}

func connectSandbox(t *testing.T, c *testutil.Client, typ string, creds obj) string {
	t.Helper()
	in := c.Must(c.Post("/api/v1/integrations", obj{"type": typ, "mode": "sandbox", "credentials": creds}), 201).Body
	id := in["id"].(string)
	testutil.Eventually(t, 30*time.Second, typ+" synced", func() bool {
		return c.Get("/api/v1/integrations/" + id).Body["status"] == "connected"
	})
	return id
}

func syncAndWait(t *testing.T, c *testutil.Client, id string) {
	t.Helper()
	before := c.Get("/api/v1/integrations/" + id).Body["last_sync_at"]
	c.Must(c.Post("/api/v1/integrations/"+id+"/sync", obj{}), 202)
	testutil.Eventually(t, 30*time.Second, "sync completes", func() bool {
		b := c.Get("/api/v1/integrations/" + id).Body
		return b["last_sync_at"] != before && b["status"] == "connected"
	})
}

var (
	hrisCreds = obj{"subdomain": "acme", "api_key": "k-123456"}
	oktaCreds = obj{"domain": "acme.okta.com", "api_token": "tok-123456"}
	ghCreds   = obj{"org": "acme", "token": "tok-123456"}
)

// Module 6 acceptance: terminate a user in the HRIS → offboarding tasks →
// "access revoked" test fails until Okta shows the user deactivated.
func TestOffboardingVerifiedByIdentityProvider(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	admin := app.Signup(t, testutil.Unique("hr")+"@example.test", testutil.Unique("People Co"))
	admin.Must(admin.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	slug := admin.Get("/api/v1/org").Body["slug"].(string)
	tom := "tom@" + slug + ".test"
	hris := connectSandbox(t, admin, "hris", hrisCreds)
	okta := connectSandbox(t, admin, "okta", oktaCreds)
	connectSandbox(t, admin, "github", ghCreds)
	testutil.Eventually(t, 30*time.Second, "people imported", func() bool { return len(items(admin.Get("/api/v1/people"))) == 8 })
	// The initial import does not create onboarding for long-tenured staff.
	for _, tk := range items(admin.Get("/api/v1/tasks?status=open")) {
		if tk["kind"] == "onboarding" || tk["kind"] == "offboarding" {
			t.Fatalf("initial import should not start on/offboarding: %v", tk["title"])
		}
	}
	idp := func() string {
		tt := platformTest(t, admin, "trustline.people.terminated_idp_deactivated")
		if tt == nil {
			return ""
		}
		return tt["last_status"].(string)
	}
	testutil.Eventually(t, 30*time.Second, "idp test evaluated", func() bool { return idp() == "pass" })

	// Terminate Tom in the HRIS.
	admin.Must(admin.Post("/api/v1/integrations/"+hris+"/sandbox/scenarios", obj{"key": "terminate:" + tom, "active": true}), 200)
	syncAndWait(t, admin, hris)
	var person obj
	testutil.Eventually(t, 30*time.Second, "tom terminated with offboarding tasks", func() bool {
		for _, p := range items(admin.Get("/api/v1/people?status=terminated")) {
			if p["email"] == tom {
				person = admin.Get("/api/v1/people/" + p["id"].(string)).Body
				return len(person["tasks"].([]any)) >= 2
			}
		}
		return false
	})
	var oktaTask, ghTask obj
	for _, x := range person["tasks"].([]any) {
		tk := x.(obj)
		if tk["kind"] == "offboarding" && tk["item_key"] == "okta" {
			oktaTask = tk
		}
		if tk["kind"] == "offboarding" && tk["item_key"] == "github" {
			ghTask = tk
		}
	}
	if oktaTask == nil || ghTask == nil || oktaTask["priority"] != "critical" {
		t.Fatalf("expected okta (critical) and github offboarding tasks: %v", person["tasks"])
	}
	testutil.Eventually(t, 30*time.Second, "access revoked test fails", func() bool { return idp() == "fail" })
	if s := controlByCode(t, admin, "HR-05")["status"]; s != "failing" {
		t.Fatalf("HR-05 should fail, got %v", s)
	}

	// Deactivate in Okta → next sync verifies and closes the task.
	admin.Must(admin.Post("/api/v1/integrations/"+okta+"/sandbox/scenarios", obj{"key": "deactivate:" + tom, "active": true}), 200)
	syncAndWait(t, admin, okta)
	testutil.Eventually(t, 30*time.Second, "idp test passes after deactivation", func() bool { return idp() == "pass" })
	testutil.Eventually(t, 20*time.Second, "okta offboarding task verified", func() bool {
		tk := admin.Get("/api/v1/tasks/" + oktaTask["id"].(string)).Body
		return tk["status"] == "done" && strings.Contains(tk["resolution"].(string), "Verified automatically")
	})
	// GitHub still has Tom → its task stays open.
	if st := admin.Get("/api/v1/tasks/" + ghTask["id"].(string)).Body["status"]; st != "open" {
		t.Fatalf("github task should stay open, got %v", st)
	}
}

func TestNewHireOnboardingTrainingAndDevices(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	admin := app.Signup(t, testutil.Unique("ob")+"@example.test", testutil.Unique("Onboard Co"))
	admin.Must(admin.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	slug := admin.Get("/api/v1/org").Body["slug"].(string)
	nina := "nina@" + slug + ".test"
	hris := connectSandbox(t, admin, "hris", hrisCreds)
	testutil.Eventually(t, 30*time.Second, "people imported", func() bool { return len(items(admin.Get("/api/v1/people"))) == 8 })
	admin.Must(admin.Post("/api/v1/integrations/"+hris+"/sandbox/scenarios", obj{"key": "hire_new_employee", "active": true}), 200)
	syncAndWait(t, admin, hris)
	var pid string
	testutil.Eventually(t, 30*time.Second, "nina onboarding", func() bool {
		for _, p := range items(admin.Get("/api/v1/people")) {
			if p["email"] == nina {
				pid = p["id"].(string)
				return len(admin.Get("/api/v1/people/" + pid).Body["tasks"].([]any)) == 5
			}
		}
		return false
	})
	// New hire was invited by email.
	tok := ""
	testutil.Eventually(t, 10*time.Second, "invite email", func() bool { tok = inviteTokens(t, app)[nina]; return tok != "" })
	n := app.Client(t)
	n.Must(n.Post("/api/v1/auth/signup", obj{"email": nina, "password": "correct-horse-battery", "name": "Nina Petrova", "invite_token": tok}), 201)
	if r := n.Get("/api/v1/org"); r.Body["name"] == nil {
		t.Fatalf("nina should be in the org")
	}

	// Training: a wrong attempt fails, a correct one passes.
	tr := n.Must(n.Get("/api/v1/training"), 200).Body
	qs := tr["course"].(obj)["questions"].([]any)
	if _, leaked := qs[0].(obj)["answer"]; leaked {
		t.Fatal("answers must not be sent to the client")
	}
	wrong := obj{}
	for _, q := range qs {
		wrong[q.(obj)["id"].(string)] = 0
	}
	if r := n.Must(n.Post("/api/v1/training/submit", obj{"answers": wrong}), 200).Body; r["passed"] == true {
		t.Fatal("all-first-option answers should fail")
	}
	right := obj{}
	for _, f := range n.Post("/api/v1/training/submit", obj{"answers": wrong}).Body["feedback"].([]any) {
		right[f.(obj)["question_id"].(string)] = f.(obj)["correct_answer"]
	}
	if r := n.Must(n.Post("/api/v1/training/submit", obj{"answers": right}), 200).Body; r["passed"] != true || r["score"].(float64) != 100 {
		t.Fatalf("correct answers should pass: %v", r)
	}

	// Device agent: rotate token, report an unencrypted laptop.
	tokResp := admin.Must(admin.Post("/api/v1/device-token", obj{}), 201).Body
	devToken := tokResp["token"].(string)
	agent := app.Client(t)
	report := func(enc bool) testutil.Resp {
		return agent.DoAuth("POST", "/api/v1/agent/report", devToken, obj{"serial": "C02NINA001", "hostname": "nina-mbp", "owner_email": nina, "platform": "macos",
			"os_version": "15.1", "disk_encrypted": enc, "screen_lock_minutes": 5, "antivirus_running": true, "antivirus_name": "XProtect", "agent_version": "1.0.0"})
	}
	if r := agent.DoAuth("POST", "/api/v1/agent/report", "tld_wrong", obj{}); r.Status != 401 {
		t.Fatalf("bad token should be 401, got %d", r.Status)
	}
	d := agent.Must(report(false), 200).Body
	if d["compliant"] != false || !strings.Contains(fmt.Sprint(d["issues"]), "Disk not encrypted") {
		t.Fatalf("unencrypted device should be non-compliant: %v", d)
	}
	testutil.Eventually(t, 30*time.Second, "disk encryption test fails", func() bool {
		tt := platformTest(t, admin, "trustline.devices.disk_encrypted")
		return tt != nil && tt["last_status"] == "fail"
	})
	agent.Must(report(true), 200)
	testutil.Eventually(t, 30*time.Second, "disk encryption test passes", func() bool {
		tt := platformTest(t, admin, "trustline.devices.disk_encrypted")
		return tt != nil && tt["last_status"] == "pass"
	})
	// Onboarding items verified: account, training, device.
	testutil.Eventually(t, 30*time.Second, "onboarding items close", func() bool {
		done := 0
		for _, x := range admin.Get("/api/v1/people/" + pid).Body["tasks"].([]any) {
			tk := x.(obj)
			if tk["status"] == "done" && (tk["item_key"] == "training" || tk["item_key"] == "device" || tk["item_key"] == "account") {
				done++
			}
		}
		return done == 3
	})
	// HR attests the background check.
	admin.Must(admin.Post("/api/v1/people/"+pid+"/background-check", obj{"status": "completed"}), 200)
	testutil.Eventually(t, 30*time.Second, "background item closes", func() bool {
		for _, x := range admin.Get("/api/v1/people/" + pid).Body["tasks"].([]any) {
			if x.(obj)["item_key"] == "background" {
				return x.(obj)["status"] == "done"
			}
		}
		return false
	})
}

// Module 7: access review campaign with revocation, verification and evidence.
func TestAccessReviewCampaign(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	admin := app.Signup(t, testutil.Unique("ar")+"@example.test", testutil.Unique("Review Co"))
	admin.Must(admin.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	slug := admin.Get("/api/v1/org").Body["slug"].(string)
	connectSandbox(t, admin, "hris", hrisCreds)
	okta := connectSandbox(t, admin, "okta", oktaCreds)
	connectSandbox(t, admin, "github", ghCreds)
	if r := admin.Post("/api/v1/access-reviews", obj{"name": "Q4", "systems": []string{"aws"}}); r.Status != 422 {
		t.Fatalf("unconnected system should be 422, got %d", r.Status)
	}
	c := admin.Must(admin.Post("/api/v1/access-reviews", obj{"name": "Q4 2026 access review", "systems": []string{"okta", "github"}, "due_days": 14}), 201).Body
	cid := c["id"].(string)
	itemsList := c["items"].([]any)
	if len(itemsList) != 13 { // 8 Okta users + 5 GitHub members
		t.Fatalf("expected 13 accounts, got %d", len(itemsList))
	}
	if r := admin.Post("/api/v1/access-reviews/"+cid+"/complete", obj{}); r.Status != 422 {
		t.Fatalf("completing with pending items should be 422, got %d", r.Status)
	}
	tom := "tom@" + slug + ".test"
	var revokeID string
	for _, x := range itemsList {
		it := x.(obj)
		decision := "approve"
		if it["integration_type"] == "okta" && it["email"] == tom {
			decision, revokeID = "revoke", it["id"].(string)
		}
		admin.Must(admin.Post("/api/v1/access-review-items/"+it["id"].(string)+"/decision", obj{"decision": decision, "comment": "reviewed"}), 200)
	}
	if revokeID == "" {
		t.Fatal("tom's okta account not in the campaign")
	}
	done := admin.Must(admin.Post("/api/v1/access-reviews/"+cid+"/complete", obj{}), 200).Body
	if done["status"] != "completed" || done["evidence_id"] == nil || done["revoked"].(float64) != 1 {
		t.Fatalf("campaign not completed with evidence: %v", done)
	}
	csv := admin.Get("/api/v1/access-reviews/" + cid + "/export.csv")
	if csv.Status != 200 || !strings.Contains(string(csv.Raw), "revoke") || strings.Count(string(csv.Raw), "\n") != 14 {
		t.Fatalf("csv export wrong: %d\n%s", csv.Status, csv.Raw)
	}
	ev := admin.Must(admin.Get("/api/v1/evidence/"+done["evidence_id"].(string)), 200).Body
	if ev["verdict"] != "pass" {
		t.Fatalf("access review evidence should count: %v", ev)
	}
	// Revocation task verified when Okta shows the account gone.
	var task obj
	for _, tk := range items(admin.Get("/api/v1/tasks?status=open")) {
		if tk["kind"] == "access_revocation" {
			task = tk
		}
	}
	if task == nil {
		t.Fatal("revocation task missing")
	}
	admin.Must(admin.Post("/api/v1/integrations/"+okta+"/sandbox/scenarios", obj{"key": "deactivate:" + tom, "active": true}), 200)
	syncAndWait(t, admin, okta)
	testutil.Eventually(t, 30*time.Second, "revocation verified", func() bool {
		return admin.Get("/api/v1/tasks/" + task["id"].(string)).Body["status"] == "done"
	})
	testutil.Eventually(t, 30*time.Second, "access review test passes", func() bool {
		tt := platformTest(t, admin, "trustline.access_reviews.current")
		return tt != nil && tt["last_status"] == "pass"
	})
}

func TestPhase2TenantIsolation(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	a := app.Signup(t, testutil.Unique("ia")+"@example.test", testutil.Unique("Iso A"))
	b := app.Signup(t, testutil.Unique("ib")+"@example.test", testutil.Unique("Iso B"))
	b.Must(b.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	p := b.Must(b.Post("/api/v1/policies", obj{"template_id": "password"}), 201).Body
	vid := p["versions"].([]any)[0].(obj)["id"].(string)
	connectSandbox(t, b, "hris", hrisCreds)
	connectSandbox(t, b, "okta", oktaCreds)
	testutil.Eventually(t, 30*time.Second, "people", func() bool { return len(items(b.Get("/api/v1/people"))) > 0 })
	person := items(b.Get("/api/v1/people"))[0]
	camp := b.Must(b.Post("/api/v1/access-reviews", obj{"systems": []string{"okta"}}), 201).Body
	item := camp["items"].([]any)[0].(obj)
	for _, path := range []string{"/api/v1/policies/" + p["id"].(string), "/api/v1/people/" + person["id"].(string), "/api/v1/access-reviews/" + camp["id"].(string), "/api/v1/access-reviews/" + camp["id"].(string) + "/export.csv"} {
		if r := a.Get(path); r.Status != 404 {
			t.Errorf("cross-tenant GET %s should be 404, got %d", path, r.Status)
		}
	}
	for _, r := range []testutil.Resp{
		a.Do("PUT", "/api/v1/policy-versions/"+vid, obj{"content_html": "<p>x</p>"}),
		a.Post("/api/v1/policy-versions/"+vid+"/acknowledge", obj{}),
		a.Post("/api/v1/access-review-items/"+item["id"].(string)+"/decision", obj{"decision": "revoke"}),
		a.Post("/api/v1/people/"+person["id"].(string)+"/background-check", obj{"status": "completed"}),
	} {
		if r.Status != 404 && r.Status != 422 {
			t.Errorf("cross-tenant mutation should be 404/422, got %d %s", r.Status, r.Raw)
		}
	}
	if len(items(a.Get("/api/v1/policies"))) != 0 || len(items(a.Get("/api/v1/people"))) != 0 {
		t.Error("tenant A should see no policies or people")
	}
}
