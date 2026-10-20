package connectors

import (
	"context"
	"testing"
)

func TestLifecycleScenarios(t *testing.T) {
	st, err := ApplyScenarios("hris", "acme", []string{"terminate:tom@acme.test", "hire_new_employee"})
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := (&Fake{Type: "hris", State: st}).Sync(context.Background())
	var tom, nina map[string]any
	for _, e := range snap.Resources["employee"] {
		switch e["email"] {
		case "tom@acme.test":
			tom = e
		case "nina@acme.test":
			nina = e
		}
	}
	if tom["status"] != "terminated" || tom["termination_date"] == nil {
		t.Fatalf("tom not terminated: %v", tom)
	}
	if nina == nil || nina["status"] != "active" {
		t.Fatalf("nina not hired: %v", nina)
	}
	ok, _ := ApplyScenarios("okta", "acme", []string{"deactivate:tom@acme.test"})
	snap, _ = (&Fake{Type: "okta", State: ok}).Sync(context.Background())
	for _, u := range snap.Resources["user"] {
		if u["email"] == "tom@acme.test" && u["status"] != "DEPROVISIONED" {
			t.Fatal("tom should be deprovisioned in okta")
		}
	}
	if _, err := ApplyScenarios("okta", "acme", []string{"deactivate"}); err == nil {
		t.Fatal("per-person scenario without a person must be rejected")
	}
	if _, err := ApplyScenarios("okta", "acme", []string{"public_bucket"}); err == nil {
		t.Fatal("unknown scenario must be rejected")
	}
	gh, _ := ApplyScenarios("github", "acme", []string{"remove_member:tom@acme.test"})
	snap, _ = (&Fake{Type: "github", State: gh}).Sync(context.Background())
	for _, m := range snap.Resources["member"] {
		if m["email"] == "tom@acme.test" {
			t.Fatal("tom should be removed from github")
		}
	}
	aws, _ := ApplyScenarios("aws", "acme", []string{"remove_user:tom@acme.test"})
	snap, _ = (&Fake{Type: "aws", State: aws}).Sync(context.Background())
	for _, u := range snap.Resources["iam_user"] {
		if u["id"] == "tom" {
			t.Fatal("tom's IAM user should be deleted")
		}
	}
}

func TestRegistryListsPerPersonScenarios(t *testing.T) {
	found := false
	for _, s := range Types["hris"].Scenarios {
		if s.Key == "terminate" && s.PerPerson {
			found = true
		}
	}
	if !found {
		t.Fatal("registry must expose per-person scenarios")
	}
}
