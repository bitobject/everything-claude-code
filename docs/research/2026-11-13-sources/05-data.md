<!-- Independent reviewer's report, 2026-11-13: Data layer: PostgreSQL 18, pgx, sqlc, goose, River.
     Raw model output, kept for its sources and detail. Which claims were re-verified is stated in
     ../2026-11-13-best-practices-review.md (sections marked [проверено] / [опровергнуто]). -->

# Data layer review (PostgreSQL 18, pgx, sqlc, goose, River)

## 1. Verdict

The data-layer material is current and better than most guidance I have seen. pgx v5, sqlc, goose and River are the mainstream Go/PostgreSQL stack in 2026. `Update(fn)` as the default, with a transaction provider only for edge cases, is exactly what Three Dots Labs recommended in 2024. The migration rules and the test harness match strong_migrations and pgtestdb.

The risks are where the pieces meet:
- `app migrate` inherits the application's pool, timeouts and database role.
- River's migrations run outside any lock.
- River job args are stored as JSON, but our rules forbid JSON tags in inner layers.
- sqlc v1.31.1 parses PostgreSQL 17 grammar, which quietly rules out several PG18 features.

The reference slice lags the skill and has no database test, so agents that "follow the reference" copy the weaker version.

*Assumptions:* production runs a Debian/glibc PostgreSQL image, there is no PgBouncer at launch, `api` and `worker` are deployed separately, and CI has a PostgreSQL server.

## 2. Confirmed

- **Transactions:** `Update(ctx, id, fn)` as the default, a provider for several aggregates, and no transaction in `context.Context`. Three Dots Labs calls UpdateFn the "go-to solution" and says putting the tx in context "only masks the root issue". Context-based managers such as avito-tech/go-transaction-manager v2.0.5 are popular in Russian teams, but explicit tx values suit AI-written code better.
- **River `InsertTx` as the outbox, with an insert-only client in `api`.** River's own `InsertTx` doc says the job is invisible until commit and rolls back with the transaction. River is still the best fit:
  - Hatchet, Asynq and Temporal cannot enqueue in the same transaction.
  - DBOS Transact Go v1.8 is a heavier durable-workflow model.
- **goose v3:** embedded files, Provider API, session locker, `NO TRANSACTION` for concurrent index builds, and migrations as a deploy step before new code. This matches River's "migrate before starting workers". The alternatives do not beat it here:
  - Atlas lints best, but `migrate lint` has been Pro-only since v0.38.
  - golang-migrate, tern and dbmate add nothing.
  - pgroll's migration files are invisible to sqlc.
- **Safe-change table:** `lock_timeout`, `CREATE INDEX CONCURRENTLY` with the INVALID-index caveat, `NOT VALID` then `VALIDATE`, NOT NULL via a validated CHECK, fast column defaults (PG11+), expand/contract, batched backfills.
- **App connection settings:** `statement_timeout` and `idle_in_transaction_session_timeout`, the DSN never echoed in errors, and the PgBouncer notes (`max_prepared_statements` now defaults to 200).
- **Explicit column lists.** They also avoid pgx's "cached plan must not change result type" errors after `ADD COLUMN`.
- Named constraints matched by `ConstraintName`, a partial unique index for dedup, keyset pagination, indexed foreign keys.
- **Tests on real PostgreSQL**, one database per test cloned from a template (the pgtestdb technique).
- **IDs and types:** UUIDv7 from the stdlib, which is monotonic in-process per the Go 1.27 docs; `timestamptz`; text plus CHECK; money as bigint minor units.
- **One schema per module, no reads across modules.** This is mainstream modular-monolith guidance. Banning cross-module foreign keys is a preference, not consensus: Jovanović keeps them and drops them when a module is extracted.

## 3. Problems

**P1. `app migrate` inherits the app's pool, timeouts and role.** High severity / medium confidence.
- **Where:** postgres-patterns §Pool, where `Open` puts `statement_timeout` into `RuntimeParams` (line 45), and `Migrate(ctx, pool, …)` (lines 353–376). The reference `cmd/api/main.go:57-64` reuses the app pool and `DATABASE_URL`.
- **Evidence:**
  - Wired as written, a 5 s app timeout kills `CREATE INDEX CONCURRENTLY` or `VALIDATE` on any real table.
  - The failed build leaves an INVALID index, and the rerun's `IF NOT EXISTS` skips it.
  - strong_migrations uses `lock_timeout` 10 s and `statement_timeout` 1 h for migrations, against 15 s for the app.
  - Sharing one DSN also gives the runtime role DDL rights.
- **Fix:**
  - `migrate` opens its own direct connection (never through PgBouncer) from `MIGRATE_DATABASE_URL`, using the schema-owner role, with `lock_timeout=5s` and `statement_timeout=0` (or 1 h).
  - The runtime role gets DML only, through grants plus `ALTER DEFAULT PRIVILEGES` on each module schema.
  - After `Up`, fail if `pg_index` has any row with `NOT indisvalid`.

**P2. Job args are stored JSON, but the rules ban JSON tags.** Medium / high.
- **Where:** STACK §2 (line 64, "Inner layers never see … JSON tags"; args live in `usecase`, line 81); postgres-patterns §UnitOfWork.
- **Evidence:**
  - River serializes untagged fields under their Go names, so a routine field rename makes queued and retrying jobs decode zero values.
  - That window is long: 25 attempts take about 3 weeks, and discarded jobs are kept 7 days.
  - Uniqueness by a subset of args needs `river:"unique"` tags.
  - Kind renames already have a mechanism, `JobArgsWithKindAliases`.
- **Fix:**
  - Allow `json` tags on job args as a documented exception, and evolve them add-only.
  - Add a golden-JSON test for each args type.
  - Put IDs only in args, never personal data.
  - Rename kinds through `KindAliases`.

**P3. sqlc v1.31.1 parses PostgreSQL 17 grammar, not 18.** Medium / high for the grammar version, medium for the exact failure of each construct.
- **Evidence:**
  - sqlc v1.31.1 (2026-04-22, the latest release) uses pg_query_go v6.2.2. Its `pg_config.h` says `PG_VERSION "17.7"`, and its keyword list has no `virtual`, `enforced` or `period`.
  - proxy.golang.org has no pg_query_go v7.
  - sqlc parses every migration file, so PG18-only DDL breaks `sqlc generate`:
    - `GENERATED … VIRTUAL`, or a generated column without `STORED` (virtual is now PG18's default);
    - `ADD CONSTRAINT … NOT NULL col NOT VALID`;
    - `NOT ENFORCED`;
    - `WITHOUT OVERLAPS` and `PERIOD`.
  - `RETURNING old/new` in queries will also fail.
- **Fix:**
  - Add a rule: the server is PG18, but files sqlc reads use the PG17 dialect.
  - Keep the CHECK-based NOT NULL recipe; PG18's two-step syntax can't be parsed.
  - Always write `STORED` on generated columns.
  - Recheck on every sqlc release.

**P4. River migrations run unlocked.** Medium / high.
- **Where:** postgres-patterns `Migrate`; the reference `migrate()` has neither a locker nor River.
- **Evidence:**
  - `rivermigrate` v0.51.2 takes no lock, and goose releases its advisory lock when `Up` returns.
  - River's docs say to wrap migrations in an advisory lock, and document pairing with goose: a Go migration using `riverdatabasesql` with a pinned `TargetVersion`, or SQL exported by `river migrate-get`.
  - River is 0.x: 12 minor releases since July 2026. 0.48 changed unique keys, 0.49 changed savepoint handling, and 0.50 has a breaking change.
- **Fix:**
  - Run River's versions as goose migrations. That gives one history and one lock, squawk lints them, and River upgrades show up in review.
  - Pin River, read the ⚠️ lines in its CHANGELOG, and migrate before deploying.

**P5. The reference lags the skill and has no database test.** Medium / high.
- **Where:** in the reference,
  - `postgres.go` sets no timeouts;
  - `main.go` has no locker and no River;
  - `repository.go` works on the pool only and has no `mapError`;
  - `sqlc.yaml` has no `emit_pointers_for_null_types` and no nullable overrides.

  `new-module.md` tells agents to copy this reference.
- **Evidence:** three behaviors are claimed only in prose:
  - the stdlib `uuid` round trip through pgx;
  - `FOR UPDATE` serializing concurrent updates;
  - a job existing only after commit.
- **Fix:** align the reference with the skill, and add the `pgtest` harness plus three adapter tests covering those behaviors.

**P6. Worker failure policy is "return its error; River retries".** Medium / high.
- **Where:** go-clean-architecture §delivery/worker (lines 74–75).
- **Evidence:**
  - With River's defaults (25 attempts, 1-minute timeout), a permanent error such as a deleted lead retries for weeks.
  - River already offers `JobCancel`, per-kind attempts, timeouts and queues, `FetchOnlyKnownKinds` (0.48) for kinds a worker doesn't know yet, and `JobCompleteTx`.
- **Fix:**
  - Map `apperr` kinds: NotFound, Invalid and Conflict become `river.JobCancel`; everything else retries.
  - Keep per-kind insert options (queue, attempts, uniqueness) in the adapter.
  - Deploy workers before any API that enqueues a new kind.

**P7. Every `Update` inside a UoW opens a savepoint, and UoW is the default for "state change + job".** Low-medium / medium.
- **Where:** postgres-patterns lines 141–146 and 272; go-clean-architecture §Transactions.
- **Evidence:**
  - Three Dots Labs calls the transaction provider "a risky pattern… don't over-use it", and says every SELECT inside it needs `FOR UPDATE`.
  - River 0.49 stopped creating savepoints inside caller transactions.
  - GitLab removed all SAVEPOINTs after subtransaction stalls on replicas.
  - Each savepoint costs two round trips and buys nothing, because `Do` rolls back as a whole.
- **Fix:**
  - Repositories built on a transaction run directly on it, without a savepoint.
  - Inside `Do`, reads that drive a decision must lock the row.
  - Consider letting aggregates record events that `Update`/`Create` persist with `InsertManyTx`.

**P8. `FOR UPDATE` where `FOR NO KEY UPDATE` is enough.** Low / high.
- **Where:** `GetLeadForUpdate` (skill and reference), rules/sql.md.
- **Evidence:** per the PostgreSQL docs, `FOR UPDATE` also blocks foreign-key checks (`FOR KEY SHARE`), so inserting child rows that reference a locked lead waits.
- **Fix:** default to `FOR NO KEY UPDATE`; use `FOR UPDATE` only when deleting or changing keys.

**P9. sqlc tooling.** Low-medium / high.
- **Evidence:**
  - Because the schema is the whole `migrations` directory, every module's `models.go` gets every module's tables unless `omit_unused_structs: true` is set.
  - Nothing stops a `leads` query from reading `billing.*`.
  - `sqlc vet` has no rules, so the reviewer's `bin/sqlc vet` is a no-op.
  - The `sqlc/db-prepare` rule, with `database: managed: true` and a `servers:` entry, prepares every query against your own PG18. No cloud is involved.
  - `sqlc verify` uploads schema and queries to sqlc Cloud; skip it.
  - "sqlc needs cgo" is outdated: v1.31.1 builds with `CGO_ENABLED=0` using a WASM parser (`parse_wasi.go`).
- **Fix:** set `omit_unused_structs: true`, add `rules: [sqlc/db-prepare, <module>-own-schema]` (a CEL `query.sql.matches`), and run `sqlc vet` in `make check`.

**P10. The safe-migration table misses some lock traps.** Low-medium / high.
- **Evidence:**
  - goose runs each file as one transaction, so locks accumulate until COMMIT. A quick `ACCESS EXCLUSIVE` statement followed by a slow one in the same file blocks the table for the whole file.
  - strong_migrations also covers unique constraints (`CREATE UNIQUE INDEX CONCURRENTLY`, then `ADD CONSTRAINT … USING INDEX`).
  - It also flags stored generated and identity columns (both rewrite the table), exclusion constraints, and retries on lock timeout.
- **Fix:** add these rows, plus the rule "one hot-table DDL per file".

**P11. Sequential migration numbers in parallel branches.** Low-medium / high.
- **Where:** `goose -s create` (postgres-patterns line 349).
- **Evidence:**
  - goose's README "strongly recommend[s]" timestamps during development and `goose fix` in CI.
  - With parallel agent branches, two PRs each pass CI with the same `000NN`, and main breaks after the second merge.
- **Fix:** use timestamps and run `goose fix` at merge, or add a CI check that new versions are unique.

**P12. Pool, timeouts and error codes.** Low / high.
- **Missing settings:**
  - Add PG17's `transaction_timeout`, which also stops long transactions that block vacuum on `river_job`.
  - Add an app `lock_timeout` (strong_migrations uses 10 s).
- **Wording:** "the ctx deadline cancels the query" is wrong. pgx's default `DeadlineContextWatcherHandler` closes the connection, and the server keeps running the query until `statement_timeout` or `client_connection_check_interval`.
- **Pool sizing:** size pools from database vCPUs and pool-wait metrics, not from `max_connections`, and set `PingTimeout`.
- **Error codes:** PG18 reports `ON DELETE RESTRICT` violations as `23001`.

**P13. Test image and collation.** Low-medium / medium.
- **Where:** golang-testing line 213 (`postgres:18-alpine`).
- **Evidence:**
  - The official image's docs say Alpine variants have no libc locales, only ICU.
  - If production sorts Cyrillic with a glibc `ru_RU` collation, tests can't reproduce it.
  - glibc upgrades can change collations under existing indexes.
- **Fix:** choose the collation provider at `CREATE DATABASE` (ICU `ru-RU`, or PG17's builtin provider), and run tests on the production image family.

## 4. Gaps

- **Now — money flowing between modules.** Leads and billing can't share a transaction, and nothing shows how to make the hand-off safe:
  - an outbox job with an idempotent consumer: a `UNIQUE(lead_id)` charge row, `INSERT … ON CONFLICT DO NOTHING` as `:execrows`, or `JobCompleteTx`;
  - invariants across rows: `UPDATE … WHERE balance >= $2`, `CHECK (balance >= 0)`, a lock on the parent row, or an append-only ledger.
- **Now — squawk configured for goose:**
  - `pg_version = "18"` and `assume_in_transaction = true`;
  - `-- squawk-disable-assume-in-transaction` in `NO TRANSACTION` files;
  - lint only added files;
  - strip `-- +goose Down` sections before linting, because the `ban-drop-*` rules fire on them;
  - exclude `require-lock-timeout` and `require-statement-timeout` once the migration connection sets both.
- **Now — observability:**
  - `otelpgx`: its tracer, and `RecordStats` for pool waits;
  - `otelriver` middleware;
  - `pg_stat_statements`;
  - River UI behind auth, since it shows job args;
  - alerts on discarded jobs and queue lag.
- **Now — personal data (152-FZ):**
  - River keeps completed jobs for 24 h and discarded jobs for 7 days, error text included.
  - Keep PII out of error strings and out of PostgreSQL logs (`log_parameter_max_length`).
  - Run a retention/anonymization job for leads.
- **Now — PostgreSQL 17/18 adoption:**
  - `transaction_timeout`;
  - a builtin or ICU collation;
  - B-tree skip scan, which can replace redundant composite indexes;
  - optionally `DEFAULT uuidv7()` for rows created by SQL scripts;
  - SCRAM only, since MD5 is deprecated.

  Avoid virtual generated columns: they can't be indexed, and a bug report on 2026-10-29 lists wrong results on 18.6.
- **Later — reporting.** Reads across schemas are forbidden, but no alternative is given. Options: a read-only role on a replica, or a reporting module fed by jobs.
- **Later — test speed:**
  - templates keyed by a hash of the migrations and reused across packages and runs, as pgtestdb does;
  - keep failed tests' databases for debugging;
  - run the test server on tmpfs with `fsync=off`;
  - use one shared server instead of a container per package.
- **Later — bulk work and pooling:**
  - batch queries with `:batchexec`;
  - `:copyfrom` (it has no `ON CONFLICT`, so use `unnest` upserts for those);
  - backfills as River resumable jobs;
  - if PgBouncer comes, River's runtime still needs a direct or session-pooled connection.
- **Later — IDs in URLs.** Use slugs for SEO pages. A v7 ID reveals its creation time and must never grant access (RFC 9562 §8), so access links get `crypto/rand` tokens.

## 5. Over-engineering

Little.
- **Down migrations "for local development":** they cost upkeep, go untested and trip squawk's drop rules, while template databases make them unnecessary. Make them optional.
- **Savepoint-capable repositories (P7):** they add round trips for no gain.

## 6. Sources

**Transactions**
- https://threedots.tech/post/database-transactions-in-go/ — Database Transactions in Go with Layered Architecture — Miłosz Smółka, Three Dots Labs — 2024-09-05
- https://about.gitlab.com/blog/2021/09/29/why-we-spent-the-last-month-eliminating-postgresql-subtransactions/ — Why we spent the last month eliminating PostgreSQL subtransactions — GitLab — 2021-09-29
- https://github.com/avito-tech/go-transaction-manager — go-transaction-manager — Avito — v2.0.5, 2026-09-26

**River**
- https://riverqueue.com/docs/migrations, /docs/unique-jobs, /docs/transactional-job-completion, /docs/pgbouncer, /docs/pro/encrypted-jobs — River docs — read 2026-11-13
- https://github.com/riverqueue/river — CHANGELOG and v0.51.2 source (rivermigrate, defaults) — River maintainers — 2026-11-09
- https://github.com/riverqueue/rivercontrib — otelriver — River maintainers — v0.12.0, 2026-07-24
- https://github.com/dbos-inc/dbos-transact-golang — DBOS Transact Go — DBOS — v1.8.0, 2026-10-28

**pgx, goose, sqlc**
- https://github.com/jackc/pgx — CHANGELOG and source (`DeadlineContextWatcherHandler`, savepoints) — Jack Christensen — v5.13.0, 2026-11-11
- https://github.com/exaring/otelpgx — otelpgx — Exaring — v0.13.0, 2026-10-08
- https://github.com/pressly/goose — README ("Hybrid Versioning") and lock package — Pressly — v3.30.1, 2026-11-12
- https://docs.sqlc.dev — config, vet, managed databases, verify; v1.31.1 `go.mod` and `parse_wasi.go` — sqlc — 2026-04-22
- https://github.com/pganalyze/pg_query_go — v6.2.2 (PG 17.7); proxy.golang.org has no v7 (checked 2026-11-13) — pganalyze

**PostgreSQL**
- https://www.postgresql.org/docs/release/18.0/ — PostgreSQL 18 release notes — PGDG — 2025-09-25
- https://www.postgresql.org/docs/release/17.0/ — PostgreSQL 17 release notes — PGDG — 2024-09-26
- https://www.postgresql.org/docs/18/ — explicit-locking, sql-altertable, sql-createindex, runtime-config-client, runtime-config-resource, ddl-generated-columns — PGDG
- https://www.postgresql.org/message-id/19812-32c98d4a667ac18f%40postgresql.org — BUG #19812, virtual generated columns — 2026-10-29

**Migration linting**
- https://github.com/ankane/strong_migrations — strong_migrations README — Andrew Kane — read 2026-11-13
- https://squawkhq.com/docs/, /docs/cli, /docs/require-timeout-settings — Squawk docs — squawk-cli 2.72.0, 2026-11-08
- https://atlasgo.io/versioned/lint — Atlas lint docs (Pro-only since v0.38) — Ariga

**Infrastructure and testing**
- https://www.pgbouncer.org/config.html — PgBouncer configuration — PgBouncer project
- https://github.com/peterldowns/pgtestdb — pgtestdb README — Peter Downs — v0.1.1, 2024-10-15
- https://hub.docker.com/_/postgres — official postgres image docs (Alpine locale caveat) — Docker

**IDs and module boundaries**
- https://www.rfc-editor.org/rfc/rfc9562.html — RFC 9562 — IETF — 2024-05
- `go doc uuid` — Go 1.27.4 standard library
- https://milanjovanovic.tech/blog/modular-monolith-data-isolation — Modular Monolith Data Isolation — Milan Jovanović — 2023-12-09
