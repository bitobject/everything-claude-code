---
paths:
  - "backend/**/*.go"
  - "backend/go.mod"
---
# Go backend rules

Detail: skills `go-clean-architecture`, `golang-patterns`, `golang-testing`, `postgres-patterns`.

- **Layers.** A full module has `internal/<module>/{contract,entity,usecase,repository/postgres,
  delivery/{httpapi,worker,facade},gateway/<other>}`; a thin module is one package with its
  queries. Plus `internal/kernel`, `internal/platform`, `internal/api`, `cmd`. `entity` and
  `usecase` import only the standard library (no `net/...`, `database/...`, `os`, `log`),
  `kernel` and their own module's `entity`/`contract`: go-arch-lint, depguard and
  `internal/archtest` enforce it. Never widen `.go-arch-lint.yml` to make code pass.
- **Use cases**: one type per use case (`CreateLead{…}.Handle`) with only the ports it needs.
  Writes go through entities; reads for display go through query ports into views.
- **Ports** are declared by the consumer (`usecase`), small, and use the module's types, never
  `pgx.Tx`, `*http.Request`, `openapi.*` or `sqlcgen.*`.
- **Modules** meet only through `<other>/contract`: a facade called from a `gateway/<other>`
  adapter (sync) or an event published in the same transaction (async, `platform/events`). No
  module reads another module's tables; nothing inside `Do`/`Update` calls another module.
- **Generated code** (`internal/gen/openapi`, `*/sqlcgen`, `*_river_*.sql`) is never edited:
  change `api/openapi.yaml` or `queries/*.sql` and regenerate; River's migrations come from
  `go run ./cmd/rivermigrations`.
- **Errors**: `apperr` values with a kind and a stable code; wrap with `%w` and the identifier
  (`"assign lead %s: %w"`); check with `errors.Is` / `errors.AsType`. Handlers return errors;
  `httpserver.ErrorWriter` maps them. Log once, at the edge. Corrupt stored data is internal.
- **Context** first in every I/O function; `context.Background()` only in `cmd`; `t.Context()` in tests.
- **Time and IDs**: use cases take a `Clock` port (UTC, microseconds); IDs are stdlib
  `uuid.NewV7()` (Go 1.27), never `github.com/google/uuid`.
- **Logging**: injected `*slog.Logger`, `…Context` methods, IDs only; never phones, names,
  emails, problem texts, tokens, request bodies (sloglint rejects such keys). Types holding
  personal data or secrets mask themselves (`LogValue`, `config.Secret`).
- **Telemetry**: spans and metric labels are named by route templates and sqlc query names, never
  raw paths, IDs or personal data; never `otelpgx.WithIncludeQueryParameters`; every River client
  gets `Middleware: jobs.Middleware()`; `telemetry.Setup` runs in `cmd` before the pool and the
  clients; inner layers don't import OpenTelemetry.
- **Jobs**: args with `json` tags, fields only added, IDs only, a golden-JSON test; workers end
  with `jobs.Outcome(err)` and are idempotent.
- **Dependencies**: standard library first; a new module in `go.mod` is the user's decision. Heavy
  tools are pinned binaries in `bin/`, not `tool` directives.
- **Tests first** at the layer that owns the behavior: entity unit tests; use cases on the fake
  store (`usecasetest`), whose behavior the store contract suite pins on PostgreSQL too;
  repositories on a real PostgreSQL (`pgtest`); handlers through `apitest.Serve` with the real use
  cases. `-race`, no sleeps; PostgreSQL tests skip only under `-short`.
- **Done** = `/ecc:verify` (`make check`) green with real output, saying whether the PostgreSQL
  tests ran.
