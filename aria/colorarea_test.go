package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// The picker pages have an area of 200 x 200 px at (16, 16), a hue slider
// track of 200 x 12 at (16, 232) and an alpha slider track below it at
// (16, 260), with thumbs of 20 px centered on their value, and a hex
// field. Window: 232 x 400.
const (
	areaLen    = 200
	areaOrigin = 16
	hueY       = 238 // the center of the hue thumb
	alphaY     = 266
)

type picker struct {
	state  aria.ColorStateOptions
	area   aria.ColorAreaOptions
	hue    aria.ColorSliderOptions
	alpha  aria.ColorSliderOptions
	field  aria.ColorFieldOptions
	parts  string // which parts to render: "a" area, "h" hue, "l" alpha, "f" field
	builds *int
}

func (p picker) page() func() tela.Node {
	return func() tela.Node { return p.usePicker() }
}

func (p picker) usePicker() tela.Node {
	if p.builds != nil {
		*p.builds++
	}
	parts := p.parts
	if parts == "" {
		parts = "ahlf"
	}
	st := aria.UseColorState(p.state)
	p.area.State, p.hue.State, p.alpha.State, p.field.State = st, st, st, st
	if p.area.AriaLabel == "" {
		p.area.AriaLabel = "Color"
	}
	area := aria.UseColorArea(p.area)
	p.hue.Channel, p.alpha.Channel = aria.ColorHue, aria.ColorAlpha
	hue := aria.UseColorSlider(p.hue)
	alpha := aria.UseColorSlider(p.alpha)
	field := aria.UseColorField(p.field)
	areaSize := tela.UseSize(area.AreaRef)
	hueTrack := tela.UseSize(hue.TrackRef)
	alphaTrack := tela.UseSize(alpha.TrackRef)
	thumb := func(sl aria.ColorSliderResult, track *tela.Value[tela.Size]) tela.Node {
		return Div(sl.ThumbProps, Class("absolute -top-1 left-0 -ml-2.5 size-5 rounded-full bg-primary"),
			Style(tela.TranslateFn(func() (x, y float32) {
				return float32(sl.ThumbPercent()) / 100 * track.Get().Width, 0
			})))
	}
	has := func(part string, n tela.Node, spacer string) tela.Node {
		if strings.Contains(parts, part) {
			return n
		}
		return Div(Class(spacer)) // the layout stays the same without the part
	}
	return Div(Class("flex w-[232px] flex-col gap-4 p-4"),
		has("a", Div(area.AreaProps, Class("relative size-[200px] rounded-md bg-muted"),
			Div(area.ThumbProps, Class("absolute left-0 top-0 -ml-2.5 -mt-2.5 size-5 rounded-full border-2 border-white"),
				Style(tela.TranslateFn(func() (x, y float32) {
					px, py := area.ThumbPercent()
					sz := areaSize.Get()
					return float32(px) / 100 * sz.Width, float32(py) / 100 * sz.Height
				})))), "size-[200px]"),
		has("h", Div(hue.Props, Div(hue.TrackProps, Class("relative h-3 w-[200px] rounded-full bg-muted"), thumb(hue, hueTrack))), "h-3"),
		has("l", Div(alpha.Props, Div(alpha.TrackProps, Class("relative h-3 w-[200px] rounded-full bg-muted"), thumb(alpha, alphaTrack))), "h-3"),
		has("f", Input(field.InputProps, Class("w-28 rounded-md border px-2")), "h-6"),
		Span(Text("color "+aria.FormatHexColor(st.Color(), true))),
	)
}

func areaThumb() telatest.Locator { return telatest.ByRole("slider", "Color") }

// areaPoint is the window position of a saturation and a brightness.
func areaPoint(sat, bri float32) (x, y float32) {
	return areaOrigin + sat/100*areaLen, areaOrigin + (100-bri)/100*areaLen
}

func center(app *telatest.App, l telatest.Locator) (x, y float32) {
	b := app.Bounds(l)
	return b.X + b.Width/2, b.Y + b.Height/2
}

func TestColorAreaSnapshot(t *testing.T) {
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(tela.Hex("#336699"))}, parts: "a"}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.ExpectAria(`
		- group "Color":
		  - slider "Color"
		- text: "color #336699ff"
	`)
	app.Expect(areaThumb(), telatest.HasValue("Saturation 67%, Brightness 60%"))
}

func TestColorAreaPointerPlacesAndDrags(t *testing.T) {
	var changes, ends events
	p := picker{
		state: aria.ColorStateOptions{
			DefaultValue: new(aria.HSB{H: 0, S: 100, B: 100, A: 1}.Color()),
			OnChange:     func(c tela.Color) { changes.add(aria.FormatHexColor(c, false)) },
		},
		area:  aria.ColorAreaOptions{OnChangeEnd: func(c tela.Color) { ends.add(aria.FormatHexColor(c, false)) }},
		parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	app.Expect(th, telatest.HasValue("Saturation 100%, Brightness 100%"))
	// A press in the area places the thumb: 50% saturation, 25% brightness.
	x, y := areaPoint(50, 25)
	app.ClickAt(x, y)
	app.Expect(th, telatest.HasValue("Saturation 50%, Brightness 25%"))
	app.ExpectFocused(th)
	if cx, cy := center(app, th); cx < x-1 || cx > x+1 || cy < y-1 || cy > y+1 {
		t.Errorf("the thumb is centered at (%g, %g), want (%g, %g)", cx, cy, x, y)
	}
	if got := ends.take(); got != "#402020" {
		t.Errorf("OnChangeEnd: %s", got)
	}
	changes.take()
	// Dragging past every edge clamps to the corners.
	app.MouseMove(x, y)
	app.MouseDown()
	app.MouseMove(400, -50)
	app.Expect(th, telatest.HasValue("Saturation 100%, Brightness 100%"))
	app.MouseMove(-40, 500)
	app.Expect(th, telatest.HasValue("Saturation 0%, Brightness 0%"))
	app.MouseMove(-40, -40)
	app.Expect(th, telatest.HasValue("Saturation 0%, Brightness 100%"))
	app.MouseMove(900, 900)
	app.Expect(th, telatest.HasValue("Saturation 100%, Brightness 0%"))
	app.MouseUp()
	if got := ends.take(); got != "#000000" {
		t.Errorf("OnChangeEnd after the drag: %s", got)
	}
	if got := changes.take(); !strings.HasPrefix(got, "#ff0000 ") && !strings.Contains(got, "#ff0000") {
		t.Errorf("OnChange: %s", got)
	}
	// Moves after the release do nothing.
	app.MouseMove(areaOrigin+50, areaOrigin+50)
	app.Expect(th, telatest.HasValue("Saturation 100%, Brightness 0%"))
}

// The thumb dragged by its edge does not jump.
func TestColorAreaThumbKeepsTheGrab(t *testing.T) {
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 200, S: 50, B: 50, A: 1}.Color())}, parts: "a"}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	cx, cy := center(app, th)
	// Grab 6 px off center and move 20 px right and 10 px up: 10% and 5%.
	app.DragFromTo(tela.Point{X: cx + 6, Y: cy + 6}, tela.Point{X: cx + 26, Y: cy - 4})
	app.Expect(th, telatest.HasValue("Saturation 60%, Brightness 55%"))
}

func TestColorAreaKeyboard(t *testing.T) {
	var changes, ends events
	p := picker{
		state: aria.ColorStateOptions{
			DefaultValue: new(aria.HSB{H: 120, S: 50, B: 50, A: 1}.Color()),
			OnChange:     func(c tela.Color) { changes.add(aria.FormatHexColor(c, false)) },
		},
		area:  aria.ColorAreaOptions{OnChangeEnd: func(c tela.Color) { ends.add(aria.FormatHexColor(c, false)) }},
		parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	app.Press("Tab")
	app.ExpectFocused(th)
	press := func(key, want string) {
		t.Helper()
		app.Press(key)
		app.Expect(th, telatest.HasValue(want))
	}
	press("ArrowRight", "Saturation 51%, Brightness 50%")
	press("ArrowUp", "Saturation 51%, Brightness 51%")
	press("ArrowLeft", "Saturation 50%, Brightness 51%")
	press("ArrowDown", "Saturation 50%, Brightness 50%")
	press("Shift+ArrowRight", "Saturation 60%, Brightness 50%")
	press("Shift+ArrowUp", "Saturation 60%, Brightness 60%")
	press("Shift+ArrowLeft", "Saturation 50%, Brightness 60%")
	press("Shift+ArrowDown", "Saturation 50%, Brightness 50%")
	// PageUp and PageDown change Y by a page; Home and End change X.
	press("PageUp", "Saturation 50%, Brightness 60%")
	press("PageDown", "Saturation 50%, Brightness 50%")
	press("End", "Saturation 60%, Brightness 50%")
	press("Home", "Saturation 50%, Brightness 50%")
	// Clamped at the corners.
	for range 12 {
		app.Press("Shift+ArrowRight")
		app.Press("Shift+ArrowUp")
	}
	app.Expect(th, telatest.HasValue("Saturation 100%, Brightness 100%"))
	changes.take()
	app.Press("ArrowRight") // nothing changes at the limit
	app.Press("ArrowUp")
	if got := changes.take(); got != "" {
		t.Errorf("OnChange at the limit: %s", got)
	}
	// OnChangeEnd follows every key that changed the color.
	if got := len(strings.Fields(ends.take())); got != 22 {
		t.Errorf("%d OnChangeEnd calls, want 22", got)
	}
}

// React Aria mirrors the area in a right-to-left direction: saturation
// grows toward the left, and so do the arrow keys.
func TestColorAreaRTL(t *testing.T) {
	p := picker{
		state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 120, S: 50, B: 50, A: 1}.Color())},
		area:  aria.ColorAreaOptions{Direction: "rtl"}, parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	app.Focus(th)
	app.Press("ArrowRight")
	app.Expect(th, telatest.HasValue("Saturation 49%, Brightness 50%"))
	app.Press("ArrowLeft")
	app.Press("ArrowLeft")
	app.Expect(th, telatest.HasValue("Saturation 51%, Brightness 50%"))
	app.Press("ArrowUp") // up is still more
	app.Expect(th, telatest.HasValue("Saturation 51%, Brightness 51%"))
	app.Press("Home") // toward the left edge: more saturation
	app.Expect(th, telatest.HasValue("Saturation 61%, Brightness 51%"))
	// A press at the left edge is the most saturation.
	app.ClickAt(areaOrigin+2, areaOrigin+areaLen/2)
	app.Expect(th, telatest.HasValue("Saturation 99%, Brightness 50%"))
	// And the thumb is drawn at the left.
	if cx, _ := center(app, th); cx > areaOrigin+4 {
		t.Errorf("the thumb is at x %g, want the left edge", cx)
	}
}

func TestColorAreaOtherChannels(t *testing.T) {
	p := picker{
		state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 100, S: 80, B: 80, A: 1}.Color())},
		area:  aria.ColorAreaOptions{XChannel: aria.ColorHue, YChannel: aria.ColorSaturation}, parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	app.Expect(th, telatest.HasValue("Hue 100°, Saturation 80%"))
	app.Focus(th)
	app.Press("Shift+ArrowRight") // a page of hue is 15 degrees
	app.Expect(th, telatest.HasValue("Hue 115°, Saturation 80%"))
	app.ClickAt(areaOrigin+areaLen/2, areaOrigin) // hue 180, saturation 100
	app.Expect(th, telatest.HasValue("Hue 180°, Saturation 100%"))
	// Brightness stayed.
	app.ExpectText("color #00ccccff")
}

func TestColorAreaDisabled(t *testing.T) {
	var changes int
	p := picker{
		state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 0, S: 50, B: 50, A: 1}.Color()), OnChange: func(tela.Color) { changes++ }},
		area:  aria.ColorAreaOptions{Disabled: true}, parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(areaThumb(), telatest.Disabled(true))
	x, y := areaPoint(80, 80)
	app.ClickAt(x, y)
	app.DragFromTo(tela.Point{X: x, Y: y}, tela.Point{X: x + 30, Y: y + 30})
	app.Expect(areaThumb(), telatest.HasValue("Saturation 50%, Brightness 50%"))
	if changes != 0 {
		t.Errorf("%d changes of a disabled area", changes)
	}
}

// The hue survives a trip through the gray axis and through black: the
// state keeps it.
func TestColorAreaKeepsTheHue(t *testing.T) {
	var last tela.Color
	p := picker{
		state: aria.ColorStateOptions{
			DefaultValue: new(aria.HSB{H: 200, S: 100, B: 100, A: 1}.Color()),
			OnChange:     func(c tela.Color) { last = c },
		},
		parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	x, y := areaPoint(0, 80) // gray
	app.ClickAt(x, y)
	app.ExpectText("color #cccccc")
	x, y = areaPoint(60, 80)
	app.ClickAt(x, y)
	if h := aria.ToHSB(last); !closeTo(h.H, 200, 0.5) {
		t.Errorf("the hue after the gray is %g, want 200 (%v)", h.H, last)
	}
	x, y = areaPoint(60, 0) // black
	app.ClickAt(x, y)
	x, y = areaPoint(60, 50)
	app.ClickAt(x, y)
	if h := aria.ToHSB(last); !closeTo(h.H, 200, 0.5) || !closeTo(h.S, 60, 0.5) {
		t.Errorf("after black: hue %g, saturation %g, want 200 and 60", h.H, h.S)
	}
}

func TestColorAreaTouchAndPen(t *testing.T) {
	for _, kind := range []platform.PointerKind{platform.PointerTouch, platform.PointerPen} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			var ends events
			p := picker{
				state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 0, S: 50, B: 50, A: 1}.Color())},
				area:  aria.ColorAreaOptions{OnChangeEnd: func(c tela.Color) { ends.add(aria.FormatHexColor(c, false)) }},
				parts: "a",
			}
			a := mountRaw(t, tela.C(p.page()), 232, 400)
			value := func() string { return rawwin.FindNode(a.tree.Semantics(), "slider", "Color").Value }
			cx, cy := areaPoint(50, 50)
			a.down(kind, cx, cy)
			a.move(kind, cx+20, cy-20)
			a.move(kind, 226, 8) // off the area: still dragging
			a.up(kind, 226, 8)
			if got := value(); got != "Saturation 100%, Brightness 100%" {
				t.Errorf("after the drag: %q", got)
			}
			if got := ends.take(); got != "#ff0000" {
				t.Errorf("OnChangeEnd: %s", got)
			}
			x, y := areaPoint(20, 40)
			a.tap(kind, x, y)
			if got := value(); got != "Saturation 20%, Brightness 40%" {
				t.Errorf("after the tap: %q", got)
			}
			// A cancel ends the drag and keeps the color.
			a.down(kind, x, y)
			a.move(kind, x+40, y)
			a.cancel(kind, x+40, y)
			a.move(kind, x+100, y)
			if got := value(); got != "Saturation 40%, Brightness 40%" {
				t.Errorf("after the cancel: %q", got)
			}
		})
	}
}

// Dragging repaints only: an uncontrolled color builds when the drag
// starts and when it ends, not on every move, and the thumb follows the
// pointer in the meantime.
func TestColorAreaDragDoesNotBuildPerMove(t *testing.T) {
	var builds int
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 30, S: 0, B: 100, A: 1}.Color())}, parts: "a", builds: &builds}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	start := builds
	x, y := center(app, th)
	app.MouseMove(x, y)
	app.MouseDown()
	for i := 1; i <= 20; i++ {
		app.MouseMove(x+float32(i)*4, y+float32(i)*3)
		gx, gy := center(app, th)
		if gx < x+float32(i)*4-1 || gx > x+float32(i)*4+1 || gy < y+float32(i)*3-1 || gy > y+float32(i)*3+1 {
			t.Fatalf("move %d: the thumb is at (%g, %g), want (%g, %g)", i, gx, gy, x+float32(i)*4, y+float32(i)*3)
		}
	}
	if during := builds - start; during > 1 {
		t.Errorf("%d builds while dragging, want the one of the drag start", during)
	}
	app.MouseUp()
	app.Expect(th, telatest.HasValue("Saturation 40%, Brightness 70%"))
	if total := builds - start; total > 2 {
		t.Errorf("%d builds for the whole drag, want 2 (start and end)", total)
	}
	st := app.Measure(func() {
		app.MouseMove(x, y)
		app.MouseDown()
		app.MouseMove(x+30, y+30)
		app.MouseMove(x+60, y+10)
		app.MouseMove(x+20, y+50)
		app.MouseUp()
	})
	t.Logf("a whole drag: %d builds, %d layouts, %d paints (%d frames)", st.Builds, st.Layouts, st.Paints, st.Frames)
	if st.Builds > 2 || st.Layouts > 2 {
		t.Errorf("a drag built %d components and laid out %d times, want 2 builds (start and end) and repaints for the moves", st.Builds, st.Layouts)
	}
}

// A controlled color shows what the caller passes and builds as often as
// the caller's state changes.
func TestColorAreaControlled(t *testing.T) {
	var set tela.Setter[tela.Color]
	var asked events
	page := func() tela.Node {
		c, s := tela.UseState(aria.HSB{H: 0, S: 0, B: 100, A: 1}.Color())
		set = s
		return picker{
			state: aria.ColorStateOptions{Value: &c, OnChange: func(n tela.Color) { asked.add(aria.FormatHexColor(n, false)); s.Set(n) }},
			parts: "a",
		}.usePicker()
	}
	app := telatest.MountFunc(t, page, telatest.Size(232, 400))
	th := areaThumb()
	app.Focus(th)
	app.Press("ArrowRight")
	app.Expect(th, telatest.HasValue("Saturation 1%, Brightness 100%"))
	x, y := areaPoint(70, 20)
	app.ClickAt(x, y)
	app.Expect(th, telatest.HasValue("Saturation 70%, Brightness 20%"))
	set.Set(tela.Hex("#336699"))
	app.Expect(th, telatest.HasValue("Saturation 67%, Brightness 60%"))
	if got := asked.take(); !strings.HasPrefix(got, "#fffcfc") {
		t.Errorf("OnChange: %s", got)
	}
}

// A controlled color that the caller refuses does not move; one that is a
// gray keeps the hue it was shown with.
func TestColorAreaControlledRefuses(t *testing.T) {
	v := aria.HSB{H: 120, S: 50, B: 50, A: 1}.Color()
	var asked events
	p := picker{
		state: aria.ColorStateOptions{Value: &v, OnChange: func(n tela.Color) { asked.add(aria.FormatHexColor(n, false)) }},
		parts: "a",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	th := areaThumb()
	app.Focus(th)
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	x, y := areaPoint(80, 80)
	app.ClickAt(x, y)
	app.Expect(th, telatest.HasValue("Saturation 50%, Brightness 50%"))
	if asked.take() == "" {
		t.Error("no OnChange for the refused changes")
	}
	cx, cy := center(app, th)
	wx, wy := areaPoint(50, 50)
	if cx < wx-1 || cx > wx+1 || cy < wy-1 || cy > wy+1 {
		t.Errorf("the thumb is at (%g, %g), want (%g, %g)", cx, cy, wx, wy)
	}
}

func TestColorAreaOptionErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		o    aria.ColorAreaOptions
		want []string
	}{
		"channel":   {aria.ColorAreaOptions{XChannel: "satration"}, []string{"XChannel", `did you mean "saturation"`}},
		"same":      {aria.ColorAreaOptions{XChannel: aria.ColorHue, YChannel: aria.ColorHue}, []string{"both", "two different channels"}},
		"alpha":     {aria.ColorAreaOptions{YChannel: aria.ColorAlpha}, []string{"YChannel", `"alpha"`}},
		"direction": {aria.ColorAreaOptions{Direction: "rtll"}, []string{"Direction", `"rtll"`}},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				msg := fmt.Sprint(recover())
				for _, w := range tc.want {
					if !strings.Contains(msg, w) {
						t.Errorf("message %q lacks %q", msg, w)
					}
				}
			}()
			telatest.MountFunc(t, picker{area: tc.o, parts: "a"}.page(), telatest.Size(232, 400))
		})
	}
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "State is required") || !strings.Contains(msg, "UseColorState") {
			t.Errorf("message %q", msg)
		}
	}()
	telatest.MountFunc(t, func() tela.Node {
		a := aria.UseColorArea(aria.ColorAreaOptions{})
		return Div(a.AreaProps)
	}, telatest.Size(100, 100))
}
