# Reference

A small, compiling slice of the architecture in `../SKILL.md`: two modules, `leads` (full profile:
entity, use cases, ports, PostgreSQL adapter, HTTP and job adapters, public contract) and
`notifications` (thin profile, reacts to a `leads` event), the OpenAPI contract, goose migrations,
sqlc queries and two composition roots (`cmd/api`, `cmd/worker`). Skills and agents point here
instead of pasting code. Each module's `CLAUDE.md` shows the module template filled in.

## Verified
On 2026-11-13 with Go 1.27.4, pgx v5.13.0, River v0.51.2, goose v3.30.1 (library),
oapi-codegen v2.8.2, kin-openapi v0.153.0, nethttp-middleware v1.3.1, govulncheck v1.9.0 and the
binaries pinned in `tools/tools.lock` (golangci-lint 2.15.0, sqlc 1.31.1, go-arch-lint 1.19.1,
goose CLI 3.29.0, squawk 2.71.0, vacuum 0.36.0, oasdiff 1.35.0), on PostgreSQL 18:
- `make ci` passes: tools-check, generate-check, `go mod tidy -diff`, gofmt, `go vet`,
  golangci-lint (0 issues), go-arch-lint, vacuum (0 errors, 0 warnings), migrations-check, squawk,
  `go test -race -shuffle=on` with every PostgreSQL test, govulncheck, `go fix -diff`;
  `make test` without a database passes too (`-short`).
- Deliberate violations fail the checks: `net/http` and `os` in a use case (depguard), `time.Now`
  in a use case (forbidigo), a `phone` log key (sloglint), pgx in a use case, a fake imported by
  production code and another module's internals imported (go-arch-lint), a struct tag in an inner
  layer, `entity.Restore*` outside a repository and a query on another module's schema
  (`internal/archtest`), a missing or edited River migration (`migrations/river_test.go`; then
  `go run ./cmd/rivermigrations` wrote the missing one with River's SQL unchanged).
- `make api-breaking` against the previous version of this contract reports 3 breaking changes
  (the new required `Idempotency-Key`, sessions on two operations) and fails.
- `make tools` downloads vacuum and oasdiff and checks their SHA-256 (earlier: a wrong SHA-256
  stops the install and leaves nothing in `backend/bin/`).

## Files
| Path | Shows |
|---|---|
| `api/openapi.yaml` | contract: RFC 9457 problems with `type` per code and `errors[]`, `x-problem-codes` per operation, `Idempotency-Key`, `Location`, `Retry-After`, session cookie security |
| `api/vacuum.yaml` | contract lint rules: problem responses and codes, security by default, bounded request strings, no personal data in parameters, the OpenAPI 3.1 subset the generator handles |
| `backend/internal/api` | the `/v1/` handler: request validation against the contract, 404/405/413/415/400/401 as problems that never echo input; `apitest` checks every test response against the contract |
| `backend/internal/kernel/apperr` | error kinds, stable codes, `Retry-After` |
| `backend/internal/leads/contract` | public API of a module: facade interface, DTO, event, sentinel error |
| `backend/internal/leads/entity` | aggregate with unexported state, value objects (masked phone), invariants, table and fuzz tests |
| `backend/internal/leads/usecase` | one type per use case, ports, unit of work, query port (CQRS-lite), idempotency, per-phone limit; `usecasetest`: the fake and the store contract suite both stores run |
| `backend/internal/leads/repository/postgres` | sqlc adapter, unit of work without savepoints, `Update(fn)` with `FOR NO KEY UPDATE`, events in the transaction, read side, key purge |
| `backend/internal/leads/delivery/httpapi` | strict-server handler, tested through the real use cases and the contract |
| `backend/internal/leads/delivery/facade` | the module's facade for other modules |
| `backend/internal/leads/delivery/worker` | a River job and its periodic schedule |
| `backend/internal/notifications` | a thin module: subscriber job, idempotent insert, golden JSON of job arguments |
| `backend/internal/platform/*` | pool and timeouts, migrations under the owner role, `pgtest`, events bus, job outcome, problem writer, middleware, `ClientIP`, readiness and drain, logging with request IDs, `Secret` |
| `backend/internal/archtest` | rules no linter checks: no struct tags in inner layers, `Restore*` only in repositories, a module's SQL stays in its schema |
| `backend/cmd/api`, `backend/cmd/worker` | composition roots (`wire.go`: import aliases `<module><layer>`); end-to-end tests on PostgreSQL |
| `backend/migrations` | goose SQL: default privileges, the module schemas, River's migrations as goose files (`cmd/rivermigrations` writes them after a River upgrade; `river_test.go` checks them) |
| `backend/.golangci.yml`, `.go-arch-lint.yml`, `.squawk.toml`, `sqlc.yaml` | the rules as configuration |
| `tools/tools.lock`, `scripts/` | pinned release binaries with SHA-256 and their installer; the migration number check |
| `Makefile` | `tools`, `generate`, `check` (definition of done), `ci`, `api-breaking`, `migration`, `migrations-fix` |

## Running it
- `make tools && make generate`: from `backend/`, every tool is `bin/<tool>`.
- Two roles: the owner runs migrations (`MIGRATE_DATABASE_URL`, `go run ./cmd/api migrate`), the
  application role only reads and writes rows (`DATABASE_URL`, `go run ./cmd/api` and
  `go run ./cmd/worker`). Once per cluster:
  ```sql
  CREATE ROLE app_owner LOGIN CREATEDB PASSWORD '...';
  CREATE ROLE app_runtime LOGIN PASSWORD '...';
  GRANT app_runtime TO app_owner; -- lets the tests SET ROLE app_runtime
  ```
- Tests: `TEST_DATABASE_URL` is the owner's DSN (any database; tests create their own). With it,
  `make test` runs everything; without it, `make test` adds `-short` and skips the PostgreSQL
  tests, and `go test` without `-short` fails them. CI runs `make ci` with the URL set.
- On a machine with little memory: `make check LINT_FLAGS=--concurrency=1 GOMEMLIMIT=600MiB`.

The contract and the migrations changed incompatibly from the previous version of this reference;
nothing was deployed from it, so they were changed in place. A real project changes them only in
compatible steps (`openapi-contract-first` and `postgres-patterns` skills).
