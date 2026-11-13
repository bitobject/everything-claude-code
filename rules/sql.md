---
paths:
  - "backend/migrations/**"
  - "backend/**/queries/*.sql"
  - "backend/sqlc.yaml"
---
# SQL, migrations and sqlc

Detail: skill `postgres-patterns`; reviewer agent `database-reviewer`.

- **One schema per module** (`leads.`, `billing.`); every SQL statement names the schema. A module's
  queries touch only its own schema (`internal/archtest` fails otherwise).
- **Migrations are goose SQL files**, one sequence, embedded in the binary, run by `api migrate`
  under the owner role. Create one with `make migration name=<snake_case>` (a timestamped name);
  `make migrations-fix` renumbers it right before merging. Never edit a migration that has been
  applied anywhere; add a new one. River's migrations are written by `go run ./cmd/rivermigrations`.
- **Breaking changes are staged**: expand (add nullable column / new table) → backfill in batches →
  switch code → contract (drop) in a later release.
- **Locks**: the migration session has `lock_timeout = 5s`; one DDL statement on a hot table per
  file; indexes on existing tables with `CREATE INDEX CONCURRENTLY` in a `-- +goose NO
  TRANSACTION` file; a unique constraint via `CREATE UNIQUE INDEX CONCURRENTLY` + `USING INDEX`.
- **sqlc reads PostgreSQL 17 syntax**: no virtual generated columns (`STORED`), no `NOT ENFORCED`,
  no `WITHOUT OVERLAPS`, no `RETURNING old/new` in migrations or queries.
- **Constraints in the database**: `NOT NULL`, `CHECK`, `UNIQUE`, foreign keys within a module's
  schema, all named. They back the entity invariants; they don't replace them.
- **Types**: `uuid` keys (UUIDv7 generated in Go), `timestamptz` for instants, `bigint` minor units
  for money, `text` + `CHECK` for enums.
- **Privileges**: the application role gets DML only (default privileges); an append-only table
  revokes `UPDATE` and `DELETE` from it in the migration that creates it.
- **Queries** live in `queries/*.sql` with sqlc annotations; after any change run `make generate`
  and commit the output. Explicit columns, no `SELECT *`. Lists are keyset-paginated with a
  bounded `LIMIT`. Rows read for change: `FOR NO KEY UPDATE`, only inside the repository's
  transaction; the aggregate's update writes every changeable column.
- Never run a migration against a shared or production database without the user's explicit approval.
