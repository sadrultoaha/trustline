package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/rbac"
)

const sessionCookie = "tl_session"

type ctxKey int

const (
	ctxSession ctxKey = iota
	ctxUser
	ctxTenant
)

// Tenant is the resolved org context for a request.
type Tenant struct {
	OrgID        domain.ID
	UserID       domain.ID
	Role         domain.Role
	MembershipID domain.ID
}

func sessionFrom(ctx context.Context) *domain.Session {
	s, _ := ctx.Value(ctxSession).(*domain.Session)
	return s
}

func userFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(ctxUser).(*domain.User)
	return u
}

// TenantFrom returns the tenant context; handlers behind requireOrg can rely on it.
func TenantFrom(ctx context.Context) Tenant {
	t, _ := ctx.Value(ctxTenant).(Tenant)
	return t
}

func actorFrom(r *http.Request) domain.Actor {
	u := userFrom(r.Context())
	if u == nil {
		return domain.SystemActor("anonymous")
	}
	return domain.Actor{Type: domain.ActorUser, ID: &u.ID, Label: u.Email, IP: r.RemoteAddr}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, Expires: expires,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// loadSession attaches session+user to the context if the cookie is valid.
func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		sess, err := s.sessions.Lookup(r.Context(), c.Value)
		if err != nil {
			if err == domain.ErrUnauthorized {
				s.clearSessionCookie(w)
			}
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.users().ByID(r.Context(), s.pool, sess.UserID)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), ctxSession, sess)
		ctx = context.WithValue(ctx, ctxUser, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()) == nil {
			writeError(w, 401, "unauthorized", "sign in required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireOrg resolves the session's current org and the user's membership in it.
func (s *Server) requireOrg(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		u := userFrom(r.Context())
		if sess == nil || sess.OrgID == nil {
			writeError(w, 403, "no_org", "select or create an organization first")
			return
		}
		m, err := s.orgs.MembershipByUser(r.Context(), s.pool, *sess.OrgID, u.ID)
		if err != nil {
			writeError(w, 403, "no_org", "you are not a member of this organization")
			return
		}
		t := Tenant{OrgID: m.OrgID, UserID: u.ID, Role: m.Role, MembershipID: m.ID}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxTenant, t)))
	})
}

// Permission aliases keep route tables short.
const (
	permOrgRead      = rbac.OrgRead
	permOrgWrite     = rbac.OrgWrite
	permMembersRead  = rbac.MembersRead
	permMembersWrite = rbac.MembersWrite
	permActivityRead = rbac.ActivityRead
)

// require enforces a permission for the tenant's role.
func (s *Server) require(p rbac.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := TenantFrom(r.Context())
			if !rbac.Can(t.Role, p) {
				writeError(w, 403, "forbidden", "your role ("+string(t.Role)+") cannot perform this action")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Require is the exported form for module routers.
func (s *Server) Require(p rbac.Permission) func(http.Handler) http.Handler { return s.require(p) }

func pathID(r *http.Request, name string) (domain.ID, error) {
	return domain.ParseID(chi.URLParam(r, name))
}

// rateLimit applies a per-key limit; key is derived by fn.
func (s *Server) rateLimit(keyPrefix string, limit int, window time.Duration, keyFn func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyPrefix + ":" + keyFn(r)
			ok, err := s.limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				s.log.Warn("rate limiter error", "err", err)
				ok = true
			}
			if !ok {
				writeError(w, 429, "rate_limited", "too many attempts, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string { return r.RemoteAddr }

func pathParam(r *http.Request, name string) string { return chi.URLParam(r, name) }
