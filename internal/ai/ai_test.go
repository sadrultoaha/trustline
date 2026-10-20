package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	in := "user alice@acme.test key AKIAABCDEFGHIJKLMNOP phone +31 20 123 4567 ssn 123-45-6789"
	out := Redact(in)
	for _, leak := range []string{"alice@acme.test", "AKIAABCDEFGHIJKLMNOP", "123-45-6789"} {
		if strings.Contains(out, leak) {
			t.Errorf("leaked %s in %s", leak, out)
		}
	}
	if !strings.Contains(out, "[email]") || !strings.Contains(out, "[aws-access-key]") {
		t.Errorf("placeholders missing: %s", out)
	}
}

func TestPromptsRender(t *testing.T) {
	sys, usr, err := render("remediation_plan.v1", RemediationInput{ControlCode: "DAT-01", TestName: "x", Failing: []Failing{{ID: "b1", Reason: "public"}}, IaCHint: "terraform:aws_s3_bucket_public_access_block"})
	if err != nil || sys == "" || !strings.Contains(usr, "b1: public") {
		t.Fatalf("render: %v %q", err, usr)
	}
	if _, _, err := render("evidence_validation.v1", EvidenceInput{ControlCode: "IAM-01"}); err != nil {
		t.Fatal(err)
	}
}

func TestFakeRemediationTerraform(t *testing.T) {
	resp, err := Fake{}.Generate(context.Background(), Request{Task: "remediation_plan", Input: RemediationInput{
		ControlCode: "DAT-01", TestName: "S3 buckets block public access", Summary: "Enable Block Public Access.",
		Steps: []string{"Open S3"}, IaCHint: "terraform:aws_s3_bucket_public_access_block",
		Failing: []Failing{{ID: "acme-marketing-assets", Reason: "public"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var plan RemediationPlan
	_ = json.Unmarshal(resp.JSON, &plan)
	if !strings.Contains(plan.Terraform, `resource "aws_s3_bucket_public_access_block" "acme_marketing_assets"`) ||
		!strings.Contains(plan.Terraform, `bucket                  = "acme-marketing-assets"`) {
		t.Fatalf("terraform: %s", plan.Terraform)
	}
	if len(plan.Steps) < 3 || !strings.Contains(plan.Steps[0], "acme-marketing-assets") {
		t.Fatalf("steps: %v", plan.Steps)
	}
}

func TestFakeEvidence(t *testing.T) {
	in := EvidenceInput{ControlCode: "IAM-01", ControlName: "MFA enforced", Keywords: []string{"mfa", "multi-factor"}, EvidenceHint: "IdP MFA report"}
	in.Filename = "okta-mfa-enrollment.png"
	if v := fakeEvidence(in, nil); v.Verdict != "pass" {
		t.Fatalf("expected pass: %+v", v)
	}
	in.Filename = "holiday-photo.png"
	if v := fakeEvidence(in, []Attachment{{MediaType: "text/plain", Data: []byte("our team lunch")}}); v.Verdict != "fail" {
		t.Fatalf("expected fail: %+v", v)
	}
	if v := fakeEvidence(in, []Attachment{{MediaType: "text/plain", Data: []byte("Multi-Factor authentication enforced for all users")}}); v.Verdict != "pass" {
		t.Fatalf("expected pass from content: %+v", v)
	}
}

// Every IaC hint used by the test library yields a concrete snippet.
func TestFakeTerraformCoversCatalogHints(t *testing.T) {
	hints := []string{"aws_s3_bucket_public_access_block", "aws_s3_bucket_policy", "aws_db_instance", "github_branch_protection", "google_storage_bucket",
		"okta_policy_rule_signon", "aws_vpc_security_group_ingress_rule", "aws_cloudtrail", "aws_iam_access_key", "googleworkspace_user"}
	for _, h := range hints {
		tf := terraformFor(h, []string{"acme/api"})
		if !strings.Contains(tf, "resource \"") || strings.Contains(tf, "compliant configuration described in the steps above") {
			t.Errorf("%s: weak snippet:\n%s", h, tf)
		}
	}
}

func TestFakeConsistencyFindsMismatches(t *testing.T) {
	text := "User access to production systems is reviewed every 90 days by system owners. Passwords must be at least 12 characters long. At least 90% of personnel must acknowledge this policy."
	rep := fakeConsistency(ConsistencyInput{Title: "Access Control", Text: text, Facts: []Fact{
		{Key: "access_review_days", Label: "Access review cadence (days)", Value: "180"},
		{Key: "password_min_length", Label: "Minimum password length", Value: "12"},
		{Key: "policy_acceptance_threshold", Label: "Acknowledgement threshold (%)", Value: "95"},
	}})
	if len(rep.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %+v", rep.Issues)
	}
	if rep.Issues[0].Setting != "access_review_days" || rep.Issues[0].PolicyValue != "90" || !strings.Contains(rep.Issues[0].Quote, "reviewed every 90 days") {
		t.Fatalf("access review issue wrong: %+v", rep.Issues[0])
	}
	same := fakeConsistency(ConsistencyInput{Text: text, Facts: []Fact{{Key: "access_review_days", Value: "90"}}})
	if len(same.Issues) != 0 {
		t.Fatalf("matching values must not be reported: %+v", same.Issues)
	}
}

func TestPolicyPromptsRender(t *testing.T) {
	if _, _, err := render("policy_draft.v1", PolicyDraftInput{Title: "X", Systems: []string{"AWS"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := render("policy_consistency.v1", ConsistencyInput{Facts: []Fact{{Key: "a", Value: "1"}}}); err != nil {
		t.Fatal(err)
	}
	d := fakePolicyDraft(PolicyDraftInput{OrgName: "Acme", Title: "Access Control Policy", Systems: []string{"Okta", "AWS"}, TemplateMarkdown: "# Access\n\n## Policy\n\ntext\n\n## Review\n\nyearly"})
	if !strings.Contains(d.Markdown, "## Systems in scope") || strings.Index(d.Markdown, "Systems in scope") > strings.Index(d.Markdown, "## Review") {
		t.Fatalf("draft not tailored: %s", d.Markdown)
	}
}
