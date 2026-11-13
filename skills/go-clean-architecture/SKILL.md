---
name: go-clean-architecture
description: Strict Clean Architecture for the Go backend with layers inside each module (entity, usecase, repository, delivery, gateway), module profiles (full, thin), each module's public contract package, events between modules, the dependency rule, ports, the composition root, transactions, error kinds and cross-module calls. Use when creating or changing a module, adding a use case, endpoint, repository, job, event or integration, or when deciding where a piece of code belongs.
---

# Go Clean Architecture (layers inside modules)

Decisions and versions: `STACK.md` at the plugin root. Working code for everything below:
`reference/` next to this file: two modules (`leads`, full profile; `notifications`, thin) that
compile and pass `make ci` on PostgreSQL. Read it before inventing a shape; each module's
`CLAUDE.md` there shows the module template filled in.

## The rule

```
                 ┌──────── cmd/api, cmd/worker (composition root: knows everything) ────────┐
                 │                                                                           │
  delivery/httpapi ─┐                                                 ┌─ repository/postgres
  delivery/worker  ─┼─▶ usecase (one type per use case + ports) ─▶ entity ◀─┤
  delivery/facade  ─┘          ▲ implements ports ─────────────────────────┤─ gateway/<other> ─▶ <other>/contract
          │                                                                │
          └─▶ contract (the module's public API: facade interface, DTOs, events, errors)
  gen/openapi, internal/api ◀── delivery/httpapi        kernel (stdlib only) ◀── everyone
  platform/* (pool, server, events bus, jobs, clock, logging) ◀── adapters and cmd only
```

Source-code dependencies point inward only. `entity` knows nothing; `usecase` knows `entity` and
its own `contract`; adapters know `usecase` and `entity`; nobody but `cmd` knows adapters; another
module sees only `contract`. Checked mechanically, not by review alone:
- `go-arch-lint` (`reference/backend/.go-arch-lint.yml`): imports between project packages and
  vendors, one component per layer per module;
- depguard (`.golangci.yml`, list `inner-layers`): the standard packages `entity` and `usecase`
  may not use (`net/...`, `database/...`, `os`, `log`), which go-arch-lint always allows;
- `internal/archtest`: no struct tags in inner layers, `entity.Restore*` only in repositories, a
  module's SQL touches only its own schema.

## Module profiles

Pick the profile in `/ecc:new-module` and write it, with the reason, in the module's `CLAUDE.md`.
- **Full** if at least one holds: a lifecycle or state machine; money or legal obligations;
  invariants across several fields or aggregates; several entry points (HTTP, jobs, other
  modules); about 10 or more use cases. Layout: `entity`, `usecase` (+ `usecasetest`),
  `repository/postgres`, `delivery/{httpapi,worker,facade}`, `gateway/<other>`, `contract`
  (`reference/backend/internal/leads`).
- **Thin** otherwise (reference data, content, notifications): one package with its handlers or
  jobs, plain service functions and its sqlc queries (`reference/backend/internal/notifications`).
  It still owns its schema, and it gets a `contract` package as soon as another module calls it.
  It becomes full when one of the criteria starts to hold.

## Where does this code go?

| I need to… | Put it in |
|---|---|
| Keep a business invariant ("only a new lead can be assigned", "a phone is +7 and 10 digits") | `entity`: a constructor or method of the aggregate or value object |
| Orchestrate one user intention (parse, load, decide, save, publish) | `usecase`: a type of its own, `CreateLead{…}.Handle(ctx, in)` |
| Describe what a use case needs from outside (storage, time, another module, an external service) | `usecase`: a small interface (port) next to the code that uses it |
| Read data for display (pages, lists, the cabinet) | `usecase`: a query port returning view structs (`LeadQueries` → `LeadView`), implemented straight from sqlc rows |
| Talk to PostgreSQL | `repository/postgres` (SQL in `queries/*.sql`, generated code in `sqlcgen/`) |
| Expose an operation over HTTP | `api/openapi.yaml` first, then `delivery/httpapi` |
| Run work in the background or on a schedule | a River job: args, worker and schedule in `delivery/worker` |
| Tell other modules that something happened | an event in `contract`, published through the `Tx` in the same transaction |
| React to another module's event | the subscriber's job and `Subscribe(bus)` in the subscriber module; `cmd` calls `Subscribe` |
| Let other modules read or call this one | a facade interface in `contract`, implemented in `delivery/facade` |
| Call another module | a port in this module's `usecase`, implemented in `gateway/<other>` over `<other>/contract` |
| Call an external service (captcha, SMS, payments) | a port in `usecase`, implemented in `gateway/<service>` |
| Open connections, configure the server, logging, the event bus | `platform/*` |
| Map an error kind to a status code | `platform/httpserver` (once, for every module) |
| Wire concrete types together, read config | `cmd/<binary>/` (`main.go`, `wire.go`) |

## Layer contracts

**entity** (`reference/backend/internal/leads/entity/lead.go`)
- Aggregates with unexported fields; state changes only through methods that keep invariants.
- Value objects for every validated field (`Phone`, `Topic`, `City`), so two strings can't swap
  places in a constructor; constructors refuse zero values.
- `Parse…`/`New…` validate; `Restore…` rebuilds from storage without re-running creation rules
  and is called only by repositories (archtest).
- Personal data masks itself when printed or logged (`Phone.String`, `Phone.LogValue`).
- Takes IDs and time as arguments (`NewLead(id, …, now)`), so it is deterministic.
- Imports: stdlib, `kernel`. No struct tags, no `context`, no I/O, no logging.

**usecase** (`reference/backend/internal/leads/usecase/`)
- One type per use case in a full module, each with only the ports it needs:
  `NewCreateLead(uow, clock, newID)`, `NewAssignLead(leads)`, `NewGetLead(queries)`. A test
  builds one use case without the dependencies of the others, and two agents adding use cases
  don't edit the same file.
- Declares its ports (`ports.go`) and its errors with kinds and codes (`ErrTooManyLeads`). Input
  and output are plain structs (`CreateLeadInput`, `LeadView`); output carries only what callers
  may see (no phone).
- Writes go through entities; reads for display go through query ports into views (CQRS-lite:
  no `Restore` and no triple mapping on the read path).
- Orchestrates: parse input into value objects, call entity methods and ports, wrap errors with
  context (`fmt.Errorf("create lead: %w", err)`). Business decisions belong in `entity`.
- Imports: stdlib, `kernel`, own `entity` and own `contract`. No vendors.
- `usecasetest/`: the in-memory fake of the storage ports and the store contract suite both the
  fake and `repository/postgres` run (golang-testing skill). Only test files import it.

**repository/postgres** (`reference/backend/internal/leads/repository/postgres/`)
- Implements the storage ports and asserts them (`var _ usecase.UnitOfWork = (*Store)(nil)`).
- Maps sqlc rows ↔ entities (`toEntity`) and rows → views (`queries.go`); never returns a row type.
- Translates storage facts into port vocabulary: `pgx.ErrNoRows` → `usecase.ErrLeadNotFound`,
  contention and timeouts → `postgres.ErrBusy` (503 with `Retry-After`). Data that fails parsing
  on the way out is corruption, an internal error: never return it with its entity kind
  (a broken phone in the database is not the client's 422).
- Reads and writes only its module's schema.

**delivery/httpapi** (`reference/backend/internal/leads/delivery/httpapi/handler.go`)
- Implements this module's operations of the generated `openapi.StrictServerInterface`.
- Holds the concrete use-case types (`UseCases{Create *usecase.CreateLead; …}`): no interface
  mirroring the use cases; handler tests run the real use cases on the fake store.
- Translate → call → translate. Return use-case errors unchanged; `httpserver.ErrorWriter` turns
  them into problems. Request validation against the contract happens before the handler
  (`internal/api`). No business `if`s here.

**delivery/worker** (`reference/backend/internal/leads/delivery/worker/worker.go`)
- River workers: decode the args, call one use case, `return jobs.Outcome(err)` (not found,
  invalid and conflict cancel the job; everything else is retried).
- A job may run more than once: the work is idempotent (a unique key, `ON CONFLICT DO NOTHING`).
- Job args are the one place with `json` tags outside the adapters: River stores them for weeks.
  Fields are only added, never renamed; IDs only, no personal data; a golden-JSON test per kind.
- Schedules live next to the worker (`PeriodicJobs()`); `cmd/worker` passes them to River.

**delivery/facade** (`reference/backend/internal/leads/delivery/facade/facade.go`): implements
the module's `contract` interfaces with its use cases; `cmd` hands it to other modules.

**contract** (`reference/backend/internal/leads/contract/contract.go`): the only package other
modules import: facade interfaces (`Reader`), DTOs (`Lead`, without personal data unless the
caller's purpose needs it), events (`LeadCreated`, versioned name), sentinel errors. Stdlib and
`kernel` only. Changed like the HTTP contract: add, never rename or remove; a breaking change to
an event is a new version published alongside the old one.

**gateway/<other>**: implements a port of this module with `<other>/contract` (or an external
client). Maps the other side's types into this module's, so a change there breaks one adapter,
not your use cases.

**kernel**: stdlib-only shared code every layer may import: `apperr`, and value types that are
genuinely universal (money, if two modules must agree on it). Keep it tiny.

**platform**: technical infrastructure (`postgres.Open`, `postgres.Migrate`, `httpserver`,
`events`, `jobs`, `logging`, `config.Secret`, `clock.System`; test support in `postgres/pgtest`).
Never imports a business module.

**cmd**: composition roots. `cmd/api/wire.go` builds each module's store → use cases → handler
and embeds every module's handler into one `StrictServerInterface`; `cmd/worker` registers every
module's workers and periodic jobs. Every process that publishes builds the event bus and calls
each subscriber module's `Subscribe`. No `init()`, no package-level mutable state, no DI
container.

## Packaging and names

Every full module has the same package names (`entity`, `usecase`, `postgres`, `httpapi`), so
code that imports two modules' packages uses aliases `<module><layer>`: `leadspg`, `leadsuc`,
`leadshttp`, `leadsworker`, `leadsfacade` (`reference/backend/cmd/api/wire.go`). Inside a module,
no aliases are needed. Another module imports `<module>/contract` under the module's name:
`leads "example.com/app/internal/leads/contract"`.

## Data crossing the boundaries

```
write: HTTP JSON ─▶ openapi.CreateLeadRequest ─▶ usecase.CreateLeadInput ─▶ entity.Phone/Topic/City ─▶ entity.Lead
                          (delivery maps)              (usecase parses)                       (repository maps) ◀─▶ sqlcgen row
read:  sqlcgen.GetLeadViewRow ─▶ usecase.LeadView ─▶ openapi.Lead ─▶ JSON
              (repository maps)          (delivery maps)
other modules: usecase.LeadView ─▶ contract.Lead (facade maps)
```

Each adapter owns its mapping functions. Hand-written mapping is the point: a contract change
becomes a compile error. No reflection copiers.

## Ports

- Declared by the consumer, in the consumer's package (`usecase`). The implementation never
  imports an interface "to implement it"; it asserts it.
- Small and named by role: `LeadRepository`, `LeadQueries`, `Clock`, `LawyerDirectory`. No `I`
  prefix, no `…Interface` suffix, no "interfaces" package.
- Methods take `ctx context.Context` first when they do I/O, and use the module's types, never
  `pgx.Tx`, `*http.Request` or generated API structs.
- No generic `Repository[T]`: each aggregate gets the queries its use cases need.

## Errors

- Errors are `*apperr.Error` values with a kind, a stable code and a safe message
  (`reference/backend/internal/kernel/apperr/apperr.go`); `WithRetryAfter` adds a hint the HTTP
  layer turns into `Retry-After`. Entities choose `Invalid`/`Conflict`; use cases and
  repositories choose `NotFound`/`Forbidden`/`RateLimited`/…; anything else is internal.
- A new kind needs a status in `httpserver.StatusOf` and a decision in `jobs.Outcome`; the
  `exhaustive` linter fails until both handle it.
- Wrap with `%w` and the identifying context; never wrap the same error twice with the same words.
- Check with `errors.Is(err, usecase.ErrLeadNotFound)` or `errors.AsType[*apperr.Error](err)`.
- Log once, at the edge (`httpserver.ErrorWriter`, the worker), never with personal data.
  Internal error text never reaches a client.

## Transactions

1. **One aggregate**: `repo.Update(ctx, id, func(*entity.Lead) error)`: the adapter opens the
   transaction, locks the row (`SELECT … FOR NO KEY UPDATE`), runs the callback, saves every
   changeable column. The use case never sees a transaction.
2. **Several writes, or a write plus an event**: the unit-of-work port.

```go
// usecase/ports.go
type Tx interface {
	Leads() LeadRepository
	IdempotencyKeys() IdempotencyKeys
	Events() Publisher // Publish(ctx, contract.LeadCreated{…}): jobs inserted in this transaction
}
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// repository/postgres: repositories run their statements on the transaction itself
func (s *Store) Do(ctx context.Context, fn func(context.Context, usecase.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return fn(ctx, txRepos{tx: tx, bus: s.bus}) })
}
```

- No savepoints per repository call (each costs two round trips and, at scale, subtransaction
  trouble): repositories built on the transaction use it directly.
- Inside `Do`/`Update`, only this module's repositories: never call another module, an external
  service or anything slow, because the transaction holds row locks and a pool connection. To
  make another module react, publish an event.
- A read inside `Do` that a decision depends on either locks the row or is backed by a
  constraint (the idempotency key's primary key serializes concurrent claims).
- Never put a `pgx.Tx` in `context.Context`: it hides which calls are transactional and breaks
  silently when a callee opens its own connection.

## Between modules

- **Public API**: a module reaches another only through `<other>/contract`. The module graph has
  no cycles and is written at the top of `.go-arch-lint.yml` and in
  `docs/architecture/context-map.md`.
- **Events** (preferred when A doesn't need B's answer): A publishes a `contract` event through
  `tx.Events()`; `platform/events` inserts one River job per subscriber in the same transaction
  (`InsertManyTx`). A doesn't know its subscribers; each subscriber module owns its job kind and
  `Subscribe(bus)`. Events carry IDs and facts that never change, never personal data.
- **Synchronous**: A declares `type LawyerDirectory interface{ ActiveIn(ctx, city CityID) ([]LawyerRef, error) }`
  in `a/usecase`; `a/gateway/lawyers` implements it with `lawyers/contract`; `cmd` wires the
  facade in. A's use cases and tests never import module B.
- **Consistency across modules** (selling a lead: assignment in `leads`, money in `billing`):
  decided per case in an ADR when `billing` is designed: (a) the money invariant lives in one
  module; (b) a saga on River jobs with idempotent steps and compensations; (c) a deliberate
  shared transaction. Until then the transaction rule above holds.
- **Data between modules**: reference data (cities, legal topics) has one owner module; others
  keep IDs, read-only copies fed by its events, or snapshots taken at the moment that matters
  (the price at the time of sale). Personal data is not copied: read it through the owner's
  facade when needed. A deletion propagates by event.
- Never read another module's tables, never share entities, never import another module's
  `entity`, `usecase`, `repository` or `delivery`.

## Adding a feature

Design the contract change first (in the plan), then merge in steps that each compile and pass
the gate:
1. Entity and use case: failing tests → invariants and the use case with fakes of its ports.
2. Storage: goose migration → `queries/*.sql` → `bin/sqlc generate` → adapter, run against the
   store contract suite on PostgreSQL.
3. Contract and delivery together: edit `api/openapi.yaml`, `go generate ./...`, implement the
   handler (the generated interface doesn't compile without it), test through `apitest.Serve`
   (every response checked against the contract). Wire in `cmd`.
4. Web: regenerate the TS client and build the UI.
Each step: `bin/go-arch-lint check`, then the full gate (`/ecc:verify`).

## Anti-patterns (reject in review)

- Top-level layering by technical type (`internal/handlers`, `internal/services`, `internal/models`).
- Entities with struct tags, or generated/sqlc types used as entities.
- Anemic entities: all rules in use cases, entities as field bags with setters.
- A module importing another module's internals instead of its `contract`; a module that knows
  its subscribers; personal data in events, job args or job errors.
- Interfaces next to their only implementation "for mocking"; mocks of entities or of use cases.
- A read path that restores an aggregate only to map it into a view.
- Calls to other modules or external services inside a transaction; savepoints per repository.
- `context.Context` carrying dependencies (DB, logger, repositories) or transactions.
- Global DB/logger, `init()` wiring, service locators, `uber/fx`, `google/wire` (archived).
- Business `if`s in handlers or SQL; error strings matched with `strings.Contains`.
- `panic` for control flow; ignoring errors without a comment.

## Sources

Robert C. Martin, *Clean Architecture* (dependency rule, use-case interactors) · Alistair Cockburn,
*Hexagonal Architecture* (ports and adapters) · Three Dots Labs, *Go with the Domain* and
`wild-workouts-go-ddd-example` (update-func repositories, CQRS-lite, a type per command) ·
Ardan Labs `service` (domain packages with prefixed names, API tests on a real database) ·
Kamil Grzybek, *Modular Monolith* (schema per module, integration events, public module API) ·
John Ousterhout, *A Philosophy of Software Design* (pass-through methods) · Google Go Style Guide
and Go Code Review Comments (consumer-side interfaces, package naming, errors) · Ben Johnson,
*Standard Package Layout* · Mat Ryer, *How I write HTTP services in Go after 13 years*
(composition root, `run` function, no globals).
