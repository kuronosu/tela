package core

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/style"
)

// What a canvas takes of its element besides its colors: the font, the
// direction, the images and the icons (the drawing API itself is tested in
// package canvas).

// The font starts as the element's, and follows what it depends on.
func TestCanvasFont(t *testing.T) {
	for _, tc := range []struct {
		class string
		want  canvas.CanvasFont
		space float64
	}{
		{"", canvas.CanvasFont{Family: "ui-sans-serif, system-ui, sans-serif", Size: 16, Weight: 400}, 0},
		{"text-2xl font-bold italic tracking-[2px]", canvas.CanvasFont{Family: "ui-sans-serif, system-ui, sans-serif", Size: 24, Weight: 700, Italic: true}, 2},
		{"font-mono text-[11px]", canvas.CanvasFont{Family: "ui-monospace, monospace", Size: 11, Weight: 400}, 0},
		{"font-['A,_B',serif]", canvas.CanvasFont{Family: `"A, B", serif`, Size: 16, Weight: 400}, 0},
	} {
		for name, n := range map[string]func(draw func(c *canvas.Canvas)) Node{
			"own":       func(draw func(c *canvas.Canvas)) Node { return canvasNode(draw, Class(tc.class)) },
			"inherited": func(draw func(c *canvas.Canvas)) Node { return div(Class(tc.class), canvasNode(draw)) },
		} {
			ran := false
			newTestTree(t, 400, 300, n(func(c *canvas.Canvas) {
				ran = true
				if c.Font != tc.want || c.LetterSpacing != tc.space {
					t.Errorf("%q %s: %+v, spacing %g", tc.class, name, c.Font, c.LetterSpacing)
				}
				if c.TextAlign != canvas.TextAlignStart || c.TextBaseline != canvas.BaselineAlphabetic {
					t.Errorf("%q: align %v, baseline %v", tc.class, c.TextAlign, c.TextBaseline)
				}
			}), nil).Frame()
			if !ran {
				t.Errorf("%q: no draw", tc.class)
			}
		}
	}
}

// A canvas draws again when the font of its element changes, and only then.
func TestCanvasFontChanges(t *testing.T) {
	var log []string
	var class Setter[string]
	var dir Setter[string]
	// One node, so that what is around it builds and its function stays.
	node := canvasNode(func(c *canvas.Canvas) {
		c.FillText("ab", 50, 20)
		m := c.MeasureText("ab")
		log = append(log, fmt.Sprintf("%g %g %v %.3f %.0f", c.Font.Size, c.Font.Weight, c.Font.Italic, m.Width, m.ActualBoundingBoxLeft))
	}, Class("w-40 h-10"))
	x := newWinHarness(t, Options{Root: Component(func() Node {
		c, sc := UseState("")
		d, sd := UseState("ltr")
		class, dir = sc, sd
		return div(Dir(d), div(Class("text-base "+c), node))
	})})
	step := func(what string, change func(), want int) string {
		t.Helper()
		log = nil
		change()
		x.drain()
		if len(log) != want {
			t.Errorf("%s: %d draws %v, want %d", what, len(log), log, want)
		}
		if len(log) > 0 {
			return log[len(log)-1]
		}
		return ""
	}
	first := step("rebuilt", func() { class.Set(" ") }, 0)
	big := step("a size", func() { class.Set("text-2xl") }, 1)
	if big == first || !strings.HasPrefix(big, "24 ") {
		t.Errorf("a size: %s", big)
	}
	step("a background", func() { class.Set("text-2xl bg-red-500") }, 0)
	step("an alignment", func() { class.Set("text-2xl text-center underline uppercase leading-9") }, 0)
	step("a weight", func() { class.Set("text-2xl font-bold") }, 1)
	step("a slant", func() { class.Set("text-2xl italic") }, 1)
	step("a letter spacing", func() { class.Set("text-2xl tracking-widest") }, 1)
	step("tabular numbers", func() { class.Set("text-2xl tabular-nums") }, 1)
	step("a family", func() { class.Set("text-2xl font-mono") }, 1)
	rtl := step("a direction", func() { dir.Set("rtl") }, 1)
	if !strings.HasSuffix(rtl, " 27") { // the point is to the right of two monospace characters
		t.Errorf("right to left: %s", rtl)
	}
	scaled := step("the text scale", func() { x.b.SetSettings(platform.SystemSettings{TextScale: 2}) }, 1)
	if !strings.HasPrefix(scaled, "48 ") {
		t.Errorf("the text scale: %s", scaled)
	}
	x.expectIdle("at the end")
}

// Fonts that the same styles resolve to otherwise (the desktop's) are drawn.
func TestCanvasFontsReshaped(t *testing.T) {
	fonts := stdfonts.NewCollection()
	var widths []float64
	draw := func(c *canvas.Canvas) {
		c.FillText("iiiiiiiiii", 5, 20)
		widths = append(widths, c.MeasureText("iiiiiiiiii").Width)
	}
	tr := NewTree(Config{Fonts: fonts})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.Mount(div(canvasNode(draw, Class("w-40 h-10")), canvasNode(draw, Class("w-40 h-10 hidden"))))
	tr.Frame()
	fonts.SetGeneric("ui-sans-serif", "JetBrains Mono")
	tr.reshapeText()
	tr.Frame()
	if len(widths) != 2 || widths[1] <= widths[0] {
		t.Errorf("widths %v", widths)
	}
	tr.reshapeText()
	before := len(widths)
	if _, changed := tr.Frame(); changed && len(widths) == before {
		t.Error("the fonts are the same, and the frame changed")
	}
}

// The theme's fonts are the canvas's.
func TestCanvasThemeFont(t *testing.T) {
	var got []string
	tr := newTestTree(t, 400, 300, canvasNode(func(c *canvas.Canvas) { got = append(got, c.Font.Family) }), nil)
	tr.Frame()
	tr.SetTheme(Theme{FontSans: "JetBrains Mono"})
	tr.Frame()
	if len(got) != 2 || !strings.HasPrefix(got[1], "JetBrains Mono, ") {
		t.Errorf("families %q", got)
	}
}

// An image is converted once while a canvas draws it, or keeps it on screen,
// and released when none does.
func TestCanvasImageReleased(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	img := image.NewRGBA(image.Rect(0, 0, 1100, 1000)) // more than the keep list holds
	if len(img.Pix) <= keepBytes {
		t.Fatal("the test image must be larger than keepBytes")
	}
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	k := convKey{img}
	var on *Value[bool]
	var more *Value[float64]
	var made []*gfx.Image
	draw := func(c *canvas.Canvas) {
		if on.Get() {
			c.DrawImageSize(img, 0, 0, 100, 50)
		}
		c.FillRect(more.Get(), 0, 5, 5)
	}
	tr := newTestTree(t, 400, 300, Component(func() Node {
		on, more = UseValue(true), UseValue(0.0)
		return canvasNode(draw, Class("w-40 h-10"))
	}), nil)
	tr.Frame()
	for i := range 3 {
		if s := cachedSlot(k); s != nil {
			if a := s.weak.Value(); a != nil {
				made = append(made, a.raster)
			}
		}
		if collectUntil(gone(k)) {
			t.Fatal("released while the canvas shows it")
		}
		more.Set(float64(i + 1))
		tr.Frame()
	}
	if len(made) != 3 || made[0] != made[2] || totalDecodes() != 1 {
		t.Errorf("%d images, %d conversions", len(made), totalDecodes())
	}
	made = nil
	on.Set(false)
	tr.Frame()
	tr.Frame() // the frame that drew it is not the last one
	if !collectUntil(gone(k)) {
		t.Error("kept after a draw without it")
	}
	on.Set(true)
	tr.Frame()
	if cachedSlot(k) == nil {
		t.Fatal("not converted again")
	}
	tr.Unmount()
	if !collectUntil(gone(k)) {
		t.Error("kept after the unmount")
	}
}

// Icons are parsed once, drawn in the fill style, and one that cannot be
// parsed is reported once.
func TestCanvasIcon(t *testing.T) {
	var logs []string
	draw := func(c *canvas.Canvas) {
		c.FillStyle = gfx.Color{R: 1, A: 1}
		c.DrawIcon(canvas.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path fill="currentColor" d="M0 0h10v10H0z"/></svg>`}, 0, 0, 20, 20)
		c.DrawIcon(canvas.Icon{SVG: "<svg"}, 20, 0, 20, 20)
	}
	_, img := Render(div(canvasNode(draw), canvasNode(draw)), RenderOptions{Config: Config{Fonts: testFonts, Log: func(m string) { logs = append(logs, m) }}, Width: 400, Height: 300})
	if p := img.RGBAAt(10, 10); p != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("the icon is %v", p)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "Canvas.DrawIcon") {
		t.Errorf("logs %q", logs)
	}
}

// A draw that erases does not erase what is behind the canvas.
func TestCanvasErases(t *testing.T) {
	_, img := Render(div(Class("bg-[#00f] size-full"), canvasNode(func(c *canvas.Canvas) {
		c.FillStyle = gfx.Color{R: 1, A: 1}
		flood(c)
		c.GlobalCompositeOperation = canvas.DestinationOut
		c.FillRect(0, 0, 20, 20)
		c.GlobalCompositeOperation = canvas.SourceOver
		c.ClearRect(20, 0, 20, 20)
	}, Style(style.TranslateFn(func() (float32, float32) { return 0, 0 })))), RenderOptions{Config: Config{Fonts: testFonts}, Width: 400, Height: 300})
	for x, want := range map[int]color.RGBA{10: {0, 0, 255, 255}, 30: {0, 0, 255, 255}, 50: {255, 0, 0, 255}} {
		if p := img.RGBAAt(x, 10); p != want {
			t.Errorf("at %d: %v, want %v", x, p, want)
		}
	}
}
