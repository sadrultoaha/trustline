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
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
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
	return res, nil
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
