package compliance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
)

type Evidence struct {
	ID              domain.ID  `json:"id"`
	ControlID       domain.ID  `json:"control_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Filename        string     `json:"filename"`
	ContentType     string     `json:"content_type"`
	SizeBytes       int64      `json:"size_bytes"`
	SHA256          string     `json:"sha256"`
	CollectedAt     time.Time  `json:"collected_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Expired         bool       `json:"expired"`
	UploadedBy      string     `json:"uploaded_by"`
	AIStatus        string     `json:"ai_status"`
	AIVerdict       *string    `json:"ai_verdict,omitempty"`
	AIConfidence    *float64   `json:"ai_confidence,omitempty"`
	AIReasoning     string     `json:"ai_reasoning"`
	AIMissing       []string   `json:"ai_missing"`
	AIModel         string     `json:"ai_model"`
	OverrideVerdict *string    `json:"override_verdict,omitempty"`
	OverrideReason  string     `json:"override_reason"`
	OverrideBy      string     `json:"override_by"`
	OverrideAt      *time.Time `json:"override_at,omitempty"`
	Verdict         string     `json:"verdict"` // effective: pass | fail | pending
	CreatedAt       time.Time  `json:"created_at"`
}

const evidenceSelect = `SELECT e.id, e.control_id, e.title, e.description, e.filename, e.content_type, e.size_bytes, e.sha256, e.collected_at, e.expires_at,
  COALESCE(u.email, ''), e.ai_status, e.ai_verdict, e.ai_confidence, e.ai_reasoning, e.ai_missing, e.ai_model,
  e.override_verdict, e.override_reason, COALESCE(ou.email, ''), e.override_at, e.created_at, e.blob_key
FROM evidence e LEFT JOIN users u ON u.id = e.uploaded_by LEFT JOIN users ou ON ou.id = e.override_by`

func scanEvidence(row pgx.Row) (*Evidence, string, error) {
	var e Evidence
	var missing []byte
	var key string
	if err := row.Scan(&e.ID, &e.ControlID, &e.Title, &e.Description, &e.Filename, &e.ContentType, &e.SizeBytes, &e.SHA256, &e.CollectedAt, &e.ExpiresAt,
		&e.UploadedBy, &e.AIStatus, &e.AIVerdict, &e.AIConfidence, &e.AIReasoning, &missing, &e.AIModel,
		&e.OverrideVerdict, &e.OverrideReason, &e.OverrideBy, &e.OverrideAt, &e.CreatedAt, &key); err != nil {
		if db.IsNoRows(err) {
			return nil, "", domain.ErrNotFound
		}
		return nil, "", err
	}
	_ = json.Unmarshal(missing, &e.AIMissing)
	if e.AIMissing == nil {
		e.AIMissing = []string{}
	}
	e.Expired = e.ExpiresAt != nil && e.ExpiresAt.Before(time.Now())
	switch {
	case e.OverrideVerdict != nil:
		e.Verdict = *e.OverrideVerdict
	case e.AIVerdict != nil:
		e.Verdict = *e.AIVerdict
	default:
		e.Verdict = "pending"
	}
	return &e, key, nil
}

func (s *Service) ListEvidence(ctx context.Context, orgID, controlID domain.ID) ([]Evidence, error) {
	rows, err := s.Pool.Query(ctx, evidenceSelect+` WHERE e.org_id = $1 AND e.control_id = $2 ORDER BY e.created_at DESC`, orgID, controlID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Evidence{}
	for rows.Next() {
		e, _, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *Service) GetEvidence(ctx context.Context, orgID, id domain.ID) (*Evidence, error) {
	e, _, err := scanEvidence(s.Pool.QueryRow(ctx, evidenceSelect+` WHERE e.org_id = $1 AND e.id = $2`, orgID, id))
	return e, err
}

const MaxEvidenceBytes = 20 << 20

var allowedTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"application/pdf": true, "text/plain": true, "text/csv": true, "application/json": true,
}

type UploadInput struct {
	Title       string
	Description string
	Filename    string
	Data        []byte
	ExpiresAt   *time.Time
}

// UploadEvidence stores the file and queues AI validation.
func (s *Service) UploadEvidence(ctx context.Context, orgID, controlID domain.ID, in UploadInput, actor domain.Actor) (*Evidence, error) {
	v := domain.NewValidation()
	if len(in.Data) == 0 {
		v.Add("file", "choose a file to upload")
	}
	if len(in.Data) > MaxEvidenceBytes {
		v.Add("file", "files must be 20 MB or smaller")
	}
	ctype := http.DetectContentType(in.Data)
	if i := strings.Index(ctype, ";"); i >= 0 {
		ctype = ctype[:i]
	}
	ext := strings.ToLower(filepath.Ext(in.Filename))
	if ctype == "text/plain" {
		switch ext {
		case ".csv":
			ctype = "text/csv"
		case ".json":
			ctype = "application/json"
		}
	}
	if len(in.Data) > 0 && !allowedTypes[ctype] {
		v.Add("file", "unsupported file type "+ctype+"; upload a PDF, image, or text/CSV/JSON file")
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = strings.TrimSuffix(filepath.Base(in.Filename), ext)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var code string
	if err := s.Pool.QueryRow(ctx, `SELECT code FROM controls WHERE org_id = $1 AND id = $2`, orgID, controlID).Scan(&code); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if in.ExpiresAt == nil {
		ttl := 365
		if c := s.Catalog.Control(code); c != nil && c.EvidenceTTLDays > 0 {
			ttl = c.EvidenceTTLDays
		}
		t := time.Now().Add(time.Duration(ttl) * 24 * time.Hour)
		in.ExpiresAt = &t
	}
	sum := sha256.Sum256(in.Data)
	id := domain.NewID()
	key := fmt.Sprintf("orgs/%s/evidence/%s/%s", orgID, id, sanitizeFilename(in.Filename))
	if err := s.Blob.Put(ctx, key, in.Data, ctype); err != nil {
		return nil, fmt.Errorf("store evidence: %w", err)
	}
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO evidence (id, org_id, control_id, title, description, blob_key, filename, content_type, size_bytes, sha256, expires_at, uploaded_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, orgID, controlID, strings.TrimSpace(in.Title), in.Description, key, sanitizeFilename(in.Filename),
			ctype, len(in.Data), hex.EncodeToString(sum[:]), in.ExpiresAt, actor.ID); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "evidence.uploaded", TargetType: "evidence", TargetID: id.String(), TargetLabel: in.Title,
			Metadata: map[string]any{"control": code, "filename": in.Filename, "sha256": hex.EncodeToString(sum[:]), "size": len(in.Data)}}); err != nil {
			return err
		}
		return s.Jobs.EnqueueTx(ctx, tx, jobs.ValidateEvidenceArgs{OrgID: orgID.String(), EvidenceID: id.String()})
	})
	if err != nil {
		_ = s.Blob.Delete(ctx, key)
		return nil, err
	}
	return s.GetEvidence(ctx, orgID, id)
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r == '.' || r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" || out == "." {
		return "file"
	}
	return out
}

// ValidateEvidence runs AI validation (worker). When AI is disabled for the
// org the evidence waits for a human review instead.
func (s *Service) ValidateEvidence(ctx context.Context, svc *ai.Service, orgID, id domain.ID) error {
	e, key, err := scanEvidence(s.Pool.QueryRow(ctx, evidenceSelect+` WHERE e.org_id = $1 AND e.id = $2`, orgID, id))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	var code string
	if err := s.Pool.QueryRow(ctx, `SELECT code FROM controls WHERE org_id = $1 AND id = $2`, orgID, e.ControlID).Scan(&code); err != nil {
		return err
	}
	cat := s.Catalog.Control(code)
	data, err := s.Blob.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("load evidence blob: %w", err)
	}
	in := ai.EvidenceInput{ControlCode: code, Title: e.Title, Description: e.Description, Filename: e.Filename, Today: time.Now().Format("2006-01-02")}
	if cat != nil {
		in.ControlName, in.ControlDescription, in.EvidenceHint, in.Keywords = cat.Name, cat.Description, cat.EvidenceHint, cat.Keywords
	}
	verdict, model, err := svc.ValidateEvidence(ctx, orgID, in, &ai.Attachment{Filename: e.Filename, MediaType: e.ContentType, Data: data})
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		switch {
		case errors.Is(err, ai.ErrDisabled):
			if _, err := tx.Exec(ctx, `UPDATE evidence SET ai_status = 'skipped', ai_reasoning = 'AI validation is disabled for this organization; a reviewer must accept or reject this evidence.' WHERE org_id = $1 AND id = $2`, orgID, id); err != nil {
				return err
			}
		case err != nil:
			if _, uerr := tx.Exec(ctx, `UPDATE evidence SET ai_status = 'error', ai_reasoning = $3 WHERE org_id = $1 AND id = $2`, orgID, id, "AI validation failed: "+err.Error()); uerr != nil {
				return uerr
			}
		default:
			missing, _ := json.Marshal(verdict.Missing)
			if _, err := tx.Exec(ctx, `UPDATE evidence SET ai_status = 'done', ai_verdict = $3, ai_confidence = $4, ai_reasoning = $5, ai_missing = $6, ai_model = $7 WHERE org_id = $1 AND id = $2`,
				orgID, id, verdict.Verdict, verdict.Confidence, verdict.Reasoning, missing, model); err != nil {
				return err
			}
			if err := activity.Record(ctx, tx, orgID, domain.Actor{Type: domain.ActorAgent, Label: "evidence-validator"}, activity.Entry{Action: "evidence.ai_validated", TargetType: "evidence", TargetID: id.String(), TargetLabel: e.Title,
				Metadata: map[string]any{"verdict": verdict.Verdict, "confidence": verdict.Confidence, "model": model}}); err != nil {
				return err
			}
		}
		ts, err := s.recompute(ctx, tx, orgID, []domain.ID{e.ControlID})
		if err != nil {
			return err
		}
		return s.handleTransitions(ctx, tx, orgID, nil, ts)
	})
}

// ReviewEvidence records a human verdict, overriding the AI.
func (s *Service) ReviewEvidence(ctx context.Context, orgID, id domain.ID, verdict, reason string, actor domain.Actor) (*Evidence, error) {
	if verdict != "pass" && verdict != "fail" {
		return nil, domain.Invalid("verdict", "verdict must be pass or fail")
	}
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var control domain.ID
		var title string
		var ai *string
		if err := tx.QueryRow(ctx, `UPDATE evidence SET override_verdict = $3, override_reason = $4, override_by = $5, override_at = now()
			WHERE org_id = $1 AND id = $2 RETURNING control_id, title, ai_verdict`, orgID, id, verdict, strings.TrimSpace(reason), actor.ID).Scan(&control, &title, &ai); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "evidence.reviewed", TargetType: "evidence", TargetID: id.String(), TargetLabel: title,
			Metadata: map[string]any{"verdict": verdict, "ai_verdict": ai, "reason": reason}}); err != nil {
			return err
		}
		ts, err := s.recompute(ctx, tx, orgID, []domain.ID{control})
		if err != nil {
			return err
		}
		return s.handleTransitions(ctx, tx, orgID, nil, ts)
	})
	if err != nil {
		return nil, err
	}
	return s.GetEvidence(ctx, orgID, id)
}

// EvidenceFile returns the stored bytes for download.
func (s *Service) EvidenceFile(ctx context.Context, orgID, id domain.ID) (*Evidence, []byte, error) {
	e, key, err := scanEvidence(s.Pool.QueryRow(ctx, evidenceSelect+` WHERE e.org_id = $1 AND e.id = $2`, orgID, id))
	if err != nil {
		return nil, nil, err
	}
	data, err := s.Blob.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	return e, data, nil
}

func (s *Service) DeleteEvidence(ctx context.Context, orgID, id domain.ID, actor domain.Actor) error {
	var key string
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var control domain.ID
		var title string
		if err := tx.QueryRow(ctx, `DELETE FROM evidence WHERE org_id = $1 AND id = $2 RETURNING control_id, title, blob_key`, orgID, id).Scan(&control, &title, &key); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "evidence.deleted", TargetType: "evidence", TargetID: id.String(), TargetLabel: title}); err != nil {
			return err
		}
		ts, err := s.recompute(ctx, tx, orgID, []domain.ID{control})
		if err != nil {
			return err
		}
		return s.handleTransitions(ctx, tx, orgID, nil, ts)
	})
	if err != nil {
		return err
	}
	return s.Blob.Delete(ctx, key)
}

// ExpireEvidence recomputes controls whose evidence crossed its expiry (daily job).
func (s *Service) ExpireEvidence(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT org_id, control_id FROM evidence WHERE expires_at <= now() AND expires_at > now() - interval '2 days'`)
	if err != nil {
		return err
	}
	byOrg := map[domain.ID][]domain.ID{}
	for rows.Next() {
		var o, c domain.ID
		if err := rows.Scan(&o, &c); err != nil {
			rows.Close()
			return err
		}
		byOrg[o] = append(byOrg[o], c)
	}
	rows.Close()
	for org, ctls := range byOrg {
		if err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
			ts, err := s.recompute(ctx, tx, org, ctls)
			if err != nil {
				return err
			}
			return s.handleTransitions(ctx, tx, org, nil, ts)
		}); err != nil {
			return err
		}
	}
	return nil
}
