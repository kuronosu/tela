package svg

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// FuzzParse checks that no input makes Parse, Draw or the rasterizer
// panic, hang or build unbounded lists. Run it with
//
//	go test -run '^$' -fuzz FuzzParse -parallel 2 ./internal/svg
func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "chromium", "cells", "*.svg"))
	for _, name := range files {
		if data, err := os.ReadFile(name); err == nil {
			f.Add(data)
		}
	}
	for _, c := range inlineCells {
		f.Add([]byte(c.inline))
	}
	for _, s := range []string{
		`<svg><path d="M0 0 A 1e9 1e-9 360 1 1 1e9 -1e9 Z"/></svg>`,
		`<svg viewBox="0 0 1e-30 1e-30"><circle r="1e30"/></svg>`,
		`<svg><use href="#a" id="a"/><g id="b"><use href="#b"/></g></svg>`,
		`<svg><linearGradient id="g" href="#g" gradientTransform="scale(0)"><stop/></linearGradient><rect width="1" height="1" fill="url(#g)"/></svg>`,
		`<svg><clipPath id="c" clip-path="url(#c)"><rect width="1" height="1" clip-path="url(#c)"/></clipPath><rect width="1" height="1" clip-path="url(#c)"/></svg>`,
		`<svg><mask id="m"><rect width="1" height="1" mask="url(#m)"/></mask><rect width="1" height="1" mask="url(#m)"/></svg>`,
		`<svg><rect width="10" height="10" stroke="#000" stroke-dasharray="1e-9" stroke-width="1e9"/></svg>`,
		`<svg><polyline points="0,0 1e9,1e9 -1e9,1e9" stroke="#000" stroke-linejoin="miter" stroke-miterlimit="1e9"/></svg>`,
	} {
		f.Add([]byte(s))
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		start := time.Now()
		defer func() {
			if el := time.Since(start); el > 5*time.Second {
				t.Fatalf("slow input: %v", el)
			}
		}()
		d, err := Parse(data)
		if err != nil {
			return
		}
		d.Size()
		d.Intrinsic()
		for _, opts := range []Options{
			{CurrentColor: gfx.Black},
			{Root: Root{Fill: Paint{Kind: PaintCurrentColor}, Stroke: Paint{Kind: PaintColor, Color: gfx.White}, StrokeWidth: 2, HasStrokeWidth: true, Color: gfx.White, HasColor: true}},
		} {
			b := gfx.NewBuilder()
			d.Draw(b, gfx.RectXYWH(0, 0, 32, 32), opts)
			l := b.Finish()
			if l.Len() > 1<<20 {
				t.Fatalf("%d commands", l.Len())
			}
			d.Record(opts)
			raster.Render(img, l, raster.Options{Scale: 1})
		}
	})
}
