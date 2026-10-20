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
	"github.com/riverqueue/river/rivertype"
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

// --- Phase 1 job args -------------------------------------------------------
// Every tenant job carries OrgID; workers scope all queries by it.

// SchedulerTickArgs fans out a sync for every enabled integration.
type SchedulerTickArgs struct{}

func (SchedulerTickArgs) Kind() string { return "scheduler_tick" }
func (SchedulerTickArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDefault, MaxAttempts: 1}
}

// SyncIntegrationArgs pulls a snapshot from one integration.
type SyncIntegrationArgs struct {
	OrgID         string `json:"org_id"`
	IntegrationID string `json:"integration_id"`
	Trigger       string `json:"trigger"` // scheduled | manual | connect
}

func (SyncIntegrationArgs) Kind() string { return "sync_integration" }
func (SyncIntegrationArgs) InsertOpts() river.InsertOpts {
	// One pending sync per integration; a tick never queues duplicates.
	return river.InsertOpts{Queue: QueueSync, MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable}}}
}

// RunTestsArgs evaluates tests over a stored snapshot.
type RunTestsArgs struct {
	OrgID      string `json:"org_id"`
	SnapshotID string `json:"snapshot_id"`
	Trigger    string `json:"trigger"`
}

func (RunTestsArgs) Kind() string { return "run_tests" }
func (RunTestsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueTests, MaxAttempts: 3}
}

// RescanOrgArgs re-runs tests on the latest snapshot of every integration
// (after a framework is adopted or custom tests change).
type RescanOrgArgs struct {
	OrgID   string `json:"org_id"`
	Trigger string `json:"trigger"`
}

func (RescanOrgArgs) Kind() string { return "rescan_org" }
func (RescanOrgArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueTests, MaxAttempts: 3}
}

// GenerateRemediationArgs asks the AI layer for fix steps and Terraform.
type GenerateRemediationArgs struct {
	OrgID  string `json:"org_id"`
	TaskID string `json:"task_id"`
}

func (GenerateRemediationArgs) Kind() string { return "generate_remediation" }
func (GenerateRemediationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueAI, MaxAttempts: 3}
}

// ValidateEvidenceArgs asks the AI layer whether evidence satisfies its control.
type ValidateEvidenceArgs struct {
	OrgID      string `json:"org_id"`
	EvidenceID string `json:"evidence_id"`
}

func (ValidateEvidenceArgs) Kind() string { return "validate_evidence" }
func (ValidateEvidenceArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueAI, MaxAttempts: 3}
}

// SlackNotifyArgs posts a message to an org's Slack incoming webhook.
type SlackNotifyArgs struct {
	OrgID string `json:"org_id"`
	Text  string `json:"text"`
	Link  string `json:"link"`
}

func (SlackNotifyArgs) Kind() string { return "slack_notify" }
func (SlackNotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDefault, MaxAttempts: 5}
}

// ExpireEvidenceArgs re-evaluates controls whose evidence expired (daily).
type ExpireEvidenceArgs struct{}

func (ExpireEvidenceArgs) Kind() string { return "expire_evidence" }
func (ExpireEvidenceArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDefault, MaxAttempts: 3}
}
