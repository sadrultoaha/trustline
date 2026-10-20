// Package app assembles services from configuration. cmd/trustline and the
// integration-test harness use the same constructors, so tests exercise the
// production wiring with fakes swapped in by config.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/blob"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/config"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/secrets"
	"github.com/trustline/trustline/internal/worker"
)

func NewBlob(ctx context.Context, cfg *config.Config) (blob.Store, error) {
	if cfg.BlobMode == "s3" {
		return blob.NewS3(ctx, blob.S3Config{Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle})
	}
	return &blob.FS{Root: cfg.BlobDir}, nil
}

func NewAIProvider(cfg *config.Config) ai.Provider {
	if cfg.AIMode == "anthropic" {
		return ai.NewAnthropic(cfg.AnthropicAPIKey, cfg.AIModel)
	}
	return ai.Fake{}
}

// NewCompliance builds the compliance service.
func NewCompliance(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, jc *jobs.Client, log *slog.Logger) (*compliance.Service, error) {
	box, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	store, err := NewBlob(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cat, err := catalog.Load()
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	return &compliance.Service{Pool: pool, Jobs: jc, Box: box, Blob: store, Catalog: cat, Log: log, BaseURL: cfg.BaseURL}, nil
}

// Worker is a running job processor.
type Worker struct {
	Client *jobs.Client
	Comp   *compliance.Service
	AI     *ai.Service
}

// NewWorker builds a River client that works every queue and runs the schedules.
func NewWorker(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, sender mail.Sender, provider ai.Provider, log *slog.Logger) (*Worker, error) {
	w := &Worker{}
	deps := &worker.Deps{Log: log, Jobs: func() *jobs.Client { return w.Client }}
	insert, err := jobs.NewInsertOnly(pool, log)
	if err != nil {
		return nil, err
	}
	comp, err := NewCompliance(ctx, cfg, pool, insert, log)
	if err != nil {
		return nil, err
	}
	aiSvc := &ai.Service{Provider: provider, Pool: pool, Log: log}
	deps.Compliance, deps.AI = comp, aiSvc
	client, err := jobs.NewWorker(pool, jobs.Deps{Mail: sender, Log: log}, jobs.WorkerConfig{
		DefaultWorkers: cfg.WorkerQueueDefault,
		PeriodicJobs:   worker.Periodic(cfg.SchedulerInterval),
		Register:       func(rw *river.Workers) { worker.Register(rw, deps) },
	})
	if err != nil {
		return nil, err
	}
	w.Client, w.Comp, w.AI = client, comp, aiSvc
	comp.Jobs = client
	return w, nil
}
