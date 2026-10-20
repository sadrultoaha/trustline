package testengine

import (
	"encoding/json"
	"testing"
)

func res(js string) []Resource {
	var out []Resource
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		panic(err)
	}
	return out
}

func mustParse(t *testing.T, js string) *Rule {
	t.Helper()
	r, err := Parse([]byte(js), nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var buckets = res(`[
 {"id":"logs","public_access_block":{"block_public_acls":true,"restrict_public_buckets":true},"encryption":{"enabled":true},"tags":{"env":"prod"}},
 {"id":"assets","public_access_block":{"block_public_acls":false,"restrict_public_buckets":true},"encryption":{"enabled":true},"tags":{"env":"prod"}},
 {"id":"scratch","public_access_block":{"block_public_acls":true,"restrict_public_buckets":true},"encryption":{"enabled":false},"tags":{"env":"dev"}}
]`)

func TestAllQuantifierAndReasons(t *testing.T) {
	r := mustParse(t, `{"resource":"s3_bucket","quantifier":"all","assert":{"and":[
		{"field":"public_access_block.block_public_acls","op":"eq","value":true},
		{"field":"public_access_block.restrict_public_buckets","op":"eq","value":true}]}}`)
	got := r.Evaluate(buckets)
	if got.Status != "fail" || len(got.Failing) != 1 || got.Failing[0].ID != "assets" {
		t.Fatalf("unexpected %+v", got)
	}
	if got.Failing[0].Reason != "public_access_block.block_public_acls = false (expected = true)" {
		t.Fatalf("reason: %q", got.Failing[0].Reason)
	}
}

func TestWhereFilter(t *testing.T) {
	r := mustParse(t, `{"resource":"s3_bucket","quantifier":"all","where":{"field":"tags.env","op":"eq","value":"prod"},
		"assert":{"field":"encryption.enabled","op":"eq","value":true}}`)
	got := r.Evaluate(buckets)
	if got.Status != "pass" || got.Evaluated != 2 {
		t.Fatalf("unexpected %+v", got)
	}
	if got.Failing == nil {
		t.Fatal("failing must be an empty slice, not nil (JSON null breaks clients)")
	}
}

func TestAnyNoneCount(t *testing.T) {
	anyR := mustParse(t, `{"resource":"x","quantifier":"any","assert":{"field":"encryption.enabled","op":"eq","value":false}}`)
	if anyR.Evaluate(buckets).Status != "pass" {
		t.Fatal("any should pass")
	}
	if anyR.Evaluate(nil).Status != "fail" {
		t.Fatal("any on empty should fail")
	}
	noneR := mustParse(t, `{"resource":"x","quantifier":"none","assert":{"field":"public_access_block.block_public_acls","op":"eq","value":false}}`)
	got := noneR.Evaluate(buckets)
	if got.Status != "fail" || got.Failing[0].ID != "assets" {
		t.Fatalf("none: %+v", got)
	}
	countR := mustParse(t, `{"resource":"x","quantifier":"count","max":1,"assert":{"field":"tags.env","op":"eq","value":"prod"}}`)
	if got := countR.Evaluate(buckets); got.Status != "fail" || len(got.Failing) != 2 {
		t.Fatalf("count: %+v", got)
	}
	countMin := mustParse(t, `{"resource":"x","quantifier":"count","min":2,"assert":{"field":"tags.env","op":"eq","value":"prod"}}`)
	if countMin.Evaluate(buckets).Status != "pass" {
		t.Fatal("count min should pass")
	}
}

func TestEmptyAllPasses(t *testing.T) {
	r := mustParse(t, `{"resource":"x","quantifier":"all","assert":{"field":"a","op":"eq","value":1}}`)
	if r.Evaluate(nil).Status != "pass" {
		t.Fatal("vacuous all should pass")
	}
}

func TestFanoutAndOperators(t *testing.T) {
	users := res(`[
	 {"id":"alice","access_keys":[{"active":true,"age_days":10},{"active":false,"age_days":400}],"groups":["eng","admins"],"email":"alice@acme.test","hire":"2024-01-10"},
	 {"id":"bob","access_keys":[{"active":true,"age_days":120}],"groups":["eng"],"email":"bob@other.test","hire":"2026-03-01"},
	 {"id":"carol","access_keys":[],"groups":[],"email":"carol@acme.test"}
	]`)
	cases := []struct {
		name string
		rule string
		fail []string
	}{
		{"fanout all", `{"field":"access_keys[*].age_days","op":"lte","value":90}`, []string{"alice", "bob"}},
		{"fanout any", `{"field":"access_keys[*].age_days","op":"lte","value":90,"match":"any"}`, []string{"bob", "carol"}},
		{"contains list", `{"field":"groups","op":"contains","value":"eng"}`, []string{"carol"}},
		{"not_contains", `{"field":"groups","op":"not_contains","value":"admins"}`, []string{"alice"}},
		{"matches", `{"field":"email","op":"matches","value":"@acme\\.test$"}`, []string{"bob"}},
		{"in", `{"field":"id","op":"in","value":["alice","bob"]}`, []string{"carol"}},
		{"nin", `{"field":"id","op":"nin","value":["alice"]}`, []string{"alice"}},
		{"exists", `{"field":"hire","op":"exists"}`, []string{"carol"}},
		{"not_exists", `{"field":"hire","op":"not_exists"}`, []string{"alice", "bob"}},
		{"len_gt", `{"field":"groups","op":"len_gt","value":0}`, []string{"carol"}},
		{"date lt", `{"field":"hire","op":"lt","value":"2025-01-01"}`, []string{"bob", "carol"}},
		{"not", `{"not":{"field":"id","op":"eq","value":"bob"}}`, []string{"bob"}},
		{"or", `{"or":[{"field":"id","op":"eq","value":"bob"},{"field":"groups","op":"len_gt","value":1}]}`, []string{"carol"}},
	}
	for _, c := range cases {
		r := mustParse(t, `{"resource":"iam_user","quantifier":"all","assert":`+c.rule+`}`)
		got := r.Evaluate(users)
		var ids []string
		for _, f := range got.Failing {
			ids = append(ids, f.ID)
		}
		if len(ids) != len(c.fail) {
			t.Errorf("%s: failing %v want %v", c.name, ids, c.fail)
			continue
		}
		for i := range ids {
			if ids[i] != c.fail[i] {
				t.Errorf("%s: failing %v want %v", c.name, ids, c.fail)
			}
		}
	}
}

func TestValidation(t *testing.T) {
	bad := []string{
		`{"resource":"","quantifier":"all","assert":{"field":"a","op":"eq","value":1}}`,
		`{"resource":"x","quantifier":"most","assert":{"field":"a","op":"eq","value":1}}`,
		`{"resource":"x","quantifier":"all"}`,
		`{"resource":"x","quantifier":"all","assert":{"field":"a","op":"near","value":1}}`,
		`{"resource":"x","quantifier":"all","assert":{"field":"a","op":"in","value":1}}`,
		`{"resource":"x","quantifier":"all","assert":{"field":"a","op":"matches","value":"("}}`,
		`{"resource":"x","quantifier":"all","assert":{"and":[]}}`,
		`{"resource":"x","quantifier":"all","assert":{"and":[{"field":"a","op":"eq","value":1}],"field":"b","op":"eq","value":1}}`,
		`{"resource":"x","quantifier":"count","assert":{"field":"a","op":"eq","value":1}}`,
		`{"resource":"x","quantifier":"all","assert":{"field":"a","op":"eq","value":1},"extra":true}`,
	}
	for _, b := range bad {
		if _, err := Parse([]byte(b), nil); err == nil {
			t.Errorf("expected error for %s", b)
		}
	}
	fields := func(resource, f string) bool { return f == "known" }
	if _, err := Parse([]byte(`{"resource":"x","quantifier":"all","assert":{"field":"unknown","op":"eq","value":1}}`), fields); err == nil {
		t.Error("expected unknown field error")
	}
}
