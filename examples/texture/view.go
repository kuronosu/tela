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

// cardWidths are the classes of the sizes. The scene is never taller than the
// window: it is 16:9, so its height bounds its width, less the window's margins
// (2rem) and the card's borders.
var cardWidths = map[CardSize]string{
	Small:  "max-w-[min(28rem,calc((100vh-2rem)*16/9))]",
	Medium: "max-w-[min(48rem,calc((100vh-2rem)*16/9))]",
	Full:   "max-w-[calc((100vh-2rem)*16/9)]",
}

// percents are the sizes of the frames to choose from.
var percents = []int{25, 50, 75, 100, 200}

func (d Demo) Build() tela.Node {
	tex := tela.UseTexture()
	// With reduced motion the scene starts as a still, until Play.
	calm, setCalm := tela.UseState(tela.UseReducedMotion() && !d.Still)
	still := d.Still || calm
	paused, setPaused := tela.UseState(d.Paused || still)
	speed, setSpeed := tela.UseState(1.0)
	mode, setMode := tela.UseState(Auto)
	status := tela.UseValue(Status{Still: d.Still})
	scene, setScene := tela.UseState(d.Scene)
	percent, setPercent := tela.UseState(cmp.Or(d.Percent, 100))
	size, setSize := tela.UseState(cmp.Or(d.Size, Medium))
	ctl := tela.UseOnce(func() *controls { return newControls(d.At, picture{scene, percent}) })

	drawn := func(size tela.TextureSize) {
		if w, h, ok := drawStill(tex, size, ctl.picture(), d.At); ok {
			status.Set(Status{Still: d.Still, Device: deviceName(tex.Device()), Painter: softwareName, Size: size, Width: w, Height: h})
		}
	}
	tela.UseLayoutEffect(func() func() {
		tex.OnResize(func(size tela.TextureSize) {
			if still {
				drawn(size)
				return
			}
			ctl.changed()
		})
		return nil
	}, still)
	tela.UseEffect(func() func() {
		if pic := (picture{scene, percent}); pic != ctl.picture() {
			ctl.show(pic)
			if still {
				drawn(tex.Size())
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
		if !still {
			stream(ctx, tex, ctl, mode, status.Set)
		}
	}, mode, still)

	row := func(label string, value func(Status) string) tela.Node {
		return Div(
			Div(Class("text-muted-foreground"), Text(label)),
			Div(Class("tabular-nums wrap-break-word"), TextFn(func() string { return value(status.Get()) })))
	}
	field := func(label string, control tela.Node) tela.Node {
		return Div(Class("flex flex-col gap-1.5"), Div(Class("text-xs text-muted-foreground"), Text(label)), control)
	}
	return Div(Class("flex h-screen items-stretch gap-4 overflow-hidden bg-background p-4"),
		Div(Class("flex min-w-0 flex-1 items-center justify-center"),
			Div(Class("w-full", cardWidths[size]),
				Div(Class("relative overflow-hidden rounded-3xl border border-border bg-black shadow-2xl"),
					Texture(tex, Class("block aspect-video w-full"), AriaLabel(scenes[scene].about)),
					Div(Class("absolute left-4 top-4 rounded-full bg-black/60 px-3 py-1 text-xs font-medium text-white/85 tabular-nums"),
						TextFn(func() string { return status.Get().badge() }))))),
		Div(Class("flex w-88 shrink-0 flex-col overflow-hidden rounded-3xl border border-border bg-card text-sm text-card-foreground"),
			Div(Class("flex shrink-0 flex-col gap-4 p-5"),
				Div(Class("flex items-center gap-3"),
					kit.Button{Class: "w-20", OnClick: func() { setCalm.Set(false); setPaused.Update(func(p bool) bool { return !p }) },
						Children: Text(either(paused, "Play", "Pause"))},
					kit.Slider{Class: "flex-1", Value: &speed, OnValueChange: setSpeed.Set, Min: 0.25, Max: 4, Step: 0.25,
						AriaLabel: "Speed", FormatValue: times},
					Span(Class("w-12 text-right tabular-nums"), Text(times(speed)))),
				field("Scene", kit.ToggleGroup[Scene]{Variant: kit.Outline, Value: &scene, OnValueChange: setScene.Set, AriaLabel: "Scene",
					DisallowEmptySelection: true, Items: sceneItems()}),
				Div(Class("flex gap-4"),
					field("Size", kit.ToggleGroup[CardSize]{Variant: kit.Outline, Value: &size, OnValueChange: setSize.Set, AriaLabel: "Size",
						DisallowEmptySelection: true, Items: []kit.ToggleGroupItem[CardSize]{
							{Value: Small, Label: "Small"}, {Value: Medium, Label: "Medium"}, {Value: Full, Label: "Full"},
						}}),
					field("Resolution", kit.Select[int]{Class: "w-24", Value: &percent, OnValueChange: setPercent.Set, AriaLabel: "Resolution",
						Items: percentItems(), Props: TestID("resolution")})),
				field("Drawn by", kit.ToggleGroup[Mode]{Variant: kit.Outline, Value: &mode, OnValueChange: setMode.Set, AriaLabel: "Drawn by",
					Items: []kit.ToggleGroupItem[Mode]{
						{Value: Auto, Label: "Same GPU"}, {Value: Other, Label: "Another GPU"}, {Value: Software, Label: "Software"},
					}})),
			Div(Class("flex min-h-0 flex-1 flex-col gap-3 overflow-hidden border-t border-border p-5 text-xs"),
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
