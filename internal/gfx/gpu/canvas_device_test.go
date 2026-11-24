//go:build telagpu && (linux || windows)

package gpu

import (
	"fmt"
	"image"
	"math/rand"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// operatorPage is the page of operatorPanes as a list.
func operatorPage(moved, label float32) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) { operatorPanes(b, moved, label) })
}

// TestOperatorsKeepPixels checks the operators at the root of lists drawn
// over what the target holds from earlier renders, whole and in a region,
// without Clear: they work on those pixels, and change none beyond.
func TestOperatorsKeepPixels(t *testing.T) {
	const w, h = 216, 216
	first := gfxtest.Build(func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 216, 150))
		b.FillRect(gfx.RectXYWH(0, 150, 216, 40), indigo.WithAlpha(0.5))
	})
	cells := func(kind int) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) { operatorCells(b, kind) })
	}
	unclipped := gfxtest.Build(func(b *gfx.Builder) {
		b.PushLayer(0.9, gfx.SrcIn)
		b.FillRect(gfx.RectXYWH(20, 20, 150, 170), photoPattern(gfx.Scale(1.3, 1.3)))
		b.PopLayer()
	})
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		cpu := image.NewRGBA(image.Rect(0, 0, w, h))
		for i, c := range []struct {
			dl     *gfx.DisplayList
			region image.Rectangle
		}{{first, image.Rectangle{}}, {cells(2), image.Rect(30, 30, 130, 100)}, {cells(0), image.Rect(60, 90, 200, 216)},
			{cells(5), image.Rect(0, 150, 216, 200)}, {unclipped, image.Rect(100, 40, 180, 180)}, {first, image.Rect(0, 0, 100, 100)}, {cells(1), image.Rectangle{}}} {
			before := readAll(t, target)
			res, err := r.Render(target, c.dl, Options{Region: c.region})
			if err != nil || res.Unsupported != nil {
				t.Fatal(err, res.Unsupported)
			}
			raster.Render(cpu, c.dl, raster.Options{Region: c.region})
			gpu := readAll(t, target)
			m := compare(gpu, cpu)
			dump(t, fmt.Sprintf("keeps_operators_%d_%s", i, deviceLabel(dev.Name())), gpu, cpu)
			t.Logf("after render %d: largest difference %d, %.3f%% of the pixels over 1 level, %.3f%% over 2", i+1, m.Max, 100*m.Over1, 100*m.Over2)
			// The differences of a level of each render add up.
			if m.Max > tolHard || m.Over2 > 4*tolShare {
				t.Errorf("after render %d: largest difference %d at %v, %.3f%% of the pixels over 2 levels", i+1, m.Max, m.Worst, 100*m.Over2)
			}
			for y := 0; y < h && !c.region.Empty(); y++ {
				for x := 0; x < w; x++ {
					if p := image.Pt(x, y); !p.In(c.region) && gpu.RGBAAt(x, y) != before.RGBAAt(x, y) {
						t.Fatalf("after render %d: the pixel %v, outside the region %v, changed", i+1, p, c.region)
					}
				}
			}
		}
	})
}

// randomCanvas records a list of shapes, patterns, clips, masks, filters,
// glass panes and layers with operators, nested up to three deep.
func randomCanvas(rng *rand.Rand, w, h float32) *gfx.DisplayList {
	f := func(lo, hi float32) float32 { return lo + rng.Float32()*(hi-lo) }
	colors := []gfx.Color{rose, amber, emerald, sky, indigo, fuchsia, slate, gfx.White, gfx.Black}
	color := func() gfx.Color {
		c := colors[rng.Intn(len(colors))]
		if rng.Intn(2) == 0 {
			c = c.WithAlpha(f(0.1, 1))
		}
		return c
	}
	box := func() gfx.Rect { return gfx.RectXYWH(f(-20, w-20), f(-20, h-20), f(8, w/2), f(8, h/2)) }
	paint := func() gfx.Paint {
		switch rng.Intn(5) {
		case 0:
			return gfx.LinearGradient{Start: gfx.Pt(f(0, w), f(0, h)), End: gfx.Pt(f(0, w), f(0, h)), Stops: stops(color(), color())}
		case 1:
			return tilePattern(gfx.PatternRepeat(rng.Intn(4)), gfx.Sampling(rng.Intn(2)),
				gfx.Translate(f(0, w), f(0, h)).Mul(gfx.Rotate(f(0, 6))).Mul(gfx.Scale(f(0.3, 6), f(0.3, 6))))
		case 2:
			return photoPattern(gfx.Affine{f(-1, 1), f(-0.3, 0.3), f(-0.3, 0.3), f(0.1, 1), f(0, w), f(0, h)})
		}
		return color()
	}
	shape := func(b *gfx.Builder) {
		r := box()
		switch rng.Intn(6) {
		case 0:
			b.FillRRect(gfx.NewRRect(r, f(2, 20)), paint())
		case 1:
			b.FillPath(gfxtest.StarPath(f(0, w), f(0, h), f(10, 50)), gfx.NonZero, paint())
		case 2:
			b.StrokePath(zigzag(r.X0, r.Y0, 10, 20, 5), gfx.NewStrokeStyle(f(1, 8)), paint())
		case 3:
			text(b, "SoA*l", f(10, 40), r.X0, r.Y1, paint())
		case 4:
			b.FillRect(gfx.RectXYWH(float32(int(r.X0)), float32(int(r.Y0)), float32(int(r.Width())), float32(int(r.Height()))), paint())
		default:
			b.FillRect(r, paint())
		}
	}
	var draw func(b *gfx.Builder, depth, n int)
	draw = func(b *gfx.Builder, depth, n int) {
		for range n {
			k := rng.Intn(10)
			switch {
			case depth == 3 || k < 4:
				shape(b)
			case k < 6:
				pop := pushClipKind(b, rng.Intn(clipKinds), float32(int(f(0, w-72))), float32(int(f(0, h-72))))
				draw(b, depth+1, 1+rng.Intn(3))
				pop()
			case k == 6 && depth == 0:
				b.BackdropFilter(gfx.NewRRect(box(), 10), gfx.Blur{StdDev: f(1, 8)})
			default:
				m := operators[rng.Intn(len(operators))]
				if rng.Intn(4) == 0 {
					m = []gfx.BlendMode{gfx.SrcOver, gfx.Multiply, gfx.PlusLighter}[rng.Intn(3)]
				}
				pop := func() {}
				if m.Unbounded() && rng.Intn(16) != 0 {
					// Without a clip it leaves nothing else of the list.
					pop = pushClipKind(b, rng.Intn(clipKinds), float32(int(f(0, w-72))), float32(int(f(0, h-72))))
				}
				op := []float32{0, 0.3, 0.8, 1, 1, 1}[rng.Intn(6)]
				var filters []gfx.Filter
				if rng.Intn(4) == 0 {
					filters = append(filters, []gfx.Filter{gfx.Blur{StdDev: f(1, 5)}, gfx.Saturate(1.4), gfx.DropShadow{Offset: gfx.Pt(3, 4), StdDev: 2, Color: gfx.Black}}[rng.Intn(3)])
				}
				if rng.Intn(5) == 0 {
					b.PushMaskLayer(maskOf(gfx.MaskMode(rng.Intn(2)), func(mb *gfx.Builder) { draw(mb, 2, 2) }), op, m, filters...)
				} else {
					b.PushLayer(op, m, filters...)
				}
				draw(b, depth+1, rng.Intn(4))
				b.PopLayer()
				pop()
			}
		}
	}
	b := gfx.NewBuilder()
	b.FillRect(gfx.RectXYWH(0, 0, w, h), color())
	draw(b, 0, 24)
	return b.Finish()
}

// TestCanvasRandom renders random lists of what Canvas draws, and compares
// them with the rasterizer's; and renders of regions of them with the full
// render, bit for bit.
func TestCanvasRandom(t *testing.T) {
	const w, h, lists = 240, 180, 40
	eachDevice(t, func(t *testing.T, dev Device) {
		worst, share := 0, 0.0
		for seed := range lists {
			rng := rand.New(rand.NewSource(int64(seed)))
			dl := randomCanvas(rng, w, h)
			scale := []float32{1, 1.5, 2}[seed%3]
			pw, ph := int(w*scale), int(h*scale)
			clear := []gfx.Color{gfx.White, gfx.Transparent}[seed/3%2]
			gpu, res := renderGPU(t, dev, pw, ph, dl, Options{Scale: scale, Clear: &clear})
			if res.Unsupported != nil {
				t.Fatalf("list %d: not drawn: %v", seed, res.Unsupported)
			}
			cpu := renderCPU(pw, ph, dl, scale, clear)
			m := compare(gpu, cpu)
			over := m.OverShare(gpu, cpu, tolLevels)
			worst, share = max(worst, m.Max), max(share, over)
			if m.Max > tolHard || over > tolShare {
				dump(t, fmt.Sprintf("canvas_%d_%s", seed, deviceLabel(dev.Name())), gpu, cpu)
				t.Errorf("list %d (x%g): largest difference %d at %v, %.3f%% of the pixels over %d levels", seed, scale, m.Max, m.Worst, 100*over, tolLevels)
			}
			// Regions.
			var r Renderer
			target, err := dev.NewTarget(pw, ph)
			if err != nil {
				t.Fatal(err)
			}
			for range 4 {
				x, y := rng.Intn(pw), rng.Intn(ph)
				region := image.Rect(x, y, x+1+rng.Intn(120), y+1+rng.Intn(90))
				sentinel := gfx.RGB8(1, 2, 3)
				if _, err := r.Render(target, nil, Options{Clear: &sentinel}); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Render(target, dl, Options{Scale: scale, Region: region, Clear: &clear}); err != nil {
					t.Fatal(err)
				}
				got := readAll(t, target)
				bad := 0
				for y := range ph {
					for x := range pw {
						want := gpu.RGBAAt(x, y)
						if !image.Pt(x, y).In(region) {
							want.R, want.G, want.B, want.A = 1, 2, 3, 255
						}
						if got.RGBAAt(x, y) != want {
							if bad++; bad == 1 {
								t.Errorf("list %d (x%g), region %v: the pixel (%d, %d) is %v, want %v", seed, scale, region, x, y, got.RGBAAt(x, y), want)
							}
						}
					}
				}
			}
			r.Trim()
			target.Close()
		}
		t.Logf("%d lists: largest difference %d, at most %.3f%% of the pixels of one over %d levels", lists, worst, 100*share, tolLevels)
	})
}
