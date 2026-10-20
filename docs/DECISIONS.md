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

## 2026-10-20 — Catalog tables are global (the only tables without org_id)
Frameworks, requirements, the crosswalk, catalog controls and catalog tests are reference content
shared by every tenant, shipped as YAML inside the binary and synced on boot. Request handlers
never write them. Everything an org can change (its controls, tests, evidence, tasks) is copied
into org-scoped tables, so the "every table has org_id" rule holds for all tenant data.

## 2026-10-20 — Sandbox connectors in Phase 1; live API clients in Phase 6
The build order puts real AWS/GitHub/Okta clients in Phase 6. Phase 1 ships the Connector
interface, credential schemas (validated and encrypted exactly as live credentials will be) and a
sandbox implementation per connector. Requesting `mode: live` returns a clear validation error
instead of a stub. The sandbox is not a mock: it is a persistent simulated account whose state
users and tests change through scenarios, which is what the acceptance flow needs.

## 2026-10-20 — 95 controls, 120 tests, PCI subset of 55
The spec asked for about 80 controls and 120 tests. Covering seven frameworks including NIST AI RMF
and ISO 42001 without forcing unrelated requirements onto generic controls needed 8 AI-governance
and 2 payments controls on top of the core set. The PCI DSS subset is 55 sub-requirements instead
of about 40 so each of the 12 principal requirements has its most testable items.

## 2026-10-20 — Readiness is requirement-based
A requirement counts as met only when every control mapped to it passes. That is stricter than
"percent of controls passing" (which is also shown), but it is what an auditor tests. New orgs see
low SOC 2 percentages until manual-evidence controls are covered, which is accurate.

## 2026-10-20 — One open remediation task per failing test
Tasks are keyed `test:<id>` with a partial unique index on open tasks. A test failing on every
sync does not create duplicates. The task closes itself when the test passes, and is dismissed if
the test is disabled or its integration disconnected.

## 2026-10-20 — AI model `claude-sonnet-4-6` by default, configurable
The spec names claude-sonnet-4-6. `AI_MODEL` overrides it. Calls use structured outputs and
adaptive thinking. The fake provider is the default (`AI_MODE=fake`) so the full flow runs offline.

## 2026-10-20 — Sync jobs are unique per integration
`SyncIntegration` uses River unique jobs keyed by args across available/pending/running/
scheduled/retryable states, so a slow sync and a scheduler tick never stack up. A sandbox change
made while a sync is running is picked up by the next tick.

## 2026-10-20 — A built-in "trustline" connector instead of bespoke checks
Policy acknowledgement, training, devices, offboarding and access-review cadence are evaluated by
the same rule engine as cloud checks, over a snapshot built from Trustline's own data. One
mechanism gives them run history, frozen evidence, remediation tasks, notifications and the
custom test builder for free. The connector is internal: it is not listed for connecting and
cannot be disconnected.

## 2026-10-20 — Tests can be "not applicable"
`all` over zero resources is a vacuous pass. That is right for "every bucket blocks public
access" but wrong for "everyone completed training" in an org with no HR data yet. Rules can set
`require_resources`; with no resources the result is `na`, which does not decide the control. All
platform people/device/policy tests use it.

## 2026-10-20 — HRIS-driven lifecycle
The first HRIS import never starts onboarding for existing staff (only hires from the last 30
days) and never offboards existing leavers. Later changes do: new hires get a checklist and an
email invite; terminations get one verified removal task per connected identity system and lose
Trustline access immediately. A reactivation dismisses open offboarding tasks but does not restore
Trustline access automatically — re-inviting is a deliberate admin action.

## 2026-10-20 — The identity provider for "access revoked" is Okta when connected
`idp_active` follows Okta when Okta is connected, otherwise Google Workspace. Separate tests cover
Google Workspace suspension, GitHub removal and AWS IAM users, so an org sees each remaining
system explicitly.

## 2026-10-20 — Acknowledgement audience = admins, editors and employees
Auditors are excluded. The policy-acknowledged test passes when, for every published policy, the
acknowledged share reaches the org's threshold (default 90%). A second test requires every
active person with an account to acknowledge every policy, so a stragglers list is always visible.

## 2026-10-20 — Platform syncs are not de-duplicated
Unique jobs drop an insert while a matching job is running, which lost changes made mid-sync.
Platform syncs are cheap, so every change enqueues one; test runs skip any snapshot older than the
latest for the same integration.

## 2026-10-20 — Access-review reviewers come from the HRIS manager
Each account is assigned to the account owner's manager when the manager has a Trustline account,
otherwise to the campaign creator. Admins and editors can decide any item; other members only
their own.

## 2026-10-20 — Risks raised by tests and AI are suggestions
A failing critical test or an AI assessment never writes straight into the register. It creates a
`suggested` risk that a person accepts or dismisses, so the register only holds risks someone has
owned. Only `critical`-severity tests raise risks, to keep the queue short.

## 2026-10-20 — Vendor reviews cite page and section; tiers are never auto-applied
Every finding in a vendor review carries `{page, section, quote}` so a reviewer can check it
against the report. The recommended tier is shown next to the current one and applied by a
person, because tier drives review cadence and contractual follow-up.

## 2026-10-20 — PDF text comes from glyph positions
The PDF library's row grouping returned whole pages as one row. `internal/pdftext` rebuilds lines
from glyph X/Y positions, and the vendor review drops page footers and running headers before
matching, so citations point at the right page. Image-only PDFs are not OCR'd (STATUS, Phase 3).

## 2026-10-20 — Questionnaire links are bearer tokens
The vendor portal is unauthenticated by design. The token is 32 random bytes, stored only as a
hash, expires after 30 days, and is locked once answers are submitted.

## 2026-10-20 — Test helper re-requests deduplicated syncs
Integration syncs are unique jobs that include the `running` state. A sync requested in the moment
between a previous sync writing `last_sync_at` and River marking that job complete is dropped. The
test helper retries until a new sync is observed. The same short window applies to the **Sync now**
button; the scheduler's next tick covers it.

## 2026-10-20 — Hosted demo: Netlify frontend, Render backend
Netlify serves static sites and short functions; Trustline needs a long-running API, a job worker,
Postgres and Redis. Netlify serves the frontend and proxies API paths to Render, which runs the
container from `deploy/Dockerfile` with managed Postgres and Key Value. API and worker share one
process (`trustline all`) so the free plan needs one service. Uploaded files are stored in
Postgres (`BLOB_MODE=postgres`) rather than on disk, because Render pre-deploy jobs and separate
services don't share a disk. At demo scale (a few MB) this costs nothing; production should use S3.

## 2026-10-20 — Public demo runs in prod mode without Google sign-in
The fake Google provider lets anyone pick any email, so it is refused in prod. The hosted demo
uses `GOOGLE_OAUTH_MODE=off`, which removes the routes and hides the buttons via
`GET /api/v1/auth/providers`. Mail goes to an in-memory outbox, so password reset and invite emails
are not delivered there; questionnaire links are shown in the UI when sent.
