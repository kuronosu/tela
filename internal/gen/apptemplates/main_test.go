package main

import (
	"sort"
	"strings"
	"testing"
)

// TestTemplatesAreUpToDate fails when cmd/tela/templates does not match the
// examples.
func TestTemplatesAreUpToDate(t *testing.T) {
	root, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := build(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := onDisk(root)
	if err != nil {
		t.Fatal(err)
	}
	var stale []string
	for name, w := range want {
		if g, ok := got[name]; !ok {
			stale = append(stale, name+" is missing")
		} else if string(g) != string(w) {
			stale = append(stale, name+" differs")
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			stale = append(stale, name+" is not generated any more")
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("the app templates are stale: run \"go generate ./cmd/tela\" and commit the result:\n\t%s", strings.Join(stale, "\n\t"))
	}
}

func TestCutTests(t *testing.T) {
	src := map[string][]byte{
		"a.go": []byte("package a\n\nfunc Keep() {}\n"),
		"a_test.go": []byte(`package a

import (
	"testing"

	"github.com/kuronosu/tela/internal/testutil/damagesample"
	"github.com/kuronosu/tela/telatest"
)

// helper is used only by a test that goes, so it goes too.
func helper() int { return 1 }

// TestOK stays.
func TestOK(t *testing.T) { telatest.MountApp(t, App) }

// TestSampled uses an internal package.
func TestSampled(t *testing.T) { damagesample.Short(t, 6) }

func TestPixels(t *testing.T) { a := telatest.MountApp(t, App); _ = helper(); a.Golden("x.png") }

func TestAria(t *testing.T) { a := telatest.MountApp(t, App); a.AriaGolden("x.yml") }

func TestUsesOther(t *testing.T) { _ = other() }

func other() int { return 2 }
`),
	}
	var log []string
	out, err := cutTests(map[string][]byte{
		"a.go":      src["a.go"],
		"a_test.go": src["a_test.go"],
		"b_test.go": []byte("package a\n\nimport \"testing\"\n\nfunc TestGone(t *testing.T) { var a interface{ Golden(string) }; a.Golden(\"x\") }\n"),
	}, func(s string) { log = append(log, s) })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["b_test.go"]; ok {
		t.Errorf("b_test.go has nothing left and should be dropped:\n%s", out["b_test.go"])
	}
	got := string(out["a_test.go"])
	for _, want := range []string{"func TestOK", "func TestAria", "func TestUsesOther", `"github.com/kuronosu/tela/telatest"`} {
		if !strings.Contains(got, want) {
			t.Errorf("a_test.go lost %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"helper", "TestSampled", "damagesample", "TestPixels"} {
		if strings.Contains(got, gone) {
			t.Errorf("a_test.go still has %q:\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "func other()") {
		t.Errorf("a_test.go lost a declaration nothing removed depends on:\n%s", got)
	}
	if string(out["a.go"]) != string(src["a.go"]) {
		t.Errorf("a non-test file changed")
	}
}

func TestExtraRequires(t *testing.T) {
	mod := "module m\n\ngo 1.27\n\nrequire (\n\tgithub.com/kuronosu/tela v0.14.7\n\tmodernc.org/sqlite v1.62.0\n)\n\nrequire (\n\tgithub.com/x/y v1.0.0 // indirect\n)\n\nreplace github.com/kuronosu/tela => ../..\n"
	if got, want := extraRequires([]byte(mod)), "modernc.org/sqlite v1.62.0\n"; got != want {
		t.Errorf("extraRequires = %q, want %q", got, want)
	}
	if got := extraRequires([]byte("module m\n\ngo 1.27\n\nrequire example.com/a v1.2.3\n")); got != "example.com/a v1.2.3\n" {
		t.Errorf("single-line require: %q", got)
	}
}

func TestRewrite(t *testing.T) {
	s := spec{"notes", "examples/notes", telaModule + "/examples/notes"}
	got := string(rewrite([]byte("import \"github.com/kuronosu/tela/examples/notes/icons\"\n// go generate ./examples/notes/icons\n//\t./examples/notes\nx := \"in examples/notes\"\n"), s))
	want := "import \"" + Placeholder + "/icons\"\n// go generate ./icons\n//\t.\nx := \"in examples/notes\"\n"
	if got != want {
		t.Errorf("rewrite =\n%s\nwant\n%s", got, want)
	}
}
