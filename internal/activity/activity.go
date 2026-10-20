// Package activity writes the immutable, org-scoped activity log. Every
// mutation in the system records an entry in the same transaction.
package activity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

type Entry struct {
	Action      string // e.g. "member.role_changed"
	TargetType  string // e.g. "member"
	TargetID    string
	TargetLabel string
	Metadata    map[string]any
}

// Record appends an entry. q may be a pool or a transaction.
func Record(ctx context.Context, q db.Querier, orgID domain.ID, actor domain.Actor, e Entry) error {
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO activity_log (id, org_id, actor_type, actor_id, actor_label, action, target_type, target_id, target_label, metadata, ip)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		domain.NewID(), orgID, actor.Type, actor.ID, actor.Label, e.Action, e.TargetType, e.TargetID, e.TargetLabel, meta, actor.IP)
	if err != nil {
		return fmt.Errorf("record activity: %w", err)
	}
	return nil
}

type ListFilter struct {
	TargetType string
	TargetID   string
	ActorID    *domain.ID
	Action     string
	Cursor     string
	Limit      int
}

// List returns entries for an org, newest first, with keyset pagination.
func List(ctx context.Context, q db.Querier, orgID domain.ID, f ListFilter) (domain.Page[domain.ActivityEntry], error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	args := []any{orgID}
	where := []string{"org_id = $1"}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.TargetType != "" {
		add("target_type = $%d", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = $%d", f.TargetID)
	}
	if f.ActorID != nil {
		add("actor_id = $%d", *f.ActorID)
	}
	if f.Action != "" {
		add("action LIKE $%d", f.Action+"%")
	}
	if f.Cursor != "" {
		ts, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return domain.Page[domain.ActivityEntry]{}, domain.Invalid("cursor", "invalid cursor")
		}
		args = append(args, ts, id)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, f.Limit+1)
	sql := `SELECT id, org_id, actor_type, actor_id, actor_label, action, target_type, target_id, target_label, metadata, ip, created_at
	        FROM activity_log WHERE ` + strings.Join(where, " AND ") +
		fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return domain.Page[domain.ActivityEntry]{}, err
	}
	defer rows.Close()
	var items []domain.ActivityEntry
	for rows.Next() {
		var e domain.ActivityEntry
		var meta []byte
		if err := rows.Scan(&e.ID, &e.OrgID, &e.ActorType, &e.ActorID, &e.ActorLabel, &e.Action, &e.TargetType, &e.TargetID, &e.TargetLabel, &meta, &e.IP, &e.CreatedAt); err != nil {
			return domain.Page[domain.ActivityEntry]{}, err
		}
		_ = json.Unmarshal(meta, &e.Metadata)
		if e.Metadata == nil {
			e.Metadata = map[string]any{}
		}
		items = append(items, e)
	}
	if items == nil {
		items = []domain.ActivityEntry{}
	}
	page := domain.Page[domain.ActivityEntry]{Items: items}
	if len(items) > f.Limit {
		page.Items = items[:f.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func encodeCursor(ts time.Time, id domain.ID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(ts.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, domain.ID, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, domain.ID{}, err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, domain.ID{}, fmt.Errorf("bad cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, domain.ID{}, err
	}
	id, err := domain.ParseID(parts[1])
	if err != nil {
		return time.Time{}, domain.ID{}, err
	}
	return ts, id, nil
}

// Ensure pgx.Tx satisfies db.Querier at compile time.
var _ db.Querier = pgx.Tx(nil)
