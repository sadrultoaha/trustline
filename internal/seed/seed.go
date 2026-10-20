// Package seed creates the demo organisation. It is idempotent: running it
// twice updates rather than duplicates. Later phases extend Run with
// frameworks, integrations, policies, vendors and audits.
package seed

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/app"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/config"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/org"
)

// DemoPassword is shared by every seeded account; it is printed by `make seed`.
const DemoPassword = "Trustline-demo-2026!"

type SeedUser struct {
	Email string
	Name  string
	Role  domain.Role
}

var AcmeUsers = []SeedUser{
	{"ada@acme.test", "Ada Lovelace", domain.RoleAdmin},
	{"grace@acme.test", "Grace Hopper", domain.RoleEditor},
	{"auditor@acme.test", "Alan Auditor", domain.RoleAuditor},
	{"sam@acme.test", "Sam Patel", domain.RoleEmployee},
	{"jo@acme.test", "Jo Nakamura", domain.RoleEmployee},
	{"kim@acme.test", "Kim Okafor", domain.RoleEmployee},
	{"maria@acme.test", "Maria Santos", domain.RoleEmployee},
	{"tom@acme.test", "Tom Becker", domain.RoleEmployee},
	{"lee@acme.test", "Lee Chen", domain.RoleEmployee},
}

var GlobexUsers = []SeedUser{
	{"linus@globex.test", "Linus Torvalds", domain.RoleAdmin},
	{"mary@globex.test", "Mary Globex", domain.RoleEditor},
}

type Result struct {
	AcmeOrgID   domain.ID
	GlobexOrgID domain.ID
	Users       map[string]domain.ID
}

//go:embed evidence/*.txt
var evidenceFS embed.FS

// Run seeds the database. Safe to call repeatedly.
func Run(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger) (*Result, error) {
	res := &Result{Users: map[string]domain.ID{}}
	orgs := org.New(pool)
	hash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return nil, err
	}
	actor := domain.SystemActor("seed")

	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		res.AcmeOrgID, err = ensureOrg(ctx, tx, orgs, "Acme Corp", "acme", AcmeUsers, hash, actor, res.Users)
		if err != nil {
			return err
		}
		res.GlobexOrgID, err = ensureOrg(ctx, tx, orgs, "Globex Corporation", "globex", GlobexUsers, hash, actor, res.Users)
		return err
	})
	if err != nil {
		return nil, err
	}
	log.Info("seeded orgs", "acme", res.AcmeOrgID, "globex", res.GlobexOrgID, "users", len(res.Users))

	jc, err := jobs.NewInsertOnly(pool, log)
	if err != nil {
		return nil, err
	}
	comp, err := app.NewCompliance(ctx, cfg, pool, jc, log)
	if err != nil {
		return nil, err
	}
	admin := func(email string) domain.Actor {
		id := res.Users[email]
		return domain.Actor{Type: domain.ActorUser, ID: &id, Label: email}
	}
	if err := seedCompliance(ctx, comp, res.AcmeOrgID, admin("ada@acme.test"), []string{"soc2", "iso27001"},
		[]string{"aws", "github", "okta", "google_workspace", "hris"}, true, log); err != nil {
		return nil, fmt.Errorf("seed acme compliance: %w", err)
	}
	if err := seedCompliance(ctx, comp, res.GlobexOrgID, admin("linus@globex.test"), []string{"soc2"}, []string{"aws"}, false, log); err != nil {
		return nil, fmt.Errorf("seed globex compliance: %w", err)
	}
	if err := seedPhase2(ctx, comp, res.AcmeOrgID, res.Users, log); err != nil {
		return nil, fmt.Errorf("seed acme people & policies: %w", err)
	}
	return res, nil
}

// corePolicies are published for the demo org.
var corePolicies = []string{"information-security", "access-control", "acceptable-use", "incident-response", "business-continuity",
	"vendor-management", "data-retention", "change-management", "risk-management", "hr-security", "ai-acceptable-use"}

func seedPhase2(ctx context.Context, comp *compliance.Service, orgID domain.ID, users map[string]domain.ID, log *slog.Logger) error {
	actor := func(email string) domain.Actor {
		id := users[email]
		return domain.Actor{Type: domain.ActorUser, ID: &id, Label: email}
	}
	members, err := orgMembers(ctx, comp, orgID)
	if err != nil {
		return err
	}
	// Policies: instantiate, approve (Grace), publish (Ada), acknowledge (everyone).
	existing, err := comp.ListPolicies(ctx, orgID, false)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, p := range existing {
		have[p.Slug] = true
	}
	for _, slug := range corePolicies {
		if have[slug] {
			continue
		}
		d, err := comp.CreatePolicy(ctx, orgID, slug, "", actor("ada@acme.test"))
		if err != nil {
			return fmt.Errorf("policy %s: %w", slug, err)
		}
		v := d.Versions[0].ID
		if err := comp.SubmitForApproval(ctx, orgID, v, members["grace@acme.test"], actor("ada@acme.test")); err != nil {
			return err
		}
		if err := comp.DecideVersion(ctx, orgID, v, true, "Reviewed against current practice.", actor("grace@acme.test"), domain.RoleEditor, members["grace@acme.test"]); err != nil {
			return err
		}
		if err := comp.Publish(ctx, orgID, v, actor("ada@acme.test")); err != nil {
			return err
		}
		for email := range members {
			if email == "auditor@acme.test" {
				continue
			}
			if err := comp.Acknowledge(ctx, orgID, v, users[email], "seed", actor(email)); err != nil {
				return err
			}
		}
	}
	// Training: everyone except Tom passed this year.
	if c := comp.Catalog.Course("security-awareness-2026"); c != nil {
		answers := map[string]int{}
		for _, q := range c.Questions {
			answers[q.ID] = q.Answer
		}
		for email := range members {
			if email == "tom@acme.test" || email == "auditor@acme.test" {
				continue
			}
			st, err := comp.TrainingFor(ctx, orgID, users[email])
			if err != nil {
				return err
			}
			if st.Completed {
				continue
			}
			if _, err := comp.SubmitQuiz(ctx, orgID, users[email], answers, actor(email)); err != nil {
				return err
			}
		}
	}
	// Devices via the same ingestion path the agent uses. Tom has none; Lee's screen lock is too long.
	yes := true
	devices := []compliance.DeviceReport{
		{Serial: "C02ADA0001", Hostname: "ada-mbp", OwnerEmail: "ada@acme.test", Platform: "macos", OSVersion: "15.1"},
		{Serial: "C02GRC0002", Hostname: "grace-mbp", OwnerEmail: "grace@acme.test", Platform: "macos", OSVersion: "15.1"},
		{Serial: "PF3SAM0003", Hostname: "sam-thinkpad", OwnerEmail: "sam@acme.test", Platform: "windows", OSVersion: "11 23H2"},
		{Serial: "C02JON0004", Hostname: "jo-mbp", OwnerEmail: "jo@acme.test", Platform: "macos", OSVersion: "15.0"},
		{Serial: "C02KIM0005", Hostname: "kim-mba", OwnerEmail: "kim@acme.test", Platform: "macos", OSVersion: "15.1"},
		{Serial: "PF3MAR0006", Hostname: "maria-xps", OwnerEmail: "maria@acme.test", Platform: "windows", OSVersion: "11 23H2"},
		{Serial: "LNXLEE0007", Hostname: "lee-framework", OwnerEmail: "lee@acme.test", Platform: "linux", OSVersion: "Ubuntu 24.04"},
	}
	for _, d := range devices {
		d.DiskEncrypted = &yes
		lock := 5
		if d.OwnerEmail == "lee@acme.test" {
			lock = 30
		}
		d.ScreenLockMinutes = &lock
		d.AntivirusRunning = true
		d.AntivirusName = map[string]string{"macos": "XProtect", "windows": "Microsoft Defender", "linux": "ClamAV"}[d.Platform]
		d.AgentVersion = "1.0.0"
		if _, err := comp.IngestDeviceReport(ctx, orgID, d); err != nil {
			return err
		}
	}
	// One completed access review (Okta + GitHub), everything approved.
	campaigns, err := comp.ListCampaigns(ctx, orgID)
	if err != nil {
		return err
	}
	if len(campaigns) == 0 {
		c, err := comp.CreateCampaign(ctx, orgID, compliance.CampaignInput{Name: "Q3 2026 access review", Systems: []string{"github", "okta"}, DueDays: 14}, actor("ada@acme.test"))
		if err != nil {
			return err
		}
		for _, it := range c.Items {
			if _, err := comp.Decide(ctx, orgID, it.ID, "approve", "Access matches role", actor("ada@acme.test"), domain.RoleAdmin, members["ada@acme.test"]); err != nil {
				return err
			}
		}
		if _, err := comp.CompleteCampaign(ctx, orgID, c.ID, actor("ada@acme.test")); err != nil {
			return err
		}
	}
	// Refresh platform data and evaluate it now so the demo is consistent immediately.
	if pid, err := comp.PlatformIntegrationID(ctx, orgID); err == nil && pid != nil {
		snap, err := comp.SyncIntegration(ctx, orgID, *pid, "seed")
		if err != nil {
			return err
		}
		if snap != nil {
			if _, err := comp.RunTests(ctx, orgID, *snap, "seed"); err != nil {
				return err
			}
		}
	}
	log.Info("seeded people, policies, training, devices and access reviews", "org", orgID)
	return nil
}

var sandboxCreds = map[string]map[string]string{
	"aws":              {"role_arn": "arn:aws:iam::412398765432:role/TrustlineAudit", "external_id": "tl-demo-external-id"},
	"gcp":              {"project_id": "acme-prod", "service_account_json": `{"type":"service_account","project_id":"acme-prod"}`},
	"github":           {"org": "acme", "token": "github_pat_demo_sandbox_token_0000"},
	"okta":             {"domain": "acme.okta.com", "api_token": "00demo-sandbox-okta-token"},
	"google_workspace": {"domain": "acme.test", "admin_email": "ada@acme.test", "service_account_json": `{"type":"service_account"}`},
	"hris":             {"subdomain": "acme", "api_key": "demo-sandbox-bamboohr-key"},
}

func seedCompliance(ctx context.Context, comp *compliance.Service, orgID domain.ID, actor domain.Actor, frameworks, integrations []string, extras bool, log *slog.Logger) error {
	for _, fw := range frameworks {
		if err := comp.AdoptFramework(ctx, orgID, fw, actor); err != nil {
			var v *domain.ValidationError
			if !errors.As(err, &v) { // already adopted is fine
				return err
			}
		}
	}
	existing, err := comp.ListIntegrations(ctx, orgID)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, i := range existing {
		have[i.Type] = true
	}
	for _, typ := range integrations {
		if have[typ] {
			continue
		}
		in, err := comp.Connect(ctx, orgID, compliance.ConnectInput{Type: typ, Mode: "sandbox", Credentials: sandboxCreds[typ]}, actor)
		if err != nil {
			return fmt.Errorf("connect %s: %w", typ, err)
		}
		// Sync and evaluate now so the demo has results before the worker runs.
		snap, err := comp.SyncIntegration(ctx, orgID, in.ID, "seed")
		if err != nil {
			return fmt.Errorf("sync %s: %w", typ, err)
		}
		if snap != nil {
			if _, err := comp.RunTests(ctx, orgID, *snap, "seed"); err != nil {
				return fmt.Errorf("run tests %s: %w", typ, err)
			}
		}
	}
	if !extras {
		return nil
	}
	// Control owners.
	owners := map[string]string{"IAM-01": "grace@acme.test", "DAT-01": "grace@acme.test", "CHG-01": "grace@acme.test", "GOV-01": "ada@acme.test", "HR-02": "ada@acme.test"}
	members, err := orgMembers(ctx, comp, orgID)
	if err != nil {
		return err
	}
	controls, err := comp.ListControls(ctx, orgID, compliance.ControlFilter{})
	if err != nil {
		return err
	}
	byCode := map[string]compliance.ControlRow{}
	for _, c := range controls {
		byCode[c.Code] = c
	}
	for code, email := range owners {
		c, ok := byCode[code]
		if !ok || c.Owner != nil {
			continue
		}
		mid := members[email]
		if err := comp.SetOwner(ctx, orgID, c.ID, &mid, actor); err != nil {
			return err
		}
	}
	// Sample evidence (validated by the worker's AI job).
	samples := []struct{ code, file, title string }{
		{"GOV-01", "information-security-policy.txt", "Information Security Policy v3.2 (approved)"},
		{"HR-02", "security-awareness-training-report.txt", "Security awareness training completion report 2026"},
		{"GOV-04", "team-offsite-agenda.txt", "Team offsite agenda"},
	}
	for _, sm := range samples {
		c, ok := byCode[sm.code]
		if !ok {
			continue
		}
		evs, err := comp.ListEvidence(ctx, orgID, c.ID)
		if err != nil {
			return err
		}
		if len(evs) > 0 {
			continue
		}
		data, err := evidenceFS.ReadFile("evidence/" + sm.file)
		if err != nil {
			return err
		}
		if _, err := comp.UploadEvidence(ctx, orgID, c.ID, compliance.UploadInput{Title: sm.title, Filename: sm.file, Data: data}, actor); err != nil {
			return err
		}
	}
	log.Info("seeded compliance", "org", orgID, "frameworks", frameworks, "integrations", integrations)
	return nil
}

func orgMembers(ctx context.Context, comp *compliance.Service, orgID domain.ID) (map[string]domain.ID, error) {
	rows, err := comp.Pool.Query(ctx, `SELECT u.email, m.id FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]domain.ID{}
	for rows.Next() {
		var e string
		var id domain.ID
		if err := rows.Scan(&e, &id); err != nil {
			return nil, err
		}
		out[e] = id
	}
	return out, rows.Err()
}

func ensureUser(ctx context.Context, tx pgx.Tx, u SeedUser, hash string) (*domain.User, error) {
	existing, _, err := auth.Users{}.ByEmail(ctx, tx, u.Email)
	if err == nil {
		// Keep the demo password stable even if someone changed it.
		if err := (auth.Users{}).SetPassword(ctx, tx, existing.ID, hash); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if err != domain.ErrNotFound {
		return nil, err
	}
	return auth.Users{}.Create(ctx, tx, auth.CreateUser{Email: u.Email, Name: u.Name, PasswordHash: hash})
}

func ensureOrg(ctx context.Context, tx pgx.Tx, orgs *org.Service, name, slug string, users []SeedUser, hash string, actor domain.Actor, out map[string]domain.ID) (domain.ID, error) {
	var orgID domain.ID
	err := tx.QueryRow(ctx, `SELECT id FROM orgs WHERE slug = $1`, slug).Scan(&orgID)
	if err != nil && !db.IsNoRows(err) {
		return orgID, err
	}
	admin, err := ensureUser(ctx, tx, users[0], hash)
	if err != nil {
		return orgID, err
	}
	out[admin.Email] = admin.ID
	if db.IsNoRows(err) || orgID == (domain.ID{}) {
		o, err := orgs.Create(ctx, tx, name, admin.ID, actor)
		if err != nil {
			return orgID, err
		}
		// Pin the slug so re-runs find it.
		if _, err := tx.Exec(ctx, `UPDATE orgs SET slug = $2 WHERE id = $1`, o.ID, slug); err != nil {
			return orgID, err
		}
		orgID = o.ID
	}
	for _, su := range users {
		u, err := ensureUser(ctx, tx, su, hash)
		if err != nil {
			return orgID, fmt.Errorf("user %s: %w", su.Email, err)
		}
		out[u.Email] = u.ID
		if _, err := orgs.MembershipByUser(ctx, tx, orgID, u.ID); err == domain.ErrNotFound {
			if _, err := orgs.AddMember(ctx, tx, orgID, u.ID, su.Role, actor); err != nil {
				return orgID, err
			}
		} else if err != nil {
			return orgID, err
		}
	}
	return orgID, nil
}
