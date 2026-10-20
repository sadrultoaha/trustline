// Package httpapi is the HTTP layer: router, middleware and handlers.
package httpapi

import (
	_ "embed"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/auth/oauth"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/config"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/org"
	"github.com/trustline/trustline/internal/ratelimit"
)

//go:embed static/trustline-agent.sh
var agentScript []byte

// Server holds dependencies shared by handlers.
type Server struct {
	cfg      *config.Config
	log      *slog.Logger
	pool     *pgxpool.Pool
	redis    *redis.Client
	sessions *auth.Sessions
	orgs     *org.Service
	jobs     *jobs.Client
	oauth    oauth.Provider
	limiter  ratelimit.Limiter
	comp     *compliance.Service
	// Mount lets later phases register module routers under /api/v1 with tenant middleware applied.
	modules []func(r chi.Router)
}

type Deps struct {
	Config   *config.Config
	Log      *slog.Logger
	Pool     *pgxpool.Pool
	Redis    *redis.Client // may be nil
	Jobs     *jobs.Client
	OAuth    oauth.Provider
	Limiter  ratelimit.Limiter
	Sessions *auth.Sessions
	// Compliance is required for Phase 1+ routes.
	Compliance *compliance.Service
}

func New(d Deps) *Server {
	s := &Server{
		cfg:      d.Config,
		log:      d.Log,
		pool:     d.Pool,
		redis:    d.Redis,
		sessions: d.Sessions,
		orgs:     org.New(d.Pool),
		jobs:     d.Jobs,
		oauth:    d.OAuth,
		limiter:  d.Limiter,
		comp:     d.Compliance,
	}
	if s.sessions == nil {
		s.sessions = auth.NewSessions(d.Pool, d.Config.SessionTTL)
	}
	if s.limiter == nil {
		s.limiter = ratelimit.Noop{}
	}
	return s
}

// AddModule registers a function that mounts tenant-scoped routes under /api/v1.
func (s *Server) AddModule(fn func(r chi.Router)) { s.modules = append(s.modules, fn) }

// Handler builds the full router.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(requestLogger(s.log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/agent/trustline-agent.sh", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		_, _ = w.Write(agentScript)
	})
	r.Get("/readyz", s.ready)

	// OAuth browser redirects live outside /api so they are plain GETs.
	if s.oauth != nil {
		r.Get("/auth/google/start", s.googleStart)
		r.Get("/auth/google/callback", s.googleCallback)
		s.oauth.Mount(r)
	}

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(jsonOnly)
		api.Use(s.loadSession) // sets user if a valid cookie exists; never rejects

		api.Get("/auth/providers", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 200, map[string]bool{"google": s.oauth != nil})
		})
		api.Post("/auth/signup", s.signup)
		api.Post("/auth/login", s.login)
		api.Post("/auth/logout", s.logout)
		api.Post("/auth/forgot-password", s.forgotPassword)
		api.Post("/auth/reset-password", s.resetPassword)
		api.Get("/invites/{token}", s.invitePreview)
		if s.comp != nil {
			api.Post("/agent/report", s.agentReport)
			s.mountVendorPortal(api)
		}
		api.Post("/invites/{token}/accept", s.inviteAccept)

		api.Group(func(authed chi.Router) {
			authed.Use(s.requireUser)
			authed.Get("/me", s.me)
			authed.Patch("/me", s.updateMe)
			authed.Post("/me/password", s.changePassword)
			authed.Post("/me/switch-org", s.switchOrg)
			authed.Post("/orgs", s.createOrg)
			authed.Group(func(t chi.Router) {
				t.Use(s.requireOrg)
				t.Get("/notifications", s.listNotifications)
				t.Post("/notifications/read", s.readNotifications)
			})

			authed.Group(func(t chi.Router) {
				t.Use(s.requireOrg)
				t.With(s.require(permOrgRead)).Get("/org", s.getOrg)
				t.With(s.require(permOrgWrite)).Patch("/org", s.updateOrg)
				t.With(s.require(permMembersRead)).Get("/org/members", s.listMembers)
				t.With(s.require(permMembersRead)).Get("/org/members/{id}", s.getMember)
				t.With(s.require(permMembersWrite)).Patch("/org/members/{id}", s.updateMember)
				t.With(s.require(permMembersWrite)).Delete("/org/members/{id}", s.removeMember)
				t.With(s.require(permMembersRead)).Get("/org/invites", s.listInvites)
				t.With(s.require(permMembersWrite)).Post("/org/invites", s.createInvite)
				t.With(s.require(permMembersWrite)).Delete("/org/invites/{id}", s.revokeInvite)
				t.With(s.require(permMembersWrite)).Post("/org/invites/{id}/resend", s.resendInvite)
				t.With(s.require(permActivityRead)).Get("/activity", s.listActivity)
				if s.comp != nil {
					s.mountCompliance(t)
					s.mountPhase2(t)
					s.mountPhase3(t)
				}
				for _, m := range s.modules {
					m(t)
				}
			})
		})
		api.NotFound(func(w http.ResponseWriter, _ *http.Request) { writeError(w, 404, "not_found", "route not found") })
	})

	if s.cfg.WebDist != "" {
		s.mountStatic(r, s.cfg.WebDist)
	} else {
		r.Get("/*", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Trustline API is running. The web app is served by Vite in development (http://localhost:5173) or set WEB_DIST.\n"))
		})
	}
	return r
}

// mountStatic serves the built SPA with index.html fallback.
func (s *Server) mountStatic(r chi.Router, dir string) {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+req.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if strings.HasPrefix(req.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fs.ServeHTTP(w, req)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, index)
	})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.pool.Ping(ctx); err != nil {
		writeError(w, 503, "db_unavailable", err.Error())
		return
	}
	if s.redis != nil {
		if err := s.redis.Ping(ctx).Err(); err != nil {
			writeError(w, 503, "redis_unavailable", err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// jsonOnly rejects state-changing requests without a JSON content type (CSRF
// hardening: browsers cannot send application/json cross-site without CORS).
func jsonOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.ContentLength != 0 {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") && !strings.HasPrefix(ct, "multipart/form-data") {
				writeError(w, 415, "unsupported_media_type", "send application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/healthz" {
				return
			}
			log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "ms", time.Since(start).Milliseconds(), "ip", r.RemoteAddr, "rid", middleware.GetReqID(r.Context()))
		})
	}
}
