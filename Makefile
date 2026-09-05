# job-find — developer convenience targets.
# Run `make help` for the list.

.PHONY: help up down seed api ingest frontend test tidy fmt vet build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

up: ## Start postgres + redis (backing stores only)
	docker compose up -d postgres redis

stack: ## Build & run the entire stack in Docker
	docker compose up --build

down: ## Stop everything
	docker compose down

seed: ## Run a one-shot ingestion pass to populate the DB
	cd backend && go run ./cmd/ingest once

api: ## Run the API server locally (:8080)
	cd backend && go run ./cmd/api

ingest: ## Run the ingestion worker locally (queue + cron)
	cd backend && go run ./cmd/ingest worker

frontend: ## Run the Next.js dev server (:3000)
	cd frontend && npm run dev

test: ## Run backend tests
	cd backend && go test ./...

tidy: ## Sync go.mod/go.sum (also generates go.sum on first run)
	cd backend && go mod tidy

fmt: ## Format Go code
	cd backend && gofmt -w .

vet: ## Vet Go code
	cd backend && go vet ./...

build: ## Build both binaries
	cd backend && go build -o bin/api ./cmd/api && go build -o bin/ingest ./cmd/ingest
