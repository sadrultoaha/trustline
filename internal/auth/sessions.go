// Package auth implements passwords, sessions, invites and password resets.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

type Sessions struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewSessions(pool *pgxpool.Pool, ttl time.Duration) *Sessions {
	return &Sessions{pool: pool, ttl: ttl}
}

// Create inserts a session and returns the raw token for the cookie.
func (s *Sessions) Create(ctx context.Context, q db.Querier, userID domain.ID, orgID *domain.ID, ip, ua string) (string, *domain.Session, error) {
	token, hash, err := NewToken()
	if err != nil {
		return "", nil, err
	}
	sess := &domain.Session{ID: domain.NewID(), UserID: userID, OrgID: orgID, ExpiresAt: time.Now().Add(s.ttl), LastSeenAt: time.Now()}
	_, err = q.Exec(ctx, `INSERT INTO sessions (id, user_id, org_id, token_hash, ip, user_agent, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, sess.ID, userID, orgID, hash, ip, truncate(ua, 512), sess.ExpiresAt)
	if err != nil {
		return "", nil, err
	}
	return token, sess, nil
}

// Lookup resolves a raw token to a live session and bumps last_seen_at (at
// most once per minute to keep writes low). Sliding expiry is applied.
func (s *Sessions) Lookup(ctx context.Context, token string) (*domain.Session, error) {
	if token == "" {
		return nil, domain.ErrUnauthorized
	}
	var sess domain.Session
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, org_id, expires_at, last_seen_at FROM sessions
		WHERE token_hash = $1 AND expires_at > now()`, HashToken(token)).
		Scan(&sess.ID, &sess.UserID, &sess.OrgID, &sess.ExpiresAt, &sess.LastSeenAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if time.Since(sess.LastSeenAt) > time.Minute {
		_, _ = s.pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now(), expires_at = now() + $2 WHERE id = $1`, sess.ID, s.ttl)
	}
	return &sess, nil
}

func (s *Sessions) SetOrg(ctx context.Context, sessionID, orgID domain.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET org_id = $2 WHERE id = $1`, sessionID, orgID)
	return err
}

func (s *Sessions) Delete(ctx context.Context, sessionID domain.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	return err
}

// DeleteAllForUser revokes every session of a user (password reset, offboarding).
func (s *Sessions) DeleteAllForUser(ctx context.Context, q db.Querier, userID domain.ID) error {
	_, err := q.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Users is the repository for the global users table.
type Users struct{}

func (Users) ByEmail(ctx context.Context, q db.Querier, email string) (*domain.User, string, error) {
	var u domain.User
	var pw, sub *string
	err := q.QueryRow(ctx, `SELECT id, email, name, avatar_url, password_hash, google_sub, created_at, last_login_at
		FROM users WHERE lower(email) = lower($1)`, strings.TrimSpace(email)).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &pw, &sub, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, "", domain.ErrNotFound
		}
		return nil, "", err
	}
	fill(&u, pw, sub)
	hash := ""
	if pw != nil {
		hash = *pw
	}
	return &u, hash, nil
}

func (Users) ByID(ctx context.Context, q db.Querier, id domain.ID) (*domain.User, error) {
	var u domain.User
	var pw, sub *string
	err := q.QueryRow(ctx, `SELECT id, email, name, avatar_url, password_hash, google_sub, created_at, last_login_at
		FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &pw, &sub, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	fill(&u, pw, sub)
	return &u, nil
}

func (Users) ByGoogleSub(ctx context.Context, q db.Querier, sub string) (*domain.User, error) {
	var u domain.User
	var pw, gs *string
	err := q.QueryRow(ctx, `SELECT id, email, name, avatar_url, password_hash, google_sub, created_at, last_login_at
		FROM users WHERE google_sub = $1`, sub).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &pw, &gs, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	fill(&u, pw, gs)
	return &u, nil
}

func fill(u *domain.User, pw, sub *string) {
	u.HasPassword = pw != nil && *pw != ""
	if sub != nil {
		u.GoogleSub = *sub
	}
}

type CreateUser struct {
	Email        string
	Name         string
	PasswordHash string // empty for OAuth-only
	GoogleSub    string
	AvatarURL    string
}

func (Users) Create(ctx context.Context, q db.Querier, in CreateUser) (*domain.User, error) {
	id := domain.NewID()
	var pw, sub *string
	if in.PasswordHash != "" {
		pw = &in.PasswordHash
	}
	if in.GoogleSub != "" {
		sub = &in.GoogleSub
	}
	_, err := q.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, google_sub, avatar_url) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, strings.TrimSpace(in.Email), strings.TrimSpace(in.Name), pw, sub, in.AvatarURL)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, domain.ErrConflict
		}
		return nil, err
	}
	return Users{}.ByID(ctx, q, id)
}

func (Users) UpdateProfile(ctx context.Context, q db.Querier, id domain.ID, name string) error {
	_, err := q.Exec(ctx, `UPDATE users SET name = $2, updated_at = now() WHERE id = $1`, id, strings.TrimSpace(name))
	return err
}

func (Users) SetPassword(ctx context.Context, q db.Querier, id domain.ID, hash string) error {
	_, err := q.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	return err
}

func (Users) LinkGoogle(ctx context.Context, q db.Querier, id domain.ID, sub, avatar string) error {
	_, err := q.Exec(ctx, `UPDATE users SET google_sub = $2, avatar_url = CASE WHEN avatar_url = '' THEN $3 ELSE avatar_url END, updated_at = now() WHERE id = $1`, id, sub, avatar)
	return err
}

func (Users) TouchLogin(ctx context.Context, q db.Querier, id domain.ID) error {
	_, err := q.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

// PasswordResets manages reset tokens.
type PasswordResets struct{}

func (PasswordResets) Create(ctx context.Context, q db.Querier, userID domain.ID) (string, error) {
	token, hash, err := NewToken()
	if err != nil {
		return "", err
	}
	_, err = q.Exec(ctx, `INSERT INTO password_resets (id, user_id, token_hash, expires_at) VALUES ($1,$2,$3, now() + interval '1 hour')`,
		domain.NewID(), userID, hash)
	return token, err
}

// Consume validates a token and marks it used, returning the user id.
func (PasswordResets) Consume(ctx context.Context, q db.Querier, token string) (domain.ID, error) {
	var userID domain.ID
	err := q.QueryRow(ctx, `UPDATE password_resets SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING user_id`, HashToken(token)).Scan(&userID)
	if err != nil {
		if db.IsNoRows(err) {
			return domain.ID{}, errors.Join(domain.ErrNotFound, errors.New("reset token is invalid or expired"))
		}
		return domain.ID{}, err
	}
	return userID, nil
}
