// Package jobs wires River: job argument types, workers and the client.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/trustline/trustline/internal/mail"
)

const (
	QueueDefault = "default"
	QueueSync    = "sync"
	QueueTests   = "tests"
	QueueAI      = "ai"
	QueueExports = "exports"
)

// --- job args ---------------------------------------------------------------

type SendEmailArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

func (SendEmailArgs) Kind() string { return "send_email" }

func (SendEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDefault, MaxAttempts: 5}
}

// --- workers ------------------------------------------------------------------

type sendEmailWorker struct {
	river.WorkerDefaults[SendEmailArgs]
	sender mail.Sender
	log    *slog.Logger
}

func (w *sendEmailWorker) Work(ctx context.Context, job *river.Job[SendEmailArgs]) error {
	err := w.sender.Send(ctx, mail.Message{To: job.Args.To, Subject: job.Args.Subject, HTML: job.Args.HTML, Text: job.Args.Text})
	if err != nil {
		w.log.Warn("send email failed", "to", job.Args.To, "err", err, "attempt", job.Attempt)
		return err
	}
	w.log.Info("email sent", "to", job.Args.To, "subject", job.Args.Subject)
	return nil
}

func (w *sendEmailWorker) Timeout(*river.Job[SendEmailArgs]) time.Duration { return 30 * time.Second }

// --- client -------------------------------------------------------------------

// Deps are what workers need. Only the worker process sets them.
type Deps struct {
	Mail mail.Sender
	Log  *slog.Logger
}

// Client wraps a River client. In API processes it only inserts jobs; in the
// worker process it also works them.
type Client struct {
	River *river.Client[pgx.Tx]
}

// NewInsertOnly returns a client that can enqueue but never works jobs.
func NewInsertOnly(pool *pgxpool.Pool, log *slog.Logger) (*Client, error) {
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: log})
	if err != nil {
		return nil, err
	}
	return &Client{River: rc}, nil
}

// WorkerConfig controls the worker process.
type WorkerConfig struct {
	DefaultWorkers int
	PeriodicJobs   []*river.PeriodicJob
	Register       func(w *river.Workers) // additional module workers (Phase 1+)
}

// NewWorker returns a client configured with all workers and queues.
func NewWorker(pool *pgxpool.Pool, deps Deps, wc WorkerConfig) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &sendEmailWorker{sender: deps.Mail, log: deps.Log})
	if wc.Register != nil {
		wc.Register(workers)
	}
	if wc.DefaultWorkers <= 0 {
		wc.DefaultWorkers = 10
	}
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: deps.Log,
		Queues: map[string]river.QueueConfig{
			QueueDefault: {MaxWorkers: wc.DefaultWorkers},
			QueueSync:    {MaxWorkers: 5},
			QueueTests:   {MaxWorkers: 10},
			QueueAI:      {MaxWorkers: 2},
			QueueExports: {MaxWorkers: 2},
		},
		Workers:      workers,
		PeriodicJobs: wc.PeriodicJobs,
	})
	if err != nil {
		return nil, err
	}
	return &Client{River: rc}, nil
}

// EnqueueTx inserts a job inside tx so it commits with the business mutation.
func (c *Client) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs) error {
	_, err := c.River.InsertTx(ctx, tx, args, nil)
	return err
}

func (c *Client) Enqueue(ctx context.Context, args river.JobArgs) error {
	_, err := c.River.Insert(ctx, args, nil)
	return err
}
