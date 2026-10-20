-- +goose Up

-- Blob storage for single-database deployments (BLOB_MODE=postgres). Keys are
-- already org-prefixed by callers; the table is not tenant-scoped itself.
CREATE TABLE blobs (
  key          text PRIMARY KEY,
  content_type text NOT NULL DEFAULT '',
  data         bytea NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE blobs;
