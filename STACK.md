# Stack and architecture decisions

The single source of truth for every agent, skill, command and rule in this plugin.
If a file here disagrees with this document, this document wins; fix the file.

Versions were checked on **2026-11-12** (proxy.golang.org, registry.npmjs.org, release notes).
Before pinning a version in a project, check again: `go list -m -versions <module>`,
`npm view <pkg> version`. Never recommend a version from memory.

## 1. Shape of the system

```
repo/
  api/openapi.yaml            contract, OpenAPI 3.1 (the subset both generators handle), the source of truth for HTTP
  api/vacuum.yaml             the contract's lint rules (make api-lint)
  backend/                    Go module (go 1.27), strict Clean Architecture
  web/                        Next.js App Router (SEO pages + the user cabinet)
  tools/tools.lock            pinned release binaries: name, version, platform, SHA-256
  scripts/                    install-tools.sh, check-tools.sh, check-migrations.sh (see "Quality gates")
  Makefile                    `make generate`, `make check` (definition of done), `make ci` (what CI runs)
```

- **Contract first.** Every HTTP change starts in `api/openapi.yaml`. The Go server interface
  and the TypeScript client are generated from it. Hand-written DTOs that mirror the API are a bug.
- **Go owns the business.** Next.js renders pages and calls the Go API. It holds no business rules
  and never talks to PostgreSQL.
- **Login** only through Russian services: email, Yandex ID, VK ID or a phone number (details in
  §2 "Cross-cutting decisions").

Working example of everything in §2: `skills/go-clean-architecture/reference/`.

## 2. Backend: strict Clean Architecture, layers inside each module

```
backend/
  cmd/api/                    composition root of the API: main.go (config, pool, server, shutdown, `api migrate`),
                              wire.go (each module: store → use cases → handler)
  cmd/worker/                 composition root of the River workers and periodic jobs
  cmd/rivermigrations/        dev tool: writes River's migrations as goose files after a River upgrade
  internal/
    <module>/                 a business capability with its own schema: leads, lawyers, billing, ...
      CLAUDE.md               purpose, profile, public API, events, invariants, data, dependencies
      contract/               the module's public API: facade interfaces, DTOs, events, errors. Stdlib + kernel
      entity/                 entities, value objects, domain rules and errors. Imports: stdlib, kernel
      usecase/                one type per use case, its ports, input/output and view structs.
                              Imports: stdlib, kernel, own entity and contract
        usecasetest/          the in-memory store and the store contract suite (imported by tests only)
      repository/postgres/    adapter: implements the storage ports with pgx + sqlc, and the read side
        queries/*.sql         sqlc input
        sqlcgen/              sqlc output (generated, never edited)
      delivery/httpapi/       adapter: this module's part of openapi.StrictServerInterface
      delivery/worker/        adapter: this module's River workers and periodic schedules
      delivery/facade/        adapter: implements the contract's interfaces for other modules
      gateway/<other>/        adapter: implements a port of this module with <other>/contract or an external client
    <thin module>/            thin profile: one package (jobs or handlers, service code) + queries/ + sqlcgen/
    api/                      the /v1 handler: request validation against the contract, problems; apitest (tests)
    archtest/                 architecture tests: struct tags, Restore* callers, SQL schemas
    kernel/                   shared kernel, stdlib only: apperr (error kinds), small value types
    platform/                 technical infrastructure: config (Secret), logging, postgres (pool, migrate, errors,
                              pgtest), httpserver (middleware, problems, ClientIP, health), events, jobs, clock.
                              Never imports a business module
    gen/openapi/              oapi-codegen output (generated, never edited)
  migrations/                 goose SQL migrations, one sequence, embedded; River's migrations as goose files
  sqlc.yaml  .golangci.yml  .go-arch-lint.yml  .squawk.toml
```

### Module profiles

- **Full** if at least one holds: a lifecycle or state machine; money or legal obligations;
  invariants across several fields or aggregates; several entry points (HTTP, jobs, other
  modules); about 10 or more use cases. All the layers above.
- **Thin** otherwise (reference data, content, notifications): one package with its handlers or
  jobs, service functions and sqlc queries. It owns its schema like any module, and gets a
  `contract` package as soon as another module calls it.
- `/ecc:new-module` asks for the profile and why the capability is a module of its own; the
  module's `CLAUDE.md` records both.

### The dependency rule (enforced by tools, not by goodwill)

| Package | May import (project) | May import (third party) |
|---|---|---|
| `contract` | `kernel` | none |
| `entity` | `kernel` | none |
| `usecase` | own `entity`, own `contract`, `kernel` | none |
| `repository/postgres` | own `usecase`, own `entity`, own `sqlcgen`, `kernel`, `platform/*` (postgres, events) | pgx |
| `delivery/httpapi` | own `usecase`, own `entity`, own `contract`, `kernel`, `gen/openapi`, `platform/*` | oapi-codegen runtime |
| `delivery/worker` | own `usecase`, `kernel`, `platform/*` (jobs) | river |
| `delivery/facade` | own `usecase`, own `contract`, `kernel` | none |
| `gateway/<other>` | own `usecase`, `<other>/contract`, `kernel` | the external service's client, if any |
| thin module | own `sqlcgen`, other modules' `contract`, `kernel`, `platform/*` | pgx, river |
| `platform/*` | `kernel`, other `platform` packages | anything approved |
| test support (`usecasetest`, `apitest`, `pgtest`) | what its package needs | imported only by `_test.go` files |
| `cmd/*` | anything | anything |

Checked by `bin/go-arch-lint check` (project packages and vendors), depguard in `.golangci.yml`
(the standard packages `entity` and `usecase` may not use: `net/...`, `database/...`, `os`, `log`;
go-arch-lint always allows the standard library) and `internal/archtest` (no struct tags in inner
layers, `Restore*` only in repositories, a module's SQL only in its own schema).

Consequences:
- Inner layers never see `pgx`, `net/http`, `os`, generated OpenAPI types, JSON tags or SQL.
- A module reaches another only through its `contract`: synchronously through a port in the
  caller's `usecase`, implemented by the caller's `gateway/<other>` over the other's facade (built
  in `cmd`); asynchronously through events. The module graph has no cycles; it is written at the
  top of `.go-arch-lint.yml` and in the project's `docs/architecture/context-map.md`.
- Ports are declared where they are used (in `usecase`), are small (1–5 methods), and are
  implemented by adapters. No "interfaces package", no interface next to its only implementation.
- Data between modules: reference data (cities, legal topics) has one owner; others keep IDs,
  read-only copies fed by its events, or snapshots (the price at the time of sale). Personal data
  is not copied; a deletion propagates by event.
- Same package names in every full module; code that imports two modules uses aliases
  `<module><layer>` (`leadspg`, `leadsuc`, `leadshttp`).

### Cross-cutting decisions

| Concern | Decision |
|---|---|
| HTTP | `net/http` `ServeMux` (method + path patterns). Routes, params and request decoding generated by **oapi-codegen v2.8** (`std-http-server` + `strict-server`). No chi/gin/echo: the generator owns routing. Middleware on `/v1/`, outermost first: OpenTelemetry (`httpserver.Observe`) → request ID → API headers (`Cache-Control: no-store`, `nosniff`) → access log → recover → timeout → body limit (1 MiB) → `http.CrossOriginProtection`. `/livez`, `/readyz` |
| Request validation | Shape: the OpenAPI request validator (`oapi-codegen/nethttp-middleware`) on the whole `/v1/` subtree, with every field error at once and the security schemes; its errors are problems that name the field and the rule, never the submitted value. Invariants: entity constructors. Permissions and business rules: use cases |
| Errors | `kernel/apperr` kinds (`Invalid`, `NotFound`, `Conflict`, `Forbidden`, `Unauthorized`, `RateLimited`, `Unavailable`, `Internal`) with a stable `code` and an optional `Retry-After`. Wrap with `%w` and context; match with `errors.Is` / `errors.AsType`. Delivery maps kinds to statuses once, as RFC 9457 problems: a `type` URI per code, `errors[]` with JSON Pointers, each operation's codes listed in the contract (`x-problem-codes`) and checked by the tests. A new kind fails the `exhaustive` linter until every switch handles it |
| Use cases | One type per use case (`CreateLead{…}.Handle`) with only the ports it needs. Writes go through entities; reads for display go through query ports straight into views (CQRS-lite) |
| Transactions | One aggregate: `repo.Update(ctx, id, func(*entity.X) error)` with the transaction inside the adapter and the row locked `FOR NO KEY UPDATE`. Several writes, or a write plus an event: the `UnitOfWork` port; its callback gets repositories bound to the transaction, which run on it directly (no savepoints). Inside it only the module's own repositories: never another module, an external service or anything slow. Never hide a `pgx.Tx` in `context.Context`. Consistency across modules (selling a lead) is decided in an ADR when `billing` is designed |
| Database | **PostgreSQL 18**, **pgx v5** (`pgxpool`), **sqlc** with `sql_package: pgx/v5` (it parses PostgreSQL 17 syntax: the files it reads stay in that dialect), one schema per module (`leads.`, `billing.`). Two roles: the owner runs migrations, the application role has DML only. Sessions: `statement_timeout` 5 s, `transaction_timeout` 10 s, `idle_in_transaction_session_timeout` 30 s, time zone UTC, timestamps scanned as UTC |
| Migrations | **goose v3** SQL files, embedded, run by `api migrate` under the owner role (lock timeout 5 s, no statement timeout, fails on an INVALID index). New files are timestamped (`make migration`) and renumbered before merging (`make migrations-fix`). River's migrations are goose files written by `cmd/rivermigrations`. Expand → migrate → contract for breaking changes. An applied migration is never edited |
| Events and jobs | **River** (PostgreSQL). A module publishes `contract` events through its unit of work; `platform/events` inserts one job per subscriber with `InsertManyTx` in the same transaction (transactional outbox). Subscriber modules own their job kinds and register with `Subscribe`, called from `cmd`. The API process uses an insert-only client; `cmd/worker` runs the workers and periodic jobs. Job args: `json` tags, fields only added, IDs only, a golden-JSON test; workers end with `jobs.Outcome` (not found, invalid, conflict → cancel; the rest retried) and are idempotent. River's tables live in schema `river` |
| Idempotency | Retryable creates take an `Idempotency-Key` (draft-ietf-httpapi-idempotency-key-header-08, a quoted Structured Field String): same key and body → the first response; another body → 422. The key is claimed in the creating transaction, kept 24 h, then purged by a periodic job |
| IDs | stdlib `uuid` (Go 1.27): `uuid.NewV7()` for new rows. No `github.com/google/uuid` (depguard) |
| JSON | stdlib `encoding/json` (backed by json/v2 since Go 1.27); `encoding/json/v2` where its options are needed. Tags only in adapters and on job args |
| Time | Use cases get a `Clock` port; everything stored or sent is UTC, at microsecond precision (PostgreSQL's). Tests of timers and timeouts use `testing/synctest` |
| Context | `ctx context.Context` first in every I/O call. Context carries request-scoped data (request ID, principal), never dependencies |
| Config | Environment variables parsed once in `cmd` with **caarlos0/env v11** into a struct; secrets as `config.Secret` (redacted when printed), removed from the environment once read; inner layers receive plain values |
| Logging | `log/slog` JSON handler wrapped in a context handler that adds the request ID, trace ID and span ID; injected `*slog.Logger`, `…Context` methods, static messages, snake_case keys (sloglint). Never personal data, secrets, tokens or request bodies: such keys are banned, types holding them mask themselves. Errors logged once at the edge, by status |
| Telemetry | OpenTelemetry: `otelhttp` (spans and RED metrics by route template), `otelpgx` (spans by sqlc query name, never argument values; pool stats), `otelriver` (`traceparent` into job metadata, work spans linked to it). W3C trace context continues from Next.js; OTLP/HTTP export configured by the standard `OTEL_*` variables, off without an endpoint. Use cases stay vendor-free |
| DI | Manual constructor injection in the composition root. No `google/wire` (archived 2025-08), no `uber/fx` (reflection, hidden graph) |
| Shutdown | `signal.NotifyContext`; on SIGTERM readiness fails, a drain delay lets the load balancer notice, then `http.Server.Shutdown` with a grace period; River stops softly, then cancels |
| Login | Only Russian services: email by a one-time code or link (no passwords stored), Yandex ID, VK ID, or a phone code. Clients, lawyers and staff log in, each into their own cabinet (`/account`, `/cabinet`, `/admin`); Go authorizes by role. Accounts with the same provider-verified email are merged automatically (with the safeguards in `security-review`). OAuth with PKCE, codes server side. Server-side sessions in the `__Host-session` cookie (`HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`): 7 days idle, 30 days absolute; a fresh login before changing the phone, email or linked providers; a list of active sessions |
| Anti-spam | Public forms, all layers: per-IP limits at the proxy (nginx); a honeypot and a minimum fill time checked in the Server Action; Yandex SmartCaptcha verified by Go (built with the first real form); business limits in PostgreSQL (leads per phone, codes per number, a daily SMS budget); kill switches for lead intake and SMS. Captcha down: code sending fails closed, lead intake fails open with every limit kept. No JavaScript, so no token: the lead is accepted under stricter limits and marked unverified. The client IP comes only from `httpserver.ClientIP`, which trusts `X-Forwarded-For` only from the proxies |
| Security | Body limit, server timeouts, a deadline per request, `http.CrossOriginProtection` (Go 1.25) for cookie-authenticated mutations, security schemes in the contract (public operations say `security: []`), parameterized SQL only (sqlc), `govulncheck` in CI |

### Testing pyramid (backend)

| Layer | How |
|---|---|
| `entity` | Pure unit tests, table-driven, stdlib `testing` + `github.com/google/go-cmp/cmp`; fuzz for parsers |
| `usecase` | The use case on `usecasetest.Store`, an in-memory implementation of the module's storage ports; no database, no HTTP. `mockery` v3 only when a fake would be big |
| storage ports | One contract suite (`usecasetest.RunStoreContract`) runs on the fake and on PostgreSQL, so the fake can't drift |
| `repository/postgres` | The suite + adapter tests on a real PostgreSQL 18 (`TEST_DATABASE_URL`; CI: a service container, locally: any server; testcontainers-go if you want each run to start its own): one database per test, cloned from a migrated template, as the application role |
| `delivery/httpapi` | Requests through the production API stack (`apitest.Serve`) with the real use cases on the fake store; every response checked against the contract and its problem codes |
| Workers, wiring | Golden JSON of job args; cancel vs retry; `cmd/api` and `cmd/worker` end to end on PostgreSQL |
| Architecture | `internal/archtest`, `migrations/river_test.go` |
| End to end | Playwright from `web/` against the real backend |

`make test` runs `go test -race` with every test when `TEST_DATABASE_URL` is set and with `-short`
(the PostgreSQL tests skipped) when it isn't; without `-short`, a missing database fails the
tests. CI runs `make ci`: the database set and `-shuffle=on`.

### Quality gates (`make check`)

`make tools-check` (Go and the pinned tools are the pinned versions) → `generate-check` (go
generate and sqlc leave no diff) → `tidy-check` (`go mod tidy -diff`) → `fmt-check` → `vet` →
`lint` (golangci-lint v2; `LINT_FLAGS=--concurrency=1` on a small machine) → `arch`
(go-arch-lint) → `api-lint` (vacuum on the contract) → `migrations-check` (no version twice; in
CI no timestamp-named file left) → `migrations-lint` (squawk) → `test` → `vuln` (govulncheck) →
`fix-check` (`go fix -diff`, modernizers show nothing). `make ci` = `make check` with
`-shuffle=on` and the PostgreSQL tests, then `make api-breaking` (oasdiff against `origin/main`).

Tools are pinned in one of two ways:
- **`tool` directive** (`go get -tool`, run as `go tool <name>`) for small tools with light
  dependencies: oapi-codegen, govulncheck. The Go checksum database verifies them. Run `go mod tidy`
  after adding one; if it fails or `go.mod` grows by dozens of lines, make it a release binary.
- **Release binaries** for the rest, listed in `tools/tools.lock` (name, version, platform, SHA-256 of
  the download). `make tools` (`scripts/install-tools.sh`) downloads each one, checks the SHA-256
  before unpacking and installs it into `backend/bin/` (git-ignored; from `backend/` call
  `bin/<tool>`). `make tools-check` (`scripts/check-tools.sh`, the first step of `make check`) fails
  when Go or a tool is missing or a different version. These are sqlc (large dependency graph; the
  release binary runs PostgreSQL's parser as WebAssembly, `CGO_ENABLED=0`),
  golangci-lint (its maintainers recommend binaries), go-arch-lint (as a `tool` it breaks
  `go mod tidy` with ambiguous genproto imports, verified 2026-11-12), the goose CLI (only for
  `make migration` and `make migrations-fix`; migrations run through the goose library), squawk
  (the migration linter), vacuum (the contract linter) and oasdiff (the breaking-change gate).
  Nothing is compiled, so installing them can't run a 4 GB machine out of memory. A tool added later
  (mockery, if a fake grows too big) gets a lock line and a download rule in `install-tools.sh`.
- **Version policy for the lock**: the newest release that is at least 14 days old; earlier only
  for a security fix. To upgrade, take the SHA-256 from the publisher's checksums file or the GitHub
  release asset digest, check it against your own download, update the lock line, then
  `make tools && make check`. The lock pins linux-amd64; another platform needs lines of its own.

## 3. Frontend: Next.js for SEO

| Concern | Decision |
|---|---|
| Framework | **Next.js 16.4** App Router, **React 19.3**, **TypeScript 6.0** (see note below), **pnpm** |
| Rendering | Public SEO pages are Server Components, prerendered (`generateStaticParams`) and cached with Cache Components (`"use cache"`, `cacheLife`, `cacheTag`); revalidated by tag when data changes. The cabinets render at request time and use Client Components where interaction needs them |
| Why | Yandex and Google must get complete HTML on the first response. Client-only rendering is not acceptable for indexable pages |
| API client | **openapi-typescript** generates `web/src/lib/api/schema.ts`; **openapi-fetch** is the client; **openapi-react-query** + **TanStack Query 5** in Client Components. These three are in maintenance mode since 2026-09-19 (openapi-typescript issue #2874: no further releases, security fixes included); kept for now, the replacement (@hey-api/openapi-ts or orval) is an open decision |
| Calls | Server Components and Server Actions call the Go API server-side (internal URL) with an 8 s timeout, the proxy's request id and client IP, and the session cookie forwarded verbatim; `traceparent` is the active span's, injected by OpenTelemetry (never forwarded by hand). The browser calls Go only from the cabinets, through the same-origin `/api` prefix. Error bodies are RFC 9457 problems read defensively; clients branch on `code` |
| Forms | Server Actions built with **next-safe-action** from `lib/api/action-client.ts` (`actionClient` for public forms, `sessionActionClient` in the cabinets), with progressive enhancement: a public form's `stateAction` goes straight into React's `useActionState`, so lead forms work without JS (next-safe-action's `useStateAction` is not used for them: it dispatches through a client wrapper). **zod 4** parses the `FormData` as the action's input schema; validation errors (`handleValidationErrorsShape`) and Go's answers return the typed values (React 19 resets the form); exception text never reaches the browser; request bodies are object literals of the parsed fields; `Idempotency-Key` minted in the browser after hydration or by the action; honeypot and fill time checked in the action, SmartCaptcha token verified by Go (built with the first real form; without JS there is no token, and Go accepts such submissions under stricter limits); a retry path for deploy skew. The Go API stays the authority |
| Mutations | Public forms: Server Actions. Cabinet interactions: `$api.useMutation` with invalidation. Each feature keeps its query keys and cache tags in `cache.ts` |
| Cabinets | Three, one per role, under `app/(private)`: `/account` (clients), `/cabinet` (lawyers), `/admin` (staff). Rendered at request time (nothing personal cached or prerendered), `noindex` from the group layout, no analytics. Go authorizes every call by role; `proxy.ts` only checks that the session cookie is present |
| Cabinet data | Server prefetch with the server client + `HydrationBoundary`; the browser reads the same openapi-react-query key |
| Login | Go's: email with a one-time code or link (no passwords), Yandex ID, VK ID, a phone code; Go merges accounts with the same verified email. Login screens call Go through the same-origin `/api`, so Go's `Set-Cookie: __Host-session` reaches the browser directly; OAuth redirects and email links point to Go's endpoints under `/api`, never to a Next page, so their tokens never pass through Next. Sessions: idle 7 days, absolute 30 days. After login the user lands in their role's cabinet; `?next=` is followed only inside that cabinet |
| Structure | `src/app` (routes) → `src/features/<feature>` (`data.ts`, `actions.ts`, `cache.ts`, `schema.ts`, `components/`) → `src/components`, `src/lib`. Enforced by ESLint (`eslint-plugin-boundaries`): no feature imports another, `lib/api/server.ts` only from `data.ts`/`actions.ts`, `process.env` only in `lib/env*` |
| SEO | Metadata API (`generateMetadata`), canonical URLs, `sitemap.ts` (split with `generateSitemaps`), `robots.ts`, JSON-LD typed with `schema-dts`, `next/image`, `next/font`, Core Web Vitals budgets. Yandex: Webmaster verification, `Clean-param` for tracking params, complete SSR HTML |
| SEO main content | No `<Suspense>` or `loading.tsx` around the main content of an indexable page (H1, text, JSON-LD, the lead form): the page awaits its cached data. Next renders HTML-limited bots (YandexBot) at request time, and there a boundary sends its fallback first and the content later in a hidden streamed segment (verified with the YandexBot user agent). Suspense only around secondary parts |
| Request interception | `proxy.ts` (Next 16 name for middleware): redirects, locale, session-cookie presence for the three cabinet prefixes (`/account`, `/cabinet`, `/admin`; without the cookie a 307 to `/login?next=…`), slug shape checks for SEO routes (malformed → 404, case duplicates → 308) |
| Browser → API | Same origin: the browser calls `/api/...`, the reverse proxy forwards it to Go, so the HttpOnly session cookie and `http.CrossOriginProtection` work without CORS. `web/src/lib/api/server.ts` (internal URL, Server Components/Actions) and `web/src/lib/api/browser.ts` (`/api` prefix, cabinets) |
| Telemetry (web) | **@vercel/otel** in `src/instrumentation.ts`: Next's request and render spans and the fetch calls to Go, exported over OTLP configured only by `OTEL_EXPORTER_OTLP_ENDPOINT`, `_PROTOCOL`, `_HEADERS` (nothing is exported without them). `propagateContextUrls` lists only the Go API, so the trace continues in Go (otelhttp) and never reaches third parties. No personal data in spans: span names are route templates, a span processor strips query strings before export (decided), custom spans carry ids, never input values. The OpenTelemetry JS 2.x line, because `@vercel/otel` 2.x does not support 3.x |
| Unprerendered SEO pages | `ensureStatic = 'navigation'` (Next 16.4): a request for params not prerendered at build waits for the complete static result. It works with Cache Components alone; `partialPrefetching: true` is set because it is part of the recommended model (unset logs a warning) and only changes link prefetching |
| Styling and UI | Tailwind CSS 4, design tokens in `@theme`. Components: **shadcn/ui on Base UI** (shadcn's default since July 2026), generated into `web/src/components/ui/` and our code from then on |
| Lint/format | ESLint 10 flat config: `eslint-config-next` (core-web-vitals, typescript) + typescript-eslint `strictTypeChecked` (typed) + `eslint-plugin-jsx-a11y-x` (recommended, the ESLint 10 fork; eslint-config-next turns on only six jsx-a11y rules, as warnings) + `eslint-plugin-boundaries` + core `no-restricted-imports`/`no-restricted-syntax` for the APIs Next 16 and Cache Components removed (each message names the fix); Prettier 3; knip for unused code. `next lint` no longer exists |
| Tests | Vitest 5 + Testing Library (components, hooks, pure logic) with MSW 3 + openapi-msw; the form's request body validated against `api/openapi.yaml` (Ajv); action clients and safe actions called directly (mocked `server-only` and `next/headers`); Playwright 1.65 + axe (WCAG 2.2 tags) for journeys, lead forms also without JS, each cabinet with its role's session; crawler checks request SEO routes as YandexBot (request-time render) and Googlebot (prerendered page): main content complete in `<main>`, no streaming placeholders, real 404s. `next build` type-checks tests and e2e |
| Agent docs | `web/AGENTS.md` (create-next-app writes it, `next dev` keeps its block current) points agents to the docs bundled in `node_modules/next/dist/docs`; `web/CLAUDE.md` imports it with `@AGENTS.md`, because Claude Code reads AGENTS.md by itself only when no CLAUDE.md exists up the tree. The `nextjs-patterns` skill holds only our decisions |
| Not used | Full Feature-Sliced Design, React taint APIs, `exactOptionalPropertyTypes` (fights generated optional fields), Storybook, a monorepo |

## 4. Versions checked 2026-11-12

Go 1.27.4 · pgx v5.13.0 · goose v3.30.1 (library) · oapi-codegen v2.8.2 (OpenAPI 3.1) ·
oapi-codegen/runtime v1.9.0 · River v0.51.2 · testcontainers-go v0.45.0 · mockery v3.9.1 ·
caarlos0/env v11.4.2 · govulncheck (x/vuln) v1.9.0 · OpenTelemetry Go v1.47.0 (2026-10-02) · otelhttp v0.72.0
(2026-10-02) · otelpgx v0.13.0 (2026-10-08) · otelriver v0.12.0 (2026-07-24) · kin-openapi v0.153.0 ·
oapi-codegen/nethttp-middleware v1.3.1 · go-cmp v0.7.0

`tools/tools.lock` (14-day policy, as of 2026-11-13): golangci-lint 2.15.0 · sqlc 1.31.1 ·
go-arch-lint 1.19.1 · goose CLI 3.29.0 (3.30.x is younger than 14 days) · squawk 2.71.0 ·
vacuum 0.36.0 · oasdiff 1.35.0

Node.js 26.13.1 (LTS; a security release, so newer than 14 days) · pnpm 12.16.1, both pinned in the
repository's `.tool-versions` (asdf); Next.js 16.4.6 needs Node >= 20.9.0.

Next.js 16.4.6 · React 19.3.1 · TypeScript 6.0.3 · openapi-typescript 7.13.0 · openapi-fetch 0.17.0 ·
openapi-react-query 0.5.4 · TanStack Query 5.107.1 · zod 4.6.7 · Tailwind 4.3.5 · ESLint 10.14.0 ·
Prettier 3.10.1 · Vitest 5.1.0 · Playwright 1.65.0 · schema-dts 2.1.0

Web helpers, exact versions (checked 2026-11-13; the newest release at least 14 days old, except where a pin above
fixes the version): eslint-config-next 16.4.6 and typescript-eslint 8.72.2 (match Next and the copy
eslint-config-next resolves) · eslint-plugin-boundaries 7.4.0 (2026-10-27) · eslint-plugin-jsx-a11y-x 0.2.0
(2026-05-10) · eslint-config-prettier 10.1.8 · @tailwindcss/postcss 4.3.5 (matches tailwindcss) · vite 8.3.5
(2026-10-21) · @vitejs/plugin-react 6.1.2 (2026-10-05) · jsdom 30.1.3 (2026-10-15) · @testing-library/react 16.3.3
· @testing-library/dom 10.4.2 · msw 3.0.3 (2026-10-10) · openapi-msw 2.1.1 (2026-10-27) · ajv 8.20.0 · yaml 2.9.1 ·
knip 6.44.0 (2026-10-29) · @axe-core/playwright 4.14.0 (2026-10-15) · server-only 0.0.1 · @types/node 26.6.6 ·
@types/react and @types/react-dom 19.3.0. shadcn/ui: shadcn 4.23.0 (2026-10-27) · @base-ui/react 1.9.0 ·
class-variance-authority 0.7.1 · cn 0.5.1 (2026-10-30) · lucide-react 1.65.0 (2026-10-30) · tw-animate-css 1.4.0
(the CLI writes `^` ranges: pin them). Tailwind goes through `@tailwindcss/postcss` as the bundled Next docs show;
create-next-app 16.4.6 scaffolds `@tailwindcss/turbopack` instead.

Server Actions and telemetry (checked 2026-11-13, same 14-day rule): next-safe-action 8.7.3 (2026-09-07; peers
next >= 14, react >= 18.2; zod is not a peer, it takes any Standard Schema) · @vercel/otel 2.2.0 (2026-10-30; 2.3.0
from 2026-11-05 is too new) · @opentelemetry/api 1.9.1 (2026-03-25; the OpenTelemetry JS 2.x packages need api
< 1.10) · @opentelemetry/api-logs, sdk-logs, instrumentation 0.223.0 (2026-10-06) · @opentelemetry/resources,
sdk-metrics, sdk-trace-base 2.12.0 (2026-10-06). `@vercel/otel` 2.x peers: `@opentelemetry/api` < 2, api-logs,
sdk-logs and instrumentation 0.2xx, resources, sdk-metrics and sdk-trace-base 2.x. OpenTelemetry JS 3.0
(2026-10-20) deprecated `@opentelemetry/sdk-trace-node` and `@vercel/otel` 2.x does not support it: stay on the 2.x
line until it does.

`web/package.json` also pins Node and pnpm in `devEngines` (`runtime` `^26.13.1`, `packageManager` `^12.16.1`,
`onFail: "error"`): pnpm 12 then refuses to install or run scripts on other versions. It does not enforce the root
project's `engines`.

**TypeScript 7.0 (native compiler, 7.0.2) is deferred**: it ships no JavaScript API, so typescript-eslint
(8.72.2, peer `typescript >=4.8.4 <6.1.0`) and openapi-typescript (7.13.0, builds output with the TS API)
don't work with it. Pin `typescript@6.0.3`; openapi-typescript prints a peer warning (`^5.x`) that is expected.
Revisit when both support TS 7.

Considered and not chosen: chi v5.5 (routing is generated), ogen v1.24 (faster, built-in OTel, but a
smaller community; a valid swap if oapi-codegen hits a limit), Atlas (declarative migrations; goose is
simpler to review), GORM (hides SQL, fights the dependency rule), google/wire (archived), testify
(Google Go style prefers plain `if` + `cmp.Diff`; acceptable if a team already uses it).

## 5. What changed in Go that the code must use

| Since | Use | Instead of |
|---|---|---|
| 1.22 | `mux.HandleFunc("GET /leads/{id}", h)`, `r.PathValue("id")` | third-party routers |
| 1.22 | `for i := range n` | `for i := 0; i < n; i++` |
| 1.24 | `t.Context()`, `b.Loop()`, `tool` directive in `go.mod` | `context.Background()` in tests, `b.N`, `tools.go` |
| 1.25 | `sync.WaitGroup.Go`, `testing/synctest`, `http.CrossOriginProtection` | manual `Add/Done`, sleeping tests, hand-made CSRF checks |
| 1.26 | `errors.AsType[*T](err)`, `new(expr)`, `slog.NewMultiHandler`, `go fix` modernizers | `var t *T; errors.As(err, &t)`, helper `ptr()` funcs |
| 1.27 | stdlib `uuid`, `encoding/json/v2`, `httptest.NewTestServer`, `synctest.Sleep`, `strings.CutLast` | `google/uuid`, ad-hoc JSON options |
| 1.27 | `go vet` flags `%w` with `*T` when `T` has a value-receiver `Error()` (pointer-receiver error types like `*apperr.Error` are fine); `stdversion` vet runs in `go test` | — |
