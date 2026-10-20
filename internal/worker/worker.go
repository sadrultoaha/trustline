// Package worker registers every background job handler and periodic schedule.
// Only the `trustline worker` process (and integration tests) use it.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/riverqueue/river"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/compliance"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
)

type Deps struct {
	Compliance *compliance.Service
	AI         *ai.Service
	Jobs       func() *jobs.Client // resolved lazily: the client is built after workers are registered
	Log        *slog.Logger
	HTTP       *http.Client
}

// Register adds all Phase 1 workers.
func Register(w *river.Workers, d *Deps) {
	if d.HTTP == nil {
		d.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	river.AddWorker(w, &tickWorker{d: d})
	river.AddWorker(w, &syncWorker{d: d})
	river.AddWorker(w, &runTestsWorker{d: d})
	river.AddWorker(w, &rescanWorker{d: d})
	river.AddWorker(w, &remediationWorker{d: d})
	river.AddWorker(w, &evidenceWorker{d: d})
	river.AddWorker(w, &slackWorker{d: d})
	river.AddWorker(w, &expireWorker{d: d})
	river.AddWorker(w, &consistencyWorker{d: d})
	river.AddWorker(w, &dailyWorker{d: d})
	river.AddWorker(w, &platformWorker{d: d})
	river.AddWorker(w, &vendorAssessmentWorker{d: d})
}

// Periodic returns the schedules: connector syncs every interval, evidence expiry daily.
func Periodic(interval time.Duration) []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(interval), func() (river.JobArgs, *river.InsertOpts) {
			return jobs.SchedulerTickArgs{}, nil
		}, &river.PeriodicJobOpts{RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour), func() (river.JobArgs, *river.InsertOpts) {
			return jobs.ExpireEvidenceArgs{}, nil
		}, &river.PeriodicJobOpts{RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour), func() (river.JobArgs, *river.InsertOpts) {
			return jobs.DailyMaintenanceArgs{}, nil
		}, &river.PeriodicJobOpts{RunOnStart: true}),
	}
}

func ids(org, other string) (domain.ID, domain.ID, error) {
	o, err := domain.ParseID(org)
	if err != nil {
		return o, o, fmt.Errorf("bad org id: %w", err)
	}
	x, err := domain.ParseID(other)
	if err != nil {
		return o, x, fmt.Errorf("bad id: %w", err)
	}
	return o, x, nil
}

type tickWorker struct {
	river.WorkerDefaults[jobs.SchedulerTickArgs]
	d *Deps
}

func (w *tickWorker) Work(ctx context.Context, _ *river.Job[jobs.SchedulerTickArgs]) error {
	list, err := w.d.Compliance.IntegrationsForTick(ctx)
	if err != nil {
		return err
	}
	for _, pair := range list {
		if err := w.d.Jobs().Enqueue(ctx, jobs.SyncIntegrationArgs{OrgID: pair[0].String(), IntegrationID: pair[1].String(), Trigger: "scheduled"}); err != nil {
			return err
		}
	}
	w.d.Log.Info("scheduler tick", "integrations", len(list))
	return nil
}

type syncWorker struct {
	river.WorkerDefaults[jobs.SyncIntegrationArgs]
	d *Deps
}

func (w *syncWorker) Work(ctx context.Context, job *river.Job[jobs.SyncIntegrationArgs]) error {
	org, id, err := ids(job.Args.OrgID, job.Args.IntegrationID)
	if err != nil {
		return river.JobCancel(err)
	}
	snap, err := w.d.Compliance.SyncIntegration(ctx, org, id, job.Args.Trigger)
	if err != nil {
		w.d.Log.Warn("sync failed", "integration", id, "err", err)
		return err
	}
	if snap != nil {
		w.d.Log.Info("synced", "integration", id, "snapshot", *snap, "trigger", job.Args.Trigger)
	}
	return nil
}

type runTestsWorker struct {
	river.WorkerDefaults[jobs.RunTestsArgs]
	d *Deps
}

func (w *runTestsWorker) Work(ctx context.Context, job *river.Job[jobs.RunTestsArgs]) error {
	org, snap, err := ids(job.Args.OrgID, job.Args.SnapshotID)
	if err != nil {
		return river.JobCancel(err)
	}
	n, err := w.d.Compliance.RunTests(ctx, org, snap, job.Args.Trigger)
	if err != nil {
		return err
	}
	w.d.Log.Info("tests evaluated", "org", org, "snapshot", snap, "tests", n)
	return nil
}

type rescanWorker struct {
	river.WorkerDefaults[jobs.RescanOrgArgs]
	d *Deps
}

func (w *rescanWorker) Work(ctx context.Context, job *river.Job[jobs.RescanOrgArgs]) error {
	org, err := domain.ParseID(job.Args.OrgID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.d.Compliance.RescanOrg(ctx, org, job.Args.Trigger)
}

type remediationWorker struct {
	river.WorkerDefaults[jobs.GenerateRemediationArgs]
	d *Deps
}

func (w *remediationWorker) Work(ctx context.Context, job *river.Job[jobs.GenerateRemediationArgs]) error {
	org, task, err := ids(job.Args.OrgID, job.Args.TaskID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.d.Compliance.GenerateRemediation(ctx, w.d.AI, org, task)
}

func (w *remediationWorker) Timeout(*river.Job[jobs.GenerateRemediationArgs]) time.Duration {
	return 3 * time.Minute
}

type evidenceWorker struct {
	river.WorkerDefaults[jobs.ValidateEvidenceArgs]
	d *Deps
}

func (w *evidenceWorker) Work(ctx context.Context, job *river.Job[jobs.ValidateEvidenceArgs]) error {
	org, ev, err := ids(job.Args.OrgID, job.Args.EvidenceID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.d.Compliance.ValidateEvidence(ctx, w.d.AI, org, ev)
}

func (w *evidenceWorker) Timeout(*river.Job[jobs.ValidateEvidenceArgs]) time.Duration {
	return 3 * time.Minute
}

type slackWorker struct {
	river.WorkerDefaults[jobs.SlackNotifyArgs]
	d *Deps
}

func (w *slackWorker) Work(ctx context.Context, job *river.Job[jobs.SlackNotifyArgs]) error {
	org, err := domain.ParseID(job.Args.OrgID)
	if err != nil {
		return river.JobCancel(err)
	}
	url, err := w.d.Compliance.SlackWebhook(ctx, org)
	if err != nil {
		return err
	}
	if url == "" {
		return nil // webhook removed since the job was queued
	}
	body, _ := json.Marshal(map[string]any{
		"text": job.Args.Text,
		"blocks": []any{
			map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": job.Args.Text}},
			map[string]any{"type": "actions", "elements": []any{map[string]any{"type": "button", "text": map[string]any{"type": "plain_text", "text": "Open in Trustline"}, "url": job.Args.Link}}},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return river.JobCancel(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := w.d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %d", res.StatusCode)
	}
	return nil
}

type expireWorker struct {
	river.WorkerDefaults[jobs.ExpireEvidenceArgs]
	d *Deps
}

func (w *expireWorker) Work(ctx context.Context, _ *river.Job[jobs.ExpireEvidenceArgs]) error {
	return w.d.Compliance.ExpireEvidence(ctx)
}

type consistencyWorker struct {
	river.WorkerDefaults[jobs.PolicyConsistencyArgs]
	d *Deps
}

func (w *consistencyWorker) Work(ctx context.Context, job *river.Job[jobs.PolicyConsistencyArgs]) error {
	org, v, err := ids(job.Args.OrgID, job.Args.VersionID)
	if err != nil {
		return river.JobCancel(err)
	}
	_, err = w.d.Compliance.CheckConsistency(ctx, w.d.AI, org, v)
	var verr *domain.ValidationError
	if errors.As(err, &verr) {
		return nil // AI disabled for the org
	}
	return err
}

type dailyWorker struct {
	river.WorkerDefaults[jobs.DailyMaintenanceArgs]
	d *Deps
}

func (w *dailyWorker) Work(ctx context.Context, _ *river.Job[jobs.DailyMaintenanceArgs]) error {
	c := w.d.Compliance
	if err := c.PolicyReviewReminders(ctx); err != nil {
		return fmt.Errorf("policy reviews: %w", err)
	}
	if err := c.ScheduleAccessReviews(ctx); err != nil {
		return fmt.Errorf("access reviews: %w", err)
	}
	if err := c.TrainingRenewals(ctx); err != nil {
		return fmt.Errorf("training renewals: %w", err)
	}
	if err := c.RiskReviewReminders(ctx); err != nil {
		return fmt.Errorf("risk reviews: %w", err)
	}
	if err := c.VendorReviewReminders(ctx); err != nil {
		return fmt.Errorf("vendor reviews: %w", err)
	}
	return nil
}

type platformWorker struct {
	river.WorkerDefaults[jobs.PlatformSyncArgs]
	d *Deps
}

func (w *platformWorker) Work(ctx context.Context, job *river.Job[jobs.PlatformSyncArgs]) error {
	org, err := domain.ParseID(job.Args.OrgID)
	if err != nil {
		return river.JobCancel(err)
	}
	id, err := w.d.Compliance.PlatformIntegrationID(ctx, org)
	if err != nil || id == nil {
		return err
	}
	_, err = w.d.Compliance.SyncIntegration(ctx, org, *id, "platform")
	return err
}

type vendorAssessmentWorker struct {
	river.WorkerDefaults[jobs.VendorAssessmentArgs]
	d *Deps
}

func (w *vendorAssessmentWorker) Work(ctx context.Context, job *river.Job[jobs.VendorAssessmentArgs]) error {
	org, id, err := ids(job.Args.OrgID, job.Args.AssessmentID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.d.Compliance.RunVendorAssessment(ctx, w.d.AI, org, id)
}

func (w *vendorAssessmentWorker) Timeout(*river.Job[jobs.VendorAssessmentArgs]) time.Duration {
	return 5 * time.Minute
}
