// Package testutil starts real Postgres via testcontainers and builds an
// in-process API server for integration tests.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/auth/oauth"
	"github.com/trustline/trustline/internal/config"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/httpapi"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/ratelimit"
)

var (
	once    sync.Once
	dbURL   string
	dbErr   error
	cleanup func()
)

// DatabaseURL returns a migrated Postgres URL, starting a container on first
// use. Set TEST_DATABASE_URL to reuse an existing database instead.
func DatabaseURL(t testing.TB) string {
	t.Helper()
	once.Do(func() {
		if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
			dbURL = u
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			c, err := tcpg.Run(ctx, "postgres:16-alpine",
				tcpg.WithDatabase("trustline_test"), tcpg.WithUsername("test"), tcpg.WithPassword("test"),
				testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
			)
			if err != nil {
				dbErr = fmt.Errorf("start postgres container: %w", err)
				return
			}
			cleanup = func() { _ = c.Terminate(context.Background()) }
			dbURL, dbErr = c.ConnectionString(ctx, "sslmode=disable")
			if dbErr != nil {
				return
			}
		}
		ctx := context.Background()
		pool, err := db.Connect(ctx, dbURL)
		if err != nil {
			dbErr = err
			return
		}
		defer pool.Close()
		dbErr = db.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	})
	if dbErr != nil {
		t.Fatalf("test database: %v", dbErr)
	}
	return dbURL
}

// Cleanup terminates the container; call from TestMain.
func Cleanup() {
	if cleanup != nil {
		cleanup()
	}
}

// Pool returns a pool connected to the test database.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, err := db.Connect(context.Background(), DatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// App is an in-process API server with fakes for every external dependency.
type App struct {
	Pool   *pgxpool.Pool
	Server *httptest.Server
	Mail   *mail.Memory
	OAuth  *oauth.Fake
	Config *config.Config
	Jobs   *jobs.Client
}

// NewApp builds the API with memory mail, memory rate limiting and fake OAuth.
// Jobs are inserted but not worked unless WorkJobs is called.
func NewApp(t testing.TB) *App {
	t.Helper()
	pool := Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if os.Getenv("TEST_LOG") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	cfg := &config.Config{Env: config.EnvTest, BaseURL: "http://app.test", SessionTTL: time.Hour, GoogleOAuthMode: "fake", MailMode: "memory"}
	jc, err := jobs.NewInsertOnly(pool, log)
	if err != nil {
		t.Fatal(err)
	}
	fake := oauth.NewFake(cfg.BaseURL, cfg.BaseURL+"/auth/google/callback")
	srv := httpapi.New(httpapi.Deps{Config: cfg, Log: log, Pool: pool, Jobs: jc, OAuth: fake, Limiter: ratelimit.NewMemory(), Sessions: auth.NewSessions(pool, cfg.SessionTTL)})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &App{Pool: pool, Server: ts, Mail: &mail.Memory{}, OAuth: fake, Config: cfg, Jobs: jc}
}

// Client is an HTTP client with its own cookie jar (one browser session).
type Client struct {
	t    testing.TB
	base string
	http *http.Client
}

func (a *App) Client(t testing.TB) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{t: t, base: a.Server.URL, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type Resp struct {
	Status int
	Body   map[string]any
	Raw    []byte
	Header http.Header
}

func (c *Client) Do(method, path string, body any) Resp {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, c.base+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := Resp{Status: res.StatusCode, Raw: raw, Header: res.Header}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func (c *Client) Get(path string) Resp          { return c.Do("GET", path, nil) }
func (c *Client) Post(path string, b any) Resp  { return c.Do("POST", path, b) }
func (c *Client) Patch(path string, b any) Resp { return c.Do("PATCH", path, b) }
func (c *Client) Delete(path string) Resp       { return c.Do("DELETE", path, nil) }
func (c *Client) Must(r Resp, status int) Resp {
	c.t.Helper()
	if r.Status != status {
		c.t.Fatalf("expected %d got %d: %s", status, r.Status, r.Raw)
	}
	return r
}

// Signup creates a user + org and returns the signed-in client.
func (a *App) Signup(t testing.TB, email, orgName string) *Client {
	t.Helper()
	c := a.Client(t)
	c.Must(c.Post("/api/v1/auth/signup", map[string]any{"email": email, "password": "correct-horse-battery", "name": strings.Split(email, "@")[0], "org_name": orgName}), 201)
	return c
}

// Unique returns a unique string for test isolation.
func Unique(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// DoForm posts an application/x-www-form-urlencoded body (HTML form submit).
func (c *Client) DoForm(path string, fields map[string]string) Resp {
	c.t.Helper()
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	req, _ := http.NewRequest("POST", c.base+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("POST form %s: %v", path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return Resp{Status: res.StatusCode, Raw: raw, Header: res.Header}
}
