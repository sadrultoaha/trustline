// Command trustline is the single binary: serve | worker | all | migrate | seed.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/trustline/trustline/internal/app"
	"github.com/trustline/trustline/internal/auth/oauth"
	"github.com/trustline/trustline/internal/bootstrap"
	"github.com/trustline/trustline/internal/config"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/httpapi"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/ratelimit"
	"github.com/trustline/trustline/internal/seed"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if os.Getenv("LOG_FORMAT") == "json" {
		log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var runErr error
	switch os.Args[1] {
	case "serve":
		runErr = serve(ctx, cfg, log)
	case "worker":
		runErr = worker(ctx, cfg, log)
	case "all":
		runErr = all(ctx, cfg, log)
	case "migrate":
		runErr = migrate(ctx, cfg, log)
	case "seed":
		runErr = runSeed(ctx, cfg, log)
	case "version":
		fmt.Println("trustline dev")
	default:
		usage()
		os.Exit(2)
	}
	if runErr != nil {
		log.Error(os.Args[1]+" failed", "err", runErr)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: trustline <serve|worker|all|migrate|seed|version>")
}

func connectDB(ctx context.Context, cfg *config.Config, log *slog.Logger) (pool, error) {
	// Retry so `docker compose up` ordering is forgiving.
	var last error
	for i := 0; i < 30; i++ {
		p, err := db.Connect(ctx, cfg.DatabaseURL)
		if err == nil {
			return p, nil
		}
		last = err
		log.Warn("database not ready, retrying", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, last
}

type pool = interface {
	Close()
}

func migrate(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	p, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer p.Close()
	if err := bootstrap.Prepare(ctx, p, log); err != nil {
		return err
	}
	log.Info("migrations applied and catalog synced")
	return nil
}

func runSeed(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	p, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer p.Close()
	if err := bootstrap.Prepare(ctx, p, log); err != nil {
		return err
	}
	res, err := seed.Run(ctx, p, cfg, log)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Demo accounts (password for all:", seed.DemoPassword+")")
	for _, u := range seed.AcmeUsers {
		fmt.Printf("  %-22s %-16s Acme Corp\n", u.Email, u.Role)
	}
	for _, u := range seed.GlobexUsers {
		fmt.Printf("  %-22s %-16s Globex Corporation\n", u.Email, u.Role)
	}
	fmt.Println()
	_ = res
	return nil
}

func newRedis(cfg *config.Config, log *slog.Logger) *redis.Client {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Warn("invalid REDIS_URL, running without redis", "err", err)
		return nil
	}
	c := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		if cfg.Env == config.EnvProd {
			log.Error("redis unreachable", "err", err)
			os.Exit(1)
		}
		log.Warn("redis unreachable, rate limiting disabled", "err", err)
		return nil
	}
	return c
}

func serve(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	p, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer p.Close()
	// The API process applies migrations on boot so `docker compose up` is enough.
	if err := bootstrap.Prepare(ctx, p, log); err != nil {
		return err
	}
	rdb := newRedis(cfg, log)
	var limiter ratelimit.Limiter = ratelimit.Noop{}
	if rdb != nil {
		limiter = &ratelimit.Redis{C: rdb}
	}
	jc, err := jobs.NewInsertOnly(p, log)
	if err != nil {
		return err
	}
	var provider oauth.Provider
	redirect := cfg.BaseURL + "/auth/google/callback"
	switch cfg.GoogleOAuthMode {
	case "real":
		provider = oauth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, redirect)
	case "fake":
		provider = oauth.NewFake(cfg.BaseURL, redirect)
	}
	comp, err := app.NewCompliance(ctx, cfg, p, jc, log)
	if err != nil {
		return err
	}
	srv := httpapi.New(httpapi.Deps{Config: cfg, Log: log, Pool: p, Redis: rdb, Jobs: jc, OAuth: provider, Limiter: limiter, Compliance: comp})
	hs := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "oauth", cfg.GoogleOAuthMode, "web_dist", cfg.WebDist)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func worker(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	p, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer p.Close()
	if err := bootstrap.Prepare(ctx, p, log); err != nil {
		return err
	}
	var sender mail.Sender
	if cfg.MailMode == "memory" {
		sender = &mail.Memory{}
	} else {
		sender = &mail.SMTP{Addr: cfg.SMTPAddr, From: cfg.MailFrom}
	}
	w, err := app.NewWorker(ctx, cfg, p, sender, app.NewAIProvider(cfg), log)
	if err != nil {
		return err
	}
	if err := w.Client.River.Start(ctx); err != nil {
		return err
	}
	log.Info("worker started", "mail", cfg.MailMode, "smtp", cfg.SMTPAddr, "ai", w.AI.Provider.Name(), "blob", w.Comp.Blob.Name(), "scheduler_interval", cfg.SchedulerInterval)
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return w.Client.River.Stop(sctx)
}

// all runs the API and the worker in one process, for single-machine hosts
// where both must share a local blob directory. If either stops, both stop.
func all(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2)
	go func() { errc <- worker(ctx, cfg, log); cancel() }()
	go func() { errc <- serve(ctx, cfg, log); cancel() }()
	return errors.Join(<-errc, <-errc)
}
