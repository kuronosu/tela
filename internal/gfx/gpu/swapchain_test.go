package gpu

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"slices"
	"testing"
)

// The swapchain's state machine (swapchain.go) against a fake surface and
// a fake presentation engine: what a window shows after every present
// must be the frame in the target, whatever lagged, was made again, went
// out of date or was minimized in between. The Vulkan side is exercised
// on Windows by core's live test (live_gpu_windows_test.go).

// memTarget is a Target in memory.
type memTarget struct{ img *image.RGBA }

func newMemTarget(w, h int) *memTarget { return &memTarget{img: image.NewRGBA(image.Rect(0, 0, w, h))} }

func (t *memTarget) Size() image.Point   { return t.img.Rect.Size() }
func (t *memTarget) Device() Device      { return nil }
func (t *memTarget) Draw(f *Frame) error { return errors.New("memTarget draws nothing") }
func (t *memTarget) Shift(image.Rectangle, int, int) error {
	return errors.New("memTarget shifts nothing")
}
func (t *memTarget) Close() {}
func (t *memTarget) ReadPixels(dst *image.RGBA, rects ...image.Rectangle) error {
	for _, r := range rects {
		draw.Draw(dst, r, t.img, r.Min, draw.Src)
	}
	return nil
}

// fakeSurface is a window's surface and its presentation engine. An image
// presented is shown and stays busy until hold more presents replaced it
// (1: the next present releases it, FIFO with one image waiting).
type fakeSurface struct {
	t      *testing.T
	extent image.Point
	min    int
	max    int
	incr   bool
	hold   int
	// images are the swapchain's, nil without one; busy the presents that
	// hold each, acquired the image the caller holds (-1: none).
	images   []*image.RGBA
	busy     []int
	acquired int
	// screen is what the window shows: the image presented last, whole.
	screen *image.RGBA
	// The answers of the next calls, when set.
	acquireStatus, presentStatus []SwapchainStatus
	// Counts and the arguments of the last present.
	creates, destroys, closes int
	copied                    int
	damage                    []image.Rectangle
	presents                  int
	closed                    bool
}

func newFakeSurface(t *testing.T, w, h int) *fakeSurface {
	return &fakeSurface{t: t, extent: image.Pt(w, h), min: 2, hold: 1, incr: true, acquired: -1}
}

func (f *fakeSurface) Surface() (SurfaceInfo, error) {
	if f.closed {
		f.t.Fatal("Surface after Close")
	}
	return SurfaceInfo{Extent: f.extent, MinImages: f.min, MaxImages: f.max, Incremental: f.incr}, nil
}

func (f *fakeSurface) Create(extent image.Point, n int) (int, error) {
	if extent != f.extent {
		f.t.Fatalf("a swapchain of %v on a surface of %v", extent, f.extent)
	}
	if f.max > 0 && n > f.max || n < f.min {
		f.t.Fatalf("a swapchain of %d images on a surface of %d to %d", n, f.min, f.max)
	}
	f.creates++
	f.images, f.busy, f.acquired = make([]*image.RGBA, n), make([]int, n), -1
	for i := range f.images {
		// Garbage: whatever the driver gives.
		f.images[i] = image.NewRGBA(image.Rectangle{Max: extent})
		for j := range f.images[i].Pix {
			f.images[i].Pix[j] = 0x5a
		}
	}
	return n, nil
}

func next[T any](q *[]T) (T, bool) {
	var zero T
	if len(*q) == 0 {
		return zero, false
	}
	v := (*q)[0]
	*q = (*q)[1:]
	return v, true
}

func (f *fakeSurface) Acquire() (int, SwapchainStatus, error) {
	if f.images == nil {
		f.t.Fatal("Acquire without a swapchain")
	}
	if f.acquired >= 0 {
		f.t.Fatal("Acquire while an image is acquired and not presented")
	}
	st, forced := next(&f.acquireStatus)
	if forced && (st == SwapchainOutOfDate || st == SwapchainNotReady) {
		return 0, st, nil
	}
	for i := range f.images {
		if f.busy[i] == 0 {
			f.acquired = i
			return i, st, nil
		}
	}
	return 0, SwapchainNotReady, nil
}

func (f *fakeSurface) Present(t Target, index int, copies, damage []image.Rectangle) (SwapchainStatus, error) {
	if index != f.acquired {
		f.t.Fatalf("presenting the image %d, the acquired one is %d", index, f.acquired)
	}
	f.acquired = -1
	mt := t.(*memTarget)
	if mt.Size() != f.extent {
		f.t.Fatalf("presenting a target of %v to a surface of %v", mt.Size(), f.extent)
	}
	img := f.images[index]
	f.copied = 0
	for _, r := range copies {
		draw.Draw(img, r, mt.img, r.Min, draw.Src)
		f.copied += r.Dx() * r.Dy()
	}
	f.damage = slices.Clone(damage)
	st, _ := next(&f.presentStatus)
	if st == SwapchainOutOfDate {
		return st, nil // not shown
	}
	for i := range f.busy {
		if f.busy[i] > 0 {
			f.busy[i]--
		}
	}
	f.busy[index] = f.hold
	f.screen = image.NewRGBA(img.Rect)
	copy(f.screen.Pix, img.Pix)
	f.presents++
	return st, nil
}

func (f *fakeSurface) DestroySwapchain() {
	if f.images == nil {
		f.t.Fatal("DestroySwapchain without a swapchain")
	}
	f.images, f.busy, f.acquired = nil, nil, -1
	f.destroys++
}

func (f *fakeSurface) Close() {
	f.images = nil
	f.closes++
	f.closed = true
}

// release gives every busy image back, as a presentation engine does in
// time.
func (f *fakeSurface) release() { clear(f.busy) }

// paint draws n random rectangles into t and returns them.
func paint(rng *rand.Rand, t *memTarget, n int) []image.Rectangle {
	size := t.Size()
	var out []image.Rectangle
	for range n {
		x, y := rng.Intn(size.X), rng.Intn(size.Y)
		r := image.Rect(x, y, x+1+rng.Intn(size.X/3), y+1+rng.Intn(size.Y/3)).Intersect(t.img.Rect)
		c := color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255}
		draw.Draw(t.img, r, image.NewUniform(c), image.Point{}, draw.Src)
		out = append(out, r)
	}
	return out
}

// shows fails the test unless the window shows the target's frame.
func shows(t *testing.T, f *fakeSurface, mt *memTarget, what string) {
	t.Helper()
	if f.screen == nil {
		t.Fatalf("%s: the window shows nothing", what)
	}
	if f.screen.Rect != mt.img.Rect || !slices.Equal(f.screen.Pix, mt.img.Pix) {
		t.Fatalf("%s: the window does not show the frame", what)
	}
}

func present(t *testing.T, s *Swapchain, mt *memTarget, damage []image.Rectangle, all, whole bool, want SwapchainResult, what string) {
	t.Helper()
	got, err := s.Present(mt, damage, all, whole)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if got != want {
		t.Fatalf("%s: Present = %d, want %d", what, got, want)
	}
}

// TestSwapchainDamage checks the accounts of the images: frames of random
// damage, with the engine holding images one or two presents, are shown
// whole, and an image copies only what it lacks once every image holds a
// frame.
func TestSwapchainDamage(t *testing.T) {
	for _, hold := range []int{1, 2} {
		rng := rand.New(rand.NewSource(int64(hold)))
		f := newFakeSurface(t, 64, 48)
		f.hold = hold
		mt := newMemTarget(64, 48)
		s := NewSwapchainOf(f)
		present(t, s, mt, nil, true, true, SwapchainShown, "the first frame")
		shows(t, f, mt, "the first frame")
		if f.copied != 64*48 || f.damage != nil {
			t.Fatalf("the first present copied %d pixels with the damage %v, want the whole frame", f.copied, f.damage)
		}
		partial := 0
		for i := range 60 {
			d := paint(rng, mt, 1+rng.Intn(3))
			got, err := s.Present(mt, d, false, false)
			if err != nil {
				t.Fatal(err)
			}
			if got == SwapchainRetry {
				f.release() // the engine let go in time
				present(t, s, mt, nil, false, false, SwapchainShown, "a retried frame")
			}
			shows(t, f, mt, "a frame")
			if i > 4 && f.copied < 64*48 {
				partial++
			}
			if got == SwapchainShown && f.damage == nil {
				t.Fatalf("frame %d: presented without rectangles, after damage %v", i, d)
			}
		}
		if partial == 0 {
			t.Errorf("hold %d: no present copied less than the whole frame", hold)
		}
		if f.creates != 1 {
			t.Errorf("hold %d: %d swapchains made, want 1", hold, f.creates)
		}
		// Nothing changed: nothing is presented, unless the window asks.
		before := f.presents
		present(t, s, mt, nil, false, false, SwapchainShown, "nothing changed")
		if f.presents != before {
			t.Error("a frame without damage was presented")
		}
		if _, ok := s.LastPresent(); ok {
			t.Error("LastPresent reports a present for a frame without damage")
		}
		f.release()
		present(t, s, mt, nil, false, true, SwapchainShown, "a whole present the window asked for")
		if f.presents != before+1 || f.damage != nil {
			t.Errorf("a whole present: %d presents, damage %v; want one, nil", f.presents-before, f.damage)
		}
		if d, ok := s.LastPresent(); !ok || d != nil {
			t.Errorf("LastPresent after a whole present: %v, %v; want nil, true", d, ok)
		}
		shows(t, f, mt, "a whole present")
		f.release()
		r := paint(rng, mt, 1)
		present(t, s, mt, r, false, false, SwapchainShown, "a partial present")
		if d, ok := s.LastPresent(); !ok || len(d) != 1 || d[0] != r[0] {
			t.Errorf("LastPresent after a partial present: %v, %v; want %v", d, ok, r)
		}
	}
}

// TestSwapchainRetry checks frames that find no image free: their damage
// is presented with the next one's, as one present with both.
func TestSwapchainRetry(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	f := newFakeSurface(t, 40, 40)
	mt := newMemTarget(40, 40)
	s := NewSwapchainOf(f)
	present(t, s, mt, nil, true, true, SwapchainShown, "the first frame")
	a := paint(rng, mt, 1)
	f.acquireStatus = []SwapchainStatus{SwapchainNotReady}
	present(t, s, mt, a, false, false, SwapchainRetry, "no image free")
	b := paint(rng, mt, 1)
	present(t, s, mt, b, false, false, SwapchainShown, "the next frame")
	shows(t, f, mt, "the next frame")
	for _, r := range append(a, b...) {
		if !slices.ContainsFunc(f.damage, func(d image.Rectangle) bool { return r.In(d) }) {
			t.Errorf("the present's damage %v lacks %v", f.damage, r)
		}
	}
	if s.Stats.Retried != 1 || s.Stats.Shown != 2 {
		t.Errorf("stats %+v", s.Stats)
	}
	// Without incremental present the damage is everything.
	f.incr = false
	f.release()
	present(t, s, mt, paint(rng, mt, 1), false, false, SwapchainShown, "without VK_KHR_incremental_present")
	if f.damage != nil {
		t.Errorf("presented the rectangles %v without incremental present", f.damage)
	}
}

// TestSwapchainOutOfDate checks a swapchain that goes out of date at an
// acquire (made again in the same Present) and at a present (the frame is
// retried, whole, on a new swapchain), and a suboptimal one (made again
// after the present, unless it was just made).
func TestSwapchainOutOfDate(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	f := newFakeSurface(t, 50, 30)
	mt := newMemTarget(50, 30)
	s := NewSwapchainOf(f)
	present(t, s, mt, nil, true, true, SwapchainShown, "the first frame")

	f.acquireStatus = []SwapchainStatus{SwapchainOutOfDate}
	present(t, s, mt, paint(rng, mt, 2), false, false, SwapchainShown, "out of date at the acquire")
	shows(t, f, mt, "out of date at the acquire")
	if f.creates != 2 || f.copied != 50*30 {
		t.Errorf("out of date at the acquire: %d swapchains, %d pixels copied; want 2 and the whole frame", f.creates, f.copied)
	}

	f.presentStatus = []SwapchainStatus{SwapchainOutOfDate}
	present(t, s, mt, paint(rng, mt, 2), false, false, SwapchainRetry, "out of date at the present")
	present(t, s, mt, nil, false, false, SwapchainShown, "the frame after")
	shows(t, f, mt, "the frame after an out-of-date present")
	if f.creates != 3 || f.damage != nil {
		t.Errorf("after an out-of-date present: %d swapchains, damage %v; want 3, the whole frame", f.creates, f.damage)
	}

	f.release()
	f.acquireStatus = []SwapchainStatus{SwapchainSuboptimal}
	present(t, s, mt, paint(rng, mt, 1), false, false, SwapchainShown, "suboptimal")
	present(t, s, mt, paint(rng, mt, 1), false, false, SwapchainShown, "after suboptimal")
	shows(t, f, mt, "after suboptimal")
	if f.creates != 4 {
		t.Errorf("after a suboptimal acquire: %d swapchains, want 4", f.creates)
	}
	// Suboptimal on a swapchain just made: kept.
	f.release()
	f.acquireStatus = []SwapchainStatus{SwapchainOutOfDate, SwapchainSuboptimal}
	present(t, s, mt, paint(rng, mt, 1), false, false, SwapchainShown, "suboptimal when just made")
	present(t, s, mt, paint(rng, mt, 1), false, false, SwapchainShown, "after it")
	if f.creates != 5 {
		t.Errorf("a swapchain suboptimal when made was made again: %d swapchains, want 5", f.creates)
	}
	if s.Stats.OutOfDate != 3 {
		t.Errorf("%d out-of-date answers counted, want 3", s.Stats.OutOfDate)
	}
}

// TestSwapchainResize checks a window resized: a frame of the new size on
// a surface still at the old one is retried, then presented whole on a
// swapchain of the new size; a surface that stays at another size than
// the frames is an error.
func TestSwapchainResize(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	f := newFakeSurface(t, 40, 30)
	mt := newMemTarget(40, 30)
	s := NewSwapchainOf(f)
	present(t, s, mt, nil, true, true, SwapchainShown, "the first frame")
	big := newMemTarget(80, 50)
	paint(rng, big, 5)
	present(t, s, big, nil, true, true, SwapchainRetry, "a frame ahead of the window")
	f.extent = big.Size()
	present(t, s, big, nil, true, true, SwapchainShown, "the resized frame")
	shows(t, f, big, "the resized frame")
	if f.creates != 2 || s.Images() != 3 {
		t.Errorf("after a resize: %d swapchains of %d images, want 2 of 3", f.creates, s.Images())
	}
	// A window that stays at another size.
	small := newMemTarget(10, 10)
	var err error
	for i := 0; i <= maxMismatch && err == nil; i++ {
		_, err = s.Present(small, nil, true, true)
	}
	if err == nil {
		t.Error("a surface at another size than the frames for good: no error")
	}
}

// TestSwapchainMinimize checks a minimized window: nothing is presented,
// the damage is kept, and the window shows the frame when it comes back,
// on the swapchain it had or on a new one.
func TestSwapchainMinimize(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	f := newFakeSurface(t, 30, 30)
	mt := newMemTarget(30, 30)
	s := NewSwapchainOf(f)
	present(t, s, mt, nil, true, true, SwapchainShown, "the first frame")
	f.extent = image.Point{}
	present(t, s, mt, paint(rng, mt, 3), false, false, SwapchainHidden, "minimized")
	present(t, s, mt, paint(rng, mt, 3), false, false, SwapchainHidden, "still minimized")
	f.extent = image.Pt(30, 30)
	f.release()
	present(t, s, mt, nil, false, false, SwapchainShown, "restored")
	shows(t, f, mt, "restored")
	if f.creates != 1 || s.Stats.Hidden != 2 {
		t.Errorf("restored: %d swapchains, %d hidden presents; want 1, 2", f.creates, s.Stats.Hidden)
	}
	// Restored out of date.
	f.extent = image.Point{}
	present(t, s, mt, paint(rng, mt, 1), false, false, SwapchainHidden, "minimized again")
	f.extent = image.Pt(30, 30)
	f.acquireStatus = []SwapchainStatus{SwapchainOutOfDate}
	present(t, s, mt, nil, false, true, SwapchainShown, "restored out of date")
	shows(t, f, mt, "restored out of date")
}

// TestSwapchainMaxImages checks the image count: the surface's minimum
// and one more, within its maximum.
func TestSwapchainMaxImages(t *testing.T) {
	for _, c := range []struct{ min, max, want int }{{2, 0, 3}, {2, 2, 2}, {3, 8, 4}, {1, 0, 2}} {
		f := newFakeSurface(t, 8, 8)
		f.min, f.max = c.min, c.max
		s := NewSwapchainOf(f)
		present(t, s, newMemTarget(8, 8), nil, true, true, SwapchainShown, "a frame")
		if s.Images() != c.want {
			t.Errorf("a surface of %d to %d images: %d, want %d", c.min, c.max, s.Images(), c.want)
		}
	}
}

// TestSwapchainRelease checks Release (the swapchain goes, the surface
// stays, the next present is whole on a new swapchain) and Close.
func TestSwapchainRelease(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	f := newFakeSurface(t, 20, 20)
	mt := newMemTarget(20, 20)
	s := NewSwapchainOf(f)
	present(t, s, mt, nil, true, true, SwapchainShown, "the first frame")
	s.Release()
	s.Release() // twice does nothing
	if f.destroys != 1 || s.Images() != 0 {
		t.Fatalf("Release: %d swapchains destroyed, %d images left", f.destroys, s.Images())
	}
	paint(rng, mt, 2)
	present(t, s, mt, nil, false, false, SwapchainShown, "after Release")
	shows(t, f, mt, "after Release")
	if f.creates != 2 || f.copied != 20*20 || f.damage != nil {
		t.Errorf("after Release: %d swapchains, %d pixels copied, damage %v", f.creates, f.copied, f.damage)
	}
	s.Close()
	s.Close()
	if f.closes != 1 {
		t.Errorf("Close closed the surface %d times", f.closes)
	}
	if _, err := s.Present(mt, nil, true, true); !errors.Is(err, ErrSwapchainClosed) {
		t.Errorf("Present after Close: %v", err)
	}
}

// failingSurface fails the calls a test names.
type failingSurface struct {
	*fakeSurface
	fail string
}

var errFake = errors.New("fake failure")

func (f *failingSurface) Surface() (SurfaceInfo, error) {
	if f.fail == "Surface" {
		return SurfaceInfo{}, errFake
	}
	return f.fakeSurface.Surface()
}

func (f *failingSurface) Create(e image.Point, n int) (int, error) {
	if f.fail == "Create" {
		return 0, errFake
	}
	return f.fakeSurface.Create(e, n)
}

func (f *failingSurface) Present(t Target, i int, c, d []image.Rectangle) (SwapchainStatus, error) {
	if f.fail == "Present" {
		f.acquired = -1
		return 0, errFake
	}
	return f.fakeSurface.Present(t, i, c, d)
}

// TestSwapchainErrors checks that an error of the device or the surface is
// Present's.
func TestSwapchainErrors(t *testing.T) {
	for _, op := range []string{"Surface", "Create", "Present"} {
		f := &failingSurface{fakeSurface: newFakeSurface(t, 8, 8), fail: op}
		s := NewSwapchainOf(f)
		if _, err := s.Present(newMemTarget(8, 8), nil, true, true); !errors.Is(err, errFake) {
			t.Errorf("%s failing: Present returned %v", op, err)
		}
	}
}
