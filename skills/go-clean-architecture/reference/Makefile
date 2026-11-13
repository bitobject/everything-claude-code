# Repository-level targets for the Go backend (template; the web half adds its pnpm scripts).
# `make check` is the definition of done; CI runs `make ci`.
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

# Pinned release binaries from tools/tools.lock live in backend/bin (git-ignored); from backend/
# every tool is bin/<tool>. Heavy tools stay out of go.mod (see STACK.md, "Quality gates").
BIN  := $(CURDIR)/backend/bin
LOCK := $(CURDIR)/tools/tools.lock

SQLC          := $(BIN)/sqlc
GOLANGCI_LINT := $(BIN)/golangci-lint
GO_ARCH_LINT  := $(BIN)/go-arch-lint
SQUAWK        := $(BIN)/squawk
GOOSE         := $(BIN)/goose
VACUUM        := $(BIN)/vacuum
OASDIFF       := $(BIN)/oasdiff

# Tests: with TEST_DATABASE_URL set, every test runs, the PostgreSQL ones included; without it,
# -short skips those (the fast local loop). CI sets the URL and TEST_FLAGS=-shuffle=on, so a
# missing database fails the run there. -shuffle is CI-only because it turns off the test cache.
TEST_FLAGS ?= $(if $(TEST_DATABASE_URL),,-short)

# The contract is compared with this ref for breaking changes.
API_BASE ?= origin/main

# Extra golangci-lint flags. On a machine with little memory (golangci-lint killed, exit 137):
# make check LINT_FLAGS=--concurrency=1 GOMEMLIMIT=600MiB
LINT_FLAGS ?=

.PHONY: tools tools-check generate generate-check tidy-check fmt-check vet lint arch api-lint \
	api-breaking migration migrations-fix migrations-check migrations-lint test vuln fix-check check ci

tools: ## download the pinned binaries into backend/bin, SHA-256 checked
	bash scripts/install-tools.sh "$(LOCK)" "$(BIN)"

tools-check: ## Go and every pinned binary are the pinned versions
	bash scripts/check-tools.sh "$(LOCK)" "$(BIN)"

generate: ## regenerate the OpenAPI server and sqlc code
	cd backend && go generate ./...
	cd backend && $(SQLC) generate

generate-check: generate ## generated code is committed and current
	git diff --exit-code -- backend/internal/gen backend/internal/*/repository/*/sqlcgen backend/internal/*/sqlcgen

tidy-check: ## go.mod and go.sum are tidy
	cd backend && go mod tidy -diff

fmt-check:
	cd backend && test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

vet:
	cd backend && go vet ./...

lint:
	cd backend && $(GOLANGCI_LINT) run $(LINT_FLAGS) ./...

arch:
	cd backend && $(GO_ARCH_LINT) check

api-lint: ## the contract follows the rules in api/vacuum.yaml
	$(VACUUM) lint --ruleset api/vacuum.yaml --details --no-banner --no-style api/openapi.yaml

api-breaking: ## the contract has no breaking change against $(API_BASE) (CI; needs the ref fetched)
	@base=$$(mktemp); trap 'rm -f "$$base"' EXIT; \
	if ! git show "$(API_BASE):api/openapi.yaml" > "$$base" 2>/dev/null; then \
		echo "api-breaking: $(API_BASE) has no api/openapi.yaml; nothing to compare"; exit 0; fi; \
	$(OASDIFF) breaking "$$base" api/openapi.yaml --fail-on ERR

migration: ## make migration name=add_lawyers: a new SQL migration named with a timestamp
	@test -n "$(name)" || { echo "usage: make migration name=<snake_case_name>"; exit 1; }
	cd backend && $(GOOSE) -dir migrations create "$(name)" sql

migrations-fix: ## renumber timestamp-named migrations sequentially (right before merging)
	cd backend && $(GOOSE) -dir migrations fix

migrations-check: ## versions are unique; in CI none is still timestamp-named
	bash scripts/check-migrations.sh backend/migrations

migrations-lint: ## squawk: migration statements that lock, rewrite or break running code
	cd backend && $(SQUAWK) --config .squawk.toml migrations/*.sql

test:
	cd backend && go test -race $(TEST_FLAGS) ./...

vuln:
	cd backend && go tool govulncheck ./...

fix-check: ## modernizers have nothing left to rewrite
	cd backend && go fix -diff ./...

check: tools-check generate-check tidy-check fmt-check vet lint arch api-lint migrations-check \
	migrations-lint test vuln fix-check

ci: ## what CI runs: everything, the PostgreSQL tests in random order, and the breaking-change gate
	$(MAKE) check TEST_FLAGS=-shuffle=on
	$(MAKE) api-breaking
