// Command rivermigrations writes the River migrations that migrations/ lacks as goose files,
// numbered after the last migration. Run it from backend/ after upgrading River:
//
//	go run ./cmd/rivermigrations
//
// River's SQL is copied unchanged, with its schema placeholder set to river, and goose applies
// it in the same history and under the same lock as the application's migrations.
// migrations/river_test.go fails while a version is missing or differs. River's migration 001
// (its own river_migration table) is skipped: only River's migrator would use it.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const (
	dir         = "migrations"
	placeholder = "/* TEMPLATE: schema */"
	driver      = "github.com/riverqueue/river/riverdriver/riverpgxv5"
)

var (
	sequential = regexp.MustCompile(`^(\d{1,11})_.*\.sql$`) // a timestamp-named file has 12+ digits
	riverFile  = regexp.MustCompile(`^\d+_river_(\d{3})\.sql$`)
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rivermigrations:", err)
		os.Exit(1)
	}
}

func run() error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("run from backend/: %w", err)
	}
	last, have := 0, map[int]bool{}
	for _, e := range entries {
		if m := sequential.FindStringSubmatch(e.Name()); m != nil {
			n, _ := strconv.Atoi(m[1])
			last = max(last, n)
		}
		if m := riverFile.FindStringSubmatch(e.Name()); m != nil {
			v, _ := strconv.Atoi(m[1])
			have[v] = true
		}
	}

	m, err := rivermigrate.New[pgx.Tx](riverpgxv5.New(nil), nil)
	if err != nil {
		return err
	}
	written := 0
	for _, v := range m.AllVersions() {
		if v.Version == 1 || have[v.Version] {
			continue
		}
		up, down := strings.ReplaceAll(v.SQLUp, placeholder, "river."), strings.ReplaceAll(v.SQLDown, placeholder, "river.")
		if strings.Contains(up+down, "TEMPLATE") {
			return fmt.Errorf("migration %03d of River has a placeholder this tool doesn't know", v.Version)
		}
		last++
		name := filepath.Join(dir, fmt.Sprintf("%05d_river_%03d.sql", last, v.Version))
		body := fmt.Sprintf(`-- River migration %03d (main line, %s), from %s
-- %s, migration/main/%03d_*.sql. River's SQL is unchanged except that its schema placeholder is
-- set to river. Written by cmd/rivermigrations; never edit an applied one.
-- squawk skips river_* files (.squawk.toml): the SQL is River's.

-- +goose Up
-- +goose StatementBegin
%s
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
%s
-- +goose StatementEnd
`, v.Version, v.Name, driver, riverVersion(), v.Version, strings.TrimSpace(up), strings.TrimSpace(down))
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil { //nolint:gosec // a source file for the repository, not a secret
			return err
		}
		fmt.Fprintln(os.Stdout, "wrote", name)
		written++
	}
	if written == 0 {
		fmt.Fprintln(os.Stdout, "migrations/ has every River migration")
	}
	return nil
}

// riverVersion is the River driver version in go.mod.
func riverVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == driver {
				return d.Version
			}
		}
	}
	return "(unknown version)"
}
