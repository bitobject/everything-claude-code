---
description: Run, write or fix Go backend tests the project's way - layer-appropriate tests (entity, usecase on the fake store, the store contract suite and repository/postgres on real PostgreSQL, delivery/httpapi through the production API stack against the OpenAPI contract), -race, failures diagnosed to the cause.
---

# /ecc:go-test

Work on Go tests following the `golang-testing` skill. Arguments choose the mode:

- no argument or a package path (`./internal/leads/...`): **run** and diagnose failures;
- `write <package or function>`: **add** missing behavior tests to existing code;
- `fix <TestName>`: **repair** a failing or flaky test.

For new behavior, use `/ecc:tdd` instead: tests come first there.

## Run

```bash
cd backend
go test -race ./...                                  # or the given packages
```

The PostgreSQL tests (repository, end-to-end, worker wiring) need `TEST_DATABASE_URL`: the schema
owner's DSN on PostgreSQL 18, a role with `CREATEDB` (README of the reference). With `-short`
they skip; without `-short` and without the URL they fail. Say which mode ran; never skip or
disable them any other way. CI adds `-shuffle=on`.

For each failure, read the test and the code under test, then classify and say which:

| Class | Signal | Action |
|---|---|---|
| Code bug | test states the intended behavior; code violates it | fix the code (smallest change) |
| Test bug | test asserts an implementation detail, wrong expectation, shared state | fix the test; keep its intent |
| Flaky | passes on rerun; depends on order, time, goroutine scheduling | reproduce with `-count=20 -shuffle=<seed>`, fix the cause (synctest, per-test database, no sleeps) |
| Environment | Docker/PostgreSQL unreachable, missing tool | report with the exact error |
| Stale generated code | build errors in sqlc or oapi-codegen types | regenerate, or hand off to `/ecc:go-build` |

Never change an assertion only to make it pass: if the expected behavior changed, say so and
point to the requirement.

## Write

1. Find untested behavior: `go test -coverprofile=cover.out ./pkg && go tool cover -func=cover.out`,
   then read the uncovered branches and ask which rule each protects.
2. Choose the layer's test type: `entity` table-driven (+ fuzz for parsers); `usecase` on
   `usecasetest.Store` with a fixed `Clock` and injected IDs; a storage behavior → a subtest in the
   store contract suite (it runs on the fake and on PostgreSQL); `repository/postgres` with
   `pgtest.New(t, migrations.FS)`; `delivery/httpapi` through `apitest.Serve` with the real use
   cases; job args → golden JSON.
3. One behavior per test or table row, `t.Context()`, `cmp.Diff(want, got)`, no sleeps, no mocks
   of pgx or `http.Client`.
4. Run the new tests and show them failing once against a deliberately broken line (or explain
   why the behavior already holds), so they are known to detect the bug.

## Fix a flaky test

```bash
go test -race -count=50 -run '^TestName$' ./pkg
go test -race -shuffle=on -count=5 ./pkg     # order dependence; rerun with the printed seed
```

Usual causes: real time (`time.Sleep`, `time.Now()` in assertions) → `testing/synctest` or a fake
`Clock`; shared database rows → `pgtest.New(t, migrations.FS)` per test; map iteration order → sort before
comparing; goroutines not awaited → `sync.WaitGroup`/channels; `t.Parallel()` with package
globals or `t.Setenv`.

## Report

```text
Run: go test -race ./internal/leads/...  (with PostgreSQL)
PASS 41 | FAIL 1 | packages 5

FAIL TestAssignTwiceIsConflict (internal/leads/usecase)
Class: code bug — Assign ignores current status
Fix: entity.Lead.Assign checks StatusNew (lead.go:88)
Rerun: PASS
```
