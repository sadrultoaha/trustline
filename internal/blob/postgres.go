package blob

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres keeps blobs in the `blobs` table. It suits hosts where the API and
// the worker run on separate machines without shared disk or object storage.
type Postgres struct {
	Pool *pgxpool.Pool
}

func (p *Postgres) Name() string { return "postgres" }

func (p *Postgres) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO blobs (key, content_type, data) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data, created_at = now()`, key, contentType, data)
	return err
}

func (p *Postgres) Get(ctx context.Context, key string) ([]byte, error) {
	var data []byte
	err := p.Pool.QueryRow(ctx, `SELECT data FROM blobs WHERE key = $1`, key).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return data, err
}

func (p *Postgres) Delete(ctx context.Context, key string) error {
	_, err := p.Pool.Exec(ctx, `DELETE FROM blobs WHERE key = $1`, key)
	return err
}
