// Package bootstrap prepares a database for use: schema migrations, River
// queue migrations and the compliance catalog sync.
package bootstrap

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
)

func Prepare(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	if err := db.Migrate(ctx, pool, log); err != nil {
		return err
	}
	return catalog.Sync(ctx, pool)
}
