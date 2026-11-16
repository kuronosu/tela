//go:build telagpu && (linux || windows)

package gpu

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/vk"
)

// shiftRef is what Target.Shift does, on an image: every pixel of r takes
// the value the pixel (dx, dy) before it had, when both are in the image.
func shiftRef(img *image.RGBA, r image.Rectangle, dx, dy int) {
	old := image.NewRGBA(img.Rect)
	copy(old.Pix, img.Pix)
	r = r.Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if src := image.Pt(x-dx, y-dy); src.In(img.Rect) {
				img.SetRGBA(x, y, old.RGBAAt(src.X, src.Y))
			}
		}
	}
}

// TestShift moves rectangles of a target the way a scroll does, by a few
// pixels and by most of the rectangle, in every direction, at the edges
// and beyond them, one shift after the other on the same target, and
// compares every pixel with the same moves made on an image.
func TestShift(t *testing.T) {
	dl := regionScene()
	const w, h = 421, 303
	cases := []struct {
		r      image.Rectangle
		dx, dy int
	}{
		{image.Rect(40, 30, 300, 250), 0, 7},     // down, overlapping
		{image.Rect(40, 30, 300, 250), 0, -13},   // up
		{image.Rect(10, 10, 200, 120), 5, 0},     // right
		{image.Rect(10, 10, 200, 120), -1, 0},    // left, one pixel
		{image.Rect(100, 50, 380, 290), 17, -29}, // both
		{image.Rect(0, 0, w, h), 0, 64},          // the whole target: the sources of the first rows are outside
		{image.Rect(0, 0, w, h), -80, 0},
		{image.Rect(-20, -20, 90, 90), 3, 3},      // beyond the target
		{image.Rect(300, 200, 500, 400), -6, -2},  // beyond the other corner
		{image.Rect(50, 50, 150, 60), 0, 9},       // sources and destinations that almost do not overlap
		{image.Rect(50, 50, 150, 60), 0, 10},      // nor at all
		{image.Rect(50, 50, 150, 60), 0, 200},     // far away
		{image.Rect(200, 100, 201, 101), 1, 1},    // one pixel
		{image.Rect(60, 60, 60, 90), 4, 4},        // empty
		{image.Rect(60, 60, 160, 90), 0, 0},       // no move
		{image.Rect(0, 0, w, h), w + 5, 0},        // every source is outside
		{image.Rect(20, 40, 400, 280), -150, 120}, // most of the rectangle
	}
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		if _, err := r.Render(target, dl, Options{Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		want := readAll(t, target)
		for _, c := range cases {
			if err := target.Shift(c.r, c.dx, c.dy); err != nil {
				t.Fatalf("Shift(%v, %d, %d): %v", c.r, c.dx, c.dy, err)
			}
			shiftRef(want, c.r, c.dx, c.dy)
			got := readAll(t, target)
			if m := compare(got, want); m.Max != 0 {
				t.Fatalf("after Shift(%v, %d, %d): %d pixels differ, the first at %v: %v, want %v", c.r, c.dx, c.dy, m.Over0, m.Worst,
					got.RGBAAt(m.Worst.X, m.Worst.Y), want.RGBAAt(m.Worst.X, m.Worst.Y))
			}
		}
		// A render after the shifts goes on over the moved pixels, and the
		// device gives the surface of the shifts up when it is trimmed.
		if _, err := r.Render(target, dl, Options{Region: image.Rect(30, 30, 90, 70), Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		r.Trim()
		if err := target.Shift(image.Rect(0, 0, w, h), 1, 0); err != nil {
			t.Fatal(err)
		}
	})
}

// TestReadRects reads several rectangles in one call: each has the pixels
// of a read of the whole target, and nothing else of the image is written.
func TestReadRects(t *testing.T) {
	dl := regionScene()
	const w, h = 421, 303
	rects := []image.Rectangle{
		image.Rect(0, 0, 64, 64), image.Rect(37, 51, 190, 143), image.Rect(200, 1, 201, h-3), image.Rect(100, 100, 101, 101),
		image.Rect(w-121, h-103, w+79, h+97), image.Rect(-50, -50, 30, 30), {}, image.Rect(500, 500, 600, 600),
		image.Rect(120, 60, 260, 200), // overlaps another one
	}
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		if _, err := r.Render(target, dl, Options{Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		full := readAll(t, target)
		sentinel := color.RGBA{1, 2, 3, 255}
		for n := 0; n <= len(rects); n++ {
			got := image.NewRGBA(image.Rect(0, 0, w, h))
			for i := 0; i < len(got.Pix); i += 4 {
				got.Pix[i], got.Pix[i+1], got.Pix[i+2], got.Pix[i+3] = sentinel.R, sentinel.G, sentinel.B, sentinel.A
			}
			if err := target.ReadPixels(got, rects[:n]...); err != nil {
				t.Fatal(err)
			}
			bad := 0
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					want := sentinel
					for _, rc := range rects[:n] {
						if image.Pt(x, y).In(rc) {
							want = full.RGBAAt(x, y)
						}
					}
					if g := got.RGBAAt(x, y); g != want {
						if bad == 0 {
							t.Errorf("%d rectangles: pixel (%d, %d) is %v, want %v", n, x, y, g, want)
						}
						bad++
					}
				}
			}
			if bad > 0 {
				t.Errorf("%d rectangles: %d pixels differ", n, bad)
			}
		}
		// An image smaller than the target takes only its own pixels.
		small := image.NewRGBA(image.Rect(50, 40, 90, 70))
		if err := target.ReadPixels(small, image.Rect(0, 0, w, h)); err != nil {
			t.Fatal(err)
		}
		for y := 40; y < 70; y++ {
			for x := 50; x < 90; x++ {
				if g, want := small.RGBAAt(x, y), full.RGBAAt(x, y); g != want {
					t.Fatalf("a small image: pixel (%d, %d) is %v, want %v", x, y, g, want)
				}
			}
		}
	})
}

// TestDeviceLost checks that a device that reported VK_ERROR_DEVICE_LOST
// stays lost: every call fails with ErrDeviceLost, and Close still
// destroys what the device holds (the validation layer reports what it
// leaves).
func TestDeviceLost(t *testing.T) {
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(32, 32)
		if err != nil {
			t.Fatal(err)
		}
		var r Renderer
		if _, err := r.Render(target, regionScene(), Options{Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		d := dev.(*vkDevice)
		// Another error is passed on as it is.
		other := fmt.Errorf("gpu: submitting: %w", vk.ErrorOutOfDeviceMemory)
		if err := d.check(other); err != other || d.lost {
			t.Fatalf("check(%v) = %v, lost %v", other, err, d.lost)
		}
		err = d.check(fmt.Errorf("gpu: waiting for the frame: %w", vk.ErrorDeviceLost))
		if !errors.Is(err, ErrDeviceLost) || !errors.Is(err, vk.ErrorDeviceLost) {
			t.Fatalf("the error of a lost device is %v", err)
		}
		if _, err := r.Render(target, nil, Options{Clear: &gfx.White}); !errors.Is(err, ErrDeviceLost) {
			t.Errorf("Render on a lost device: %v", err)
		}
		if err := target.ReadPixels(image.NewRGBA(image.Rect(0, 0, 32, 32)), image.Rect(0, 0, 32, 32)); !errors.Is(err, ErrDeviceLost) {
			t.Errorf("ReadPixels on a lost device: %v", err)
		}
		if err := target.Shift(image.Rect(0, 0, 32, 32), 1, 1); !errors.Is(err, ErrDeviceLost) {
			t.Errorf("Shift on a lost device: %v", err)
		}
		if _, err := dev.NewTarget(8, 8); !errors.Is(err, ErrDeviceLost) {
			t.Errorf("NewTarget on a lost device: %v", err)
		}
		r.Trim() // releases what the device holds: nothing is drawn
		target.Close()
	})
}

// TestWaitTimeout checks that a submission whose wait fails, a timeout
// included, loses the device: the submission may still run, so the call
// fails with ErrDeviceLost (the caller moves every window to the CPU) and
// nothing is submitted or reused after it, for every kind of wait (a
// render, a read, a shift).
func TestWaitTimeout(t *testing.T) {
	for _, op := range []string{"Render", "ReadPixels", "Shift"} {
		t.Run(op, func(t *testing.T) {
			eachDevice(t, func(t *testing.T, dev Device) {
				target, err := dev.NewTarget(32, 32)
				if err != nil {
					t.Fatal(err)
				}
				defer target.Close()
				var r Renderer
				defer r.Trim()
				if _, err := r.Render(target, regionScene(), Options{Clear: &gfx.White}); err != nil {
					t.Fatal(err)
				}
				d := dev.(*vkDevice)
				waits := 0
				d.waitErr = func(err error) error {
					// The submission did finish (the fence is signaled): the
					// wait says it did not, as one that timed out does.
					waits++
					if err != nil {
						return err
					}
					return vk.Timeout
				}
				defer func() { d.waitErr = nil }()
				switch op {
				case "Render":
					_, err = r.Render(target, regionScene(), Options{Clear: &gfx.Black})
				case "ReadPixels":
					err = target.ReadPixels(image.NewRGBA(image.Rect(0, 0, 32, 32)), image.Rect(0, 0, 32, 32))
				case "Shift":
					err = target.Shift(image.Rect(0, 0, 32, 32), 1, 1)
				}
				if !errors.Is(err, ErrDeviceLost) || !errors.Is(err, vk.Timeout) || !d.lost {
					t.Fatalf("%s whose wait timed out: %v (lost %v), want an error that wraps ErrDeviceLost and the timeout", op, err, d.lost)
				}
				// Nothing is submitted after it.
				before := waits
				if err := target.ReadPixels(image.NewRGBA(image.Rect(0, 0, 32, 32)), image.Rect(0, 0, 32, 32)); !errors.Is(err, ErrDeviceLost) {
					t.Errorf("ReadPixels after the timeout: %v", err)
				}
				if _, err := r.Render(target, regionScene(), Options{Clear: &gfx.White}); !errors.Is(err, ErrDeviceLost) {
					t.Errorf("Render after the timeout: %v", err)
				}
				if waits != before {
					t.Errorf("%d submissions after the timeout", waits-before)
				}
			})
		})
	}
}
