package main

import (
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
	// At is where the scene's clock starts.
	At time.Duration
	// Paused starts it paused.
	Paused bool
	// Still draws the scene At in software, inside the frame that shows it,
	// and starts no loop: for snapshots.
	Still bool
}

func (d Demo) Build() tela.Node {
	tex := tela.UseTexture()
	paused, setPaused := tela.UseState(d.Paused || d.Still)
	speed, setSpeed := tela.UseState(1.0)
	mode, setMode := tela.UseState(Auto)
	status := tela.UseValue(Status{Still: d.Still})
	ctl := tela.UseOnce(func() *controls { return newControls(d.At) })

	tela.UseLayoutEffect(func() func() {
		tex.OnResize(func(size tela.TextureSize) {
			if d.Still {
				drawStill(tex, size, d.At)
				status.Set(Status{Still: true, Device: "the CPU", Painter: softwareName, Size: size})
				return
			}
			ctl.changed()
		})
		return nil
	}, tela.Once)
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
		Div(Class("w-full max-w-3xl overflow-hidden rounded-3xl border border-border bg-card text-card-foreground shadow-2xl"),
			Div(Class("relative"),
				Texture(tex, Class("block aspect-video w-full bg-black"), AriaLabel("A ringed planet with four moons, turning")),
				Div(Class("absolute left-4 top-4 rounded-full bg-black/45 px-3 py-1 text-xs font-medium text-white/85 tabular-nums"),
					TextFn(func() string { return status.Get().badge() }))),
			Div(Class("flex flex-wrap items-center gap-x-6 gap-y-4 border-t border-border px-6 py-4"),
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

// times writes a speed: 1.5×.
func times(v float64) string { return fmt.Sprintf("%g×", v) }

// drawStill shows the scene at a time, drawn in software before the frame
// that shows it is painted (Texture.OnResize).
func drawStill(tex *tela.Texture, size tela.TextureSize, at time.Duration) {
	if size.Width == 0 {
		return
	}
	u := sceneAt(at, size.Width, size.Height)
	img := image.NewRGBA(image.Rect(0, 0, size.Width, size.Height))
	drawScene(img, &u)
	tex.SubmitImage(img, func() {})
}
