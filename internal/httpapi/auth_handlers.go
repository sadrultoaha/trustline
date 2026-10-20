package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/org"
	"github.com/trustline/trustline/internal/rbac"
)

func (s *Server) users() auth.Users { return auth.Users{} }

func (s *Server) limited(w http.ResponseWriter, r *http.Request, key string, limit int, window time.Duration) bool {
	ok, err := s.limiter.Allow(r.Context(), key, limit, window)
	if err != nil {
		s.log.Warn("rate limiter error", "err", err)
		return false
	}
	if !ok {
		writeError(w, 429, "rate_limited", "too many attempts, try again later")
		return true
	}
	return false
}

// meResponse is the shape of GET /me and of every auth success response.
type meResponse struct {
	User        *domain.User      `json:"user"`
	Org         *domain.Org       `json:"org,omitempty"`
	Role        domain.Role       `json:"role,omitempty"`
	Permissions []string          `json:"permissions"`
	Orgs        []org.OrgWithRole `json:"orgs"`
}

func (s *Server) buildMe(r *http.Request, u *domain.User, sess *domain.Session) (*meResponse, error) {
	ctx := r.Context()
	orgs, err := s.orgs.ListForUser(ctx, s.pool, u.ID)
	if err != nil {
		return nil, err
	}
	resp := &meResponse{User: u, Orgs: orgs, Permissions: []string{}}
	if sess != nil && sess.OrgID != nil {
		for _, o := range orgs {
			if o.ID == *sess.OrgID {
				oc := o.Org
				resp.Org = &oc
				resp.Role = o.Role
				resp.Permissions = rbac.PermissionsOf(o.Role)
			}
		}
	}
	return resp, nil
}

// firstOrg picks the org a new session should start in.
func firstOrg(orgs []org.OrgWithRole) *domain.ID {
	if len(orgs) == 0 {
		return nil
	}
	id := orgs[0].ID
	return &id
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, q db.Querier, u *domain.User, orgID *domain.ID) (*domain.Session, error) {
	token, sess, err := s.sessions.Create(r.Context(), q, u.ID, orgID, clientIP(r), r.UserAgent())
	if err != nil {
		return nil, err
	}
	_ = s.users().TouchLogin(r.Context(), q, u.ID)
	s.setSessionCookie(w, token, sess.ExpiresAt)
	return sess, nil
}

// --- signup / login ---------------------------------------------------------

type signupReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Name        string `json:"name"`
	OrgName     string `json:"org_name"`
	InviteToken string `json:"invite_token"`
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if s.limited(w, r, "signup:"+clientIP(r), 100, time.Hour) {
		return
	}
	var in signupReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	v := domain.NewValidation()
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if !strings.Contains(in.Email, "@") {
		v.Add("email", "a valid email is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		v.Add("name", "your name is required")
	}
	if err := auth.ValidatePassword(in.Password, 12); err != nil {
		v.Add("password", err.Error())
	}
	if in.InviteToken == "" && strings.TrimSpace(in.OrgName) == "" {
		v.Add("org_name", "organization name is required")
	}
	if err := v.Err(); err != nil {
		s.fail(w, r, err)
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var resp *meResponse
	err = db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		u, err := s.users().Create(r.Context(), tx, auth.CreateUser{Email: in.Email, Name: in.Name, PasswordHash: hash})
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return domain.Invalid("email", "an account with this email already exists")
			}
			return err
		}
		actor := domain.Actor{Type: domain.ActorUser, ID: &u.ID, Label: u.Email, IP: clientIP(r)}
		var orgID domain.ID
		if in.InviteToken != "" {
			m, err := s.orgs.AcceptInvite(r.Context(), tx, in.InviteToken, u, clientIP(r))
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return domain.Invalid("invite_token", "this invite is no longer valid")
				}
				return err
			}
			orgID = m.OrgID
		} else {
			o, err := s.orgs.Create(r.Context(), tx, in.OrgName, u.ID, actor)
			if err != nil {
				return err
			}
			orgID = o.ID
		}
		sess, err := s.startSession(w, r, tx, u, &orgID)
		if err != nil {
			return err
		}
		if err := activity.Record(r.Context(), tx, orgID, actor, activity.Entry{Action: "auth.signup", TargetType: "user", TargetID: u.ID.String(), TargetLabel: u.Email}); err != nil {
			return err
		}
		// Build the response inside the tx so the new rows are visible.
		orgs, err := s.orgs.ListForUser(r.Context(), tx, u.ID)
		if err != nil {
			return err
		}
		resp = &meResponse{User: u, Orgs: orgs, Permissions: []string{}}
		for _, o := range orgs {
			if o.ID == *sess.OrgID {
				oc := o.Org
				resp.Org = &oc
				resp.Role = o.Role
				resp.Permissions = rbac.PermissionsOf(o.Role)
			}
		}
		return nil
	})
	if err != nil {
		s.clearSessionCookie(w)
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, resp)
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in loginReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	// Per-IP: all attempts. Per-email: failed attempts only (brute-force guard),
	// so legitimate repeated sign-ins never lock an account.
	if s.limited(w, r, "login:ip:"+clientIP(r), 300, 10*time.Minute) {
		return
	}
	emailKey := "login:fail:" + in.Email
	if ok, err := s.limiter.Peek(r.Context(), emailKey, 10, 10*time.Minute); err == nil && !ok {
		writeError(w, 429, "rate_limited", "too many attempts, try again later")
		return
	}
	u, hash, err := s.users().ByEmail(r.Context(), s.pool, in.Email)
	if err != nil || hash == "" || !auth.VerifyPassword(hash, in.Password) {
		_, _ = s.limiter.Allow(r.Context(), emailKey, 10, 10*time.Minute)
		if err == nil || errors.Is(err, domain.ErrNotFound) {
			// Burn time comparably when user is missing to limit enumeration.
			if err != nil {
				auth.VerifyPassword("$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", in.Password)
			}
			writeError(w, 401, "invalid_credentials", "incorrect email or password")
			return
		}
		s.fail(w, r, err)
		return
	}
	orgs, err := s.orgs.ListForUser(r.Context(), s.pool, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	orgID := firstOrg(orgs)
	sess, err := s.startSession(w, r, s.pool, u, orgID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if orgID != nil {
		_ = activity.Record(r.Context(), s.pool, *orgID, domain.Actor{Type: domain.ActorUser, ID: &u.ID, Label: u.Email, IP: clientIP(r)},
			activity.Entry{Action: "auth.login", TargetType: "user", TargetID: u.ID.String(), TargetLabel: u.Email, Metadata: map[string]any{"method": "password"}})
	}
	resp, err := s.buildMe(r, u, sess)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, resp)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if sess := sessionFrom(r.Context()); sess != nil {
		_ = s.sessions.Delete(r.Context(), sess.ID)
	}
	s.clearSessionCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- password reset ---------------------------------------------------------

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if s.limited(w, r, "forgot:"+in.Email, 5, time.Hour) || s.limited(w, r, "forgot:ip:"+clientIP(r), 100, time.Hour) {
		return
	}
	// Always answer OK to avoid account enumeration.
	u, _, err := s.users().ByEmail(r.Context(), s.pool, in.Email)
	if err == nil {
		_ = db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
			token, err := auth.PasswordResets{}.Create(r.Context(), tx, u.ID)
			if err != nil {
				return err
			}
			link := s.cfg.BaseURL + "/reset-password?token=" + url.QueryEscape(token)
			html, text := mail.Render(mail.TemplateData{
				Title: "Reset your password", Body: "We received a request to reset the password for " + u.Email + ". This link expires in one hour.",
				ActionURL: link, ActionLabel: "Reset password", Footer: "If you did not request this, you can ignore this email.",
			})
			return s.jobs.EnqueueTx(r.Context(), tx, jobs.SendEmailArgs{To: u.Email, Subject: "Reset your Trustline password", HTML: html, Text: text})
		})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := auth.ValidatePassword(in.Password, 12); err != nil {
		s.fail(w, r, domain.Invalid("password", err.Error()))
		return
	}
	err := db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		userID, err := auth.PasswordResets{}.Consume(r.Context(), tx, in.Token)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.Invalid("token", "this reset link is invalid or has expired")
			}
			return err
		}
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			return err
		}
		if err := s.users().SetPassword(r.Context(), tx, userID, hash); err != nil {
			return err
		}
		if err := s.sessions.DeleteAllForUser(r.Context(), tx, userID); err != nil {
			return err
		}
		u, err := s.users().ByID(r.Context(), tx, userID)
		if err != nil {
			return err
		}
		orgs, _ := s.orgs.ListForUser(r.Context(), tx, userID)
		for _, o := range orgs {
			_ = activity.Record(r.Context(), tx, o.ID, domain.Actor{Type: domain.ActorUser, ID: &u.ID, Label: u.Email, IP: clientIP(r)},
				activity.Entry{Action: "auth.password_reset", TargetType: "user", TargetID: u.ID.String(), TargetLabel: u.Email})
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.clearSessionCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Google OAuth -------------------------------------------------------------

const oauthStateCookie = "tl_oauth"

type oauthState struct {
	Nonce  string `json:"n"`
	Invite string `json:"i,omitempty"`
	Next   string `json:"x,omitempty"`
}

func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	st := oauthState{Nonce: base64.RawURLEncoding.EncodeToString(b), Invite: r.URL.Query().Get("invite"), Next: r.URL.Query().Get("next")}
	raw, _ := json.Marshal(st)
	enc := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: enc, Path: "/auth/google", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, s.oauth.AuthCodeURL(st.Nonce), http.StatusFound)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	redirectErr := func(msg string) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(msg), http.StatusFound)
	}
	c, err := r.Cookie(oauthStateCookie)
	if err != nil {
		redirectErr("sign-in session expired, please try again")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/auth/google", MaxAge: -1})
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	var st oauthState
	if err != nil || json.Unmarshal(raw, &st) != nil || st.Nonce == "" || st.Nonce != r.URL.Query().Get("state") {
		redirectErr("invalid sign-in state")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		redirectErr("google sign-in was cancelled")
		return
	}
	id, err := s.oauth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.log.Warn("oauth exchange failed", "err", err)
		redirectErr("could not verify your Google account")
		return
	}
	var next string
	err = db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		u, err := s.users().ByGoogleSub(r.Context(), tx, id.Subject)
		created := false
		if errors.Is(err, domain.ErrNotFound) {
			u, _, err = s.users().ByEmail(r.Context(), tx, id.Email)
			if errors.Is(err, domain.ErrNotFound) {
				u, err = s.users().Create(r.Context(), tx, auth.CreateUser{Email: id.Email, Name: id.Name, GoogleSub: id.Subject, AvatarURL: id.AvatarURL})
				created = true
			} else if err == nil {
				err = s.users().LinkGoogle(r.Context(), tx, u.ID, id.Subject, id.AvatarURL)
			}
		}
		if err != nil {
			return err
		}
		actor := domain.Actor{Type: domain.ActorUser, ID: &u.ID, Label: u.Email, IP: clientIP(r)}
		if st.Invite != "" {
			if _, err := s.orgs.AcceptInvite(r.Context(), tx, st.Invite, u, clientIP(r)); err != nil {
				var v *domain.ValidationError
				if !errors.As(err, &v) && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrConflict) {
					return err
				}
				s.log.Warn("invite accept via oauth failed", "err", err)
			}
		}
		orgs, err := s.orgs.ListForUser(r.Context(), tx, u.ID)
		if err != nil {
			return err
		}
		orgID := firstOrg(orgs)
		if _, err := s.startSession(w, r, tx, u, orgID); err != nil {
			return err
		}
		if orgID != nil {
			_ = activity.Record(r.Context(), tx, *orgID, actor, activity.Entry{Action: "auth.login", TargetType: "user", TargetID: u.ID.String(), TargetLabel: u.Email, Metadata: map[string]any{"method": "google", "created": created}})
			next = "/app"
		} else {
			next = "/onboarding"
		}
		if st.Next != "" && strings.HasPrefix(st.Next, "/") && !strings.HasPrefix(st.Next, "//") {
			next = st.Next
		}
		return nil
	})
	if err != nil {
		s.log.Error("oauth login failed", "err", err)
		redirectErr("could not complete sign-in")
		return
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// --- me ------------------------------------------------------------------------

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	resp, err := s.buildMe(r, userFrom(r.Context()), sessionFrom(r.Context()))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, resp)
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		s.fail(w, r, domain.Invalid("name", "name is required"))
		return
	}
	u := userFrom(r.Context())
	if err := s.users().UpdateProfile(r.Context(), s.pool, u.ID, in.Name); err != nil {
		s.fail(w, r, err)
		return
	}
	if sess := sessionFrom(r.Context()); sess.OrgID != nil {
		_ = activity.Record(r.Context(), s.pool, *sess.OrgID, actorFrom(r), activity.Entry{Action: "user.profile_updated", TargetType: "user", TargetID: u.ID.String(), TargetLabel: u.Email})
	}
	s.me(w, r)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	u := userFrom(r.Context())
	_, hash, err := s.users().ByEmail(r.Context(), s.pool, u.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if hash != "" && !auth.VerifyPassword(hash, in.Current) {
		s.fail(w, r, domain.Invalid("current_password", "current password is incorrect"))
		return
	}
	if err := auth.ValidatePassword(in.New, 12); err != nil {
		s.fail(w, r, domain.Invalid("new_password", err.Error()))
		return
	}
	newHash, err := auth.HashPassword(in.New)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.users().SetPassword(r.Context(), s.pool, u.ID, newHash); err != nil {
		s.fail(w, r, err)
		return
	}
	if sess := sessionFrom(r.Context()); sess.OrgID != nil {
		_ = activity.Record(r.Context(), s.pool, *sess.OrgID, actorFrom(r), activity.Entry{Action: "auth.password_changed", TargetType: "user", TargetID: u.ID.String(), TargetLabel: u.Email})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) switchOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrgID string `json:"org_id"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := domain.ParseID(in.OrgID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	u := userFrom(r.Context())
	if _, err := s.orgs.MembershipByUser(r.Context(), s.pool, id, u.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	sess := sessionFrom(r.Context())
	if err := s.sessions.SetOrg(r.Context(), sess.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	sess.OrgID = &id
	resp, err := s.buildMe(r, u, sess)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, resp)
}

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	u := userFrom(r.Context())
	sess := sessionFrom(r.Context())
	err := db.Tx(r.Context(), s.pool, func(tx pgx.Tx) error {
		o, err := s.orgs.Create(r.Context(), tx, in.Name, u.ID, actorFrom(r))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE sessions SET org_id = $2 WHERE id = $1`, sess.ID, o.ID); err != nil {
			return err
		}
		sess.OrgID = &o.ID
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp, err := s.buildMe(r, u, sess)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, resp)
}
