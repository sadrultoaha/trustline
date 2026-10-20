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

## Phase 1 — Integrations, test engine, scheduler, controls/frameworks UI
⬜ not started

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
