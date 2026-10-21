package tela

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/core"
)

// rootCard is a struct component for App.Root.
type rootCard struct{ Title string }

func (c rootCard) Build() Node { return core.Text(c.Title) }

// TestAppOptions checks that every App field reaches the core options.
func TestAppOptions(t *testing.T) {
	app := App{Title: "T", AppID: "a.b", Width: 3, Height: 4, RootClass: "bg-white",
		ClassicScrollbars: true, InstantScroll: true, Root: C(func() Node { return Node{} })}
	o := app.options()
	if o.Title != "T" || o.AppID != "a.b" || o.Width != 3 || o.Height != 4 || o.RootClass != "bg-white" ||
		!o.ClassicScrollbars || !o.InstantScroll || o.Root == (Node{}) {
		t.Errorf("options %+v", o)
	}
}

// TestAppDev checks the dev-mode rule of App.Dev (ADR-085): nil follows
// the build (a test binary is a development build), a value overrides
// it, and TELA_DEV overrides both.
func TestAppDev(t *testing.T) {
	on, off := true, false
	root := C(func() Node { return Node{} })
	for _, c := range []struct {
		env  string
		dev  *bool
		want bool
	}{
		{"", nil, true},
		{"", &off, false},
		{"", &on, true},
		{"0", &on, false},
		{"0", nil, false},
		{"1", &off, true},
	} {
		t.Setenv("TELA_DEV", c.env)
		if got := (App{Root: root, Dev: c.dev}).options().Dev; got != c.want {
			t.Errorf("TELA_DEV=%q Dev=%v: dev %v, want %v", c.env, c.dev, got, c.want)
		}
	}
}

// TestMainError checks that Main prints Run's error and exits with 1.
func TestMainError(t *testing.T) {
	defer func(r func(App) error, e func(int), w io.Writer) { mainRun, mainExit, mainStderr = r, e, w }(mainRun, mainExit, mainStderr)
	var stderr bytes.Buffer
	mainStderr = &stderr
	code := -1
	mainExit = func(c int) { code = c }

	mainRun = func(App) error { return errors.New("auto: no display server available") }
	Main(App{Root: C(func() Node { return Node{} })})
	if code != 1 || stderr.String() != "tela: auto: no display server available\n" {
		t.Errorf("on error: exit %d, stderr %q", code, stderr.String())
	}

	code = -1
	stderr.Reset()
	mainRun = func(App) error { return nil }
	Main(App{Root: C(func() Node { return Node{} })})
	if code != -1 || stderr.Len() != 0 {
		t.Errorf("without an error: exit %d, stderr %q", code, stderr.String())
	}
}

// TestAppRoot: Root is a Component, a struct value or C(fn), and what
// reaches the core options is its node; a nil Root is an error that says
// how to fix it (ADR-094).
func TestAppRoot(t *testing.T) {
	fn := func() Node { return core.Text("fn") }
	for name, root := range map[string]Component{
		"struct": rootCard{Title: "card"},
		"C":      C(fn),
		"CP":     CP(func(n int) Node { return core.Text(fmt.Sprint(n)) }, 3),
		"node":   core.Text("node"),
	} {
		o := App{Root: root}.options()
		if o.Root.IsZero() {
			t.Errorf("%s: the root node is zero", name)
		}
	}
	for _, f := range []func(){
		func() { _ = Run(App{}) },
		func() { Main(App{}) },
	} {
		func() {
			defer func() {
				msg := fmt.Sprint(recover())
				if !strings.Contains(msg, "needs a Root component") || !strings.Contains(msg, "Root: tela.C(Counter)") || !strings.Contains(msg, "Root: Counter{}") {
					t.Errorf("nil Root: panic %q", msg)
				}
			}()
			f()
		}()
	}
}
