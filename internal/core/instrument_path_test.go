package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// A site in the working directory is relative to it, one in another
// directory of the module is qualified by the module path, and one
// outside any module is its base name.
func TestSourceSiteRel(t *testing.T) {
	_, file, line, _ := runtime.Caller(0)
	if got, want := (SourceSite{File: file, Line: line}).Rel(), fmt.Sprintf("instrument_path_test.go:%d", line); got != want {
		t.Errorf("Rel() = %q, want %q", got, want)
	}
	if got := (SourceSite{}).Rel(); got != "" {
		t.Errorf("no site: Rel() = %q", got)
	}
	// A file of the module outside the working directory (internal/core).
	other := filepath.Join(filepath.Dir(file), "..", "..", "app.go")
	if got := (SourceSite{File: filepath.ToSlash(filepath.Clean(other)), Line: 3}).Rel(); got != "github.com/kuronosu/tela/app.go:3" {
		t.Errorf("a file of the module outside the directory: %q", got)
	}
	// Outside any module (the temporary directory has no go.mod above).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := (SourceSite{File: filepath.ToSlash(filepath.Join(dir, "x.go")), Line: 9}).Rel(); got != "x.go:9" {
		t.Errorf("a file outside any module: %q", got)
	}
	// A module of its own (a directory not looked at before: results are
	// cached).
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/other\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := (SourceSite{File: filepath.ToSlash(filepath.Join(dir, "x.go")), Line: 9}).Rel(); got != "example.com/other/x.go:9" {
		t.Errorf("a file of another module: %q", got)
	}
	// Already relative (-trimpath).
	if got := (SourceSite{File: "example.com/m@v1.0.0/a.go", Line: 2}).Rel(); got != "example.com/m@v1.0.0/a.go:2" {
		t.Errorf("a trimmed path: %q", got)
	}
}

// RelTo is Rel in another working directory: a file under it is relative
// to it, the others follow Rel's rules.
func TestSourceSiteRelTo(t *testing.T) {
	_, file, line, _ := runtime.Caller(0)
	file = filepath.ToSlash(file)
	core := filepath.ToSlash(filepath.Dir(file))
	root := filepath.ToSlash(filepath.Dir(filepath.Dir(filepath.Dir(file)))) // the module's directory
	if got, want := (SourceSite{File: file, Line: line}).RelTo(root), fmt.Sprintf("internal/core/instrument_path_test.go:%d", line); got != want {
		t.Errorf("RelTo(the module's directory) = %q, want %q", got, want)
	}
	if got, want := (SourceSite{File: file, Line: line}).RelTo(core+"/"), fmt.Sprintf("instrument_path_test.go:%d", line); got != want {
		t.Errorf("RelTo(its own directory, with a slash) = %q, want %q", got, want)
	}
	if got, want := (SourceSite{File: root + "/app.go", Line: 3}).RelTo(core), "github.com/kuronosu/tela/app.go:3"; got != want {
		t.Errorf("RelTo(another directory of the module) = %q, want %q", got, want)
	}
	if got := (SourceSite{}).RelTo(core); got != "" {
		t.Errorf("no site: RelTo() = %q", got)
	}
}

// A working directory reached through a symbolic link (macOS's /var is
// /private/var) still holds the files the compiler recorded under the
// link's target, and the other way round.
func TestSourceSiteRelToSymlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symbolic links:", err)
	}
	for _, c := range []struct{ cwd, file string }{
		{link, filepath.Join(real, "ui", "card.go")},
		{real, filepath.Join(link, "ui", "card.go")},
	} {
		if got := (SourceSite{File: filepath.ToSlash(c.file), Line: 7}).RelTo(c.cwd); got != "ui/card.go:7" {
			t.Errorf("RelTo(%s) of %s = %q, want ui/card.go:7", c.cwd, c.file, got)
		}
	}
}

// The layout JSON of an instrumented tree says where each element was
// made, and an element a library made is shown at the app's line that
// wrote the library component.
func TestInspectSrc(t *testing.T) {
	var divLine int
	tr := newInsTree(t, func() Node {
		return here(&divLine, Primitive(style.KindDiv, Key("box"), Text("hi")))
	})
	n := tr.Inspect()
	var find func(n InspectNode) *InspectNode
	find = func(n InspectNode) *InspectNode {
		if n.Key == "box" {
			return &n
		}
		for _, c := range n.Children {
			if r := find(c); r != nil {
				return r
			}
		}
		return nil
	}
	box := find(n)
	if box == nil {
		t.Fatalf("no box with key box in %+v", n)
	}
	if want := fmt.Sprintf("instrument_path_test.go:%d", divLine); box.Src != want {
		t.Errorf("Src = %q, want %q", box.Src, want)
	}
	js, err := box.MarshalJSON()
	if err != nil || !strings.Contains(string(js), `"src":"instrument_path_test.go:`) {
		t.Errorf("JSON lacks the src (%v): %s", err, js)
	}
	// Off, there is no src.
	tr.SetInstrument(false)
	if got := find(tr.Inspect()); got == nil || got.Src != "" {
		t.Errorf("not instrumented: %+v", got)
	}
}
