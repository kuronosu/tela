package core

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// Tests of the dev-mode rules and diagnostics of ADR-085.

func TestReleaseBuildRule(t *testing.T) {
	set := func(kv ...string) []debug.BuildSetting {
		var s []debug.BuildSetting
		for i := 0; i < len(kv); i += 2 {
			s = append(s, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return s
	}
	for _, c := range []struct {
		name    string
		bi      *debug.BuildInfo
		ok      bool
		release bool
	}{
		{"no build info", nil, false, false},
		{"go run", &debug.BuildInfo{Path: "example.com/app", Main: debug.Module{Path: "example.com/app", Version: "(devel)"}}, true, false},
		{"go build in a checkout", &debug.BuildInfo{Path: "example.com/app", Main: debug.Module{Version: "v0.6.1-0.20261019154434-0340de02fe3b+dirty"},
			Settings: set("vcs", "git", "vcs.revision", "0340de0", "vcs.modified", "true")}, true, false},
		{"go build at a clean tag", &debug.BuildInfo{Path: "example.com/app", Main: debug.Module{Version: "v1.2.0"},
			Settings: set("vcs", "git", "vcs.revision", "0340de0", "vcs.modified", "false")}, true, false},
		{"go build -buildvcs=false", &debug.BuildInfo{Path: "example.com/app", Main: debug.Module{Version: "(devel)"}}, true, false},
		{"go install pkg@version", &debug.BuildInfo{Path: "example.com/app", Main: debug.Module{Version: "v1.2.0"},
			Settings: set("-buildmode", "exe", "CGO_ENABLED", "0")}, true, true},
		{"-trimpath", &debug.BuildInfo{Path: "example.com/app", Main: debug.Module{Version: "(devel)"},
			Settings: set("-trimpath", "true", "vcs", "git", "vcs.modified", "true")}, true, true},
		{"test binary", &debug.BuildInfo{Path: "example.com/app.test", Main: debug.Module{Version: "v1.2.0"},
			Settings: set("-trimpath", "true")}, true, false},
	} {
		if got := releaseFrom(c.bi, c.ok); got != c.release {
			t.Errorf("%s: release %v, want %v", c.name, got, c.release)
		}
	}
	// This test binary is a development build.
	if ReleaseBuild() {
		t.Error("a test binary counts as a release build")
	}
}

func TestResolveDev(t *testing.T) {
	on, off := true, false
	for _, c := range []struct {
		env      string
		explicit *bool
		want     bool
	}{
		{"", nil, true}, // a test binary is a development build
		{"", &off, false},
		{"", &on, true},
		{"0", nil, false},
		{"0", &on, false},
		{"1", &off, true},
		{"yes", &off, false}, // other values are ignored
	} {
		t.Setenv(EnvDev, c.env)
		if got := ResolveDev(c.explicit); got != c.want {
			t.Errorf("TELA_DEV=%q explicit %v: %v, want %v", c.env, c.explicit, got, c.want)
		}
	}
	t.Setenv(EnvDev, "0")
	if NewTree(Config{Fonts: testFonts, Dev: true}).dev {
		t.Error("TELA_DEV=0 did not turn a Config.Dev tree off")
	}
	t.Setenv(EnvDev, "1")
	if !NewTree(Config{Fonts: testFonts}).dev {
		t.Error("TELA_DEV=1 did not turn a tree on")
	}
}

// ---------------------------------------------------------------------
// Hook boundaries.

// Card is a component with state, called directly by the tests below.
func Card() Node {
	n, _ := UseState(0)
	return Textf("card %d", n)
}

// useCardState is a custom hook.
func useCardState() int {
	n, _ := UseState(1)
	return n
}

// cardHelper is neither a component of its own nor a custom hook.
func cardHelper() int { return useCardState() }

// GenericCard is a generic component.
func GenericCard[T any](p T) Node {
	UseState(p)
	inner := func() Node {
		UseRef(0)
		return Text("g")
	}
	return inner()
}

type cardView struct{ title string }

// Render is a method component.
func (c *cardView) Render() Node {
	UseState(c.title)
	return Text(c.title)
}

func TestHookBoundaryPanics(t *testing.T) {
	// Card called directly inside another component.
	page := func() Node { return div(Card()) }
	msg := panicMessage(func() { devTree(t, Component(page), nil) })
	for _, want := range []string{
		"UseState at devcheck_test.go:",
		"is called from core.Card, which is neither the component being built (a function literal in core.TestHookBoundaryPanics)",
		"wrap Card in a component boundary, tela.C(Card) or tela.CP(Card, props)",
		"name it UseCard or useCard",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("direct call: panic %q\ndoes not contain %q", msg, want)
		}
	}
	// A custom hook called from a helper that is not one: the helper is
	// named.
	page2 := func() Node { cardHelper(); return Text("x") }
	if msg := panicMessage(func() { devTree(t, Component(page2), nil) }); !strings.Contains(msg, "is called from core.cardHelper") {
		t.Errorf("custom hook in a helper: panic %q", msg)
	}
	// Hooks in the renderItem of a VirtualList take the list's slots.
	list := func() Node {
		return VirtualList(3, func(int) float32 { return 20 }, func(i int) Node {
			UseState(i)
			return Text("row")
		})
	}
	msg = panicMessage(func() {
		tr := devTree(t, Component(list), nil)
		tr.Frame()
	})
	if !strings.Contains(msg, "is called from a function literal in core.TestHookBoundaryPanics") ||
		!strings.Contains(msg, "VirtualList: return tela.CP(Row, i)") {
		t.Errorf("hook in renderItem: panic %q", msg)
	}
}

func TestHookBoundaryAccepts(t *testing.T) {
	seq := func(yield func(int) bool) { yield(1) }
	section := func(body func() Node) Node { return div(body()) } // a helper that takes a closure
	page := func() Node {
		UseState(0)
		useCardState() // a custom hook
		row := func(i int) Node {
			UseRef(i) // a closure called during the build
			return Text("r")
		}
		for range seq {
			UseId() // a range-over-func body
		}
		return div(
			row(1),
			section(func() Node {
				UseValue(0) // a closure called by a helper
				return Text("s")
			}),
			Component(Card), // a boundary of its own
			ComponentProps(GenericCard[int], 3),
			ComponentProps(GenericCard[string], "x"),
			Component((&cardView{title: "m"}).Render), // a method value
			VirtualList(2, func(int) float32 { return 20 }, func(i int) Node { return ComponentProps(GenericCard[int], i) }),
		)
	}
	var logs []string
	tr := devTree(t, Component(page), &logs)
	tr.Frame()
	tr.Mount(Component(page)) // later builds
	tr.Frame()
	for _, m := range logs {
		if strings.Contains(m, "boundary") {
			t.Errorf("message %q", m)
		}
	}
}

func TestHookBoundaryOffOutsideDev(t *testing.T) {
	page := func() Node { return div(Card()) }
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	if msg := panicMessage(func() { tr.Mount(Component(page)) }); msg != "" {
		t.Errorf("outside dev mode a direct call panics: %q", msg)
	}
}

func TestHookOutsideBuildMessage(t *testing.T) {
	// A component called where a node is expected: Mount(t, Card()).
	msg := panicMessage(func() { Card() })
	for _, want := range []string{
		"UseState called outside a component build, from core.Card (devcheck_test.go:",
		"wrap the root in tela.C(Card)",
		"telatest.Mount(t, tela.C(Card))",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic %q\ndoes not contain %q", msg, want)
		}
	}
	// Through a custom hook, the component is still found.
	if msg := panicMessage(func() { cardHelper() }); !strings.Contains(msg, "tela.C(cardHelper)") {
		t.Errorf("through a custom hook: %q", msg)
	}
	// A hook in a function literal (a handler, a goroutine) keeps the
	// general advice.
	msg = panicMessage(func() { UseRef(0) })
	if !strings.Contains(msg, "not in event handlers, effects or goroutines") || !strings.Contains(msg, "called from a function literal in core.TestHookOutsideBuildMessage") {
		t.Errorf("closure: %q", msg)
	}
}

func TestFuncNames(t *testing.T) {
	for _, c := range []struct{ fn, base, desc string }{
		{"example.com/app.Page", "Page", "app.Page"},
		{"example.com/app.Page.func1", "Page", "a function literal in app.Page"},
		{"example.com/app.Page.func1.2", "Page", "a function literal in app.Page"},
		{"example.com/app.Page-range1", "Page", "a function literal in app.Page"},
		{"example.com/app.Page.func1-range1", "Page", "a function literal in app.Page"},
		{"example.com/app.(*S).Render", "Render", "app.(*S).Render"},
		{"example.com/my.app.T.M.func3", "M", "a function literal in my.app.T.M"},
		{"main.glob..func1", "glob", "a function literal in main.glob"},
	} {
		if got := funcBase(c.fn); got != c.base {
			t.Errorf("funcBase(%q) = %q, want %q", c.fn, got, c.base)
		}
		if got := describeFunc(c.fn); got != c.desc {
			t.Errorf("describeFunc(%q) = %q, want %q", c.fn, got, c.desc)
		}
	}
	for name, want := range map[string]bool{"UseTheme": true, "useDrag": true, "Use": true, "use2": true, "User": false, "useful": false, "Card": false} {
		if isHookName(name) != want {
			t.Errorf("isHookName(%q) = %v", name, !want)
		}
	}
	if got := normFunc("app.(*Grid[...]).Render-fm"); got != "app.(*Grid).Render" {
		t.Errorf("normFunc = %q", got)
	}
	if !ownedBy("app.Page.func1.2", "app.Page") || !ownedBy("app.Page-range1", "app.Page") || ownedBy("app.PageRow", "app.Page") {
		t.Error("ownedBy")
	}
}

// ---------------------------------------------------------------------
// Warnings.

func TestNilDepsWarning(t *testing.T) {
	var logs []string
	comp := func() Node {
		UseEffect(func() func() { return nil }, nil)
		UseLayoutEffect(func() func() { return nil }, nil)
		UseMemo(func() int { return 1 }, nil)
		UseEffectDo(func() {}, nil)
		UseUpdateEffect(func() func() { return nil }, nil)
		UseEffect(func() func() { return nil }, (*int)(nil)) // a typed nil is a dependency
		UseMount(func() func() { return nil })
		return Text("x")
	}
	tr := devTree(t, Component(comp), &logs)
	tr.Frame()
	tr.Mount(Component(comp))
	tr.Frame()
	want := []string{
		"UseEffect(f, nil) at core.TestNilDepsWarning.func1 (devcheck_test.go:",
		"UseLayoutEffect(f, nil)", "tela.UseLayoutEffect(f, true)",
		"UseMemo(f, nil)", "use tela.UseOnce(f)",
		"UseEffectDo(f, nil)",
		"UseUpdateEffect(f, nil)", "it never runs",
	}
	all := strings.Join(logs, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("logs do not contain %q:\n%s", w, all)
		}
	}
	if n := strings.Count(all, "nil deps is deprecated"); n != 5 {
		t.Errorf("%d nil-deps warnings, want 5 (once per call site):\n%s", n, all)
	}
}

func TestContextDefaultWarning(t *testing.T) {
	type ctl struct {
		Name   string
		Set    Setter[int]
		OnSave func()
	}
	type safe struct {
		Name string
		Set  Setter[int]
	}
	comp := func(c *Context[ctl]) func() Node {
		return func() Node {
			v := UseContext(c)
			_ = v
			return Text("x")
		}
	}
	cases := []struct {
		name string
		root func(logs *[]string) Node
		want string
	}{
		{"zero setter", func(*[]string) Node {
			c := CreateContext(ctl{OnSave: func() {}})
			return Component(comp(c))
		}, "its field Set is a zero Setter[int]"},
		{"nil func", func(*[]string) Node {
			c := CreateContext(struct{ OnSave func() }{})
			return Component(func() Node { UseContext(c); return Text("x") })
		}, "its field OnSave is a nil func()"},
		{"nil pointer default", func(*[]string) Node {
			c := CreateContext[*int](nil)
			return Component(func() Node { UseContext(c); return Text("x") })
		}, "the default is a nil *int"},
		{"nil interface default", func(*[]string) Node {
			c := CreateContext[any](nil)
			return Component(func() Node { UseContext(c); return Text("x") })
		}, "the default is nil"},
		{"nested", func(*[]string) Node {
			type outer struct{ Inner struct{ Cb Callback[func()] } }
			c := CreateContext(outer{})
			return Component(func() Node { UseContext(c); return Text("x") })
		}, "its field Inner.Cb is a zero Callback[func()]"},
	}
	for _, c := range cases {
		var logs []string
		tr := devTree(t, c.root(&logs), &logs)
		tr.Frame()
		all := strings.Join(logs, "\n")
		if !strings.Contains(all, c.want) || !strings.Contains(all, "no Provider above") || !strings.Contains(all, "devcheck_test.go:") {
			t.Errorf("%s: logs %q, want %q", c.name, all, c.want)
		}
	}
	// A safe default, or a Provider above, says nothing.
	var logs []string
	c := CreateContext(ctl{})
	reader := func() Node { UseContext(c); return Text("x") }
	safeReader := func() Node { UseContext(CreateContext("light")); return Text("y") }
	tr := devTree(t, div(
		c.Provider(ctl{OnSave: func() {}}, Component(reader)),
		Component(safeReader),
	), &logs)
	tr.Frame()
	if len(logs) != 0 {
		t.Errorf("logs %q, want none", logs)
	}
	// Once per context.
	logs = nil
	tr = devTree(t, div(Component(reader), Component(func() Node { UseContext(c); return Text("z") })), &logs)
	tr.Frame()
	if n := len(logs); n != 1 {
		t.Errorf("%d warnings for one context, want 1: %q", n, logs)
	}
	// Unexported fields are not looked into; exported zero handles are.
	if why := zeroUnsafe(safe{Name: "x"}); why != "its field Set is a zero Setter[int]" {
		t.Errorf("zeroUnsafe(safe) = %q", why)
	}
	if why := zeroUnsafe(struct {
		Name string
		hide *int
	}{}); why != "" {
		t.Errorf("zeroUnsafe(unexported nil) = %q", why)
	}
}

func TestLabelWarning(t *testing.T) {
	cases := []struct {
		name string
		root Node
		want string // "" for no warning
	}{
		{"no control", div(label(Text("Email"))), `Label <label> "Email" labels no control`},
		{"missing id", div(label(For("mail"), Text("Email")), input(ID("email"))), `has For("mail"), but no mounted element has Id("mail")`},
		{"not a control", div(label(For("box"), Text("Box")), div(ID("box"))), `<div> has Id("box") but is not an Input, Textarea or Button`},
		{"wraps", div(label(Text("Name"), input())), ""},
		{"for after", div(label(For("e"), Text("E")), input(ID("e"))), ""},
		{"button", div(label(button(Text("Go")))), ""},
	}
	for _, c := range cases {
		var logs []string
		tr := devTree(t, c.root, &logs)
		tr.Frame()
		all := strings.Join(logs, "\n")
		if c.want == "" && all != "" || c.want != "" && !strings.Contains(all, c.want) {
			t.Errorf("%s: logs %q, want %q", c.name, all, c.want)
		}
	}
	// Once per label: rebuilding or a second identical label says nothing
	// new.
	var logs []string
	tr := devTree(t, div(label(Text("A")), label(Text("A"))), &logs)
	tr.Frame()
	tr.Mount(div(label(Text("A")), label(Text("A")), label(Text("A"))))
	tr.Frame()
	if len(logs) != 1 {
		t.Errorf("logs %q, want one warning", logs)
	}
}

func TestKeyWarningsPerCallSite(t *testing.T) {
	items := []int{1, 2, 3}
	var logs []string
	row := func(i int) Node {
		return div(Map(items, func(int) Node { return div() })) // the same call site for every row
	}
	tr := devTree(t, div(row(0), row(1), row(2)), &logs)
	tr.Frame()
	if len(logs) != 1 || !strings.Contains(logs[0], "3 children of a list (Map or List)") ||
		!strings.Contains(logs[0], "(the list made at core.TestKeyWarningsPerCallSite.func1 (devcheck_test.go:") {
		t.Errorf("keyless lists: logs %q, want one warning with the call site", logs)
	}
	logs = nil
	dup := func() Node {
		return div(Map(items, func(int) Node { return div(Key("same")) }))
	}
	tr = devTree(t, div(dup(), dup()), &logs)
	tr.Frame()
	if len(logs) != 1 || !strings.Contains(logs[0], "duplicate key same") || !strings.Contains(logs[0], "devcheck_test.go:") {
		t.Errorf("duplicate keys: logs %q, want one warning with the call site", logs)
	}
	// Siblings written by hand still warn per parent.
	logs = nil
	tr = devTree(t, div(div(div(Key(1)), div(Key(1))), div(div(Key(2)), div(Key(2)))), &logs)
	tr.Frame()
	if len(logs) != 2 {
		t.Errorf("duplicate keys among hand-written siblings: logs %q, want two", logs)
	}
}

func TestValueGetPendingWarning(t *testing.T) {
	var v *Value[int]
	var builds int
	comp := func() Node {
		v = UseValue(0)
		builds++
		_ = v.Get() // in a build: tracked, never reported
		return Text("x")
	}
	var logs []string
	tr := devTree(t, Component(comp), &logs)
	tr.Frame()
	_ = v.Get() // nothing queued
	if len(logs) != 0 {
		t.Fatalf("logs %q, want none", logs)
	}
	handler := func(x int) int {
		v.Set(x)
		return v.Get() // stale: the warning
	}
	for i, applied := range []int{0, 5} {
		if got := handler(5 + i); got != applied {
			t.Errorf("Get = %d with a change queued, want the applied %d; its semantics changed", got, applied)
		}
		if got := v.Latest(); got != 5+i {
			t.Errorf("Latest = %d", got)
		}
		tr.Frame()
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "Value.Get at core.TestValueGetPendingWarning.func2 (devcheck_test.go:") ||
		!strings.Contains(logs[0], "use Latest()") {
		t.Errorf("logs %q, want one warning naming the call site", logs)
	}
	// Outside dev mode nothing is checked.
	logs = nil
	tr2 := NewTree(Config{Fonts: testFonts, Log: func(m string) { logs = append(logs, m) }})
	tr2.SetViewport(gfx.Size{W: 100, H: 100})
	tr2.Mount(Component(comp))
	tr2.Frame()
	v.Set(1)
	v.Get()
	if len(logs) != 0 {
		t.Errorf("non-dev logs %q", logs)
	}
}

func TestIsZero(t *testing.T) {
	var s Setter[int]
	var c Callback[func()]
	if !s.IsZero() || !c.IsZero() {
		t.Error("zero handles do not report IsZero")
	}
	comp := func() Node {
		_, s = UseState(0)
		c = UseCallback(func() {})
		return Text("x")
	}
	devTree(t, Component(comp), nil)
	if s.IsZero() || c.IsZero() {
		t.Error("handles from hooks report IsZero")
	}
}
