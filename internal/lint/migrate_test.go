package lint

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrateDir writes src as the file example.go of a new directory inside
// testdata (it must be in the module to import its packages) and returns the
// directory, as a pattern, and the file.
func migrateDir(t *testing.T, src string) (pattern, file string) {
	t.Helper()
	tmp, err := os.MkdirTemp("testdata", "mig-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	file = filepath.Join(tmp, "example.go")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return "./" + filepath.ToSlash(tmp), file
}

// gofmtOf formats a source, so that a comparison does not depend on how a
// raw string was indented.
func gofmtOf(t *testing.T, src string) string {
	t.Helper()
	out, err := format.Source([]byte(src))
	if err != nil {
		t.Fatalf("the example does not parse: %v\n%s", err, src)
	}
	return string(out)
}

// checkMigration runs the example of m: the old code reports the migrate
// rule, -fix rewrites it into the new code, and the new code compiles and
// lints clean.
func checkMigration(t *testing.T, m Migration, opts Options) {
	t.Helper()
	pattern, file := migrateDir(t, m.Example.Old)
	opts.Patterns = []string{pattern}

	res, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("the old code of the example reports nothing (load errors %v)", res.LoadErrors)
	}
	for _, d := range res.Diagnostics {
		if d.Rule != RuleMigrate || !d.Fixable || !strings.Contains(d.Message, "migrate: "+m.ID) {
			t.Errorf("finding %s: want a fixable migrate finding that names %q", d, m.ID)
		}
	}

	opts.Fix = true
	res, err = Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.FixError != nil || len(res.Diagnostics) != 0 || len(res.LoadErrors) != 0 {
		t.Errorf("after -fix: fix error %v, findings %v, load errors %v; want none", res.FixError, res.Diagnostics, res.LoadErrors)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if want := gofmtOf(t, m.Example.New); string(got) != want {
		t.Errorf("-fix gives\n%s\nwant\n%s", got, want)
	}

	// The new code compiles (a load error otherwise) and has no findings.
	opts.Fix = false
	pattern, _ = migrateDir(t, m.Example.New)
	opts.Patterns = []string{pattern}
	res, err = Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) != 0 || len(res.Diagnostics) != 0 {
		t.Errorf("the new code of the example: findings %v, load errors %v; want none", res.Diagnostics, res.LoadErrors)
	}
}

// Every migration of the table has a sound entry and an example that -fix
// turns into compiling code.
func TestMigrations(t *testing.T) {
	for _, p := range migrationProblems(Migrations) {
		t.Error(p)
	}
	if len(Migrations) == 0 {
		t.Fatal("the table has no migrations: the breaking changes since v0.14.0 have them")
	}
	for _, m := range Migrations {
		t.Run(m.ID, func(t *testing.T) {
			parallelLint(t)
			needGo(t)
			checkMigration(t, m, Options{})
		})
	}
}

// The kinds of rewrite, on a package of the fixtures whose old spellings are
// the ones of the table below.
var testMigrations = []Migration{
	{
		ID: "widget-label", Since: "v0.1.0",
		Member:  &MemberRename{Pkg: "internal/lint/testdata/migapi", Type: "Widget", Old: "Label", New: "Title"},
		Example: Example{Old: "x", New: "x"},
	},
	{
		ID: "widget-display", Since: "v0.1.0",
		Member:  &MemberRename{Pkg: "internal/lint/testdata/migapi", Type: "Widget", Old: "Display", New: "Show"},
		Example: Example{Old: "x", New: "x"},
	},
	{
		ID: "migapi-make", Since: "v0.1.0",
		Ident:   &IdentRename{Pkg: "internal/lint/testdata/migapi", Old: "Make", New: "Build"},
		Example: Example{Old: "x", New: "x"},
	},
	{
		ID: "migold-moved", Since: "v0.1.0",
		Import: &ImportMove{
			Old: modulePath + "/internal/lint/testdata/migold", New: modulePath + "/internal/lint/testdata/migapi",
			NewName: "migapi",
		},
		Example: Example{Old: "x", New: "x"},
	},
}

func TestMigrationKinds(t *testing.T) {
	parallelLint(t)
	needGo(t)
	const api = `"github.com/kuronosu/tela/internal/lint/testdata/migapi"`
	old := `package example

import (
	` + api + `
)

func f(w *migapi.Widget) {
	w.Display()
	w.Label = "x"
	_ = w.Title
	_ = migapi.Widget{Label: "y"}
	_ = migapi.Make()
	g := w.Display
	g()
	var _ = (*migapi.Widget).Display
}
`
	want := `package example

import (
	` + api + `
)

func f(w *migapi.Widget) {
	w.Show()
	w.Title = "x"
	_ = w.Title
	_ = migapi.Widget{Title: "y"}
	_ = migapi.Build()
	g := w.Show
	g()
	var _ = (*migapi.Widget).Show
}
`
	pattern, file := migrateDir(t, old)
	res, err := Run(Options{Patterns: []string{pattern}, Migrations: testMigrations, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.FixError != nil {
		t.Fatal(res.FixError)
	}
	got, _ := os.ReadFile(file)
	if string(got) != want {
		t.Errorf("-fix gives\n%s\nwant\n%s", got, want)
	}
	if len(res.Fixed) != 6 {
		t.Errorf("fixed %d findings, want 6: %v", len(res.Fixed), res.Fixed)
	}
}

// What the new API still has is never reported, and the rule can be ignored.
func TestMigrationLeavesCurrentCode(t *testing.T) {
	parallelLint(t)
	needGo(t)
	src := `package example

import "github.com/kuronosu/tela/internal/lint/testdata/migapi"

func f(w *migapi.Widget) {
	w.Show()
	_ = migapi.Build()
	` + "//tela:" + `ignore migrate -- the old name is the subject of this test
	_ = migapi.Make()
}
`
	pattern, _ := migrateDir(t, src)
	res, err := Run(Options{Patterns: []string{pattern}, Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 0 {
		t.Errorf("findings %v, want none", res.Diagnostics)
	}
}

// A dot import names the package's identifiers without a qualifier; a field
// or key of the same name is not one.
func TestMigrationDotImport(t *testing.T) {
	parallelLint(t)
	needGo(t)
	old := `package example

import . "github.com/kuronosu/tela/internal/lint/testdata/migapi"

type thing struct{ Make int }

func f(t thing) {
	_ = Make()
	_ = t.Make
	_ = thing{Make: 1}
}
`
	pattern, file := migrateDir(t, old)
	res, err := Run(Options{Patterns: []string{pattern}, Migrations: testMigrations, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	want := strings.Replace(old, "_ = Make()", "_ = Build()", 1)
	if string(got) != want || len(res.Fixed) != 1 {
		t.Errorf("-fix gives (%d fixed)\n%s\nwant\n%s", len(res.Fixed), got, want)
	}
}

// A package that moved: the import path changes and so do the uses of its
// name, when the name changed; the other rules wait until it compiles.
func TestMigrationImportMove(t *testing.T) {
	parallelLint(t)
	needGo(t)
	old := `package example

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/lint/testdata/migold"
)

func f() tela.Node {
	fmt.Println(migold.Hello())
	return h.Div(h.Class("flex rounded-xxl"))
}
`
	pattern, file := migrateDir(t, old)
	res, err := Run(Options{Patterns: []string{pattern}, Migrations: testMigrations})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Rule != RuleMigrate {
		t.Fatalf("findings %v, want the migrate finding alone: the package does not type-check yet", res.Diagnostics)
	}
	res, err = Run(Options{Patterns: []string{pattern}, Migrations: testMigrations, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	want := strings.NewReplacer(
		"testdata/migold", "testdata/migapi",
		"migold.Hello", "migapi.Hello",
	).Replace(old)
	if string(got) != want {
		t.Errorf("-fix gives\n%s\nwant\n%s", got, want)
	}
	// The pass that follows sees the package compile and reports its class.
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Rule != RuleClass || len(res.LoadErrors) != 0 {
		t.Errorf("after -fix: findings %v, load errors %v; want the class finding alone", res.Diagnostics, res.LoadErrors)
	}
	if len(res.Fixed) < 1 || res.Fixed[0].Rule != RuleMigrate {
		t.Errorf("fixed %v, want the migrate finding", res.Fixed)
	}
}

// The import a rewrite needs is added, the existing one is reused under its
// local name.
func TestAddImport(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"block", "package p\n\nimport (\n\t\"a\"\n)\n", "package p\n\nimport (\n\t\"a\"\n\t\"x/y\"\n)\n"},
		{"none", "package p\n\nvar v int\n", "package p\n\nimport \"x/y\"\n\nvar v int\n"},
		{"single", "package p\n\nimport \"a\"\n", "package p\n\nimport \"a\"\n\nimport \"x/y\"\n"},
		{"present", "package p\n\nimport (\n\t\"x/y\"\n)\n", "package p\n\nimport (\n\t\"x/y\"\n)\n"},
	}
	for _, tc := range tests {
		got, err := format.Source(addImport([]byte(tc.src), "x/y"))
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		want, _ := format.Source([]byte(tc.want))
		if string(got) != string(want) {
			t.Errorf("%s: got\n%s\nwant\n%s", tc.name, got, want)
		}
	}
}
