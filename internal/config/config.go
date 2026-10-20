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
	GoogleOAuthMode    string // "fake" | "real" | "off"
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

	// Evidence storage
	BlobMode    string // "s3" | "fs"
	BlobDir     string
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3PathStyle bool

	// AI
	AIMode          string // "fake" | "anthropic"
	AnthropicAPIKey string
	AIModel         string
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
		BlobMode:           getenv("BLOB_MODE", "fs"),
		BlobDir:            getenv("BLOB_DIR", "./var/blobs"),
		S3Endpoint:         getenv("S3_ENDPOINT", ""),
		S3Region:           getenv("S3_REGION", "us-east-1"),
		S3Bucket:           getenv("S3_BUCKET", "trustline-evidence"),
		S3AccessKey:        getenv("S3_ACCESS_KEY", ""),
		S3SecretKey:        getenv("S3_SECRET_KEY", ""),
		S3PathStyle:        getbool("S3_PATH_STYLE", true),
		AIMode:             getenv("AI_MODE", "fake"),
		AnthropicAPIKey:    getenv("ANTHROPIC_API_KEY", ""),
		AIModel:            getenv("AI_MODEL", "claude-sonnet-4-6"),
	}
	if c.EncryptionKey == "" && c.Env != EnvProd {
		c.EncryptionKey = "dev-only-insecure-encryption-key-change-me"
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
	case "off":
	case "real":
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
			errs = append(errs, errors.New("GOOGLE_OAUTH_MODE=real requires GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET"))
		}
	default:
		errs = append(errs, fmt.Errorf("GOOGLE_OAUTH_MODE must be fake, real or off (got %q)", c.GoogleOAuthMode))
	}
	switch c.MailMode {
	case "smtp", "memory":
	default:
		errs = append(errs, fmt.Errorf("MAIL_MODE must be smtp or memory (got %q)", c.MailMode))
	}
	switch c.BlobMode {
	case "s3", "fs", "postgres":
	default:
		errs = append(errs, fmt.Errorf("BLOB_MODE must be s3, fs or postgres (got %q)", c.BlobMode))
	}
	switch c.AIMode {
	case "fake":
	case "anthropic":
		// The SDK also resolves ANTHROPIC_AUTH_TOKEN / ant profiles, so a missing key is not fatal.
	default:
		errs = append(errs, fmt.Errorf("AI_MODE must be fake or anthropic (got %q)", c.AIMode))
	}
	if len(c.EncryptionKey) < 16 {
		errs = append(errs, errors.New("ENCRYPTION_KEY must be at least 16 characters"))
	}
	if c.Env == EnvProd && c.EncryptionKey == "" {
		errs = append(errs, errors.New("ENCRYPTION_KEY is required in prod"))
	}
	return errors.Join(errs...)
}

func (c *Config) IsDev() bool { return c.Env == EnvDev }
