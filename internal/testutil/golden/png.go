package golden

import (
	"image"
	"image/png"
	"os"
	"testing"
)

// ReadRGBA reads the PNG at path as an *image.RGBA with its origin at
// (0, 0), for tests that compare a render with a reference picture (such as
// a Chromium screenshot). It fails the test when the file cannot be read or
// decoded.
func ReadRGBA(t testing.TB, path string) *image.RGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (a Chromium reference: regenerate with TELA_CHROMIUM=1)", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// WritePNG writes img to path with the default compression (a debug dump,
// not a committed file).
func WritePNG(t testing.TB, path string, img image.Image) {
	t.Helper()
	writeEncoded(t, path, img, png.DefaultCompression)
}

// WriteBestPNG writes img to path with the best compression, for the
// reference pictures that are committed.
func WriteBestPNG(t testing.TB, path string, img image.Image) {
	t.Helper()
	writeEncoded(t, path, img, png.BestCompression)
}

func writeEncoded(t testing.TB, path string, img image.Image, level png.CompressionLevel) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := (&png.Encoder{CompressionLevel: level}).Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
