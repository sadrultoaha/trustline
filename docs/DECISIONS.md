# Decisions

Format: date, decision, reason, consequences. Newest at the bottom.

## 2026-10-19 — Single Go binary with subcommands
`trustline serve|worker|migrate|seed` instead of separate `cmd/` mains. Same build, same config
loader, fewer Dockerfiles. Compose runs the binary twice (api, worker).

## 2026-10-19 — River for background jobs, Redis for cache/rate limits
The stack allows "River or asynq". River is Postgres-backed, so a job can be inserted in the same
transaction as the mutation that caused it (no lost side effects on rollback). asynq would make
Redis a correctness dependency. Redis stays for rate limiting, short-lived caches, and later
pub/sub for UI live updates.

## 2026-10-19 — pgx directly, no sqlc
The stack allows "sqlc or pgx". Hand-written queries via pgx v5 with small repository structs keep
the toolchain to `go build` alone (no codegen step in CI or for contributors). Queries live next to
the domain package that owns them. If query volume becomes a maintenance burden, sqlc can be
introduced per package without changing callers.

## 2026-10-19 — goose for schema migrations, embedded in the binary
Migrations ship inside the binary (`embed.FS`), so `trustline migrate` works in any environment
without a separate tool. River's own queue tables are migrated programmatically in the same
command.

## 2026-10-19 — Application-level tenant scoping now; Postgres RLS deferred to Phase 6
Every repository method takes `orgID` and scopes by it; the HTTP tenant test proves foreign IDs
resolve to 404. RLS would require a non-superuser app role, `SET LOCAL` on every transaction and
a bypass flag for cross-org system jobs. That is worth doing as defense-in-depth but it is
hardening, not functionality, so it is scheduled for Phase 6 and tracked in STATUS.md.

## 2026-10-19 — Sessions in Postgres, not Redis
Sessions are tenant-relevant audit data (who was logged in to which org, from where). Keeping
them in Postgres makes them queryable for the security admin UI and survives Redis restarts.
Volume is low.

## 2026-10-19 — Code-based TanStack Router routes
File-based routing needs the Vite plugin and a generated route tree; code-based routes are
explicit, type-safe, and keep the build to `vite build`.

## 2026-10-19 — shadcn/ui components vendored by hand
The shadcn CLI is interactive. The components are plain TSX files under `web/src/components/ui`
following the upstream sources, which is exactly how shadcn is meant to be used.

## 2026-10-19 — Frontend served by the Go binary in Docker; Vite dev server locally
No `embed` of `web/dist`: the Go server serves a directory given by `WEB_DIST`. This keeps
`go build` independent of the Node toolchain for backend-only tests and CI jobs.

## 2026-10-19 — Email verification not required at signup
Demo and E2E flows should not depend on reading a verification mail. Invites and password resets
do go through email (via mailpit) so the mail path is fully exercised. Verification can be added
as an org setting later.

## 2026-10-19 — MinIO via `bitnamilegacy/minio`
`minio/minio` on Docker Hub no longer exists and `quay.io/minio/minio` refuses anonymous pulls, so
`docker compose up` failed. The Bitnami legacy image is the same MinIO server with an S3 API on
:9000 and console on :9001, and creates the `trustline-evidence` bucket on boot. The app only
speaks S3, so any S3-compatible store (AWS S3 in prod) works unchanged.

## 2026-10-20 — Rate limits keyed on identity first, IP second
Per-email limits (10 logins / 10 min, 5 resets / hour) are the brute-force guard. Per-IP limits are
loose (300 logins / 10 min, 100 signups / hour) because whole offices share one NAT address; the
tighter per-IP values locked out the E2E suite, which is what a real office would see too.

## 2026-10-20 — Migrations hold a Postgres advisory lock
The API and worker both migrate on boot. Without a lock they raced on first start and one crashed.
`db.Migrate` now takes `pg_advisory_lock` so concurrent callers serialise.
