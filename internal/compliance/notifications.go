package compliance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/org"
)

type Notification struct {
	ID        domain.ID  `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// notifyControl fans a control event out to in-app notifications for admins,
// editors and the owner, email (if enabled) and the org's Slack webhook.
func (s *Service) notifyControl(ctx context.Context, tx pgx.Tx, orgID domain.ID, c Transition, kind, title, body string) error {
	link := "/app/controls/" + c.ControlID.String()
	type recipient struct {
		id    domain.ID
		email string
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT u.id, u.email FROM org_memberships m JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1 AND (m.role IN ('admin','editor') OR m.id = $2)`, orgID, c.OwnerID)
	if err != nil {
		return err
	}
	var rs []recipient
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.id, &r.email); err != nil {
			rows.Close()
			return err
		}
		rs = append(rs, r)
	}
	rows.Close()
	o, err := org.New(s.Pool).Get(ctx, tx, orgID)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if _, err := tx.Exec(ctx, `INSERT INTO notifications (id, org_id, user_id, kind, title, body, link) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			domain.NewID(), orgID, r.id, kind, title, body, link); err != nil {
			return err
		}
		if o.Settings.EmailNotifications {
			html, text := mail.Render(mail.TemplateData{Title: title, Body: body, ActionURL: s.BaseURL + link, ActionLabel: "Open control", Footer: "You receive this because you administer " + o.Name + " in Trustline."})
			if err := s.Jobs.EnqueueTx(ctx, tx, jobs.SendEmailArgs{To: r.email, Subject: "[" + o.Name + "] " + title, HTML: html, Text: text}); err != nil {
				return err
			}
		}
	}
	if o.Settings.SlackWebhookURL != "" {
		icon := ":red_circle:"
		if kind != "control_failing" {
			icon = ":large_green_circle:"
		}
		if err := s.Jobs.EnqueueTx(ctx, tx, jobs.SlackNotifyArgs{OrgID: orgID.String(), Text: fmt.Sprintf("%s *%s*\n%s", icon, title, body), Link: s.BaseURL + link}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ListNotifications(ctx context.Context, orgID, userID domain.ID, limit int) ([]Notification, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.Pool.Query(ctx, `SELECT id, kind, title, body, link, read_at, created_at FROM notifications WHERE org_id = $1 AND user_id = $2 ORDER BY created_at DESC LIMIT $3`, orgID, userID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &n.Link, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, n)
	}
	var unread int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE org_id = $1 AND user_id = $2 AND read_at IS NULL`, orgID, userID).Scan(&unread); err != nil {
		return nil, 0, err
	}
	return out, unread, rows.Err()
}

func (s *Service) MarkNotificationsRead(ctx context.Context, orgID, userID domain.ID, ids []domain.ID) error {
	if len(ids) == 0 {
		_, err := s.Pool.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE org_id = $1 AND user_id = $2 AND read_at IS NULL`, orgID, userID)
		return err
	}
	_, err := s.Pool.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE org_id = $1 AND user_id = $2 AND id = ANY($3) AND read_at IS NULL`, orgID, userID, ids)
	return err
}

// SlackWebhook returns the org's webhook URL (worker).
func (s *Service) SlackWebhook(ctx context.Context, orgID domain.ID) (string, error) {
	o, err := org.New(s.Pool).Get(ctx, s.Pool, orgID)
	if err != nil {
		return "", err
	}
	return o.Settings.SlackWebhookURL, nil
}

var _ = db.IsNoRows
