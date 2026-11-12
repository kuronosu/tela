package cocoa

import (
	"bytes"
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// chainSim drives a surfaceChain the way writeSurface does, with BGRA
// buffers in place of IOSurfaces.
type chainSim struct {
	c     surfaceChain
	back  *image.RGBA
	bufs  map[uintptr][]byte // BGRA, stride 4*width
	next  uintptr
	busy  map[uintptr]bool
	front uintptr
}

func newChainSim(size image.Point) *chainSim {
	return &chainSim{back: image.NewRGBA(image.Rectangle{Max: size}), bufs: map[uintptr][]byte{}, busy: map[uintptr]bool{}}
}

// present changes rects of the retained frame to fresh colors and shows
// a surface, returning its ref.
func (s *chainSim) present(t *testing.T, rnd *rand.Rand, rects []image.Rectangle) uintptr {
	t.Helper()
	size := s.back.Rect.Size()
	for _, r := range rects {
		v := byte(rnd.IntN(256))
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				i := s.back.PixOffset(x, y)
				copy(s.back.Pix[i:i+4], []byte{v, byte(x), byte(y), 255})
			}
		}
	}
	c := &s.c
	if c.size != size || len(c.surfs) == 0 {
		for _, ref := range c.resize(size) {
			delete(s.bufs, ref)
		}
	} else {
		c.damage(rects)
	}
	i := c.next(func(ref uintptr) bool { return s.busy[ref] })
	if i < 0 {
		s.next++
		s.bufs[s.next] = make([]byte, 4*size.X*size.Y)
		i = c.add(s.next)
	}
	ref := c.surfs[i].ref
	if ref == s.front {
		t.Fatalf("the surface shown (%d) was chosen to be written", ref)
	}
	if s.busy[ref] && len(c.surfs) < maxSurfaces {
		t.Fatalf("a busy surface was written while the chain could grow")
	}
	copied := 0
	for _, r := range c.stale(i) {
		platform.CopyRGBAToBGRA(s.bufs[ref], 4*size.X, s.back, r)
		copied += 4 * r.Dx() * r.Dy()
	}
	c.shownAt(i, copied)
	s.front = ref
	want := make([]byte, 4*size.X*size.Y)
	platform.CopyRGBAToBGRA(want, 4*size.X, s.back, s.back.Rect)
	if !bytes.Equal(s.bufs[ref], want) {
		t.Fatalf("the surface shown differs from the retained frame")
	}
	return ref
}

// TestSurfaceChain presents random damages through a chain whose surfaces
// are sometimes still in use, with resizes, and checks that the surface
// shown always holds the whole retained frame, that the one shown is
// never written, and that the chain stays within maxSurfaces.
func TestSurfaceChain(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	size := image.Pt(37, 23)
	s := newChainSim(size)
	full := []image.Rectangle{{Max: size}}
	s.present(t, rnd, full)
	for frame := range 2000 {
		if frame%500 == 499 {
			size = image.Pt(20+rnd.IntN(30), 10+rnd.IntN(30))
			old := s.back
			s.back = image.NewRGBA(image.Rectangle{Max: size})
			platform.CopyRGBA(s.back, old, s.back.Rect.Intersect(old.Rect))
			s.present(t, rnd, []image.Rectangle{{Max: size}})
			continue
		}
		var rects []image.Rectangle
		for range rnd.IntN(4) + 1 {
			x, y := rnd.IntN(size.X), rnd.IntN(size.Y)
			rects = append(rects, image.Rect(x, y, x+1+rnd.IntN(8), y+1+rnd.IntN(8)).Intersect(image.Rectangle{Max: size}))
		}
		// The window server holds the surface shown and, at times, the
		// one shown before it.
		prev := s.front
		ref := s.present(t, rnd, rects)
		clear(s.busy)
		s.busy[ref] = true
		if rnd.IntN(3) == 0 {
			s.busy[prev] = true
		}
		if len(s.c.surfs) > maxSurfaces {
			t.Fatalf("frame %d: %d surfaces", frame, len(s.c.surfs))
		}
	}
	t.Logf("%d surfaces created, %d bytes copied in all (a full frame is %d bytes)", s.c.created, s.c.totalCopied, 4*size.X*size.Y)
}

// TestSurfaceChainCopiesOnlyDamage: once two surfaces exist and the
// window server lets them go, a frame copies its own damage and the one
// of the frame before.
func TestSurfaceChainCopiesOnlyDamage(t *testing.T) {
	rnd := rand.New(rand.NewPCG(3, 4))
	size := image.Pt(100, 80)
	s := newChainSim(size)
	s.present(t, rnd, []image.Rectangle{{Max: size}})
	s.present(t, rnd, []image.Rectangle{image.Rect(0, 0, 10, 10)})   // the second surface: full
	s.present(t, rnd, []image.Rectangle{image.Rect(20, 20, 25, 25)}) // back to the first
	if got, want := s.c.lastCopied, 4*(100+25); got != want {
		t.Errorf("copied %d bytes, want %d (this damage and the last)", got, want)
	}
	// Inside the damage the second surface missed: copied once.
	s.present(t, rnd, []image.Rectangle{image.Rect(20, 20, 22, 22)})
	if got, want := s.c.lastCopied, 4*25; got != want {
		t.Errorf("copied %d bytes, want %d", got, want)
	}
	if s.c.created != 2 {
		t.Errorf("%d surfaces created, want 2", s.c.created)
	}
}

func TestAddStale(t *testing.T) {
	r := func(x0, y0, x1, y1 int) image.Rectangle { return image.Rect(x0, y0, x1, y1) }
	got := addStale(nil, []image.Rectangle{r(0, 0, 4, 4), r(1, 1, 2, 2), {}, r(10, 10, 12, 12)})
	if len(got) != 2 || got[0] != r(0, 0, 4, 4) || got[1] != r(10, 10, 12, 12) {
		t.Errorf("a rectangle inside another: %v", got)
	}
	got = addStale(got, []image.Rectangle{r(0, 0, 20, 20)})
	if len(got) != 1 || got[0] != r(0, 0, 20, 20) {
		t.Errorf("a rectangle over the others: %v", got)
	}
	got = addStale(got, []image.Rectangle{r(0, 0, 20, 20)})
	if len(got) != 1 {
		t.Errorf("the same rectangle again: %v", got)
	}
	var many []image.Rectangle
	for i := range maxStale + 1 {
		many = append(many, r(2*i, 0, 2*i+1, 1))
	}
	got = addStale(nil, many)
	if len(got) != 1 || got[0] != r(0, 0, 2*maxStale+1, 1) {
		t.Errorf("past maxStale: %v", got)
	}
}
