SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE ?= docker compose
INFRA   := postgres redis mailpit minio
GO      ?= go
export DATABASE_URL ?= postgres://trustline:trustline@localhost:5432/trustline?sslmode=disable
export REDIS_URL    ?= redis://localhost:6380/0
export SMTP_ADDR    ?= localhost:1025
export BASE_URL     ?= http://localhost:5173
export SCHEDULER_INTERVAL ?= 1m

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

## ---- local development (Go + Vite on the host, infra in docker) ----------

infra: ## Start postgres, redis, mailpit, minio in docker
	$(COMPOSE) up -d $(INFRA)
	@echo "waiting for postgres..." && until $(COMPOSE) exec -T postgres pg_isready -U trustline >/dev/null 2>&1; do sleep 1; done

dev: infra migrate ## Run API, worker and Vite dev server (Ctrl-C stops all)
	@trap 'kill 0' INT TERM; \
	  $(GO) run ./cmd/trustline serve & \
	  $(GO) run ./cmd/trustline worker & \
	  (cd web && npm run dev) & \
	  wait

api: ## Run only the API on :8080
	$(GO) run ./cmd/trustline serve

worker: ## Run only the worker
	$(GO) run ./cmd/trustline worker

migrate: ## Apply migrations
	$(GO) run ./cmd/trustline migrate

seed: ## Create the demo org and accounts (idempotent)
	$(GO) run ./cmd/trustline seed

build: ## Build the Go binary and the web app
	mkdir -p bin && $(GO) build -o bin/trustline ./cmd/trustline
	cd web && npm run build

## ---- tests ---------------------------------------------------------------

test: ## Go unit + integration tests (testcontainers starts postgres)
	$(GO) vet ./...
	$(GO) test ./... -count=1

test-web: ## Typecheck + lint the web app
	cd web && npm run lint && npx tsc -b

e2e: ## Full stack in docker + Playwright suite
	$(COMPOSE) up -d --build
	@echo "waiting for app..." && until curl -sf http://localhost:8080/readyz >/dev/null; do sleep 1; done
	$(COMPOSE) exec -T app /app/trustline seed
	$(COMPOSE) exec -T redis redis-cli FLUSHALL >/dev/null # fresh rate-limit windows for the suite
	cd e2e && npm run test

e2e-ui: ## Playwright in UI mode against an already running stack
	cd e2e && npx playwright test --ui

## ---- docker stack ---------------------------------------------------------

up: ## Build and start the full stack in docker (app on :8080)
	$(COMPOSE) up -d --build
	@echo "waiting for app..." && until curl -sf http://localhost:8080/readyz >/dev/null; do sleep 1; done
	$(COMPOSE) exec -T app /app/trustline seed

down: ## Stop the stack
	$(COMPOSE) down

reset: ## Stop the stack and delete volumes
	$(COMPOSE) down -v

logs: ## Tail app + worker logs
	$(COMPOSE) logs -f app worker

.PHONY: help infra dev api worker migrate seed build test test-web e2e e2e-ui up down reset logs
