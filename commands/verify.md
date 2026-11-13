---
description: Run the full quality gate for the change (make check - pinned tools, generate-check, tidy, fmt, vet, golangci-lint, go-arch-lint, contract lint, migration checks, race tests, govulncheck, go fix; web typecheck, lint, tests, build) and report PASS/FAIL with real output.
argument-hint: "[quick | full | pre-pr]"
---

Follow the `verification-loop` skill in mode: $ARGUMENTS (if empty: full).

- Prefer the project's own targets (`make check`, `pnpm check`) when they exist; they are the
  definition of done. `pre-pr` runs `make ci` (shuffled tests, the breaking-change gate) when a
  database is available.
- Say whether the Go tests ran with PostgreSQL or with `-short`; a change to SQL, migrations,
  adapters, workers or wiring needs the PostgreSQL run.
- Run checks for the halves that changed (`git diff --name-only HEAD`), all of them for `pre-pr`.
- Report each check with its real result; on failure, quote the first relevant lines of output.
- Never make a check pass by weakening it (skipping tests, loosening lint, `//nolint` without a
  reason). If a check looks wrong, stop and say so.
