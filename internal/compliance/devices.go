package compliance

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

type Device struct {
	ID                domain.ID `json:"id"`
	Serial            string    `json:"serial"`
	Hostname          string    `json:"hostname"`
	OwnerEmail        string    `json:"owner_email"`
	OwnerName         string    `json:"owner_name"`
	Platform          string    `json:"platform"`
	OSVersion         string    `json:"os_version"`
	DiskEncrypted     bool      `json:"disk_encrypted"`
	ScreenLockMinutes *int      `json:"screen_lock_minutes,omitempty"`
	AntivirusRunning  bool      `json:"antivirus_running"`
	AntivirusName     string    `json:"antivirus_name"`
	AgentVersion      string    `json:"agent_version"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	Compliant         bool      `json:"compliant"`
	Issues            []string  `json:"issues"`
}

func (s *Service) ListDevices(ctx context.Context, orgID domain.ID, owner *string) ([]Device, error) {
	args := []any{orgID}
	filter := ""
	if owner != nil {
		args = append(args, strings.ToLower(*owner))
		filter = " AND lower(d.owner_email) = $2"
	}
	rows, err := s.Pool.Query(ctx, `SELECT d.id, d.serial, d.hostname, d.owner_email, COALESCE(p.name, ''), d.platform, d.os_version, d.disk_encrypted, d.screen_lock_minutes,
		d.antivirus_running, d.antivirus_name, d.agent_version, d.last_seen_at, `+deviceCompliantSQL+`
		FROM devices d LEFT JOIN people p ON p.id = d.person_id AND p.org_id = d.org_id WHERE d.org_id = $1`+filter+` ORDER BY d.owner_email, d.hostname`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Serial, &d.Hostname, &d.OwnerEmail, &d.OwnerName, &d.Platform, &d.OSVersion, &d.DiskEncrypted, &d.ScreenLockMinutes,
			&d.AntivirusRunning, &d.AntivirusName, &d.AgentVersion, &d.LastSeenAt, &d.Compliant); err != nil {
			return nil, err
		}
		d.Issues = []string{}
		if !d.DiskEncrypted {
			d.Issues = append(d.Issues, "Disk not encrypted")
		}
		if d.ScreenLockMinutes == nil || *d.ScreenLockMinutes > 15 {
			d.Issues = append(d.Issues, "Screen lock longer than 15 minutes")
		}
		if !d.AntivirusRunning {
			d.Issues = append(d.Issues, "Anti-malware not running")
		}
		if time.Since(d.LastSeenAt) > 7*24*time.Hour {
			d.Issues = append(d.Issues, "No check-in for over 7 days")
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type DeviceTokenInfo struct {
	Exists     bool       `json:"exists"`
	Hint       string     `json:"hint"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Token      string     `json:"token,omitempty"` // only returned on rotation
}

func (s *Service) DeviceToken(ctx context.Context, orgID domain.ID) (*DeviceTokenInfo, error) {
	var info DeviceTokenInfo
	err := s.Pool.QueryRow(ctx, `SELECT token_hint, created_at, last_used_at FROM device_tokens WHERE org_id = $1`, orgID).Scan(&info.Hint, &info.CreatedAt, &info.LastUsedAt)
	if db.IsNoRows(err) {
		return &info, nil
	}
	if err != nil {
		return nil, err
	}
	info.Exists = true
	return &info, nil
}

// RotateDeviceToken issues a new agent enrollment token (shown once).
func (s *Service) RotateDeviceToken(ctx context.Context, orgID domain.ID, actor domain.Actor) (*DeviceTokenInfo, error) {
	raw, hash, err := auth.NewToken()
	if err != nil {
		return nil, err
	}
	token := "tld_" + raw
	hash = auth.HashToken(token)
	hint := token[:8] + "…" + token[len(token)-4:]
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO device_tokens (org_id, token_hash, token_hint, created_by) VALUES ($1,$2,$3,$4)
			ON CONFLICT (org_id) DO UPDATE SET token_hash = EXCLUDED.token_hash, token_hint = EXCLUDED.token_hint, created_by = EXCLUDED.created_by, created_at = now(), last_used_at = NULL`,
			orgID, hash, hint, actor.ID); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "device_token.rotated", TargetType: "org", TargetID: orgID.String(), TargetLabel: hint})
	})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &DeviceTokenInfo{Exists: true, Hint: hint, CreatedAt: &now, Token: token}, nil
}

// OrgForDeviceToken authenticates an agent.
func (s *Service) OrgForDeviceToken(ctx context.Context, token string) (domain.ID, error) {
	var org domain.ID
	err := s.Pool.QueryRow(ctx, `UPDATE device_tokens SET last_used_at = now() WHERE token_hash = $1 RETURNING org_id`, auth.HashToken(token)).Scan(&org)
	if db.IsNoRows(err) {
		return org, domain.ErrUnauthorized
	}
	return org, err
}

type DeviceReport struct {
	Serial            string `json:"serial"`
	Hostname          string `json:"hostname"`
	OwnerEmail        string `json:"owner_email"`
	Platform          string `json:"platform"`
	OSVersion         string `json:"os_version"`
	DiskEncrypted     *bool  `json:"disk_encrypted"`
	ScreenLockMinutes *int   `json:"screen_lock_minutes"`
	AntivirusRunning  bool   `json:"antivirus_running"`
	AntivirusName     string `json:"antivirus_name"`
	AgentVersion      string `json:"agent_version"`
}

// IngestDeviceReport upserts a device from an agent check-in.
func (s *Service) IngestDeviceReport(ctx context.Context, orgID domain.ID, in DeviceReport) (*Device, error) {
	v := domain.NewValidation()
	in.Serial = strings.TrimSpace(in.Serial)
	in.OwnerEmail = strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	if in.Serial == "" || len(in.Serial) > 128 {
		v.Add("serial", "serial is required")
	}
	if !strings.Contains(in.OwnerEmail, "@") {
		v.Add("owner_email", "owner_email is required")
	}
	if in.DiskEncrypted == nil {
		v.Add("disk_encrypted", "disk_encrypted is required")
	}
	switch in.Platform {
	case "macos", "windows", "linux":
	default:
		v.Add("platform", "platform must be macos, windows or linux")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var id domain.ID
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var person *domain.ID
		var pid domain.ID
		if err := tx.QueryRow(ctx, `SELECT id FROM people WHERE org_id = $1 AND lower(email) = $2`, orgID, in.OwnerEmail).Scan(&pid); err == nil {
			person = &pid
		}
		var inserted bool
		if err := tx.QueryRow(ctx, `INSERT INTO devices (id, org_id, serial, hostname, owner_email, person_id, platform, os_version, disk_encrypted, screen_lock_minutes, antivirus_running, antivirus_name, agent_version, last_seen_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13, now())
			ON CONFLICT (org_id, serial) DO UPDATE SET hostname = EXCLUDED.hostname, owner_email = EXCLUDED.owner_email, person_id = EXCLUDED.person_id, platform = EXCLUDED.platform,
			  os_version = EXCLUDED.os_version, disk_encrypted = EXCLUDED.disk_encrypted, screen_lock_minutes = EXCLUDED.screen_lock_minutes, antivirus_running = EXCLUDED.antivirus_running,
			  antivirus_name = EXCLUDED.antivirus_name, agent_version = EXCLUDED.agent_version, last_seen_at = now()
			RETURNING id, (xmax = 0)`, domain.NewID(), orgID, in.Serial, in.Hostname, in.OwnerEmail, person, in.Platform, in.OSVersion, *in.DiskEncrypted,
			in.ScreenLockMinutes, in.AntivirusRunning, in.AntivirusName, in.AgentVersion).Scan(&id, &inserted); err != nil {
			return err
		}
		if inserted {
			if err := activity.Record(ctx, tx, orgID, domain.Actor{Type: domain.ActorAPIKey, Label: "device-agent"}, activity.Entry{Action: "device.enrolled", TargetType: "device", TargetID: id.String(),
				TargetLabel: in.Hostname + " (" + in.Serial + ")", Metadata: map[string]any{"owner": in.OwnerEmail}}); err != nil {
				return err
			}
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	list, err := s.ListDevices(ctx, orgID, &in.OwnerEmail)
	if err != nil {
		return nil, err
	}
	for _, d := range list {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (s *Service) DeleteDevice(ctx context.Context, orgID, id domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var label string
		if err := tx.QueryRow(ctx, `DELETE FROM devices WHERE org_id = $1 AND id = $2 RETURNING hostname || ' (' || serial || ')'`, orgID, id).Scan(&label); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "device.removed", TargetType: "device", TargetID: id.String(), TargetLabel: label}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}
