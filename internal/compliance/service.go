// Package compliance implements frameworks, controls, integrations, the test
// engine runtime, evidence, remediation tasks and notifications. Every query
// is scoped by org_id; services write the activity log in the same
// transaction as the mutation and enqueue follow-up jobs transactionally.
package compliance

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/blob"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/jobs"
	"github.com/trustline/trustline/internal/secrets"
)

type Service struct {
	Pool    *pgxpool.Pool
	Jobs    *jobs.Client
	Box     *secrets.Box
	Blob    blob.Store
	Catalog *catalog.Catalog
	Log     *slog.Logger
	BaseURL string
}
