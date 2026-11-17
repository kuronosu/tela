package golden

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeT records failures instead of failing the real test. Fatalf stops the
// helper with a panic that run recovers, like runtime.Goexit would.
type fakeT struct {
	testing.TB
	errors []string
	fatal  bool
	logs   []string
}

type fatalSentinel struct{}

func (f *fakeT) Helper()      {}
func (f *fakeT) Name() string { return "TestFake" }
func (f *fakeT) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}
func (f *fakeT) Fatalf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
	f.fatal = true
	panic(fatalSentinel{})
}
func (f *fakeT) Logf(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func run(t *testing.T, name string, img image.Image, opts ...Option) *fakeT {
	t.Helper()
	f := &fakeT{TB: t}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSentinel); !ok {
					panic(r)
				}
			}
		}()
		Image(f, name, img, opts...)
	}()
	return f
}

func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(10, 20, 26, 28)) // non-zero origin
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			a := uint8((x*37 + y*11) % 256)
			r := uint8(int(a) * (x % 7) / 6)
			img.SetRGBA(x, y, color.RGBA{r, a / 2, a, a}) // premultiplied, translucent
		}
	}
	return img
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestMissingGolden(t *testing.T) {
	t.Setenv(UpdateEnv, "")
	dir := t.TempDir()
	f := run(t, "missing", testImage(), Dir(dir))
	if !f.fatal || len(f.errors) != 1 {
		t.Fatalf("want one fatal error, got %v", f.errors)
	}
	msg := f.errors[0]
	if !strings.Contains(msg, "does not exist") || !strings.Contains(msg, "TELA_UPDATE=1") || !strings.Contains(msg, "TestFake") {
		t.Errorf("message is not actionable: %s", msg)
	}
	if !exists(filepath.Join(dir, "missing_actual.png")) {
		t.Error("the actual image was not written")
	}
}

func TestUpdateThenCompare(t *testing.T) {
	dir := t.TempDir()
	img := testImage()

	t.Setenv(UpdateEnv, "1")
	if f := run(t, "sub/dir/case.png", img, Dir(dir)); len(f.errors) != 0 {
		t.Fatalf("update failed: %v", f.errors)
	}
	if !exists(filepath.Join(dir, "sub", "dir", "case.png")) {
		t.Fatal("golden not written (the .png suffix must be stripped and subdirectories created)")
	}

	t.Setenv(UpdateEnv, "")
	// The round trip through PNG is exact even for translucent pixels.
	if f := run(t, "sub/dir/case", img, Dir(dir)); len(f.errors) != 0 {
		t.Fatalf("identical image does not match its golden: %v", f.errors)
	}

	// Change three pixels by 3 levels and one by 40.
	changed := image.NewRGBA(img.Rect)
	copy(changed.Pix, img.Pix)
	bump := func(x, y int, d uint8) {
		c := changed.RGBAAt(x, y)
		c.A = 255
		c.R = min(c.R+d, 255)
		changed.SetRGBA(x, y, c)
	}
	img2 := func() *image.RGBA { // opaque base so deltas are exact
		m := image.NewRGBA(img.Rect)
		for i := range m.Pix {
			m.Pix[i] = 100
			if i%4 == 3 {
				m.Pix[i] = 255
			}
		}
		return m
	}()
	t.Setenv(UpdateEnv, "1")
	run(t, "opaque", img2, Dir(dir))
	t.Setenv(UpdateEnv, "")
	copy(changed.Pix, img2.Pix)
	bump(11, 21, 3)
	bump(12, 21, 3)
	bump(13, 21, 3)
	bump(20, 25, 40)

	f := run(t, "opaque", changed, Dir(dir))
	if len(f.errors) != 1 || !strings.Contains(f.errors[0], "4 pixel(s) differ") ||
		!strings.Contains(f.errors[0], "largest channel difference 40 at (10, 5)") {
		t.Fatalf("unexpected result: %v", f.errors)
	}
	if !exists(filepath.Join(dir, "opaque_actual.png")) || !exists(filepath.Join(dir, "opaque_diff.png")) {
		t.Fatal("actual/diff images not written on failure")
	}
	if f := run(t, "opaque", changed, Dir(dir), MaxDelta(3)); len(f.errors) != 1 || !strings.Contains(f.errors[0], "1 pixel(s) differ") {
		t.Fatalf("MaxDelta(3): %v", f.errors)
	}
	if f := run(t, "opaque", changed, Dir(dir), MaxDelta(3), MaxPixels(1)); len(f.errors) != 0 {
		t.Fatalf("MaxDelta(3)+MaxPixels(1) should pass: %v", f.errors)
	}
	if exists(filepath.Join(dir, "opaque_actual.png")) || exists(filepath.Join(dir, "opaque_diff.png")) {
		t.Error("stale actual/diff images not removed after a passing comparison")
	}
}

func TestSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(UpdateEnv, "1")
	run(t, "size", image.NewRGBA(image.Rect(0, 0, 4, 4)), Dir(dir))
	t.Setenv(UpdateEnv, "")
	f := run(t, "size", image.NewRGBA(image.Rect(0, 0, 5, 4)), Dir(dir))
	if len(f.errors) != 1 || !strings.Contains(f.errors[0], "size 5x4") {
		t.Fatalf("unexpected result: %v", f.errors)
	}
}

// TestReadRGBAFastPath checks ReadRGBA's direct premultiplication of a
// picture that decodes to *image.NRGBA against image.RGBA.Set, for every
// pair of channel and alpha values.
func TestReadRGBAFastPath(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			copy(src.Pix[src.PixOffset(x, y):][:4], []uint8{uint8(x), 255 - uint8(x), uint8(x) / 2, uint8(y)})
		}
	}
	path := filepath.Join(t.TempDir(), "nrgba.png")
	if err := writePNG(path, src); err != nil {
		t.Fatal(err)
	}
	got := ReadRGBA(t, path)
	want := image.NewRGBA(src.Rect)
	for y := range 256 {
		for x := range 256 {
			want.Set(x, y, src.At(x, y))
		}
	}
	for y := range 256 {
		for x := range 256 {
			if g, w := got.RGBAAt(x, y), want.RGBAAt(x, y); g != w {
				t.Fatalf("at (%d, %d): %v, want %v", x, y, g, w)
			}
		}
	}
}

// TestToNRGBAFastPaths checks the direct conversions of *image.RGBA and
// *image.NRGBA against the generic one (color.NRGBAModel), for every pair
// of channel and alpha values (premultiplied or not) and for sub-images
// whose origin is not (0, 0).
func TestToNRGBAFastPaths(t *testing.T) {
	r := image.Rect(3, -2, 3+256, -2+256)
	rgba, nrgba := image.NewRGBA(r), image.NewNRGBA(r)
	for y := range 256 {
		for x := range 256 {
			c := uint8(x)
			px := []uint8{c, 255 - c, c / 2, uint8(y)}
			copy(rgba.Pix[rgba.PixOffset(r.Min.X+x, r.Min.Y+y):][:4], px)
			copy(nrgba.Pix[nrgba.PixOffset(r.Min.X+x, r.Min.Y+y):][:4], px)
		}
	}
	for _, img := range []image.Image{rgba, nrgba, rgba.SubImage(image.Rect(10, 5, 200, 90)), nrgba.SubImage(image.Rect(10, 5, 200, 90))} {
		got := toNRGBA(img)
		b := img.Bounds()
		if got.Rect != image.Rect(0, 0, b.Dx(), b.Dy()) {
			t.Fatalf("%T: bounds %v", img, got.Rect)
		}
		for y := range b.Dy() {
			for x := range b.Dx() {
				want := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				if g := got.NRGBAAt(x, y); g != want {
					t.Fatalf("%T at (%d, %d): %v, want %v", img, x, y, g, want)
				}
			}
		}
	}
}
