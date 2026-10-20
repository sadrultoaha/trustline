package compliance

import (
	"context"

	"github.com/trustline/trustline/internal/domain"
)

type Dashboard struct {
	Controls struct {
		Total         int `json:"total"`
		Passing       int `json:"passing"`
		Failing       int `json:"failing"`
		NeedsEvidence int `json:"needs_evidence"`
	} `json:"controls"`
	Tests struct {
		Total   int `json:"total"`
		Passing int `json:"passing"`
		Failing int `json:"failing"`
	} `json:"tests"`
	OpenTasks       int                `json:"open_tasks"`
	OverdueTasks    int                `json:"overdue_tasks"`
	Integrations    int                `json:"integrations"`
	Frameworks      []FrameworkSummary `json:"frameworks"`
	FailingControls []ControlRow       `json:"failing_controls"`
}

func (s *Service) Dashboard(ctx context.Context, orgID domain.ID) (*Dashboard, error) {
	d := &Dashboard{}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status='passing'), count(*) FILTER (WHERE status='failing'), count(*) FILTER (WHERE status='needs_evidence') FROM controls WHERE org_id = $1`, orgID).
		Scan(&d.Controls.Total, &d.Controls.Passing, &d.Controls.Failing, &d.Controls.NeedsEvidence); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE last_status='pass'), count(*) FILTER (WHERE last_status IN ('fail','error')) FROM tests WHERE org_id = $1 AND enabled`, orgID).
		Scan(&d.Tests.Total, &d.Tests.Passing, &d.Tests.Failing); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='open'), count(*) FILTER (WHERE status='open' AND due_at < now()) FROM tasks WHERE org_id = $1`, orgID).
		Scan(&d.OpenTasks, &d.OverdueTasks); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM integrations WHERE org_id = $1`, orgID).Scan(&d.Integrations); err != nil {
		return nil, err
	}
	fws, err := s.ListFrameworks(ctx, orgID)
	if err != nil {
		return nil, err
	}
	d.Frameworks = []FrameworkSummary{}
	for _, f := range fws {
		if f.Adopted {
			d.Frameworks = append(d.Frameworks, f)
		}
	}
	failing, err := s.ListControls(ctx, orgID, ControlFilter{Status: "failing"})
	if err != nil {
		return nil, err
	}
	if len(failing) > 8 {
		failing = failing[:8]
	}
	d.FailingControls = failing
	return d, nil
}
