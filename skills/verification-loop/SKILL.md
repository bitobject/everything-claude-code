---
name: verification-loop
description: The quality gate for this stack — what to run, in which order, for the Go backend (make check - pinned tools, generate-check, tidy, gofmt, vet, golangci-lint, go-arch-lint, contract lint, migration checks, race tests with or without PostgreSQL, govulncheck, go fix; make ci adds shuffled tests and the breaking-change gate) and the Next.js web (typecheck, lint, tests, build), and how to report. Use before saying a change is done, before a commit or PR, and after refactors.
---

# Verification loop

"Done" means the gate passed on the real code, and the report shows the real output.
If the project has `make check` / `pnpm check`, those are the gate — run them, not a subset.

## Modes

| Mode | When | Runs |
|---|---|---|
| `quick` | while iterating | build + vet/typecheck + the tests of the packages you touched |
| `full` (default) | before saying "done" | everything below for each half that changed |
| `pre-pr` | before a PR | `full` for both halves + generated-code check + security scans + diff review |

## Backend, in order — stop at the first failure and fix it

`make check` from the repository root runs exactly this (reference `Makefile`):

```bash
make tools-check        # Go and backend/bin tools are the pinned versions (fix: make tools)
make generate-check     # OpenAPI server + sqlc regenerated, no diff
make tidy-check         # go mod tidy -diff
make fmt-check          # gofmt -l
make vet                # go vet ./...
make lint               # golangci-lint (.golangci.yml); LINT_FLAGS=--concurrency=1 on a small machine
make arch               # go-arch-lint: the dependency rule
make api-lint           # vacuum: the contract rules (api/vacuum.yaml)
make migrations-check   # no migration version twice; in CI, no timestamp-named file left
make migrations-lint    # squawk: locks, rewrites, unsafe statements
make test               # go test -race; with TEST_DATABASE_URL every test, without it -short
make vuln               # govulncheck: reachable vulnerabilities
make fix-check          # go fix -diff: modernizers have nothing left
```

`make ci` (what CI runs) = `make check` with `-shuffle=on` and the PostgreSQL tests (CI sets
`TEST_DATABASE_URL`, so a missing database fails), then `make api-breaking` (oasdiff against
`origin/main`).

The report says which test mode ran: **with PostgreSQL** (every test) or **`-short`** (the
repository, end-to-end and worker-wiring tests skipped). A change to SQL, migrations, adapters,
workers or wiring is not done on `-short`: run it with a database (README of the reference) or
say plainly that those tests didn't run.

## Web (`web/`), in order

`pnpm check` is the web's definition of done (`coding-standards` skill): typecheck
(`next typegen && tsc --noEmit`), lint (typed rules, boundaries, accessibility), format, tests,
knip, then `pnpm build` (it calls the Go API: start it first). Around it:

```bash
pnpm generate:api && git diff --exit-code src/lib/api/   # client matches the contract
pnpm check
pnpm e2e                                   # pre-pr, or when a user journey changed (production build + Go)
```

On a machine with little memory, run the heavy steps one at a time and give Node more heap if a
step is killed (`NODE_OPTIONS=--max-old-space-size=1024`); say so in the report.

## Always

- `git status --short` and `git diff --stat HEAD`: only intended files changed; no `.env`,
  dumps, `bin/`, `node_modules`, coverage output.
- No debug output left (`fmt.Print*`, `console.log`) — the Stop hook checks added lines.
- No secrets: `git diff HEAD | grep -nEi '(secret|token|password|api[_-]?key|PRIVATE KEY)'`.
- Tests that changed expectations: each one is explained by a requirement change, not by
  "made it pass".

## Report

```
VERIFICATION (full): FAIL

backend  tools ✓  generate ✓  tidy ✓  fmt ✓  vet ✓  lint ✗ 2  arch ✓  api ✓  migrations ✓  test ✓ (-race, PostgreSQL)  vuln ✓  fix ✓
web      api ✓  typecheck ✓  lint ✓  format ✓  test ✓ 61/61  knip ✓  build ✓  e2e –

Failures:
- golangci-lint: internal/leads/usecase/assign.go:31:2: forbidigo: use of `time.Now` forbidden …
  → inject the Clock port (fixing now)
```

Quote real output for every failure. Don't round "2 lint warnings" to "lint OK", and never make
a check pass by weakening it (skips, `//nolint` without a reason, loosened config, deleted tests).
If a check itself looks wrong, stop and ask.
