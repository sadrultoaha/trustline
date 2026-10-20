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
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/ai"
	"github.com/trustline/trustline/internal/auth"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/mail"
	"github.com/trustline/trustline/internal/org"
)

type Vendor struct {
	ID              domain.ID  `json:"id"`
	Name            string     `json:"name"`
	Domain          string     `json:"domain"`
	Description     string     `json:"description"`
	Category        string     `json:"category"`
	Status          string     `json:"status"`
	Tier            string     `json:"tier"`
	DataAccess      string     `json:"data_access"`
	HandlesPII      bool       `json:"handles_pii"`
	Owner           *Owner     `json:"owner,omitempty"`
	Source          string     `json:"source"`
	DiscoveryRef    string     `json:"discovery_ref"`
	DiscoveryUsers  int        `json:"discovery_users"`
	DiscoveryNote   string     `json:"discovery_note"`
	RecommendedTier string     `json:"recommended_tier"`
	RiskSummary     string     `json:"risk_summary"`
	ReviewCycleDays int        `json:"review_cycle_days"`
	NextReviewAt    *time.Time `json:"next_review_at,omitempty"`
	LastReviewedAt  *time.Time `json:"last_reviewed_at,omitempty"`
	DiscoveredAt    *time.Time `json:"discovered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ReviewOverdue   bool       `json:"review_overdue"`
	HasReport       bool       `json:"has_security_report"`
	HasDPA          bool       `json:"has_dpa"`
	Documents       int        `json:"documents"`
}

const vendorSelect = `SELECT v.id, v.name, v.domain, v.description, v.category, v.status, v.tier, v.data_access, v.handles_pii, m.id, u.name, u.email,
  v.source, v.discovery_ref, v.discovery_users, v.discovery_note, v.recommended_tier, v.risk_summary, v.review_cycle_days, v.next_review_at, v.last_reviewed_at,
  v.discovered_at, v.created_at,
  EXISTS(SELECT 1 FROM vendor_documents d WHERE d.vendor_id = v.id AND d.kind IN ('soc2_type2','soc2_type1','iso27001') AND (d.expires_at IS NULL OR d.expires_at > now())),
  EXISTS(SELECT 1 FROM vendor_documents d WHERE d.vendor_id = v.id AND d.kind IN ('dpa','baa')),
  (SELECT count(*) FROM vendor_documents d WHERE d.vendor_id = v.id)
FROM vendors v
LEFT JOIN org_memberships m ON m.id = v.owner_membership_id AND m.org_id = v.org_id
LEFT JOIN users u ON u.id = m.user_id`

func scanVendor(row pgx.Row) (*Vendor, error) {
	var v Vendor
	var mid *domain.ID
	var mname, memail *string
	if err := row.Scan(&v.ID, &v.Name, &v.Domain, &v.Description, &v.Category, &v.Status, &v.Tier, &v.DataAccess, &v.HandlesPII, &mid, &mname, &memail,
		&v.Source, &v.DiscoveryRef, &v.DiscoveryUsers, &v.DiscoveryNote, &v.RecommendedTier, &v.RiskSummary, &v.ReviewCycleDays, &v.NextReviewAt, &v.LastReviewedAt,
		&v.DiscoveredAt, &v.CreatedAt, &v.HasReport, &v.HasDPA, &v.Documents); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if mid != nil {
		v.Owner = &Owner{MembershipID: *mid, Name: deref(mname), Email: deref(memail)}
	}
	v.ReviewOverdue = v.Status == "active" && (v.NextReviewAt == nil || v.NextReviewAt.Before(time.Now()))
	return &v, nil
}

func (s *Service) ListVendors(ctx context.Context, orgID domain.ID, status string) ([]Vendor, error) {
	args := []any{orgID}
	where := " WHERE v.org_id = $1"
	if status != "" {
		args = append(args, status)
		where += " AND v.status = $2"
	} else {
		where += " AND v.status <> 'dismissed'"
	}
	rows, err := s.Pool.Query(ctx, vendorSelect+where+` ORDER BY CASE v.tier WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, lower(v.name)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vendor{}
	for rows.Next() {
		v, err := scanVendor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

type VendorDocument struct {
	ID          domain.ID  `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	SHA256      string     `json:"sha256"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	UploadedBy  string     `json:"uploaded_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

type VendorQuestionnaire struct {
	ID             domain.ID         `json:"id"`
	TemplateID     string            `json:"template_id"`
	RecipientEmail string            `json:"recipient_email"`
	Status         string            `json:"status"`
	Answers        map[string]string `json:"answers"`
	SentAt         time.Time         `json:"sent_at"`
	SubmittedAt    *time.Time        `json:"submitted_at,omitempty"`
	ExpiresAt      time.Time         `json:"expires_at"`
	Link           string            `json:"link,omitempty"` // only right after sending
}

type VendorAssessmentRow struct {
	ID              domain.ID            `json:"id"`
	DocumentID      *domain.ID           `json:"document_id,omitempty"`
	QuestionnaireID *domain.ID           `json:"questionnaire_id,omitempty"`
	Status          string               `json:"status"`
	Result          *ai.VendorAssessment `json:"result,omitempty"`
	Model           string               `json:"model"`
	Error           string               `json:"error"`
	CreatedAt       time.Time            `json:"created_at"`
	CompletedAt     *time.Time           `json:"completed_at,omitempty"`
}

type VendorDetail struct {
	Vendor
	DocumentsList  []VendorDocument               `json:"documents_list"`
	Questionnaires []VendorQuestionnaire          `json:"questionnaires"`
	Assessments    []VendorAssessmentRow          `json:"assessments"`
	Template       *catalog.QuestionnaireTemplate `json:"questionnaire_template,omitempty"`
}

func (s *Service) GetVendor(ctx context.Context, orgID, id domain.ID) (*VendorDetail, error) {
	v, err := scanVendor(s.Pool.QueryRow(ctx, vendorSelect+" WHERE v.org_id = $1 AND v.id = $2", orgID, id))
	if err != nil {
		return nil, err
	}
	d := &VendorDetail{Vendor: *v, DocumentsList: []VendorDocument{}, Questionnaires: []VendorQuestionnaire{}, Assessments: []VendorAssessmentRow{}}
	if len(s.Catalog.VendorQuestionnaires) > 0 {
		d.Template = &s.Catalog.VendorQuestionnaires[0]
	}
	rows, err := s.Pool.Query(ctx, `SELECT d.id, d.kind, d.title, d.filename, d.content_type, d.size_bytes, d.sha256, d.expires_at, COALESCE(u.email,''), d.created_at
		FROM vendor_documents d LEFT JOIN users u ON u.id = d.uploaded_by WHERE d.org_id = $1 AND d.vendor_id = $2 ORDER BY d.created_at DESC`, orgID, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var x VendorDocument
		if err := rows.Scan(&x.ID, &x.Kind, &x.Title, &x.Filename, &x.ContentType, &x.SizeBytes, &x.SHA256, &x.ExpiresAt, &x.UploadedBy, &x.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		d.DocumentsList = append(d.DocumentsList, x)
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT id, template_id, recipient_email, status, answers, sent_at, submitted_at, expires_at FROM vendor_questionnaires WHERE org_id = $1 AND vendor_id = $2 ORDER BY sent_at DESC`, orgID, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var q VendorQuestionnaire
		var raw []byte
		if err := rows.Scan(&q.ID, &q.TemplateID, &q.RecipientEmail, &q.Status, &raw, &q.SentAt, &q.SubmittedAt, &q.ExpiresAt); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(raw, &q.Answers)
		if q.Status != "submitted" && q.ExpiresAt.Before(time.Now()) {
			q.Status = "expired"
		}
		d.Questionnaires = append(d.Questionnaires, q)
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT id, document_id, questionnaire_id, status, result, model, error, created_at, completed_at FROM vendor_assessments WHERE org_id = $1 AND vendor_id = $2 ORDER BY created_at DESC`, orgID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a VendorAssessmentRow
		var raw []byte
		if err := rows.Scan(&a.ID, &a.DocumentID, &a.QuestionnaireID, &a.Status, &raw, &a.Model, &a.Error, &a.CreatedAt, &a.CompletedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			var r ai.VendorAssessment
			if json.Unmarshal(raw, &r) == nil {
				a.Result = &r
			}
		}
		d.Assessments = append(d.Assessments, a)
	}
	return d, rows.Err()
}

type VendorInput struct {
	Name              *string `json:"name"`
	Domain            *string `json:"domain"`
	Description       *string `json:"description"`
	Category          *string `json:"category"`
	Tier              *string `json:"tier"`
	DataAccess        *string `json:"data_access"`
	HandlesPII        *bool   `json:"handles_pii"`
	OwnerMembershipID *string `json:"owner_membership_id"`
	Status            *string `json:"status"`
}

func (in VendorInput) validate(creating bool) error {
	v := domain.NewValidation()
	if (creating && in.Name == nil) || (in.Name != nil && strings.TrimSpace(*in.Name) == "") {
		v.Add("name", "name is required")
	}
	if in.Tier != nil && !oneOf(*in.Tier, "critical", "high", "medium", "low", "unassessed") {
		v.Add("tier", "tier must be critical, high, medium, low or unassessed")
	}
	if in.DataAccess != nil && !oneOf(*in.DataAccess, "none", "internal", "confidential", "restricted") {
		v.Add("data_access", "data access must be none, internal, confidential or restricted")
	}
	if in.Status != nil && !oneOf(*in.Status, "active", "offboarded") {
		v.Add("status", "status must be active or offboarded")
	}
	return v.Err()
}

func oneOf(v string, opts ...string) bool {
	for _, o := range opts {
		if v == o {
			return true
		}
	}
	return false
}

func reviewCycleFor(tier string) int {
	switch tier {
	case "critical", "high":
		return 365
	case "medium":
		return 730
	}
	return 1095
}

func (s *Service) CreateVendor(ctx context.Context, orgID domain.ID, in VendorInput, actor domain.Actor) (*VendorDetail, error) {
	if err := in.validate(true); err != nil {
		return nil, err
	}
	owner, _, err := parseOwner(in.OwnerMembershipID)
	if err != nil {
		return nil, err
	}
	tier := orDefault(in.Tier, "unassessed")
	id := domain.NewID()
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		pii := in.HandlesPII != nil && *in.HandlesPII
		if _, err := tx.Exec(ctx, `INSERT INTO vendors (id, org_id, name, domain, description, category, status, tier, data_access, handles_pii, owner_membership_id, review_cycle_days)
			VALUES ($1,$2,$3,$4,$5,$6,'active',$7,$8,$9,$10,$11)`, id, orgID, strings.TrimSpace(*in.Name), strings.ToLower(deref(in.Domain)), deref(in.Description), deref(in.Category),
			tier, orDefault(in.DataAccess, "none"), pii, owner, reviewCycleFor(tier)); err != nil {
			if db.IsUniqueViolation(err) {
				return domain.Invalid("name", "a vendor with this name already exists")
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.created", TargetType: "vendor", TargetID: id.String(), TargetLabel: *in.Name}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetVendor(ctx, orgID, id)
}

func (s *Service) UpdateVendor(ctx context.Context, orgID, id domain.ID, in VendorInput, actor domain.Actor) (*VendorDetail, error) {
	if err := in.validate(false); err != nil {
		return nil, err
	}
	owner, setOwner, err := parseOwner(in.OwnerMembershipID)
	if err != nil {
		return nil, err
	}
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		cur, err := scanVendor(tx.QueryRow(ctx, vendorSelect+" WHERE v.org_id = $1 AND v.id = $2 FOR UPDATE OF v", orgID, id))
		if err != nil {
			return err
		}
		changes := map[string]any{}
		upd := func(col string, v any) error {
			changes[col] = v
			_, err := tx.Exec(ctx, `UPDATE vendors SET `+col+` = $3, updated_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, v)
			return err
		}
		steps := []struct {
			ok  bool
			col string
			val any
		}{
			{in.Name != nil, "name", strings.TrimSpace(deref(in.Name))},
			{in.Domain != nil, "domain", strings.ToLower(deref(in.Domain))},
			{in.Description != nil, "description", deref(in.Description)},
			{in.Category != nil, "category", deref(in.Category)},
			{in.DataAccess != nil, "data_access", deref(in.DataAccess)},
			{in.HandlesPII != nil, "handles_pii", in.HandlesPII != nil && *in.HandlesPII},
			{setOwner, "owner_membership_id", owner},
			{in.Status != nil, "status", deref(in.Status)},
		}
		for _, st := range steps {
			if st.ok {
				if err := upd(st.col, st.val); err != nil {
					if db.IsUniqueViolation(err) {
						return domain.Invalid("name", "a vendor with this name already exists")
					}
					return err
				}
			}
		}
		if in.Tier != nil && *in.Tier != cur.Tier {
			if err := upd("tier", *in.Tier); err != nil {
				return err
			}
			if err := upd("review_cycle_days", reviewCycleFor(*in.Tier)); err != nil {
				return err
			}
			if cur.LastReviewedAt != nil {
				if err := upd("next_review_at", cur.LastReviewedAt.Add(time.Duration(reviewCycleFor(*in.Tier))*24*time.Hour)); err != nil {
					return err
				}
			}
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.updated", TargetType: "vendor", TargetID: id.String(), TargetLabel: cur.Name, Metadata: changes}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetVendor(ctx, orgID, id)
}

// TriageVendor adds a discovered vendor to the inventory or dismisses it.
func (s *Service) TriageVendor(ctx context.Context, orgID, id domain.ID, add bool, actor domain.Actor) (*VendorDetail, error) {
	status, action := "dismissed", "vendor.dismissed"
	if add {
		status, action = "active", "vendor.added_from_discovery"
	}
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `UPDATE vendors SET status = $3, updated_at = now() WHERE org_id = $1 AND id = $2 AND status = 'discovered' RETURNING name`, orgID, id, status).Scan(&name); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("status", "only discovered vendors can be triaged")
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: action, TargetType: "vendor", TargetID: id.String(), TargetLabel: name}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetVendor(ctx, orgID, id)
}

// RestoreVendor undoes a dismissal: the vendor returns to the discovered queue.
func (s *Service) RestoreVendor(ctx context.Context, orgID, id domain.ID, actor domain.Actor) (*VendorDetail, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `UPDATE vendors SET status = CASE WHEN discovered_at IS NULL THEN 'active' ELSE 'discovered' END, updated_at = now()
			WHERE org_id = $1 AND id = $2 AND status = 'dismissed' RETURNING name`, orgID, id).Scan(&name); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("status", "only dismissed vendors can be restored")
			}
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.restored", TargetType: "vendor", TargetID: id.String(), TargetLabel: name}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetVendor(ctx, orgID, id)
}

// knownApps enriches discovered applications.
var knownApps = map[string][2]string{
	"slack": {"slack.com", "Communication"}, "zoom": {"zoom.us", "Communication"}, "notion": {"notion.so", "Productivity"},
	"salesforce": {"salesforce.com", "CRM"}, "github enterprise": {"github.com", "Engineering"}, "aws iam identity center": {"aws.amazon.com", "Cloud infrastructure"},
	"google workspace": {"google.com", "Productivity"}, "pdf magic converter": {"pdfmagic.example", "Productivity"},
}

// discoverVendors records applications seen in SSO and OAuth grants (inside the sync transaction).
func (s *Service) discoverVendors(ctx context.Context, tx pgx.Tx, orgID domain.ID, connector string, res map[string][]map[string]any) error {
	type app struct {
		name, ref, note string
		users           int
	}
	var apps []app
	switch connector {
	case "google_workspace":
		for _, g := range res["oauth_grant"] {
			note := fmt.Sprintf("OAuth app with scopes: %s", strings.Join(asList(g["scopes"]), ", "))
			if g["high_risk_scopes"] == true {
				note += " (high-risk scopes)"
			}
			if g["verified"] == false {
				note += "; publisher not verified by Google"
			}
			users := 0
			if f, ok := g["users"].(float64); ok {
				users = int(f)
			}
			apps = append(apps, app{str(g["app_name"]), str(g["id"]), note, users})
		}
	case "okta":
		for _, a := range res["app"] {
			users := 0
			if f, ok := a["assigned_users"].(float64); ok {
				users = int(f)
			}
			apps = append(apps, app{str(a["id"]), str(a["id"]), "SSO application (" + str(a["sign_on_mode"]) + ")", users})
		}
	default:
		return nil
	}
	created := 0
	for _, a := range apps {
		if a.name == "" || strings.EqualFold(a.name, "Trustline") {
			continue
		}
		meta := knownApps[strings.ToLower(a.name)]
		tag, err := tx.Exec(ctx, `INSERT INTO vendors (id, org_id, name, domain, category, status, source, discovery_ref, discovery_users, discovery_note, discovered_at)
			VALUES ($1,$2,$3,$4,$5,'discovered',$6,$7,$8,$9, now())
			ON CONFLICT (org_id, lower(name)) DO UPDATE SET discovery_users = EXCLUDED.discovery_users, discovery_note = EXCLUDED.discovery_note, updated_at = now()
			WHERE vendors.source = EXCLUDED.source`,
			domain.NewID(), orgID, a.name, meta[0], meta[1], connector, a.ref, a.users, a.note)
		if err != nil {
			return err
		}
		created += int(tag.RowsAffected())
	}
	if created > 0 {
		return activity.Record(ctx, tx, orgID, domain.SystemActor(connector+"-sync"), activity.Entry{Action: "vendor.discovered", TargetType: "vendor", TargetLabel: systemNames[connector],
			Metadata: map[string]any{"applications": len(apps)}})
	}
	return nil
}

var documentKinds = map[string]bool{"soc2_type2": true, "soc2_type1": true, "iso27001": true, "pentest": true, "dpa": true, "baa": true, "insurance": true, "other": true}

type VendorDocInput struct {
	Kind      string
	Title     string
	Filename  string
	Data      []byte
	ExpiresAt *time.Time
}

// UploadVendorDocument stores a document; security reports are assessed automatically.
func (s *Service) UploadVendorDocument(ctx context.Context, orgID, vendorID domain.ID, in VendorDocInput, actor domain.Actor) (*VendorDetail, error) {
	v := domain.NewValidation()
	if !documentKinds[in.Kind] {
		v.Add("kind", "choose a document type")
	}
	if len(in.Data) == 0 || len(in.Data) > MaxEvidenceBytes {
		v.Add("file", "choose a file up to 20 MB")
	}
	ctype := http.DetectContentType(in.Data)
	if i := strings.Index(ctype, ";"); i >= 0 {
		ctype = ctype[:i]
	}
	if len(in.Data) > 0 && !allowedTypes[ctype] {
		v.Add("file", "unsupported file type "+ctype)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var name string
	if err := s.Pool.QueryRow(ctx, `SELECT name FROM vendors WHERE org_id = $1 AND id = $2`, orgID, vendorID).Scan(&name); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = strings.TrimSuffix(filepath.Base(in.Filename), filepath.Ext(in.Filename))
	}
	if in.ExpiresAt == nil && (in.Kind == "soc2_type2" || in.Kind == "soc2_type1" || in.Kind == "iso27001" || in.Kind == "pentest") {
		t := time.Now().Add(365 * 24 * time.Hour)
		in.ExpiresAt = &t
	}
	sum := sha256.Sum256(in.Data)
	docID := domain.NewID()
	key := fmt.Sprintf("orgs/%s/vendors/%s/%s/%s", orgID, vendorID, docID, sanitizeFilename(in.Filename))
	if err := s.Blob.Put(ctx, key, in.Data, ctype); err != nil {
		return nil, err
	}
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO vendor_documents (id, org_id, vendor_id, kind, title, blob_key, filename, content_type, size_bytes, sha256, expires_at, uploaded_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, docID, orgID, vendorID, in.Kind, in.Title, key, sanitizeFilename(in.Filename), ctype, len(in.Data), hex.EncodeToString(sum[:]), in.ExpiresAt, actor.ID); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.document_uploaded", TargetType: "vendor", TargetID: vendorID.String(), TargetLabel: name,
			Metadata: map[string]any{"kind": in.Kind, "filename": in.Filename, "sha256": hex.EncodeToString(sum[:])}}); err != nil {
			return err
		}
		if in.Kind == "soc2_type2" || in.Kind == "soc2_type1" || in.Kind == "iso27001" || in.Kind == "pentest" {
			if err := s.queueAssessment(ctx, tx, orgID, vendorID, &docID, nil, actor); err != nil {
				return err
			}
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		_ = s.Blob.Delete(ctx, key)
		return nil, err
	}
	return s.GetVendor(ctx, orgID, vendorID)
}

func (s *Service) queueAssessment(ctx context.Context, tx pgx.Tx, orgID, vendorID domain.ID, docID, questionnaireID *domain.ID, actor domain.Actor) error {
	id := domain.NewID()
	if _, err := tx.Exec(ctx, `INSERT INTO vendor_assessments (id, org_id, vendor_id, document_id, questionnaire_id, requested_by) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, orgID, vendorID, docID, questionnaireID, actor.ID); err != nil {
		return err
	}
	return s.Jobs.EnqueueTx(ctx, tx, jobs.VendorAssessmentArgs{OrgID: orgID.String(), AssessmentID: id.String()})
}

// RequestAssessment re-runs the AI review of a document.
func (s *Service) RequestAssessment(ctx context.Context, orgID, vendorID, docID domain.ID, actor domain.Actor) error {
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_documents WHERE org_id = $1 AND vendor_id = $2 AND id = $3)`, orgID, vendorID, docID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return domain.ErrNotFound
		}
		return s.queueAssessment(ctx, tx, orgID, vendorID, &docID, nil, actor)
	})
}

// RunVendorAssessment performs a queued assessment (worker).
func (s *Service) RunVendorAssessment(ctx context.Context, svc *ai.Service, orgID, id domain.ID) error {
	var vendorID domain.ID
	var docID, qID *domain.ID
	if err := s.Pool.QueryRow(ctx, `SELECT vendor_id, document_id, questionnaire_id FROM vendor_assessments WHERE org_id = $1 AND id = $2 AND status = 'pending'`, orgID, id).Scan(&vendorID, &docID, &qID); err != nil {
		if db.IsNoRows(err) {
			return nil
		}
		return err
	}
	v, err := s.GetVendor(ctx, orgID, vendorID)
	if err != nil {
		return err
	}
	in := ai.VendorAssessmentInput{VendorName: v.Name, DataAccess: v.DataAccess, HandlesPII: v.HandlesPII}
	var att *ai.Attachment
	if docID != nil {
		var key, ctype, fname, kind string
		if err := s.Pool.QueryRow(ctx, `SELECT blob_key, content_type, filename, kind FROM vendor_documents WHERE org_id = $1 AND id = $2`, orgID, *docID).Scan(&key, &ctype, &fname, &kind); err != nil {
			return err
		}
		data, err := s.Blob.Get(ctx, key)
		if err != nil {
			return err
		}
		in.DocumentKind = kind
		att = &ai.Attachment{Filename: fname, MediaType: ctype, Data: data}
	} else if qID != nil {
		var raw []byte
		if err := s.Pool.QueryRow(ctx, `SELECT answers FROM vendor_questionnaires WHERE org_id = $1 AND id = $2`, orgID, *qID).Scan(&raw); err != nil {
			return err
		}
		var answers map[string]string
		_ = json.Unmarshal(raw, &answers)
		in.DocumentKind = "questionnaire"
		in.Answers = s.answersText(answers)
	}
	res, model, err := svc.AssessVendor(ctx, orgID, in, att)
	return db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		switch {
		case errors.Is(err, ai.ErrDisabled):
			_, err = tx.Exec(ctx, `UPDATE vendor_assessments SET status = 'skipped', error = 'AI is disabled for this organization; review the document manually.', completed_at = now() WHERE org_id = $1 AND id = $2`, orgID, id)
			return err
		case err != nil:
			if _, uerr := tx.Exec(ctx, `UPDATE vendor_assessments SET status = 'error', error = $3, completed_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, err.Error()); uerr != nil {
				return uerr
			}
			return nil
		}
		raw, _ := json.Marshal(res)
		if _, err := tx.Exec(ctx, `UPDATE vendor_assessments SET status = 'done', result = $3, model = $4, completed_at = now() WHERE org_id = $1 AND id = $2`, orgID, id, raw, model); err != nil {
			return err
		}
		cycle := v.ReviewCycleDays
		if _, err := tx.Exec(ctx, `UPDATE vendors SET recommended_tier = $3, risk_summary = $4, last_reviewed_at = now(), next_review_at = now() + make_interval(days => $5), updated_at = now()
			WHERE org_id = $1 AND id = $2`, orgID, vendorID, res.RecommendedTier, res.RiskSummary, cycle); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'done', resolution = 'Vendor re-assessed.', completed_at = now(), updated_at = now()
			WHERE org_id = $1 AND kind = 'vendor_review' AND status = 'open' AND meta->>'vendor_id' = $2`, orgID, vendorID.String()); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, domain.Actor{Type: domain.ActorAgent, Label: "vendor-assessor"}, activity.Entry{Action: "vendor.assessed", TargetType: "vendor", TargetID: vendorID.String(),
			TargetLabel: v.Name, Metadata: map[string]any{"opinion": res.Opinion, "exceptions": len(res.Exceptions), "recommended_tier": res.RecommendedTier, "model": model}}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
}

func (s *Service) answersText(answers map[string]string) string {
	if len(s.Catalog.VendorQuestionnaires) == 0 {
		return ""
	}
	var b strings.Builder
	for _, q := range s.Catalog.VendorQuestionnaires[0].Questions {
		fmt.Fprintf(&b, "%s: %s\n", q.Question, answers[q.ID])
	}
	return b.String()
}

// ApplyRecommendedTier adopts the AI's recommendation.
func (s *Service) ApplyRecommendedTier(ctx context.Context, orgID, id domain.ID, actor domain.Actor) (*VendorDetail, error) {
	var tier string
	if err := s.Pool.QueryRow(ctx, `SELECT recommended_tier FROM vendors WHERE org_id = $1 AND id = $2`, orgID, id).Scan(&tier); err != nil {
		if db.IsNoRows(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if tier == "" {
		return nil, domain.Invalid("tier", "no recommendation yet; assess the vendor first")
	}
	return s.UpdateVendor(ctx, orgID, id, VendorInput{Tier: &tier}, actor)
}

func (s *Service) MarkVendorReviewed(ctx context.Context, orgID, id domain.ID, actor domain.Actor) (*VendorDetail, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `UPDATE vendors SET last_reviewed_at = now(), next_review_at = now() + make_interval(days => review_cycle_days), updated_at = now()
			WHERE org_id = $1 AND id = $2 AND status = 'active' RETURNING name`, orgID, id).Scan(&name); err != nil {
			if db.IsNoRows(err) {
				return domain.Invalid("status", "only active vendors can be reviewed")
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'done', resolution = 'Vendor reviewed.', completed_at = now(), updated_at = now()
			WHERE org_id = $1 AND kind = 'vendor_review' AND status = 'open' AND meta->>'vendor_id' = $2`, orgID, id.String()); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.reviewed", TargetType: "vendor", TargetID: id.String(), TargetLabel: name}); err != nil {
			return err
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetVendor(ctx, orgID, id)
}

func (s *Service) VendorDocumentFile(ctx context.Context, orgID, vendorID, docID domain.ID) (*VendorDocument, []byte, error) {
	var d VendorDocument
	var key string
	if err := s.Pool.QueryRow(ctx, `SELECT id, kind, title, filename, content_type, size_bytes, sha256, blob_key FROM vendor_documents WHERE org_id = $1 AND vendor_id = $2 AND id = $3`,
		orgID, vendorID, docID).Scan(&d.ID, &d.Kind, &d.Title, &d.Filename, &d.ContentType, &d.SizeBytes, &d.SHA256, &key); err != nil {
		if db.IsNoRows(err) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}
	data, err := s.Blob.Get(ctx, key)
	return &d, data, err
}

// --- questionnaires & vendor portal -----------------------------------------

func (s *Service) SendQuestionnaire(ctx context.Context, orgID, vendorID domain.ID, email string, actor domain.Actor) (*VendorQuestionnaire, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return nil, domain.Invalid("recipient_email", "a valid email is required")
	}
	if len(s.Catalog.VendorQuestionnaires) == 0 {
		return nil, domain.Invalid("template", "no questionnaire template available")
	}
	tpl := s.Catalog.VendorQuestionnaires[0]
	token, hash, err := auth.NewToken()
	if err != nil {
		return nil, err
	}
	id := domain.NewID()
	link := s.BaseURL + "/vendor-portal/" + token
	err = db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM vendors WHERE org_id = $1 AND id = $2`, orgID, vendorID).Scan(&name); err != nil {
			if db.IsNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		o, err := org.New(s.Pool).Get(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO vendor_questionnaires (id, org_id, vendor_id, template_id, token_hash, recipient_email, sent_by, expires_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7, now() + interval '30 days')`, id, orgID, vendorID, tpl.ID, hash, email, actor.ID); err != nil {
			return err
		}
		html, text := mail.Render(mail.TemplateData{Title: o.Name + " requests a security review",
			Body:      fmt.Sprintf("%s uses %s and asks you to complete a %d-question security questionnaire. No account is needed; you can save and come back. The link expires in 30 days.", o.Name, name, len(tpl.Questions)),
			ActionURL: link, ActionLabel: "Open questionnaire", Footer: "Sent via Trustline on behalf of " + o.Name + "."})
		if err := s.Jobs.EnqueueTx(ctx, tx, jobs.SendEmailArgs{To: email, Subject: "Security questionnaire from " + o.Name, HTML: html, Text: text}); err != nil {
			return err
		}
		return activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.questionnaire_sent", TargetType: "vendor", TargetID: vendorID.String(), TargetLabel: name, Metadata: map[string]any{"to": email}})
	})
	if err != nil {
		return nil, err
	}
	return &VendorQuestionnaire{ID: id, TemplateID: tpl.ID, RecipientEmail: email, Status: "sent", Answers: map[string]string{}, SentAt: time.Now(), ExpiresAt: time.Now().Add(30 * 24 * time.Hour), Link: link}, nil
}

type PortalView struct {
	OrgName    string                        `json:"org_name"`
	VendorName string                        `json:"vendor_name"`
	Status     string                        `json:"status"`
	ExpiresAt  time.Time                     `json:"expires_at"`
	Template   catalog.QuestionnaireTemplate `json:"template"`
	Answers    map[string]string             `json:"answers"`
}

func (s *Service) portalLookup(ctx context.Context, q db.Querier, token string, lock bool) (*PortalView, domain.ID, domain.ID, domain.ID, error) {
	var pv PortalView
	var qid, orgID, vendorID domain.ID
	var raw []byte
	var tplID string
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF q"
	}
	err := q.QueryRow(ctx, `SELECT q.id, q.org_id, q.vendor_id, o.name, v.name, q.status, q.expires_at, q.template_id, q.answers
		FROM vendor_questionnaires q JOIN orgs o ON o.id = q.org_id JOIN vendors v ON v.id = q.vendor_id WHERE q.token_hash = $1`+suffix, auth.HashToken(token)).
		Scan(&qid, &orgID, &vendorID, &pv.OrgName, &pv.VendorName, &pv.Status, &pv.ExpiresAt, &tplID, &raw)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, qid, orgID, vendorID, domain.ErrNotFound
		}
		return nil, qid, orgID, vendorID, err
	}
	if pv.Status != "submitted" && pv.ExpiresAt.Before(time.Now()) {
		pv.Status = "expired"
	}
	for _, t := range s.Catalog.VendorQuestionnaires {
		if t.ID == tplID {
			pv.Template = t
		}
	}
	_ = json.Unmarshal(raw, &pv.Answers)
	if pv.Answers == nil {
		pv.Answers = map[string]string{}
	}
	return &pv, qid, orgID, vendorID, nil
}

func (s *Service) PortalGet(ctx context.Context, token string) (*PortalView, error) {
	pv, _, _, _, err := s.portalLookup(ctx, s.Pool, token, false)
	return pv, err
}

// PortalSave stores answers from the vendor; submit=true finalizes and triggers assessment.
func (s *Service) PortalSave(ctx context.Context, token string, answers map[string]string, submit bool) (*PortalView, error) {
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		pv, qid, orgID, vendorID, err := s.portalLookup(ctx, tx, token, true)
		if err != nil {
			return err
		}
		if pv.Status == "submitted" || pv.Status == "expired" {
			return domain.Invalid("status", "this questionnaire is "+pv.Status)
		}
		valid := map[string]catalog.QuestionnaireQuestion{}
		for _, q := range pv.Template.Questions {
			valid[q.ID] = q
		}
		clean := map[string]string{}
		for k, v := range answers {
			q, ok := valid[k]
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			if len(v) > 4000 {
				v = v[:4000]
			}
			if q.Type == "yes_no" && v != "" && v != "Yes" && v != "No" {
				return domain.Invalid(k, "answer Yes or No")
			}
			clean[k] = v
		}
		if submit {
			var missing []string
			for _, q := range pv.Template.Questions {
				if clean[q.ID] == "" {
					missing = append(missing, q.ID)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				return domain.Invalid("answers", fmt.Sprintf("%d question(s) still need an answer", len(missing)))
			}
		}
		raw, _ := json.Marshal(clean)
		status := "in_progress"
		if submit {
			status = "submitted"
		}
		if _, err := tx.Exec(ctx, `UPDATE vendor_questionnaires SET answers = $2, status = $3, submitted_at = CASE WHEN $3 = 'submitted' THEN now() ELSE submitted_at END WHERE id = $1`, qid, raw, status); err != nil {
			return err
		}
		if !submit {
			return nil
		}
		actor := domain.Actor{Type: domain.ActorSystem, Label: "vendor-portal"}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "vendor.questionnaire_submitted", TargetType: "vendor", TargetID: vendorID.String(), TargetLabel: pv.VendorName}); err != nil {
			return err
		}
		if err := s.notifyAdmins(ctx, tx, orgID, "vendor_questionnaire", pv.VendorName+" submitted the security questionnaire", "AI review of the answers has started.", "/app/vendors/"+vendorID.String()); err != nil {
			return err
		}
		return s.queueAssessment(ctx, tx, orgID, vendorID, nil, &qid, actor)
	})
	if err != nil {
		return nil, err
	}
	return s.PortalGet(ctx, token)
}

// VendorReviewReminders opens review tasks for vendors due within 30 days (daily job).
func (s *Service) VendorReviewReminders(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT org_id, id, name, owner_membership_id, next_review_at FROM vendors
		WHERE status = 'active' AND tier IN ('critical','high','medium') AND next_review_at IS NOT NULL AND next_review_at < now() + interval '30 days'`)
	if err != nil {
		return err
	}
	type due struct {
		org, id domain.ID
		name    string
		owner   *domain.ID
		at      time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.org, &d.id, &d.name, &d.owner, &d.at); err != nil {
			rows.Close()
			return err
		}
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		at := d.at
		if err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
			_, err := s.openWorkItem(ctx, tx, d.org, workItem{Kind: "vendor_review", Title: "Vendor review due: " + d.name, Priority: "medium",
				Description: "Collect the vendor's latest SOC 2 report or questionnaire and re-assess.", SourceKey: "vendorreview:" + d.id.String() + ":" + d.at.Format("2006-01-02"),
				Assignee: d.owner, Due: &at, ControlID: controlIDByCode(ctx, tx, d.org, "VEN-02"), Meta: map[string]any{"vendor_id": d.id.String()}}, true)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// RiskReviewReminders opens review tasks for risks due within 14 days (daily job).
func (s *Service) RiskReviewReminders(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT org_id, id, title, owner_membership_id, next_review_at FROM risks
		WHERE status IN ('open','in_treatment','accepted') AND next_review_at IS NOT NULL AND next_review_at < now() + interval '14 days'`)
	if err != nil {
		return err
	}
	type due struct {
		org, id domain.ID
		title   string
		owner   *domain.ID
		at      time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.org, &d.id, &d.title, &d.owner, &d.at); err != nil {
			rows.Close()
			return err
		}
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		at := d.at
		if err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
			_, err := s.openWorkItem(ctx, tx, d.org, workItem{Kind: "risk_review", Title: "Risk review due: " + d.title, Priority: "medium",
				Description: "Re-assess likelihood, impact and the treatment plan, then mark the risk reviewed.", SourceKey: "riskreview:" + d.id.String() + ":" + d.at.Format("2006-01-02"),
				Assignee: d.owner, Due: &at, ControlID: controlIDByCode(ctx, tx, d.org, "RSK-02"), Meta: map[string]any{"risk_id": d.id.String()}}, true)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
