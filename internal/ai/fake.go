package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Fake is a deterministic provider used in development, tests and CI. It
// produces the same structured outputs as the real model from the request's
// structured input, so every AI-backed flow runs end to end offline.
type Fake struct{}

func (Fake) Name() string { return "fake" }

func (Fake) Generate(ctx context.Context, req Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out any
	switch req.Task {
	case "remediation_plan":
		in, ok := req.Input.(RemediationInput)
		if !ok {
			return nil, fmt.Errorf("fake: remediation_plan needs RemediationInput")
		}
		out = fakeRemediation(in)
	case "evidence_validation":
		in, ok := req.Input.(EvidenceInput)
		if !ok {
			return nil, fmt.Errorf("fake: evidence_validation needs EvidenceInput")
		}
		out = fakeEvidence(in, req.Attachments)
	case "policy_draft":
		in, ok := req.Input.(PolicyDraftInput)
		if !ok {
			return nil, fmt.Errorf("fake: policy_draft needs PolicyDraftInput")
		}
		out = fakePolicyDraft(in)
	case "policy_consistency":
		in, ok := req.Input.(ConsistencyInput)
		if !ok {
			return nil, fmt.Errorf("fake: policy_consistency needs ConsistencyInput")
		}
		out = fakeConsistency(in)
	case "vendor_assessment":
		in, ok := req.Input.(VendorAssessmentInput)
		if !ok {
			return nil, fmt.Errorf("fake: vendor_assessment needs VendorAssessmentInput")
		}
		out = fakeVendorAssessment(in, req.Attachments)
	case "risk_assessment":
		in, ok := req.Input.(RiskAssessmentInput)
		if !ok {
			return nil, fmt.Errorf("fake: risk_assessment needs RiskAssessmentInput")
		}
		out = fakeRiskAssessment(in)
	default:
		return nil, fmt.Errorf("fake: unsupported task %q", req.Task)
	}
	b, _ := json.Marshal(out)
	// Token estimates keep cost logging meaningful in development.
	return &Response{JSON: b, Model: "fake-1", InputTokens: int64(len(req.System)+len(req.Prompt)) / 4, OutputTokens: int64(len(b)) / 4}, nil
}

var nonIdent = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func tfName(id string) string {
	n := strings.Trim(nonIdent.ReplaceAllString(strings.ToLower(id), "_"), "_")
	if n == "" {
		n = "resource"
	}
	if n[0] >= '0' && n[0] <= '9' {
		n = "r_" + n
	}
	return n
}

func fakeRemediation(in RemediationInput) RemediationPlan {
	ids := make([]string, 0, len(in.Failing))
	for _, f := range in.Failing {
		if f.ID != "-" && f.ID != "?" {
			ids = append(ids, f.ID)
		}
	}
	sort.Strings(ids)
	steps := []string{}
	if len(ids) > 0 {
		steps = append(steps, fmt.Sprintf("Confirm the affected %s resources: %s.", strings.ReplaceAll(in.Resource, "_", " "), strings.Join(ids, ", ")))
	}
	steps = append(steps, in.Steps...)
	steps = append(steps, fmt.Sprintf("Run the test \"%s\" again from Trustline and confirm it passes.", in.TestName))
	hint := strings.TrimPrefix(in.IaCHint, "terraform:")
	tf := ""
	if in.IaCHint != "" {
		tf = terraformFor(hint, ids)
	}
	return RemediationPlan{
		Summary:   fmt.Sprintf("%s Affects %d resource(s) under control %s.", strings.TrimSpace(in.Summary), len(ids), in.ControlCode),
		Steps:     steps,
		Terraform: tf,
		Notes:     "Apply in a non-production environment first if the change affects network reachability or authentication. Trustline re-checks on the next sync.",
	}
}

func terraformFor(resourceType string, ids []string) string {
	if len(ids) == 0 {
		ids = []string{"example"}
	}
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteString("\n")
		}
		n := tfName(id)
		switch resourceType {
		case "aws_s3_bucket_public_access_block":
			fmt.Fprintf(&b, "resource \"aws_s3_bucket_public_access_block\" %q {\n  bucket                  = %q\n  block_public_acls       = true\n  ignore_public_acls      = true\n  block_public_policy     = true\n  restrict_public_buckets = true\n}\n", n, id)
		case "aws_s3_bucket_policy":
			fmt.Fprintf(&b, "# Remove public grants from %s and block public access at the bucket level.\nresource \"aws_s3_bucket_public_access_block\" %q {\n  bucket                  = %q\n  block_public_acls       = true\n  ignore_public_acls      = true\n  block_public_policy     = true\n  restrict_public_buckets = true\n}\n\nresource \"aws_s3_bucket_ownership_controls\" %q {\n  bucket = %q\n  rule {\n    object_ownership = \"BucketOwnerEnforced\"\n  }\n}\n", id, n, id, n, id)
		case "aws_ebs_encryption_by_default":
			b.WriteString("resource \"aws_ebs_encryption_by_default\" \"this\" {\n  enabled = true\n}\n")
			return b.String()
		case "aws_default_security_group":
			fmt.Fprintf(&b, "# Strip all rules from the default security group %s.\nresource \"aws_default_security_group\" %q {\n  vpc_id = var.vpc_id\n  # no ingress or egress blocks = deny all\n}\n", id, n)
		case "aws_iam_user_policy_attachment", "aws_iam_role_policy_attachment":
			fmt.Fprintf(&b, "# Detach AdministratorAccess from %s and grant access through a role instead.\nresource \"aws_iam_user_group_membership\" %q {\n  user   = %q\n  groups = [var.least_privilege_group]\n}\n", id, n, id)
		case "aws_iam_user_login_profile", "aws_iam_virtual_mfa_device":
			fmt.Fprintf(&b, "# MFA devices must be enrolled by %s; enforce it with a deny-without-MFA policy.\nresource \"aws_iam_user_policy_attachment\" %q {\n  user       = %q\n  policy_arn = aws_iam_policy.require_mfa.arn\n}\n", id, n+"_require_mfa", id)
		case "aws_iam_instance_profile":
			fmt.Fprintf(&b, "resource \"aws_iam_instance_profile\" %q {\n  name = \"%s-ssm\"\n  role = var.ssm_managed_instance_role # with AmazonSSMManagedInstanceCore\n}\n", n, id)
		case "github_repository_dependabot_security_updates":
			repo := id
			if i := strings.Index(id, "/"); i >= 0 {
				repo = id[i+1:]
			}
			fmt.Fprintf(&b, "resource \"github_repository_dependabot_security_updates\" %q {\n  repository = %q\n  enabled    = true\n}\n", tfName(repo), repo)
		case "github_membership", "github_repository_collaborator":
			fmt.Fprintf(&b, "# Remove or downgrade %s.\nresource \"github_membership\" %q {\n  username = %q\n  role     = \"member\"\n}\n", id, n, id)
		case "google_project_iam_audit_config":
			b.WriteString("resource \"google_project_iam_audit_config\" \"all_services\" {\n  project = var.project_id\n  service = \"allServices\"\n  audit_log_config { log_type = \"ADMIN_READ\" }\n  audit_log_config { log_type = \"DATA_READ\" }\n  audit_log_config { log_type = \"DATA_WRITE\" }\n}\n")
			return b.String()
		case "google_compute_network":
			fmt.Fprintf(&b, "# Delete the default network in %s; define explicit VPCs instead.\nresource \"google_compute_network\" \"main\" {\n  name                    = \"main\"\n  auto_create_subnetworks = false\n}\n", id)
		case "google_org_policy_policy":
			fmt.Fprintf(&b, "resource \"google_org_policy_policy\" %q {\n  name   = \"projects/%s/policies/compute.requireOsLogin\"\n  parent = \"projects/%s\"\n  spec {\n    rules {\n      enforce = \"TRUE\"\n    }\n  }\n}\n", n, id, id)
		case "google_scc_notification_config":
			b.WriteString("resource \"google_scc_notification_config\" \"high_findings\" {\n  config_id    = \"high-findings\"\n  organization = var.org_id\n  pubsub_topic = var.findings_topic\n  streaming_config {\n    filter = \"severity=\\\"HIGH\\\" OR severity=\\\"CRITICAL\\\"\"\n  }\n}\n")
			return b.String()
		case "okta_user_admin_roles":
			fmt.Fprintf(&b, "resource \"okta_user_admin_roles\" %q {\n  user_id     = var.user_id # %s\n  admin_roles = [] # remove standing admin; grant just-in-time instead\n}\n", n, id)
		case "okta_group_rule", "okta_app_saml", "okta_app_oauth", "okta_app_oauth_api_scope":
			fmt.Fprintf(&b, "resource \"okta_app_saml\" %q {\n  label             = %q\n  preconfigured_app = var.okta_catalog_app_name\n  status            = \"ACTIVE\"\n}\n", n, id)
		case "googleworkspace_user":
			fmt.Fprintf(&b, "resource \"googleworkspace_user\" %q {\n  primary_email = %q\n  suspended     = true\n  name {\n    family_name = var.family_name\n    given_name  = var.given_name\n  }\n}\n", n, id)
		case "googleworkspace_role_assignment":
			fmt.Fprintf(&b, "resource \"googleworkspace_role_assignment\" %q {\n  role_id     = var.super_admin_role_id\n  assigned_to = var.backup_admin_user_id # in addition to %s\n}\n", n, id)
		case "aws_s3_bucket_logging":
			fmt.Fprintf(&b, "resource \"aws_s3_bucket_logging\" %q {\n  bucket        = %q\n  target_bucket = var.access_log_bucket\n  target_prefix = \"s3/%s/\"\n}\n", n, id, id)
		case "aws_s3_bucket_server_side_encryption_configuration":
			fmt.Fprintf(&b, "resource \"aws_s3_bucket_server_side_encryption_configuration\" %q {\n  bucket = %q\n  rule {\n    apply_server_side_encryption_by_default {\n      sse_algorithm     = \"aws:kms\"\n      kms_master_key_id = var.kms_key_arn\n    }\n  }\n}\n", n, id)
		case "aws_s3_bucket_versioning":
			fmt.Fprintf(&b, "resource \"aws_s3_bucket_versioning\" %q {\n  bucket = %q\n  versioning_configuration {\n    status = \"Enabled\"\n  }\n}\n", n, id)
		case "aws_db_instance":
			fmt.Fprintf(&b, "resource \"aws_db_instance\" %q {\n  identifier                 = %q\n  # keep existing arguments; set the compliant values below\n  storage_encrypted          = true\n  publicly_accessible        = false\n  multi_az                   = true\n  backup_retention_period    = 14\n  deletion_protection        = true\n  auto_minor_version_upgrade = true\n}\n", n, id)
		case "aws_security_group_rule", "aws_security_group", "aws_vpc_security_group_ingress_rule":
			fmt.Fprintf(&b, "# Replace the 0.0.0.0/0 administrative ingress on %s with a restricted source.\nresource \"aws_vpc_security_group_ingress_rule\" %q {\n  security_group_id = %q\n  ip_protocol       = \"tcp\"\n  from_port         = 22\n  to_port           = 22\n  cidr_ipv4         = var.admin_cidr # e.g. your VPN egress /32\n}\n", id, n, id)
		case "aws_cloudtrail":
			fmt.Fprintf(&b, "resource \"aws_cloudtrail\" %q {\n  name                          = %q\n  s3_bucket_name                = var.cloudtrail_bucket\n  is_multi_region_trail         = true\n  include_global_service_events = true\n  enable_logging                = true\n  enable_log_file_validation    = true\n  kms_key_id                    = var.kms_key_arn\n}\n", n, id)
		case "aws_kms_key":
			fmt.Fprintf(&b, "resource \"aws_kms_key\" %q {\n  description         = \"%s\"\n  enable_key_rotation = true\n}\n", n, id)
		case "aws_guardduty_detector":
			fmt.Fprintf(&b, "resource \"aws_guardduty_detector\" %q {\n  enable = true # region %s\n}\n", n, id)
		case "aws_instance":
			fmt.Fprintf(&b, "resource \"aws_instance\" %q {\n  # instance %s — keep existing arguments\n  metadata_options {\n    http_tokens   = \"required\"\n    http_endpoint = \"enabled\"\n  }\n  root_block_device {\n    encrypted = true\n  }\n}\n", n, id)
		case "aws_iam_account_password_policy":
			b.WriteString("resource \"aws_iam_account_password_policy\" \"strict\" {\n  minimum_password_length        = 14\n  require_symbols                = true\n  require_numbers                = true\n  require_uppercase_characters   = true\n  require_lowercase_characters   = true\n  password_reuse_prevention      = 24\n  max_password_age               = 90\n  allow_users_to_change_password = true\n}\n")
			return b.String()
		case "aws_iam_access_key":
			fmt.Fprintf(&b, "# Rotate the access key of %s: create the new key, deploy it, then remove the old one.\nresource \"aws_iam_access_key\" %q {\n  user = %q\n}\n", id, n+"_rotated", id)
		case "github_branch_protection":
			repo := id
			if i := strings.Index(id, "/"); i >= 0 {
				repo = id[i+1:]
			}
			fmt.Fprintf(&b, "resource \"github_branch_protection\" %q {\n  repository_id  = %q\n  pattern        = \"main\"\n  enforce_admins = true\n  allows_force_pushes = false\n\n  required_pull_request_reviews {\n    required_approving_review_count = 1\n    dismiss_stale_reviews           = true\n    require_code_owner_reviews      = true\n  }\n\n  required_status_checks {\n    strict = true\n  }\n}\n", tfName(repo), repo)
		case "github_repository":
			repo := id
			if i := strings.Index(id, "/"); i >= 0 {
				repo = id[i+1:]
			}
			fmt.Fprintf(&b, "resource \"github_repository\" %q {\n  name                 = %q\n  visibility           = \"private\"\n  vulnerability_alerts = true\n\n  security_and_analysis {\n    secret_scanning {\n      status = \"enabled\"\n    }\n    secret_scanning_push_protection {\n      status = \"enabled\"\n    }\n  }\n}\n", tfName(repo), repo)
		case "github_organization_settings":
			fmt.Fprintf(&b, "resource \"github_organization_settings\" %q {\n  billing_email                            = var.billing_email\n  default_repository_permission            = \"read\"\n  members_can_create_public_repositories   = false\n  two_factor_requirement                   = true # set in the org UI if your provider version lacks this argument\n}\n", n)
		case "google_storage_bucket":
			fmt.Fprintf(&b, "resource \"google_storage_bucket\" %q {\n  name                        = %q\n  location                    = \"EU\"\n  uniform_bucket_level_access = true\n  public_access_prevention    = \"enforced\"\n  versioning {\n    enabled = true\n  }\n}\n", n, id)
		case "google_sql_database_instance":
			fmt.Fprintf(&b, "resource \"google_sql_database_instance\" %q {\n  name = %q\n  settings {\n    availability_type = \"REGIONAL\"\n    ip_configuration {\n      ipv4_enabled = false\n      ssl_mode     = \"ENCRYPTED_ONLY\"\n    }\n    backup_configuration {\n      enabled                        = true\n      point_in_time_recovery_enabled = true\n    }\n  }\n}\n", n, id)
		case "google_project_iam_member", "google_project_iam_binding":
			fmt.Fprintf(&b, "# Remove the binding %s and grant a predefined role instead.\nresource \"google_project_iam_member\" %q {\n  project = var.project_id\n  role    = \"roles/viewer\" # replace with the narrowest predefined role\n  member  = var.member\n}\n", id, n)
		case "google_service_account_key":
			fmt.Fprintf(&b, "resource \"google_service_account_key\" %q {\n  service_account_id = var.service_account_id # key %s\n  keepers = {\n    rotation = time_rotating.every_90_days.id\n  }\n}\n\nresource \"time_rotating\" \"every_90_days\" {\n  rotation_days = 90\n}\n", n, id)
			return b.String()
		case "okta_policy_mfa", "okta_policy_rule_signon", "okta_policy_signon":
			b.WriteString("resource \"okta_policy_rule_signon\" \"require_mfa\" {\n  policy_id          = var.default_signon_policy_id\n  name               = \"Require MFA\"\n  status             = \"ACTIVE\"\n  mfa_required       = true\n  mfa_prompt         = \"ALWAYS\"\n  session_idle       = 30\n  session_lifetime   = 720\n}\n")
			return b.String()
		case "okta_policy_password":
			b.WriteString("resource \"okta_policy_password\" \"default\" {\n  name                   = \"Default Policy\"\n  password_min_length    = 14\n  password_history_count = 24\n}\n")
			return b.String()
		case "okta_user":
			fmt.Fprintf(&b, "resource \"okta_user\" %q {\n  login  = %q\n  email  = %q\n  status = \"DEPROVISIONED\"\n}\n", n, id, id)
		default:
			fmt.Fprintf(&b, "resource %q %q {\n  # Update %s to the compliant configuration described in the steps above.\n}\n", resourceType, n, id)
		}
	}
	return b.String()
}

var printable = regexp.MustCompile(`[\x20-\x7E]{4,}`)

func extractText(att Attachment) string {
	if strings.HasPrefix(att.MediaType, "text/") || att.MediaType == "application/json" {
		return string(att.Data)
	}
	// PDFs and images: collect printable runs (uncompressed PDF text, PNG tEXt chunks).
	return strings.Join(printable.FindAllString(string(att.Data), -1), " ")
}

func fakeEvidence(in EvidenceInput, atts []Attachment) EvidenceVerdict {
	haystack := strings.ToLower(in.Title + " " + in.Description + " " + in.Filename)
	for _, a := range atts {
		haystack += " " + strings.ToLower(extractText(a))
	}
	norm := strings.NewReplacer("_", " ", "-", " ")
	haystack = norm.Replace(haystack)
	var hits []string
	for _, k := range in.Keywords {
		if strings.Contains(haystack, norm.Replace(strings.ToLower(k))) {
			hits = append(hits, k)
		}
	}
	if len(hits) > 0 {
		conf := 0.7 + 0.1*float64(len(hits))
		if conf > 0.95 {
			conf = 0.95
		}
		return EvidenceVerdict{Verdict: "pass", Confidence: conf,
			Reasoning: fmt.Sprintf("The evidence references %s, which is what control %s (%s) requires. It appears relevant and sufficient for this control; confirm it covers the full audit period.", strings.Join(hits, ", "), in.ControlCode, in.ControlName),
			Missing:   []string{}}
	}
	return EvidenceVerdict{Verdict: "fail", Confidence: 0.8,
		Reasoning: fmt.Sprintf("Nothing in the file or its description relates to %s. Control %s expects: %s", strings.ToLower(in.ControlName), in.ControlCode, in.EvidenceHint),
		Missing:   []string{in.EvidenceHint}}
}
