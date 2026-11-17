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
	if n, ok := img.(*image.NRGBA); ok {
		// What image/png decodes a picture with alpha to: premultiplied
		// directly, as color.RGBAModel converts a color.NRGBA
		// (TestReadRGBAFastPath).
		for y := range b.Dy() {
			premulRow(out.Pix[out.PixOffset(0, y):][:4*b.Dx()], n.Pix[n.PixOffset(b.Min.X, b.Min.Y+y):])
		}
		return out
	}
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// premulRow converts the non-premultiplied pixels of src into the
// premultiplied pixels of dst (len(dst) bytes) with color.NRGBA.RGBA's
// arithmetic: each channel widened to 16 bits, times alpha over 0xff.
func premulRow(dst, src []uint8) {
	for i := 0; i < len(dst); i += 4 {
		s := src[i : i+4 : i+4]
		d := dst[i : i+4 : i+4]
		a := uint32(s[3])
		d[0] = uint8((uint32(s[0]) * 0x101 * a / 0xff) >> 8)
		d[1] = uint8((uint32(s[1]) * 0x101 * a / 0xff) >> 8)
		d[2] = uint8((uint32(s[2]) * 0x101 * a / 0xff) >> 8)
		d[3] = s[3]
	}
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
