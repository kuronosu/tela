package main

import (
	"cmp"
	"context"
	"fmt"
	"image"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Demo is the window's content.
type Demo struct {
	// Scene, Percent and Size are what the selectors start at: the scene, the
	// size of its frames (0 is 100% of the size they are shown at) and the
	// size of the card ("" is Medium).
	Scene   Scene
	Percent int
	Size    CardSize
	// At is where the scene's clock starts.
	At time.Duration
	// Paused starts it paused.
	Paused bool
	// Still draws the scene At in software, inside the frame that shows it,
	// and starts no loop: for snapshots.
	Still bool
}

// CardSize is how wide the card is.
type CardSize string

const (
	Small  CardSize = "small"
	Medium CardSize = "medium"
	Full   CardSize = "full"
)

// cardWidths are the classes of the sizes; Full is the window's width.
var cardWidths = map[CardSize]string{Small: "max-w-md", Medium: "max-w-3xl", Full: ""}

// sceneWidth leaves room for the controls below the scene in a short window:
// the scene is narrower there than the card, whose sides are black.
const sceneWidth = "max-w-[max(20rem,calc((100vh-11rem)*16/9))]"

// percents are the sizes of the frames to choose from.
var percents = []int{25, 50, 75, 100, 200}

func (d Demo) Build() tela.Node {
	tex := tela.UseTexture()
	paused, setPaused := tela.UseState(d.Paused || d.Still)
	speed, setSpeed := tela.UseState(1.0)
	mode, setMode := tela.UseState(Auto)
	status := tela.UseValue(Status{Still: d.Still})
	scene, setScene := tela.UseState(d.Scene)
	percent, setPercent := tela.UseState(cmp.Or(d.Percent, 100))
	size, setSize := tela.UseState(cmp.Or(d.Size, Medium))
	ctl := tela.UseOnce(func() *controls { return newControls(d.At, picture{scene, percent}) })

	still := func(size tela.TextureSize) {
		if w, h, ok := drawStill(tex, size, ctl.picture(), d.At); ok {
			status.Set(Status{Still: true, Device: "the CPU", Painter: softwareName, Size: size, Width: w, Height: h})
		}
	}
	tela.UseLayoutEffect(func() func() {
		tex.OnResize(func(size tela.TextureSize) {
			if d.Still {
				still(size)
				return
			}
			ctl.changed()
		})
		return nil
	}, tela.Once)
	tela.UseEffect(func() func() {
		if pic := (picture{scene, percent}); pic != ctl.picture() {
			ctl.show(pic)
			if d.Still {
				still(tex.Size())
			}
		}
		return nil
	}, scene, percent)
	tela.UseEffect(func() func() {
		ctl.set(paused, speed)
		return nil
	}, paused, speed)
	// The loop draws with one renderer; another choice starts another.
	tela.UseGo(func(ctx context.Context) {
		if !d.Still {
			stream(ctx, tex, ctl, mode, status.Set)
		}
	}, mode)

	row := func(label string, value func(Status) string) tela.Node {
		return Div(Class("flex items-baseline gap-4"),
			Span(Class("w-28 shrink-0 text-muted-foreground"), Text(label)),
			Span(Class("min-w-0 flex-1 tabular-nums"), TextFn(func() string { return value(status.Get()) })))
	}
	return Div(Class("flex min-h-screen items-center justify-center bg-background p-6"),
		Div(Class("w-full overflow-hidden rounded-3xl border border-border bg-card text-card-foreground shadow-2xl", cardWidths[size]),
			Div(Class("relative bg-black"),
				Div(Class("mx-auto", sceneWidth),
					Texture(tex, Class("block aspect-video w-full"), AriaLabel(scenes[scene].about))),
				Div(Class("absolute left-4 top-4 rounded-full bg-black/60 px-3 py-1 text-xs font-medium text-white/85 tabular-nums"),
					TextFn(func() string { return status.Get().badge() }))),
			Div(Class("flex flex-wrap items-center justify-between gap-x-6 gap-y-4 border-t border-border px-6 pt-4"),
				kit.ToggleGroup[Scene]{Variant: kit.Outline, Value: &scene, OnValueChange: setScene.Set, AriaLabel: "Scene",
					DisallowEmptySelection: true, Items: sceneItems()},
				Div(Class("flex items-center gap-3 text-sm"),
					Span(Class("text-muted-foreground"), Text("Resolution")),
					kit.Select[int]{Class: "w-24", Value: &percent, OnValueChange: setPercent.Set, AriaLabel: "Resolution",
						Items: percentItems(), Props: TestID("resolution")}),
				Div(Class("flex items-center gap-3 text-sm"),
					Span(Class("text-muted-foreground"), Text("Size")),
					kit.ToggleGroup[CardSize]{Variant: kit.Outline, Value: &size, OnValueChange: setSize.Set, AriaLabel: "Size",
						DisallowEmptySelection: true, Items: []kit.ToggleGroupItem[CardSize]{
							{Value: Small, Label: "Small"}, {Value: Medium, Label: "Medium"}, {Value: Full, Label: "Full"},
						}})),
			Div(Class("flex flex-wrap items-center gap-x-6 gap-y-4 px-6 py-4"),
				kit.Button{Class: "w-20", OnClick: func() { setPaused.Update(func(p bool) bool { return !p }) },
					Children: Text(either(paused, "Play", "Pause"))},
				Div(Class("flex min-w-48 flex-1 items-center gap-3 text-sm"),
					Span(Class("text-muted-foreground"), Text("Speed")),
					kit.Slider{Value: &speed, OnValueChange: setSpeed.Set, Min: 0.25, Max: 4, Step: 0.25,
						AriaLabel: "Speed", FormatValue: times},
					Span(Class("w-12 text-right tabular-nums"), Text(times(speed)))),
				kit.ToggleGroup[Mode]{Variant: kit.Outline, Value: &mode, OnValueChange: setMode.Set, AriaLabel: "Drawn by",
					Items: []kit.ToggleGroupItem[Mode]{
						{Value: Auto, Label: "Same GPU"}, {Value: Other, Label: "Another GPU"}, {Value: Software, Label: "Software"},
					}}),
			Div(Class("flex flex-col gap-1.5 border-t border-border px-6 py-4 text-sm"),
				row("Tela draws on", func(s Status) string { return s.Device }),
				row("The scene on", Status.painter),
				row("Frames", Status.frames))))
}

// either returns a when cond holds, else b.
func either(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func sceneItems() (out []kit.ToggleGroupItem[Scene]) {
	for i, s := range scenes {
		out = append(out, kit.ToggleGroupItem[Scene]{Value: Scene(i), Label: s.name})
	}
	return out
}

func percentItems() (out []kit.SelectItem[int]) {
	for _, p := range percents {
		out = append(out, kit.SelectItem[int]{Value: p, Label: fmt.Sprintf("%d%%", p)})
	}
	return out
}

// times writes a speed: 1.5×.
func times(v float64) string { return fmt.Sprintf("%g×", v) }

// drawStill shows the scene at a time, drawn in software before the frame
// that shows it is painted (Texture.OnResize).
func drawStill(tex *tela.Texture, size tela.TextureSize, pic picture, at time.Duration) (w, h int, ok bool) {
	if size.Width == 0 {
		return 0, 0, false
	}
	w, h = frameSize(size, pic.percent, frameLimit)
	u := sceneAt(pic.scene, at, w, h)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	drawScene(img, &u)
	tex.SubmitImage(img, func() {})
	return w, h, true
}
