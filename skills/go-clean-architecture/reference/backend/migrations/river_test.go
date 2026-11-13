package migrations_test

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"example.com/app/migrations"
)

var riverFile = regexp.MustCompile(`^\d+_river_(\d{3})\.sql$`)

// TestRiverMigrationsAreRiversSQL: the river_* files are exactly River's migrations for the River
// version in go.mod, so a River upgrade that adds or changes a migration fails here until
// `go run ./cmd/rivermigrations` has written it.
func TestRiverMigrationsAreRiversSQL(t *testing.T) {
	m, err := rivermigrate.New[pgx.Tx](riverpgxv5.New(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]string{}
	for _, v := range m.AllVersions() {
		if v.Version != 1 { // River's own river_migration table: not used
			want[v.Version] = []string{withSchema(v.SQLUp), withSchema(v.SQLDown)}
		}
	}
	files, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		match := riverFile.FindStringSubmatch(f.Name())
		if match == nil {
			continue
		}
		version, _ := strconv.Atoi(match[1])
		body, err := fs.ReadFile(migrations.FS, f.Name())
		if err != nil {
			t.Fatal(err)
		}
		river, ok := want[version]
		if !ok {
			t.Errorf("%s: River has no migration %03d", f.Name(), version)
			continue
		}
		delete(want, version)
		if got := statementBlocks(string(body)); len(got) != 2 || got[0] != river[0] || got[1] != river[1] {
			t.Errorf("%s differs from River's migration %03d: restore it (River never changes a released migration, and an applied one is never edited)", f.Name(), version)
		}
	}
	for version := range want {
		t.Errorf("River's migration %03d is missing: run `go run ./cmd/rivermigrations` in backend/", version)
	}
}

func withSchema(sql string) string {
	return strings.TrimSpace(strings.ReplaceAll(sql, "/* TEMPLATE: schema */", "river."))
}

// statementBlocks returns the bodies between goose's StatementBegin and StatementEnd markers.
func statementBlocks(s string) []string {
	var blocks []string
	for {
		_, rest, ok := strings.Cut(s, "-- +goose StatementBegin\n")
		if !ok {
			return blocks
		}
		block, after, ok := strings.Cut(rest, "-- +goose StatementEnd")
		if !ok {
			return blocks
		}
		blocks = append(blocks, strings.TrimSpace(block))
		s = after
	}
}
