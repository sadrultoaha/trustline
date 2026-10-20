// Package seed creates the demo organisation. It is idempotent: running it
// twice updates rather than duplicates. Later phases extend Run with
// frameworks, integrations, policies, vendors and audits.
package seed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/auth"
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

// Run seeds the database. Safe to call repeatedly.
func Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (*Result, error) {
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
	return res, nil
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
