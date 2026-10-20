package compliance

import (
	"context"
	"time"

	"github.com/trustline/trustline/internal/domain"
)

type PendingPolicy struct {
	PolicyID    domain.ID  `json:"policy_id"`
	VersionID   domain.ID  `json:"version_id"`
	Title       string     `json:"title"`
	Version     int        `json:"version"`
	PublishedAt time.Time  `json:"published_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
}

type MyWork struct {
	Policies    []PendingPolicy `json:"policies"`
	Training    *TrainingStatus `json:"training"`
	Devices     []Device        `json:"devices"`
	Tasks       []TaskRow       `json:"tasks"`
	ReviewItems []ReviewItem    `json:"review_items"`
	Pending     int             `json:"pending"`
}

// MyWork is everything the signed-in person needs to do.
func (s *Service) MyWork(ctx context.Context, orgID, userID, membershipID domain.ID, email string) (*MyWork, error) {
	w := &MyWork{Policies: []PendingPolicy{}}
	rows, err := s.Pool.Query(ctx, `SELECT p.id, v.id, p.title, v.version, v.published_at, a.accepted_at
		FROM policies p JOIN policy_versions v ON v.id = p.current_version_id AND v.status = 'published'
		LEFT JOIN policy_acceptances a ON a.policy_version_id = v.id AND a.user_id = $2
		WHERE p.org_id = $1 ORDER BY a.accepted_at IS NOT NULL, p.title`, orgID, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PendingPolicy
		if err := rows.Scan(&p.PolicyID, &p.VersionID, &p.Title, &p.Version, &p.PublishedAt, &p.AcceptedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if p.AcceptedAt == nil {
			w.Pending++
		}
		w.Policies = append(w.Policies, p)
	}
	rows.Close()
	if w.Training, err = s.TrainingFor(ctx, orgID, userID); err != nil && err != domain.ErrNotFound {
		return nil, err
	}
	if w.Training != nil && !w.Training.Completed {
		w.Pending++
	}
	if w.Devices, err = s.ListDevices(ctx, orgID, &email); err != nil {
		return nil, err
	}
	mid := membershipID
	tasks, err := s.ListTasks(ctx, orgID, TaskFilter{AssigneeID: &mid, Status: "open"})
	if err != nil {
		return nil, err
	}
	w.Tasks = []TaskRow{}
	for _, t := range tasks {
		if t.Kind == "policy_acceptance" {
			continue // shown as policies
		}
		w.Tasks = append(w.Tasks, t)
	}
	w.Pending += len(w.Tasks)
	if w.ReviewItems, err = s.ItemsForReviewer(ctx, orgID, membershipID); err != nil {
		return nil, err
	}
	for _, it := range w.ReviewItems {
		if it.Decision == "pending" {
			w.Pending++
		}
	}
	return w, nil
}
