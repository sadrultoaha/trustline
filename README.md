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
| linus@globex.test   | admin    | Globex Corporation |

Mailpit (all outgoing email): http://localhost:8025 · MinIO console: http://localhost:9001

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
