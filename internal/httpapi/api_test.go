package httpapi_test

import (
	"os"
	"strings"
	"testing"

	"github.com/trustline/trustline/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

func TestSignupLoginLogout(t *testing.T) {
	app := testutil.NewApp(t)
	email := testutil.Unique("alice") + "@example.test"
	c := app.Client(t)

	// Validation
	r := c.Post("/api/v1/auth/signup", map[string]any{"email": "bad", "password": "short", "name": "", "org_name": ""})
	if r.Status != 422 {
		t.Fatalf("expected 422 got %d %s", r.Status, r.Raw)
	}
	fields := r.Body["error"].(map[string]any)["fields"].(map[string]any)
	for _, f := range []string{"email", "password", "name", "org_name"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("expected validation error for %s", f)
		}
	}

	r = c.Must(c.Post("/api/v1/auth/signup", map[string]any{"email": email, "password": "correct-horse-battery", "name": "Alice", "org_name": "Alice Inc"}), 201)
	if r.Body["role"] != "admin" {
		t.Fatalf("signup should make admin, got %v", r.Body["role"])
	}
	if r.Body["org"].(map[string]any)["slug"] != "alice-inc" {
		t.Fatalf("unexpected slug %v", r.Body["org"])
	}
	// Duplicate email
	r = app.Client(t).Post("/api/v1/auth/signup", map[string]any{"email": strings.ToUpper(email), "password": "correct-horse-battery", "name": "Alice", "org_name": "Dup"})
	if r.Status != 422 {
		t.Fatalf("duplicate signup should be 422, got %d %s", r.Status, r.Raw)
	}

	c.Must(c.Get("/api/v1/me"), 200)
	c.Must(c.Post("/api/v1/auth/logout", map[string]any{}), 200)
	c.Must(c.Get("/api/v1/me"), 401)

	c2 := app.Client(t)
	if r := c2.Post("/api/v1/auth/login", map[string]any{"email": email, "password": "wrong"}); r.Status != 401 {
		t.Fatalf("wrong password should be 401, got %d", r.Status)
	}
	r = c2.Must(c2.Post("/api/v1/auth/login", map[string]any{"email": email, "password": "correct-horse-battery"}), 200)
	if r.Body["org"] == nil {
		t.Fatal("login should select an org")
	}
	c2.Must(c2.Get("/api/v1/org"), 200)
}

func TestLoginRateLimit(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.Client(t)
	email := testutil.Unique("rl") + "@example.test"
	var last int
	for i := 0; i < 12; i++ {
		last = c.Post("/api/v1/auth/login", map[string]any{"email": email, "password": "nope"}).Status
	}
	if last != 429 {
		t.Fatalf("expected 429 after repeated failures, got %d", last)
	}
	// Successful sign-ins do not count toward the per-email limit.
	ok := testutil.Unique("okrl") + "@example.test"
	app.Signup(t, ok, testutil.Unique("RL Co"))
	for i := 0; i < 15; i++ {
		c.Must(c.Post("/api/v1/auth/login", map[string]any{"email": ok, "password": "correct-horse-battery"}), 200)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	app := testutil.NewApp(t)
	a := app.Signup(t, testutil.Unique("a")+"@example.test", "Org A")
	b := app.Signup(t, testutil.Unique("b")+"@example.test", "Org B")

	// Org B creates an invite and has a member and activity.
	inv := b.Must(b.Post("/api/v1/org/invites", map[string]any{"email": testutil.Unique("inv") + "@example.test", "role": "editor"}), 201)
	invID := inv.Body["id"].(string)
	members := b.Must(b.Get("/api/v1/org/members"), 200)
	bMemberID := members.Body["items"].([]any)[0].(map[string]any)["id"].(string)
	bOrgID := b.Must(b.Get("/api/v1/org"), 200).Body["id"].(string)

	// Org A cannot see any of it.
	if r := a.Get("/api/v1/org/members/" + bMemberID); r.Status != 404 {
		t.Fatalf("cross-tenant member read should be 404, got %d %s", r.Status, r.Raw)
	}
	if r := a.Patch("/api/v1/org/members/"+bMemberID, map[string]any{"role": "employee"}); r.Status != 404 {
		t.Fatalf("cross-tenant member write should be 404, got %d", r.Status)
	}
	if r := a.Delete("/api/v1/org/invites/" + invID); r.Status != 404 {
		t.Fatalf("cross-tenant invite revoke should be 404, got %d", r.Status)
	}
	if r := a.Post("/api/v1/org/invites/"+invID+"/resend", map[string]any{}); r.Status != 404 {
		t.Fatalf("cross-tenant invite resend should be 404, got %d", r.Status)
	}
	// A's member list contains only A's user.
	am := a.Must(a.Get("/api/v1/org/members"), 200).Body["items"].([]any)
	if len(am) != 1 || am[0].(map[string]any)["id"] == bMemberID {
		t.Fatalf("org A sees wrong members: %v", am)
	}
	// A's activity contains no B entries.
	act := a.Must(a.Get("/api/v1/activity"), 200).Body["items"].([]any)
	for _, e := range act {
		if e.(map[string]any)["org_id"] == bOrgID {
			t.Fatalf("org A activity leaked org B entry: %v", e)
		}
	}
	// A cannot switch into B.
	if r := a.Post("/api/v1/me/switch-org", map[string]any{"org_id": bOrgID}); r.Status != 404 {
		t.Fatalf("switch to foreign org should be 404, got %d", r.Status)
	}
	// B's invite still exists (A's 404s did not mutate anything).
	if n := len(b.Must(b.Get("/api/v1/org/invites"), 200).Body["items"].([]any)); n != 1 {
		t.Fatalf("org B should still have 1 invite, got %d", n)
	}
}

func TestInviteFlowAndRBAC(t *testing.T) {
	app := testutil.NewApp(t)
	admin := app.Signup(t, testutil.Unique("admin")+"@example.test", "Invite Co")
	editorEmail := testutil.Unique("editor") + "@example.test"
	auditorEmail := testutil.Unique("auditor") + "@example.test"

	admin.Must(admin.Post("/api/v1/org/invites", map[string]any{"email": editorEmail, "role": "editor"}), 201)
	admin.Must(admin.Post("/api/v1/org/invites", map[string]any{"email": auditorEmail, "role": "auditor"}), 201)

	// Pull tokens out of the enqueued email jobs (the worker is not running here).
	tokens := inviteTokens(t, app)
	if tokens[editorEmail] == "" || tokens[auditorEmail] == "" {
		t.Fatalf("expected invite emails for both invitees, got %v", tokens)
	}

	// Public preview
	pub := app.Client(t)
	p := pub.Must(pub.Get("/api/v1/invites/"+tokens[editorEmail]), 200)
	if p.Body["org_name"] != "Invite Co" || p.Body["role"] != "editor" {
		t.Fatalf("unexpected preview %v", p.Body)
	}
	if r := pub.Get("/api/v1/invites/not-a-token"); r.Status != 404 {
		t.Fatalf("bad token should 404, got %d", r.Status)
	}

	// Editor signs up through the invite.
	editor := app.Client(t)
	r := editor.Must(editor.Post("/api/v1/auth/signup", map[string]any{"email": editorEmail, "password": "correct-horse-battery", "name": "Ed", "invite_token": tokens[editorEmail]}), 201)
	if r.Body["role"] != "editor" {
		t.Fatalf("expected editor role, got %v", r.Body["role"])
	}
	// Token is single-use.
	if r := pub.Get("/api/v1/invites/" + tokens[editorEmail]); r.Status != 404 {
		t.Fatalf("used token should 404, got %d", r.Status)
	}
	// Invite list no longer shows it.
	if n := len(admin.Must(admin.Get("/api/v1/org/invites"), 200).Body["items"].([]any)); n != 1 {
		t.Fatalf("expected 1 pending invite, got %d", n)
	}

	// Editor: can read org, cannot manage members or org settings.
	editor.Must(editor.Get("/api/v1/org"), 200)
	editor.Must(editor.Get("/api/v1/org/members"), 200)
	if r := editor.Post("/api/v1/org/invites", map[string]any{"email": "x@example.test", "role": "editor"}); r.Status != 403 {
		t.Fatalf("editor should not invite, got %d", r.Status)
	}
	if r := editor.Patch("/api/v1/org", map[string]any{"name": "Hacked"}); r.Status != 403 {
		t.Fatalf("editor should not rename org, got %d", r.Status)
	}

	// Auditor signs up with the wrong email → rejected; with the right one → read-only.
	auditor := app.Client(t)
	if r := auditor.Post("/api/v1/auth/signup", map[string]any{"email": testutil.Unique("other") + "@example.test", "password": "correct-horse-battery", "name": "X", "invite_token": tokens[auditorEmail]}); r.Status != 422 {
		t.Fatalf("mismatched email should be 422, got %d %s", r.Status, r.Raw)
	}
	auditor.Must(auditor.Post("/api/v1/auth/signup", map[string]any{"email": auditorEmail, "password": "correct-horse-battery", "name": "Au", "invite_token": tokens[auditorEmail]}), 201)
	auditor.Must(auditor.Get("/api/v1/activity"), 200)
	auditor.Must(auditor.Get("/api/v1/org/members"), 200)
	if r := auditor.Patch("/api/v1/org", map[string]any{"name": "Nope"}); r.Status != 403 {
		t.Fatalf("auditor must be read-only, got %d", r.Status)
	}

	// Admin changes editor's role, cannot demote the last admin, cannot remove self.
	members := admin.Must(admin.Get("/api/v1/org/members"), 200).Body["items"].([]any)
	var adminMID, editorMID string
	for _, m := range members {
		mm := m.(map[string]any)
		switch mm["email"] {
		case editorEmail:
			editorMID = mm["id"].(string)
		default:
			if mm["role"] == "admin" {
				adminMID = mm["id"].(string)
			}
		}
	}
	admin.Must(admin.Patch("/api/v1/org/members/"+editorMID, map[string]any{"role": "employee"}), 200)
	if r := admin.Patch("/api/v1/org/members/"+adminMID, map[string]any{"role": "editor"}); r.Status != 422 {
		t.Fatalf("demoting last admin should be 422, got %d %s", r.Status, r.Raw)
	}
	if r := admin.Delete("/api/v1/org/members/" + adminMID); r.Status != 422 {
		t.Fatalf("removing self should be 422, got %d", r.Status)
	}
	// Now-employee cannot read members.
	if r := editor.Get("/api/v1/org/members"); r.Status != 403 {
		t.Fatalf("employee should not list members, got %d", r.Status)
	}
	admin.Must(admin.Delete("/api/v1/org/members/"+editorMID), 200)
	// Removed user has no org any more.
	if r := editor.Get("/api/v1/org"); r.Status != 403 {
		t.Fatalf("removed member should get 403 no_org, got %d", r.Status)
	}

	// Activity log recorded the chain.
	act := admin.Must(admin.Get("/api/v1/activity?limit=100"), 200).Body["items"].([]any)
	want := map[string]bool{"org.created": false, "invite.created": false, "invite.accepted": false, "member.role_changed": false, "member.removed": false}
	for _, e := range act {
		want[e.(map[string]any)["action"].(string)] = true
	}
	for k, v := range want {
		if !v {
			t.Errorf("activity missing %s", k)
		}
	}
}

func TestActivityLogIsImmutable(t *testing.T) {
	app := testutil.NewApp(t)
	app.Signup(t, testutil.Unique("imm")+"@example.test", "Immutable")
	if _, err := app.Pool.Exec(t.Context(), `UPDATE activity_log SET action = 'x' WHERE action = 'org.created'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only error, got %v", err)
	}
	if _, err := app.Pool.Exec(t.Context(), `DELETE FROM activity_log`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only error on delete, got %v", err)
	}
}

func TestPasswordReset(t *testing.T) {
	app := testutil.NewApp(t)
	email := testutil.Unique("reset") + "@example.test"
	c := app.Signup(t, email, "Reset Co")
	c.Must(c.Post("/api/v1/auth/forgot-password", map[string]any{"email": email}), 200)
	// Unknown email also 200 (no enumeration).
	c.Must(c.Post("/api/v1/auth/forgot-password", map[string]any{"email": "nobody@example.test"}), 200)
	token := ""
	for _, j := range pendingEmailJobs(t, app) {
		if j.To == email && strings.Contains(j.Subject, "Reset") {
			token = extractParam(j.Text, "token=")
		}
	}
	if token == "" {
		t.Fatal("no reset email enqueued")
	}
	n := app.Client(t)
	n.Must(n.Post("/api/v1/auth/reset-password", map[string]any{"token": token, "password": "new-correct-horse-battery"}), 200)
	// Old session revoked, token single-use, new password works.
	c.Must(c.Get("/api/v1/me"), 401)
	if r := n.Post("/api/v1/auth/reset-password", map[string]any{"token": token, "password": "another-long-password"}); r.Status != 422 {
		t.Fatalf("reused token should be 422, got %d", r.Status)
	}
	n.Must(n.Post("/api/v1/auth/login", map[string]any{"email": email, "password": "new-correct-horse-battery"}), 200)
}

func TestGoogleOAuthFakeFlow(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.Client(t)
	// Start → redirect to fake consent page carrying state.
	r := c.Get("/auth/google/start")
	if r.Status != 302 {
		t.Fatalf("expected redirect, got %d", r.Status)
	}
	loc := r.Header.Get("Location")
	if !strings.Contains(loc, "/auth/fake-google/authorize?state=") {
		t.Fatalf("unexpected location %s", loc)
	}
	state := loc[strings.Index(loc, "state=")+6:]
	// Consent page lists accounts.
	page := c.Get("/auth/fake-google/authorize?state=" + state)
	if page.Status != 200 || !strings.Contains(string(page.Raw), "ada@acme.test") {
		t.Fatalf("consent page broken: %d", page.Status)
	}
	// Submit the form (as the browser would).
	form := c.DoForm("/auth/fake-google/authorize", map[string]string{"state": state, "email": "ada@acme.test", "name": "Ada Lovelace"})
	if form.Status != 302 {
		t.Fatalf("expected redirect to callback, got %d", form.Status)
	}
	cb := form.Header.Get("Location")
	if !strings.Contains(cb, "/auth/google/callback?code=") {
		t.Fatalf("unexpected callback %s", cb)
	}
	cbr := c.Get(cb[strings.Index(cb, "/auth/google/callback"):])
	if cbr.Status != 302 || cbr.Header.Get("Location") != "/onboarding" {
		t.Fatalf("new oauth user should land on onboarding, got %d %s", cbr.Status, cbr.Header.Get("Location"))
	}
	me := c.Must(c.Get("/api/v1/me"), 200)
	if me.Body["user"].(map[string]any)["email"] != "ada@acme.test" {
		t.Fatalf("unexpected me %v", me.Body)
	}
	// Create org from onboarding, then everything tenant-scoped works.
	c.Must(c.Post("/api/v1/orgs", map[string]any{"name": "Ada Org"}), 201)
	c.Must(c.Get("/api/v1/org/members"), 200)
}

func TestOrgSettingsValidation(t *testing.T) {
	app := testutil.NewApp(t)
	c := app.Signup(t, testutil.Unique("set")+"@example.test", "Settings Co")
	r := c.Patch("/api/v1/org", map[string]any{"settings": map[string]any{"access_review_days": 0, "password_min_length": 12, "policy_acceptance_threshold": 90, "ai_enabled": true, "timezone": "Mars/Olympus"}})
	if r.Status != 422 {
		t.Fatalf("expected 422 got %d %s", r.Status, r.Raw)
	}
	r = c.Must(c.Patch("/api/v1/org", map[string]any{"name": "Renamed", "settings": map[string]any{"access_review_days": 180, "password_min_length": 14, "policy_acceptance_threshold": 95, "ai_enabled": false, "timezone": "Europe/Amsterdam"}}), 200)
	if r.Body["name"] != "Renamed" || r.Body["settings"].(map[string]any)["access_review_days"].(float64) != 180 {
		t.Fatalf("update not applied: %s", r.Raw)
	}
}

func TestDuplicateOrgNamesGetDistinctSlugs(t *testing.T) {
	app := testutil.NewApp(t)
	name := testutil.Unique("Dup Org")
	a := app.Signup(t, testutil.Unique("a")+"@example.test", name)
	b := app.Signup(t, testutil.Unique("b")+"@example.test", name)
	c := app.Signup(t, testutil.Unique("c")+"@example.test", name)
	sa := a.Must(a.Get("/api/v1/org"), 200).Body["slug"].(string)
	sb := b.Must(b.Get("/api/v1/org"), 200).Body["slug"].(string)
	sc := c.Must(c.Get("/api/v1/org"), 200).Body["slug"].(string)
	if sa == sb || sb == sc || sa == sc || sa == "" {
		t.Fatalf("slugs must differ: %q %q %q", sa, sb, sc)
	}
}
