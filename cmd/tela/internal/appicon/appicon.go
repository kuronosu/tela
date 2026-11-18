// Package appicon turns the app's icon source (ADR-294) into the images
// each system wants: Windows' .ico and its resource entries, macOS's .icns
// and the PNG sizes of the freedesktop hicolor theme.
//
// The source is icon.svg (drawn at every size by Tela's own SVG renderer,
// so small sizes are as sharp as large ones) or icon.png (square, at least
// 512 px, scaled down by averaging the source pixels each destination
// pixel covers). The same source gives the same bytes on every machine:
// the scaling is integer arithmetic, the rasterizer is deterministic
// (ADR-027) and the PNG encoder is the standard library's.
package appicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/svg"
)

// MinPNGSize is the smallest side of a PNG source: the largest image the
// formats hold (512 px for the hicolor theme and the .icns' ic09) comes
// from it without being blown up.
const MinPNGSize = 512

// maxSourceBytes and maxPNGSide bound what Load reads.
const (
	maxSourceBytes = 64 << 20
	maxPNGSide     = 16384
)

// Names are the file names Find looks for, in order.
var Names = []string{"icon.svg", "icon.png"}

// Find returns the path of the icon in dir (icon.svg, else icon.png), or ""
// when the folder has none.
func Find(dir string) string {
	for _, n := range Names {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// Source is a loaded icon source. It is not safe for concurrent use: it
// caches the PNGs it made.
type Source struct {
	path string
	svg  *svg.Doc       // the vector source, or nil
	pix  *pixels        // the raster source, or nil
	png  map[int][]byte // size -> PNG
}

// Load reads the icon at path: an SVG or a PNG, by its extension.
func Load(path string) (*Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("the app icon: %v; put an icon.svg or an icon.png (512 px or more) next to main.go, or pass -icon", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSourceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the app icon %s: %v", path, err)
	}
	if len(data) > maxSourceBytes {
		return nil, fmt.Errorf("the app icon %s is larger than %d MiB; use a smaller file", path, maxSourceBytes>>20)
	}
	s, err := Decode(data, strings.ToLower(filepath.Ext(path)))
	if err != nil {
		return nil, fmt.Errorf("the app icon %s: %v", path, err)
	}
	s.path = path
	return s, nil
}

// Decode reads an icon source of the given kind: ".svg" or ".png".
func Decode(data []byte, ext string) (*Source, error) {
	s := &Source{png: map[int][]byte{}}
	switch ext {
	case ".svg":
		doc, err := svg.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("not a usable SVG (%v); export it as plain SVG without text, images or filters", err)
		}
		s.svg = doc
		if !hasAlpha(s.drawSVG(64)) {
			return nil, errors.New("the SVG draws nothing; check its viewBox and that its shapes have a fill")
		}
	case ".png":
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("not a usable PNG (%v)", err)
		}
		switch {
		case cfg.Width != cfg.Height:
			return nil, fmt.Errorf("the PNG is %dx%d; an icon is square, so crop or pad it to a square (1024x1024 is best)", cfg.Width, cfg.Height)
		case cfg.Width < MinPNGSize:
			return nil, fmt.Errorf("the PNG is %dx%d; it must be at least %dx%d (1024x1024 is best) so the large sizes are not blown up, or use an icon.svg", cfg.Width, cfg.Height, MinPNGSize, MinPNGSize)
		case cfg.Width > maxPNGSide:
			return nil, fmt.Errorf("the PNG is %dx%d; the largest size is %dx%d", cfg.Width, cfg.Height, maxPNGSide, maxPNGSide)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("not a usable PNG (%v)", err)
		}
		s.pix = premultiplied(img)
		if !hasAlphaPix(s.pix) {
			return nil, errors.New("the PNG is fully transparent; check that it is the right file")
		}
	default:
		return nil, fmt.Errorf("unknown icon type %q; use icon.svg or icon.png", ext)
	}
	return s, nil
}

// Path returns where the source was loaded from ("" for Decode).
func (s *Source) Path() string { return s.path }

// MaxSize returns the largest size worth rendering: the side of a PNG
// source, and 0 (no limit) for an SVG.
func (s *Source) MaxSize() int {
	if s.pix != nil {
		return s.pix.w
	}
	return 0
}

// Render returns the icon at size x size pixels, not premultiplied. A PNG
// source scaled up is only as sharp as the source: callers keep to
// MaxSize.
func (s *Source) Render(size int) *image.NRGBA {
	if size < 1 {
		panic(fmt.Sprintf("appicon: size %d is not positive", size))
	}
	var rgba *image.RGBA
	if s.svg != nil {
		rgba = s.drawSVG(size)
	} else {
		rgba = s.pix.scale(size)
	}
	out := image.NewNRGBA(rgba.Rect)
	for i := 0; i < len(rgba.Pix); i += 4 {
		c := color.NRGBAModel.Convert(color.RGBA{rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3]}).(color.NRGBA)
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return out
}

// drawSVG renders the vector source into a transparent size x size image.
func (s *Source) drawSVG(size int) *image.RGBA {
	b := gfx.NewBuilder()
	s.svg.Draw(b, gfx.RectXYWH(0, 0, float32(size), float32(size)), svg.Options{CurrentColor: gfx.Black})
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	clear := gfx.Transparent
	raster.Render(img, b.Finish(), raster.Options{Clear: &clear})
	return img
}

func hasAlpha(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			return true
		}
	}
	return false
}

// PNG returns the icon at size x size pixels as a PNG file.
func (s *Source) PNG(size int) ([]byte, error) {
	if b, ok := s.png[size]; ok {
		return b, nil
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, s.Render(size)); err != nil {
		return nil, err
	}
	s.png[size] = buf.Bytes()
	return s.png[size], nil
}

// ICOSizes are the images of an .ico: all PNG-compressed, 16 to 256.
var ICOSizes = []int{16, 24, 32, 48, 64, 128, 256}

// Image is one size of the icon as a PNG file.
type Image struct {
	Size int
	PNG  []byte
}

// ICOImages returns the images of ICOSizes.
func (s *Source) ICOImages() ([]Image, error) {
	var out []Image
	for _, n := range ICOSizes {
		b, err := s.PNG(n)
		if err != nil {
			return nil, err
		}
		out = append(out, Image{n, b})
	}
	return out, nil
}

// ICO returns the icon as a Windows .ico file: the PNG-compressed images of
// ICOSizes (Windows Vista and later).
func (s *Source) ICO() ([]byte, error) {
	imgs, err := s.ICOImages()
	if err != nil {
		return nil, err
	}
	// ICONDIR, then one ICONDIRENTRY per image, then the images.
	var buf bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(imgs))})
	off := 6 + 16*len(imgs)
	for _, im := range imgs {
		dim := byte(im.Size) // 256 is 0
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, le, [2]uint16{1, 32}) // planes, bits per pixel
		binary.Write(&buf, le, [2]uint32{uint32(len(im.PNG)), uint32(off)})
		off += len(im.PNG)
	}
	for _, im := range imgs {
		buf.Write(im.PNG)
	}
	return buf.Bytes(), nil
}

// icnsTypes are the PNG elements of an .icns file, by the size of their
// image, in the order they are written: icp4 and icp5 are the 16 and 32 px
// icons, ic11 and ic12 the 16 and 32 px icons at twice the resolution (32
// and 64 px), ic07 to ic10 the 128, 256, 512 and 1024 px icons, ic13 and
// ic14 the 128 and 256 px ones at twice the resolution (256 and 512 px).
var icnsTypes = []struct {
	typ  string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"ic11", 32}, {"ic12", 64}, {"ic07", 128},
	{"ic13", 256}, {"ic08", 256}, {"ic14", 512}, {"ic09", 512}, {"ic10", 1024},
}

// ICNS returns the icon as a macOS .icns file. Its elements are PNG
// images; the 1024 px one (ic10) is left out of a PNG source smaller than
// that, since macOS scales the 512 px one at least as well.
func (s *Source) ICNS() ([]byte, error) {
	var body bytes.Buffer
	for _, e := range icnsTypes {
		if m := s.MaxSize(); m != 0 && e.size > m {
			continue
		}
		b, err := s.PNG(e.size)
		if err != nil {
			return nil, err
		}
		body.WriteString(e.typ)
		binary.Write(&body, binary.BigEndian, uint32(8+len(b)))
		body.Write(b)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// HicolorSizes are the sizes of the freedesktop icon theme's PNG icons
// (hicolor/<n>x<n>/apps).
var HicolorSizes = []int{16, 22, 24, 32, 48, 64, 128, 256, 512}

// Hicolor returns the PNG icons of the hicolor theme by size.
func (s *Source) Hicolor() (map[int][]byte, error) {
	out := map[int][]byte{}
	for _, n := range HicolorSizes {
		b, err := s.PNG(n)
		if err != nil {
			return nil, err
		}
		out[n] = b
	}
	return out, nil
}

// pixels is a raster source: premultiplied 16-bit channels, RGBA order.
type pixels struct {
	w, h int
	pix  []uint16
}

func premultiplied(img image.Image) *pixels {
	b := img.Bounds()
	p := &pixels{w: b.Dx(), h: b.Dy(), pix: make([]uint16, 4*b.Dx()*b.Dy())}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			p.pix[i], p.pix[i+1], p.pix[i+2], p.pix[i+3] = uint16(r), uint16(g), uint16(bl), uint16(a)
			i += 4
		}
	}
	return p
}

func hasAlphaPix(p *pixels) bool {
	for i := 3; i < len(p.pix); i += 4 {
		if p.pix[i] != 0 {
			return true
		}
	}
	return false
}

// scale returns the source at size x size premultiplied 8-bit pixels: each
// destination pixel is the average of the source area it covers (a box
// filter with exact fractional edges), in integer arithmetic, so the
// result does not depend on the machine.
func (p *pixels) scale(size int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	if size == p.w && size == p.h {
		for i, v := range p.pix {
			out.Pix[i] = uint8((uint32(v)*255 + 32767) / 65535)
		}
		return out
	}
	// Horizontal pass: for each source row, the sums over the source
	// columns of each destination column, weighted by the overlap.
	// Source column i covers [i*size, (i+1)*size) in units of 1/size of a
	// destination pixel; destination column x covers [x*w, (x+1)*w).
	w, h := uint64(p.w), uint64(p.h)
	n := uint64(size)
	rows := make([]uint64, p.h*size*4)
	for y := 0; y < p.h; y++ {
		for x := 0; x < size; x++ {
			lo, hi := uint64(x)*w, uint64(x+1)*w
			var sum [4]uint64
			for i := lo / n; i*n < hi; i++ {
				wt := min(hi, (i+1)*n) - max(lo, i*n)
				src := p.pix[(y*p.w+int(i))*4:]
				sum[0] += uint64(src[0]) * wt
				sum[1] += uint64(src[1]) * wt
				sum[2] += uint64(src[2]) * wt
				sum[3] += uint64(src[3]) * wt
			}
			copy(rows[(y*size+x)*4:], sum[:])
		}
	}
	// Vertical pass.
	den := w * h
	for y := 0; y < size; y++ {
		lo, hi := uint64(y)*h, uint64(y+1)*h
		for x := 0; x < size; x++ {
			var sum [4]uint64
			for j := lo / n; j*n < hi; j++ {
				wt := min(hi, (j+1)*n) - max(lo, j*n)
				src := rows[(int(j)*size+x)*4:]
				sum[0] += src[0] * wt
				sum[1] += src[1] * wt
				sum[2] += src[2] * wt
				sum[3] += src[3] * wt
			}
			for c := range sum {
				v16 := (sum[c] + den/2) / den // 0..65535
				out.Pix[(y*size+x)*4+c] = uint8((v16*255 + 32767) / 65535)
			}
		}
	}
	return out
}
