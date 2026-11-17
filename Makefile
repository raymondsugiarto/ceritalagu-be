# Common tasks. Run `make help` to list them.
APP      := server
BIN      := bin/$(APP)
MODULE   := github.com/yourorg/music-api
VERSION  ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
GOLANGCI := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

.PHONY: help run worker build test cover lint fmt tidy docker
help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-16s %s\n", $$1, $$2}'

run: ## Run locally (config/resources/*.yml + .env + env overrides)
	go run ./cmd/$(APP) serve

worker: ## Run the song generation worker (needs Redis + SECURITY_MASTERKEY)
	go run ./cmd/$(APP) worker

build: ## Build a static binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X $(MODULE)/internal/cli.version=$(VERSION)" -o $(BIN) ./cmd/$(APP)

test: ## Run tests with the race detector
	go test -race -count=1 ./...

cover: ## Run tests and open the coverage report
	go test -race -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

lint: ## Run golangci-lint
	$(GOLANGCI) run ./...

fmt: ## Format code (gofmt + goimports via golangci-lint)
	$(GOLANGCI) fmt ./...

tidy: ## Tidy go.mod/go.sum
	go mod tidy

docker: ## Build the container image
	docker build -t $(APP):local .

# ---- Database (rules: ../docs/database-standards.md) ----
.PHONY: migrate-create migrate-up migrate-down migrate-version migrate-force
migrate-create: ## Create an up/down migration pair: make migrate-create name=create_song
	@test -n "$(name)" || (echo "usage: make migrate-create name=<verb>_<scope>" && exit 1)
	migrate create -ext sql -dir db/migrations/postgres -seq $(name)

migrate-up: ## Apply pending migrations (step=N for N only, schema=x for another schema)
	@echo "--- running db migration ---"
	@go run ./cmd/$(APP) db migrate up $(step) --schema=$(schema)

migrate-down: ## Roll back migrations (step=N, default 1)
	@go run ./cmd/$(APP) db migrate down $(step) --schema=$(schema)

migrate-version: ## Print the current migration version
	@go run ./cmd/$(APP) db migrate version --schema=$(schema)

migrate-force: ## Force a version (dirty-state recovery): make migrate-force version=N
	@go run ./cmd/$(APP) db migrate force $(version) --schema=$(schema)
