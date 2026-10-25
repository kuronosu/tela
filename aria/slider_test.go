package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// The slider pages have a track of 200 px at x 16 (horizontal) or y 16
// (vertical), with thumbs of 20 px centered on their value.
const trackLen = 200

func sliderPage(o aria.SliderOptions) func() tela.Node {
	return func() tela.Node { return useSliderPage(o, nil) }
}

// useSliderPage is the body of sliderPage, a custom hook so that a test can
// call it from its own component. Every build bumps builds.
func useSliderPage(o aria.SliderOptions, builds *int) tela.Node {
	if builds != nil {
		*builds++
	}
	s := aria.UseSlider(o)
	vertical := o.Orientation == "vertical"
	n := len(s.Values)
	root, track := "w-[232px] p-4", "relative h-2 w-[200px] rounded-full bg-muted"
	if vertical {
		root, track = "h-[232px] p-4", "relative h-[200px] w-2 rounded-full bg-muted"
	}
	return Div(s.Props, Class(root),
		Div(s.TrackProps, Class(track),
			Range(n, func(i int) tela.Node {
				return Div(s.ThumbProps(i), Key(i), Class("absolute size-5 rounded-full bg-primary"),
					If(!vertical, Class("-top-1.5 left-0 -ml-2.5")),
					If(vertical, Class("-left-1.5 top-0 -mt-2.5")),
					Style(tela.TranslateFn(func() (x, y float32) {
						p := float32(s.ThumbPercent(i)) / 100 * trackLen
						switch {
						case vertical:
							return 0, trackLen - p
						case o.Direction == "rtl":
							return trackLen - p, 0
						}
						return p, 0
					})))
			})))
}

func slider(name string) telatest.Locator { return telatest.ByRole("slider", name) }

func TestSliderKeyboard(t *testing.T) {
	var changes, ends events
	o := aria.SliderOptions{
		DefaultValues: []float64{50}, AriaLabel: "Volume",
		OnValuesChange: func(v []float64) { changes.add(fmt.Sprint(v)) },
		OnChangeEnd:    func(v []float64) { ends.add(fmt.Sprint(v)) },
	}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	vol := slider("Volume")
	app.ExpectAria(`
		- group "Volume":
		  - slider "Volume"
	`)
	app.Press("Tab")
	app.ExpectFocused(vol)
	app.Press("ArrowRight")
	app.Expect(vol, telatest.HasValue("51"))
	app.Press("ArrowUp")
	app.Expect(vol, telatest.HasValue("52"))
	app.Press("ArrowLeft")
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Expect(vol, telatest.HasValue("49"))
	app.Press("Shift+ArrowRight") // a page: a tenth of the range
	app.Expect(vol, telatest.HasValue("59"))
	app.Press("PageUp")
	app.Expect(vol, telatest.HasValue("69"))
	app.Press("PageDown")
	app.Press("PageDown")
	app.Expect(vol, telatest.HasValue("49"))
	app.Press("End")
	app.Expect(vol, telatest.HasValue("100"))
	app.Press("ArrowRight") // stays at Max
	app.Expect(vol, telatest.HasValue("100"))
	app.Press("Home")
	app.Expect(vol, telatest.HasValue("0"))
	app.Press("ArrowLeft")
	app.Expect(vol, telatest.HasValue("0"))
	if got := changes.take(); got != "[51] [52] [51] [50] [49] [59] [69] [59] [49] [100] [0]" {
		t.Errorf("OnValuesChange: %s", got)
	}
	// OnChangeEnd comes after every key that changed the value.
	if got := ends.take(); got != "[51] [52] [51] [50] [49] [59] [69] [59] [49] [100] [0]" {
		t.Errorf("OnChangeEnd: %s", got)
	}
}

func TestSliderStepMinMax(t *testing.T) {
	o := aria.SliderOptions{DefaultValues: []float64{-5}, Min: -10, Max: 10, Step: 2.5, AriaLabel: "Offset"}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	th := slider("Offset")
	app.Focus(th)
	app.Press("ArrowRight")
	app.Expect(th, telatest.HasValue("-2.5"))
	app.Press("PageUp") // a tenth of the range is 2, rounded to the step 2.5
	app.Expect(th, telatest.HasValue("0"))
	app.Press("End")
	app.Expect(th, telatest.HasValue("10"))
	app.Press("Home")
	app.Expect(th, telatest.HasValue("-10"))

	// A step with decimals does not drift: ten steps of 0.1 are 1.
	f := aria.SliderOptions{DefaultValues: []float64{0}, Max: 1, Step: 0.1, AriaLabel: "Opacity"}
	app = telatest.MountFunc(t, sliderPage(f), telatest.Size(300, 100))
	th = slider("Opacity")
	app.Focus(th)
	for range 3 {
		app.Press("ArrowRight")
	}
	app.Expect(th, telatest.HasValue("0.3"))
	for range 10 {
		app.Press("ArrowRight")
	}
	app.Expect(th, telatest.HasValue("1"))
}

func TestSliderRTLAndVerticalKeys(t *testing.T) {
	o := aria.SliderOptions{DefaultValues: []float64{50}, Direction: "rtl", AriaLabel: "Volume"}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	app.Focus(slider("Volume"))
	app.Press("ArrowRight") // mirrored
	app.Expect(slider("Volume"), telatest.HasValue("49"))
	app.Press("ArrowLeft")
	app.Press("ArrowLeft")
	app.Expect(slider("Volume"), telatest.HasValue("51"))
	app.Press("ArrowUp") // up is still more
	app.Expect(slider("Volume"), telatest.HasValue("52"))

	v := aria.SliderOptions{DefaultValues: []float64{50}, Orientation: "vertical", AriaLabel: "Volume"}
	app = telatest.MountFunc(t, sliderPage(v), telatest.Size(100, 300))
	app.Focus(slider("Volume"))
	app.Press("ArrowUp")
	app.Expect(slider("Volume"), telatest.HasValue("51"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Expect(slider("Volume"), telatest.HasValue("49"))
}

// A press on the track moves the nearest thumb to the point; dragging goes
// on from there.
func TestSliderTrackPressAndDrag(t *testing.T) {
	var ends events
	o := aria.SliderOptions{DefaultValues: []float64{50}, AriaLabel: "Volume", OnChangeEnd: func(v []float64) { ends.add(fmt.Sprint(v)) }}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	vol := slider("Volume")
	// The track runs from x 16 to 216, at y 20.
	app.ClickAt(16+0.25*trackLen, 20)
	app.Expect(vol, telatest.HasValue("25"))
	app.ExpectFocused(vol)
	if got := ends.take(); got != "[25]" {
		t.Errorf("OnChangeEnd after a click: %s", got)
	}
	// Press on the track and drag past both ends.
	app.MouseMove(16+0.5*trackLen, 20)
	app.MouseDown()
	app.MouseMove(16+0.8*trackLen, 60)
	app.MouseMove(16+0.9*trackLen, 60)
	app.MouseMove(300, 60) // outside the window's track: clamped, the thumb keeps the mouse
	app.Expect(vol, telatest.HasValue("100"))
	app.MouseMove(-50, 60)
	app.Expect(vol, telatest.HasValue("0"))
	app.MouseUp()
	app.Expect(vol, telatest.HasValue("0"))
	if got := ends.take(); got != "[0]" {
		t.Errorf("OnChangeEnd after a drag: %s", got)
	}
	// Moves after the release do nothing.
	app.MouseMove(16+0.5*trackLen, 20)
	app.Expect(vol, telatest.HasValue("0"))
}

// A thumb dragged by its middle does not jump: it follows the pointer's
// movement.
func TestSliderThumbDragKeepsTheGrab(t *testing.T) {
	o := aria.SliderOptions{DefaultValues: []float64{50}, AriaLabel: "Volume"}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	vol := slider("Volume")
	b := app.Bounds(vol)
	// Grab the thumb 8 px off its center and move 20 px: 10 units.
	app.DragFromTo(tela.Point{X: b.X + b.Width/2 + 8, Y: b.Y + b.Height/2}, tela.Point{X: b.X + b.Width/2 + 28, Y: b.Y + b.Height/2})
	app.Expect(vol, telatest.HasValue("60"))
	app.DragBy(vol, -40, 0)
	app.Expect(vol, telatest.HasValue("40"))
}

func TestSliderStepRoundingOnPointer(t *testing.T) {
	o := aria.SliderOptions{DefaultValues: []float64{0}, Step: 10, AriaLabel: "Volume"}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	vol := slider("Volume")
	app.ClickAt(16+0.34*trackLen, 20)
	app.Expect(vol, telatest.HasValue("30"))
	app.ClickAt(16+0.36*trackLen, 20)
	app.Expect(vol, telatest.HasValue("40"))
}

func TestSliderVerticalAndRTLPointer(t *testing.T) {
	v := aria.SliderOptions{DefaultValues: []float64{50}, Orientation: "vertical", AriaLabel: "Volume"}
	app := telatest.MountFunc(t, sliderPage(v), telatest.Size(100, 300))
	// The bottom of the track is Min: y 16 + 200 = 216.
	app.ClickAt(20, 16+0.25*trackLen)
	app.Expect(slider("Volume"), telatest.HasValue("75"))
	app.ClickAt(20, 16+0.9*trackLen)
	app.Expect(slider("Volume"), telatest.HasValue("10"))

	r := aria.SliderOptions{DefaultValues: []float64{50}, Direction: "rtl", AriaLabel: "Volume"}
	app = telatest.MountFunc(t, sliderPage(r), telatest.Size(300, 100))
	app.ClickAt(16+0.25*trackLen, 20) // Min is at the right
	app.Expect(slider("Volume"), telatest.HasValue("75"))
}

func rangeOptions() aria.SliderOptions {
	return aria.SliderOptions{
		DefaultValues: []float64{20, 80}, AriaLabel: "Price",
		ThumbLabels: []string{"Minimum price", "Maximum price"},
	}
}

func TestSliderRange(t *testing.T) {
	app := telatest.MountFunc(t, sliderPage(rangeOptions()), telatest.Size(300, 100))
	lo, hi := slider("Minimum price"), slider("Maximum price")
	app.ExpectAria(`
		- group "Price":
		  - slider "Minimum price"
		  - slider "Maximum price"
	`)
	// A press on the track moves the nearest thumb.
	app.ClickAt(16+0.3*trackLen, 20)
	app.Expect(lo, telatest.HasValue("30"))
	app.Expect(hi, telatest.HasValue("80"))
	app.ClickAt(16+0.6*trackLen, 20) // 60 is closer to 80 than to 30
	app.Expect(hi, telatest.HasValue("60"))
	app.ClickAt(16+0.1*trackLen, 20)
	app.Expect(lo, telatest.HasValue("10"))
	app.ClickAt(16+0.99*trackLen, 20)
	app.Expect(hi, telatest.HasValue("99"))
	// Thumbs do not cross: dragging one into the other stops at it.
	app.Focus(lo)
	app.Press("End")
	app.Expect(lo, telatest.HasValue("99"))
	app.Expect(hi, telatest.HasValue("99"))
	app.Focus(hi)
	app.Press("Home")
	app.Expect(hi, telatest.HasValue("99"))
	app.Press("ArrowLeft")
	app.Expect(hi, telatest.HasValue("99"))
	app.Focus(lo)
	app.Press("ArrowLeft")
	app.Expect(lo, telatest.HasValue("98"))
}

func TestSliderRangeDragDoesNotCross(t *testing.T) {
	app := telatest.MountFunc(t, sliderPage(rangeOptions()), telatest.Size(300, 100))
	lo, hi := slider("Minimum price"), slider("Maximum price")
	app.DragBy(lo, 0.9*trackLen, 0)
	app.Expect(lo, telatest.HasValue("80"))
	app.Expect(hi, telatest.HasValue("80"))
	app.DragBy(hi, -0.9*trackLen, 0)
	app.Expect(hi, telatest.HasValue("80"))
}

func TestSliderMinStepsBetweenThumbs(t *testing.T) {
	o := rangeOptions()
	o.Step, o.MinStepsBetweenThumbs = 5, 2
	o.DefaultValues = []float64{20, 40}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	lo, hi := slider("Minimum price"), slider("Maximum price")
	app.Focus(lo)
	app.Press("End")
	app.Expect(lo, telatest.HasValue("30")) // 40 - 2 steps of 5
	app.Focus(hi)
	app.Press("Home")
	app.Expect(hi, telatest.HasValue("40"))
	app.Press("ArrowLeft")
	app.Expect(hi, telatest.HasValue("40"))
}

func TestSliderControlled(t *testing.T) {
	var asked []string
	var set tela.Setter[[]float64]
	page := func() tela.Node {
		v, s := tela.UseState([]float64{30})
		set = s
		return useSliderPage(aria.SliderOptions{Values: &v, AriaLabel: "Volume",
			OnValuesChange: func(n []float64) { asked = append(asked, fmt.Sprint(n)); s.Set(n) }}, nil)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	vol := slider("Volume")
	app.Focus(vol)
	app.Press("ArrowRight")
	app.Expect(vol, telatest.HasValue("31"))
	app.ClickAt(16+0.6*trackLen, 20)
	app.Expect(vol, telatest.HasValue("60"))
	set.Set([]float64{10})
	app.Expect(vol, telatest.HasValue("10"))
	if got := strings.Join(asked, " "); got != "[31] [60]" {
		t.Errorf("OnValuesChange: %s", got)
	}
}

// A controlled slider shows what the caller passes; the values the user
// asks for and the caller does not store are refused.
func TestSliderControlledRefuses(t *testing.T) {
	var asked events
	v := []float64{30}
	o := aria.SliderOptions{Values: &v, AriaLabel: "Volume", OnValuesChange: func(n []float64) { asked.add(fmt.Sprint(n)) }}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	vol := slider("Volume")
	app.Focus(vol)
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	app.ClickAt(16+0.8*trackLen, 20)
	app.DragBy(vol, 50, 0)
	app.Expect(vol, telatest.HasValue("30"))
	if got := asked.take(); got == "" || strings.Contains(got, "[30]") {
		t.Errorf("OnValuesChange: %q, want the asked values, none of them 30", got)
	}
	// The thumb has not moved on screen either.
	b := app.Bounds(vol)
	if want := float32(16 + 0.3*trackLen); b.X+b.Width/2 < want-1 || b.X+b.Width/2 > want+1 {
		t.Errorf("the thumb is centered at %g, want %g", b.X+b.Width/2, want)
	}
}

// LiveValues gives a controlled slider the values the caller has now, which
// moved since the last build with no build to tell it: a key or a press
// starts from them, not from the values of the last build.
func TestSliderLiveValues(t *testing.T) {
	live := []float64{30}
	var asked events
	built := []float64{30} // what the one build saw
	o := aria.SliderOptions{Values: &built, AriaLabel: "Volume",
		LiveValues:     func() []float64 { return live },
		OnValuesChange: func(n []float64) { asked.add(fmt.Sprint(n)); live = n }}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	vol := slider("Volume")
	live = []float64{80} // a clock moved it; nothing was built
	app.Focus(vol)
	app.Press("ArrowRight")
	if got := asked.take(); got != "[81]" {
		t.Errorf("after a key, OnValuesChange got %q, want [81] (from the live 80, not the built 30)", got)
	}
	live = []float64{10}
	app.ClickAt(16+0.1*trackLen+4, 20) // a press close to the live value of 10
	if got := asked.take(); got != "[12]" {
		t.Errorf("after a press, OnValuesChange got %q, want [12]", got)
	}
}

func TestSliderDisabled(t *testing.T) {
	var changes int
	o := aria.SliderOptions{DefaultValues: []float64{30}, Disabled: true, AriaLabel: "Volume", OnValuesChange: func([]float64) { changes++ }}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	app.Expect(slider("Volume"), telatest.Disabled(true))
	app.ClickAt(16+0.8*trackLen, 20)
	app.DragFromTo(tela.Point{X: 16 + 0.3*trackLen, Y: 20}, tela.Point{X: 16 + 0.9*trackLen, Y: 20})
	app.Expect(slider("Volume"), telatest.HasValue("30"))
	if changes != 0 {
		t.Errorf("%d changes of a disabled slider", changes)
	}
}

func TestSliderValueText(t *testing.T) {
	o := aria.SliderOptions{
		DefaultValues: []float64{50}, AriaLabel: "Volume",
		FormatValue: func(v float64) string { return fmt.Sprintf("%g%%", v) },
	}
	app := telatest.MountFunc(t, sliderPage(o), telatest.Size(300, 100))
	app.Expect(slider("Volume"), telatest.HasValue("50%"))
	app.Focus(slider("Volume"))
	app.Press("ArrowRight")
	app.Expect(slider("Volume"), telatest.HasValue("51%"))
}

// The slider is named by the element with LabelProps.
func TestSliderLabel(t *testing.T) {
	page := func() tela.Node {
		s := aria.UseSlider(aria.SliderOptions{DefaultValues: []float64{50}})
		return Div(s.Props, Class("w-64"),
			Span(s.LabelProps, Text("Brightness")),
			Div(s.TrackProps, Class("relative h-2 w-[200px]"), Div(s.ThumbProps(0), Class("size-5"))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.ExpectRole("slider", "Brightness")
	app.ExpectRole("group", "Brightness")
}

// A touch or a pen drags a thumb too, and the track takes a tap.
func TestSliderTouchDrag(t *testing.T) {
	for _, kind := range []platform.PointerKind{platform.PointerTouch, platform.PointerPen} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			var ends events
			o := aria.SliderOptions{DefaultValues: []float64{50}, AriaLabel: "Volume", OnChangeEnd: func(v []float64) { ends.add(fmt.Sprint(v)) }}
			a := mountRaw(t, tela.C(sliderPage(o)), 300, 100)
			value := func() string { return findNode(a.tree.Semantics(), "slider", "Volume").Value }
			// The thumb is at x 116, y 20.
			a.down(kind, 116, 20)
			a.move(kind, 136, 40)
			a.move(kind, 156, 60) // off the track: still dragging
			a.up(kind, 156, 60)
			if got := value(); got != "70" {
				t.Errorf("after the drag: %q, want 70", got)
			}
			if got := ends.take(); got != "[70]" {
				t.Errorf("OnChangeEnd: %s", got)
			}
			// A tap on the track.
			a.tap(kind, 16+0.1*trackLen, 20)
			if got := value(); got != "10" {
				t.Errorf("after the tap: %q, want 10", got)
			}
			// A cancelled pointer ends the drag, keeping the value.
			a.down(kind, 36, 20)
			a.move(kind, 76, 20)
			a.cancel(kind, 76, 20)
			a.move(kind, 156, 20)
			if got := value(); got != "30" {
				t.Errorf("after the cancel: %q, want 30", got)
			}
			if got := ends.take(); got != "[10] [30]" {
				t.Errorf("OnChangeEnd after the tap and the cancel: %q", got)
			}
		})
	}
}

// Dragging repaints only: an uncontrolled slider builds when the drag
// starts and when it ends, not on every move, and the thumb follows the
// pointer in the meantime.
func TestSliderDragDoesNotBuildPerMove(t *testing.T) {
	var builds int
	page := func() tela.Node {
		return useSliderPage(aria.SliderOptions{DefaultValues: []float64{0}, AriaLabel: "Volume"}, &builds)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	vol := slider("Volume")
	start := builds
	b := app.Bounds(vol)
	x, y := b.X+b.Width/2, b.Y+b.Height/2
	app.MouseMove(x, y)
	app.MouseDown()
	for i := 1; i <= 20; i++ {
		app.MouseMove(x+float32(i)*5, y)
		if got := app.Bounds(vol).X + app.Bounds(vol).Width/2; got < x+float32(i)*5-1 || got > x+float32(i)*5+1 {
			t.Fatalf("move %d: the thumb is centered at %g, want %g", i, got, x+float32(i)*5)
		}
	}
	during := builds - start
	if during > 1 {
		t.Errorf("%d builds while dragging, want the one of the drag start", during)
	}
	app.MouseUp()
	app.Expect(vol, telatest.HasValue("50"))
	if total := builds - start; total > 2 {
		t.Errorf("%d builds for the whole drag, want 2 (start and end)", total)
	}
	st := app.Measure(func() {
		app.MouseMove(x+100, y)
		app.MouseDown()
		app.MouseMove(x+60, y)
		app.MouseMove(x+20, y)
		app.MouseUp()
	})
	t.Logf("a whole drag: %d builds, %d layouts, %d paints (%d frames)", st.Builds, st.Layouts, st.Paints, st.Frames)
	if st.Builds > 2 || st.Layouts > 2 {
		t.Errorf("a drag built %d components and laid out %d times, want 2 builds (start and end) and repaints for the moves", st.Builds, st.Layouts)
	}
}

func TestSliderOptionErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		o    aria.SliderOptions
		want []string
	}{
		"orientation": {aria.SliderOptions{Orientation: "vertcal"}, []string{"Orientation", `did you mean "vertical"`}},
		"max":         {aria.SliderOptions{Min: 10, Max: 5}, []string{"Max (5)", "greater than Min (10)"}},
		"step":        {aria.SliderOptions{Step: -1}, []string{"Step -1", "positive"}},
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
			telatest.MountFunc(t, sliderPage(tc.o), telatest.Size(100, 100))
		})
	}
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "ThumbProps(3)") || !strings.Contains(msg, "1 thumbs") {
			t.Errorf("message %q", msg)
		}
	}()
	telatest.MountFunc(t, func() tela.Node {
		s := aria.UseSlider(aria.SliderOptions{})
		return Div(s.ThumbProps(3))
	}, telatest.Size(100, 100))
}
