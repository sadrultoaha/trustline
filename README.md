# Trustline

Compliance automation / GRC platform: continuous control monitoring, evidence, policies, risk,
vendors, personnel, access reviews, audits, trust center, questionnaire automation, and an AI
layer with an MCP server.

* [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — how it is built
* [docs/DECISIONS.md](docs/DECISIONS.md) — why
* [docs/STATUS.md](docs/STATUS.md) — what works today

## Quick start (everything in Docker)

```bash
make up          # builds the image, starts postgres/redis/mailpit/minio/app/worker, seeds demo data
open http://localhost:8080
```

Demo accounts (password `Trustline-demo-2026!`):

| Email               | Role     | Org                |
|---------------------|----------|--------------------|
| ada@acme.test       | admin    | Acme Corp          |
| grace@acme.test     | editor   | Acme Corp          |
| auditor@acme.test   | auditor  | Acme Corp          |
| sam@acme.test       | employee | Acme Corp          |
| jo@, kim@, maria@, tom@, lee@acme.test | employee | Acme Corp |
| linus@globex.test   | admin    | Globex Corporation |

Mailpit (all outgoing email): http://localhost:8025 · MinIO console: http://localhost:9001

## Try continuous monitoring (2 minutes)

1. Sign in as `ada@acme.test`. The dashboard shows SOC 2 and ISO 27001 readiness and the failing controls.
2. Open **Integrations → AWS → Manage** and turn on **Make the marketing bucket public**.
3. Open **Controls → DAT-01**. Within one scheduler tick (20 seconds in Docker) it turns red, the bell shows a notification, and a remediation task appears with step-by-step instructions and a Terraform fix.
4. Turn the scenario off and click **Sync now**: the control recovers and the task closes itself.

## Try policies and offboarding (3 minutes)

1. **Policies → Information Security Policy → New version**, edit, **Submit for approval** to Grace. Sign in as `grace@acme.test`, **Approve**; back as Ada, **Publish**. GOV-05 turns red until everyone acknowledges it from **My work**.
2. **Integrations → HRIS → Manage**, choose `tom@acme.test`, **Terminate employee**, **Sync now**. Tom appears under **People → Terminated** with an offboarding checklist, and "Terminated employees are deactivated in the identity provider" fails.
3. **Integrations → Okta**, **Deactivate user** for Tom, **Sync now**: the test passes and the Okta item verifies itself.
4. **Access reviews → New review** (Okta + GitHub): revoke, keep the rest, **Complete review**; the CSV is filed as evidence on IAM-04.

Set `AI_MODE=anthropic` and `ANTHROPIC_API_KEY` in `.env.docker` to use Claude instead of the deterministic fake.

## Local development (hot reload)

```bash
cp .env.example .env
make dev         # infra in docker, Go API + worker + Vite on the host → http://localhost:5173
make seed
```

## Tests

```bash
make test        # go vet + unit + integration (testcontainers starts postgres)
make test-web    # lint + typecheck the web app
make e2e         # full docker stack + Playwright
```

## Requirements

Go 1.23+ (the module pins a newer toolchain and downloads it automatically), Node 22, Docker.
