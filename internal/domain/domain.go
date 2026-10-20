// Package domain holds types shared across all modules. It depends on nothing
// else inside the repository.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ID = uuid.UUID

func NewID() ID { return uuid.Must(uuid.NewV7()) }

func ParseID(s string) (ID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, ErrInvalidID
	}
	return id, nil
}

// Role is an org-scoped role held by a membership.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleEditor   Role = "editor"
	RoleAuditor  Role = "auditor"
	RoleEmployee Role = "employee"
)

var AllRoles = []Role{RoleAdmin, RoleEditor, RoleAuditor, RoleEmployee}

func (r Role) Valid() bool {
	for _, x := range AllRoles {
		if x == r {
			return true
		}
	}
	return false
}

// Error kinds. HTTP maps these to status codes; services return them wrapped.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidID    = errors.New("invalid id")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("rate limited")
)

// ValidationError carries field-level messages for the client.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %v", e.Fields)
}

func Invalid(field, msg string) error {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

func NewValidation() *ValidationError { return &ValidationError{Fields: map[string]string{}} }

func (e *ValidationError) Add(field, msg string) { e.Fields[field] = msg }
func (e *ValidationError) Err() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

// Org is the tenant root.
type Org struct {
	ID        ID          `json:"id"`
	Name      string      `json:"name"`
	Slug      string      `json:"slug"`
	Settings  OrgSettings `json:"settings"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// OrgSettings is stored as jsonb; extend freely.
type OrgSettings struct {
	AccessReviewDays          int    `json:"access_review_days"` // policy consistency checks (Phase 2)
	PasswordMinLength         int    `json:"password_min_length"`
	PolicyAcceptanceThreshold int    `json:"policy_acceptance_threshold"` // percent
	AIEnabled                 bool   `json:"ai_enabled"`
	Timezone                  string `json:"timezone"`
	SlackWebhookURL           string `json:"slack_webhook_url"`
	EmailNotifications        bool   `json:"email_notifications"`
}

func DefaultOrgSettings() OrgSettings {
	return OrgSettings{AccessReviewDays: 90, PasswordMinLength: 12, PolicyAcceptanceThreshold: 90, AIEnabled: true, Timezone: "UTC", EmailNotifications: true}
}

type User struct {
	ID          ID         `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	HasPassword bool       `json:"has_password"`
	GoogleSub   string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type Membership struct {
	ID        ID        `json:"id"`
	OrgID     ID        `json:"org_id"`
	UserID    ID        `json:"user_id"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Member is a membership joined with its user, for listing.
type Member struct {
	Membership
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type Invite struct {
	ID         ID         `json:"id"`
	OrgID      ID         `json:"org_id"`
	Email      string     `json:"email"`
	Role       Role       `json:"role"`
	InvitedBy  *ID        `json:"invited_by,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Session struct {
	ID         ID
	UserID     ID
	OrgID      *ID
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

// Actor identifies who performed an action for the activity log.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorSystem ActorType = "system"
	ActorAPIKey ActorType = "api_key"
	ActorAgent  ActorType = "agent"
)

type Actor struct {
	Type  ActorType
	ID    *ID
	Label string
	IP    string
}

func SystemActor(label string) Actor { return Actor{Type: ActorSystem, Label: label} }

type ActivityEntry struct {
	ID          ID             `json:"id"`
	OrgID       ID             `json:"org_id"`
	ActorType   ActorType      `json:"actor_type"`
	ActorID     *ID            `json:"actor_id,omitempty"`
	ActorLabel  string         `json:"actor_label"`
	Action      string         `json:"action"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id,omitempty"`
	TargetLabel string         `json:"target_label,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	IP          string         `json:"ip,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Page is a cursor-paginated list result.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}
