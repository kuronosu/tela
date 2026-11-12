package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The bases of damaged rectangles (damagebase.go, ADR-275): a rectangle
// whose pixels differ only from some list position on is rendered from
// that position, over the framebuffer or over a snapshot of the previous
// frame, and its pixels are those of a full rasterization.

// paneScrollPage is a 600x400 page like the dashboard's orders view:
// stripes under a gradient, a fixed glass pane (backdrop-blur-2xl) over
// most of the window, and in it a scroll container of translucent rows
// that fills the pane, so that a scroll moves most of the window over the
// blurred backdrop.
func paneScrollPage(ref *ElementRef) Node {
	return Component(func() Node {
		*ref = UseElementRef()
		stripes := []Child{Class("absolute inset-0 flex")}
		for i := range 60 {
			c := "bg-slate-900"
			if i%2 == 1 {
				c = "bg-amber-300"
			}
			stripes = append(stripes, div(Key(i), Class("h-full w-[10px] shrink-0 "+c)))
		}
		rows := []Child{AttachRef(*ref), Key("rows"), Class("h-full overflow-y-auto p-3")}
		for i := range 40 {
			rows = append(rows, div(Key(i), Class("mb-2 flex h-10 items-center gap-3 rounded-lg border border-white/30 bg-white/10 px-3 text-sm text-white"),
				div(Class("size-5 rounded-full bg-sky-400/80")), Textf("Order %d", 1000+i)))
		}
		return div(Class("relative h-screen overflow-hidden bg-linear-to-br from-indigo-500 to-rose-500"),
			div(stripes...),
			div(Class("absolute inset-[16px] rounded-2xl border border-white/40 bg-white/20 backdrop-blur-2xl"), div(rows...)),
		)
	})
}

// checkFull compares fb with a full rasterization of the tree's frame.
func checkFull(t *testing.T, tr *Tree, fb *image.RGBA, scale float32, what string, d Damage) {
	t.Helper()
	ref := image.NewRGBA(fb.Rect)
	raster.Clear(ref, image.Rectangle{}, tr.CanvasColor())
	raster.Render(ref, tr.LastFrame(), raster.Options{Scale: scale})
	if p, ok := firstDiff(fb, ref); !ok {
		t.Fatalf("%s: the damage %v (plans %+v) gives other pixels than a full rasterization at %v: %v, want %v",
			what, d.Rects, d.plans, p, fb.RGBAAt(p.X, p.Y), ref.RGBAAt(p.X, p.Y))
	}
}

// TestScrollOverGlassFromSnapshot scrolls rows that fill a glass pane: the
// rows are translucent over the blurred backdrop, so their pixels cannot
// be shifted, and they cover most of the window. The first scroll frame
// takes a snapshot of the pixels under the rows (the wallpaper, the pane's
// blur and fill); the next ones render the rows over it, not the whole
// frame, and match a full rasterization.
func TestScrollOverGlassFromSnapshot(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var ref ElementRef
			tr := newTestTree(t, 600, 400, paneScrollPage(&ref), nil)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(600*scale+0.5), int(400*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			based := 0
			for n := 1; n <= 6; n++ {
				ref.ScrollBy(0, 13)
				tr.Frame()
				d := tr.RasterizeFrame(fb, &r, false)
				checkFull(t, tr, fb, scale, fmt.Sprintf("step %d", n), d)
				if n == 1 {
					continue // no snapshot yet
				}
				if d.Full {
					t.Fatalf("step %d: the scroll rasterized the whole frame", n)
				}
				for k, pl := range d.plans {
					if pl.base >= 0 && d.Rects[k].Dx()*d.Rects[k].Dy() > w*h/2 {
						based++
					}
				}
			}
			if based < 4 {
				t.Errorf("%d scroll frames rendered the rows over a snapshot, want 5", based)
			}
		})
	}
}

// overlayPage is the glass page under an optional scrim and dialog, like
// the dashboard's palette opening: the scrim covers the window and is
// painted after the page, and the dialog scales in.
func overlayPage(open *bool, grow *int) Node {
	return Component(func() Node {
		var ref ElementRef
		page := paneScrollPage(&ref)
		if !*open {
			return div(Class("relative h-screen"), page)
		}
		if *grow == 0 {
			return div(Class("relative h-screen"), page, div(Class("absolute inset-0 bg-black/50")))
		}
		size := 200 + 8**grow
		return div(Class("relative h-screen"), page,
			div(Class("absolute inset-0 bg-black/50")),
			div(Class(fmt.Sprintf("absolute left-[150px] top-[80px] h-[%dpx] w-[%dpx] rounded-xl border border-white/30 bg-white/30 backdrop-blur-md", size, size))))
	})
}

// TestOverlayFromFramebuffer opens a scrim over the page: the page's
// pixels are in the framebuffer, so the frame draws the scrim over them
// instead of rasterizing the whole window; then a dialog grows over the
// scrim, each frame drawn over the snapshot of the page and scrim that
// the frame before took.
func TestOverlayFromFramebuffer(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			open, grow := false, 0
			var set func(int)
			root := Component(func() Node {
				v, s := UseStateSet(0)
				set = s
				_ = v
				return overlayPage(&open, &grow)
			})
			tr := newTestTree(t, 600, 400, root, nil)
			tr.SetScale(scale)
			tr.Frame()
			w, h := int(600*scale+0.5), int(400*scale+0.5)
			fb := image.NewRGBA(image.Rect(0, 0, w, h))
			var r raster.Renderer
			tr.RasterizeFrame(fb, &r, true)
			open = true
			set(-1)
			tr.Frame()
			d := tr.RasterizeFrame(fb, &r, false)
			checkFull(t, tr, fb, scale, "scrim", d)
			if d.Full || len(d.plans) == 0 || d.plans[0].base != baseFB {
				t.Fatalf("the scrim's frame: full %v, plans %+v; want the scrim drawn over the framebuffer", d.Full, d.plans)
			}
			snapped := 0
			for n := 1; n <= 6; n++ {
				grow = n
				set(n)
				tr.Frame()
				d := tr.RasterizeFrame(fb, &r, false)
				checkFull(t, tr, fb, scale, fmt.Sprintf("dialog %d", n), d)
				if d.Full {
					t.Fatalf("dialog %d: a full frame", n)
				}
				if n == 1 && (len(d.plans) != 1 || d.plans[0].base != baseFB) {
					t.Errorf("the dialog's first frame: plans %+v; want it drawn over the framebuffer", d.plans)
				}
				for _, pl := range d.plans {
					if pl.base >= 0 {
						snapped++
					}
				}
			}
			if snapped < 3 {
				t.Errorf("%d dialog frames drawn over a snapshot, want at least 3 of 6", snapped)
			}
		})
	}
}

// TestSnapshotsTrimmed: the snapshots are memory the next frame may use;
// a window at rest drops them (TrimRaster), and one with nothing worth
// snapshotting (solid boxes) takes none.
func TestSnapshotsTrimmed(t *testing.T) {
	var ref ElementRef
	tr := newTestTree(t, 600, 400, paneScrollPage(&ref), nil)
	tr.Frame()
	fb := image.NewRGBA(image.Rect(0, 0, 600, 400))
	var r raster.Renderer
	tr.RasterizeFrame(fb, &r, true)
	ref.ScrollBy(0, 13)
	tr.Frame()
	tr.RasterizeFrame(fb, &r, false)
	if tr.lay.snapBytes() == 0 {
		t.Fatal("the scroll over glass took no snapshot")
	}
	if !tr.RasterTrimDue(&r) {
		t.Error("a tree at rest holding snapshots asks for no trim")
	}
	tr.TrimRaster(&r)
	if n := tr.lay.snapBytes(); n != 0 {
		t.Errorf("after TrimRaster the snapshots hold %d bytes", n)
	}

	var ref2 ElementRef
	plain := Component(func() Node {
		ref2 = UseElementRef()
		rows := []Child{AttachRef(ref2), Class("h-screen overflow-y-auto bg-white")}
		for i := range 40 {
			rows = append(rows, div(Key(i), Class("m-2 h-10 rounded bg-slate-100"), Textf("Row %d", i)))
		}
		return div(rows...)
	})
	tr2 := newTestTree(t, 600, 400, plain, nil)
	tr2.Frame()
	tr2.RasterizeFrame(fb, &r, true)
	for range 3 {
		ref2.ScrollBy(0, 7)
		tr2.Frame()
		tr2.RasterizeFrame(fb, &r, false)
	}
	if n := tr2.lay.snapBytes(); n != 0 {
		t.Errorf("a page of solid boxes holds %d bytes of snapshots", n)
	}
}
