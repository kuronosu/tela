//go:build teladev

package core

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/gfx"
)

// Unit tests of the dev state's rules (ADR-113).

type devKeepItem struct {
	Name  string
	Tags  []string
	Sub   *devKeepItem
	Score map[string]float64
	At    time.Time
	Color gfx.Color
}

type devKeepAny struct{ V any }

type devKeepFunc struct{ OnSave func() }

type devKeepHandle struct {
	Count int
	Set   Setter[int]
}

type devKeepPrivate struct {
	Name string
	n    int
}

type devKeepEmbedded struct {
	devKeepPrivate
}

func TestDevKeepable(t *testing.T) {
	cyc := &devKeepItem{Name: "a"}
	cyc.Sub = cyc
	for _, c := range []struct {
		name   string
		enc    func() ([]byte, devWhy)
		reason string // "" kept
		silent bool
	}{
		{"int", func() ([]byte, devWhy) { return devEncodeValue(42) }, "", false},
		{"string", func() ([]byte, devWhy) { return devEncodeValue("x") }, "", false},
		{"struct", func() ([]byte, devWhy) {
			return devEncodeValue(devKeepItem{Name: "a", Tags: []string{"b"}, Sub: &devKeepItem{Name: "c"},
				Score: map[string]float64{"x": 1}, At: time.Unix(1, 0)})
		}, "", false},
		{"time", func() ([]byte, devWhy) { return devEncodeValue(time.Unix(5, 0)) }, "", false},
		{"any int", func() ([]byte, devWhy) { return devEncodeValue(devKeepAny{V: 3}) }, "", false},
		{"any nil", func() ([]byte, devWhy) { return devEncodeValue[any](nil) }, "", false},
		{"func", func() ([]byte, devWhy) { return devEncodeValue(func() {}) }, "holds a func()", false},
		{"chan", func() ([]byte, devWhy) { return devEncodeValue(make(chan int)) }, "holds a chan int", false},
		{"unsafe", func() ([]byte, devWhy) { return devEncodeValue(unsafe.Pointer(nil)) }, "holds an unsafe.Pointer", false},
		// The types of this test are named types of package core, which is
		// Tela's own: what they hold resets silently (a user's type warns).
		{"field func", func() ([]byte, devWhy) { return devEncodeValue(devKeepFunc{}) }, "holds a core.devKeepFunc, which holds a func()", true},
		{"private", func() ([]byte, devWhy) { return devEncodeValue(devKeepPrivate{n: 1}) }, "with the unexported field n", true},
		{"embedded", func() ([]byte, devWhy) { return devEncodeValue(devKeepEmbedded{}) }, "with the unexported field devKeepPrivate", true},
		{"any func", func() ([]byte, devWhy) { return devEncodeValue(devKeepAny{V: func() {}}) }, "an interface value that is a func()", false},
		{"cycle", func() ([]byte, devWhy) { return devEncodeValue(cyc) }, "holds itself", false},
		{"mutex", func() ([]byte, devWhy) { return devEncodeValue(&sync.Mutex{}) }, "sync.Mutex with the unexported field", false},
		{"setter", func() ([]byte, devWhy) { return devEncodeValue(Setter[int]{}) }, "Setter", true},
		{"handle field", func() ([]byte, devWhy) { return devEncodeValue(devKeepHandle{}) }, "Setter", true},
		{"element ref", func() ([]byte, devWhy) { return devEncodeValue(ElementRef{}) }, "ElementRef", true},
		{"value", func() ([]byte, devWhy) { return devEncodeValue((*Value[int])(nil)) }, "Value", true},
		{"node", func() ([]byte, devWhy) { return devEncodeValue(Node{}) }, "Node", true},
		{"set", func() ([]byte, devWhy) { return devEncodeValue(map[string]struct{}{}) }, "without exported fields", false},
		{"unregistered", func() ([]byte, devWhy) { return devEncodeValue(devKeepAny{V: devKeepItem{}}) }, "gob.Register", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			data, why := c.enc()
			switch {
			case c.reason == "" && why.reason != "":
				t.Errorf("not kept: %s", why.reason)
			case c.reason == "" && len(data) == 0:
				t.Errorf("no data")
			case c.reason != "" && !strings.Contains(why.reason, c.reason):
				t.Errorf("reason %q, want %q", why.reason, c.reason)
			case c.reason != "" && why.silent != c.silent:
				t.Errorf("silent %v, want %v (%s)", why.silent, c.silent, why.reason)
			}
		})
	}
}

func TestDevValueRoundTrip(t *testing.T) {
	type tab uint8
	check := func(name string, ok bool) {
		t.Helper()
		if !ok {
			t.Errorf("%s: the decoded value differs", name)
		}
	}
	b, _ := devEncodeValue(tab(3))
	v1, err := devDecodeValue[tab](b)
	check("named uint", err == nil && v1 == 3)
	b, _ = devEncodeValue(-12345)
	v2, err := devDecodeValue[int](b)
	check("int", err == nil && v2 == -12345)
	b, _ = devEncodeValue(float32(1.25))
	v3, err := devDecodeValue[float32](b)
	check("float32", err == nil && v3 == 1.25)
	in := devKeepItem{Name: "a", Tags: []string{"b"}, Sub: &devKeepItem{Name: "c"}, Score: map[string]float64{"x": 1},
		At: time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC), Color: gfx.Color{R: 0.5, A: 1}}
	b, _ = devEncodeValue(in)
	v4, err := devDecodeValue[devKeepItem](b)
	check("struct", err == nil && reflect.DeepEqual(v4, in))
	b, _ = devEncodeValue[any]("s")
	v5, err := devDecodeValue[any](b)
	check("any", err == nil && v5 == "s")
	// A value of another kind does not decode.
	b, _ = devEncodeValue("text")
	if _, err := devDecodeValue[int](b); err == nil {
		t.Error("a string decoded as an int")
	}
	b, _ = devEncodeValue(300)
	if _, err := devDecodeValue[int8](b); err == nil {
		t.Error("300 decoded as an int8")
	}
}

func TestDevIdentities(t *testing.T) {
	// Function literals of one function share an identity without their
	// numbers; generic shapes and method value suffixes go.
	f1 := func() Node { return Node{} }
	f2 := func() Node { return Node{} }
	c1, c2 := Component(f1).d.comp, Component(f2).d.comp
	if a, b := devCompIdent(c1), devCompIdent(c2); a != b || !strings.HasSuffix(a, ".TestDevIdentities.func") {
		t.Errorf("closure identities %q and %q", a, b)
	}
	if got, want := devCompIdent(Component(devCounter).d.comp), "F:"+corePkg+".devCounter"; got != want {
		t.Errorf("identity %q, want %q", got, want)
	}
	if got, want := devCompIdent(componentOf(devCard{}).d.comp), "S:"+corePkg+".devCard"; got != want {
		t.Errorf("identity %q, want %q", got, want)
	}
	// Keys.
	for _, c := range []struct {
		k    any
		want string
	}{
		{"a", `"a"`},
		{7, "7"},
		{int64(7), "int64=7"},
		{keyPair{"x", int32(2)}, `("x",int32=2)`},
		{devKeyStruct{ID: 3}, corePkg + `.devKeyStruct=core.devKeyStruct{ID:3}`},
	} {
		if got := devKeyString(c.k); got != c.want {
			t.Errorf("key %#v: %q, want %q", c.k, got, c.want)
		}
	}
	// Full type names.
	if got, want := devTypeString(reflect.TypeFor[map[string][]*devKeepItem]()), "map[string][]*"+corePkg+".devKeepItem"; got != want {
		t.Errorf("type %q, want %q", got, want)
	}
}

type devKeyStruct struct{ ID int }

func TestDevClassify(t *testing.T) {
	for _, c := range []struct {
		fn, file           string
		decides            bool
		framework, transit bool
	}{
		{corePkg + ".UseState[...]", "hooks.go", false, false, false},
		{telaPkg + ".UseState[...]", "hooks.go", false, false, false},
		{telaPkg + "/aria.UseHover", "hover.go", true, true, true},
		{telaPkg + "/aria.usePressCore", "press.go", true, true, true},
		{telaPkg + "/aria.useBoolState", "state.go", true, true, false},
		{telaPkg + "/i18n.UseLocale", "hooks.go", true, true, false},
		{telaPkg + "/examples/dashboard.Page.Build", "main.go", true, false, false},
		{"example.com/app.Counter", "counter.go", true, false, false},
		{telaPkg + "/aria.TestX.func1", "x_test.go", true, false, false},
	} {
		cr, ok := devClassify(c.fn, c.file)
		if ok != c.decides || cr.framework != c.framework || cr.transient != c.transit {
			t.Errorf("%s: %+v %v, want framework %v, transient %v, decides %v", c.fn, cr, ok, c.framework, c.transit, c.decides)
		}
	}
}

func TestDevRegisterState(t *testing.T) {
	locale := "es"
	RegisterDevState("test.locale", func() ([]byte, error) { return []byte(locale), nil },
		func(b []byte) error { locale = string(b); return nil })
	RegisterDevState("test.broken", func() ([]byte, error) { panic("boom") }, func([]byte) error { return nil })
	t.Cleanup(func() {
		RegisterDevState("test.locale", nil, nil)
		RegisterDevState("test.broken", nil, nil)
	})
	var warns []string
	a := devTestTree(t, div(Component(devCounter)), nil, &warns)
	data, rep := devSnap(t, a)
	if !slices.Equal(rep.Extra, []string{"test.locale"}) || len(rep.ExtraErrors) != 1 || len(warns) != 1 {
		t.Errorf("report %+v, warnings %q", rep, warns)
	}
	locale = "en" // the next process starts with its default
	devTestTree(t, div(Component(devCounter)), data, nil)
	if locale != "es" {
		t.Errorf("locale %q after the reload, want es", locale)
	}
}

func TestDevCaptureApp(t *testing.T) {
	tr := devTestTree(t, div(Component(devCounter)), nil, nil)
	stop := tr.ReceivePosts()
	defer stop()
	devSet(t, tr, devComp(t, tr, "devCounter"), 0, 3)
	var got []byte
	CaptureAppDevState(func(data []byte, rep DevStateReport, err error) {
		if err != nil || rep.Kept != 1 {
			t.Errorf("capture: %v %+v", err, rep)
		}
		got = data
	})
	if got != nil {
		t.Fatal("captured outside the UI thread's update pass")
	}
	tr.ApplyUpdates()
	if got == nil {
		t.Fatal("not captured")
	}
	b := devTestTree(t, div(Component(devCounter)), got, nil)
	if n := devState[int](t, devComp(t, b, "devCounter"), 0); n != 3 {
		t.Errorf("state %d, want 3", n)
	}
	// Seeding after Mount is ignored, with a warning.
	var warns []string
	c := devTestTree(t, div(Component(devCounter)), nil, &warns)
	c.SeedDevState(got)
	if len(warns) != 1 || !strings.Contains(warns[0], "after Mount") {
		t.Errorf("warnings %q", warns)
	}
}
