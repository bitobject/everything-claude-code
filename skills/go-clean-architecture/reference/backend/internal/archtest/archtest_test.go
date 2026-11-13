// Package archtest checks the rules go-arch-lint can't see, in the style of the Go project's
// own go/build deps test. go-arch-lint checks imports between components; these check what is
// inside them:
//   - entity and usecase declare no struct tags: no JSON, DB or validation concerns inside;
//   - entity.Restore* is called only by repository adapters (it skips the creation rules);
//   - a module's SQL names no other module's schema, nor River's (sqlc sees every schema).
//
// The standard library imports allowed in entity and usecase are checked by depguard
// (.golangci.yml), because go-arch-lint always allows the standard library.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const internalDir = ".."

// goFiles returns the non-test Go files under dir.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func parse(t *testing.T, path string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// modules returns the business modules: directories under internal/ other than the shared ones.
func modules(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Fatal(err)
	}
	shared := map[string]bool{"api": true, "archtest": true, "gen": true, "kernel": true, "platform": true}
	var mods []string
	for _, e := range entries {
		if e.IsDir() && !shared[e.Name()] {
			mods = append(mods, e.Name())
		}
	}
	return mods
}

func TestInnerLayersHaveNoStructTags(t *testing.T) {
	for _, mod := range modules(t) {
		for _, layer := range []string{"entity", "usecase"} {
			dir := filepath.Join(internalDir, mod, layer)
			if _, err := os.Stat(dir); err != nil {
				continue // a thin module has no layers
			}
			for _, path := range goFiles(t, dir) {
				if strings.Contains(path, "test"+string(filepath.Separator)) {
					continue // test helpers (usecasetest)
				}
				ast.Inspect(parse(t, path), func(n ast.Node) bool {
					if f, ok := n.(*ast.Field); ok && f.Tag != nil {
						t.Errorf("%s: struct tag %s: inner layers carry no json/db tags; map in an adapter", path, f.Tag.Value)
					}
					return true
				})
			}
		}
	}
}

func TestRestoreIsCalledOnlyByRepositories(t *testing.T) {
	for _, path := range goFiles(t, internalDir) {
		if strings.Contains(path, string(filepath.Separator)+"repository"+string(filepath.Separator)) {
			continue
		}
		f := parse(t, path)
		entityNames := map[string]bool{}
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); strings.HasSuffix(p, "/entity") {
				name := "entity"
				if imp.Name != nil {
					name = imp.Name.Name
				}
				entityNames[name] = true
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); ok && entityNames[x.Name] && strings.HasPrefix(sel.Sel.Name, "Restore") {
				t.Errorf("%s: %s.%s outside a repository adapter: create aggregates with New*, rebuild them only in repository/", path, x.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// qualified matches schema-qualified names: leads.leads, billing.charges, river.river_job.
var qualified = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)\b`)

func TestQueriesStayInTheirModuleSchema(t *testing.T) {
	mods := modules(t)
	foreign := map[string]bool{"river": true}
	for _, m := range mods {
		foreign[m] = true
	}
	for _, mod := range mods {
		err := filepath.WalkDir(filepath.Join(internalDir, mod), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".sql" {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(b), "\n") {
				line, _, _ = strings.Cut(line, "--") // comments may name anything
				for _, m := range qualified.FindAllStringSubmatch(line, -1) {
					if schema := strings.ToLower(m[1]); foreign[schema] && schema != mod {
						t.Errorf("%s:%d: %s reads or writes schema %q: a module uses only its own schema; call the other module's contract", path, i+1, mod, schema)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
