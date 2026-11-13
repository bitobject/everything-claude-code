---
description: Find behavior that no test exercises (Go backend and web) by reading coverage as a signal, then add behavior-focused tests where a real rule is untested. No percentage targets.
---

# /ecc:test-coverage

Use coverage to find code nobody tested, then decide, branch by branch, whether a behavior is
missing a test. Arguments narrow the scope (`./internal/leads/...`, `web/src/app/cabinet`).

## Go backend

```bash
cd backend
go test -race -coverprofile=cover.out -coverpkg=./internal/... ./...
grep -v -e /sqlcgen/ -e /gen/openapi/ cover.out > cover.app.out   # generated code is not ours to test
go tool cover -func=cover.app.out | sort -k3 -n | head -40         # least covered functions
go tool cover -html=cover.app.out -o cover.html                    # read the red lines
```

`-coverpkg` counts lines exercised by tests in other packages (adapters are often exercised by
integration tests). Repository tests need PostgreSQL (`TEST_DATABASE_URL`); without it the numbers
for adapters are meaningless, so say which mode ran.

Read uncovered code in this order, because that is where untested rules hurt most:

1. `entity`: an uncovered branch is usually an untested invariant or state transition → add a
   table row.
2. `usecase`: uncovered error branches (not found, conflict, storage failure, a failed unit of
   work), permission checks → tests on the fake store.
3. `repository/postgres`: unmapped constraint errors, locking paths, NULL handling → the store
   contract suite or adapter tests via `pgtest.New(t, migrations.FS)`.
4. `delivery/httpapi`: error kinds without a status/problem test, malformed input → tests through
   `apitest.Serve`.

For each uncovered block decide and say which:
- **Missing behavior test** → write it (patterns in the `golang-testing` skill), one behavior per
  test, asserting outcomes.
- **Dead code** → propose deletion (`refactor-cleaner` agent) instead of testing it.
- **Not worth a unit test** (`cmd` wiring, `String()` methods, log lines) → leave it; say so.

## Web

```bash
cd web
pnpm vitest run --coverage   # needs the @vitest/coverage-v8 package
```

Same reading: untested branches in form handling, zod schemas, Server Action input mapping and
conditional rendering get Testing Library tests; user journeys belong to Playwright (`/ecc:e2e`),
which coverage does not measure.

## Report

```text
Scope: ./internal/leads/...
Before: entity 92.1%  usecase 71.4%  postgres 68.0%  httpapi 80.2%   (signal only)

Added
  usecase  TestCloseUnknownLeadIsNotFound         — error branch had no test
  usecase  TestCloseStoresNothingWhenPublishFails — failed unit of work untested
  postgres TestCreateDuplicatePhoneIsConflict     — 23505 mapping untested
Left uncovered (by decision)
  cmd/api/main.go wiring; entity.Status.String
Proposed deletion
  usecase.legacyAssign (no callers)

After: usecase 89.3%  postgres 81.5%
```

Never add tests that only execute code without asserting behavior, and never set or chase a
coverage threshold.
