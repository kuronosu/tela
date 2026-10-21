package h_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Tests of the phase-7 ergonomics (ADR-091): Class with parts, handlers
// without the event, Bind with a func, MapKeyed, Nodes, Scale, StyleProp
// as a child and the VirtualList props.

func panicText(f func()) (msg string) {
	defer func() { msg = fmt.Sprint(recover()) }()
	f()
	return "no panic"
}

func TestClassParts(t *testing.T) {
	root := Div(Class("flex flex-col gap-2 p-4"),
		Div(Key("join"), Class("h-10", "", "w-24", "  ", "bg-red-500")),
		Div(Key("one"), Class("h-10 w-24")),
		Div(Key("none"), Class(), Class("h-4 w-4")),
	)
	b := inspect(t, root, 400, 300)
	if got := b.find("join").Style.Classes; got != "h-10 w-24 bg-red-500" {
		t.Errorf("Class parts joined as %q", got)
	}
	if got := b.find("one").Style.Classes; got != "h-10 w-24" {
		t.Errorf("one part changed to %q", got)
	}
	if got := b.find("none").Style.Classes; got != "h-4 w-4" {
		t.Errorf("Class() added %q", got)
	}
	if r := b.find("join").Rect; r.Width != 96 || r.Height != 40 {
		t.Errorf("joined classes laid out %vx%v, want 96x40", r.Width, r.Height)
	}
}

// handlerCount counts the calls of the two shapes of a handler.
type handlerCount struct{ plain, event map[string]int }

func newCounts() *handlerCount {
	return &handlerCount{plain: map[string]int{}, event: map[string]int{}}
}

// plainFn and eventFn return the two shapes of the handler named name.
func (c *handlerCount) plainFn(name string) func() { return func() { c.plain[name]++ } }

func eventFn[E any](c *handlerCount, name string) func(E) {
	return func(E) { c.event[name]++ }
}

func (c *handlerCount) check(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if c.plain[n] == 0 || c.event[n] == 0 || c.plain[n] != c.event[n] {
			t.Errorf("%s: func() ran %d times, func(event) %d times; want both, equally often", n, c.plain[n], c.event[n])
		}
	}
}

func TestHandlersTakeFuncOrFuncWithEvent(t *testing.T) {
	c := newCounts()
	page := func() tela.Node {
		return Div(Class("flex flex-col gap-2 p-2"),
			// Two elements per kind, one with each shape, nested so that
			// one gesture reaches both (events bubble).
			Div(
				OnClick(c.plainFn("click")), OnMouseDown(c.plainFn("mousedown")), OnMouseUp(c.plainFn("mouseup")),
				OnPointerDown(c.plainFn("pointerdown")), OnPointerUp(c.plainFn("pointerup")),
				OnDoubleClick(c.plainFn("dblclick")),
				Div(TestID("click"), Class("h-8 w-40 bg-slate-200"),
					OnClick(eventFn[tela.MouseEvent](c, "click")), OnMouseDown(eventFn[tela.MouseEvent](c, "mousedown")),
					OnMouseUp(eventFn[tela.MouseEvent](c, "mouseup")),
					OnPointerDown(eventFn[tela.PointerEvent](c, "pointerdown")), OnPointerUp(eventFn[tela.PointerEvent](c, "pointerup")),
					OnDoubleClick(eventFn[tela.MouseEvent](c, "dblclick")),
					Text("click"))),
			Div(
				OnMouseEnter(c.plainFn("mouseenter")), OnMouseLeave(c.plainFn("mouseleave")), OnPointerEnter(c.plainFn("pointerenter")),
				OnPointerLeave(c.plainFn("pointerleave")),
				Div(TestID("hover"), Class("h-8 w-40 bg-slate-300"),
					OnMouseEnter(eventFn[tela.MouseEvent](c, "mouseenter")), OnMouseLeave(eventFn[tela.MouseEvent](c, "mouseleave")),
					OnPointerEnter(eventFn[tela.PointerEvent](c, "pointerenter")), OnPointerLeave(eventFn[tela.PointerEvent](c, "pointerleave")),
					Text("hover"))),
			Div(
				OnKeyDown(c.plainFn("keydown")), OnKeyUp(c.plainFn("keyup")), OnFocus(c.plainFn("focus")), OnBlur(c.plainFn("blur")),
				Button(Class("h-8 w-40 bg-slate-400"),
					OnKeyDown(eventFn[tela.KeyboardEvent](c, "keydown")), OnKeyUp(eventFn[tela.KeyboardEvent](c, "keyup")),
					OnFocus(eventFn[tela.FocusEvent](c, "focus")), OnBlur(eventFn[tela.FocusEvent](c, "blur")),
					Text("focus"))),
			Button(Class("h-8 w-40 bg-slate-500"), Text("other")),
			// Drag events do not bubble: both shapes on the same element.
			Div(TestID("drag"), Class("h-8 w-40 bg-slate-600"),
				OnDragStart(c.plainFn("dragstart")), OnDrag(c.plainFn("drag")), OnDragEnd(c.plainFn("dragend")),
				OnDragStart(eventFn[tela.DragEvent](c, "dragstart")), OnDrag(eventFn[tela.DragEvent](c, "drag")),
				OnDragEnd(eventFn[tela.DragEvent](c, "dragend")), Text("drag")),
			Div(TestID("wheel"), Class("h-16 overflow-y-auto"),
				OnWheel(c.plainFn("wheel")), OnScroll(c.plainFn("scroll")),
				Div(Class("h-64 w-40 bg-slate-700"),
					OnWheel(eventFn[tela.WheelEvent](c, "wheel")), Text("wheel"))),
			Form(OnSubmit(c.plainFn("submit")), OnChange(c.plainFn("change")), OnBeforeInput(c.plainFn("beforeinput")),
				OnSelect(c.plainFn("select")),
				OnSubmit(eventFn[tela.SubmitEvent](c, "submit")), OnChange(eventFn[tela.ChangeEvent](c, "change")),
				OnBeforeInput(eventFn[tela.InputEvent](c, "beforeinput")), OnSelect(eventFn[tela.SelectEvent](c, "select")),
				Input(AriaLabel("field"), Class("h-8 w-40 border"))),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))

	app.Click(telatest.ByTestID("click"))
	app.DoubleClick(telatest.ByTestID("click"))
	c.check(t, "click", "mousedown", "mouseup", "pointerdown", "pointerup", "dblclick")

	app.Hover(telatest.ByTestID("hover"))
	app.Hover(telatest.ByTestID("click"))
	c.check(t, "mouseenter", "mouseleave", "pointerenter", "pointerleave")

	app.Focus(telatest.ByText("focus"))
	app.Press("a")
	app.Focus(telatest.ByText("other"))
	c.check(t, "keydown", "keyup", "focus", "blur")

	app.Drag(telatest.ByTestID("drag"), telatest.ByTestID("hover"))
	c.check(t, "dragstart", "drag", "dragend")

	app.Scroll(telatest.ByTestID("wheel"), 0, 20)
	c.check(t, "wheel")
	if c.plain["scroll"] == 0 {
		t.Error("OnScroll(func()) did not run")
	}

	app.Focus(telatest.ByRole("textbox", "field"))
	app.Type("ab")
	app.Press("Control+a")
	c.check(t, "change", "beforeinput", "select")
	app.Press("Enter")
	c.check(t, "submit")
}

func TestHandlerLongPressAndContextMenuShapes(t *testing.T) {
	c := newCounts()
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(
			Div(TestID("a"), Class("h-10 w-40 bg-slate-300"), OnLongPress(c.plainFn("longpress")), Text("a")),
			Div(TestID("b"), Class("h-10 w-40 bg-slate-400"), OnLongPress(eventFn[tela.PointerEvent](c, "longpress")), Text("b")),
			// Built, not driven: both shapes compile for every handler.
			Div(OnContextMenu(func() {}), OnContextMenu(func(tela.MouseEvent) {}),
				OnMouseMove(func() {}), OnPointerMove(func() {}), OnPointerCancel(func() {}),
				OnCompositionStart(func() {}), OnCompositionUpdate(func(tela.CompositionEvent) {}), OnCompositionEnd(func() {})),
		)
	}, telatest.Size(300, 200))
	app.Click(telatest.ByTestID("a"), telatest.Delay(700*time.Millisecond))
	app.Click(telatest.ByTestID("b"), telatest.Delay(700*time.Millisecond))
	if c.plain["longpress"] != 1 || c.event["longpress"] != 1 {
		t.Errorf("long presses: %v and %v, want 1 each", c.plain, c.event)
	}
}

func TestHandlerNilPanicsNameBothShapes(t *testing.T) {
	var click func(tela.MouseEvent)
	var key func()
	for name, tc := range map[string]struct {
		f    func()
		want string
	}{
		"OnClick":   {func() { OnClick(click) }, "OnClick(func() { ... }) or OnClick(func(e tela.MouseEvent) { ... })"},
		"OnKeyDown": {func() { OnKeyDown(key) }, "OnKeyDown(func(e tela.KeyboardEvent) { ... })"},
		"OnSubmit":  {func() { OnSubmit[func()](nil) }, "OnSubmit(func(e tela.SubmitEvent) { ... })"},
	} {
		if msg := panicText(tc.f); !strings.Contains(msg, tc.want) {
			t.Errorf("%s(nil): panic %q does not contain %q", name, msg, tc.want)
		}
	}
}

func TestBindWithFunc(t *testing.T) {
	type action struct{ text string }
	var typed []string
	page := func() tela.Node {
		text, d := tela.UseReducer(func(_ string, a action) string { return a.text }, "")
		viaSetter, setter := tela.UseState("")
		return Form(
			// A func(string): a closure over a Dispatcher.
			Input(AriaLabel("reducer"), Bind(text, func(s string) { typed = append(typed, s); d.Dispatch(action{s}) })),
			Input(AriaLabel("setter"), Bind(viaSetter, setter)),
			P(Textf("reducer=%s setter=%s", text, viaSetter)),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Focus(telatest.ByRole("textbox", "reducer"))
	app.Type("hi")
	app.Focus(telatest.ByRole("textbox", "setter"))
	app.Type("yo")
	app.ExpectText("reducer=hi setter=yo")
	if len(typed) != 2 || typed[1] != "hi" {
		t.Errorf("the func received %q, want [h hi]", typed)
	}
	if msg := panicText(func() { Bind[func(string)]("x", nil) }); !strings.Contains(msg, "Bind(text, nil)") {
		t.Errorf("Bind(nil func): %q", msg)
	}
	if msg := panicText(func() { Bind("x", tela.Setter[string]{}) }); !strings.Contains(msg, "zero Setter") {
		t.Errorf("Bind(zero Setter): %q", msg)
	}
}

// todoRow is a struct component with its own state: a counter of clicks.
type todoRow struct {
	ID   int
	Name string
}

func (r todoRow) Build() tela.Node {
	n, setN := tela.UseState(0)
	return Button(Textf("%s:%d", r.Name, n), OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
}

func rowTexts(app *telatest.App) string {
	return strings.ReplaceAll(strings.TrimSpace(app.AriaSnapshot()), "\n", " ")
}

func TestMapKeyedKeepsStateAcrossReorder(t *testing.T) {
	var set tela.Setter[[]todoRow]
	page := func() tela.Node {
		rows, s := tela.UseState([]todoRow{{1, "a"}, {2, "b"}, {3, "c"}})
		set = s
		return Div(
			MapKeyed(rows, func(r todoRow) int { return r.ID }, func(r todoRow) tela.Component { return r }),
			// A node, with the key replaced by the item's.
			MapKeyed([]int{1, 2}, func(i int) string { return fmt.Sprint("n", i) }, func(i int) tela.Component {
				return Span(Key("ignored"), Textf("n%d", i))
			}),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Click(telatest.ByText("b:0"))
	app.Click(telatest.ByText("b:1"))
	app.Click(telatest.ByText("c:0"))
	set.Set([]todoRow{{3, "c"}, {2, "b"}, {4, "d"}, {1, "a"}})
	app.ExpectAria(`
		- button "c:1"
		- button "b:2"
		- button "d:0"
		- button "a:0"
		- text: n1n2`)
}

func TestMapKeyedFunctionComponentsAndPanics(t *testing.T) {
	type props struct{ ID int }
	row := func(p props) tela.Node {
		n, setN := tela.UseState(0)
		return Button(Textf("r%d:%d", p.ID, n), OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
	}
	var set tela.Setter[[]int]
	page := func() tela.Node {
		ids, s := tela.UseState([]int{1, 2})
		set = s
		return Div(MapKeyed(ids, func(i int) int { return i }, func(i int) tela.Component { return tela.CP(row, props{i}) }))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Click(telatest.ByText("r2:0"))
	set.Set([]int{2, 1})
	app.ExpectAria(`
		- button "r2:1"
		- button "r1:0"`)
	if msg := panicText(func() { MapKeyed[int, int]([]int{1}, nil, func(int) tela.Component { return nil }) }); !strings.Contains(msg, "MapKeyed(items, nil") {
		t.Errorf("nil key func: %q", msg)
	}
	if msg := panicText(func() { MapKeyed[int, int]([]int{1}, func(i int) int { return i }, nil) }); !strings.Contains(msg, "MapKeyed(items, key, nil") {
		t.Errorf("nil render: %q", msg)
	}
	if msg := panicText(func() {
		MapKeyed([]int{1}, func(i int) int { return i }, func(int) tela.Component { return Class("x").Build() })
	}); msg == "no panic" {
		// A prop node is not a component.
		t.Error("a Prop was accepted by MapKeyed")
	}
}

func TestNodes(t *testing.T) {
	var cells []tela.Node
	for _, s := range []string{"a", "b", "c"} {
		cells = append(cells, Div(Key(s), Class("h-4"), Text(s)))
	}
	withNodes := inspect(t, Div(Key("root"), Class("w-[100px]"), Div(Key("first"), Class("h-2")), Nodes(cells), Div(Key("last"), Class("h-2"))), 200, 200)
	plain := inspect(t, Div(Key("root"), Class("w-[100px]"), Div(Key("first"), Class("h-2")), cells[0], cells[1], cells[2], Div(Key("last"), Class("h-2"))), 200, 200)
	for _, k := range []string{"first", "a", "b", "c", "last"} {
		a, b := withNodes.find(k), plain.find(k)
		if a == nil || b == nil || a.Rect != b.Rect {
			t.Errorf("%s: %+v with Nodes, %+v as children", k, a, b)
		}
	}
	if n := len(withNodes.find("root").Children); n != 5 {
		t.Errorf("Nodes added %d children, want 5 (no box of its own)", n)
	}
	// The slice is copied: changing it later changes nothing.
	n := Nodes(cells)
	cells[0] = Div(Key("changed"))
	if b := inspect(t, Div(n), 100, 100); b.find("changed") != nil || b.find("a") == nil {
		t.Error("Nodes kept the caller's slice")
	}
}

func TestScaleDrivesDeferredProps(t *testing.T) {
	var v *tela.Value[float32]
	builds := 0
	app := telatest.MountFunc(t, func() tela.Node {
		builds++
		v = tela.UseValue(float32(0.5))
		return Div(Class("w-40"),
			Div(TestID("box"), Class("w-10 bg-sky-500"), Style(tela.HeightFn(Scale(v, 36)))),
			Div(TestID("mapped"), Class("h-4 bg-sky-300"), Style(tela.WidthFn(tela.MapValue(v, func(x float32) float32 { return 10 + 20*x })))),
		)
	}, telatest.Size(300, 200))
	if h := app.Bounds(telatest.ByTestID("box")).Height; h != 18 {
		t.Errorf("Scale(v, 36) with v = 0.5: height %v, want 18", h)
	}
	if w := app.Bounds(telatest.ByTestID("mapped")).Width; w != 20 {
		t.Errorf("MapValue: width %v, want 20", w)
	}
	v.Set(1)
	app.Advance(32 * time.Millisecond)
	if h, w := app.Bounds(telatest.ByTestID("box")).Height, app.Bounds(telatest.ByTestID("mapped")).Width; h != 36 || w != 30 {
		t.Errorf("after Set(1): height %v width %v, want 36 and 30", h, w)
	}
	if builds != 1 {
		t.Errorf("the component built %d times: the Value was read outside the deferred props", builds)
	}
	if msg := panicText(func() { Scale(nil, 2) }); !strings.Contains(msg, "Scale(nil") {
		t.Errorf("Scale(nil): %q", msg)
	}
}

func TestStylePropsAsChildren(t *testing.T) {
	root := Div(Class("relative h-[100px] w-[200px]"),
		Div(Key("child"), Class("w-4 h-4"), tela.Width(120), tela.Height(30), tela.Bg(tela.Hex("#336699"))),
		Div(Key("style"), Class("w-4 h-4"), Style(tela.Width(120), tela.Height(30), tela.Bg(tela.Hex("#336699")))),
		Div(Key("wh"), tela.WH(50, 20)),
		Div(Key("box"), Class("absolute"), tela.Box(7, 9, 40, 25)),
		Div(Key("if-on"), If(true, tela.Opacity(0.5)), Class("h-2 w-2")),
		Div(Key("if-off"), If(false, tela.Width(99)), Class("h-2 w-2")),
		Fragment(If(false, tela.Width(1)), Div(Key("in-fragment"), Class("h-1"))),
	)
	b := inspect(t, root, 300, 300)
	child, style := b.find("child"), b.find("style")
	if child.Rect.Width != 120 || child.Rect.Height != 30 || child.Style.Background != "#336699" ||
		child.Rect.Width != style.Rect.Width || child.Rect.Height != style.Rect.Height || child.Style.Background != style.Style.Background {
		t.Errorf("StyleProp children: %+v, Style(...): %+v", child, style)
	}
	if wh := b.find("wh"); wh.Rect.Width != 50 || wh.Rect.Height != 20 {
		t.Errorf("WH(50, 20): %vx%v", wh.Rect.Width, wh.Rect.Height)
	}
	if bx := b.find("box").Rect; bx.X != 7 || bx.Y != 9 || bx.Width != 40 || bx.Height != 25 {
		t.Errorf("Box(7, 9, 40, 25): %+v", bx)
	}
	if got := b.find("if-on").Style.Opacity; got != 0.5 {
		t.Errorf("If(true, Opacity(0.5)): opacity %v", got)
	}
	if got := b.find("if-off").Rect.Width; got != 8 {
		t.Errorf("If(false, Width(99)): width %v, want 8", got)
	}
	if b.find("in-fragment") == nil {
		t.Error("a zero StyleProp broke a Fragment")
	}
	if msg := panicText(func() { Fragment(tela.Width(1)) }); !strings.Contains(msg, "Fragment does not take Style") {
		t.Errorf("Fragment(StyleProp): %q", msg)
	}
	if msg := panicText(func() { tela.WithKey(tela.Width(1), 1) }); !strings.Contains(msg, "Prop") {
		t.Errorf("WithKey(StyleProp): %q", msg)
	}
}

// --- VirtualList ---------------------------------------------------------

type keyedRow struct{ ID int }

func (r keyedRow) Build() tela.Node {
	n, setN := tela.UseState(0)
	return Button(Class("block h-[30px] w-full text-left"), Textf("row%d:%d", r.ID, n),
		OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
}

func TestVirtualListItemKeyFollowsTheItem(t *testing.T) {
	for _, keyed := range []bool{true, false} {
		t.Run(fmt.Sprint("keyed=", keyed), func(t *testing.T) {
			var set tela.Setter[[]keyedRow]
			page := func() tela.Node {
				rows, s := tela.UseState([]keyedRow{{10}, {11}, {12}, {13}, {14}})
				set = s
				props := []tela.Prop{Class("h-[150px] w-[200px]"), Overscan(0)}
				if keyed {
					props = append(props, ItemKey(func(i int) int { return rows[i].ID }))
				}
				return VirtualList(len(rows), func(int) float32 { return 30 },
					func(i int) tela.Node { return tela.WithKey(rows[i], rows[i].ID) }, props...)
			}
			app := telatest.MountFunc(t, page, telatest.Size(300, 200))
			app.Click(telatest.ByText("row11:0"))
			// Two items are inserted above: the first three rows on screen
			// are the new ones and row 11 moves down two places.
			set.Set([]keyedRow{{1}, {2}, {10}, {11}, {12}, {13}, {14}})
			app.ExpectText("row11")
			got := rowTexts(app)
			if keyed && !strings.Contains(got, "row11:1") {
				t.Errorf("with ItemKey the state stayed behind: %s", got)
			}
			if !keyed && strings.Contains(got, "row11:1") {
				t.Errorf("without ItemKey the state followed the item: %s", got)
			}
		})
	}
}

func TestVirtualListItemHeight(t *testing.T) {
	var list tela.ElementRef
	page := func() tela.Node {
		list = tela.UseElementRef()
		return VirtualList(10_000, nil,
			// Taller than the item height: it is not measured.
			func(i int) tela.Node { return Div(Class("h-[50px]"), Textf("Row %d", i)) },
			ItemHeight(30), Ref(list), Class("h-[150px] w-[200px]"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200), telatest.InstantScroll())
	list.ScrollToIndex(1000, tela.AlignStart)
	app.ExpectText("Row 1000")
	if _, y := list.ScrollOffset(); y != 30_000 {
		t.Errorf("offset %v after ScrollToIndex(1000): rows are 30 px each, want 30000", y)
	}
	list.ScrollToIndex(9_999, tela.AlignEnd)
	app.ExpectText("Row 9999")
	// The last row is 50 px tall in a 30 px slot: its 20 px of overflow
	// are scrollable content, as for any element.
	if _, y := list.ScrollOffset(); y != 10_000*30-150+20 {
		t.Errorf("offset %v at the end, want %v", y, 10_000*30-150+20)
	}
	if msg := panicText(func() { VirtualList(1, nil, func(int) tela.Node { return Div() }) }); !strings.Contains(msg, "ItemHeight(40)") {
		t.Errorf("VirtualList without sizes: %q", msg)
	}
	if msg := panicText(func() { ItemHeight(0) }); !strings.Contains(msg, "ItemHeight(0)") {
		t.Errorf("ItemHeight(0): %q", msg)
	}
	if msg := panicText(func() { Div(ItemHeight(10)) }); !strings.Contains(msg, "prop of VirtualList") {
		t.Errorf("ItemHeight on a Div: %q", msg)
	}
	if msg := panicText(func() { Div(ItemKey(func(int) int { return 0 })) }); !strings.Contains(msg, "prop of VirtualList") {
		t.Errorf("ItemKey on a Div: %q", msg)
	}
	if msg := panicText(func() { ItemKey[int](nil) }); !strings.Contains(msg, "ItemKey(nil)") {
		t.Errorf("ItemKey(nil): %q", msg)
	}
}

func TestVirtualListRoles(t *testing.T) {
	rows := func(props ...tela.Prop) tela.Node {
		return VirtualList(3, nil, func(i int) tela.Node { return Div(Textf("Row %d", i)) }, append(props, ItemHeight(20))...)
	}
	snap := func(n tela.Node) string {
		s, err := telatest.AriaSnapshot(n, telatest.Size(200, 100))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got, want := snap(rows(Class("h-[100px]"))), "- list:\n  - listitem: Row 0\n  - listitem: Row 1\n  - listitem: Row 2"; got != want {
		t.Errorf("default roles:\n%s\nwant:\n%s", got, want)
	}
	if got, want := snap(rows(Class("h-[100px]"), AriaLabel("Rows"))), "- list \"Rows\":\n  - listitem: Row 0\n  - listitem: Row 1\n  - listitem: Row 2"; got != want {
		t.Errorf("named list:\n%s\nwant:\n%s", got, want)
	}
	// Role replaces the list's; its items are no listitems then.
	if got, want := snap(rows(Class("h-[100px]"), Role("listbox"))), "- listbox: Row 0 Row 1 Row 2"; got != want {
		t.Errorf("Role(listbox):\n%s\nwant:\n%s", got, want)
	}
}
