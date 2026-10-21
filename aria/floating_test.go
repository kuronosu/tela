package aria_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.51 }

func expectRect(t *testing.T, what string, got tela.Rect, x, y, w, h float32) {
	t.Helper()
	if !near(got.X, x) || !near(got.Y, y) || !near(got.Width, w) || !near(got.Height, h) {
		t.Errorf("%s: got %+v, want x %g y %g width %g height %g", what, got, x, y, w, h)
	}
}

// TestFloatingFlipsInTheFirstFrame: a popover below a button near the
// bottom of the window is above it in the first frame that shows it, with
// no frame of its own; its Placement follows in the next one.
func TestFloatingFlipsInTheFirstFrame(t *testing.T) {
	page := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Placement("bottom"), aria.Offset(4), aria.Flip(), aria.Shift(8))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("anchor"), Class("absolute left-[100px] top-[250px] h-8 w-24"), Text("Menu")),
			Portal(Div(fl.Props, TestID("pop"), Class("h-24 w-48 bg-white"), Textf("placement %s", fl.Placement))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300), telatest.NoSettle())
	expectRect(t, "anchor", app.Bounds(telatest.ByTestID("anchor")), 100, 250, 96, 32)
	// Centered on the anchor, 4 px above it.
	expectRect(t, "popover in the first frame", app.Bounds(telatest.ByTestID("pop")), 52, 150, 192, 96)
	// Build-phase styling follows one frame later.
	app.ExpectText("placement top")
	expectRect(t, "popover once settled", app.Bounds(telatest.ByTestID("pop")), 52, 150, 192, 96)
}

// TestFloatingShiftsInsideTheWindow: shift keeps the popover padding pixels
// from the window's edges, on both sides.
func TestFloatingShiftsInsideTheWindow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		left  string
		wantX float32
	}{
		{"right edge", "left-[340px]", 400 - 8 - 192},
		{"left edge", "left-[5px]", 8},
		{"no shift", "left-[150px]", 150 + 20 - 96},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := func() tela.Node {
				anchor := tela.UseElementRef()
				fl := aria.UseFloating(anchor, aria.Offset(4), aria.Flip(), aria.Shift(8))
				return Div(Class("relative h-full"),
					Button(Ref(anchor), TestID("anchor"), Class("absolute top-10 h-8 w-10 "+tc.left), Text("M")),
					Portal(Div(fl.Props, TestID("pop"), Class("h-24 w-48 bg-white"))))
			}
			app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300), telatest.NoSettle())
			expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), tc.wantX, 40+32+4, 192, 96)
		})
	}
}

// TestFloatingFollowsScroll: scrolling an ancestor of the anchor moves the
// popover without building; the anchor scrolled out of view is reported by
// Hide.
func TestFloatingFollowsScroll(t *testing.T) {
	var hidden *tela.Value[bool]
	page := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Offset(2), aria.Flip(), aria.Hide())
		hidden = fl.ReferenceHidden
		return Div(
			Div(TestID("list"), Class("h-[200px] w-[300px] overflow-y-auto"),
				Div(Class("h-[100px]")),
				Button(Ref(anchor), TestID("anchor"), Class("block h-8 w-24"), Text("Open")),
				Div(Class("h-[600px]"))),
			Portal(Div(fl.Props, TestID("pop"), Class("h-10 w-10 bg-white"))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300))
	expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), 0, 100+32+2, 40, 40)
	if hidden.Latest() {
		t.Errorf("the anchor is in view but Hide reports it hidden")
	}
	before := app.Stats()
	app.Scroll(telatest.ByTestID("list"), 0, 40)
	expectRect(t, "popover after scrolling 40", app.Bounds(telatest.ByTestID("pop")), 0, 100+32+2-40, 40, 40)
	app.Scroll(telatest.ByTestID("list"), 0, 30)
	expectRect(t, "popover after scrolling 70", app.Bounds(telatest.ByTestID("pop")), 0, 100+32+2-70, 40, 40)
	after := app.Stats()
	if after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts {
		t.Errorf("scrolling built, restyled or laid out: %+v -> %+v", before, after)
	}
	if after.Paints == before.Paints {
		t.Errorf("scrolling painted no frame")
	}
	// Scrolled past the anchor: it is out of the list's view.
	app.Scroll(telatest.ByTestID("list"), 0, 80)
	if !hidden.Latest() {
		t.Errorf("the anchor is scrolled out of view but Hide does not report it")
	}
	app.Scroll(telatest.ByTestID("list"), 0, -200)
	if hidden.Latest() {
		t.Errorf("the anchor is back in view but Hide reports it hidden")
	}
}

// TestFloatingSizeLimitsTheHeight: Size limits the popover to the room
// below the anchor in the frame that shows it, and its content scrolls.
func TestFloatingSizeLimitsTheHeight(t *testing.T) {
	page := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Offset(4),
			aria.Size(func(a aria.Available) {
				a.MaxHeight(a.Height)
				a.MaxWidth(a.Width)
			}, aria.SizeOptions{Padding: 8}))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("anchor"), Class("absolute left-4 top-[50px] h-8 w-24"), Text("Open")),
			Portal(Div(fl.Props, TestID("pop"), Class("flex w-40 flex-col overflow-y-auto bg-white"),
				Range(40, func(i int) tela.Node {
					return Div(TestID(fmt.Sprintf("row-%d", i)), Class("h-8 shrink-0"), Textf("Row %d", i))
				}))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300), telatest.NoSettle())
	// 300 - 8 padding - (50 + 32 + 4) = 206 high, in the first frame.
	expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), 16, 86, 160, 206)
	r0 := app.Bounds(telatest.ByTestID("row-0"))
	app.Scroll(telatest.ByTestID("pop"), 0, 64)
	r2 := app.Bounds(telatest.ByTestID("row-2"))
	if !near(r2.Y, r0.Y) {
		t.Errorf("after scrolling the content by 64, row 2 is at y %g, want the place row 0 had (%g)", r2.Y, r0.Y)
	}
	expectRect(t, "popover after scrolling", app.Bounds(telatest.ByTestID("pop")), 16, 86, 160, 206)
}

// TestFloatingArrowPointsAtTheAnchor: the arrow's center is on the anchor's
// center wherever the popover shifts or flips to, in the first frame.
func TestFloatingArrowPointsAtTheAnchor(t *testing.T) {
	for _, tc := range []struct {
		name         string
		left, top    string
		wantX, wantY float32 // popover, with the arrow's data below
		arrowLeft    float32 // arrow's x in the window
		arrowTop     float32
	}{
		// The anchor is 40 wide at x 340: its center is 360.
		{"shifted below", "left-[340px]", "top-10", 200, 76, 360 - 8, 76 - 8},
		// Near the bottom the popover is above, with the arrow on its bottom edge.
		{"flipped above", "left-[100px]", "top-[250px]", 100 + 20 - 96, 250 - 4 - 96, 120 - 8, 250 - 4 - 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := func() tela.Node {
				anchor, arrow := tela.UseElementRef(), tela.UseElementRef()
				fl := aria.UseFloating(anchor, aria.Offset(4), aria.Flip(), aria.Shift(8), aria.Arrow(arrow))
				return Div(Class("relative h-full"),
					Button(Ref(anchor), TestID("anchor"), Class("absolute h-8 w-10 "+tc.left+" "+tc.top), Text("M")),
					Portal(Div(fl.Props, TestID("pop"), Class("h-24 w-48 bg-white"),
						Div(Ref(arrow), TestID("arrow"), fl.ArrowProps, Class("size-4 bg-black")))))
			}
			app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300), telatest.NoSettle())
			expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), tc.wantX, tc.wantY, 192, 96)
			anchor := app.Bounds(telatest.ByTestID("anchor"))
			arrowRect := app.Bounds(telatest.ByTestID("arrow"))
			expectRect(t, "arrow", arrowRect, tc.arrowLeft, tc.arrowTop, 16, 16)
			if c, w := arrowRect.X+arrowRect.Width/2, anchor.X+anchor.Width/2; !near(c, w) {
				t.Errorf("the arrow's center is at x %g, the anchor's at %g", c, w)
			}
		})
	}
}

// TestFloatingArrowOnTheSide: left and right placements put the arrow on
// the vertical edges and move it along y.
func TestFloatingArrowOnTheSide(t *testing.T) {
	page := func() tela.Node {
		anchor, arrow := tela.UseElementRef(), tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Placement("right"), aria.Offset(6), aria.Flip(), aria.Shift(8), aria.Arrow(arrow))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("anchor"), Class("absolute left-[20px] top-[10px] h-8 w-10"), Text("M")),
			Portal(Div(fl.Props, TestID("pop"), Class("h-24 w-48 bg-white"),
				Div(Ref(arrow), TestID("arrow"), fl.ArrowProps, Class("size-4 bg-black")))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300), telatest.NoSettle())
	// Right of the anchor (x 20 to 60) by 6, shifted down from y -30 to the
	// padding; the anchor's center (y 26) is 18 below the popover's top.
	expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), 66, 8, 192, 96)
	expectRect(t, "arrow", app.Bounds(telatest.ByTestID("arrow")), 66-8, 26-8, 16, 16)
}

// TestFloatingRTL: "-start" aligns the popover with the anchor's start
// edge, which is its right edge in a right-to-left element.
func TestFloatingRTL(t *testing.T) {
	for _, tc := range []struct {
		dir   string
		wantX float32
	}{
		{"ltr", 250},
		{"rtl", 250 + 40 - 192},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			page := func() tela.Node {
				anchor := tela.UseElementRef()
				fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Flip())
				return Div(Class("relative h-full"),
					Button(Ref(anchor), TestID("anchor"), Class("absolute left-[250px] top-10 h-8 w-10"), Text("M")),
					Portal(Div(fl.Props, Dir(tc.dir), TestID("pop"), Class("h-24 w-48 bg-white"))))
			}
			app := telatest.Mount(t, tela.C(page), telatest.Size(600, 300), telatest.NoSettle())
			expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), tc.wantX, 72, 192, 96)
		})
	}
}

// TestFloatingVirtualAnchor: a popover anchored to a rectangle (the pointer
// of a context menu) follows it without building.
func TestFloatingVirtualAnchor(t *testing.T) {
	var at *tela.Value[tela.Rect]
	page := func() tela.Node {
		at = tela.UseValue(tela.Rect{X: 100, Y: 120})
		fl := aria.UseFloating(tela.ElementRef{}, aria.Placement("bottom-start"), aria.Flip(), aria.Shift(4),
			aria.AnchorRect(at.Get))
		return Div(Class("h-full"),
			Portal(Div(fl.Props, TestID("pop"), Class("h-24 w-48 bg-white"), Textf("placement %s", fl.Placement))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300))
	expectRect(t, "popover at the pointer", app.Bounds(telatest.ByTestID("pop")), 100, 120, 192, 96)
	before := app.Stats()
	at.Set(tela.Rect{X: 150, Y: 60})
	app.WaitFor("the popover to follow the rectangle", func() bool {
		return near(app.Bounds(telatest.ByTestID("pop")).X, 150)
	})
	expectRect(t, "popover at the new rectangle", app.Bounds(telatest.ByTestID("pop")), 150, 60, 192, 96)
	if after := app.Stats(); after.Builds != before.Builds {
		t.Errorf("moving the virtual anchor built %d times", after.Builds-before.Builds)
	}
	// Near the corner no "bottom" placement fits and neither does "top-start"
	// (Floating UI's order: bottom-end, top-start, top-end): top-end does.
	at.Set(tela.Rect{X: 380, Y: 290})
	app.WaitFor("the popover to flip", func() bool {
		return near(app.Bounds(telatest.ByTestID("pop")).Y, 290-96)
	})
	expectRect(t, "popover in the corner", app.Bounds(telatest.ByTestID("pop")), 380-192, 290-96, 192, 96)
}

// TestFloatingFollowsAnchorMoves: the popover follows an anchor that a
// layout change moves, in the same frame.
func TestFloatingFollowsAnchorMoves(t *testing.T) {
	var setTop tela.Setter[int]
	page := func() tela.Node {
		top, set := tela.UseState(20)
		setTop = set
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Offset(4))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("anchor"), Style(tela.Top(float32(top))), Class("absolute left-10 h-8 w-10"), Text("M")),
			Portal(Div(fl.Props, TestID("pop"), Class("h-10 w-20 bg-white"))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300))
	expectRect(t, "popover", app.Bounds(telatest.ByTestID("pop")), 20, 56, 80, 40)
	setTop.Set(100)
	app.WaitFor("the popover to follow the anchor", func() bool { return near(app.Bounds(telatest.ByTestID("pop")).Y, 136) })
}

// TestFloatingMistakes: the errors that teach.
func TestFloatingMistakes(t *testing.T) {
	panicOf := func(f func()) (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		f()
		return "no panic"
	}
	for _, tc := range []struct {
		name string
		f    func()
		want []string
	}{
		{"placement", func() { aria.Placement("bottm-start") }, []string{`unknown placement "bottm-start"`, `Did you mean "bottom-start"?`, "top-end"}},
		{"placement case", func() { aria.Placement("Top") }, []string{`Did you mean "top"?`}},
		{"flip strategy", func() { aria.Flip(aria.FlipOptions{FallbackStrategy: "best"}) }, []string{"FallbackStrategy", "bestFit", "initialPlacement"}},
		{"flip fallback", func() { aria.Flip(aria.FlipOptions{FallbackPlacements: []string{"up"}}) }, []string{`unknown placement "up"`}},
		{"hide", func() { aria.Hide(aria.HideOptions{Strategy: "x"}) }, []string{"referenceHidden", "escaped"}},
		{"size nil", func() { aria.Size(nil) }, []string{"MaxHeight"}},
		{"arrow", func() { aria.Arrow(tela.ElementRef{}) }, []string{"tela.UseElementRef", "h.Ref"}},
		{"anchor rect", func() { aria.AnchorRect(nil) }, []string{"window coordinates"}},
		{"two options", func() { aria.Flip(aria.FlipOptions{}, aria.FlipOptions{}) }, []string{"at most one"}},
	} {
		got := panicOf(tc.f)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: panic %q does not say %q", tc.name, got, w)
			}
		}
	}
}

// TestFloatingWithoutAnchor: no ref and no AnchorRect says what to pass.
func TestFloatingWithoutAnchor(t *testing.T) {
	var msg string
	func() {
		defer func() { msg = fmt.Sprint(recover()) }()
		aria.UseFloating(tela.ElementRef{})
	}()
	for _, w := range []string{"no anchor", "h.Ref", "aria.AnchorRect"} {
		if !strings.Contains(msg, w) {
			t.Errorf("panic %q does not mention %q", msg, w)
		}
	}
}
