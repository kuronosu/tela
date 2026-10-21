package telatest_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Counter is the example of ARCHITECTURE section 1 in a smaller form.
func Counter() tela.Node {
	count, setCount := tela.UseState(0)
	return Div(Class("flex flex-col items-start gap-4 p-6"),
		H1(Class("text-3xl font-bold"), Textf("Count: %d", count)),
		Button(Class("px-4 py-2 rounded-lg bg-indigo-500 text-white hover:bg-indigo-600 active:scale-95"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Text("Increment")),
	)
}

func TestMountAndClick(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
	app.ExpectText("Count: 0")
	app.ExpectRole("heading", "Count: 0")
	app.Click(telatest.ByRole("button", "Increment"))
	app.ExpectText("Count: 1")
	app.Click(telatest.ByText("Increment"))
	app.Click(telatest.ByText("increment")) // case-insensitive, like getByText
	app.ExpectText("Count: 3")
	app.ExpectNoText("Count: 0")
	app.ExpectAria(`
		- 'heading "Count: 3" [level=1]'
		- button "Increment"
	`)
	if got, want := app.AriaSnapshot(), "- 'heading \"Count: 3\" [level=1]'\n- button \"Increment\""; got != want {
		t.Errorf("AriaSnapshot() = %q, want %q", got, want)
	}
}

func TestMountOptions(t *testing.T) {
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("size-10 bg-white dark:bg-black"))
	}), telatest.Size(100, 50), telatest.Scale(2), telatest.Dark())
	img := app.Screenshot()
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Fatalf("screenshot is %v, want 200x100 (100x50 at scale 2)", b)
	}
	if p := img.RGBAAt(5, 5); p.R > 5 || p.G > 5 || p.B > 5 {
		t.Errorf("pixel (5,5) is %v: dark: should have made the box black", p)
	}
	// Screenshot returns a copy.
	img.Pix[0] = 99
	if app.Screenshot().Pix[0] == 99 {
		t.Error("Screenshot returned the window's own image")
	}
}

func TestOptionErrors(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(Counter), telatest.Size(0, 100))
	}, "telatest.Size(0, 100)", "positive width and height")
	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(Counter), telatest.Scale(0))
	}, "telatest.Scale(0)")
	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(Counter), telatest.Timeout(0))
	}, "telatest.Timeout")
}

// ---------------------------------------------------------------------
// Locators.

func Page() tela.Node {
	return Div(Class("p-4 flex flex-col gap-2"),
		H1(Text("Settings")),
		P(Text("Pick the "), Strong(Text("theme")), Text(" you like")),
		Button(Text("Save")),
		Button(Text("Save as")),
		Button(Text("Reset")),
		Div(Class("hidden"), Button(Text("Hidden button"))),
		Div(AriaHidden(true), Button(Text("Aria hidden button"))),
		Div(Class("rounded-full size-4 bg-red-500")),
		Div(Class("rounded-full size-4 bg-blue-500")),
		A(Href("https://example.com"), Text("Docs")),
		Button(Disabled(true), Text("Locked")),
	)
}

func TestLocatorsByRole(t *testing.T) {
	app := telatest.Mount(t, tela.C(Page), telatest.Size(400, 500))
	if n := app.Count(telatest.ByRole("button", "")); n != 4 {
		t.Errorf("%d buttons, want 4 (Save, Save as, Reset, Locked; hidden ones excluded)", n)
	}
	if n := app.Count(telatest.ByRole("button", "Save")); n != 1 {
		t.Errorf("%d buttons named Save, want 1 (the name is exact)", n)
	}
	if n := app.Count(telatest.ByRole("generic", "")); n != 0 {
		t.Errorf("%d generic elements: implicit generics are not matched by role", n)
	}
	if n := app.Count(telatest.ByRole("heading", "Settings")); n != 1 {
		t.Errorf("%d headings", n)
	}
	if n := app.Count(telatest.ByRole("link", "Docs")); n != 1 {
		t.Errorf("%d links", n)
	}
	if n := app.Count(telatest.ByRole("button", "Hidden button")); n != 0 {
		t.Error("a display: none button was matched")
	}
	if n := app.Count(telatest.ByRole("button", "Aria hidden button")); n != 0 {
		t.Error("an AriaHidden button was matched")
	}
	if n := app.Count(telatest.ByRole("button", "").Nth(-1)); n != 1 {
		t.Errorf("Nth(-1) matches %d elements, want 1", n)
	}
	if n := app.Count(telatest.ByRole("button", "").Nth(9)); n != 0 {
		t.Errorf("Nth(9) matches %d elements, want 0", n)
	}
	app.ExpectCount(telatest.ByRole("button", "").Filter(telatest.HasText("save")), 2)
}

func TestLocatorsByText(t *testing.T) {
	app := telatest.Mount(t, tela.C(Page), telatest.Size(400, 500))
	// The deepest elements that contain the text.
	for _, tc := range []struct {
		loc  telatest.Locator
		want int
	}{
		{telatest.ByText("theme"), 1},                               // the Strong, not its paragraph
		{telatest.ByText("Pick the theme you like"), 1},             // spans inline children: the paragraph
		{telatest.ByText("save"), 2},                                // Save and Save as
		{telatest.ByText("save", telatest.Exact()), 0},              // exact is case sensitive
		{telatest.ByText("Save", telatest.Exact()), 1},              // and whole
		{telatest.ByText("Hidden button"), 0},                       // hidden text is not text
		{telatest.ByText("Docs"), 1},                                // the link
		{telatest.ByText("Save").Nth(1), 1},                         // Save as
		{telatest.ByText("save").Filter(telatest.HasText("as")), 1}, // narrowed
	} {
		if got := app.Count(tc.loc); got != tc.want {
			t.Errorf("%s matches %d elements, want %d", tc.loc, got, tc.want)
		}
	}
	// The strong element holds "theme": its bounds are inside the paragraph's.
	pxBox := app.Bounds(telatest.ByText("Pick the theme you like"))
	px, py, pw, ph := pxBox.X, pxBox.Y, pxBox.Width, pxBox.Height
	sxBox := app.Bounds(telatest.ByText("theme"))
	sx, sy, sw, sh := sxBox.X, sxBox.Y, sxBox.Width, sxBox.Height
	if sx < px || sy < py || sx+sw > px+pw+0.5 || sy+sh > py+ph+0.5 {
		t.Errorf("theme at (%v,%v %vx%v) is not inside its paragraph (%v,%v %vx%v)", sx, sy, sw, sh, px, py, pw, ph)
	}
}

func TestLocatorsByClass(t *testing.T) {
	app := telatest.Mount(t, tela.C(Page), telatest.Size(400, 500))
	app.ExpectCount(telatest.ByClass("rounded-full", "size-4"), 2)
	app.ExpectCount(telatest.ByClass("rounded-full bg-red-500"), 1)
	xBox := app.Bounds(telatest.ByClass("bg-blue-500"))
	x, y, w, h := xBox.X, xBox.Y, xBox.Width, xBox.Height
	if w != 16 || h != 16 || x != 16 || y <= 0 {
		t.Errorf("bounds of the blue dot = (%v, %v, %v, %v), want a 16x16 box at x=16", x, y, w, h)
	}
}

func TestLocatorString(t *testing.T) {
	for _, tc := range []struct {
		loc  telatest.Locator
		want string
	}{
		{telatest.ByRole("button", "Save"), `getByRole("button", name="Save")`},
		{telatest.ByRole("heading", ""), `getByRole("heading")`},
		{telatest.ByText("x", telatest.Exact()), `getByText("x", exact)`},
		{telatest.ByClass("a", "b"), `getByClass("a b")`},
		{telatest.ByText("x").Filter(telatest.HasText("y")).Nth(2), `getByText("x").filter(hasText="y").nth(2)`},
	} {
		if got := tc.loc.String(); got != tc.want {
			t.Errorf("String() = %s, want %s", got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------
// Failure messages.

func TestStrictness(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Page), telatest.Size(400, 500), telatest.Timeout(200*time.Millisecond))
		app.Click(telatest.ByText("Save"))
	}, "strict mode violation", `getByText("Save") matches 2 elements`, `1) button "Save"`, `2) button "Save as"`,
		"Nth(i)", "aria snapshot:", `- button "Save as"`)
}

func TestNoMatchSuggestions(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByRole("button", "Incremet"))
	}, `no element matches getByRole("button", name="Incremet")`, `did you mean button "Increment"?`, "aria snapshot:", `- button "Increment"`)

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByRole("heading", "Increment"))
	}, `headings on screen: "Count: 0"`, `an element named "Increment" has the role button instead`)

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByRole("buton", "Increment"))
	}, `ByRole: unknown role "buton"`, `did you mean "button"?`)

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByText("Incremnt"))
	}, `no element matches getByText("Incremnt")`, `did you mean "Increment"?`)

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByRole("button", "Increment").Filter(telatest.HasText("nothing")))
	}, `alone matches 1 element(s); the filter or Nth removed them`)

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300))
		app.Click(telatest.ByText(""))
	}, "ByText with an empty text")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300))
		app.Click(telatest.Locator{})
	}, "the zero Locator matches nothing")
}

func TestExpectFailures(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.ExpectText("Count: 2")
	}, `expected the window to show the text "Count: 2"`, `did you mean "Count: 0"?`, "aria snapshot:")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.ExpectNoText("Count: 0")
	}, `expected the window not to show the text "Count: 0"`)
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.ExpectRole("button", "Reset")
	}, `no element matches getByRole("button", name="Reset")`)
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.ExpectCount(telatest.ByRole("button", ""), 2)
	}, "expected getByRole(\"button\") to match 2 element(s), it matches 1")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.ExpectAria(`
			- heading "Count: 0" [level=1]
			- button "Decrement"
		`)
	}, "the aria snapshot differs", `first difference at line 1`, "expected:", "actual:")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
		app.ExpectFocused(telatest.ByRole("button", "Increment"))
	}, "to be focused", "but nothing is")
}

// ---------------------------------------------------------------------
// Pointer actions.

type recorder struct{ events []string }

func (r *recorder) add(format string, args ...any) {
	r.events = append(r.events, fmt.Sprintf(format, args...))
}
func (r *recorder) take() string {
	s := strings.Join(r.events, ",")
	r.events = nil
	return s
}

func TestClickIsRealPointerInput(t *testing.T) {
	var rec recorder
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("p-4"),
			Button(Class("px-4 py-2 bg-slate-200 hover:bg-slate-300 active:bg-slate-400 cursor-pointer"),
				OnPointerEnter(func(tela.PointerEvent) { rec.add("enter") }),
				OnPointerDown(func(tela.PointerEvent) { rec.add("down") }),
				OnPointerUp(func(tela.PointerEvent) { rec.add("up") }),
				OnClick(func(e tela.MouseEvent) { rec.add("click%d", e.ClickCount) }),
				Text("Go")))
	}), telatest.Size(300, 200))
	btn := telatest.ByRole("button", "Go")
	app.Click(btn)
	if got := rec.take(); got != "enter,down,up,click1" {
		t.Errorf("a click sent %q, want enter,down,up,click1", got)
	}
	app.ExpectFocused(btn) // the press focused it, like a user
	if got := app.Cursor(); got != "pointer" {
		t.Errorf("cursor = %q, want pointer", got)
	}
	app.DoubleClick(btn)
	if got := rec.take(); got != "down,up,click1,down,up,click2" {
		t.Errorf("a double click sent %q", got)
	}
	// Hover changes the picture through hover:bg-slate-300; away restores it.
	xBox := app.Bounds(btn)
	x, y, w, h := xBox.X, xBox.Y, xBox.Width, xBox.Height
	probe := func() uint8 { return app.Screenshot().RGBAAt(int(x+w/2), int(y+h-2)).R }
	app.MouseMove(5, 150)
	rest := probe()
	app.Hover(btn)
	if hover := probe(); hover >= rest {
		t.Errorf("hover:bg-slate-300 did not darken the button: %d -> %d", rest, hover)
	}
	app.MouseMove(5, 150)
	if probe() != rest {
		t.Error("the button did not return to its resting look")
	}
	// A held press shows active:.
	app.Advance(time.Second) // a new click sequence for the manual press
	app.Hover(btn)
	hover := probe()
	app.MouseDown()
	if active := probe(); active >= hover {
		t.Errorf("active:bg-slate-400 did not darken the pressed button: %d -> %d", hover, active)
	}
	app.MouseUp()
	if got := rec.take(); !strings.HasSuffix(got, "down,up,click1") {
		t.Errorf("manual press sent %q", got)
	}
}

func TestClickWaitsForEnabledAndFailsOnDisabled(t *testing.T) {
	// A button enabled by an effect that runs on a later frame.
	var clicks atomic.Int32
	app := telatest.Mount(t, tela.C(func() tela.Node {
		ready, setReady := tela.UseState(false)
		tela.UseEffect(func() func() {
			go func() {
				time.Sleep(30 * time.Millisecond)
				setReady.Set(true)
			}()
			return nil
		}, tela.Once)
		return Button(Disabled(!ready), OnClick(func(tela.MouseEvent) { clicks.Add(1) }), Text("Go"))
	}), telatest.Size(200, 100))
	app.Click(telatest.ByRole("button", "Go")) // waits until it is enabled
	if clicks.Load() != 1 {
		t.Errorf("%d clicks, want 1", clicks.Load())
	}

	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(func() tela.Node {
			return Button(Disabled(true), Text("Locked"))
		}), telatest.Size(200, 100), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByRole("button", "Locked"))
	}, `cannot click getByRole("button", name="Locked")`, "is disabled")
}

func TestClickFailsWhenCovered(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(func() tela.Node {
			return Div(Class("relative"),
				Button(Class("px-4 py-2"), Text("Under")),
				Div(Class("absolute inset-0 z-10 bg-black/20"), Text("Overlay")))
		}), telatest.Size(200, 100), telatest.Timeout(100*time.Millisecond))
		app.Click(telatest.ByRole("button", "Under"))
	}, `cannot click getByRole("button", name="Under")`, "receives the pointer at its center", "pointer-events-none")
}

func TestClickScrollsIntoView(t *testing.T) {
	var clicked atomic.Bool
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("h-[200px] overflow-y-auto"),
			Div(Class("h-[600px]")),
			Button(OnClick(func(tela.MouseEvent) { clicked.Store(true) }), Text("Far away")))
	}), telatest.Size(200, 200), telatest.InstantScroll())
	y := app.Bounds(telatest.ByRole("button", "Far away")).Y
	if y < 200 {
		t.Fatalf("the button is at y=%v: not below the fold", y)
	}
	app.Click(telatest.ByRole("button", "Far away"))
	if !clicked.Load() {
		t.Error("the click did not reach the button below the fold")
	}
}

func TestDragAndDragBy(t *testing.T) {
	var rec recorder
	var last atomic.Value
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("p-4 flex gap-8"),
			Div(Class("size-12 bg-indigo-500"), Role("button"), AriaLabel("Handle"),
				OnDragStart(func(e tela.DragEvent) { rec.add("start") }),
				OnDrag(func(e tela.DragEvent) { last.Store(e) }),
				OnDragEnd(func(e tela.DragEvent) { rec.add("end") })),
			Div(Class("size-12 bg-slate-300"), Role("region"), AriaLabel("Target")))
	}), telatest.Size(300, 200))
	app.DragBy(telatest.ByRole("button", "Handle"), 80, 40)
	if got := rec.take(); got != "start,end" {
		t.Errorf("drag sent %q", got)
	}
	e := last.Load().(tela.DragEvent)
	if e.OffsetX != 80 || e.OffsetY != 40 {
		t.Errorf("the last drag offset is (%v, %v), want (80, 40)", e.OffsetX, e.OffsetY)
	}
	hxBox := app.Bounds(telatest.ByRole("button", "Handle"))
	hx, hy, hw, hh := hxBox.X, hxBox.Y, hxBox.Width, hxBox.Height
	txBox := app.Bounds(telatest.ByRole("region", "Target"))
	tx, ty, tw, th := txBox.X, txBox.Y, txBox.Width, txBox.Height
	app.Drag(telatest.ByRole("button", "Handle"), telatest.ByRole("region", "Target"), telatest.Steps(4))
	e = last.Load().(tela.DragEvent)
	if wantX, wantY := (tx+tw/2)-(hx+hw/2), (ty+th/2)-(hy+hh/2); e.OffsetX != wantX || e.OffsetY != wantY {
		t.Errorf("Drag to the target ended with offset (%v, %v), want (%v, %v)", e.OffsetX, e.OffsetY, wantX, wantY)
	}
	if got := rec.take(); got != "start,end" {
		t.Errorf("the second drag sent %q", got)
	}
}

func TestLongPressWithDelay(t *testing.T) {
	var rec recorder
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Button(Class("px-4 py-2"),
			OnLongPress(func(tela.PointerEvent) { rec.add("long") }),
			OnClick(func(tela.MouseEvent) { rec.add("click") }),
			Text("Hold"))
	}), telatest.Size(200, 100))
	app.Click(telatest.ByRole("button", "Hold"))
	if got := rec.take(); got != "click" {
		t.Errorf("a quick click sent %q, want click", got)
	}
	app.Click(telatest.ByRole("button", "Hold"), telatest.Delay(600*time.Millisecond))
	if got := rec.take(); got != "long" {
		t.Errorf("a press held for 600 ms sent %q, want long (and no click)", got)
	}
	// The same through Advance with a manual press.
	app.MouseDown()
	if got := rec.take(); got != "" {
		t.Errorf("the press alone sent %q", got)
	}
	app.Advance(400 * time.Millisecond)
	if got := rec.take(); got != "" {
		t.Errorf("after 400 ms the long press fired early: %q", got)
	}
	app.Advance(200 * time.Millisecond)
	if got := rec.take(); got != "long" {
		t.Errorf("after 600 ms the long press sent %q", got)
	}
	app.MouseUp()
}

func TestScrollAndAutoWaitForAnimations(t *testing.T) {
	var scrolled atomic.Int32
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("h-[200px] w-[300px] overflow-y-auto"),
			OnScroll(func(tela.ScrollEvent) { scrolled.Add(1) }),
			Map([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}, func(i int) tela.Node {
				return Div(Key(i), Class("h-[50px]"), Textf("Row %d", i))
			}))
	}), telatest.Size(300, 200))
	row := telatest.ByText("Row 10", telatest.Exact())
	y0 := app.Bounds(row).Y
	if y0 < 200 {
		t.Fatalf("Row 10 starts at %v: not below the fold", y0)
	}
	// A wheel at the list scrolls by the delta, at once.
	app.Scroll(telatest.ByText("Row 1", telatest.Exact()), 0, 120)
	y1 := app.Bounds(row).Y
	if y1 != y0-120 {
		t.Errorf("after Scroll(0, 120) Row 10 is at %v, want %v", y1, y0-120)
	}
	if scrolled.Load() == 0 {
		t.Error("OnScroll did not run")
	}
	// A page-down key animates for 150 ms: the action settles the app, so the
	// animation has finished when it returns.
	app.Press("PageDown")
	y2 := app.Bounds(row).Y
	if y2 >= y1 {
		t.Errorf("PageDown did not scroll: %v -> %v", y1, y2)
	}
	now := app.Now()
	app.Press("End")
	if app.Now() == now {
		t.Error("an animated scroll did not move the clock")
	}
	y3 := app.Bounds(telatest.ByText("Row 19", telatest.Exact())).Y
	if y3+50 > 200.5 || y3+50 < 199.5 {
		t.Errorf("after End the last row ends at %v, want the bottom of the 200 px list", y3+50)
	}
}

// ---------------------------------------------------------------------
// Keyboard.

func TestPressAndType(t *testing.T) {
	var rec recorder
	key := func(e tela.KeyboardEvent, dir string) {
		var m []string
		for _, mod := range []struct {
			name string
			on   bool
		}{{"shift", e.Shift}, {"ctrl", e.Ctrl}, {"alt", e.Alt}, {"meta", e.Meta}} {
			if mod.on {
				m = append(m, mod.name)
			}
		}
		s := fmt.Sprintf("%s %s/%s", dir, e.Key, e.Code)
		if e.Location != 0 {
			s += "/" + e.Location.String()
		}
		if len(m) > 0 {
			s += " " + strings.Join(m, "+")
		}
		rec.add("%s", s)
	}
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Div(
			OnKeyDown(func(e tela.KeyboardEvent) { key(e, "v") }),
			OnKeyUp(func(e tela.KeyboardEvent) { key(e, "^") }),
			Button(Text("One")), Button(Text("Two")))
	}), telatest.Size(300, 200))

	app.Press("a")
	if got := rec.take(); got != "v a/KeyA,^ a/KeyA" {
		t.Errorf("Press(a) sent %q", got)
	}
	app.Press("Shift+a")
	if got := rec.take(); got != "v Shift/ShiftLeft/left shift,v A/KeyA shift,^ A/KeyA shift,^ Shift/ShiftLeft/left" {
		t.Errorf("Press(Shift+a) sent %q", got)
	}
	app.Press("Control+s")
	if got := rec.take(); got != "v Control/ControlLeft/left ctrl,v s/KeyS ctrl,^ s/KeyS ctrl,^ Control/ControlLeft/left" {
		t.Errorf("Press(Control+s) sent %q", got)
	}
	app.Press("Meta+Shift+!")
	if got := rec.take(); !strings.Contains(got, "v !/Digit1 shift+meta") {
		t.Errorf("Press(Meta+Shift+!) sent %q", got)
	}
	app.Press("Enter")
	if got := rec.take(); got != "v Enter/Enter,^ Enter/Enter" {
		t.Errorf("Press(Enter) sent %q", got)
	}
	app.Press("ControlOrMeta+Space")
	if got := rec.take(); !strings.Contains(got, "v  /Space") {
		t.Errorf("Press(ControlOrMeta+Space) sent %q", got)
	}
	app.Press("Control++")
	if got := rec.take(); !strings.Contains(got, "v +/Equal") {
		t.Errorf("Press(Control++) sent %q", got)
	}
	app.Press("Shift")
	if got := rec.take(); got != "v Shift/ShiftLeft/left shift,^ Shift/ShiftLeft/left" {
		t.Errorf("Press(Shift) sent %q", got)
	}
	app.Type("aB\n")
	if got := rec.take(); got != "v a/KeyA,^ a/KeyA,v B/KeyB,^ B/KeyB,v Enter/Enter,^ Enter/Enter" {
		t.Errorf("Type sent %q", got)
	}
	app.Type("é")
	if got := rec.take(); got != "v é/Unidentified,^ é/Unidentified" {
		t.Errorf("Type(é) sent %q", got)
	}

	// Tab moves the focus through the buttons, Shift+Tab back; Enter clicks.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "One"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Two"))
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByRole("button", "One"))
	app.Focus(telatest.ByRole("button", "Two"))
	app.ExpectFocused(telatest.ByRole("button", "Two"))
}

func TestEnterActivatesFocusedButton(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
	app.Focus(telatest.ByRole("button", "Increment"))
	app.Press("Enter")
	app.ExpectText("Count: 1")
	app.Press("Space")
	app.ExpectText("Count: 2")
}

func TestPressErrors(t *testing.T) {
	for _, tc := range []struct {
		combo string
		want  []string
	}{
		{"Entr", []string{`unknown key "Entr"`, `did you mean "Enter"`}},
		{"Ctrl+s", []string{`"Ctrl" is not a modifier`, `"Control"`}},
		{"Esc", []string{`did you mean "Escape"`}},
		{"Control+", []string{"unknown key", `"Control+"`}}, // the + is the key
		{"", []string{"no key"}},
		{"a+b", []string{`"a" is not a modifier`}},
	} {
		mustFail(t, func(tb testing.TB) {
			app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300))
			app.Press(tc.combo)
		}, tc.want...)
	}
}

func TestFocusErrors(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(Counter), telatest.Size(400, 300))
		app.Focus(telatest.ByRole("heading", "Count: 0"))
	}, "is not focusable", "h.TabIndex")
}

// ---------------------------------------------------------------------
// Auto-wait.

func TestAutoWaitForEffectsAndGoroutines(t *testing.T) {
	// A chain of effects, each setting state on the next frame.
	app := telatest.Mount(t, tela.C(func() tela.Node {
		stage, setStage := tela.UseState(0)
		tela.UseEffect(func() func() {
			if stage < 3 {
				setStage.Set(stage + 1)
			}
			return nil
		}, stage)
		return Textf("stage %d", stage)
	}), telatest.Size(200, 100))
	app.ExpectText("stage 3")

	// State set by a goroutine after real time.
	app2 := telatest.Mount(t, tela.C(func() tela.Node {
		text, setText := tela.UseState("loading")
		tela.UseEffect(func() func() {
			go func() {
				time.Sleep(40 * time.Millisecond)
				setText.Set("loaded")
			}()
			return nil
		}, tela.Once)
		return Text(text)
	}), telatest.Size(200, 100))
	app2.ExpectText("loading")
	app2.ExpectText("loaded")
	app2.ExpectNoText("loading")

	// App time: a timeout of 3 s of the app's clock passes without waiting.
	app3 := telatest.Mount(t, tela.C(func() tela.Node {
		show, setShow := tela.UseState(true)
		tela.UseEffect(func() func() {
			// There are no timers in the API: tela.Post from a goroutine that
			// sleeps in real time is the closest.
			go func() {
				time.Sleep(20 * time.Millisecond)
				tela.Post(func() { setShow.Set(false) })
			}()
			return nil
		}, tela.Once)
		return Fragment(If(show, Text("toast")))
	}), telatest.Size(200, 100))
	app3.ExpectNoText("toast")
}

func TestAdvanceRunsFramesAtTheirTime(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
	before := app.Now()
	app.Advance(250 * time.Millisecond)
	if got := app.Now() - before; got != 250*time.Millisecond {
		t.Errorf("Advance(250ms) moved the clock by %v", got)
	}
	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(Counter)).Advance(-time.Second)
	}, "cannot go backwards")
}

func TestEndlessActivityDoesNotBlockActions(t *testing.T) {
	// A component that sets state on every build never lets the app go quiet.
	f := mustPass(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(func() tela.Node {
			n, setN := tela.UseState(0)
			tela.UseEffect(func() func() { setN.Set(n + 1); return nil })
			return Div(Textf("n=%d", n), Button(Text("Go")))
		}), telatest.Size(200, 100), telatest.Timeout(300*time.Millisecond))
		start := time.Now()
		app.Click(telatest.ByRole("button", "Go")) // does not wait for quiet
		app.ExpectRole("button", "Go")
		if d := time.Since(start); d > 2*time.Second {
			tb.Errorf("the actions took %v: they kept waiting for the app to go quiet", d)
		}
	})
	var warned int
	for _, l := range f.logs {
		if strings.Contains(l, "still requesting frames") {
			warned++
		}
	}
	if warned != 1 {
		t.Errorf("%d warnings about endless activity, want 1:\n%s", warned, strings.Join(f.logs, "\n"))
	}
}

// ---------------------------------------------------------------------
// Lifecycle, dev messages, stats.

func TestCloseRunsCleanupsAndForbidsUse(t *testing.T) {
	var cleaned atomic.Bool
	app := telatest.Mount(t, tela.C(func() tela.Node {
		tela.UseEffect(func() func() { return func() { cleaned.Store(true) } }, tela.Once)
		return Text("x")
	}), telatest.Size(100, 100))
	app.ExpectText("x")
	app.Close()
	if !cleaned.Load() {
		t.Error("Close did not run the effect cleanups")
	}
	app.Close() // idempotent

	mustFail(t, func(tb testing.TB) {
		a := telatest.Mount(tb, tela.C(Counter), telatest.Size(100, 100))
		a.Close()
		a.ExpectText("Count")
	}, "the app was closed")
}

func TestUnmountsWhenTheTestEnds(t *testing.T) {
	var cleaned atomic.Bool
	f := run(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(func() tela.Node {
			tela.UseEffect(func() func() { return func() { cleaned.Store(true) } }, tela.Once)
			return Text("x")
		}), telatest.Size(100, 100))
	})
	if f.failed {
		t.Fatal(f.failures())
	}
	if !cleaned.Load() {
		t.Error("the end of the test did not unmount the app")
	}
}

func TestDevMessages(t *testing.T) {
	t.Setenv("TELA_DEV", "")
	root := func() tela.Node { return Div(Class("rounded-xxl"), Text("x")) }
	// Dev mode is the default; Dev() is a no-op alias kept for old tests.
	for _, opts := range [][]telatest.Option{nil, {telatest.Dev()}, {telatest.NoDev(), telatest.Dev()}} {
		f := run(t, func(tb testing.TB) {
			telatest.Mount(tb, tela.C(root), append([]telatest.Option{telatest.Size(100, 100)}, opts...)...)
		})
		got := f.failures()
		if !f.failed || !strings.Contains(got, "dev message") || !strings.Contains(got, "rounded-2xl") {
			t.Errorf("%d options: an unknown class did not fail the test with a suggestion:\n%s", len(opts), got)
		}
	}
	f := run(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(root), telatest.Size(100, 100), telatest.AllowDev("rounded-xxl"))
	})
	if f.failed {
		t.Errorf("AllowDev did not accept the message:\n%s", f.failures())
	}
	f = run(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(root), telatest.Size(100, 100), telatest.NoDev())
	})
	if f.failed {
		t.Errorf("a message failed the test under NoDev():\n%s", f.failures())
	}
	// TELA_DEV=0 overrides the default, TELA_DEV=1 overrides NoDev.
	t.Setenv("TELA_DEV", "0")
	if f = run(t, func(tb testing.TB) { telatest.Mount(tb, tela.C(root), telatest.Size(100, 100)) }); f.failed {
		t.Errorf("TELA_DEV=0: a message failed the test:\n%s", f.failures())
	}
	t.Setenv("TELA_DEV", "1")
	if f = run(t, func(tb testing.TB) { telatest.Mount(tb, tela.C(root), telatest.Size(100, 100), telatest.NoDev()) }); !f.failed {
		t.Error("TELA_DEV=1 with NoDev: the unknown class did not fail the test")
	}
}

// TestMountComponentCall checks the message of a component called where a
// node is expected: its hooks run before any build.
func TestMountComponentCall(t *testing.T) {
	var msg string
	func() {
		defer func() { msg = fmt.Sprint(recover()) }()
		telatest.Mount(t, Counter())
	}()
	for _, want := range []string{
		"UseState called outside a component build, from telatest_test.Counter (telatest_test.go:",
		"wrap the root in tela.C(Counter)",
		"telatest.Mount(t, tela.C(Counter))",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic %q\ndoes not contain %q", msg, want)
		}
	}
}

func TestDevWarningsAreLogs(t *testing.T) {
	t.Setenv("TELA_DEV", "")
	items := []string{"a", "b", "c"}
	root := func() tela.Node {
		return Div(Map(items, func(s string) tela.Node { return Div(Text(s)) }))
	}
	f := mustPass(t, func(tb testing.TB) { telatest.Mount(tb, tela.C(root), telatest.Size(100, 100)) })
	logs := strings.Join(f.logs, "\n")
	if !strings.Contains(logs, "dev warning: tela: 3 children of a list (Map or List)") || !strings.Contains(logs, "telatest_test.go:") {
		t.Errorf("the keyless list was not logged as a warning with its call site:\n%s", logs)
	}
}

func TestStats(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
	before := app.Stats()
	app.Click(telatest.ByRole("button", "Increment"))
	after := app.Stats()
	if after.Builds <= before.Builds || after.Frames <= before.Frames {
		t.Errorf("a click built %d components in %d frames", after.Builds-before.Builds, after.Frames-before.Frames)
	}
}

func TestIDPrefixIsDeterministic(t *testing.T) {
	id := func() string {
		var got string
		app := telatest.Mount(t, tela.C(func() tela.Node {
			got = tela.UseID()
			return Text("x")
		}))
		app.ExpectText("x")
		return got
	}
	a, b := id(), id()
	if a != b || !strings.HasPrefix(a, "tt-") {
		t.Errorf("UseID gave %q then %q: want the same, with the prefix tt-", a, b)
	}
}
