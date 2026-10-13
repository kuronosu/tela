package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestUseCallbackForwardsToLatest(t *testing.T) {
	var cbs []Callback[func() string]
	label := "first"
	comp := func() Node {
		l := label
		cb := UseCallback(func() string { return l })
		cbs = append(cbs, cb)
		return Text(l)
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	fn := cbs[0].Fn()
	if got := fn(); got != "first" {
		t.Errorf("fn() = %q", got)
	}
	label = "second"
	tr.Mount(Component(comp))
	if cbs[0] != cbs[1] {
		t.Error("the Callback changed between builds")
	}
	if funcID(cbs[1].Fn()) != funcID(fn) {
		t.Error("Fn returned another forwarder")
	}
	if got := fn(); got != "second" {
		t.Errorf("the old forwarder called %q, want the latest function", got)
	}
}

// handlerProps holds a Callback: the struct stays comparable.
type handlerProps struct {
	Label   string
	OnPress Callback[func(int)]
}

var pressButtonBuilds int

func pressButton(p handlerProps) Node {
	pressButtonBuilds++
	return Text(p.Label)
}

func TestCallbackInPropsKeepsCPSkipping(t *testing.T) {
	pressButtonBuilds = 0
	var presses []string
	var cb Callback[func(int)]
	count := 0
	page := func() Node {
		c := count
		cb = UseCallback(func(n int) { presses = append(presses, fmt.Sprint(c, ":", n)) })
		return ComponentProps(pressButton, handlerProps{Label: "go", OnPress: cb})
	}
	tr := newTestTree(t, 100, 100, Component(page), nil)
	for count = 1; count <= 3; count++ {
		tr.Mount(Component(page))
	}
	if pressButtonBuilds != 1 || tr.Stats().Skips != 3 {
		t.Errorf("%d builds and %d skips of the child, want 1 and 3", pressButtonBuilds, tr.Stats().Skips)
	}
	cb.Fn()(7)
	if strings.Join(presses, " ") != "3:7" {
		t.Errorf("presses %q, want the latest closure (3:7)", presses)
	}
}

type namedHandler func(string) int

type pressEvent struct{ X, Y float32 }

func TestUseCallbackShapes(t *testing.T) {
	var calls []string
	var variadic Callback[func(prefix string, xs ...int) string]
	var multi Callback[func(int, string) (string, error)]
	var named Callback[namedHandler]
	var errFn Callback[func() error]
	var custom Callback[func(pressEvent)]
	made := 0
	RegisterForwarder(func(l *func(pressEvent)) func(pressEvent) {
		made++
		return func(e pressEvent) { (*l)(e) }
	})
	comp := func() Node {
		variadic = UseCallback(func(prefix string, xs ...int) string { return prefix + fmt.Sprint(xs) })
		multi = UseCallback(func(n int, s string) (string, error) {
			return strconv.Itoa(n) + s, errors.New("e")
		})
		named = UseCallback(namedHandler(func(s string) int { calls = append(calls, s); return len(s) }))
		errFn = UseCallback(func() error { return errors.New("boom") })
		custom = UseCallback(func(e pressEvent) { calls = append(calls, fmt.Sprint(e.X)) })
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Mount(Component(comp))
	if got := variadic.Fn()("p", 1, 2); got != "p[1 2]" {
		t.Errorf("variadic: %q", got)
	}
	if s, err := multi.Fn()(4, "x"); s != "4x" || err == nil {
		t.Errorf("multi: %q %v", s, err)
	}
	if n := named.Fn()("abc"); n != 3 {
		t.Errorf("named: %d", n)
	}
	if err := errFn.Fn()(); err == nil || err.Error() != "boom" {
		t.Errorf("func() error: %v", err)
	}
	custom.Fn()(pressEvent{X: 2})
	if strings.Join(calls, " ") != "abc 2" || made != 1 {
		t.Errorf("calls %q, registered forwarder made %d times (want once)", calls, made)
	}
}

func TestUseCallbackMisuse(t *testing.T) {
	notFunc := func() Node { UseCallback(5); return Text("x") }
	if msg := panicMessage(func() { newTestTree(t, 10, 10, Component(notFunc), nil) }); !strings.Contains(msg, "UseCallback[int]: the argument must be a function") {
		t.Errorf("panic %q", msg)
	}
	nilFunc := func() Node { UseCallback[func()](nil); return Text("x") }
	if msg := panicMessage(func() { newTestTree(t, 10, 10, Component(nilFunc), nil) }); !strings.Contains(msg, "UseCallback(nil)") {
		t.Errorf("panic %q", msg)
	}
}

func TestUseRef(t *testing.T) {
	var refs []*Ref[int]
	comp := func() Node {
		r := UseRef(5)
		refs = append(refs, r)
		r.Current++
		return Text("x")
	}
	tr := newTestTree(t, 10, 10, Component(comp), nil)
	tr.Mount(Component(comp))
	if refs[0] != refs[1] || refs[1].Current != 7 {
		t.Errorf("refs %p %p, Current %d; want one Ref at 7", refs[0], refs[1], refs[1].Current)
	}
	if tr.Stats().Builds != 2 {
		t.Errorf("%d builds, want 2 (a Ref change does not rebuild)", tr.Stats().Builds)
	}
}

func TestUseId(t *testing.T) {
	var ids []string
	item := func(int) Node {
		ids = append(ids, UseId(), UseId())
		return Text("i")
	}
	page := func(n int) Node {
		items := make([]Node, n)
		for i := range items {
			items[i] = WithKey(ComponentProps(item, i), i)
		}
		return div(List(items))
	}
	mk := func(prefix string) *Tree {
		tr := NewTree(Config{Fonts: testFonts, IDPrefix: prefix})
		tr.Mount(page(2))
		return tr
	}
	tr := mk("app-")
	if want := "app-0 app-1 app-2 app-3"; strings.Join(ids, " ") != want {
		t.Errorf("ids %q, want %q", ids, want)
	}
	// New slots continue the count (the old items skip their build).
	ids = nil
	tr.Mount(page(3))
	if want := "app-4 app-5"; strings.Join(ids, " ") != want {
		t.Errorf("ids %q, want %q", ids, want)
	}
	// The same mount order gives the same IDs.
	ids = nil
	mk("app-")
	if want := "app-0 app-1 app-2 app-3"; strings.Join(ids, " ") != want {
		t.Errorf("ids %q, want %q", ids, want)
	}
	// Stable across builds.
	ids = nil
	comp := func() Node { ids = append(ids, UseId()); return Text("x") }
	tr = NewTree(Config{Fonts: testFonts, IDPrefix: "c"})
	tr.Mount(Component(comp))
	tr.Mount(Component(comp))
	if strings.Join(ids, " ") != "c0 c0" {
		t.Errorf("ids %q, want c0 c0", ids)
	}
}

func TestUseIdDefaultPrefixes(t *testing.T) {
	var ids []string
	comp := func() Node { ids = append(ids, UseId()); return Text("x") }
	NewTree(Config{Fonts: testFonts}).Mount(Component(comp))
	NewTree(Config{Fonts: testFonts}).Mount(Component(comp))
	if len(ids) != 2 || ids[0] == ids[1] || !strings.HasPrefix(ids[0], "t") || !strings.HasSuffix(ids[0], "-0") {
		t.Errorf("ids %q: want distinct t<n>-0 IDs", ids)
	}
	// Render is deterministic whatever ran before.
	ids = nil
	Render(Component(comp), RenderOptions{Config: Config{Fonts: testFonts}, Width: 10, Height: 10})
	Render(Component(comp), RenderOptions{Config: Config{Fonts: testFonts}, Width: 10, Height: 10})
	if strings.Join(ids, " ") != "r-0 r-0" {
		t.Errorf("Render ids %q, want r-0 r-0", ids)
	}
}
