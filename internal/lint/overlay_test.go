package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The overlay gives the source of a file that the run reads instead of the
// disk's (an editor's unsaved buffer): findings are in the buffer, at its
// offsets, with absolute paths when asked.
func TestOverlay(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "overlay"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "buffer.go")
	src := `package overlay

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

func Buffer() tela.Node {
	return h.Div(h.Class("flex p-4 bg-rde-500"))
}
`
	res, err := Run(Options{
		Dir: dir, Patterns: []string{"."}, AbsPaths: true,
		Overlay: map[string][]byte{file: []byte(src)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) > 0 {
		t.Fatalf("load errors: %v", res.LoadErrors)
	}
	if len(res.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want one", res.Diagnostics)
	}
	d := res.Diagnostics[0]
	if d.File != file || d.Rule != RuleClass || d.Line != 9 || d.Suggestion != "bg-red-500" {
		t.Errorf("diagnostic = %+v, want %s:9 class with the suggestion bg-red-500", d, file)
	}
	if res.Parser == nil {
		t.Error("Result.Parser is nil")
	}
	if len(res.ClassStrings) != 1 {
		t.Fatalf("ClassStrings = %v, want one", res.ClassStrings)
	}
	cs := res.ClassStrings[0]
	if cs.File != file || src[cs.Start:cs.End] != `"flex p-4 bg-rde-500"` {
		t.Errorf("ClassStrings[0] = %+v selects %q", cs, src[cs.Start:min(cs.End, len(src))])
	}
	if _, err := os.Stat(file); err == nil {
		t.Errorf("%s exists on disk: the overlay must not write it", file)
	}
}

// A directive in the buffer silences the finding below it: whether the
// comment stands alone on its line is read from the buffer, not from the
// disk's file (which may be shorter, or another text, or not exist).
func TestOverlayIgnoreDirective(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "overlay"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "buffer.go")
	src := `package overlay

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

func Buffer() tela.Node {
	//tela:ignore class -- the buffer checks the typo
	return h.Div(h.Class("flex p-4 bg-rde-500"))
}
`
	res, err := Run(Options{
		Dir: dir, Patterns: []string{"."}, AbsPaths: true,
		Overlay: map[string][]byte{file: []byte(src)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 0 {
		t.Errorf("diagnostics = %v, want none: the directive above the line is in the buffer", res.Diagnostics)
	}
}

// An import that only the buffer has is listed, and a type error does not
// stop a lenient run from checking what resolved.
func TestOverlayImportAndLenient(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "overlay"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "buffer.go")
	src := `package overlay

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

func Buffer() tela.Node {
	_ = strings.ToUpper(undefinedName)
	return h.Div(h.Class("flex bg-rde-500"))
}
`
	opts := Options{Dir: dir, Patterns: []string{"."}, Overlay: map[string][]byte{file: []byte(src)}}
	res, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 0 || len(res.LoadErrors) != 1 || !strings.Contains(res.LoadErrors[0], "undefinedName") {
		t.Errorf("strict run: diagnostics %v, load errors %v; want no diagnostics and the type error", res.Diagnostics, res.LoadErrors)
	}
	opts.Lenient = true
	res, err = Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) != 1 || len(res.Diagnostics) != 1 || res.Diagnostics[0].Rule != RuleClass {
		t.Errorf("lenient run: diagnostics %v, load errors %v; want the class finding and the type error", res.Diagnostics, res.LoadErrors)
	}
}
