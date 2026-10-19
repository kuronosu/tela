package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// The backdrop gallery: panels with a backdrop filter and an opacity over
// stripes, compared with Chromium by cssGallery. CSS draws the filtered
// backdrop as part of the element, so the element's opacity fades it, and
// an element at opacity 0 shows no backdrop filter at all (ADR-084).

// backdropStripes returns the 26 stripes, 8 px wide, of a panel's backdrop.
func backdropStripes(a, b string) []inode {
	out := make([]inode, 26)
	for i := range out {
		c := a
		if i%2 == 1 {
			c = b
		}
		out[i] = iel("div", "h-full w-[8px] shrink-0 "+c)
	}
	return out
}

// backdropRow is a row with a panel of class panel over stripes a and b.
func backdropRow(name, panel, a, b string) cssRow {
	kids := append(backdropStripes(a, b), iel("div", "absolute left-[16px] top-[16px] h-[48px] w-[176px] "+panel))
	return cssRow{name: name, height: 96, kids: []inode{iel("div", "relative flex h-[80px] w-[208px]", kids...)}}
}

var backdropGallery = &cssGallery{
	name:  "backdrop",
	width: 240,
	mean:  [2]float64{0.5, 0.5},
	maxd:  [2]int{12, 12}, // the rounded panel's anti-aliased corners
	geom:  [2]float64{0.02, 0.02},
	rows: []cssRow{
		backdropRow("opaque", "backdrop-blur-md", "bg-[#000000]", "bg-[#ffffff]"),
		backdropRow("half", "opacity-50 backdrop-blur-md", "bg-[#000000]", "bg-[#ffffff]"),
		backdropRow("half-bg", "opacity-50 bg-white/20 backdrop-blur-md", "bg-[#000000]", "bg-[#ffffff]"),
		backdropRow("zero", "opacity-0 backdrop-blur-md", "bg-[#000000]", "bg-[#ffffff]"),
		backdropRow("quarter-rounded", "opacity-25 rounded-xl bg-sky-500/50 backdrop-blur-md", "bg-[#000000]", "bg-[#ffffff]"),
		backdropRow("half-grayscale", "opacity-50 backdrop-grayscale", "bg-[#ff0000]", "bg-[#0000ff]"),
		backdropRow("zero-grayscale", "opacity-0 backdrop-grayscale", "bg-[#ff0000]", "bg-[#0000ff]"),
	},
}

// TestChromiumBackdropOpacity compares the gallery with Chromium.
func TestChromiumBackdropOpacity(t *testing.T) { backdropGallery.compare(t) }

// TestBackdropOpacityZero: a panel at opacity 0 with a backdrop filter
// changes no pixel, and one at opacity 50 is half way between its
// backdrop and its blurred backdrop.
func TestBackdropOpacityZero(t *testing.T) {
	render := func(panel string, scale float32) *image.RGBA {
		kids := []Child{Class("relative flex h-[80px] w-[208px]")}
		for i := range 26 {
			c := "bg-[#000000]"
			if i%2 == 1 {
				c = "bg-[#ffffff]"
			}
			kids = append(kids, div(Class("h-full w-[8px] shrink-0 "+c)))
		}
		if panel != "" {
			kids = append(kids, div(Class("absolute left-[16px] top-[16px] h-[48px] w-[176px] "+panel)))
		}
		tr := newTestTree(t, 208, 80, div(kids...), nil)
		tr.SetScale(scale)
		dl, _ := tr.Frame()
		return tr.Rasterize(dl, scale)
	}
	for _, scale := range []float32{1, 2} {
		none := render("", scale)
		for _, p := range []string{"opacity-0 backdrop-blur-md", "opacity-0 backdrop-grayscale", "opacity-0 bg-white/20 backdrop-blur-md"} {
			if d := diffRegion(render(p, scale), none, none.Rect); d.max != 0 {
				t.Errorf("x%g %q changes pixels: max %d", scale, p, d.max)
			}
		}
		full, half := render("backdrop-blur-md", scale), render("opacity-50 backdrop-blur-md", scale)
		// The middle of a black stripe under the panel (x 84, y 40).
		x, y := int(84*scale), int(40*scale)
		f, h := full.RGBAAt(x, y), half.RGBAAt(x, y)
		if want := int(f.R) / 2; int(h.R) < want-1 || int(h.R) > want+1 {
			t.Errorf("x%g: opacity-50 over black %v, blurred %v: want half of it", scale, h, f)
		}
	}
}

// TestBackdropOpacityDamage: glass panels that fade in and out (an opacity
// transition, and an OpacityFn layer the compositor fades) over stripes
// and a moving box: every incremental frame equals a full rasterization,
// at integer and fractional scales.
func TestBackdropOpacityDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var fade *Value[float32]
			var set Setter[int]
			root := Component(func() Node {
				n, s := UseState(0)
				set = s
				fade = UseValue(float32(0))
				kids := []Child{Class("relative flex h-[160px] w-[300px]")}
				for i := range 30 {
					c := "bg-slate-900"
					if i%2 == 1 {
						c = "bg-amber-300"
					}
					kids = append(kids, div(Class("h-full w-[10px] shrink-0 "+c)))
				}
				kids = append(kids,
					div(Class("absolute top-[60px] size-10 rounded-full bg-rose-500"), Style(style.Translate(float32(8*n), 0))),
					div(Key("hover"), Class("absolute left-[20px] top-[20px] h-[60px] w-[120px] rounded-xl bg-white/20 backdrop-blur-md opacity-0 hover:opacity-100 transition-opacity duration-300")),
					div(Class("absolute left-[160px] top-[70px] h-[70px] w-[120px] rounded-lg bg-sky-500/30 backdrop-blur-sm"),
						Style(style.OpacityFn(func() float32 { return fade.Get() }))),
				)
				return div(kids...)
			})
			x := newDamageHarness(t, root, 300, 160, scale, Config{}, 7)
			x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 60, Y: 40}})
			for i := range 24 {
				x.frame(fmt.Sprintf("fade in %d", i))
				fade.Set(float32(i) / 23)
				if i%4 == 0 {
					set.Set(i / 4)
				}
			}
			x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 290, Y: 5}})
			for i := range 24 {
				x.frame(fmt.Sprintf("fade out %d", i))
				fade.Set(1 - float32(i)/23)
			}
			if x.partial == 0 {
				t.Error("no frame was rasterized partially")
			}
		})
	}
}
