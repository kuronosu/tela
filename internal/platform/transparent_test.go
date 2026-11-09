package platform

import (
	"image"
	"image/color"
	"math/rand"
	"slices"
	"testing"
)

// maskFrame returns a w×h frame with alpha a over the rectangles rs.
func maskFrame(w, h int, a uint8, rs ...image.Rectangle) *image.RGBA {
	fb := image.NewRGBA(image.Rect(0, 0, w, h))
	for _, r := range rs {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				fb.SetRGBA(x, y, color.RGBA{A: a})
			}
		}
	}
	return fb
}

// naiveContains is the reference: the pixel's alpha.
func naiveContains(fb *image.RGBA, p image.Point) bool {
	return p.In(fb.Rect) && fb.Pix[fb.PixOffset(p.X, p.Y)+3] >= InputAlpha
}

func checkMask(t *testing.T, m *InputMask, fb *image.RGBA, limit int) {
	t.Helper()
	rects := m.Rects(limit)
	if len(rects) > limit {
		t.Fatalf("Rects(%d) returned %d rectangles", limit, len(rects))
	}
	exact := m.Rects(1 << 30)
	for y := 0; y < fb.Rect.Dy(); y++ {
		for x := 0; x < fb.Rect.Dx(); x++ {
			p := image.Pt(x, y)
			want := naiveContains(fb, p)
			if got := m.Contains(p); got != want {
				t.Fatalf("Contains(%v) = %v, want %v", p, got, want)
			}
			inExact := slices.ContainsFunc(exact, func(r image.Rectangle) bool { return p.In(r) })
			if inExact != want {
				t.Fatalf("exact Rects contain %v: %v, want %v (%v)", p, inExact, want, exact)
			}
			if want && !slices.ContainsFunc(rects, func(r image.Rectangle) bool { return p.In(r) }) {
				t.Fatalf("Rects(%d) miss %v", limit, p)
			}
		}
	}
}

func TestInputMaskFullAndPartial(t *testing.T) {
	fb := maskFrame(40, 30, 255, image.Rect(5, 5, 35, 25))
	var m InputMask
	if !m.Update(fb, []image.Rectangle{fb.Rect}) {
		t.Fatal("first Update reported no change")
	}
	if got, want := m.Rects(10), []image.Rectangle{image.Rect(5, 5, 35, 25)}; !slices.Equal(got, want) {
		t.Fatalf("Rects = %v, want %v", got, want)
	}
	// The same frame again changes nothing.
	if m.Update(fb, []image.Rectangle{fb.Rect}) {
		t.Fatal("an equal frame reported a change")
	}
	// A shadow below InputAlpha takes no input; a hole is punched.
	for y := 10; y < 12; y++ {
		for x := 10; x < 20; x++ {
			fb.SetRGBA(x, y, color.RGBA{A: InputAlpha - 1})
		}
	}
	if !m.Update(fb, []image.Rectangle{image.Rect(8, 9, 22, 13)}) {
		t.Fatal("a hole reported no change")
	}
	checkMask(t, &m, fb, 100)
	if got := len(m.Rects(100)); got != 4 {
		t.Fatalf("Rects = %v, want 4 bands (top, the hole's two sides, bottom)", m.Rects(100))
	}
	// A new size starts over.
	fb2 := maskFrame(10, 10, 200, image.Rect(0, 0, 3, 3))
	m.Update(fb2, []image.Rectangle{fb2.Rect})
	checkMask(t, &m, fb2, 10)
	if m.Size() != image.Pt(10, 10) {
		t.Fatalf("Size = %v", m.Size())
	}
}

// TestInputMaskRandomDamage checks incremental updates against a full
// scan, and the cover of Rects at small limits.
func TestInputMaskRandomDamage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	fb := maskFrame(37, 23, 0)
	var m InputMask
	m.Update(fb, []image.Rectangle{fb.Rect})
	for range 200 {
		var damage []image.Rectangle
		for range 1 + rng.Intn(3) {
			x0, y0 := rng.Intn(37), rng.Intn(23)
			r := image.Rect(x0, y0, x0+1+rng.Intn(15), y0+1+rng.Intn(10))
			damage = append(damage, r)
			a := uint8(rng.Intn(256))
			for y := r.Min.Y; y < min(r.Max.Y, 23); y++ {
				for x := r.Min.X; x < min(r.Max.X, 37); x++ {
					if rng.Intn(4) > 0 {
						fb.SetRGBA(x, y, color.RGBA{A: a})
					}
				}
			}
		}
		m.Update(fb, damage)
		checkMask(t, &m, fb, 1+rng.Intn(40))
	}
}

func TestInputMaskEmpty(t *testing.T) {
	var m InputMask
	if !m.Empty() || m.Contains(image.Pt(0, 0)) || len(m.Rects(4)) != 0 {
		t.Fatal("the zero mask is not empty")
	}
	fb := maskFrame(4, 4, 10, image.Rect(0, 0, 4, 4))
	m.Update(fb, []image.Rectangle{fb.Rect})
	if !m.Empty() {
		t.Fatal("pixels below InputAlpha take input")
	}
}

func TestResolveTransparentNeedsFrameless(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Transparent without Frameless did not panic")
		}
	}()
	WindowOptions{Transparent: true}.Resolve()
}

func TestResolveTransparentFrameless(t *testing.T) {
	o := WindowOptions{Transparent: true, Frameless: true}.Resolve()
	if !o.Transparent || !o.Frameless {
		t.Fatalf("Resolve dropped the options: %+v", o)
	}
}
