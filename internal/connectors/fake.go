package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Fake is the sandbox connector. Its state is a full resource map stored on the
// integration row; Sync returns a deep copy stamped with the current time. The
// state starts from Baseline and is changed through Scenarios or a direct edit,
// so tests (and demo users) can flip any control from passing to failing.
type Fake struct {
	Type  string
	State map[string]any
}

func (f *Fake) Sync(ctx context.Context) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.State == nil {
		return nil, fmt.Errorf("sandbox state missing")
	}
	if v, ok := f.State["_simulate_error"].(string); ok && v != "" {
		return nil, fmt.Errorf("%s API error: %s", f.Type, v)
	}
	raw, err := json.Marshal(f.State["resources"])
	if err != nil {
		return nil, err
	}
	var res map[string][]map[string]any
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("sandbox state is not a resource map: %w", err)
	}
	return &Snapshot{Connector: f.Type, TakenAt: time.Now().UTC(), Resources: res}, nil
}

// Person is one member of the sandbox company; every fake connector derives its
// users from the same roster so identities line up across systems.
type Person struct {
	Login, Name, Department, Title, Manager string
	Admin, Engineer                         bool
	LastLoginDays                           int
	HireDate                                string
}

// Roster returns the sandbox company for an org slug. The first six people
// match the demo seed accounts for the "acme" org.
func Roster() []Person {
	return []Person{
		{"ada", "Ada Lovelace", "Engineering", "CEO", "", true, true, 1, "2021-02-01"},
		{"grace", "Grace Hopper", "Engineering", "Staff Engineer", "ada", false, true, 2, "2021-06-14"},
		{"sam", "Sam Patel", "Sales", "Account Executive", "maria", false, false, 3, "2023-01-09"},
		{"jo", "Jo Nakamura", "Engineering", "Software Engineer", "grace", false, true, 1, "2023-09-04"},
		{"kim", "Kim Okafor", "Operations", "People Partner", "ada", false, false, 5, "2022-03-21"},
		{"maria", "Maria Santos", "Sales", "VP Sales", "ada", false, false, 2, "2022-01-10"},
		{"tom", "Tom Becker", "Engineering", "Software Engineer", "grace", false, true, 127, "2022-07-18"},
		{"lee", "Lee Chen", "Engineering", "Site Reliability Engineer", "grace", true, true, 1, "2024-02-05"},
	}
}

func emailOf(login, domain string) string { return login + "@" + domain }

// Baseline returns realistic starting state. It is mostly compliant with a few
// deliberate, common gaps so a fresh org sees both passing and failing tests.
func Baseline(typ, orgSlug string) map[string]any {
	domain := orgSlug + ".test"
	people := Roster()
	var res map[string]any
	switch typ {
	case "aws":
		res = awsBaseline(orgSlug, people)
	case "gcp":
		res = gcpBaseline(orgSlug, domain)
	case "github":
		res = githubBaseline(orgSlug, domain, people)
	case "okta":
		res = oktaBaseline(domain, people)
	case "google_workspace":
		res = gwsBaseline(domain, people)
	case "hris":
		res = hrisBaseline(domain, people)
	default:
		res = map[string]any{}
	}
	return map[string]any{"resources": res, "scenarios": []any{}}
}

func awsBaseline(slug string, people []Person) map[string]any {
	var users []any
	for _, p := range people {
		if !p.Engineer {
			continue
		}
		users = append(users, map[string]any{
			"id": p.Login, "arn": "arn:aws:iam::412398765432:user/" + p.Login, "console_access": true,
			"mfa_enabled": p.Login != "tom", "last_login_days": p.LastLoginDays, "admin": false, "is_service_account": false,
			"access_keys": []any{},
		})
	}
	users = append(users,
		map[string]any{"id": "ci-deployer", "arn": "arn:aws:iam::412398765432:user/ci-deployer", "console_access": false, "mfa_enabled": false, "last_login_days": 0, "admin": false, "is_service_account": true,
			"access_keys": []any{map[string]any{"id": "AKIAEXAMPLECI0000001", "active": true, "age_days": 41, "last_used_days": 0}}},
		map[string]any{"id": "legacy-backup", "arn": "arn:aws:iam::412398765432:user/legacy-backup", "console_access": false, "mfa_enabled": false, "last_login_days": 0, "admin": false, "is_service_account": true,
			"access_keys": []any{map[string]any{"id": "AKIAEXAMPLEBK0000002", "active": true, "age_days": 212, "last_used_days": 3}}},
	)
	bucket := func(name string, logging bool, class string) map[string]any {
		return map[string]any{"id": name, "region": "us-east-1",
			"public_access_block": map[string]any{"block_public_acls": true, "ignore_public_acls": true, "block_public_policy": true, "restrict_public_buckets": true},
			"public": false, "encryption": map[string]any{"enabled": true, "algorithm": "aws:kms"}, "versioning": true, "logging_enabled": logging,
			"tags": map[string]any{"data_classification": class}}
	}
	return map[string]any{
		"iam_user": users,
		"iam_account": []any{map[string]any{"id": "412398765432", "root_mfa_enabled": true, "root_access_keys": 0,
			"password_policy": map[string]any{"min_length": 14, "require_symbols": true, "require_numbers": true, "require_uppercase": true, "reuse_prevention": 24, "max_age_days": 90}}},
		"s3_bucket": []any{
			bucket(slug+"-prod-uploads", true, "confidential"),
			bucket(slug+"-marketing-assets", true, "public"),
			bucket(slug+"-cloudtrail-logs", true, "internal"),
			bucket(slug+"-tmp-exports", false, "confidential"),
		},
		"cloudtrail_trail": []any{map[string]any{"id": "org-trail", "is_multi_region": true, "is_logging": true, "log_file_validation": true, "kms_encrypted": true}},
		"rds_instance": []any{
			map[string]any{"id": "prod-postgres", "engine": "postgres", "environment": "production", "storage_encrypted": true, "publicly_accessible": false, "backup_retention_days": 30, "multi_az": true, "deletion_protection": true, "auto_minor_version_upgrade": true, "iam_auth_enabled": true},
			map[string]any{"id": "analytics-replica", "engine": "postgres", "environment": "production", "storage_encrypted": true, "publicly_accessible": false, "backup_retention_days": 30, "multi_az": false, "deletion_protection": true, "auto_minor_version_upgrade": true, "iam_auth_enabled": false},
		},
		"ec2_instance": []any{
			map[string]any{"id": "i-0a1b2c3d4e5f60001", "name": "api-1", "imdsv2_required": true, "public_ip": false, "ebs_encrypted": true, "ssm_managed": true},
			map[string]any{"id": "i-0a1b2c3d4e5f60002", "name": "api-2", "imdsv2_required": true, "public_ip": false, "ebs_encrypted": true, "ssm_managed": true},
			map[string]any{"id": "i-0a1b2c3d4e5f60003", "name": "bastion", "imdsv2_required": true, "public_ip": true, "ebs_encrypted": true, "ssm_managed": true},
		},
		"security_group": []any{
			map[string]any{"id": "sg-0aa1", "name": "alb-public", "open_ssh": false, "open_rdp": false, "open_all": false, "ingress": []any{map[string]any{"from_port": 443, "to_port": 443, "cidr": "0.0.0.0/0"}}},
			map[string]any{"id": "sg-0aa2", "name": "app-internal", "open_ssh": false, "open_rdp": false, "open_all": false, "ingress": []any{map[string]any{"from_port": 8080, "to_port": 8080, "cidr": "10.0.0.0/16"}}},
			map[string]any{"id": "sg-0aa3", "name": "bastion", "open_ssh": false, "open_rdp": false, "open_all": false, "ingress": []any{map[string]any{"from_port": 22, "to_port": 22, "cidr": "203.0.113.10/32"}}},
		},
		"kms_key": []any{
			map[string]any{"id": "mrk-1a2b3c", "alias": "alias/prod-data", "customer_managed": true, "rotation_enabled": true},
			map[string]any{"id": "mrk-4d5e6f", "alias": "alias/logs", "customer_managed": true, "rotation_enabled": true},
		},
		"guardduty_detector": []any{
			map[string]any{"id": "us-east-1", "enabled": true, "high_severity_findings": 0},
			map[string]any{"id": "eu-west-1", "enabled": true, "high_severity_findings": 0},
		},
	}
}

func gcpBaseline(slug, domain string) map[string]any {
	return map[string]any{
		"iam_binding": []any{
			map[string]any{"id": "group:platform@" + domain + "/roles/container.admin", "member": "group:platform@" + domain, "role": "roles/container.admin", "primitive_role": false, "external_member": false, "member_type": "group"},
			map[string]any{"id": "user:ada@" + domain + "/roles/owner", "member": "user:ada@" + domain, "role": "roles/owner", "primitive_role": true, "external_member": false, "member_type": "user"},
			map[string]any{"id": "serviceAccount:deployer/roles/run.admin", "member": "serviceAccount:deployer@" + slug + "-prod.iam.gserviceaccount.com", "role": "roles/run.admin", "primitive_role": false, "external_member": false, "member_type": "serviceAccount"},
		},
		"service_account_key": []any{
			map[string]any{"id": "7f3c2a", "service_account": "deployer@" + slug + "-prod.iam.gserviceaccount.com", "user_managed": true, "age_days": 33},
		},
		"gcs_bucket": []any{
			map[string]any{"id": slug + "-ml-datasets", "location": "EU", "public": false, "uniform_access": true, "versioning": true, "cmek": true, "logging_enabled": true},
			map[string]any{"id": slug + "-backups", "location": "EU", "public": false, "uniform_access": true, "versioning": true, "cmek": true, "logging_enabled": true},
		},
		"cloudsql_instance": []any{
			map[string]any{"id": "events-db", "database_version": "POSTGRES_16", "public_ip": false, "authorized_networks_open": false, "ssl_required": true, "backups_enabled": true, "point_in_time_recovery": true, "high_availability": true},
		},
		"project": []any{
			map[string]any{"id": slug + "-prod", "audit_logging_all_services": true, "os_login_enabled": true, "default_network_exists": false, "scc_enabled": true},
		},
	}
}

func githubBaseline(slug, domain string, people []Person) map[string]any {
	var members []any
	for _, p := range people {
		if !p.Engineer {
			continue
		}
		role := "member"
		if p.Admin {
			role = "admin"
		}
		members = append(members, map[string]any{"id": p.Login + "-" + slug, "email": emailOf(p.Login, domain), "role": role, "two_factor_enabled": true, "last_active_days": p.LastLoginDays})
	}
	repo := func(name string, production bool, protected bool, critical, overdue int) map[string]any {
		reviews := 0
		if protected {
			reviews = 2
		}
		return map[string]any{"id": slug + "/" + name, "private": true, "archived": !production && !protected, "production": production, "default_branch": "main",
			"branch_protection": map[string]any{"enabled": protected, "required_approving_reviews": reviews, "dismiss_stale_reviews": protected, "require_code_owner_reviews": protected, "enforce_admins": protected, "required_status_checks": protected, "allow_force_pushes": !protected},
			"dependabot_alerts_enabled": true, "dependabot_open_critical": critical, "dependabot_open_critical_overdue": overdue,
			"secret_scanning_enabled": true, "code_scanning_enabled": production, "outside_collaborators": 0}
	}
	return map[string]any{
		"organization": []any{map[string]any{"id": slug, "two_factor_required": true, "default_repo_permission": "read", "members_can_create_public_repos": false}},
		"member":       members,
		"repository": []any{
			repo("api", true, true, 0, 0),
			repo("web-app", true, true, 0, 0),
			repo("infrastructure", true, true, 0, 0),
			repo("billing-service", true, true, 1, 1),
			repo("design-prototypes", false, false, 0, 0),
		},
	}
}

func oktaBaseline(domain string, people []Person) map[string]any {
	var users []any
	for _, p := range people {
		users = append(users, map[string]any{"id": emailOf(p.Login, domain), "email": emailOf(p.Login, domain), "name": p.Name, "status": "ACTIVE",
			"mfa_enrolled": true, "factors": []any{"okta_verify", "webauthn"}, "admin": p.Admin, "last_login_days": p.LastLoginDays, "is_service_account": false,
			"groups": []any{"Everyone", p.Department}})
	}
	return map[string]any{
		"user": users,
		"policy": []any{map[string]any{"id": "Default Policy", "password_min_length": 14, "password_history": 24, "mfa_required": true, "session_idle_timeout_minutes": 30, "session_lifetime_hours": 12}},
		"app": []any{
			map[string]any{"id": "AWS IAM Identity Center", "sign_on_mode": "SAML_2_0", "critical": true, "assigned_users": 5},
			map[string]any{"id": "GitHub Enterprise", "sign_on_mode": "SAML_2_0", "critical": true, "assigned_users": 5},
			map[string]any{"id": "Google Workspace", "sign_on_mode": "SAML_2_0", "critical": true, "assigned_users": 8},
			map[string]any{"id": "Salesforce", "sign_on_mode": "SAML_2_0", "critical": true, "assigned_users": 2},
			map[string]any{"id": "Trustline", "sign_on_mode": "OPENID_CONNECT", "critical": false, "assigned_users": 8},
		},
	}
}

func gwsBaseline(domain string, people []Person) map[string]any {
	var users []any
	for _, p := range people {
		users = append(users, map[string]any{"id": emailOf(p.Login, domain), "name": p.Name, "suspended": false, "is_admin": p.Login == "ada" || p.Login == "lee",
			"two_step_enrolled": true, "last_login_days": p.LastLoginDays, "org_unit": "/" + p.Department})
	}
	return map[string]any{
		"user":   users,
		"domain": []any{map[string]any{"id": domain, "two_step_enforced": true, "password_min_length": 12, "less_secure_apps_allowed": false, "external_sharing_default": "allowlisted", "session_length_hours": 12}},
		"oauth_grant": []any{
			map[string]any{"id": "1093-slack", "app_name": "Slack", "scopes": []any{"openid", "email", "profile"}, "high_risk_scopes": false, "users": 8, "verified": true},
			map[string]any{"id": "2201-zoom", "app_name": "Zoom", "scopes": []any{"https://www.googleapis.com/auth/calendar"}, "high_risk_scopes": false, "users": 6, "verified": true},
			map[string]any{"id": "3302-notion", "app_name": "Notion", "scopes": []any{"openid", "email", "https://www.googleapis.com/auth/drive.readonly"}, "high_risk_scopes": false, "users": 7, "verified": true},
			map[string]any{"id": "4410-pdfmagic", "app_name": "PDF Magic Converter", "scopes": []any{"https://www.googleapis.com/auth/drive"}, "high_risk_scopes": true, "users": 1, "verified": false},
		},
	}
}

func hrisBaseline(domain string, people []Person) map[string]any {
	var emps []any
	for i, p := range people {
		mgr := ""
		if p.Manager != "" {
			mgr = emailOf(p.Manager, domain)
		}
		bg := "completed"
		if p.Login == "lee" {
			bg = "pending"
		}
		emps = append(emps, map[string]any{"id": fmt.Sprintf("E%03d", 101+i), "email": emailOf(p.Login, domain), "name": p.Name, "department": p.Department,
			"title": p.Title, "manager_email": mgr, "status": "active", "employment_type": "full_time", "hire_date": p.HireDate,
			"termination_date": nil, "location": "Amsterdam", "background_check": bg})
	}
	return map[string]any{"employee": emps}
}

// ---------------------------------------------------------------------------
// Scenarios: named, reversible mutations of sandbox state.

type ScenarioInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type scenario struct {
	ScenarioInfo
	apply func(res map[string]any, slug string)
}

func find(res map[string]any, typ, id string) map[string]any {
	list, _ := res[typ].([]any)
	for _, x := range list {
		if m, ok := x.(map[string]any); ok && fmt.Sprint(m["id"]) == id {
			return m
		}
	}
	return nil
}

func all(res map[string]any, typ string) []map[string]any {
	list, _ := res[typ].([]any)
	var out []map[string]any
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

var scenarios = map[string][]scenario{
	"aws": {
		{ScenarioInfo{"public_bucket", "Make the marketing bucket public", "Disables Block Public Access on <org>-marketing-assets and grants public read."}, func(r map[string]any, slug string) {
			if b := find(r, "s3_bucket", slug+"-marketing-assets"); b != nil {
				b["public_access_block"] = map[string]any{"block_public_acls": false, "ignore_public_acls": false, "block_public_policy": false, "restrict_public_buckets": false}
				b["public"] = true
			}
		}},
		{ScenarioInfo{"cloudtrail_off", "Stop CloudTrail logging", "Stops logging on the organization trail."}, func(r map[string]any, _ string) {
			for _, t := range all(r, "cloudtrail_trail") {
				t["is_logging"] = false
			}
		}},
		{ScenarioInfo{"open_ssh", "Open SSH to the internet", "Adds 0.0.0.0/0 on port 22 to the bastion security group."}, func(r map[string]any, _ string) {
			if g := find(r, "security_group", "sg-0aa3"); g != nil {
				g["open_ssh"] = true
				g["ingress"] = []any{map[string]any{"from_port": 22, "to_port": 22, "cidr": "0.0.0.0/0"}}
			}
		}},
		{ScenarioInfo{"rds_public", "Expose the production database", "Makes prod-postgres publicly accessible."}, func(r map[string]any, _ string) {
			if d := find(r, "rds_instance", "prod-postgres"); d != nil {
				d["publicly_accessible"] = true
			}
		}},
		{ScenarioInfo{"fix_baseline", "Remediate the baseline gaps", "Enables MFA for every console user, rotates old keys, enables access logging and Multi-AZ."}, func(r map[string]any, _ string) {
			for _, u := range all(r, "iam_user") {
				if u["console_access"] == true {
					u["mfa_enabled"] = true
					u["last_login_days"] = 1
				}
				if keys, ok := u["access_keys"].([]any); ok {
					for _, k := range keys {
						if km, ok := k.(map[string]any); ok {
							km["age_days"] = 3
						}
					}
				}
			}
			for _, b := range all(r, "s3_bucket") {
				b["logging_enabled"] = true
			}
			for _, d := range all(r, "rds_instance") {
				d["multi_az"] = true
			}
		}},
	},
	"github": {
		{ScenarioInfo{"unprotect_api", "Remove branch protection on api", "Disables branch protection on the api repository's default branch."}, func(r map[string]any, slug string) {
			if repo := find(r, "repository", slug+"/api"); repo != nil {
				repo["branch_protection"] = map[string]any{"enabled": false, "required_approving_reviews": 0, "dismiss_stale_reviews": false, "require_code_owner_reviews": false, "enforce_admins": false, "required_status_checks": false, "allow_force_pushes": true}
			}
		}},
		{ScenarioInfo{"disable_2fa_requirement", "Stop requiring 2FA", "Turns off the organization's 2FA requirement and a member disables 2FA."}, func(r map[string]any, _ string) {
			for _, o := range all(r, "organization") {
				o["two_factor_required"] = false
			}
			if ms := all(r, "member"); len(ms) > 1 {
				ms[len(ms)-1]["two_factor_enabled"] = false
			}
		}},
		{ScenarioInfo{"fix_baseline", "Resolve Dependabot alerts", "Closes the open critical Dependabot alerts."}, func(r map[string]any, _ string) {
			for _, repo := range all(r, "repository") {
				repo["dependabot_open_critical"] = 0
				repo["dependabot_open_critical_overdue"] = 0
			}
		}},
	},
	"gcp": {
		{ScenarioInfo{"public_gcs", "Make the datasets bucket public", "Grants allUsers read on the ML datasets bucket."}, func(r map[string]any, slug string) {
			if b := find(r, "gcs_bucket", slug+"-ml-datasets"); b != nil {
				b["public"] = true
				b["uniform_access"] = false
			}
		}},
		{ScenarioInfo{"fix_baseline", "Replace primitive owner role", "Replaces the user-level owner binding with a predefined role."}, func(r map[string]any, _ string) {
			for _, b := range all(r, "iam_binding") {
				if b["primitive_role"] == true {
					b["role"] = "roles/resourcemanager.projectIamAdmin"
					b["primitive_role"] = false
				}
			}
		}},
	},
	"okta": {
		{ScenarioInfo{"mfa_optional", "Make MFA optional", "Sets the sign-on policy to not require MFA and un-enrolls one user."}, func(r map[string]any, _ string) {
			for _, p := range all(r, "policy") {
				p["mfa_required"] = false
			}
			if us := all(r, "user"); len(us) > 2 {
				us[2]["mfa_enrolled"] = false
				us[2]["factors"] = []any{}
			}
		}},
		{ScenarioInfo{"fix_baseline", "Deactivate dormant accounts", "Deprovisions users inactive for more than 90 days."}, func(r map[string]any, _ string) {
			for _, u := range all(r, "user") {
				if d, ok := u["last_login_days"].(float64); ok && d > 90 {
					u["status"] = "DEPROVISIONED"
				}
			}
		}},
	},
	"google_workspace": {
		{ScenarioInfo{"two_step_off", "Stop enforcing 2-Step Verification", "Turns off 2SV enforcement for the domain."}, func(r map[string]any, _ string) {
			for _, d := range all(r, "domain") {
				d["two_step_enforced"] = false
			}
		}},
		{ScenarioInfo{"fix_baseline", "Revoke the unverified app", "Revokes access for unverified apps with high-risk scopes."}, func(r map[string]any, _ string) {
			list, _ := r["oauth_grant"].([]any)
			var keep []any
			for _, g := range list {
				if m, ok := g.(map[string]any); ok && m["verified"] == false && m["high_risk_scopes"] == true {
					continue
				}
				keep = append(keep, g)
			}
			r["oauth_grant"] = keep
		}},
	},
	"hris": {
		{ScenarioInfo{"missing_manager", "Remove a manager assignment", "Clears the manager of one employee."}, func(r map[string]any, _ string) {
			if es := all(r, "employee"); len(es) > 3 {
				es[3]["manager_email"] = ""
			}
		}},
		{ScenarioInfo{"fix_baseline", "Complete pending background checks", "Marks pending background checks as completed."}, func(r map[string]any, _ string) {
			for _, e := range all(r, "employee") {
				if e["background_check"] == "pending" {
					e["background_check"] = "completed"
				}
			}
		}},
	},
}

func scenarioInfo(typ string) []ScenarioInfo {
	var out []ScenarioInfo
	for _, s := range scenarios[typ] {
		out = append(out, s.ScenarioInfo)
	}
	return out
}

// ApplyScenarios rebuilds state from the baseline plus the given active
// scenarios (in a stable order), so toggling one off restores the baseline.
// Manual edits are preserved separately as overrides by the caller.
func ApplyScenarios(typ, orgSlug string, active []string) (map[string]any, error) {
	known := map[string]scenario{}
	for _, s := range scenarios[typ] {
		known[s.Key] = s
	}
	sorted := append([]string(nil), active...)
	sort.Strings(sorted)
	state := Baseline(typ, orgSlug)
	// Normalise through JSON so numbers are float64 like stored state.
	raw, _ := json.Marshal(state)
	_ = json.Unmarshal(raw, &state)
	res, _ := state["resources"].(map[string]any)
	var applied []any
	for _, k := range sorted {
		s, ok := known[k]
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q for %s", k, typ)
		}
		s.apply(res, orgSlug)
		applied = append(applied, k)
	}
	if applied == nil {
		applied = []any{}
	}
	state["scenarios"] = applied
	return state, nil
}

// ActiveScenarios reads the scenario list from stored state.
func ActiveScenarios(state map[string]any) []string {
	list, _ := state["scenarios"].([]any)
	var out []string
	for _, x := range list {
		out = append(out, strings.TrimSpace(fmt.Sprint(x)))
	}
	return out
}
