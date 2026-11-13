---
description: Fix a failing build with minimal changes — routes Go errors to go-build-resolver and TypeScript/Next.js errors to build-error-resolver, one error group at a time.
argument-hint: "[go | web | the failing command]"
---

Fix the build: $ARGUMENTS (if empty: detect which half fails).

1. Reproduce: run the failing command (or `cd backend && go build ./... && go vet ./...`, and
   `cd web && pnpm exec tsc --noEmit && pnpm build`) and capture the output.
2. Go errors → **go-build-resolver** agent. TypeScript/Next.js errors → **build-error-resolver**
   agent. Generated code out of date → regenerate (`go generate ./...`, `bin/sqlc generate`,
   `pnpm generate:api`), never hand-edit it.
3. Fix one group of related errors at a time, rebuild after each, stop when green.
4. No architectural changes, no weakened checks, no `//nolint`/`@ts-ignore` to silence errors.
   If the right fix is a design change, stop and explain it.
