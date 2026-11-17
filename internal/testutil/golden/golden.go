// Package golden compares images produced by tests against golden PNG files.
//
// It is a small, standard-library-only helper shared by the rasterizer tests
// and telatest. A golden lives at testdata/<name>.png, relative to
// the working directory of the test (the package directory under go test).
//
//	golden.Image(t, "rrect_scale2", img)                          // exact match
//	golden.Image(t, "blur", img, golden.MaxDelta(2), golden.MaxPixels(10))
//
// Regenerate goldens with TELA_UPDATE=1 go test ./... (ADR-022) and review the
// PNGs before committing them. On a mismatch, <name>_actual.png and
// <name>_diff.png are written next to the golden (both patterns are
// gitignored); they are removed again when the test passes.
//
// # Comparison
//
// PNG stores non-premultiplied colors, so both images are first converted to
// non-premultiplied 8-bit RGBA exactly as image/png encodes them (Go's
// color.NRGBAModel); a golden written by this package therefore decodes to the
// same bytes as the image it was written from. The per-channel difference of
// two pixels is then measured on their premultiplied values, so that pixels
// that are almost transparent do not amplify tiny differences.
package golden

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UpdateEnv is the environment variable that, set to "1", makes Image write
// the golden instead of comparing against it (ADR-022).
const UpdateEnv = "TELA_UPDATE"

// Option configures Image.
type Option func(*config)

type config struct {
	maxDelta  int
	maxPixels int
	dir       string
}

// MaxDelta sets the largest per-channel difference (0-255, on premultiplied
// values) for which two pixels still count as equal. The default is 0.
func MaxDelta(d uint8) Option { return func(c *config) { c.maxDelta = int(d) } }

// MaxPixels sets how many pixels may differ by more than MaxDelta before the
// comparison fails. The default is 0.
func MaxPixels(n int) Option { return func(c *config) { c.maxPixels = max(n, 0) } }

// Dir changes the directory that holds the goldens. The default is
// "testdata".
func Dir(dir string) Option { return func(c *config) { c.dir = dir } }

// Image compares img against the golden testdata/<name>.png and reports a
// test failure if they differ by more than the configured tolerance (exact by
// default). name may contain slashes to use subdirectories; a trailing ".png"
// is ignored.
//
// With TELA_UPDATE=1 in the environment the golden is (re)written instead and
// the test does not fail. A missing golden is a failure that says how to
// create it.
func Image(t testing.TB, name string, img image.Image, opts ...Option) {
	t.Helper()
	cfg := config{dir: "testdata"}
	for _, o := range opts {
		o(&cfg)
	}
	name = strings.TrimSuffix(name, ".png")
	base := filepath.Join(cfg.dir, filepath.FromSlash(name))
	goldenPath := base + ".png"
	actualPath := base + "_actual.png"
	diffPath := base + "_diff.png"

	got := toNRGBA(img)
	if os.Getenv(UpdateEnv) == "1" {
		if err := writePNG(goldenPath, got); err != nil {
			t.Fatalf("golden: cannot write %s: %v", goldenPath, err)
		}
		removeStale(actualPath, diffPath)
		t.Logf("golden: wrote %s; review it before committing", goldenPath)
		return
	}

	want, err := readPNG(goldenPath)
	if err != nil {
		writeArtifact(t, actualPath, got)
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("golden: %s does not exist; create it with %s=1 go test -run '%s' and review the PNG (the rendered image is in %s)",
				goldenPath, UpdateEnv, t.Name(), actualPath)
		}
		t.Fatalf("golden: cannot read %s: %v", goldenPath, err)
	}

	if got.Rect.Size() != want.Rect.Size() {
		writeArtifact(t, actualPath, got)
		t.Errorf("golden: %s: size %dx%d, golden %s is %dx%d (rendered image in %s; regenerate with %s=1 if the change is intended)",
			name, got.Rect.Dx(), got.Rect.Dy(), goldenPath, want.Rect.Dx(), want.Rect.Dy(), actualPath, UpdateEnv)
		return
	}

	if bytes.Equal(got.Pix, want.Pix) {
		removeStale(actualPath, diffPath) // identical (both start at (0, 0), same size): nothing to measure
		return
	}
	d := compare(got, want, cfg.maxDelta)
	if d.count > cfg.maxPixels {
		writeArtifact(t, actualPath, got)
		writeArtifact(t, diffPath, d.img)
		t.Errorf("golden: %s: %d pixel(s) differ by more than %d (allowed: %d); largest channel difference %d at (%d, %d): got %v, want %v.\n"+
			"See %s and %s. If the change is intended, regenerate with %s=1 and review the PNG.",
			name, d.count, cfg.maxDelta, cfg.maxPixels, d.worst, d.worstAt.X, d.worstAt.Y, d.gotAt, d.wantAt,
			actualPath, diffPath, UpdateEnv)
		return
	}
	removeStale(actualPath, diffPath)
}

// toNRGBA converts img to a non-premultiplied image with origin (0, 0),
// exactly as image/png converts the pixels it encodes.
func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	// The images of the tests (renders, and the goldens image/png decodes)
	// are *image.RGBA or *image.NRGBA: their rows are converted directly,
	// with color.NRGBAModel's arithmetic (TestToNRGBAFastPaths), instead of
	// an interface call and an allocation per pixel.
	switch src := img.(type) {
	case *image.NRGBA:
		for y := 0; y < b.Dy(); y++ {
			copy(out.Pix[out.PixOffset(0, y):][:4*b.Dx()], src.Pix[src.PixOffset(b.Min.X, b.Min.Y+y):])
		}
		return out
	case *image.RGBA:
		for y := 0; y < b.Dy(); y++ {
			unpremulRow(out.Pix[out.PixOffset(0, y):][:4*b.Dx()], src.Pix[src.PixOffset(b.Min.X, b.Min.Y+y):])
		}
		return out
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return out
}

// unpremulRow converts the premultiplied pixels of src into the
// non-premultiplied pixels of dst (len(dst) bytes), as color.NRGBAModel
// converts a color.RGBA.
func unpremulRow(dst, src []uint8) {
	for i := 0; i < len(dst); i += 4 {
		s := src[i : i+4 : i+4]
		d := dst[i : i+4 : i+4]
		switch a := uint32(s[3]); a {
		case 0xff:
			d[0], d[1], d[2], d[3] = s[0], s[1], s[2], 0xff
		case 0:
			d[0], d[1], d[2], d[3] = 0, 0, 0, 0
		default:
			// color.RGBA.RGBA widens each channel to 16 bits (x*0x101), and
			// the model divides by the widened alpha.
			a16 := a * 0x101
			d[0] = uint8((uint32(s[0]) * 0x101 * 0xffff / a16) >> 8)
			d[1] = uint8((uint32(s[1]) * 0x101 * 0xffff / a16) >> 8)
			d[2] = uint8((uint32(s[2]) * 0x101 * 0xffff / a16) >> 8)
			d[3] = uint8(a)
		}
	}
}

type diff struct {
	count         int
	worst         int
	worstAt       image.Point
	gotAt, wantAt color.RGBA
	img           *image.NRGBA
}

// compare counts the pixels whose premultiplied channels differ by more than
// maxDelta and builds a diff image: differing pixels in red (brighter for
// larger differences, at least half intensity), equal pixels as a faded gray
// copy of the golden.
func compare(got, want *image.NRGBA, maxDelta int) diff {
	r := got.Rect
	d := diff{img: image.NewNRGBA(r)}
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			i := got.PixOffset(x, y)
			g := premul(got.Pix[i : i+4])
			w := premul(want.Pix[i : i+4])
			delta := max(absDiff(g.R, w.R), absDiff(g.G, w.G), absDiff(g.B, w.B), absDiff(g.A, w.A))
			o := d.img.Pix[i : i+4]
			if delta > maxDelta {
				d.count++
				if delta > d.worst {
					d.worst, d.worstAt, d.gotAt, d.wantAt = delta, image.Pt(x, y), g, w
				}
				o[0], o[1], o[2], o[3] = uint8(128+delta/2), 0, 0, 255
				continue
			}
			// Faded luminance of the golden over white, for orientation.
			lum := (int(w.R)*299 + int(w.G)*587 + int(w.B)*114) / 1000
			v := uint8(255 - (int(w.A)-lum)/4)
			o[0], o[1], o[2], o[3] = v, v, v, 255
		}
	}
	return d
}

func premul(p []uint8) color.RGBA {
	c := color.NRGBA{p[0], p[1], p[2], p[3]}
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func readPNG(path string) (*image.NRGBA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding PNG: %w", err)
	}
	return toNRGBA(img), nil
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeArtifact(t testing.TB, path string, img image.Image) {
	t.Helper()
	if err := writePNG(path, img); err != nil {
		t.Logf("golden: cannot write %s: %v", path, err)
	}
}

func removeStale(paths ...string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}
