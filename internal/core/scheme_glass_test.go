package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/raster"
)

// A color scheme switch under glass: the panes' tint and text change, the
// wallpaper under them does not (ADR-275, the theme criterion of phase 24).

// schemeGlassPage is a 600x400 page like the dashboard with one wallpaper
// for both schemes: a root whose background follows the scheme, hidden
// under an opaque wallpaper (a gradient and stripes) that does not, and
// three glass panes whose tint, border and text follow the scheme.
func schemeGlassPage() Node {
	return Component(func() Node {
		stripes := []Child{Class("absolute inset-0 flex")}
		for i := range 60 {
			c := "bg-slate-900"
			if i%2 == 1 {
				c = "bg-amber-300"
			}
			stripes = append(stripes, div(Key(i), Class("h-full w-[10px] shrink-0 "+c)))
		}
		pane := "absolute rounded-2xl border border-white/40 bg-white/40 p-3 text-sm text-slate-900 backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/40 dark:text-slate-100"
		return div(Class("relative h-screen overflow-hidden bg-white dark:bg-slate-950"),
			div(Class("absolute inset-0 bg-linear-to-br from-indigo-500 to-rose-500"), div(stripes...)),
			div(Class(pane+" left-[8px] top-[8px] h-[384px] w-[60px]")),
			div(Class(pane+" left-[76px] top-[8px] h-[384px] w-[380px]"), Text("Revenue 12,480")),
			div(Class(pane+" left-[464px] top-[8px] h-[384px] w-[128px]"), Text("Pulse")))
	})
}

// TestSchemeSwitchUnderGlass switches the scheme of a page at rest (its
// deferred trim ran): the frame is one frame, every pane's blur comes from
// the cache (no backdrop pixel is filtered), and its pixels are those of a
// full rasterization; a forced full frame filters every pane's backdrop.
func TestSchemeSwitchUnderGlass(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			tr := newTestTree(t, 600, 400, schemeGlassPage(), nil)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(600*scale+0.5), int(400*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			_, fullCost := tr.lay.bdc.Stats()
			// A frame with nothing new carries the panes' outputs (stable).
			tr.Frame()
			tr.TrimRaster(&r) // the window rests
			for n, s := range []ColorScheme{ColorSchemeDark, ColorSchemeLight, ColorSchemeDark} {
				tr.SetColorScheme(s)
				tr.Frame()
				if tr.Animating() || tr.Dirty() {
					t.Fatalf("switch %d: the switch is more than one frame", n)
				}
				h0, c0 := tr.lay.bdc.Stats()
				d := tr.RasterizeFrame(fb, &r, false)
				h1, c1 := tr.lay.bdc.Stats()
				checkFull(t, tr, fb, scale, fmt.Sprintf("switch %d", n), d)
				if h1-h0 != 3 || c1 != c0 {
					t.Errorf("switch %d: %d panes drawn from the cache, %d backdrop pixels filtered (a full frame filters %d); want 3 and none",
						n, h1-h0, c1-c0, fullCost)
				}
				tr.TrimRaster(&r)
			}
		})
	}
}

// TestOccludedDamage: a change under an opaque cover changes no pixel and
// damages nothing; under a cover in a translucent group, or a translucent
// cover, it does.
func TestOccludedDamage(t *testing.T) {
	for _, tc := range []struct {
		name, cover string
		hidden      bool
	}{
		{"opaque", "absolute inset-0 bg-linear-to-br from-indigo-500 to-rose-500", true},
		{"group", "absolute inset-0 bg-linear-to-br from-indigo-500 to-rose-500 opacity-90", false},
		{"translucent", "absolute inset-0 bg-indigo-500/90", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var set func(int)
			root := Component(func() Node {
				v, s := UseStateSet(0)
				set = s
				bg := "bg-white"
				if v == 1 {
					bg = "bg-slate-950"
				}
				return div(Class("relative h-screen "+bg), div(Class(tc.cover)),
					div(Class("absolute left-[20px] top-[20px] size-10 bg-amber-300")))
			})
			tr := newTestTree(t, 300, 200, root, nil)
			tr.Frame()
			fb := image.NewRGBA(image.Rect(0, 0, 300, 200))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			set(1)
			tr.Frame()
			d := tr.RasterizeFrame(fb, &r, false)
			checkFull(t, tr, fb, 1, tc.name, d)
			if hidden := !d.Full && len(d.Rects) == 0; hidden != tc.hidden {
				t.Errorf("the change under the cover: damage %v (full %v), want hidden %v", d.Rects, d.Full, tc.hidden)
			}
		})
	}
}

// BenchmarkSchemeSwitchUnderGlass is the rasterization of the frame of a
// scheme switch under glass ("switch") against a forced full frame of the
// same page ("full").
func BenchmarkSchemeSwitchUnderGlass(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		w, h := int(600*scale+0.5), int(400*scale+0.5)
		b.Run(fmt.Sprintf("switch_x%g", scale), func(b *testing.B) {
			tr := newTestTree(b, 600, 400, schemeGlassPage(), nil)
			tr.SetScale(scale)
			tr.Frame()
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			tr.Frame()
			s := ColorSchemeDark
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tr.TrimRaster(&r)
				tr.SetColorScheme(s)
				s = ColorSchemeDark + ColorSchemeLight - s
				tr.Frame()
				b.StartTimer()
				tr.RasterizeFrame(fb, &r, false)
			}
		})
		b.Run(fmt.Sprintf("full_x%g", scale), func(b *testing.B) {
			tr := newTestTree(b, 600, 400, schemeGlassPage(), nil)
			tr.SetScale(scale)
			tr.Frame()
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			for b.Loop() {
				tr.RasterizeFrame(fb, &r, true)
			}
		})
	}
}
