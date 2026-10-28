package tela

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"reflect"
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

// TestAppWindowOptions checks that the window fields of App (ADR-180)
// reach the core options.
func TestAppWindowOptions(t *testing.T) {
	closed := 0
	place := WindowPlacement{Width: 500, Height: 400, X: 3, Y: 4, HasPosition: true, Maximized: true, RestoreToken: "t"}
	app := App{
		MinWidth: 1, MinHeight: 2, MaxWidth: 3, MaxHeight: 4,
		FixedSize: true, Frameless: true, AlwaysOnTop: true, Hidden: true,
		Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}, Placement: place,
		OnClose: func(WindowHandle) { closed++ },
		Root:    C(func() Node { return Node{} }),
	}
	o := app.options()
	if o.MinWidth != 1 || o.MinHeight != 2 || o.MaxWidth != 3 || o.MaxHeight != 4 || !o.FixedSize || !o.Frameless ||
		!o.AlwaysOnTop || !o.Hidden || len(o.Icon) != 1 || o.Placement != place || o.OnClose == nil {
		t.Errorf("options %+v", o)
	}
	o.OnClose(WindowHandle{})
	if closed != 1 {
		t.Error("OnClose does not reach the core options")
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

// TestWindowFieldsReachCore checks that every field of core.WindowSpec has
// its twin on Window, which Build passes on, and on App (except Modal, a
// child window's), which options passes on: a field added to WindowSpec
// and left out of one of these lists (or of core.Options; see
// TestOptionsCarryEveryWindowField in internal/core) fails here instead of
// being dropped without a word.
func TestWindowFieldsReachCore(t *testing.T) {
	st := reflect.TypeFor[core.WindowSpec]()
	root := C(func() Node { return Node{} })

	var w Window
	wv := reflect.ValueOf(&w).Elem()
	for i := range st.NumField() {
		f := st.Field(i)
		if f.Name == "Root" {
			continue // a Component here, a core.Node there
		}
		wf, ok := reflect.TypeFor[Window]().FieldByName(f.Name)
		if !ok || wf.Type != f.Type {
			t.Errorf("Window has no field %s %v of core.WindowSpec", f.Name, f.Type)
			continue
		}
		fillNonZero(wv.FieldByIndex(wf.Index))
	}
	w.Root = root
	checkPassedOn(t, "Window.spec", reflect.ValueOf(w), reflect.ValueOf(w.spec()), nil)

	var app App
	av := reflect.ValueOf(&app).Elem()
	skip := map[string]bool{"Modal": true, "Root": true, "OnClose": true} // App.OnClose takes the WindowHandle
	for i := range st.NumField() {
		f := st.Field(i)
		if skip[f.Name] {
			continue
		}
		af, ok := reflect.TypeFor[App]().FieldByName(f.Name)
		if !ok || af.Type != f.Type {
			t.Errorf("App has no field %s %v of core.WindowSpec", f.Name, f.Type)
			continue
		}
		fillNonZero(av.FieldByIndex(af.Index))
	}
	app.Root = root
	app.OnClose = func(WindowHandle) {}
	checkPassedOn(t, "App.options", reflect.ValueOf(app), reflect.ValueOf(app.options()), skip)
	if o := app.options(); o.OnClose == nil || o.Root == (Node{}) {
		t.Errorf("App.options drops OnClose or Root: %+v", o)
	}
}

// checkPassedOn reports the fields of core.WindowSpec, other than Root and
// those in skip, whose value in to (found by name) is not the one in from.
func checkPassedOn(t *testing.T, what string, from, to reflect.Value, skip map[string]bool) {
	t.Helper()
	st := reflect.TypeFor[core.WindowSpec]()
	for i := range st.NumField() {
		name := st.Field(i).Name
		if name == "Root" || skip[name] {
			continue
		}
		a, b := from.FieldByName(name), to.FieldByName(name)
		if !a.IsValid() || !b.IsValid() {
			continue // reported above
		}
		if a.Kind() == reflect.Func {
			if a.IsNil() != b.IsNil() {
				t.Errorf("%s drops %s", what, name)
			}
			continue
		}
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			t.Errorf("%s drops %s: %v, want %v", what, name, b, a)
		}
	}
}

// fillNonZero sets v (settable) to a value other than its zero value,
// field by field for a struct; a func does nothing.
func fillNonZero(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(3)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(3)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(7)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillNonZero(s.Index(0))
		v.Set(s)
	case reflect.Interface:
		if img := reflect.ValueOf(image.NewRGBA(image.Rect(0, 0, 1, 1))); img.Type().Implements(v.Type()) {
			v.Set(img)
		}
	case reflect.Func:
		v.Set(reflect.MakeFunc(v.Type(), func([]reflect.Value) []reflect.Value { return nil }))
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				fillNonZero(v.Field(i))
			}
		}
	}
}
