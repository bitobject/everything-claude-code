// Package pgtest gives each test a PostgreSQL database of its own, cloned from a template that
// is migrated once per set of migrations, so tests run in parallel and never see each other's
// rows.
//
// TEST_DATABASE_URL is the schema owner's DSN (the role that runs migrations, allowed to CREATE
// DATABASE) on a PostgreSQL 18 server; the README shows the one-time role setup. The pool New
// returns switches to the application role (TEST_APP_ROLE, default app_runtime) with SET ROLE,
// so tests run with the privileges production has and a missing GRANT fails a test.
//
// With -short these tests are skipped (the local fast loop); without TEST_DATABASE_URL they
// fail, so CI can't pass by silently not running them.
package pgtest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/app/internal/platform/postgres"
)

// templateLock serializes template builds across test processes (go test runs packages in
// parallel).
const templateLock = 7236001

// New returns a pool on a fresh database with fsys migrated, closed and dropped when t ends.
func New(t testing.TB, fsys fs.FS) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("needs PostgreSQL: run without -short, with TEST_DATABASE_URL set")
	}
	owner := os.Getenv("TEST_DATABASE_URL")
	if owner == "" {
		t.Fatal("TEST_DATABASE_URL is not set: point it at PostgreSQL 18 (see README), or run go test -short")
	}
	appRole := os.Getenv("TEST_APP_ROLE")
	if appRole == "" {
		appRole = "app_runtime"
	}
	ctx := t.Context()

	admin, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatalf("pgtest: connect as the owner: %v", err)
	}
	defer func() { _ = admin.Close(context.WithoutCancel(ctx)) }()

	tpl := templateName(t, fsys)
	ensureTemplate(t, admin, owner, tpl, fsys)

	name := "pgtest_" + randomHex(t)
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident+" TEMPLATE "+pgx.Identifier{tpl}.Sanitize()); err != nil {
		t.Fatalf("pgtest: clone %s: %v", tpl, err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.WithoutCancel(ctx), owner)
		if err != nil {
			t.Errorf("pgtest: drop %s: %v", name, err)
			return
		}
		defer func() { _ = c.Close(context.WithoutCancel(ctx)) }()
		if _, err := c.Exec(context.WithoutCancel(ctx), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("pgtest: drop %s: %v", name, err)
		}
	})

	cfg := postgres.DefaultConfig("test")
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+pgx.Identifier{appRole}.Sanitize())
		return err
	}
	pool, err := postgres.Open(ctx, withDatabase(t, owner, name), cfg)
	if err != nil {
		t.Fatalf("pgtest: open %s: %v", name, err)
	}
	t.Cleanup(pool.Close) // runs before the drop above (cleanups run last-in, first-out)
	return pool
}

// ensureTemplate builds tpl unless it exists. It builds under another name and renames at the
// end, so a template is either complete or absent.
func ensureTemplate(t testing.TB, admin *pgx.Conn, owner, tpl string, fsys fs.FS) {
	t.Helper()
	ctx := t.Context()
	if _, err := admin.Exec(ctx, "SELECT pg_advisory_lock($1)", templateLock); err != nil {
		t.Fatalf("pgtest: lock the template: %v", err)
	}
	defer func() { _, _ = admin.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", templateLock) }()

	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", tpl).Scan(&exists); err != nil {
		t.Fatalf("pgtest: look for the template: %v", err)
	}
	if exists {
		return
	}
	building := tpl + "_building"
	bIdent := pgx.Identifier{building}.Sanitize()
	for _, stmt := range []string{"DROP DATABASE IF EXISTS " + bIdent + " WITH (FORCE)", "CREATE DATABASE " + bIdent} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("pgtest: prepare the template: %v", err)
		}
	}
	if err := postgres.Migrate(ctx, withDatabase(t, owner, building), fsys, slog.New(slog.DiscardHandler)); err != nil {
		_, _ = admin.Exec(context.WithoutCancel(ctx), "DROP DATABASE IF EXISTS "+bIdent+" WITH (FORCE)")
		t.Fatalf("pgtest: migrate the template: %v", err)
	}
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+bIdent+" RENAME TO "+pgx.Identifier{tpl}.Sanitize()); err != nil {
		t.Fatalf("pgtest: finish the template: %v", err)
	}
}

// templateName changes whenever a migration file changes, so a stale template is never used.
func templateName(t testing.TB, fsys fs.FS) string {
	t.Helper()
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", path, len(b))
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		t.Fatalf("pgtest: read migrations: %v", err)
	}
	return "pgtest_tpl_" + hex.EncodeToString(h.Sum(nil))[:16]
}

func withDatabase(t testing.TB, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("pgtest: TEST_DATABASE_URL is not a URL") // the error may echo the password
	}
	u.Path = "/" + name
	return u.String()
}

func randomHex(t testing.TB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
