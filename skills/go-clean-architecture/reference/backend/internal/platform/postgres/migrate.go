package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrate applies the pending goose migrations in fsys (the application's and River's: one
// history, one lock). dsn is the schema owner's (MIGRATE_DATABASE_URL), never the application
// role's, which may only read and write rows. The session waits at most 5 s for a lock, so a DDL
// statement stuck behind a long transaction fails instead of queueing every query behind it,
// and has no statement timeout, so CREATE INDEX CONCURRENTLY can finish. Afterwards it fails if
// any index is INVALID: a failed concurrent build leaves one, and IF NOT EXISTS would skip it.
func Migrate(ctx context.Context, dsn string, fsys fs.FS, log *slog.Logger) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return errors.New("parse migration database config: invalid DSN") // may echo the password
	}
	cfg.RuntimeParams["application_name"] = "migrate"
	cfg.RuntimeParams["timezone"] = "UTC"
	cfg.RuntimeParams["lock_timeout"] = "5000"
	cfg.RuntimeParams["statement_timeout"] = "0"
	cfg.RuntimeParams["transaction_timeout"] = "0"

	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()

	locker, err := lock.NewPostgresSessionLocker() // advisory lock: one migrator at a time
	if err != nil {
		return fmt.Errorf("goose locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	for _, r := range results {
		log.InfoContext(ctx, "migrated", "version", r.Source.Version, "duration", r.Duration)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT n.nspname || '.' || c.relname
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT i.indisvalid
		ORDER BY 1`)
	if err != nil {
		return fmt.Errorf("check indexes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var invalid []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("check indexes: %w", err)
		}
		invalid = append(invalid, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check indexes: %w", err)
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid indexes after migrating (drop and rebuild them): %s", strings.Join(invalid, ", "))
	}
	return nil
}
