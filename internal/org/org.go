// Package org manages organisations, memberships and invites.
package org

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func randomSuffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = s[:48]
	}
	if s == "" {
		s = "org"
	}
	return s
}

// Create inserts an org and makes userID its admin, in the given transaction.
func (s *Service) Create(ctx context.Context, tx pgx.Tx, name string, adminUserID domain.ID, actor domain.Actor) (*domain.Org, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.Invalid("name", "organization name is required")
	}
	base := Slugify(name)
	settings, _ := json.Marshal(domain.DefaultOrgSettings())
	var org *domain.Org
	for i := 0; i < 5; i++ {
		slug := base
		if i > 0 {
			slug = fmt.Sprintf("%s-%s", base, randomSuffix())
		}
		id := domain.NewID()
		// A nested transaction is a savepoint: a unique violation must not abort
		// the caller's transaction, so each attempt is isolated.
		sp, err := tx.Begin(ctx)
		if err != nil {
			return nil, err
		}
		_, err = sp.Exec(ctx, `INSERT INTO orgs (id, name, slug, settings) VALUES ($1,$2,$3,$4)`, id, name, slug, settings)
		if err != nil {
			_ = sp.Rollback(ctx)
			if db.IsUniqueViolation(err) {
				continue
			}
			return nil, err
		}
		if err := sp.Commit(ctx); err != nil {
			return nil, err
		}
		org, err = s.Get(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		break
	}
	if org == nil {
		return nil, errors.New("could not allocate org slug")
	}
	if _, err := s.addMembership(ctx, tx, org.ID, adminUserID, domain.RoleAdmin); err != nil {
		return nil, err
	}
	if err := activity.Record(ctx, tx, org.ID, actor, activity.Entry{Action: "org.created", TargetType: "org", TargetID: org.ID.String(), TargetLabel: org.Name}); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *Service) Get(ctx context.Context, q db.Querier, id domain.ID) (*domain.Org, error) {
	var o domain.Org
	var settings []byte
	err := q.QueryRow(ctx, `SELECT id, name, slug, settings, created_at, updated_at FROM orgs WHERE id = $1`, id).
		Scan(&o.ID, &o.Name, &o.Slug, &settings, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	o.Settings = domain.DefaultOrgSettings()
	_ = json.Unmarshal(settings, &o.Settings)
	return &o, nil
}

type UpdateOrg struct {
	Name     *string
	Settings *domain.OrgSettings
}

func (s *Service) Update(ctx context.Context, orgID domain.ID, in UpdateOrg, actor domain.Actor) (*domain.Org, error) {
	var out *domain.Org
	err := db.Tx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.Get(ctx, tx, orgID)
		if err != nil {
			return err
		}
		meta := map[string]any{}
		if in.Name != nil {
			n := strings.TrimSpace(*in.Name)
			if n == "" {
				return domain.Invalid("name", "organization name is required")
			}
			meta["name"] = map[string]string{"from": cur.Name, "to": n}
			cur.Name = n
		}
		if in.Settings != nil {
			v := domain.NewValidation()
			if in.Settings.AccessReviewDays < 1 || in.Settings.AccessReviewDays > 365 {
				v.Add("settings.access_review_days", "must be between 1 and 365")
			}
			if in.Settings.PasswordMinLength < 8 || in.Settings.PasswordMinLength > 128 {
				v.Add("settings.password_min_length", "must be between 8 and 128")
			}
			if in.Settings.PolicyAcceptanceThreshold < 1 || in.Settings.PolicyAcceptanceThreshold > 100 {
				v.Add("settings.policy_acceptance_threshold", "must be between 1 and 100")
			}
			if _, err := time.LoadLocation(in.Settings.Timezone); err != nil {
				v.Add("settings.timezone", "unknown timezone")
			}
			if u := strings.TrimSpace(in.Settings.SlackWebhookURL); u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
				v.Add("settings.slack_webhook_url", "must be an http(s) URL")
			}
			if err := v.Err(); err != nil {
				return err
			}
			meta["settings"] = map[string]any{"from": cur.Settings, "to": *in.Settings}
			cur.Settings = *in.Settings
		}
		settings, _ := json.Marshal(cur.Settings)
		if _, err := tx.Exec(ctx, `UPDATE orgs SET name = $2, settings = $3, updated_at = now() WHERE id = $1`, orgID, cur.Name, settings); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "org.updated", TargetType: "org", TargetID: orgID.String(), TargetLabel: cur.Name, Metadata: meta}); err != nil {
			return err
		}
		out, err = s.Get(ctx, tx, orgID)
		return err
	})
	return out, err
}

// --- memberships ---------------------------------------------------------

func (s *Service) addMembership(ctx context.Context, q db.Querier, orgID, userID domain.ID, role domain.Role) (*domain.Membership, error) {
	id := domain.NewID()
	_, err := q.Exec(ctx, `INSERT INTO org_memberships (id, org_id, user_id, role) VALUES ($1,$2,$3,$4)`, id, orgID, userID, role)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, domain.ErrConflict
		}
		return nil, err
	}
	return s.MembershipByUser(ctx, q, orgID, userID)
}

// AddMember adds an existing user to an org with a role (used by seed and invite acceptance).
func (s *Service) AddMember(ctx context.Context, q db.Querier, orgID, userID domain.ID, role domain.Role, actor domain.Actor) (*domain.Membership, error) {
	m, err := s.addMembership(ctx, q, orgID, userID, role)
	if err != nil {
		return nil, err
	}
	u, _ := auth.Users{}.ByID(ctx, q, userID)
	label := ""
	if u != nil {
		label = u.Email
	}
	if err := activity.Record(ctx, q, orgID, actor, activity.Entry{Action: "member.added", TargetType: "member", TargetID: m.ID.String(), TargetLabel: label, Metadata: map[string]any{"role": role}}); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) MembershipByUser(ctx context.Context, q db.Querier, orgID, userID domain.ID) (*domain.Membership, error) {
	var m domain.Membership
	err := q.QueryRow(ctx, `SELECT id, org_id, user_id, role, created_at, updated_at FROM org_memberships WHERE org_id = $1 AND user_id = $2`, orgID, userID).
		Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

const memberSelect = `SELECT m.id, m.org_id, m.user_id, m.role, m.created_at, m.updated_at, u.email, u.name, u.avatar_url, u.last_login_at
	FROM org_memberships m JOIN users u ON u.id = m.user_id`

func scanMember(row pgx.Row) (*domain.Member, error) {
	var m domain.Member
	err := row.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt, &m.Email, &m.Name, &m.AvatarURL, &m.LastLoginAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *Service) ListMembers(ctx context.Context, orgID domain.ID) ([]domain.Member, error) {
	rows, err := s.pool.Query(ctx, memberSelect+` WHERE m.org_id = $1 ORDER BY u.name, u.email`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Member{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Service) GetMember(ctx context.Context, orgID, membershipID domain.ID) (*domain.Member, error) {
	return scanMember(s.pool.QueryRow(ctx, memberSelect+` WHERE m.org_id = $1 AND m.id = $2`, orgID, membershipID))
}

func (s *Service) adminCount(ctx context.Context, q db.Querier, orgID domain.ID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM org_memberships WHERE org_id = $1 AND role = 'admin'`, orgID).Scan(&n)
	return n, err
}

func (s *Service) UpdateMemberRole(ctx context.Context, orgID, membershipID domain.ID, role domain.Role, actor domain.Actor) (*domain.Member, error) {
	if !role.Valid() {
		return nil, domain.Invalid("role", "invalid role")
	}
	var out *domain.Member
	err := db.Tx(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := scanMember(tx.QueryRow(ctx, memberSelect+` WHERE m.org_id = $1 AND m.id = $2 FOR UPDATE OF m`, orgID, membershipID))
		if err != nil {
			return err
		}
		if m.Role == domain.RoleAdmin && role != domain.RoleAdmin {
			n, err := s.adminCount(ctx, tx, orgID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return domain.Invalid("role", "an organization must keep at least one admin")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE org_memberships SET role = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, membershipID, role); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "member.role_changed", TargetType: "member", TargetID: m.ID.String(), TargetLabel: m.Email,
			Metadata: map[string]any{"from": m.Role, "to": role}}); err != nil {
			return err
		}
		out, err = scanMember(tx.QueryRow(ctx, memberSelect+` WHERE m.org_id = $1 AND m.id = $2`, orgID, membershipID))
		return err
	})
	return out, err
}

func (s *Service) RemoveMember(ctx context.Context, orgID, membershipID domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := scanMember(tx.QueryRow(ctx, memberSelect+` WHERE m.org_id = $1 AND m.id = $2 FOR UPDATE OF m`, orgID, membershipID))
		if err != nil {
			return err
		}
		if m.Role == domain.RoleAdmin {
			n, err := s.adminCount(ctx, tx, orgID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return domain.Invalid("member", "an organization must keep at least one admin")
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM org_memberships WHERE org_id = $1 AND id = $2`, orgID, membershipID); err != nil {
			return err
		}
		// Sessions pointing at this org for the removed user fall back to no org.
		if _, err := tx.Exec(ctx, `UPDATE sessions SET org_id = NULL WHERE user_id = $1 AND org_id = $2`, m.UserID, orgID); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "member.removed", TargetType: "member", TargetID: m.ID.String(), TargetLabel: m.Email,
			Metadata: map[string]any{"role": m.Role}})
	})
}

// OrgWithRole is what a user sees in the org switcher.
type OrgWithRole struct {
	domain.Org
	Role domain.Role `json:"role"`
}

func (s *Service) ListForUser(ctx context.Context, q db.Querier, userID domain.ID) ([]OrgWithRole, error) {
	rows, err := q.Query(ctx, `SELECT o.id, o.name, o.slug, o.settings, o.created_at, o.updated_at, m.role
		FROM org_memberships m JOIN orgs o ON o.id = m.org_id WHERE m.user_id = $1 ORDER BY o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgWithRole{}
	for rows.Next() {
		var o OrgWithRole
		var settings []byte
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &settings, &o.CreatedAt, &o.UpdatedAt, &o.Role); err != nil {
			return nil, err
		}
		o.Settings = domain.DefaultOrgSettings()
		_ = json.Unmarshal(settings, &o.Settings)
		out = append(out, o)
	}
	return out, rows.Err()
}

// --- invites -------------------------------------------------------------

const inviteTTL = 7 * 24 * time.Hour

// CreateInvite inserts a pending invite and returns it with the raw token.
// The caller enqueues the email in the same transaction.
func (s *Service) CreateInvite(ctx context.Context, tx pgx.Tx, orgID domain.ID, email string, role domain.Role, actor domain.Actor) (*domain.Invite, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	v := domain.NewValidation()
	if !strings.Contains(email, "@") || len(email) < 3 {
		v.Add("email", "a valid email is required")
	}
	if !role.Valid() {
		v.Add("role", "invalid role")
	}
	if err := v.Err(); err != nil {
		return nil, "", err
	}
	// Already a member?
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = $1 AND lower(u.email) = $2)`, orgID, email).Scan(&exists); err != nil {
		return nil, "", err
	}
	if exists {
		return nil, "", domain.Invalid("email", "this person is already a member")
	}
	// Replace any pending invite for the same email.
	if _, err := tx.Exec(ctx, `DELETE FROM org_invites WHERE org_id = $1 AND lower(email) = $2 AND accepted_at IS NULL`, orgID, email); err != nil {
		return nil, "", err
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return nil, "", err
	}
	id := domain.NewID()
	var invitedBy *domain.ID
	if actor.Type == domain.ActorUser {
		invitedBy = actor.ID
	}
	_, err = tx.Exec(ctx, `INSERT INTO org_invites (id, org_id, email, role, token_hash, invited_by, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, orgID, email, role, hash, invitedBy, time.Now().Add(inviteTTL))
	if err != nil {
		return nil, "", err
	}
	inv, err := s.getInvite(ctx, tx, `WHERE org_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return nil, "", err
	}
	if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "invite.created", TargetType: "invite", TargetID: id.String(), TargetLabel: email, Metadata: map[string]any{"role": role}}); err != nil {
		return nil, "", err
	}
	return inv, token, nil
}

func (s *Service) getInvite(ctx context.Context, q db.Querier, where string, args ...any) (*domain.Invite, error) {
	var i domain.Invite
	err := q.QueryRow(ctx, `SELECT id, org_id, email, role, invited_by, expires_at, accepted_at, created_at FROM org_invites `+where, args...).
		Scan(&i.ID, &i.OrgID, &i.Email, &i.Role, &i.InvitedBy, &i.ExpiresAt, &i.AcceptedAt, &i.CreatedAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &i, nil
}

func (s *Service) ListInvites(ctx context.Context, orgID domain.ID) ([]domain.Invite, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, org_id, email, role, invited_by, expires_at, accepted_at, created_at FROM org_invites
		WHERE org_id = $1 AND accepted_at IS NULL ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Invite{}
	for rows.Next() {
		var i domain.Invite
		if err := rows.Scan(&i.ID, &i.OrgID, &i.Email, &i.Role, &i.InvitedBy, &i.ExpiresAt, &i.AcceptedAt, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Service) GetInvite(ctx context.Context, orgID, inviteID domain.ID) (*domain.Invite, error) {
	return s.getInvite(ctx, s.pool, `WHERE org_id = $1 AND id = $2`, orgID, inviteID)
}

func (s *Service) RevokeInvite(ctx context.Context, orgID, inviteID domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.pool, func(tx pgx.Tx) error {
		inv, err := s.getInvite(ctx, tx, `WHERE org_id = $1 AND id = $2 AND accepted_at IS NULL`, orgID, inviteID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM org_invites WHERE org_id = $1 AND id = $2`, orgID, inviteID); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "invite.revoked", TargetType: "invite", TargetID: inv.ID.String(), TargetLabel: inv.Email})
	})
}

// InvitePreview is the public view of an invite, shown before accepting.
type InvitePreview struct {
	Email   string      `json:"email"`
	Role    domain.Role `json:"role"`
	OrgName string      `json:"org_name"`
	Expired bool        `json:"expired"`
}

func (s *Service) PreviewInvite(ctx context.Context, token string) (*InvitePreview, error) {
	inv, err := s.getInvite(ctx, s.pool, `WHERE token_hash = $1 AND accepted_at IS NULL`, auth.HashToken(token))
	if err != nil {
		return nil, err
	}
	o, err := s.Get(ctx, s.pool, inv.OrgID)
	if err != nil {
		return nil, err
	}
	return &InvitePreview{Email: inv.Email, Role: inv.Role, OrgName: o.Name, Expired: time.Now().After(inv.ExpiresAt)}, nil
}

// AcceptInvite joins the user to the org. The user's email must match the invite.
func (s *Service) AcceptInvite(ctx context.Context, tx pgx.Tx, token string, user *domain.User, ip string) (*domain.Membership, error) {
	inv, err := s.getInvite(ctx, tx, `WHERE token_hash = $1 AND accepted_at IS NULL FOR UPDATE`, auth.HashToken(token))
	if err != nil {
		return nil, err
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, domain.Invalid("token", "this invite has expired")
	}
	if !strings.EqualFold(inv.Email, user.Email) {
		return nil, domain.Invalid("email", fmt.Sprintf("this invite was sent to %s; sign in with that address", inv.Email))
	}
	if _, err := tx.Exec(ctx, `UPDATE org_invites SET accepted_at = now() WHERE id = $1`, inv.ID); err != nil {
		return nil, err
	}
	m, err := s.addMembership(ctx, tx, inv.OrgID, user.ID, inv.Role)
	if err != nil {
		return nil, err
	}
	actor := domain.Actor{Type: domain.ActorUser, ID: &user.ID, Label: user.Email, IP: ip}
	if err := activity.Record(ctx, tx, inv.OrgID, actor, activity.Entry{Action: "invite.accepted", TargetType: "member", TargetID: m.ID.String(), TargetLabel: user.Email, Metadata: map[string]any{"role": inv.Role, "invite_id": inv.ID}}); err != nil {
		return nil, err
	}
	return m, nil
}
