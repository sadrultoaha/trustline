package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trustline/trustline/internal/testutil"
)

type obj = map[string]any

func items(r testutil.Resp) []obj {
	raw, _ := r.Body["items"].([]any)
	out := make([]obj, 0, len(raw))
	for _, x := range raw {
		out = append(out, x.(obj))
	}
	return out
}

func findBy(list []obj, key, val string) obj {
	for _, x := range list {
		if x[key] == val {
			return x
		}
	}
	return nil
}

var awsCreds = obj{"role_arn": "arn:aws:iam::412398765432:role/TrustlineAudit", "external_id": "ext-123456"}

// connectAWS connects the AWS sandbox and waits for the first sync + test run.
func connectAWS(t *testing.T, c *testutil.Client) obj {
	t.Helper()
	in := c.Must(c.Post("/api/v1/integrations", obj{"type": "aws", "mode": "sandbox", "credentials": awsCreds}), 201).Body
	id := in["id"].(string)
	testutil.Eventually(t, 30*time.Second, "aws first sync and test run", func() bool {
		r := c.Get("/api/v1/integrations/" + id)
		return r.Body["status"] == "connected" && c.Get("/api/v1/tests?integration=aws&status=pending").Body["items"] != nil &&
			len(items(c.Get("/api/v1/tests?integration=aws&status=pending"))) == 0 && len(items(c.Get("/api/v1/tests?integration=aws"))) > 0
	})
	return in
}

func controlByCode(t *testing.T, c *testutil.Client, code string) obj {
	t.Helper()
	ctl := findBy(items(c.Must(c.Get("/api/v1/controls?q="+code), 200)), "code", code)
	if ctl == nil {
		t.Fatalf("control %s not in scope", code)
	}
	return ctl
}

// Module 1 acceptance: flip the sandbox to a public bucket → within one
// scheduler tick the control fails, a notification fires, and a remediation
// task exists with steps and a Terraform snippet from the AI layer.
func TestPublicBucketFlipsControlAndCreatesRemediation(t *testing.T) {
	slack := struct {
		sync.Mutex
		bodies []string
	}{}
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		slack.Lock()
		slack.bodies = append(slack.bodies, string(b))
		slack.Unlock()
		w.WriteHeader(200)
	}))
	defer hook.Close()

	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("sec")+"@example.test", testutil.Unique("Bucket Co"))
	c.Must(c.Patch("/api/v1/org", obj{"settings": obj{"access_review_days": 90, "password_min_length": 12, "policy_acceptance_threshold": 90, "ai_enabled": true, "timezone": "UTC", "slack_webhook_url": hook.URL, "email_notifications": true}}), 200)
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	integ := connectAWS(t, c)

	dat01 := controlByCode(t, c, "DAT-01")
	if dat01["status"] != "passing" {
		t.Fatalf("DAT-01 should pass on the baseline, got %v (%v)", dat01["status"], dat01["status_reason"])
	}

	// Flip the sandbox, then run one scheduler tick.
	c.Must(c.Post("/api/v1/integrations/"+integ["id"].(string)+"/sandbox/scenarios", obj{"key": "public_bucket", "active": true}), 200)
	if err := app.Jobs.Enqueue(t.Context(), schedulerTick()); err != nil {
		t.Fatal(err)
	}
	testutil.Eventually(t, 30*time.Second, "DAT-01 failing after tick", func() bool {
		return controlByCode(t, c, "DAT-01")["status"] == "failing"
	})

	// Notification.
	n := c.Must(c.Get("/api/v1/notifications"), 200)
	if n.Body["unread"].(float64) < 1 || !strings.Contains(items(n)[0]["title"].(string), "DAT-01") {
		t.Fatalf("expected DAT-01 notification, got %s", n.Raw)
	}
	// Slack webhook delivered.
	testutil.Eventually(t, 20*time.Second, "slack webhook", func() bool {
		slack.Lock()
		defer slack.Unlock()
		for _, b := range slack.bodies {
			if strings.Contains(b, "DAT-01") {
				return true
			}
		}
		return false
	})

	// Remediation task with AI steps + Terraform naming the bucket.
	var task obj
	testutil.Eventually(t, 30*time.Second, "remediation task with terraform", func() bool {
		for _, tk := range items(c.Get("/api/v1/tasks?status=open")) {
			if tk["control_code"] == "DAT-01" && tk["ai_status"] == "done" {
				task = tk
				return true
			}
		}
		return false
	})
	tf := task["terraform"].(string)
	if !strings.Contains(tf, `resource "aws_s3_bucket_public_access_block"`) || !strings.Contains(tf, "restrict_public_buckets = true") || !strings.Contains(tf, "-marketing-assets") {
		t.Fatalf("terraform missing fix for the bucket:\n%s\ntask: %s", tf, mustJSON(task))
	}
	if len(task["steps"].([]any)) < 3 {
		t.Fatalf("expected step-by-step instructions, got %v", task["steps"])
	}

	// Test detail exposes a frozen evidence snapshot for the failing run.
	tests := items(c.Get("/api/v1/tests?control=" + dat01["id"].(string)))
	var failingTest obj
	for _, tt := range tests {
		if tt["last_status"] == "fail" {
			failingTest = tt
		}
	}
	run := c.Must(c.Get("/api/v1/test-runs/"+failingTest["last_run_id"].(string)), 200).Body
	if run["snapshot_sha256"] == "" || len(run["resources"].([]any)) == 0 || len(run["failing"].([]any)) == 0 {
		t.Fatalf("run lacks frozen evidence: %s", mustJSON(run))
	}

	// Revert → next sync passes → task auto-resolves, control recovers.
	c.Must(c.Post("/api/v1/integrations/"+integ["id"].(string)+"/sandbox/scenarios", obj{"key": "public_bucket", "active": false}), 200)
	c.Must(c.Post("/api/v1/integrations/"+integ["id"].(string)+"/sync", obj{}), 202)
	testutil.Eventually(t, 30*time.Second, "DAT-01 recovers", func() bool {
		return controlByCode(t, c, "DAT-01")["status"] == "passing"
	})
	done := c.Must(c.Get("/api/v1/tasks/"+task["id"].(string)), 200).Body
	if done["status"] != "done" || !strings.Contains(done["resolution"].(string), "Resolved automatically") {
		t.Fatalf("task should auto-resolve: %v", done)
	}

	// Activity log captured the chain.
	act := items(c.Get("/api/v1/activity?limit=200"))
	for _, want := range []string{"integration.connected", "integration.sandbox_scenario", "control.status_changed", "task.created", "task.remediation_generated"} {
		if findBy(act, "action", want) == nil {
			t.Errorf("activity missing %s", want)
		}
	}
}

// Module 2 acceptance: adopting a second framework maps existing passing
// controls through the crosswalk, so readiness is non-zero immediately.
func TestSecondFrameworkReadinessViaCrosswalk(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("cw")+"@example.test", testutil.Unique("Crosswalk Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	connectAWS(t, c)
	soc2 := c.Must(c.Get("/api/v1/frameworks/soc2"), 200).Body
	if soc2["readiness"].(obj)["controls_passing"].(float64) == 0 {
		t.Fatalf("soc2 should have passing controls: %v", soc2["readiness"])
	}
	// Adopt ISO 27001: the response itself (no worker round-trip) is non-zero.
	iso := c.Must(c.Post("/api/v1/frameworks/iso27001/adopt", obj{}), 200).Body
	r := iso["readiness"].(obj)
	if r["controls_passing"].(float64) == 0 || r["requirements_met"].(float64) == 0 || r["percent"].(float64) <= 0 {
		t.Fatalf("iso27001 readiness should be non-zero immediately: %v", r)
	}
	// Drilldown lists sections and requirements with control status.
	if len(iso["sections"].([]any)) != 4 {
		t.Fatalf("expected 4 ISO sections, got %d", len(iso["sections"].([]any)))
	}
	// Adopting twice is rejected.
	if r := c.Post("/api/v1/frameworks/iso27001/adopt", obj{}); r.Status != 422 {
		t.Fatalf("double adopt should be 422, got %d", r.Status)
	}
	list := items(c.Get("/api/v1/frameworks"))
	if len(list) != 7 {
		t.Fatalf("expected 7 frameworks in the catalog, got %d", len(list))
	}
}

func TestEvidenceUploadAIValidationAndOverride(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("ev")+"@example.test", testutil.Unique("Evidence Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	ctl := controlByCode(t, c, "IAM-01")
	if ctl["status"] != "needs_evidence" {
		t.Fatalf("IAM-01 without integrations should need evidence, got %v", ctl["status"])
	}
	png, err := os.ReadFile("../../testdata/evidence/okta-mfa-policy-screenshot.png")
	if err != nil {
		t.Fatal(err)
	}
	ev := c.Must(c.Upload("/api/v1/controls/"+ctl["id"].(string)+"/evidence", "file", "okta-mfa-policy-screenshot.png", png, map[string]string{"title": "Okta MFA policy"}), 201).Body
	if ev["verdict"] != "pending" || ev["expires_at"] == nil {
		t.Fatalf("new evidence should be pending with expiry: %v", ev)
	}
	testutil.Eventually(t, 30*time.Second, "AI verdict", func() bool {
		return c.Get("/api/v1/evidence/"+ev["id"].(string)).Body["ai_status"] == "done"
	})
	got := c.Get("/api/v1/evidence/" + ev["id"].(string)).Body
	if got["ai_verdict"] != "pass" || got["ai_reasoning"] == "" {
		t.Fatalf("expected AI pass with reasoning: %v", got)
	}
	if s := controlByCode(t, c, "IAM-01")["status"]; s != "passing" {
		t.Fatalf("IAM-01 should pass with accepted evidence, got %v", s)
	}
	// Download returns identical bytes.
	dl := c.Get("/api/v1/evidence/" + ev["id"].(string) + "/download")
	if dl.Status != 200 || string(dl.Raw) != string(png) {
		t.Fatalf("download mismatch: %d %d bytes", dl.Status, len(dl.Raw))
	}
	// Human override to fail flips the control back.
	c.Must(c.Post("/api/v1/evidence/"+ev["id"].(string)+"/review", obj{"verdict": "fail", "reason": "Screenshot is from the staging tenant"}), 200)
	if s := controlByCode(t, c, "IAM-01")["status"]; s != "needs_evidence" {
		t.Fatalf("override should flip control, got %v", s)
	}
	// Irrelevant evidence is rejected by the AI.
	other := controlByCode(t, c, "HR-01")
	bad := c.Must(c.Upload("/api/v1/controls/"+other["id"].(string)+"/evidence", "file", "lunch.txt", []byte("Team lunch on Friday at noon."), nil), 201).Body
	testutil.Eventually(t, 30*time.Second, "AI verdict on irrelevant file", func() bool {
		return c.Get("/api/v1/evidence/"+bad["id"].(string)).Body["ai_status"] == "done"
	})
	if v := c.Get("/api/v1/evidence/" + bad["id"].(string)).Body["ai_verdict"]; v != "fail" {
		t.Fatalf("irrelevant evidence should fail, got %v", v)
	}
	// Unsupported type rejected.
	if r := c.Upload("/api/v1/controls/"+other["id"].(string)+"/evidence", "file", "x.exe", []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff"), nil); r.Status != 422 {
		t.Fatalf("binary upload should be 422, got %d %s", r.Status, r.Raw)
	}
}

func TestAIOptOutSkipsValidation(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("noai")+"@example.test", testutil.Unique("No AI Co"))
	c.Must(c.Patch("/api/v1/org", obj{"settings": obj{"access_review_days": 90, "password_min_length": 12, "policy_acceptance_threshold": 90, "ai_enabled": false, "timezone": "UTC", "slack_webhook_url": "", "email_notifications": false}}), 200)
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	ctl := controlByCode(t, c, "GOV-01")
	ev := c.Must(c.Upload("/api/v1/controls/"+ctl["id"].(string)+"/evidence", "file", "infosec-policy.txt", []byte("Information security policy approved"), nil), 201).Body
	testutil.Eventually(t, 30*time.Second, "validation skipped", func() bool {
		return c.Get("/api/v1/evidence/"+ev["id"].(string)).Body["ai_status"] == "skipped"
	})
	var calls int
	if err := app.Pool.QueryRow(t.Context(), `SELECT count(*) FROM ai_calls a JOIN org_memberships m ON m.org_id = a.org_id JOIN users u ON u.id = m.user_id WHERE u.email LIKE 'noai%'`).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("no AI calls expected for opted-out org, got %d", calls)
	}
	c.Must(c.Post("/api/v1/evidence/"+ev["id"].(string)+"/review", obj{"verdict": "pass"}), 200)
	if s := controlByCode(t, c, "GOV-01")["status"]; s != "passing" {
		t.Fatalf("human-accepted evidence should pass the control, got %v", s)
	}
}

func TestCustomTestBuilderRules(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	c := app.Signup(t, testutil.Unique("ct")+"@example.test", testutil.Unique("Custom Co"))
	c.Must(c.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	connectAWS(t, c)
	ctl := controlByCode(t, c, "CRY-01")
	rule := obj{"resource": "s3_bucket", "quantifier": "all",
		"where":  obj{"field": "tags.data_classification", "op": "eq", "value": "confidential"},
		"assert": obj{"and": []any{obj{"field": "encryption.algorithm", "op": "eq", "value": "aws:kms"}, obj{"field": "versioning", "op": "eq", "value": true}}}}
	// Preview against live snapshot.
	p := c.Must(c.Post("/api/v1/tests/preview", obj{"integration_type": "aws", "rule": rule}), 200).Body
	if p["status"] != "pass" || p["evaluated"].(float64) != 2 {
		t.Fatalf("preview: %v", p)
	}
	// Unknown field rejected.
	bad := obj{"resource": "s3_bucket", "quantifier": "all", "assert": obj{"field": "colour", "op": "eq", "value": "blue"}}
	if r := c.Post("/api/v1/tests/preview", obj{"integration_type": "aws", "rule": bad}); r.Status != 422 || !strings.Contains(string(r.Raw), "unknown field") {
		t.Fatalf("expected unknown field 422, got %d %s", r.Status, r.Raw)
	}
	created := c.Must(c.Post("/api/v1/tests", obj{"name": "Confidential buckets use KMS and versioning", "control_id": ctl["id"], "integration_type": "aws", "severity": "high", "rule": rule,
		"remediation": obj{"summary": "Use KMS and versioning", "steps": []string{"Enable versioning"}}}), 201).Body
	testutil.Eventually(t, 30*time.Second, "custom test runs", func() bool {
		return c.Get("/api/v1/tests/"+created["id"].(string)).Body["last_status"] == "pass"
	})
	// Run now works synchronously; disable dismisses from control status.
	c.Must(c.Post("/api/v1/tests/"+created["id"].(string)+"/run", obj{}), 200)
	c.Must(c.Patch("/api/v1/tests/"+created["id"].(string), obj{"enabled": false}), 200)
	schema := c.Must(c.Get("/api/v1/test-schema"), 200).Body
	if len(schema["connectors"].([]any)) != 6 {
		t.Fatalf("schema should list 6 connectors")
	}
}

func TestPhase1TenantIsolation(t *testing.T) {
	app := testutil.NewApp(t)
	app.StartWorker(t)
	a := app.Signup(t, testutil.Unique("ta")+"@example.test", testutil.Unique("Tenant A"))
	b := app.Signup(t, testutil.Unique("tb")+"@example.test", testutil.Unique("Tenant B"))
	b.Must(b.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	integ := connectAWS(t, b)
	ctl := controlByCode(t, b, "GOV-01")
	ev := b.Must(b.Upload("/api/v1/controls/"+ctl["id"].(string)+"/evidence", "file", "policy.txt", []byte("Information security policy"), nil), 201).Body
	test := items(b.Get("/api/v1/tests"))[0]

	for _, path := range []string{
		"/api/v1/controls/" + ctl["id"].(string),
		"/api/v1/integrations/" + integ["id"].(string),
		"/api/v1/integrations/" + integ["id"].(string) + "/snapshot",
		"/api/v1/evidence/" + ev["id"].(string),
		"/api/v1/evidence/" + ev["id"].(string) + "/download",
		"/api/v1/tests/" + test["id"].(string),
		"/api/v1/test-runs/" + test["last_run_id"].(string),
	} {
		if r := a.Get(path); r.Status != 404 {
			t.Errorf("cross-tenant GET %s should be 404, got %d", path, r.Status)
		}
	}
	if r := a.Post("/api/v1/integrations/"+integ["id"].(string)+"/sandbox/scenarios", obj{"key": "public_bucket", "active": true}); r.Status != 404 {
		t.Errorf("cross-tenant scenario should be 404, got %d", r.Status)
	}
	if r := a.Delete("/api/v1/integrations/" + integ["id"].(string)); r.Status != 404 {
		t.Errorf("cross-tenant disconnect should be 404, got %d", r.Status)
	}
	if r := a.Post("/api/v1/evidence/"+ev["id"].(string)+"/review", obj{"verdict": "fail"}); r.Status != 404 {
		t.Errorf("cross-tenant review should be 404, got %d", r.Status)
	}
	if len(items(a.Get("/api/v1/controls"))) != 0 || len(items(a.Get("/api/v1/integrations"))) != 0 || len(items(a.Get("/api/v1/tests"))) != 0 {
		t.Errorf("tenant A should see no controls/integrations/tests")
	}
}

func TestAuditorReadOnlyOnPhase1(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.Signup(t, testutil.Unique("adm")+"@example.test", testutil.Unique("RBAC Co"))
	admin.Must(admin.Post("/api/v1/frameworks/soc2/adopt", obj{}), 200)
	audEmail := testutil.Unique("aud") + "@example.test"
	admin.Must(admin.Post("/api/v1/org/invites", obj{"email": audEmail, "role": "auditor"}), 201)
	aud := app.Client(t)
	aud.Must(aud.Post("/api/v1/auth/signup", obj{"email": audEmail, "password": "correct-horse-battery", "name": "Aud", "invite_token": inviteTokens(t, app)[audEmail]}), 201)
	ctl := items(aud.Must(aud.Get("/api/v1/controls"), 200))[0]
	aud.Must(aud.Get("/api/v1/frameworks/soc2"), 200)
	aud.Must(aud.Get("/api/v1/dashboard"), 200)
	for _, r := range []testutil.Resp{
		aud.Post("/api/v1/frameworks/iso27001/adopt", obj{}),
		aud.Patch("/api/v1/controls/"+ctl["id"].(string), obj{"owner_membership_id": ""}),
		aud.Post("/api/v1/integrations", obj{"type": "aws", "credentials": awsCreds}),
		aud.Upload("/api/v1/controls/"+ctl["id"].(string)+"/evidence", "file", "x.txt", []byte("x"), nil),
	} {
		if r.Status != 403 {
			t.Errorf("auditor mutation should be 403, got %d %s", r.Status, r.Raw)
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
