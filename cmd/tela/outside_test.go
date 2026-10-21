package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the directory of the tela module.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && strings.HasPrefix(string(b), "module github.com/kuronosu/tela\n") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod of github.com/kuronosu/tela not found above the test directory")
		}
		dir = parent
	}
}

// outsideModule creates a module outside the repository that requires tela
// (through a replace directive) and has two packages: one with Preview and
// one with Previews.
func outsideModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": fmt.Sprintf(`module example.com/outside

go 1.27

require github.com/kuronosu/tela v0.0.0

replace github.com/kuronosu/tela => %s
`, filepath.ToSlash(repoRoot(t))),
		"screen/screen.go": `package screen

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func Preview() tela.Node { return tela.C(screen) }

func screen() tela.Node {
	count, _ := tela.UseState(7)
	return Div(Class("flex min-h-screen flex-col gap-2 bg-slate-100 p-4 dark:bg-slate-900"),
		H1(Class("text-2xl font-bold"), Textf("Outside %d", count)),
		Button(Class("rounded-lg bg-sky-500 px-4 py-2"), Text("Go")),
	)
}
`,
		"screens/screens.go": `package screens

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

var Previews = tela.Previews{
	"One": {Root: tela.C(func() tela.Node { return Div(Class("size-full bg-rose-500"), H2(Text("First"))) })},
	"Two": {Root: Two{}},
}

type Two struct{}

func (Two) Build() tela.Node {
	return Div(Class("size-full bg-emerald-500"), H2(Text("Second")))
}
`,
		"shell/shell.go": `package shell

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

var App = tela.App{
	Width: 90, Height: 60, RootClass: "bg-violet-500", ColorScheme: tela.Dark,
	Root: tela.C(func() tela.Node { return Div(Class("size-6 bg-white dark:bg-black")) }),
}
`,
	}
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Resolve the requirements from the module cache, without the network.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Skipf("cannot resolve the dependencies of a module outside the repository offline (go mod tidy: %v):\n%s", err, out)
	}
	return dir
}

// TestOutsideModule runs the commands on a module that is not tela's: the
// runner uses the public telatest package instead of internal/ (ADR-059).
func TestOutsideModule(t *testing.T) {
	skipShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is not in PATH")
	}
	dir := outsideModule(t)
	t.Chdir(dir)

	t.Run("snapshot", func(t *testing.T) {
		img, stdout, _ := snap(t, "--size", "200x100", "--scale", "2", "./screen")
		if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 200 {
			t.Errorf("image is %v, want 400x200", b)
		}
		if got := pixel(img, 2, 2); got != [3]uint8{0xf1, 0xf5, 0xf9} {
			t.Errorf("background %v, want slate-100", got)
		}
		if !strings.HasPrefix(stdout, "wrote ") {
			t.Errorf("stdout %q", stdout)
		}
		dark, _, _ := snap(t, "--size", "200x100", "--dark", "./screen")
		if got := pixel(dark, 2, 2); got != [3]uint8{0x0f, 0x17, 0x2b} {
			t.Errorf("dark background %v, want slate-900", got)
		}
	})

	t.Run("inspect", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if code := run([]string{"inspect", "./screen"}, &out, &errOut); code != 0 {
			t.Fatalf("inspect: code %d\nstderr: %s", code, errOut.String())
		}
		if want := "- heading \"Outside 7\" [level=1]\n- button \"Go\"\n"; out.String() != want {
			t.Errorf("inspect --aria printed\n%s\nwant\n%s", out.String(), want)
		}
		out.Reset()
		if code := run([]string{"inspect", "--layout", "--size", "200x100", "./screen"}, &out, &errOut); code != 0 {
			t.Fatalf("inspect --layout: code %d\nstderr: %s", code, errOut.String())
		}
		var tree struct {
			Kind     string `json:"kind"`
			Children []any  `json:"children"`
		}
		if err := json.Unmarshal(out.Bytes(), &tree); err != nil || tree.Kind != "root" || len(tree.Children) == 0 || !strings.Contains(out.String(), "Outside 7") {
			t.Errorf("inspect --layout did not print the layout tree (%v):\n%.300s", err, out.String())
		}
	})

	t.Run("app", func(t *testing.T) {
		// App alone: its size, root class and scheme, and its Root as the screen.
		img, _, _ := snap(t, "./shell")
		if b := img.Bounds(); b.Dx() != 90 || b.Dy() != 60 {
			t.Errorf("image is %v, want the App's 90x60", b)
		}
		if got := pixel(img, 80, 50); got != [3]uint8{0x8e, 0x51, 0xff} {
			t.Errorf("background %v, want violet-500", got)
		}
		if got := pixel(img, 5, 5); got != [3]uint8{0, 0, 0} {
			t.Errorf("box %v, want black (App.ColorScheme is Dark)", got)
		}
	})

	t.Run("previews", func(t *testing.T) {
		img, stdout, _ := snap(t, "--size", "40x30", "--preview", "Two", "./screens")
		if got := pixel(img, 20, 25); got != [3]uint8{0x00, 0xbc, 0x7d} {
			t.Errorf("pixel = %v, want emerald-500", got)
		}
		if !strings.Contains(stdout, `preview "Two"`) || !strings.Contains(stdout, "others: One") {
			t.Errorf("stdout %q", stdout)
		}
		var out, errOut bytes.Buffer
		if code := run([]string{"inspect", "./screens"}, &out, &errOut); code != 0 || out.String() != "- heading \"First\" [level=2]\n" {
			t.Errorf("inspect: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
		}
	})
}
