---
description: Fix Go backend build failures (compile, go vet, golangci-lint, go-arch-lint, go.mod, stale sqlc/oapi-codegen output) with minimal changes via the go-build-resolver agent.
---

# /ecc:go-build

Delegate to the **go-build-resolver** agent. Arguments, if any, narrow the scope (a package
path such as `./internal/leads/...`, or a pasted error).

## What happens

1. Run, from `backend/`, stopping at the first failing stage:
   `go build ./...` → `go vet ./...` → `go test -run '^$' ./...` (compiles tests) →
   `bin/go-arch-lint check` → `bin/golangci-lint run ./...`.
2. If the errors point at generated code, regenerate first (`make generate`; after a River
   upgrade, `go run ./cmd/rivermigrations`) and re-run, because stale output causes most
   "undefined" and "does not implement" errors after contract or query changes.
3. Fix errors one package at a time, lowest layer first (`kernel` → `entity` → `usecase` →
   adapters → `cmd`), re-running the build after each fix.
4. Verify with `go build ./... && go vet ./... && go test -race ./...`, then `make check`.
5. Report fixed errors (`file:line`, cause, fix), files changed, and anything left for a human.

## Rules the agent follows

- Smallest change that fixes the cause. No refactoring, no new features.
- Never: `//nolint`, `-vet=off`, editing `sqlcgen/` or `gen/openapi/`, widening
  `.go-arch-lint.yml`, deleting go.sum lines, changing test expectations to pass.
- Go 1.27 vet checks are real bugs: `stdversion` (API newer than the `go` directive) and
  `%w wants operand of error type T, not pointer type *T`.
- Stops and reports after 3 failed attempts on one error, or when the fix needs an architectural
  change (new port, moved types, contract or migration change).

## Not for

- Failing test assertions: use `/ecc:go-test` (or `/ecc:tdd` for new behavior).
- Next.js/TypeScript build errors: use `/ecc:build-fix`.
- Database design questions: use the `database-reviewer` agent.
