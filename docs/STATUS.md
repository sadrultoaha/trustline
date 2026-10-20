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

## Phase 2 — Policies, Personnel, Access reviews (Modules 3, 6, 7)

| Item | Status | Notes |
|---|---|---|
| Policy library: 22 templates (InfoSec, Access Control, Password, Acceptable Use, Incident Response, BCP/DR, Vendor, Data Retention, Change, Risk, AI Acceptable Use, Encryption, Asset, Secure Development, Logging, Vulnerability, Physical, Data Classification, HR Security, Code of Conduct, Privacy, Network) | ✅ | placeholders filled from org settings |
| Rich-text editor (TipTap), sanitized HTML storage | ✅ | |
| Versioning + approval workflow (draft → review → approve/reject → publish → superseded) | ✅ | approver or admin decides |
| Annual review reminders (daily job, task + notification), mark reviewed | ✅ | |
| Employee acknowledgement per version, tasks, My work | ✅ | |
| AI policy draft from org context | ✅ | `policy_draft.v1` |
| AI policy ↔ settings inconsistency detection (on demand and after publish) | ✅ | `policy_consistency.v1` |
| Acceptance: publish v2 → staff get acknowledgement tasks → GOV-05 fails until threshold | ✅ | Go `TestPolicyPublishAcknowledgementFlow`, E2E `07-policies` |
| HRIS sync → people, onboarding checklist (account, policies, training, device, background) | ✅ | auto-verified items, new-hire email invite |
| Offboarding: per-system removal tasks verified by sync, SLA 1 business day, Trustline access removed immediately | ✅ | |
| Acceptance: terminate in HRIS → offboarding → "access revoked" fails until Okta deactivation | ✅ | Go `TestOffboardingVerifiedByIdentityProvider`, E2E `08-personnel` |
| Built-in security awareness course + quiz + completion records, annual renewal tasks | ✅ | |
| Device agent endpoint + enrollment token + agent script (macOS/Linux/Windows) | ✅ | disk encryption, screen lock, anti-malware tests |
| Background check attestation | ✅ | |
| Access review campaigns: pull users/roles per integration, manager reviewers, keep/revoke, verified revocation tasks, CSV export as IAM-04 evidence, scheduled campaigns | ✅ | Go `TestAccessReviewCampaign`, E2E `09-access-reviews` |
| Platform connector: policy/person/device/access-review tests on the rule engine (18 tests) | ✅ | |
| Cross-tenant tests for Phase 2 endpoints | ✅ | `TestPhase2TenantIsolation` |
| Policy ↔ control linking from both sides (control page "Policies" card, policy page "Supports controls") | ✅ | done in Phase 3 work: Go `TestPolicyControlLinks`, E2E `12-policy-control-links` |
| SCIM / CSV HRIS import | ⏸ | BambooHR-style sandbox connector covers the flow; CSV/SCIM import with the live clients in Phase 6 |
| MDM integrations (Jamf, Intune, Kandji) | ⏸ | the agent endpoint covers device compliance; MDM API connectors in Phase 6 |

## Phase 3 — Risk, Vendors (Modules 4, 5)

| Item | Status | Notes |
|---|---|---|
| Risk register: likelihood × impact (5×5), inherent and residual scores, levels, categories | ✅ | |
| Heatmap (inherent and residual), click a cell to filter | ✅ | |
| Treatment (mitigate/accept/transfer/avoid), plans, owners, statuses | ✅ | |
| Risk ↔ control links, shown on both pages | ✅ | |
| Review cadence: per-risk cycle, daily reminder job, tasks, mark reviewed | ✅ | |
| "Complete with AI" risk assessment from failing tests, systems, vendors and people | ✅ | `risk_assessment.v1`; results are suggestions, never auto-accepted |
| Acceptance: new failing test on a critical control → suggested risk → accept links them | ✅ | Go `TestCriticalFailureRaisesSuggestedRisk`, E2E `10-risks` |
| Platform tests for risks and vendors (RSK-01/02, VEN-01/02/03) | ✅ | 8 rule-engine tests on the built-in connector |
| Vendor inventory: manual entry + discovery from Google Workspace OAuth grants and Okta apps | ✅ | discovered vendors wait for triage (add or dismiss); dismissals can be restored from the Dismissed tab |
| Tiering (critical/high/medium/low), data access, PII, owner, review cycle by tier | ✅ | |
| Document upload (SOC 2 I/II, ISO 27001, pen test, DPA, BAA, insurance, other) with expiry | ✅ | blob storage |
| Security questionnaire (20 questions) by magic link, public vendor portal, save draft + submit | ✅ | token hashed, 30-day expiry, rate limited |
| AI vendor review of a SOC 2 report or questionnaire answers: opinion, controls tested, exceptions, CUECs, subservice orgs, risk summary, recommended tier | ✅ | `vendor_assessment.v1`; PDF text from `internal/pdftext` |
| Acceptance: sample SOC 2 PDF in `/testdata` → assessment with page and section citations | ✅ | Go `TestVendorSOC2AssessmentWithCitations`, E2E `11-vendors` |
| Cross-tenant tests for Phase 3 endpoints (including the portal token) | ✅ | `TestPhase3TenantIsolation` |
| Seed: 5 risks with owners and controls, AI suggestions, Northwind (SOC 2 + DPA, assessed), Slack, discovered apps | ✅ | |
| Scanned (image-only) PDFs | ⏸ | no OCR; the review says no text was found. With `AI_MODE=anthropic` the PDF could go to Claude as a document block — Phase 5 |
| Vendor discovery from other SSO providers (Entra ID, JumpCloud) | ⏸ | connectors arrive with the live clients in Phase 6 |

## Phase 4 — Audit hub, Trust Center, Questionnaires
⬜ not started

## Phase 5 — AI layer, in-app agent, MCP server
⬜ not started

## Phase 6 — Hardening, real clients, OpenAPI, demo script
⬜ not started
