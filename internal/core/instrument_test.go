package core

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// The instrumentation's tests (instrument.go) are not parallel: the
// creation sites are captured process-wide while a tree in dev mode is
// instrumented (insSites), and TestInstrumentOffAllocs counts the
// allocations of node constructors with none.

// newInsTree returns an instrumented tree in dev mode with root() mounted
// (made after the instrumentation is on, so its nodes have sites) and
// laid out.
func newInsTree(t *testing.T, root func() Node) *Tree {
	t.Helper()
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(root())
	tr.Frame()
	return tr
}

// here stores the line of its caller in *line and returns v: a call
// site's line, read on the line of the call it wraps.
func here[T any](line *int, v T) T {
	_, _, *line, _ = runtime.Caller(1)
	return v
}

// insByKind returns the elements of kind, in tree order.
func insByKind(tr *Tree, kind string) []ElementInfo {
	var out []ElementInfo
	for _, e := range tr.Elements(true) {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// insOne returns the one element of kind.
func insOne(t *testing.T, tr *Tree, kind string) ElementInfo {
	t.Helper()
	es := insByKind(tr, kind)
	if len(es) != 1 {
		t.Fatalf("%d elements of kind %q, want 1: %v", len(es), kind, kindsOf(tr))
	}
	return es[0]
}

func kindsOf(tr *Tree) []string {
	var out []string
	for _, e := range tr.Elements(false) {
		out = append(out, strings.Repeat(" ", e.Depth)+e.Kind)
	}
	return out
}

// lastReasons returns the reasons of the last build of e.
func lastReasons(t *testing.T, e ElementInfo) []BuildReason {
	t.Helper()
	if len(e.Recent) == 0 {
		t.Fatalf("%s has no builds", e.Kind)
	}
	return e.Recent[len(e.Recent)-1].Reasons
}

func causesOf(rs []BuildReason) []BuildCause {
	var out []BuildCause
	for _, r := range rs {
		out = append(out, r.Cause)
	}
	return out
}

func sameFile(site SourceSite) bool {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Base(site.File) == filepath.Base(file)
}

// Struct components of the tests.

type insCard struct {
	Title string
	N     int
}

func (c insCard) Build() Node { return Primitive(style.KindDiv, Text(c.Title)) }

type insList struct {
	Title string
	Items []string
}

func (c insList) Build() Node { return Primitive(style.KindDiv, Text(c.Title)) }

var insCtx = CreateContext(0)

type insCtxReader struct{}

func (insCtxReader) Build() Node { return Textf("%d", UseContext(insCtx)) }

type insValueReader struct{ V *Value[int] }

func (c insValueReader) Build() Node { return Textf("%d", c.V.Get()) }

// insPage returns its children: a struct component that a build returns,
// whose site is insPage's declaration.
type insPage struct{}

var insPageLine int

func (insPage) Build() Node { return insCard{Title: "page"} }

func init() {
	// The line of insPage.Build's declaration, above.
	pc := reflect.ValueOf(insPage.Build).Pointer()
	_, insPageLine = runtime.FuncForPC(pc).FileLine(pc)
}

// Every element gets an ID once, never reused; kinds read as a Go user
// writes them; the box and the semantics node join with Inspect and the
// aria snapshot.
func TestInstrumentElements(t *testing.T) {
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			show, _ := UseState(true)
			return Primitive(style.KindDiv, Key("panel"), Class("p-4 flex gap-2"),
				Primitive(style.KindButton, Key("save"), Text("Save")),
				If(show, insCard{Title: "hello", N: 2}),
			)
		})
	})
	es := tr.Elements(false)
	seen := map[ElementID]bool{}
	for i, e := range es {
		if e.ID == 0 || seen[e.ID] {
			t.Fatalf("element %d (%s) has ID %d: zero or repeated", i, e.Kind, e.ID)
		}
		seen[e.ID] = true
		if i > 0 && !seen[e.Parent] {
			t.Errorf("%s's parent %d comes after it", e.Kind, e.Parent)
		}
	}
	want := []string{"root", " core.TestInstrumentElements.func1.1", "  div", "   button", "    text", "   core.insCard", "    div", "     text"}
	if got := kindsOf(tr); !slices.Equal(got, want) {
		t.Errorf("kinds:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if c := insOne(t, tr, "core.insCard"); !c.Component || c.Key != "" {
		t.Errorf("insCard: %+v", c)
	}
	btn := insOne(t, tr, "button")
	if btn.Key != "save" || btn.Bounds == nil || btn.Bounds.Empty() {
		t.Errorf("button: key %q, bounds %v", btn.Key, btn.Bounds)
	}
	panel := es[2]
	if panel.Classes != "p-4 flex gap-2" || panel.Key != "panel" {
		t.Errorf("panel: %+v", panel)
	}
	if es[1].Bounds != nil {
		t.Errorf("a component owns no box: %v", es[1].Bounds)
	}

	// The IDs stay; the semantics node and the inspect node carry them.
	tr.Semantics()
	again := tr.Elements(false)
	if again[3].ID != btn.ID {
		t.Errorf("the button's ID moved from %d to %d", btn.ID, again[3].ID)
	}
	node := tr.NodeOfElement(btn.ID)
	if node == 0 || tr.ElementOfNode(node) != btn.ID {
		t.Fatalf("node of the button %d, its element %d", node, tr.ElementOfNode(node))
	}
	in, err := tr.InspectElement(node)
	if err != nil || in.Element != btn.ID || in.Kind != "button" {
		t.Errorf("InspectElement: %+v, %v", in, err)
	}
	if byID, err := tr.InspectElementID(btn.ID); err != nil || byID.Rect != in.Rect {
		t.Errorf("InspectElementID: %v, %v; want %v", byID.Rect, err, in.Rect)
	}
	if j, _ := in.MarshalJSON(); !strings.Contains(string(j), fmt.Sprintf(`"element":%d`, btn.ID)) {
		t.Errorf("the JSON lacks the element: %s", j)
	}
	if r := tr.Inspect(); r.Element == 0 {
		t.Error("the root box has no element")
	}

	// Unmounted, an element's ID is gone and not given again.
	card := insOne(t, tr, "core.insCard")
	comp := tr.ins.element(es[1].ID)
	comp.hooks[0].(*stateSlot[bool]).value = false
	tr.markBuild(comp)
	tr.Frame()
	if _, ok := tr.Element(card.ID); ok {
		t.Error("the unmounted card is still listed")
	}
	tr.Elements(false)
	for _, e := range tr.Elements(false) {
		if e.ID <= card.ID && !seen[e.ID] {
			t.Errorf("ID %d (%s) reuses an old number", e.ID, e.Kind)
		}
	}

	// Off, nothing is listed and the JSON has no element.
	tr.SetInstrument(false)
	if tr.Elements(false) != nil || tr.Inspect().Element != 0 {
		t.Error("the instrumentation is off but lists elements")
	}
	// On again, the elements get new IDs, above every old one.
	tr.SetInstrument(true)
	for _, e := range tr.Elements(false) {
		if seen[e.ID] {
			t.Errorf("%s got the old ID %d", e.Kind, e.ID)
		}
	}
}

// Hooks list their kind, call site and value; struct components their
// fields; a Provider its value.
func TestInstrumentHooksAndProps(t *testing.T) {
	var stateLine, memoLine int
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			n, _ := UseState(here(&stateLine, 41))
			label := UseMemo(here(&memoLine, func() string { return "memo" }), n)
			ref := UseRef([]int{1, 2, 3})
			v := UseValue(7)
			_ = ref
			return insCtx.Provider(n, Primitive(style.KindDiv, Text(label)), insValueReader{V: v}, insList{Title: "t", Items: []string{"a", "b"}})
		})
	})
	comp := insOne(t, tr, "core.TestInstrumentHooksAndProps.func1.1")
	want := []struct{ kind, value string }{{"UseState", "41"}, {"UseMemo", `"memo"`}, {"UseRef", "[1 2 3]"}, {"UseValue", "7"}}
	if len(comp.Hooks) != len(want) {
		t.Fatalf("hooks: %+v", comp.Hooks)
	}
	for i, w := range want {
		h := comp.Hooks[i]
		if h.Index != i || h.Kind != w.kind || h.Value != w.value || !sameFile(h.Site) {
			t.Errorf("hook %d: %+v; want %s = %s in this file", i, h, w.kind, w.value)
		}
	}
	if s := comp.Hooks[0].Site; s.Line != stateLine || !strings.HasSuffix(s.Func, "TestInstrumentHooksAndProps.func1.1") {
		t.Errorf("UseState's site %+v, want line %d", s, stateLine)
	}
	if s := comp.Hooks[1].Site; s.Line != memoLine {
		t.Errorf("UseMemo's site %v, want line %d", s, memoLine)
	}
	prov := insOne(t, tr, "Context[int].Provider")
	if len(prov.Props) != 1 || prov.Props[0] != (PropInfo{"value", "41"}) {
		t.Errorf("provider props %+v", prov.Props)
	}
	list := insOne(t, tr, "core.insList")
	if fmt.Sprint(list.Props) != `[{Title "t"} {Items ["a" "b"]}]` {
		t.Errorf("insList props %v", list.Props)
	}
	vr := insOne(t, tr, "core.insValueReader")
	if len(vr.Props) != 1 || vr.Props[0].Value != "*Value[int]" {
		t.Errorf("a *Value prop shows %+v, want its type", vr.Props)
	}
}

// The creation site of an element is the line of the test that made its
// node: a primitive, a text, a struct component given to an element, a
// struct component returned by a build (its parent's declaration).
func TestInstrumentCreationSites(t *testing.T) {
	var divLine, textLine, cardLine, compLine int
	tr := newInsTree(t, func() Node {
		return here(&compLine, Component(func() Node {
			return Primitive(style.KindForm, here(&divLine, Key("box")),
				here(&textLine, Text("hi")),
				Primitive(style.KindDiv, here(&cardLine, insCard{Title: "c"})),
				insPage{},
			)
		}))
	})
	section := insOne(t, tr, "form")
	if !sameFile(section.Site) || section.Site.Line != divLine {
		t.Errorf("form made at %+v, want this file's line %d", section.Site, divLine)
	}
	if got := section.Site.String(); got != fmt.Sprintf("instrument_test.go:%d", divLine) {
		t.Errorf("Site.String() = %q", got)
	}
	var text ElementInfo
	for _, e := range tr.Elements(false) {
		if e.Kind == "text" && e.Text == "hi" {
			text = e
		}
	}
	if text.Site.Line != textLine {
		t.Errorf("text made at %v, want line %d", text.Site, textLine)
	}
	cards := insByKind(tr, "core.insCard")
	if len(cards) != 2 {
		t.Fatalf("%d cards", len(cards))
	}
	if cards[0].Site.Line != cardLine {
		t.Errorf("the card given to a div is at %v, want line %d", cards[0].Site, cardLine)
	}
	if cards[1].Site.Line != insPageLine || !strings.Contains(cards[1].Site.Func, "insPage.Build") {
		t.Errorf("the card insPage.Build returned is at %+v, want insPage.Build's line %d", cards[1].Site, insPageLine)
	}
	// The card's own div was made in its Build method, in this file.
	for _, e := range tr.Elements(false) {
		if e.Kind == "div" && e.Parent == cards[0].ID && !strings.Contains(e.Site.Func, "insCard.Build") {
			t.Errorf("the card's div is at %+v, want insCard.Build", e.Site)
		}
	}
	if c := insOne(t, tr, "core.TestInstrumentCreationSites.func1.1"); c.Site.Line != compLine {
		t.Errorf("the root component made at %v, want line %d", c.Site, compLine)
	}
}

// The rules of creation sites for the frames of other packages.
func TestCreationSiteFrames(t *testing.T) {
	for _, tt := range []struct {
		fn, file string
		app      bool
		plumbing bool
	}{
		{"main.Settings", "/app/settings.go", true, false},
		{"example.com/app.Page.Build", "/app/page.go", true, false},
		{telaPkg + "/examples/dashboard.Preview", "/tela/examples/dashboard/main.go", true, false},
		{telaPkg + "/kit.Button.Build", "/tela/kit/button.go", false, false},
		{telaPkg + "/aria.UseButton", "/tela/aria/button.go", false, false},
		{telaPkg + "/h.Div", "/tela/h/elements.go", false, true},
		{telaPkg + ".C", "/tela/tela.go", false, true},
		{corePkg + ".Primitive", "/tela/internal/core/node.go", false, true},
		{corePkg + ".TestX.func1", "/tela/internal/core/x_test.go", true, false},
		{telaPkg + "x/other.F", "/telax/other.go", true, false},
	} {
		if got := appFrame(tt.fn, tt.file); got != tt.app {
			t.Errorf("appFrame(%s) = %v", tt.fn, got)
		}
		if got := frameworkFrame(tt.fn, tt.file); got != tt.plumbing {
			t.Errorf("frameworkFrame(%s) = %v", tt.fn, got)
		}
	}
	for _, fn := range []string{corePkg + ".(*Tree).build", corePkg + ".buildFrame", corePkg + ".(*compDesc).run"} {
		if !buildMachinery(fn) {
			t.Errorf("%s is not the build machinery", fn)
		}
	}
	if buildMachinery(corePkg + ".Primitive") {
		t.Error("Primitive is the build machinery")
	}
}

// goName removes package paths.
func TestGoName(t *testing.T) {
	for in, want := range map[string]string{
		"github.com/kuronosu/tela/kit.Button":               "kit.Button",
		"main.App.func1":                                    "main.App.func1",
		"example.com/x/app.List[example.com/x/y.Item]":      "app.List[y.Item]",
		"Context[github.com/a/b.Theme]":                     "Context[b.Theme]",
		"github.com/a/b.(*T).M":                             "b.(*T).M",
		"map[string]github.com/a/b.T":                       "map[string]b.T",
		"github.com/a/b.F[github.com/c/d.X,github.com/e.Y]": "b.F[d.X,e.Y]",
	} {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The short form of values is bounded and deterministic.
func TestShortValue(t *testing.T) {
	type pt struct{ X, Y int }
	type wrap struct {
		P    *pt
		Name string
		f    func()
	}
	long := strings.Repeat("x", 500)
	for _, tt := range []struct {
		v    any
		want string
	}{
		{nil, "nil"},
		{3, "3"},
		{float32(0.1), "0.1"},
		{"a\"b", `"a\"b"`},
		{[]int{1, 2}, "[1 2]"},
		{[]int(nil), "[]"},
		{make([]int, 20), "[0 0 0 0 0 0 0 0 …+12]"},
		{map[string]int{"b": 2, "a": 1}, "map[\"a\":1 \"b\":2]"},
		{pt{1, 2}, "{X:1 Y:2}"},
		{wrap{P: &pt{3, 4}, Name: "n", f: nil}, "{P:&core.pt{X:3 Y:4} Name:\"n\" f:nil}"},
		{&pt{5, 6}, "&core.pt{X:5 Y:6}"},
		{fmt.Errorf("boom"), "boom"},
		{Setter[int]{}, "Setter[int]"},
		{node{&desc{kind: nodeElement, elem: style.KindDiv}}, "<div>"},
		{func() {}, "func"},
	} {
		if got := shortValue(tt.v); got != tt.want {
			t.Errorf("shortValue(%#v) = %q, want %q", tt.v, got, tt.want)
		}
	}
	if s := shortValue(long); len(s) > maxValueLen+4 || !strings.HasSuffix(s, "…\"") {
		t.Errorf("a long string: %d bytes, %q", len(s), s[len(s)-8:])
	}
	big := make([][]string, 50)
	for i := range big {
		big[i] = []string{long, long}
	}
	if s := shortValue(big); len(s) > maxValueLen+len("…") {
		t.Errorf("a big slice: %d bytes", len(s))
	}
	type cyc struct{ Next *cyc }
	c := &cyc{}
	c.Next = c
	if s := shortValue(c); s != "&core.cyc{Next:&core.cyc{…}}" {
		t.Errorf("a cycle: %q", s)
	}
	// Unexported fields are read without Interface: a node, a struct
	// component, a nil Node, a Stringer, a map and a pointer cycle behind
	// them.
	type shy struct {
		body Node
		card Node
		none Node
	}
	if got, want := shortValue(shy{body: Primitive(style.KindDiv), card: insCard{Title: "c"}}),
		`{body:<div> card:core.insCard{Title:"c" N:0} none:nil}`; got != want {
		t.Errorf("unexported nodes:\n got %s\nwant %s", got, want)
	}
	type shy2 struct {
		err   error
		m     map[string]Node
		cycle *cyc
	}
	if got, want := shortValue(shy2{err: errors.New("boom"), m: map[string]Node{"b": Text("t"), "a": nil}, cycle: c}),
		`{err:&errors.errorString{s:"boom"} m:map["a":nil "b":"t"] cycle:&core.cyc{Next:&…}}`; got != want {
		t.Errorf("unexported fields:\n got %s\nwant %s", got, want)
	}
}

// insShy is a struct component with an unexported field of type Node.
type insShy struct {
	Title string
	body  Node
}

func (c insShy) Build() Node { return Primitive(style.KindDiv, Text(c.Title), c.body) }

// The props of a struct component with an unexported Node field are
// listed, not a panic.
func TestInstrumentUnexportedNodeProp(t *testing.T) {
	tr := newInsTree(t, func() Node {
		return Primitive(style.KindDiv, insShy{Title: "card", body: Primitive(style.KindP, Text("x"))})
	})
	c := insOne(t, tr, "core.insShy")
	if fmt.Sprint(c.Props) != `[{Title "card"} {body <p>}]` {
		t.Errorf("props %v", c.Props)
	}
	if e, ok := tr.Element(c.ID); !ok || len(e.Props) != 2 {
		t.Errorf("Element: %+v, %v", e, ok)
	}
}

// A setter's build names the hook and its site; a mount is a mount.
func TestInstrumentReasonState(t *testing.T) {
	var set Setter[int]
	var line int
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			n, s := UseState(here(&line, 0))
			set = s
			return Textf("%d", n)
		})
	})
	const kind = "core.TestInstrumentReasonState.func1.1"
	c := insOne(t, tr, kind)
	if got := causesOf(lastReasons(t, c)); !slices.Equal(got, []BuildCause{CauseMount}) || c.Builds != 1 {
		t.Errorf("first build: %v, %d builds", got, c.Builds)
	}
	set.Set(1)
	set.Set(2)
	tr.Frame()
	c = insOne(t, tr, kind)
	rs := lastReasons(t, c)
	if len(rs) != 1 || rs[0].Cause != CauseState || rs[0].Hook != 0 || rs[0].HookKind != "UseState" || rs[0].Site.Line != line {
		t.Fatalf("reasons %+v; want UseState #1 at line %d", rs, line)
	}
	if got, want := rs[0].String(), fmt.Sprintf("UseState #1 at instrument_test.go:%d was set", line); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if c.Builds != 2 || len(c.Recent) != 2 || c.Recent[1].Frame != tr.Stats().Frames {
		t.Errorf("%d builds, recent %+v, frame %d", c.Builds, c.Recent, tr.Stats().Frames)
	}
	// The history keeps the last builds.
	for i := range 3 * maxBuildHistory {
		set.Set(10 + i)
		tr.Frame()
	}
	if c = insOne(t, tr, kind); len(c.Recent) != maxBuildHistory || c.Builds != 2+3*maxBuildHistory {
		t.Errorf("%d recent builds of %d", len(c.Recent), c.Builds)
	}
}

// A context's new value names the context and its Provider; a component
// with equal props builds for nothing else.
func TestInstrumentReasonContext(t *testing.T) {
	var set Setter[int]
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			n, s := UseState(0)
			set = s
			return insCtx.Provider(n, Primitive(style.KindDiv, insCtxReader{}))
		})
	})
	set.Set(5)
	tr.Frame()
	r := insOne(t, tr, "core.insCtxReader")
	prov := insOne(t, tr, "Context[int].Provider")
	rs := lastReasons(t, r)
	if len(rs) != 1 || rs[0].Cause != CauseContext || rs[0].Context != "Context[int]" || rs[0].Element != prov.ID ||
		rs[0].HookKind != "UseContext" || !sameFile(rs[0].Site) {
		t.Errorf("reasons %+v; want the context of provider %d", rs, prov.ID)
	}
	if r.Builds != 2 {
		t.Errorf("%d builds", r.Builds)
	}
}

// Unequal struct props name the fields that differ and those that cannot
// be compared; a component of tela.C builds with its parent.
func TestInstrumentReasonProps(t *testing.T) {
	var set Setter[string]
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			title, s := UseState("a")
			set = s
			return Primitive(style.KindDiv,
				insCard{Title: title, N: 1},
				insList{Title: "same", Items: []string{"x"}},
				Component(func() Node { return Text("child") }),
			)
		})
	})
	set.Set("b")
	tr.Frame()
	card := insOne(t, tr, "core.insCard")
	if rs := lastReasons(t, card); len(rs) != 1 || rs[0].Cause != CauseProps || !slices.Equal(rs[0].Fields, []string{"Title"}) || rs[0].Uncomparable != nil {
		t.Errorf("card: %+v", rs)
	} else if got := rs[0].String(); got != "props: Title differ" {
		t.Errorf("card: %q", got)
	}
	list := insOne(t, tr, "core.insList")
	if rs := lastReasons(t, list); len(rs) != 1 || rs[0].Cause != CauseProps || rs[0].Fields != nil || !slices.Equal(rs[0].Uncomparable, []string{"Items"}) {
		t.Errorf("list: %+v", rs)
	}
	child := insOne(t, tr, "core.TestInstrumentReasonProps.func1.1.1")
	if rs := lastReasons(t, child); !slices.Equal(causesOf(rs), []BuildCause{CauseParent}) {
		t.Errorf("tela.C child: %+v", rs)
	}
	// Equal props skip: the card did not build again.
	tr.markBuild(tr.ins.element(insOne(t, tr, "core.TestInstrumentReasonProps.func1.1").ID))
	tr.Frame()
	if c := insOne(t, tr, "core.insCard"); c.Builds != 2 {
		t.Errorf("the card built %d times, want 2 (equal props skip)", c.Builds)
	}
}

// A Value read in a build names the component and the hook of the Value.
func TestInstrumentReasonValue(t *testing.T) {
	var v *Value[int]
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			UseState(0)
			v = UseValue(1)
			return Primitive(style.KindDiv, insValueReader{V: v})
		})
	})
	v.Set(2)
	tr.Frame()
	owner := insOne(t, tr, "core.TestInstrumentReasonValue.func1.1")
	r := insOne(t, tr, "core.insValueReader")
	rs := lastReasons(t, r)
	if len(rs) != 1 || rs[0].Cause != CauseValue || rs[0].Element != owner.ID || rs[0].Hook != 1 || rs[0].HookKind != "UseValue" {
		t.Errorf("reasons %+v; want UseValue #2 of %d", rs, owner.ID)
	}
	if owner.Builds != 1 {
		t.Errorf("the owner built %d times", owner.Builds)
	}
}

// Off, the instrumentation adds no allocation to a build, a frame or a
// node constructor.
func TestInstrumentOffAllocs(t *testing.T) {
	if n := insSites.Load(); n != 0 {
		t.Fatalf("%d instrumented trees capture creation sites", n)
	}
	kept := Primitive(style.KindDiv, Text("kept"))
	tr := newTestTree(t, 200, 100, Component(func() Node {
		UseState(0)
		return kept
	}), nil)
	tr.Frame()
	comp := tr.root.children[0]
	build := func() float64 {
		// Under buildMu, without withBuildLock: in a test binary, whose
		// trees are in both modes, it reads the goroutine's id for devNow,
		// which allocates.
		return testing.AllocsPerRun(200, func() {
			buildMu.Lock()
			tr.build(comp)
			buildMu.Unlock()
		})
	}
	frame := func() float64 {
		return testing.AllocsPerRun(200, func() {
			tr.markBuild(comp)
			tr.Frame()
		})
	}
	builds := tr.Stats().Builds
	if a := build(); a != 0 {
		t.Errorf("a build: %v allocations, want 0", a)
	}
	if tr.Stats().Builds == builds {
		t.Fatal("nothing built")
	}
	frames := frame()
	if a := testing.AllocsPerRun(200, func() { _ = Text("x") }); a != 1 {
		t.Errorf("Text: %v allocations, want 1 (its desc)", a)
	}
	if a := testing.AllocsPerRun(200, func() { _ = childNode(insCard{Title: "x"}) }); a != 2 {
		t.Errorf("a struct component's node: %v allocations, want 2 (its desc and compDesc)", a)
	}
	// Turned on and off again, it costs nothing either.
	tr.SetInstrument(true)
	tr.markBuild(comp)
	tr.Frame()
	if a := build(); a == 0 {
		t.Error("an instrumented build allocates nothing: is it recorded?")
	}
	tr.SetInstrument(false)
	if a := build(); a != 0 {
		t.Errorf("a build after an instrumented session: %v allocations, want 0", a)
	}
	if a := frame(); a != frames {
		t.Errorf("a frame after an instrumented session: %v allocations, want %v as before", a, frames)
	}
}

// The windows of an app are instrumented together: a child window opened
// while its parent's tree is instrumented is too, and its tree stops
// counting when it closes.
func TestInstrumentChildWindows(t *testing.T) {
	var setOpen Setter[bool]
	x := newWinHarness(t, Options{Config: Config{Dev: true}, Root: Component(func() Node {
		open, s := UseState(false)
		setOpen = s
		return Primitive(style.KindDiv, Text("main"), If(open, win(ChildWindowSpec{
			WindowSpec: WindowSpec{Title: "Child"}, OnClose: func() {}, Root: Primitive(style.KindP, Text("child")),
		})))
	})})
	base := insSites.Load()
	x.h.SetInstrument(true)
	t.Cleanup(func() { x.h.SetInstrument(false) })
	setOpen.Set(true)
	x.drain()
	trees := x.h.Trees()
	if len(trees) != 2 || !trees[0].Instrumented() || !trees[1].Instrumented() || insSites.Load() != base+2 {
		t.Fatalf("%d trees, instrumented sites %d", len(trees), insSites.Load()-base)
	}
	if p := insOne(t, trees[1], "p"); p.Bounds == nil {
		t.Errorf("the child's paragraph: %+v", p)
	}
	setOpen.Set(false)
	x.drain()
	if trees[1].Instrumented() || insSites.Load() != base+1 {
		t.Errorf("after the close: instrumented %v, sites %d", trees[1].Instrumented(), insSites.Load()-base)
	}
	x.h.SetInstrument(false)
	if insSites.Load() != base {
		t.Errorf("after SetInstrument(false): %d", insSites.Load()-base)
	}
}

// The cost of the instrumentation on, against BenchmarkMount10k and
// BenchmarkUpdateText10k (bench_test.go), which have it off.
func BenchmarkInstrumentedMount10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	b.ReportAllocs()
	for b.Loop() {
		tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
		tr.SetViewport(gfx.Size{W: 800, H: 600})
		tr.SetInstrument(true)
		tr.Mount(bigList(benchRows, setters))
		tr.Frame()
		tr.SetInstrument(false)
	}
}

func BenchmarkInstrumentedUpdateText10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
	tr.SetViewport(gfx.Size{W: 800, H: 600})
	tr.SetInstrument(true)
	b.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(bigList(benchRows, setters))
	tr.Frame()
	labels := [2]string{"Updated label", "Another label"}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		setters[benchRows/2](labels[i&1])
		i++
		tr.Frame()
	}
}

// The listing of a 10k-element tree.
func BenchmarkInstrumentedElements10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
	tr.SetViewport(gfx.Size{W: 800, H: 600})
	tr.SetInstrument(true)
	b.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(bigList(benchRows, setters))
	tr.Frame()
	b.ReportAllocs()
	for b.Loop() {
		tr.Elements(false)
	}
}
