# Trustline Architecture

Trustline is a compliance-automation (GRC) platform: continuous control monitoring over
connected systems, evidence management, policies, risk, vendors, personnel, access reviews,
audits, a public trust center, questionnaire automation, and an AI layer with an MCP server.

This document describes the system as built. `DECISIONS.md` records why; `STATUS.md` records
what is done.

---

## 1. Service layout

```
                     ┌───────────────────────────────────────────┐
   browser ──HTTP──▶ │  trustline serve  (Go, chi)               │
   MCP client ─────▶ │   • REST API  /api/v1/*                   │
   webhooks ───────▶ │   • static web app (web/dist)             │
                     │   • MCP server /mcp (Phase 5)             │
                     │   • enqueues jobs (River, Postgres-backed)│
                     └──────┬────────────────┬───────────────────┘
                            │                │
                   ┌────────▼───────┐  ┌─────▼──────┐   ┌──────────┐
                   │ PostgreSQL 16  │  │  Redis 7   │   │  MinIO   │
                   │ data + jobs    │  │ cache, rate│   │ evidence │
                   │ (River queue)  │  │ limits     │   │ files    │
                   └────────▲───────┘  └────────────┘   └────▲─────┘
                            │                                │
                     ┌──────┴────────────────────────────────┴───┐
                     │  trustline worker (Go, River workers)     │
                     │   • email, connector sync, test runs,     │
                     │     scheduler ticks, AI jobs, exports     │
                     └───────────────────────────────────────────┘
                                         │
                                 ┌───────▼────────┐
                                 │ mailpit (local) │
                                 └────────────────┘
```

One Go binary, `trustline`, with subcommands:

| Subcommand | Role |
|---|---|
| `serve`   | HTTP API + static frontend. Inserts jobs; runs no workers. |
| `worker`  | River worker process. Runs all background jobs and periodic schedules. |
| `migrate` | Applies SQL migrations (goose, embedded) and River's queue migrations. |
| `seed`    | Idempotently creates the demo org and demo data. |

`serve` and `worker` are separate containers in docker-compose so they scale and crash
independently, but they share the same code and config.

### External services and fakes

Every external dependency sits behind an interface with a real and a fake implementation,
selected by config. The fake must be realistic enough for the complete user flow to run.

| Interface | Real | Fake | Config key |
|---|---|---|---|
| `mail.Sender` | SMTP (mailpit locally) | in-memory outbox (tests) | `MAIL_MODE=smtp\|memory` |
| `oauth.Provider` (Google) | `golang.org/x/oauth2` + userinfo | self-hosted consent page at `/auth/fake-google/authorize` | `GOOGLE_OAUTH_MODE=fake\|real` |
| `connectors.Connector` | AWS/GCP/GitHub/Okta/… API clients (Phase 6) | sandbox: controllable state stored on the integration row | per-integration `mode` (`sandbox`/`live`) |
| `ai.Provider` | Anthropic Messages API (`AI_MODEL`, default `claude-sonnet-4-6`) | deterministic fake producing the same structured outputs | `AI_MODE=fake\|anthropic` |
| `blob.Store` | S3-compatible (MinIO locally) | local filesystem | `BLOB_MODE=s3\|fs` |
| Slack notifications | incoming-webhook POST (org setting) | any HTTP endpoint (tests use `httptest`) | `settings.slack_webhook_url` |

---

## 2. Package structure

```
trustline/
├── cmd/trustline/           main: subcommand dispatch (serve, worker, migrate, seed)
├── internal/
│   ├── config/              env-driven config struct, validation
│   ├── db/                  pgx pool, migrations (embedded SQL, goose), tx helpers
│   │   └── migrations/      NNNN_name.sql (goose format, up+down)
│   ├── domain/              shared types: IDs, enums (Role, …), typed errors
│   ├── activity/            immutable ActivityLog writer; every mutation calls Record()
│   ├── auth/                password hashing (argon2id), sessions, invites, password reset
│   │   └── oauth/           Provider interface; google (real) + fake
│   ├── rbac/                Role → Permission matrix; Require() middleware
│   ├── org/                 orgs, memberships, invites repo + service
│   ├── mail/                Sender interface; smtp + memory; HTML templates
│   ├── ratelimit/           Redis sliding-window limiter
│   ├── jobs/                River client setup, job arg types, workers, periodic jobs
│   ├── httpapi/             chi router, middleware, JSON helpers, handlers per module
│   ├── seed/                demo data
│   └── testutil/            testcontainers Postgres + Redis, HTTP test client
├── web/                     React + TypeScript + Vite (TanStack Router/Query, Tailwind, shadcn/ui)
├── e2e/                     Playwright suite (runs against the composed stack)
├── deploy/                  Dockerfiles
├── docs/                    ARCHITECTURE, DECISIONS, STATUS
├── testdata/                sample files used by tests and the demo (SOC 2 PDF, questionnaire)
├── docker-compose.yml
└── Makefile                 dev, test, e2e, seed, migrate
```

Phase 1 packages:

```
internal/
├── catalog/            embedded compliance content + loader + DB sync
│   └── data/           controls.yaml, resources.yaml, frameworks/*.yaml, tests/*.yaml
├── testengine/         rule parsing, validation and evaluation (pure, no I/O)
├── connectors/         Connector interface, registry + credential schemas, sandbox fakes & scenarios
├── compliance/         frameworks, readiness, controls, status recompute, integrations, test runs,
│                       evidence, remediation tasks, notifications, dashboard
├── ai/                 Provider (Anthropic + Fake), prompts/*.tmpl, redaction, cost logging
├── blob/               evidence storage (S3/MinIO + filesystem)
├── secrets/            AES-256-GCM for integration credentials
├── worker/             River workers + periodic schedules
├── app/                constructors shared by cmd/ and the test harness
└── bootstrap/          migrations + catalog sync on boot
```

Later phases add `policies`, `risks`, `vendors`, `people`, `accessreview`, `audits`,
`trustcenter`, `questionnaires`, `mcp`, `webhooks`, `apikeys`.

### Layering rules

* `httpapi` handlers parse/validate input, call a service, and encode output. No SQL in handlers.
* Services own transactions and write `ActivityLog` entries inside the same transaction as the
  mutation. A mutation without an activity entry is a bug.
* Repositories take `orgID` as an explicit parameter for every tenant-scoped query.
* `domain` has no dependencies on other internal packages.

---

## 3. Multi-tenancy

* `orgs` is the tenant root. `users` is a global identity table (one email = one user);
  `org_memberships (org_id, user_id, role)` connects them. A user may belong to many orgs.
* **Every tenant table has `org_id uuid NOT NULL REFERENCES orgs(id)`** and an index starting
  with `org_id`.
* A session carries `current_org_id`. Middleware resolves the session, loads the membership for
  that org, and puts `tenant.Context{OrgID, UserID, Role}` on the request context.
* Every repository method that touches tenant data takes `orgID` and includes
  `WHERE org_id = $1`. Lookups by primary key are `WHERE id = $2 AND org_id = $1`, so a
  foreign ID resolves to "not found" rather than leaking.
* `internal/httpapi/tenant_test.go` proves cross-tenant reads fail: org A's session requesting
  org B's member, invite and activity rows gets 404 / empty lists.
* Postgres Row-Level Security as defense-in-depth is scheduled for Phase 6 (see DECISIONS).

---

## 4. Auth and RBAC

* **Email + password**: argon2id (64 MiB, t=3, p=2). Login is rate limited per IP and per email.
* **Google OAuth**: `oauth.Provider` interface; real implementation uses `x/oauth2`. The fake
  serves its own consent page listing fake accounts so the E2E suite exercises the full
  redirect → code → exchange → session flow.
* **Sessions**: random 256-bit token, SHA-256 stored in `sessions`, `HttpOnly; SameSite=Lax`
  cookie `tl_session`, 30-day sliding expiry. Logout deletes the row.
* **Invites**: `org_invites` with hashed token and 7-day expiry; email sent via a job.
* **Password reset**: `password_resets` with hashed token, 1-hour expiry.
* **Roles** (`org_memberships.role`): `admin`, `editor`, `auditor`, `employee`.
  `rbac.Permissions[role]` is the matrix; `rbac.Require(perm)` middleware enforces it.
  Auditors can read everything and mutate nothing (except audit requests, Phase 4).
  Employees see only their own tasks, trainings and policies to accept.

---

## 5. Background jobs and scheduling

**River** (Postgres-backed job queue) is used for all async work. Choosing River over asynq
keeps the queue transactional: a job is inserted in the same transaction as the business
mutation, so a mutation and its side effects commit or roll back together. Redis is used for
rate limiting and caching, not for the queue.

Queues:

| Queue | Workers | Purpose |
|---|---|---|
| `default` | 10 | email, notifications, activity fan-out, webhooks |
| `sync` | 5 | connector syncs (one job per integration), snapshot storage |
| `tests` | 10 | test-engine runs over a snapshot |
| `ai` | 2 | LLM calls (evidence validation, policy drafts, questionnaire answers) |
| `exports` | 2 | audit evidence zips, questionnaire exports |

Periodic jobs (River `PeriodicJobs`, run only by the worker):

* `SchedulerTick` hourly (configurable, 1 minute in dev/e2e): for each org, for each enabled
  integration, enqueue `ConnectorSync`. `ConnectorSync` stores a snapshot then enqueues
  `RunTests{snapshotID}`. `RunTests` evaluates every test bound to that integration, writes
  `test_runs`, and on a pass→fail transition emits `ControlStatusChanged` which fans out to
  notifications, remediation-task creation and webhooks.
* `EvidenceExpiryCheck` daily, `PolicyReviewReminder` daily, `AccessReviewCampaignOpen` daily.

Job args are JSON structs in `internal/jobs`; every job carries `OrgID` and sets tenant context
before doing work. Job failures retry with River's default exponential backoff (max 5 attempts).

---

## 6. Test-rule schema (test engine, Phase 1)

Automated tests are declarative rules evaluated against a **snapshot**. The UI's no-code
builder compiles to the same format.

### Snapshot

```json
{
  "connector": "aws",
  "integration_id": "…",
  "taken_at": "2026-10-19T12:00:00Z",
  "resources": {
    "iam_user":  [ { "id": "alice", "mfa_enabled": true, "access_keys": [ { "age_days": 12 } ] } ],
    "s3_bucket": [ { "id": "logs", "public_access_block": { "block_public_acls": true } } ]
  }
}
```

`resources` is a map of resource type → array of objects. Each object has a stable `id`.

### Rule

```yaml
id: aws.s3.block_public_access
name: All S3 buckets block public access
connector: aws
resource: s3_bucket
severity: high               # low | medium | high | critical
quantifier: all              # all | any | none | count
# count only: { min: 1, max: 5 }
where:                       # optional pre-filter on the collection
  { field: region, op: in, value: [us-east-1, eu-west-1] }
assert:                      # expression every (all) / some (any) / no (none) resource must satisfy
  and:
    - { field: public_access_block.block_public_acls,       op: eq, value: true }
    - { field: public_access_block.ignore_public_acls,      op: eq, value: true }
    - { field: public_access_block.block_public_policy,     op: eq, value: true }
    - { field: public_access_block.restrict_public_buckets, op: eq, value: true }
remediation:
  summary: Enable S3 Block Public Access on every bucket.
  steps:
    - Open the S3 console → bucket → Permissions → Block public access → Edit.
    - Enable all four settings and save.
  iac_hint: terraform:aws_s3_bucket_public_access_block
```

Fan-out semantics: a path with `[*]` yields several values; the condition holds when **every**
value satisfies it (vacuously true when there are none) unless the condition sets
`"match": "any"`. Quantifier `all` over an empty (filtered) set passes; `any` over an empty set
fails. Every failing resource gets a human-readable reason
(`public_access_block.block_public_acls = false (expected = true)`).

Expression grammar (JSON or YAML):

```
expr := { "and": [expr, …] }
      | { "or":  [expr, …] }
      | { "not": expr }
      | { "field": path, "op": op, "value": any }

path := dotted path into the resource; `[*]` fans out over arrays (e.g. access_keys[*].age_days)
op   := eq | neq | gt | gte | lt | lte | in | nin | contains | not_contains
      | exists | not_exists | matches (RE2 regex) | len_gt | len_lt
      (gt/gte/lt/lte also compare RFC3339 / YYYY-MM-DD dates)
```

Result of a run:

```json
{
  "status": "fail",
  "evaluated": 12,
  "failing": [ { "id": "public-assets", "reason": "public_access_block.block_public_acls = false" } ],
  "snapshot_id": "…"
}
```

The frozen snapshot plus this result is the evidence for that run and is never mutated.

---

## 7. HTTP API conventions

* Base path `/api/v1`. JSON in/out. Errors: `{"error": {"code": "not_found", "message": "…"}}`.
* Auth via session cookie; API keys (`Authorization: Bearer tl_…`) arrive in Phase 6.
* Lists are cursor-paginated: `?cursor=&limit=` → `{"items": […], "next_cursor": "…"}`.
* All mutations are `POST`/`PATCH`/`DELETE`; the activity log records actor, action, target.
* `GET /healthz` (process up) and `GET /readyz` (DB + Redis reachable).

---

## 8. Frontend

Vite + React 18 + TypeScript. TanStack Router (code-based routes), TanStack Query for all
server state, Tailwind + shadcn/ui components (vendored under `web/src/components/ui`).
`web/src/api/` holds a typed fetch client; `web/src/features/<module>/` holds pages and hooks.
In development Vite proxies `/api` and `/auth` to the Go server on `:8080`. In Docker the Go
server serves `web/dist`.

---

## 9. Testing

| Layer | Tool | Where |
|---|---|---|
| Unit | `go test` | next to code |
| Integration | `go test` + testcontainers (postgres:16, redis:7) | `internal/**/..._test.go` using `testutil` |
| E2E | Playwright (Chromium) against `docker compose` stack | `e2e/` |
| CI | GitHub Actions: lint, unit+integration, build web, E2E | `.github/workflows/ci.yml` |

The E2E suite is the acceptance test for each phase and, by the end, walks the full story.

---

## 10. Security baseline (Phase 0)

* Secrets only via environment; `.env.example` documents them.
* Passwords argon2id; tokens stored hashed; cookies `HttpOnly`, `Secure` outside dev.
* Rate limits on signup/login/forgot-password (Redis).
* `ActivityLog` is append-only (DB trigger rejects UPDATE/DELETE).
* Integration credentials (Phase 1) encrypted at rest with AES-256-GCM under `ENCRYPTION_KEY`.

---

## 11. Continuous monitoring (Phase 1)

```
 SchedulerTick (every SCHEDULER_INTERVAL; 20s in compose, 1h default)
   └─ for each enabled integration → SyncIntegration (queue: sync, unique per integration)
        connector.Sync() → snapshots row (frozen JSON + SHA-256) → RunTests (queue: tests)
          evaluate enabled tests for that connector → test_runs rows
          recompute affected controls → transitions
            test → fail      : open remediation task (one open task per test) → GenerateRemediation (queue: ai)
            test fail → pass : task auto-resolved ("Resolved automatically…")
            control → failing: notifications (admins, editors, owner) + email jobs + Slack job
            control recovers : same channels
```

**Catalog → org instances.** Catalog tables (`catalog_*`) are global and read-only, synced from
YAML on every boot. Adopting a framework creates org `controls` for every control code that
framework maps to (existing controls are reused, which is how the crosswalk carries readiness).
Connecting an integration instantiates org `tests` for every catalog test whose connector matches
and whose control is in scope. One integration per connector type per org.

**Control status** (`compliance.deriveStatus`):

| Condition | Status |
|---|---|
| any enabled test failing or erroring | `failing` |
| ≥1 enabled test and all pass | `passing` |
| otherwise, accepted unexpired evidence exists (`COALESCE(override, ai) = pass`) | `passing` |
| otherwise | `needs_evidence` |

**Readiness.** A requirement is *met* when every control it maps to is in the org and passing.
Framework readiness % = met requirements / total requirements. Control-level counts are shown
alongside.

**Evidence.** Uploaded to the blob store under `orgs/<org>/evidence/<id>/<file>`, SHA-256
recorded, expiry defaults to the control's `evidence_ttl_days`. `ValidateEvidence` (queue: ai)
asks the AI layer for `{verdict, confidence, reasoning, missing}`; a reviewer's override always
wins. A daily job re-evaluates controls whose evidence expired.

**Sandbox connectors.** Each sandbox integration stores its full resource state. The baseline is
realistic for the org (same people across Okta, Google Workspace, GitHub, AWS and the HRIS) and
deliberately contains a handful of common gaps. Named scenarios (e.g. `public_bucket`) are
reversible mutations: state = baseline + active scenarios. `PUT …/sandbox/state` replaces the
state wholesale for advanced cases.

## 12. AI layer (Phase 1 slice)

`ai.Service` is the only way code reaches a model. It checks the org opt-out
(`settings.ai_enabled`), renders a versioned prompt template (`prompts/<task>.v<N>.tmpl`, with
`system` and `user` blocks), redacts emails, cloud keys, tokens, card numbers and phone numbers,
calls the provider with a JSON schema (Anthropic structured outputs, adaptive thinking), parses
the result, and logs tokens, cost and latency to `ai_calls`. Tasks implemented: `remediation_plan`
(steps + Terraform for the failing resources) and `evidence_validation`. Phase 5 adds the
assistant, agent tools and MCP on the same package.
