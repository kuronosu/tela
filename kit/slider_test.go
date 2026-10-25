package kit_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's Slider (new-york-v4/ui/slider.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnSlider      = "relative flex w-full touch-none items-center select-none data-[disabled]:opacity-50 data-[orientation=vertical]:h-full data-[orientation=vertical]:min-h-44 data-[orientation=vertical]:w-auto data-[orientation=vertical]:flex-col"
	shadcnSliderTrack = "relative grow overflow-hidden rounded-full bg-muted data-[orientation=horizontal]:h-1.5 data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-1.5"
	shadcnSliderRange = "absolute bg-primary data-[orientation=horizontal]:h-full data-[orientation=vertical]:w-full"
	shadcnSliderThumb = "block size-4 shrink-0 rounded-full border border-primary bg-white shadow-sm ring-ring/50 transition-[color,box-shadow] hover:ring-4 focus-visible:ring-4 focus-visible:outline-hidden disabled:pointer-events-none disabled:opacity-50"
)

// sliderHTML is the markup Radix and shadcn render for a slider with the
// given values on 0 to 100: the range's inset and each thumb's position
// are the ones Radix computes (the thumb stays inside the track: its
// center runs from half its size, 8 px, to the track's end less 8 px).
func sliderHTML(probe string, values []float64, vertical, disabled bool) string {
	orient, size := "horizontal", "left"
	if vertical {
		orient, size = "vertical", "bottom"
	}
	dis := ` aria-disabled="false"`
	disTrack, disThumb := "", ""
	if disabled {
		dis = ` aria-disabled="true" data-disabled=""`
		disTrack, disThumb = ` data-disabled=""`, ` data-disabled=""`
	}
	lo, hi := 0.0, values[0]
	if len(values) > 1 {
		lo, hi = slices.Min(values), slices.Max(values)
	}
	var rng string
	if vertical {
		rng = fmt.Sprintf("bottom: %g%%; top: %g%%;", lo, 100-hi)
	} else {
		rng = fmt.Sprintf("left: %g%%; right: %g%%;", lo, 100-hi)
	}
	var thumbs strings.Builder
	for i, v := range values {
		offset := 8 - v*8/50
		transform := "translateX(-50%)"
		if vertical {
			transform = "translateY(50%)"
		}
		fmt.Fprintf(&thumbs, `<span style="transform: %s; position: absolute; %s: calc(%g%% + %gpx);"><span role="slider" aria-label="thumb %d" aria-valuemin="0" aria-valuemax="100" aria-orientation=%q data-orientation=%q tabindex="0" data-slot="slider-thumb"%s class="%s" aria-valuenow="%g"></span></span>`,
			transform, size, v, offset, i, orient, orient, disThumb, shadcnSliderThumb, v)
	}
	return fmt.Sprintf(`<span dir="ltr" data-orientation=%q%s data-slot="slider" data-probe=%q class="%s"><span data-orientation=%q%s data-slot="slider-track" class="%s"><span data-orientation=%q%s data-slot="slider-range" class="%s" style="%s"></span></span>%s</span>`,
		orient, dis, probe, shadcnSlider, orient, disTrack, shadcnSliderTrack, orient, disTrack, shadcnSliderRange, rng, thumbs.String())
}

func sliderCell(name string, s kit.Slider, width int) cell {
	s.Props = h.TestID(name + "-slider")
	values := s.DefaultValues
	if len(values) == 0 {
		values = []float64{*s.DefaultValue}
	}
	s.AriaLabel = name
	return cell{name: name, width: width, probes: []string{name + "-slider"}, tela: s,
		html: sliderHTML(name+"-slider", values, s.Orientation == kit.Vertical, s.Disabled)}
}

func sliderSheet() *sheet {
	return &sheet{name: "slider", slot: 300, cells: []cell{
		sliderCell("zero", kit.Slider{DefaultValue: new(0.0)}, 280),
		sliderCell("forty", kit.Slider{DefaultValue: new(40.0)}, 280),
		sliderCell("full", kit.Slider{DefaultValue: new(100.0)}, 280),
		sliderCell("third", kit.Slider{DefaultValue: new(33.0)}, 281),
		sliderCell("disabled", kit.Slider{DefaultValue: new(60.0), Disabled: true}, 280),
		sliderCell("range", kit.Slider{DefaultValues: []float64{25, 75}}, 280),
		sliderCell("range-close", kit.Slider{DefaultValues: []float64{48, 52}}, 280),
		sliderCell("vertical", kit.Slider{DefaultValue: new(30.0), Orientation: kit.Vertical}, 20),
		sliderCell("vertical-range", kit.Slider{DefaultValues: []float64{20, 80}, Orientation: kit.Vertical}, 20),
	}}
}

func TestSliderGolden(t *testing.T)   { sliderSheet().golden(t) }
func TestChromiumSlider(t *testing.T) { sliderSheet().chromium(t) }

func sliderRole(name string) telatest.Locator { return telatest.ByRole("slider", name) }

func TestSliderKeyboard(t *testing.T) {
	var got [][]float64
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[300px] p-4"), kit.Slider{AriaLabel: "Volume", DefaultValue: new(50.0), Step: 5,
			OnValuesChange: func(v []float64) { got = append(got, v) }})
	})
	vol := sliderRole("Volume")
	app.Expect(vol, telatest.HasValue("50"))
	app.Focus(vol)
	app.Press("ArrowRight")
	app.Expect(vol, telatest.HasValue("55"))
	app.Press("ArrowLeft")
	app.Press("ArrowLeft")
	app.Expect(vol, telatest.HasValue("45"))
	app.Press("Home")
	app.Expect(vol, telatest.HasValue("0"))
	app.Press("End")
	app.Expect(vol, telatest.HasValue("100"))
	app.Press("PageDown")
	app.Expect(vol, telatest.HasValue("90"))
	if len(got) != 6 || got[0][0] != 55 {
		t.Errorf("OnValuesChange got %v", got)
	}
}

func TestSliderPointer(t *testing.T) {
	var committed []float64
	var changed []float64
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[300px] p-4"), kit.Slider{AriaLabel: "Volume", Props: h.TestID("s"),
			OnValueChange: func(v float64) { changed = append(changed, v) },
			OnValueCommit: func(v []float64) { committed = v }})
	})
	root := app.Bounds(telatest.ByTestID("s"))
	y := root.Y + root.Height/2
	// A press on the track moves the knob there and goes on dragging it.
	app.MouseMove(root.X+root.Width/4, y)
	app.MouseDown()
	app.MouseMove(root.X+root.Width*3/4, y)
	app.MouseUp()
	app.Expect(sliderRole("Volume"), telatest.HasValue("75"))
	if len(committed) != 1 || committed[0] != 75 {
		t.Errorf("OnValueCommit got %v, want [75]", committed)
	}
	if len(changed) == 0 || changed[len(changed)-1] != 75 {
		t.Errorf("OnValueChange got %v, want it to end at 75", changed)
	}
}

// TestSliderKnobFollowsTheValue checks the knob's place along the track at
// the ends and in the middle: it stays inside the track, as Radix's.
func TestSliderKnobFollowsTheValue(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[300px] p-4"), kit.Slider{AriaLabel: "Volume", Props: h.TestID("s"), DefaultValue: new(0.0)})
	})
	root := app.Bounds(telatest.ByTestID("s"))
	knob := sliderRole("Volume")
	if b := app.Bounds(knob); b.X != root.X || b.Width != 16 {
		t.Errorf("at 0 the knob is at %g (width %g), want the track's left edge %g and 16 wide", b.X, b.Width, root.X)
	}
	app.Focus(knob)
	app.Press("End")
	if b := app.Bounds(knob); b.X+b.Width != root.X+root.Width {
		t.Errorf("at 100 the knob ends at %g, want the track's right edge %g", b.X+b.Width, root.X+root.Width)
	}
	app.Press("Home")
	for range 50 {
		app.Press("ArrowRight")
	}
	if b := app.Bounds(knob); b.X+b.Width/2 < root.X+root.Width/2-0.1 || b.X+b.Width/2 > root.X+root.Width/2+0.1 {
		t.Errorf("at 50 the knob is centered at %g, want %g", b.X+b.Width/2, root.X+root.Width/2)
	}
}

func TestSliderRange(t *testing.T) {
	var last []float64
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[300px] p-4"), kit.Slider{DefaultValues: []float64{20, 80}, AriaLabel: "Price", Props: h.TestID("s"),
			ThumbLabels: []string{"Minimum price", "Maximum price"}, OnValuesChange: func(v []float64) { last = v }})
	})
	lo, hi := sliderRole("Minimum price"), sliderRole("Maximum price")
	app.Expect(lo, telatest.HasValue("20"))
	app.Expect(hi, telatest.HasValue("80"))
	// The range between the knobs is filled: pixels inside it are primary.
	root := app.Bounds(telatest.ByTestID("s"))
	y := root.Y + root.Height/2
	inside, outside := app.PixelAt(root.X+root.Width/2, y), app.PixelAt(root.X+4, y)
	if inside == outside {
		t.Errorf("the range is not filled: %v inside and outside", inside)
	}
	// A knob stops at its neighbor.
	app.Focus(lo)
	app.Press("End")
	app.Expect(lo, telatest.HasValue("80"))
	if !slices.Equal(last, []float64{80, 80}) {
		t.Errorf("values %v, want [80 80]", last)
	}
}

// controlledSlider keeps its value in the caller.
type controlledSlider struct{ refuse bool }

func (c controlledSlider) Build() tela.Node {
	v, setV := tela.UseState(30.0)
	return h.Div(h.Class("w-[300px] p-4"), kit.Slider{AriaLabel: "Level", Value: &v, OnValueChange: func(n float64) {
		if !c.refuse {
			setV.Set(n)
		}
	}}, h.Span(h.Class("sr-only"), h.Text(fmt.Sprintf("level is %g", v))))
}

func TestSliderControlled(t *testing.T) {
	app := telatest.Mount(t, controlledSlider{})
	app.ExpectText("level is 30")
	app.Focus(sliderRole("Level"))
	app.Press("ArrowRight")
	app.ExpectText("level is 31")
	app.Expect(sliderRole("Level"), telatest.HasValue("31"))
	refused := telatest.Mount(t, controlledSlider{refuse: true})
	refused.Focus(sliderRole("Level"))
	refused.Press("ArrowRight")
	refused.Expect(sliderRole("Level"), telatest.HasValue("30"))
}

func TestSliderVerticalAndDisabled(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex h-[200px] gap-8 p-4"),
			kit.Slider{AriaLabel: "Tall", Orientation: kit.Vertical, DefaultValue: new(0.0), Props: h.TestID("v")},
			kit.Slider{AriaLabel: "Off", Disabled: true, DefaultValue: new(50.0), Class: "w-[100px]"})
	})
	tall := sliderRole("Tall")
	track := app.Bounds(telatest.ByTestID("v"))
	// Min is at the bottom.
	if b := app.Bounds(tall); b.Y+b.Height != track.Y+track.Height {
		t.Errorf("at 0 the knob ends at %g, want the bottom of the track %g", b.Y+b.Height, track.Y+track.Height)
	}
	app.Focus(tall)
	app.Press("ArrowUp")
	app.Expect(tall, telatest.HasValue("1"))
	app.Press("End")
	if b := app.Bounds(tall); b.Y != track.Y {
		t.Errorf("at 100 the knob is at %g, want the top of the track %g", b.Y, track.Y)
	}
	app.Expect(sliderRole("Off"), telatest.Disabled(true))
	app.Focus(tall)
	app.Press("Home")
	app.Expect(tall, telatest.HasValue("0"))
}

func TestSliderAriaSnapshot(t *testing.T) {
	got, err := telatest.AriaSnapshot(h.Div(
		kit.Slider{AriaLabel: "Volume", DefaultValue: new(30.0)},
		kit.Slider{AriaLabel: "Price", DefaultValues: []float64{10, 90}, ThumbLabels: []string{"Minimum", "Maximum"}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`group "Volume"`, `slider "Volume"`, `group "Price"`, `slider "Minimum"`, `slider "Maximum"`} {
		if !strings.Contains(got, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, got)
		}
	}
}

// TestSliderDragRepaintsOnly measures a drag of 20 moves: the slider builds
// when the drag starts and when it ends, not on every move (ADR-132), and
// a one-knob slider does not even lay out again, its range being slid by a
// paint-phase translate.
func TestSliderDragRepaintsOnly(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[300px] p-4"), kit.Slider{AriaLabel: "Volume", Props: h.TestID("s")})
	})
	root := app.Bounds(telatest.ByTestID("s"))
	y := root.Y + root.Height/2
	app.MouseMove(root.X+10, y)
	app.MouseDown()
	st := app.Measure(func() {
		for i := 1; i <= 20; i++ {
			app.MouseMove(root.X+10+float32(i)*10, y)
		}
	})
	t.Logf("20 moves of one drag: %d builds, %d layouts, %d paints (%d frames)", st.Builds, st.Layouts, st.Paints, st.Frames)
	if st.Builds > 0 {
		t.Errorf("%d builds while dragging, want none", st.Builds)
	}
	if st.Layouts > 0 {
		t.Errorf("%d layouts while dragging a one-knob slider, want none", st.Layouts)
	}
	app.MouseUp()
	// The press was on the knob, so the grab is kept: 200 px of 268 is 75.
	app.Expect(sliderRole("Volume"), telatest.HasValue("75"))
	// A range slider's fill needs a layout per move, and still no builds.
	app2 := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[300px] p-4"), kit.Slider{AriaLabel: "Price", DefaultValues: []float64{10, 90}, Props: h.TestID("s")})
	})
	root = app2.Bounds(telatest.ByTestID("s"))
	app2.MouseMove(root.X+root.Width*0.1, y)
	app2.MouseDown()
	st = app2.Measure(func() {
		for i := 1; i <= 20; i++ {
			app2.MouseMove(root.X+root.Width*0.1+float32(i)*5, y)
		}
	})
	t.Logf("a range slider: %d builds, %d layouts, %d paints (%d frames)", st.Builds, st.Layouts, st.Paints, st.Frames)
	if st.Builds > 0 {
		t.Errorf("%d builds while dragging a range slider, want none", st.Builds)
	}
}

// TestSliderValueSource checks a slider moved by a tela.Value: a change of
// the Value moves the knob with no build, and a drag stores its values in
// it, also with no build.
func TestSliderValueSource(t *testing.T) {
	var pos *tela.Value[float64]
	builds := 0
	app := telatest.MountFunc(t, func() tela.Node {
		pos = tela.UseValue(20.0)
		return h.Div(h.Class("w-[300px] p-4"), tela.C(func() tela.Node {
			builds++
			return h.Fragment(kit.Slider{AriaLabel: "Seek", Props: h.TestID("s"), ValueSource: pos,
				OnValueChange: func(v float64) { pos.Set(v) }})
		}))
	})
	root := app.Bounds(telatest.ByTestID("s"))
	knob := sliderRole("Seek")
	center := func() float32 { b := app.Bounds(knob); return (b.X + b.Width/2 - root.X - 8) / (root.Width - 16) }
	if c := center(); c < 0.199 || c > 0.201 {
		t.Fatalf("the knob starts at %g of the track, want 0.2", c)
	}

	// The clock: the Value moves, nothing builds, the knob follows.
	before := builds
	st := app.Measure(func() {
		tela.Post(func() { pos.Set(60) })
		app.Advance(16 * time.Millisecond)
	})
	if c := center(); c < 0.599 || c > 0.601 {
		t.Errorf("the knob is at %g of the track after the Value moved to 60, want 0.6", c)
	}
	if st.Builds > 0 || builds != before {
		t.Errorf("%d builds (the slider's parent %d) for a change of the Value, want none", st.Builds, builds-before)
	}

	// A key starts from the Value's current value, not from the one the
	// slider was built with (20).
	app.Focus(knob)
	app.Press("ArrowRight")
	if got := pos.Latest(); got != 61 {
		t.Errorf("after ArrowRight the Value is %g, want 61", got)
	}
	app.Expect(knob, telatest.HasValue("61")) // the semantics follow a key

	// A drag stores each position in the Value and builds nothing.
	y := root.Y + root.Height/2
	app.MouseMove(root.X+root.Width*0.3, y)
	app.MouseDown()
	st = app.Measure(func() {
		for i := 1; i <= 20; i++ {
			app.MouseMove(root.X+root.Width*0.3+float32(i)*5, y)
		}
	})
	app.MouseUp()
	if st.Builds > 0 {
		t.Errorf("%d builds while dragging a slider with a ValueSource, want none", st.Builds)
	}
	if got := pos.Latest(); got < 40 || got > 70 {
		t.Errorf("after the drag the Value is %g, want the last position (about 62)", got)
	}
}

func TestSliderUndefinedOrientation(t *testing.T) {
	_, err := telatest.AriaSnapshot(kit.Slider{Orientation: "diagonal"})
	if err == nil || !strings.Contains(err.Error(), `Slider.Orientation "diagonal"`) {
		t.Errorf("error %v, want the orientation reported", err)
	}
}

func ExampleSlider() {
	t := &exampleT{}
	defer t.close()
	volume := 30.0
	page := func() tela.Node {
		return h.Div(h.Class("w-64 p-6"), kit.Slider{AriaLabel: "Volume", DefaultValue: &volume, Step: 10,
			OnValueChange: func(v float64) { volume = v }})
	}
	app := telatest.MountFunc(t, page)
	app.Focus(telatest.ByRole("slider", "Volume"))
	app.Press("ArrowRight")
	fmt.Println("volume", volume)
	// Output: volume 40
}
