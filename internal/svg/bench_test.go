package svg

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Benchmarks of the costs ADR-095 records: parsing, drawing (recording
// commands) and placing a recorded list, for a typical 24x24 stroke icon
// (icon-stroke.svg: a Lucide-style house, three shapes) and an
// illustration (illustration.svg: gradients, a clip path, a mask, layers,
// dashes; 13 shapes).

func benchDoc(b *testing.B, name string) (*Doc, []byte) {
	data, err := os.ReadFile(filepath.Join("testdata", "chromium", "cells", name+".svg"))
	if err != nil {
		b.Fatal(err)
	}
	d, err := Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	return d, data
}

func benchParse(b *testing.B, name string) {
	_, data := benchDoc(b, name)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(data); err != nil {
			b.Fatal(err)
		}
	}
}

func benchDraw(b *testing.B, name string, size float32) {
	d, _ := benchDoc(b, name)
	bld := gfx.NewBuilder()
	opts := Options{CurrentColor: gfx.Black}
	b.ReportAllocs()
	for b.Loop() {
		d.Draw(bld, gfx.RectXYWH(0, 0, size, size), opts)
		bld.Finish()
	}
}

func benchDrawRecorded(b *testing.B, name string, size float32) {
	d, _ := benchDoc(b, name)
	bld := gfx.NewBuilder()
	list := d.Record(Options{CurrentColor: gfx.Black})
	b.ReportAllocs()
	for b.Loop() {
		d.DrawRecorded(bld, gfx.RectXYWH(0, 0, size, size), list)
		bld.Finish()
	}
}

func benchRaster(b *testing.B, name string, size float32, scale float32) {
	d, _ := benchDoc(b, name)
	bld := gfx.NewBuilder()
	d.Draw(bld, gfx.RectXYWH(0, 0, size, size), Options{CurrentColor: gfx.Black})
	list := bld.Finish()
	n := int(size * scale)
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	r := raster.NewRenderer()
	b.ReportAllocs()
	for b.Loop() {
		r.Render(img, list, raster.Options{Scale: scale, Workers: 1})
	}
}

func BenchmarkParseIcon(b *testing.B)                { benchParse(b, "icon-stroke") }
func BenchmarkParseIllustration(b *testing.B)        { benchParse(b, "illustration") }
func BenchmarkDrawIcon(b *testing.B)                 { benchDraw(b, "icon-stroke", 24) }
func BenchmarkDrawIllustration(b *testing.B)         { benchDraw(b, "illustration", 200) }
func BenchmarkDrawRecordedIcon(b *testing.B)         { benchDrawRecorded(b, "icon-stroke", 24) }
func BenchmarkDrawRecordedIllustration(b *testing.B) { benchDrawRecorded(b, "illustration", 200) }
func BenchmarkRasterIcon(b *testing.B)               { benchRaster(b, "icon-stroke", 24, 2) }
func BenchmarkRasterIllustration(b *testing.B)       { benchRaster(b, "illustration", 200, 2) }
