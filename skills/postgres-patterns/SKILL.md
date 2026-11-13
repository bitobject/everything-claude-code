---
name: postgres-patterns
description: PostgreSQL in the Go backend - roles (schema owner vs application), pgx v5 pool, timeouts and UTC scanning, sqlc configuration (PostgreSQL 17 dialect), repository adapters that map sqlc rows to entities and views, Update with SELECT ... FOR NO KEY UPDATE, UnitOfWork with tx-scoped repositories and no savepoints, events and River jobs in the same transaction, Idempotency-Key storage, worker error policy, schema per module, goose migrations (timestamped names, River as goose files, expand/contract, CREATE INDEX CONCURRENTLY, lock timeouts), indexing, keyset pagination, money between modules, PgError to apperr mapping. Use when writing SQL, migrations, sqlc queries, jobs or repository/postgres code.
---

# PostgreSQL patterns

Binding decisions are in `STACK.md`: PostgreSQL 18, pgx v5 (`pgxpool`), sqlc with
`sql_package: pgx/v5`, goose v3 SQL migrations embedded in the binary, River for jobs, one schema
per module, stdlib `uuid`. Working code: `../go-clean-architecture/reference/backend/`. Tests for
this code: `golang-testing` skill. Reviews: `database-reviewer` agent.

## Where database code lives

| Package | Contains | Imports pgx? |
|---|---|---|
| `platform/postgres` | `Open` (pool), `Migrate` (goose, owner role), `MapError`/`ErrBusy`, `pgtest` (test databases) | yes |
| `platform/events`, `platform/jobs` | the event bus (one River job per subscriber), `jobs.Outcome` (retry or cancel) | River |
| `<module>/repository/postgres` | adapters implementing `usecase` ports; `queries/*.sql`; `sqlcgen/` (generated) | yes |
| `<module>/usecase` | ports (`LeadRepository`, `LeadQueries`, `UnitOfWork`, `Tx`, `Publisher`) | no |
| `<module>/contract` | events other modules subscribe to | no |
| `<module>/delivery/worker`, thin module package | River job args, workers, periodic schedules | River |
| `migrations/` | goose `.sql` files + `embed.go`; `river_test.go` | no |

Never pass `pgx.Tx`, `pgtype.*` or `sqlcgen` types across a port. Never put a transaction in
`context.Context`: transactions are explicit values handed to tx-scoped repositories.

## Roles

Two roles per database (`reference/backend/migrations/00001_privileges.sql`, README there):
- **owner** (`app_owner`) owns the schemas and runs migrations: `MIGRATE_DATABASE_URL`,
  `api migrate`;
- **application** (`app_runtime`) runs the API and the worker: `DATABASE_URL`; only `SELECT`,
  `INSERT`, `UPDATE`, `DELETE`, granted by `ALTER DEFAULT PRIVILEGES FOR ROLE app_owner`. No DDL,
  no `TRUNCATE`. An append-only table (ledger, audit log) revokes `UPDATE` and `DELETE` from it in
  the migration that creates it. River's reindexer needs `GRANT MAINTAIN ON river.river_job`.
- Tests connect as the owner and `SET ROLE app_runtime` on every connection (`pgtest`), so a
  query that needs more than DML fails in tests, not in production.

## Pool

```go
// platform/postgres (reference: internal/platform/postgres/postgres.go)
func Open(ctx context.Context, dsn string, c Config) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("parse database config: invalid DSN") // the parse error may echo the password
	}
	cfg.MaxConns = c.MaxConns
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnLifetimeJitter = 5 * time.Minute // avoid reconnect storms
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// pgx scans timestamptz into time.Local whatever the session time zone: scan it as UTC.
		conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		if c.AfterConnect != nil {
			return c.AfterConnect(ctx, conn) // tests: SET ROLE
		}
		return nil
	}
	rp := cfg.ConnConfig.RuntimeParams // session defaults, sent at connect
	rp["application_name"] = c.AppName
	rp["timezone"] = "UTC"
	rp["statement_timeout"] = ms(c.StatementTimeout)                    // 5 s: one statement
	rp["transaction_timeout"] = ms(c.TransactionTimeout)                // 10 s: a whole transaction (PG 17+)
	rp["idle_in_transaction_session_timeout"] = ms(c.IdleInTransactionLimit) // 30 s
	// NewWithConfig, then Ping with a 5 s timeout; close the pool if the ping fails.
}
```

- Two clocks bound every query: the request deadline (`httpserver.Timeout` in the API, the job's
  context in the worker) and the server-side limits above. When a context ends during a query,
  pgx (default `DeadlineContextWatcherHandler`) sets a deadline on the socket and drops the
  connection; it sends no cancel request. The server stops working on the query only through its
  own limits, which is why `statement_timeout` and `transaction_timeout` are always set.
- Size `MaxConns` from the database, not from traffic hopes: start from about twice the
  database's vCPU count in total, divide it by the number of processes (API replicas + workers),
  stay below the server's `max_connections` minus admin headroom, then measure.
- Times: write UTC, read UTC (the codec above). PostgreSQL keeps microseconds, so `clock.System`
  truncates to microseconds: a value read back equals the value written and returned.
- The pool is created once in `cmd/*` and passed to constructors; close it after the HTTP server
  and the River client have stopped.
- Behind PgBouncer: it rejects startup parameters it does not track (the timeouts included), so
  set them per role instead (`ALTER ROLE app_runtime SET statement_timeout = '5s'`); in
  transaction mode, keep pgx's statement cache only while PgBouncer's `max_prepared_statements`
  is non-zero, otherwise change `ConnConfig.DefaultQueryExecMode`.

## sqlc

`backend/sqlc.yaml`, one `sql` entry per module so each module's queries generate into its own
package (`reference/backend/sqlc.yaml`):

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: migrations          # sqlc reads the -- +goose Up sections and ignores Down
    queries: internal/leads/repository/postgres/queries
    gen:
      go: &go
        package: sqlcgen
        out: internal/leads/repository/postgres/sqlcgen
        sql_package: pgx/v5
        omit_unused_structs: true
        overrides:
          - db_type: uuid
            go_type: { import: uuid, type: UUID }
          - db_type: timestamptz
            go_type: { import: time, type: Time }
  - engine: postgresql          # a thin module: generated code next to its single package
    schema: migrations
    queries: internal/notifications/queries
    gen:
      go:
        <<: *go
        out: internal/notifications/sqlcgen
```

- The release sqlc (1.31) parses **PostgreSQL 17** syntax. The server is 18, but the files sqlc
  reads (migrations and queries) stay in the PG 17 dialect: no virtual generated columns (write
  `STORED`), no `NOT ENFORCED`, no `WITHOUT OVERLAPS`, no `RETURNING old/new`. The release binary
  needs no C toolchain.
- Every entry reads all migrations, so sqlc alone doesn't keep a module in its schema:
  `internal/archtest` fails when a module's queries name another module's schema (or `river`).
- The stdlib `uuid.UUID` is a `[16]byte`: pgx v5 encodes and scans it natively.
- Regenerate with `make generate`; `make generate-check` fails when `sqlcgen/` is stale. Never
  edit `sqlcgen/`. Don't use `sqlc verify` or `sqlc push`: they send the schema and queries to
  sqlc's cloud.

Query file conventions (`reference/.../queries/leads.sql`):

```sql
-- name: GetLeadForUpdate :one
-- FOR NO KEY UPDATE blocks other writers of the row but not inserts into tables whose foreign
-- keys reference it (FOR UPDATE would block their key checks too).
SELECT id, phone, topic, city, status, created_at
FROM leads.leads
WHERE id = $1
FOR NO KEY UPDATE;

-- name: UpdateLead :exec
-- Writes every column the aggregate can change; id and created_at never change.
UPDATE leads.leads
SET phone = sqlc.arg(phone), topic = sqlc.arg(topic), city = sqlc.arg(city), status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: GetLeadView :one
-- The read side: only what a view shows, no phone number.
SELECT id, topic, city, status, created_at FROM leads.leads WHERE id = $1;
```

- Schema-qualify every table (`leads.leads`).
- List columns explicitly; never `SELECT *` (a new column silently changes the generated row).
- Name parameters with `sqlc.arg(name)` (`sqlc.narg` for nullable) once there are more than two.
- `:execrows` when the affected row count is the answer (claims, optimistic locking, purges),
  `:one` when absence is an error (`pgx.ErrNoRows`), `:copyfrom` for bulk loads.
- Read queries for views select only what the view shows (no personal data a page doesn't need).

## Repository adapter

```go
// LeadRepository works on the pool or, inside a unit of work, on its transaction.
type LeadRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

func (r *LeadRepository) queries() *sqlcgen.Queries {
	if r.tx != nil {
		return sqlcgen.New(r.tx)
	}
	return sqlcgen.New(r.pool)
}

// toEntity re-parses the value objects, so a corrupt row fails loudly instead of spreading.
func toEntity(row sqlcgen.LeadsLead) (*entity.Lead, error) {
	phone, err := entity.ParsePhone(row.Phone)
	if err != nil {
		return nil, corrupt(row.ID, "phone", err)
	}
	// … topic, city, status the same way
	return entity.RestoreLead(row.ID, phone, topic, city, status, row.CreatedAt), nil
}

// corrupt keeps the rule's message but not its kind: bad data in our database is our bug (500),
// not the client's input (422).
func corrupt(id uuid.UUID, field string, err error) error {
	return fmt.Errorf("stored lead %s has an invalid %s: %s", id, field, err.Error())
}

func mapError(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.ErrLeadNotFound
	}
	return fmt.Errorf("%s: %w", what, postgres.MapError(err))
}
```

Rows → entities only through the entity's `Restore…` constructor (it skips creation rules such
as "status starts as new"); entities → params only through getters. Rows → views directly, in
the read-side adapter (`queries.go`), with no entity in between. JSON columns are decoded here.

### Update: load, change, save in one transaction

```go
func (r *LeadRepository) Update(ctx context.Context, id uuid.UUID, fn func(*entity.Lead) error) (*entity.Lead, error) {
	var lead *entity.Lead
	update := func(q *sqlcgen.Queries) error {
		row, err := q.GetLeadForUpdate(ctx, id) // row lock until commit: concurrent updates queue
		if err != nil {
			return mapError(err, "select lead for update")
		}
		l, err := toEntity(row)
		if err != nil {
			return err
		}
		if err := fn(l); err != nil {
			return err // domain error: rolled back, returned unchanged
		}
		if err := q.UpdateLead(ctx, sqlcgen.UpdateLeadParams{ID: l.ID(), Phone: l.Phone().E164(),
			Topic: l.Topic().String(), City: l.City().String(), Status: string(l.Status())}); err != nil {
			return mapError(err, "update lead")
		}
		lead = l
		return nil
	}
	if r.tx != nil { // inside a unit of work: its transaction, no savepoint
		return lead, update(sqlcgen.New(r.tx))
	}
	return lead, pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error { return update(sqlcgen.New(tx)) })
}
```

- `pgx.BeginFunc` commits when `fn` returns nil and rolls back otherwise; it returns `fn`'s error
  as is, so `errors.Is(err, entity.ErrNotAssignable)` still works for the caller.
- The update writes every changeable column: a method that changes another field is saved too.
  The store contract suite has the test ("update saves the whole aggregate fn leaves").
- `fn` is pure domain logic: no I/O while the row is locked.
- When contention is rare and the lock unwanted: a `version` column,
  `UPDATE … WHERE id = $1 AND version = $2` as `:execrows`; zero rows → `apperr.KindConflict`.

## UnitOfWork, events and jobs

A unit of work when one use case writes several things or must publish an event with a state
change. The port lives in `usecase`; the callback gets repositories bound to the transaction.

```go
// repository/postgres
func (s *Store) Do(ctx context.Context, fn func(context.Context, usecase.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(ctx, txRepos{tx: tx, bus: s.bus})
	})
}

// txRepos builds the repositories of one transaction: they run their statements in it directly.
func (r txRepos) Leads() usecase.LeadRepository            { return &LeadRepository{tx: r.tx} }
func (r txRepos) IdempotencyKeys() usecase.IdempotencyKeys { return idempotencyKeys{q: sqlcgen.New(r.tx)} }
func (r txRepos) Events() usecase.Publisher                { return publisher(r) } // bus.Publish(ctx, tx, e)
```

- **No savepoints.** A repository on a transaction does not `Begin` again: each savepoint costs two
  round trips, and many of them cause subtransaction trouble at scale.
- **Inside `Do`, only this module's repositories**: no other module, no external service, nothing
  slow; the transaction holds locks and a pool connection. Publish an event instead.
- **Reads that decide** inside `Do` lock the row (`FOR NO KEY UPDATE`) or rely on a constraint
  (the idempotency key's primary key makes a concurrent claim wait for the first transaction).
- **Events** (`platform/events`): `Publish` inserts one River job per subscriber with
  `InsertManyTx` in the caller's transaction: a job exists only if the change commits
  (transactional outbox). River runs one worker per job kind, so each subscriber module owns a
  kind; the publisher never names its subscribers. The API process uses an insert-only client
  (`&river.Config{Schema: "river"}`, never started); `cmd/worker` builds the client with
  `Queues`, `Workers` and `PeriodicJobs`.

### Workers

```go
// a thin module's worker (reference: internal/notifications/notifications.go)
func (w *Worker) Work(ctx context.Context, job *river.Job[LeadCreatedArgs]) error {
	lead, err := w.leads.Lead(ctx, job.Args.LeadID) // the other module's facade, not its tables
	if err != nil {
		return jobs.Outcome(fmt.Errorf("read lead %s: %w", job.Args.LeadID, err))
	}
	err = w.q.InsertMessage(ctx, …) // INSERT … ON CONFLICT (lead_id) DO NOTHING: the job may run twice
	if err != nil {
		return jobs.Outcome(fmt.Errorf("insert message: %w", postgres.MapError(err)))
	}
	return nil
}
```

- **Error policy** (`jobs.Outcome`): `NotFound`, `Invalid` and `Conflict` cancel the job
  (`river.JobCancel`): a retry can't fix them, and River would retry for about three weeks
  (25 attempts). Everything else is returned and retried with backoff.
- **Idempotent**: a job can run again after a crash or a timeout; make its effect a no-op the
  second time (a unique key and `ON CONFLICT DO NOTHING`, or a state check under a lock).
- **Args are JSON for weeks**: `json` tags, fields only added, never renamed; `Kind()` never
  changes once deployed; a golden-JSON test per kind (`TestLeadCreatedArgsJSON`).
- **No personal data in River's tables**: River keeps completed and cancelled jobs 24 hours and
  discarded ones 7 days, with their args and error texts. Args carry IDs; workers don't wrap
  personal data into errors.
- **Periodic jobs**: a schedule next to the worker (`delivery/worker.PeriodicJobs()`), with
  `RunOnStart` when the interval is long: the elected leader keeps the schedule in memory only.
- **Stopping**: `client.Start(context.WithoutCancel(ctx))` (cancelling Start's context stops
  hard), then on SIGTERM `client.Stop` with a grace period, `StopAndCancel` after it
  (`reference/backend/cmd/worker/main.go`).

## Idempotency-Key storage

`createLead` takes an `Idempotency-Key` (`openapi-contract-first` skill); the use case claims the
key in the transaction that creates the lead (`reference/.../queries/idempotency_keys.sql`):

```sql
-- name: ClaimIdempotencyKey :execrows
-- 0 rows: the key belongs to a live request (read it with GetIdempotencyKey). A concurrent claim
-- of the same key waits until this transaction ends.
INSERT INTO leads.idempotency_keys AS k (key, fingerprint, lead_id, created_at)
VALUES (sqlc.arg(key), sqlc.arg(fingerprint), sqlc.arg(lead_id), sqlc.arg(created_at))
ON CONFLICT (key) DO UPDATE
SET fingerprint = excluded.fingerprint, lead_id = excluded.lead_id, created_at = excluded.created_at
WHERE k.created_at < sqlc.arg(expired_before);
```

- Claimed: create the lead. Not claimed and the same fingerprint (SHA-256 of the request's
  normalized fields): return the first lead. Another fingerprint: 422 `idempotency.key_reused`.
- The claim rolls back with the lead, so a failed request frees its key.
- An expired key (older than the TTL, 24 h) is taken over by the next request that uses it; an
  hourly periodic job deletes expired rows (`PurgeIdempotencyKeys`, `DELETE … WHERE created_at <
  $1`). The fingerprint is derived from personal data (a phone number's hash is reversed by trying
  every number), so rows live only as long as they are useful.

## Errors: PgError → apperr

`platform/postgres/errors.go`: `MapError` turns contention and server-side timeouts into
`ErrBusy` (`KindUnavailable`, `Retry-After: 1`), keeping the cause for the log; `IsUniqueViolation(err,
constraint)` matches a named constraint. Module adapters handle their own facts first.

| SQLSTATE | Meaning | Map to |
|---|---|---|
| no rows (`pgx.ErrNoRows`) | absent | module's `…NotFound` (`KindNotFound`) |
| `23505` unique | duplicate | module error with `KindConflict`, chosen by `ConstraintName` |
| `23503` foreign key | missing or still referenced parent | `KindConflict` or `KindInvalid`, by constraint |
| `23514` check, `23502` not null | invariant broken | a bug: the entity should have refused it; leave internal |
| `40001`, `40P01`, `55P03`, `57014`, `25P04` | contention, timeouts | `postgres.ErrBusy` (retryable) |
| context deadline or cancel | the caller's deadline or a gone client | left as is: `ErrorWriter` answers 503 `timeout` or 499 |

Match constraints by name, so name them in migrations. Use `errors.AsType[*pgconn.PgError]`.
Never return a `PgError` message to a client.

## Migrations (goose)

- One sequence for the whole backend, embedded with `//go:embed *.sql` (`migrations/embed.go`).
- **New file**: `make migration name=add_lawyers` creates `<timestamp>_add_lawyers.sql`, so
  parallel branches never pick the same number. `make migrations-fix` renumbers it sequentially
  right before merging; `make migrations-check` fails on a version used twice and, in CI, on a
  timestamp name left. If main got the same number meanwhile, take the next one.
- **Running** (`platform/postgres.Migrate`, `api migrate`): the owner's DSN, a session with
  `lock_timeout = 5s`, no `statement_timeout` or `transaction_timeout` (so `CREATE INDEX
  CONCURRENTLY` can finish), goose's session advisory lock (one migrator at a time); afterwards
  it fails if any index is `INVALID`. Run it as a deploy step before the new version starts,
  never from `api` startup.
- **River's migrations are goose files** (`NNNNN_river_VVV.sql`): one history, one lock, one
  command. `go run ./cmd/rivermigrations` writes the versions a River upgrade adds;
  `migrations/river_test.go` fails while one is missing or differs from River's SQL. squawk skips
  them (`excluded_paths`), the files about River that we write are linted.
- `make migrations-lint` runs squawk (`backend/.squawk.toml`, `pg_version = "18.0"`) and fails on
  blocking or unsafe statements. A deliberate drop carries `-- squawk-ignore <rule>` on the line
  before it. The timeout rules are off because the `Migrate` session sets the timeouts.
- Never edit a migration that ran anywhere shared; add a new one. Down sections exist for local
  development; production rolls forward.
- Name every constraint and index the code matches on.

```sql
-- +goose Up
CREATE SCHEMA leads;
CREATE TABLE leads.leads (
    id         uuid        PRIMARY KEY,           -- uuid.NewV7() from the application
    phone      text        NOT NULL,              -- personal data
    topic      text        NOT NULL,
    city       text        NOT NULL,
    status     text        NOT NULL CONSTRAINT leads_status_check CHECK (status IN ('new', 'assigned', 'closed')),
    created_at timestamptz NOT NULL
);
CREATE INDEX leads_phone_created_at_idx ON leads.leads (phone, created_at);

-- +goose Down
-- squawk-ignore ban-drop-table
DROP TABLE leads.leads;
-- squawk-ignore ban-drop-schema
DROP SCHEMA leads;
```

Indexes on existing tables are built without blocking writes, outside a transaction:

```sql
-- +goose NO TRANSACTION
-- squawk-disable-assume-in-transaction
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS leads_created_at_id_idx ON leads.leads (created_at DESC, id DESC);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS leads.leads_created_at_id_idx;
```

One statement per `NO TRANSACTION` file: nothing rolls back a half-applied one. A failed
concurrent build leaves an `INVALID` index that `IF NOT EXISTS` would skip; `Migrate` reports it:
drop it and rerun.

### Safe changes on live tables

| Change | Safe way |
|---|---|
| Add column | nullable, or `NOT NULL DEFAULT <constant>` (no rewrite since PG 11) |
| Add a stored generated or an identity column | rewrites the table: add a plain column, backfill in batches, then switch |
| Make column `NOT NULL` | `ADD CONSTRAINT c CHECK (col IS NOT NULL) NOT VALID` → `VALIDATE CONSTRAINT c` (next migration) → `SET NOT NULL` (uses the validated check) → drop the check |
| Add foreign key | `ADD CONSTRAINT … NOT VALID`, then `VALIDATE CONSTRAINT` in the next migration; index the referencing column |
| Add a unique constraint | `CREATE UNIQUE INDEX CONCURRENTLY` (own file), then `ADD CONSTRAINT … UNIQUE USING INDEX` |
| Add index | `CREATE INDEX CONCURRENTLY` in a `NO TRANSACTION` file |
| Rename or retype column | expand → migrate → contract: add the new column, write both, backfill in batches, switch reads, stop writing the old one, drop it (separate releases) |
| Drop column | first deploy code that no longer reads or writes it, then drop |
| Backfill | batches of a few thousand rows by primary key, in a job or a separate migration; never one `UPDATE` over a big table |

One DDL statement on a hot table per file: a transactional file holds every lock it takes until
it commits, so a second statement keeps the table blocked while it waits for its own lock.

## Data types

| Use | Type | Avoid |
|---|---|---|
| IDs | `uuid` (v7 from the app) | `serial`; random v4 keys on big tables (index locality) |
| Text | `text` + `CHECK (length(…) <= n)` when a limit is a rule | `varchar(255)` by habit |
| Time | `timestamptz`, written and scanned as UTC, microsecond precision | `timestamp` without zone |
| Money | `bigint` minor units | `float`, `real`, `money` |
| Enum-like | `text` + `CHECK (… IN (…))` | `CREATE TYPE … AS ENUM` (changing it needs DDL) |
| Flexible attributes | `jsonb`, decoded in the adapter | `json`, EAV tables |
| Generated column | `GENERATED ALWAYS AS (…) STORED` | `VIRTUAL` (PG 18 only: sqlc can't parse it) |

## Money between modules

- The consumer is idempotent: the charge row has `UNIQUE (lead_id)` and is inserted with
  `ON CONFLICT DO NOTHING`, so a repeated job charges once.
- A balance never goes negative by a check-then-write race: `UPDATE billing.balances SET amount =
  amount - $2 WHERE lawyer_id = $1 AND amount >= $2` as `:execrows` (0 rows: insufficient
  funds), or an append-only ledger of entries whose sum is the balance.
- Which module owns the invariant of a lead sale is an ADR (go-clean-architecture skill, "Between
  modules").

## Indexing

| Query shape | Index |
|---|---|
| `WHERE a = $1` | B-tree `(a)` |
| `WHERE a = $1 AND b > $2` | composite `(a, b)`: equality columns first, then range |
| `ORDER BY created_at DESC, id DESC LIMIT n` | `(created_at DESC, id DESC)` |
| Subset only (`WHERE status <> 'closed'`) | partial index with the same predicate |
| Read only indexed columns | covering: `(a) INCLUDE (b, c)` |
| `jsonb @> …`, full text | GIN |
| Append-only time series range | BRIN |

Index every foreign key column. Check plans with `EXPLAIN (ANALYZE, BUFFERS)` on
production-sized data; drop indexes that `pg_stat_user_indexes` shows unused.

## Keyset pagination

Never `OFFSET` on large tables. Paginate by the sort key plus a unique tiebreaker:

```sql
WHERE (created_at, id) < (sqlc.arg(before_created_at)::timestamptz, sqlc.arg(before_id)::uuid)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);
```

- The cursor is the last row's `(created_at, id)`, encoded opaquely by `delivery/httpapi`; the
  use case sees typed values.
- First page: a cursor after every row (far-future time and `uuid.Max()`), or a separate query.
- With UUIDv7 keys made at creation and "newest first", `id` alone is time-ordered:
  `WHERE id < $1 ORDER BY id DESC`.
- Fetch `page_size + 1` rows to know whether a next page exists without a `COUNT(*)`.

## Concurrency

- Keep transactions short: no network calls, no waiting on users, no big loops inside.
- `FOR NO KEY UPDATE` to change a row; `FOR UPDATE` only when the key itself changes or the row
  is deleted.
- Lock rows in a consistent order (`ORDER BY id FOR NO KEY UPDATE`) when locking several.
- Queue-like reads several workers share: River already does `FOR UPDATE SKIP LOCKED`; don't
  build a second queue.
- Retry serialization failures and deadlocks (`40001`, `40P01`) by rerunning the whole
  transaction (`Update`/`Do`), and only when it is idempotent; never retry one statement inside a
  failed transaction.

## Test databases

`pgtest.New(t, migrations.FS)` (golang-testing skill): a template database per migration set,
one database per test, the application role. The image matches production: PostgreSQL 18 on the
same libc. Collations differ between glibc and musl, so with Cyrillic text sorted by a glibc
locale in production, don't test on an Alpine image.
