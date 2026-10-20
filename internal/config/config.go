// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Env string

const (
	EnvDev  Env = "dev"
	EnvTest Env = "test"
	EnvProd Env = "prod"
)

// Config is the complete runtime configuration. Every field is set from an
// environment variable; .env.example documents them.
type Config struct {
	Env         Env
	HTTPAddr    string
	BaseURL     string // public URL of the app, used in emails and OAuth redirects
	DatabaseURL string
	RedisURL    string
	WebDist     string // directory containing the built frontend; empty = API only

	SecureCookies bool
	SessionTTL    time.Duration

	// Google OAuth
	GoogleOAuthMode    string // "fake" | "real"
	GoogleClientID     string
	GoogleClientSecret string

	// Mail
	MailMode string // "smtp" | "memory"
	SMTPAddr string // host:port
	MailFrom string

	// Encryption key for secrets at rest (32 bytes, base64 or hex). Used from Phase 1.
	EncryptionKey string

	// Scheduler tick for connector syncs (Phase 1). Short in dev so demos are snappy.
	SchedulerInterval time.Duration

	// Worker concurrency
	WorkerQueueDefault int
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getdur(key string, def time.Duration) time.Duration {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getint(key string, def int) int {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	env := Env(getenv("TRUSTLINE_ENV", string(EnvDev)))
	c := &Config{
		Env:                env,
		HTTPAddr:           getenv("HTTP_ADDR", ":8080"),
		BaseURL:            strings.TrimRight(getenv("BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL:        getenv("DATABASE_URL", "postgres://trustline:trustline@localhost:5432/trustline?sslmode=disable"),
		RedisURL:           getenv("REDIS_URL", "redis://localhost:6380/0"),
		WebDist:            getenv("WEB_DIST", ""),
		SecureCookies:      getbool("SECURE_COOKIES", env == EnvProd),
		SessionTTL:         getdur("SESSION_TTL", 30*24*time.Hour),
		GoogleOAuthMode:    getenv("GOOGLE_OAUTH_MODE", "fake"),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
		MailMode:           getenv("MAIL_MODE", "smtp"),
		SMTPAddr:           getenv("SMTP_ADDR", "localhost:1025"),
		MailFrom:           getenv("MAIL_FROM", "Trustline <no-reply@trustline.local>"),
		EncryptionKey:      getenv("ENCRYPTION_KEY", ""),
		SchedulerInterval:  getdur("SCHEDULER_INTERVAL", time.Hour),
		WorkerQueueDefault: getint("WORKER_QUEUE_DEFAULT", 10),
	}
	return c, c.Validate()
}

// Validate returns an error for inconsistent configuration.
func (c *Config) Validate() error {
	var errs []error
	switch c.Env {
	case EnvDev, EnvTest, EnvProd:
	default:
		errs = append(errs, fmt.Errorf("TRUSTLINE_ENV must be dev, test or prod (got %q)", c.Env))
	}
	switch c.GoogleOAuthMode {
	case "fake":
		if c.Env == EnvProd {
			errs = append(errs, errors.New("GOOGLE_OAUTH_MODE=fake is not allowed in prod"))
		}
	case "real":
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
			errs = append(errs, errors.New("GOOGLE_OAUTH_MODE=real requires GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET"))
		}
	default:
		errs = append(errs, fmt.Errorf("GOOGLE_OAUTH_MODE must be fake or real (got %q)", c.GoogleOAuthMode))
	}
	switch c.MailMode {
	case "smtp", "memory":
	default:
		errs = append(errs, fmt.Errorf("MAIL_MODE must be smtp or memory (got %q)", c.MailMode))
	}
	if c.Env == EnvProd && c.EncryptionKey == "" {
		errs = append(errs, errors.New("ENCRYPTION_KEY is required in prod"))
	}
	return errors.Join(errs...)
}

func (c *Config) IsDev() bool { return c.Env == EnvDev }
