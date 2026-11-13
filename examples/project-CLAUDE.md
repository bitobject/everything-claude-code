# <project name>

<!-- Loaded into every session: every line costs context. Budget: ~120 lines.
     Area rules → .claude/rules/ (paths: frontmatter). One backend module → its own CLAUDE.md.
     Procedures → skills. Stack decisions → the ecc plugin's STACK.md (don't repeat them here;
     where this project differs, an ADR in docs/adr/ says so). -->

## What this is
<Two or three sentences: who uses it, what it does, what makes money.>

## Stack
Go 1.27 backend (strict Clean Architecture, modules with layers inside) + Next.js 16 App Router
(SEO pages + cabinet) + PostgreSQL. HTTP contract: `api/openapi.yaml` (OpenAPI 3.1), Go server
and TS client generated from it. Defaults: ecc plugin `STACK.md`; this project's own decisions
and deviations: `docs/adr/` (an accepted ADR wins over `STACK.md`).

## Layout
- `api/openapi.yaml` — the contract; change it first.
- `backend/cmd/api`, `backend/cmd/worker` — composition roots (wiring only).
- `backend/internal/<module>/{contract,entity,usecase,repository/postgres,delivery/{httpapi,worker,facade},gateway}`
  — full modules; a thin module is one package with its queries. Modules: <list them with one
  line each and their profile>. Each has a `CLAUDE.md` (purpose, profile, public API, events,
  invariants, data, dependencies); it loads when Claude touches that module. Another module is
  reached only through its `contract` package.
- `backend/internal/api` — request validation against the contract, problems for rejected
  requests. `backend/internal/platform/*` — pool, migrations, server and middleware, events bus,
  jobs, logging, clock. `backend/internal/kernel` — apperr.
- `backend/migrations/` — goose SQL, one sequence (new files timestamped, renumbered by
  `make migrations-fix` before merging; River's written by `go run ./cmd/rivermigrations`).
  `backend/sqlc.yaml` — one entry per module.
- `web/src/app/(public)` — indexable pages; `web/src/app/(cabinet)` — logged-in UI;
  `web/src/lib/api/` — generated schema + server/browser clients.
- `docs/plans/<slug>.md` — plans (acceptance criteria, steps); `docs/adr/` — decisions (MADR 4.0);
  `docs/architecture/` — subdomains, context map, C4, glossary.

## Commands
- `make tools` — pinned release binaries from `tools/tools.lock` into `backend/bin/`, SHA-256 checked
  (sqlc, golangci-lint, go-arch-lint, goose, squawk, vacuum, oasdiff); once per clone and after the
  lock changes.
- `make generate` — OpenAPI server + sqlc; `cd web && pnpm generate:api` — TS client.
- `make check` — **definition of done** for the backend; `cd web && pnpm check` for the web.
  `make ci` — what CI runs (shuffled tests on PostgreSQL, the contract's breaking-change gate).
- One Go test: `cd backend && go test -race -run TestName ./internal/leads/usecase/`.
- PostgreSQL tests need `TEST_DATABASE_URL` (the schema owner's DSN: `<how to get one>`); without
  it `make test` adds `-short` and skips them; a run without `-short` and without it fails.
- `make run` / `cd web && pnpm dev` — local servers (<ports>).

## Workflow
- Non-trivial work: `/ecc:plan` (writes `docs/plans/<slug>.md`) → user says "go" → per step:
  `/ecc:tdd docs/plans/<slug>.md step <n>` → `/ecc:verify` → `/ecc:code-review` (checks the plan).
- New module: `/ecc:new-module <name> <capability>`. API change: the `openapi-contract-first` skill.
- Ask (AskUserQuestion, options + recommendation) when the spec, docs or code don't decide something.
- Minor choices made without asking go in an **Assumptions** section at the end of the reply.

## Conventions
Only what an agent would get wrong without being told. Format: rule — why.
- (none yet)

## Data protection
Personal data of users (phones, names, problem descriptions) never appears in logs, URLs,
analytics, test fixtures or prompts. Use generated data. Don't read `.env*` or secrets.

## Where knowledge goes
1. Mechanically checkable → a test, lint rule (`.golangci.yml` forbidigo), `.go-arch-lint.yml`, or a hook.
2. One area → `.claude/rules/<area>.md` with `paths:`.
3. One backend module → its `backend/internal/<module>/CLAUDE.md` (same headings in every module).
4. Procedure → `.claude/skills/<name>/SKILL.md`.
5. Short, everywhere → Conventions above.
6. Decision with trade-offs → `docs/adr/NNNN-<title>.md` (MADR 4.0, from `docs/adr/TEMPLATE.md`).
