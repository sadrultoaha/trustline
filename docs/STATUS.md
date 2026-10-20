# Status

Legend: ✅ done · 🟡 partial · ⬜ not started · ⏸ deferred (reason in DECISIONS.md)

## Phase 0 — Scaffold, auth, orgs, RBAC, CI, seed, E2E harness

| Item | Status | Notes |
|---|---|---|
| Repo layout, single `trustline` binary (serve/worker/migrate/seed) | ✅ | `cmd/trustline` |
| docker-compose: postgres, redis, mailpit, minio, app, worker | ✅ | `make up` |
| Makefile: dev, test, e2e, seed, migrate (+ up/down/reset/logs) | ✅ | |
| Migrations (goose, embedded) + River queue migrations | ✅ | applied on boot by api and worker |
| Email + password auth (argon2id), sessions (hashed tokens, sliding expiry) | ✅ | |
| Google OAuth: real client + fake consent page, selected by config | ✅ | `GOOGLE_OAUTH_MODE` |
| Password reset via email | ✅ | mailpit locally |
| Orgs, memberships, org switcher, create additional org | ✅ | |
| Roles admin / editor / auditor / employee with permission matrix | ✅ | `internal/rbac` |
| Invites by email (hashed token, 7-day expiry, resend, revoke) | ✅ | |
| Activity log, append-only (DB trigger), cursor-paginated viewer | ✅ | every mutation records an entry |
| Multi-tenancy: org_id everywhere, org-scoped repos, cross-tenant test | ✅ | `TestCrossTenantIsolation` + E2E `04-tenant-isolation` |
| Rate limiting on login/signup/forgot (Redis) | ✅ | memory fallback in tests |
| Background jobs (River) with email worker | ✅ | |
| Frontend: Vite + React + TS, TanStack Router/Query, Tailwind, shadcn/ui | ✅ | |
| Pages: login, signup, forgot/reset, invite, onboarding, dashboard, settings (org, members, activity, profile) | ✅ | |
| Go unit + integration tests (testcontainers) | ✅ | `make test` |
| Playwright harness + Phase 0 suite (auth, OAuth, members/RBAC, tenant isolation) | ✅ | `make e2e` |
| GitHub Actions CI (go, web, e2e) | ✅ | `.github/workflows/ci.yml` |
| Seed: Acme Corp (6 users across all roles) + Globex | ✅ | extended per phase |
| Postgres row-level security | ⏸ | Phase 6 hardening |
| Email verification at signup | ⏸ | not needed for demo; see DECISIONS |

## Phase 1 — Integrations, test engine, scheduler, controls/frameworks UI (Modules 1 & 2)

| Item | Status | Notes |
|---|---|---|
| Frameworks seeded as data: SOC 2 (all 5 TSC, 61), ISO 27001:2022 Annex A (93), HIPAA Security Rule (50), GDPR (29), PCI DSS 4.0 subset (55), NIST AI RMF (72), ISO 42001 (38) | ✅ | `internal/catalog/data/frameworks` |
| Crosswalk: one control → requirements in many frameworks | ✅ | 95 controls |
| Automated test library: 120 declarative tests across 6 connectors | ✅ | validated against resource schemas in CI |
| Test engine: all/any/none/count, where filters, nested AND/OR/NOT, 15 operators, fan-out paths, failure reasons | ✅ | `internal/testengine` |
| Connector interface + registry + credential schemas, credentials encrypted (AES-256-GCM) | ✅ | |
| Sandbox connectors: AWS, GCP, GitHub, Okta, Google Workspace, HRIS (BambooHR-style) with scenarios + state editing | ✅ | shared company roster across systems |
| Live API clients for AWS / GCP / GitHub / Okta / Google Workspace / BambooHR | ⏸ | Phase 6 per build order; `mode: live` returns a validation error |
| MDM/device agent endpoint, Jira and Slack *connectors* | ⏸ | device reports → Phase 2 (Personnel); Jira → Phase 6. Slack *notifications* are done (below) |
| Scheduler (hourly default, 20s in compose) + on-demand sync + run test now | ✅ | River periodic job, unique sync jobs |
| Frozen evidence snapshot per run (JSON + SHA-256), viewable per run | ✅ | |
| No-code custom test builder (field/operator/value, nested groups, live preview) | ✅ | compiles to the same rule JSON |
| Control status: passing / failing / needs evidence, with reason | ✅ | |
| Remediation tasks with steps + Terraform from the AI layer; auto-resolve on pass | ✅ | |
| Notifications: in-app bell, email (mailpit), Slack webhook | ✅ | |
| Control detail: status, owner, mapped requirements across frameworks, tests, evidence with expiry | ✅ | linked policies arrive with Policies in Phase 2 |
| Evidence upload to S3/MinIO, AI validation (pass/fail + reasoning), human override, download, expiry job | ✅ | |
| Framework readiness dashboard with requirement drilldown | ✅ | |
| AI layer slice: provider interface (Anthropic + fake), versioned prompts, structured output, redaction, per-org opt-out, token/cost log | ✅ | assistant/agent/MCP in Phase 5 |
| Acceptance: public bucket → control red within one tick, notification, remediation task with Terraform | ✅ | Go `TestPublicBucketFlipsControlAndCreatesRemediation`, E2E `05-monitoring` |
| Acceptance: second framework → crosswalk readiness non-zero immediately | ✅ | Go `TestSecondFrameworkReadinessViaCrosswalk`, E2E `06-frameworks-evidence` |
| Cross-tenant tests for all Phase 1 endpoints | ✅ | `TestPhase1TenantIsolation` |
| Seed: Acme with SOC 2 + ISO 27001, 5 integrations, passing and failing controls, owners, evidence | ✅ | |

## Phase 2 — Policies, Personnel, Access reviews
⬜ not started

## Phase 3 — Risk, Vendors
⬜ not started

## Phase 4 — Audit hub, Trust Center, Questionnaires
⬜ not started

## Phase 5 — AI layer, in-app agent, MCP server
⬜ not started

## Phase 6 — Hardening, real clients, OpenAPI, demo script
⬜ not started
