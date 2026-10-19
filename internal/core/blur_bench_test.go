package core

import (
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/raster"
)

// glassPage is a 1920x1080 page: a gradient with 24 paragraphs of white
// text, a 200x200 tile whose color follows a state (set receives its
// setter), and a full-width glass panel over both (backdrop-blur-xl,
// bg-white/20, a border on each side).
func glassPage(set *func(bool)) Node {
	return Component(func() Node {
		on, setOn := UseStateSet(false)
		*set = setOn
		tile := "bg-sky-400"
		if on {
			tile = "bg-amber-400"
		}
		text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 8)
		paras := make([]Child, 0, 25)
		paras = append(paras, Class("absolute inset-0 flex flex-col gap-2 p-8 text-white"))
		for i := range 24 {
			paras = append(paras, p(Key(i), Text(text)))
		}
		return div(Class("relative h-screen overflow-hidden bg-linear-to-br from-indigo-500 via-fuchsia-500 to-rose-500"),
			div(paras...),
			div(Class("absolute top-[440px] left-[860px] size-[200px] rounded-2xl "+tile)),
			div(Class("absolute inset-x-0 top-[340px] h-[400px] border-y border-white/40 bg-white/20 backdrop-blur-xl")),
		)
	})
}

// BenchmarkGlassFrame1080p is the glass page at 1920x1080, scale 1: a full
// rasterization (full) and the frame after the tile under the panel
// changed color (tile: build, layout, paint, damage and the rasterization
// of the damage).
func BenchmarkGlassFrame1080p(b *testing.B) {
	for _, full := range []bool{true, false} {
		name := "tile"
		if full {
			name = "full"
		}
		b.Run(name, func(b *testing.B) {
			var set func(bool)
			tr := newTestTree(b, 1920, 1080, glassPage(&set), nil)
			tr.Frame()
			fb := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			i := 0
			var px int
			b.ReportAllocs()
			for b.Loop() {
				i++
				if !full {
					set(i&1 == 1)
					tr.Frame()
				}
				d := tr.RasterizeFrame(fb, &r, full)
				tr.FlushEffects()
				if !full && d.Full {
					b.Fatal("a tile change rasterized the whole frame")
				}
				px = 0
				for _, rc := range d.Rects {
					px += rc.Dx() * rc.Dy()
				}
			}
			if !full {
				b.ReportMetric(float64(px), "px/op")
			}
		})
	}
}

// TestGlassDamage checks the incremental frames of the glass page against
// full rasterizations.
func TestGlassDamage(t *testing.T) {
	var set func(bool)
	tr := newTestTree(t, 1920, 1080, glassPage(&set), nil)
	tr.Frame()
	fb := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	ref := image.NewRGBA(fb.Rect)
	var r raster.Renderer
	tr.RasterizeFrame(fb, &r, true)
	for i := range 3 {
		set(i%2 == 0)
		tr.Frame()
		d := tr.RasterizeFrame(fb, &r, false)
		if d.Full {
			t.Fatalf("frame %d: a tile change rasterized the whole frame", i)
		}
		raster.Clear(ref, image.Rectangle{}, tr.CanvasColor())
		raster.Render(ref, tr.LastFrame(), raster.Options{Scale: 1})
		if string(ref.Pix) != string(fb.Pix) {
			t.Fatalf("frame %d: the damage %v gives other pixels than a full rasterization", i, d.Rects)
		}
		t.Logf("frame %d: damage %v", i, d.Rects)
	}
}

// glassStickyScene: backdrop filters with the features damage treats
// specially: a sticky glass header over a scrolling page, a glass card in a
// blend group (a layer: rendered on one goroutine), text clipped to a
// gradient (a masked layer), state changes under the glass in a scroll
// container, and a horizontal strip with a sticky glass cell (ADR-078).
func glassStickyScene(st *sceneState) Node {
	item := func(i int) Node {
		v, set := UseState(0)
		st.setters[i] = set
		cls := "mx-3 my-2 h-10 rounded-lg px-3 text-sm"
		switch v % 3 {
		case 1:
			cls += " bg-amber-300"
		case 2:
			cls += " bg-sky-400"
		default:
			cls += " bg-white/40"
		}
		return div(Class(cls), Textf("Item %d", i))
	}
	comp := func() Node {
		st.refs[0], st.refs[1] = UseElementRef(), UseElementRef()
		items := []Child{AttachRef(st.refs[0]), Key("list"), Class("relative mx-3 h-48 overflow-y-auto rounded-xl"),
			div(Class("sticky top-0 z-10 h-8 bg-white/30 px-2 text-xs backdrop-blur-md"), Text("List header"))}
		for i := range 16 {
			items = append(items, WithKey(ComponentProps(item, i), i))
		}
		strip := []Child{AttachRef(st.refs[1]), Key("strip"), Class("mx-3 mt-3 flex overflow-x-auto rounded-lg bg-linear-to-r from-amber-300 to-emerald-400"),
			div(Class("sticky left-0 z-10 w-20 shrink-0 bg-white/20 p-2 text-xs backdrop-blur-lg"), Text("Name"))}
		for i := range 8 {
			strip = append(strip, div(Key(i), Class("h-12 w-24 shrink-0 border-r border-white/50 p-2 text-xs"), Textf("Col %d", i)))
		}
		return div(Class("relative bg-linear-to-br from-indigo-500 via-fuchsia-500 to-rose-400 pb-[300px]"),
			div(Class("sticky top-0 z-20 h-10 border-b border-white/40 bg-white/20 px-3 text-white backdrop-blur-lg"), Text("Glass header")),
			div(Class("m-3 rounded-xl bg-amber-200 p-2 mix-blend-multiply"),
				div(Class("h-12 rounded-lg bg-white/30 p-2 backdrop-blur-sm"), Text("In a blend group"))),
			p(Class("mx-3 bg-linear-to-r from-sky-300 to-emerald-300 bg-clip-text text-2xl font-bold text-transparent"), Text("Clipped text")),
			div(items...),
			div(strip...),
		)
	}
	return Component(comp)
}

// TestDamageGlassSticky: incremental frames of glassStickyScene are
// bit-identical to full rasterizations at every damage scale.
func TestDamageGlassSticky(t *testing.T) {
	steps := 120
	if testing.Short() {
		steps = 30
	}
	runScene(t, "glass_sticky", glassStickyScene, commonOps, Config{InstantScroll: true}, steps)
}
