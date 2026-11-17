package appicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
<rect width="100" height="100" rx="22" fill="#2563eb"/>
<circle cx="50" cy="50" r="28" fill="#ffffff"/>
<path d="M36 52l10 10 18-22" fill="none" stroke="#2563eb" stroke-width="8" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`

// testPNG is a square PNG of the given side: a blue field with a red square
// in the middle that covers a quarter of it, and a transparent corner.
func testPNG(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			c := color.NRGBA{0, 0, 255, 255}
			if x >= side/4 && x < side*3/4 && y >= side/4 && y < side*3/4 {
				c = color.NRGBA{255, 0, 0, 255}
			}
			if x < side/8 && y < side/8 {
				c = color.NRGBA{}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestSVGSource(t *testing.T) {
	s, err := Decode([]byte(testSVG), ".svg")
	if err != nil {
		t.Fatal(err)
	}
	if s.MaxSize() != 0 {
		t.Errorf("MaxSize %d for an SVG, want 0", s.MaxSize())
	}
	for _, size := range []int{16, 48, 512} {
		img := s.Render(size)
		if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Errorf("Render(%d) is %v", size, img.Bounds())
		}
		// The corner is outside the rounded square: transparent. The
		// middle is the white disc or the check mark's blue.
		if c := img.NRGBAAt(0, 0); c.A != 0 {
			t.Errorf("%d px: corner %v, want transparent", size, c)
		}
		if c := img.NRGBAAt(size/2, size*7/8+size/32); c.A != 255 || c.B < 200 || c.R > 80 {
			t.Errorf("%d px: pixel below the disc %v, want the blue of the background", size, c)
		}
		if c := img.NRGBAAt(size*3/10, size*4/10); c.A != 255 || c.R < 240 || c.G < 240 {
			t.Errorf("%d px: pixel in the disc %v, want white", size, c)
		}
	}
	a, _ := s.PNG(64)
	s2, _ := Decode([]byte(testSVG), ".svg")
	b, _ := s2.PNG(64)
	if !bytes.Equal(a, b) {
		t.Error("the same source gave different PNG bytes")
	}
}

func TestPNGSource(t *testing.T) {
	s, err := Decode(testPNG(t, 1024), ".png")
	if err != nil {
		t.Fatal(err)
	}
	if s.MaxSize() != 1024 {
		t.Errorf("MaxSize %d, want 1024", s.MaxSize())
	}
	// The same size is the source itself.
	if c := s.Render(1024).NRGBAAt(512, 512); c != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("1024 px center %v", c)
	}
	// 1024 -> 100 is not an integer ratio: the edges of the red square
	// (at 25 and 75) fall on pixel edges, those of the corner at 12.5
	// blend half of a pixel.
	img := s.Render(100)
	if c := img.NRGBAAt(50, 50); c != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("100 px center %v", c)
	}
	if c := img.NRGBAAt(10, 90); c != (color.NRGBA{0, 0, 255, 255}) {
		t.Errorf("100 px blue %v", c)
	}
	if c := img.NRGBAAt(2, 2); c.A != 0 {
		t.Errorf("100 px corner %v, want transparent", c)
	}
	edge := img.NRGBAAt(12, 2) // the corner ends at x = 12.8 px
	if edge.A == 0 || edge.A == 255 {
		t.Errorf("100 px corner edge %v, want partly transparent", edge)
	}
	// Averaging keeps the mean: a 2:1 box of blue and red is purple.
	if c := s.Render(2).NRGBAAt(1, 1); c.A != 255 {
		t.Errorf("2 px %v", c)
	}
}

func TestSourceErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		ext  string
		want string
	}{
		{"small png", testPNG(t, 256), ".png", "must be at least 512x512"},
		{"not square", func() []byte {
			var buf bytes.Buffer
			png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 600, 512)))
			return buf.Bytes()
		}(), ".png", "an icon is square"},
		{"transparent png", func() []byte {
			var buf bytes.Buffer
			png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 512, 512)))
			return buf.Bytes()
		}(), ".png", "fully transparent"},
		{"garbage png", []byte("nope"), ".png", "not a usable PNG"},
		{"garbage svg", []byte("nope"), ".svg", "not a usable SVG"},
		{"empty svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"/>`), ".svg", "draws nothing"},
		{"other", []byte("x"), ".jpg", "use icon.svg or icon.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(tc.data, tc.ext)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestFindAndLoad(t *testing.T) {
	dir := t.TempDir()
	if p := Find(dir); p != "" {
		t.Errorf("Find in an empty folder: %q", p)
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.png"), testPNG(t, 512), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := Find(dir); filepath.Base(p) != "icon.png" {
		t.Errorf("Find: %q", p)
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(testSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Find(dir)
	if filepath.Base(p) != "icon.svg" {
		t.Errorf("Find with both: %q, want the SVG", p)
	}
	s, err := Load(p)
	if err != nil || s.Path() != p {
		t.Errorf("Load: %v, %v", s, err)
	}
	if _, err := Load(filepath.Join(dir, "missing.svg")); err == nil || !strings.Contains(err.Error(), "-icon") {
		t.Errorf("Load of a missing file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.png"), testPNG(t, 300), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(dir, "bad.png")); err == nil || !strings.Contains(err.Error(), "bad.png") {
		t.Errorf("Load of a small PNG: %v", err)
	}
}

func TestICO(t *testing.T) {
	s, err := Decode([]byte(testSVG), ".svg")
	if err != nil {
		t.Fatal(err)
	}
	ico, err := s.ICO()
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if le.Uint16(ico[0:]) != 0 || le.Uint16(ico[2:]) != 1 || int(le.Uint16(ico[4:])) != len(ICOSizes) {
		t.Fatalf("header % x", ico[:6])
	}
	for i, size := range ICOSizes {
		e := ico[6+16*i:]
		dim := int(e[0])
		if dim == 0 {
			dim = 256
		}
		if dim != size || e[1] != e[0] {
			t.Errorf("entry %d is %d px, want %d", i, dim, size)
		}
		n, off := int(le.Uint32(e[8:])), int(le.Uint32(e[12:]))
		img := decodePNG(t, ico[off:off+n])
		if img.Bounds().Dx() != size {
			t.Errorf("entry %d holds a %v image, want %d px", i, img.Bounds(), size)
		}
	}
}

func TestICNS(t *testing.T) {
	check := func(name string, src []byte, ext string, want []string) {
		t.Run(name, func(t *testing.T) {
			s, err := Decode(src, ext)
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.ICNS()
			if err != nil {
				t.Fatal(err)
			}
			if string(b[:4]) != "icns" || int(binary.BigEndian.Uint32(b[4:])) != len(b) {
				t.Fatalf("header % x, length %d", b[:8], len(b))
			}
			var got []string
			for p := 8; p < len(b); {
				typ := string(b[p : p+4])
				n := int(binary.BigEndian.Uint32(b[p+4:]))
				img := decodePNG(t, b[p+8:p+n])
				for _, e := range icnsTypes {
					if e.typ == typ && img.Bounds().Dx() != e.size {
						t.Errorf("%s holds %v, want %d px", typ, img.Bounds(), e.size)
					}
				}
				got = append(got, typ)
				p += n
			}
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("elements %v, want %v", got, want)
			}
		})
	}
	all := []string{"icp4", "icp5", "ic11", "ic12", "ic07", "ic13", "ic08", "ic14", "ic09", "ic10"}
	check("svg", []byte(testSVG), ".svg", all)
	check("png 1024", testPNG(t, 1024), ".png", all)
	check("png 512", testPNG(t, 512), ".png", all[:9])
}

func TestHicolor(t *testing.T) {
	s, err := Decode([]byte(testSVG), ".svg")
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Hicolor()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != len(HicolorSizes) {
		t.Errorf("%d sizes, want %d", len(m), len(HicolorSizes))
	}
	for size, b := range m {
		if img := decodePNG(t, b); img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Errorf("size %d holds %v", size, img.Bounds())
		}
	}
}
