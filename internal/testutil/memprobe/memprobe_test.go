package memprobe

import (
	"runtime"
	"slices"
	"testing"
)

func TestClassify(t *testing.T) {
	const m = "github.com/kuronosu/tela/"
	tests := []struct {
		name  string
		stack []string // innermost first
		want  string
	}{
		{"host framebuffer", []string{"image.NewRGBA", m + "internal/core.resizeFramebuffer", m + "internal/core.(*Host).frame", "testing.tRunner"}, CatFramebuffer},
		{"headless present copy", []string{"image.NewRGBA", m + "internal/platform/headless.(*Window).Present", m + "internal/core.(*Host).frame"}, CatFramebuffer},
		{"rig framebuffer is the harness standing in for a host", []string{"image.NewRGBA", m + "examples/dashboard.newRig", "testing.tRunner"}, CatFramebuffer},
		{"image.NewRGBA of the rasterizer is scratch, not a framebuffer",
			[]string{"image.NewRGBA", m + "internal/gfx/raster.(*Renderer).pushLayer", m + "internal/core.(*Tree).RasterizeFrame", m + "internal/core.(*Host).frame"}, CatRasterBuf},
		{"getBuf under the backdrop filter", []string{"runtime.makeslice", m + "internal/gfx/raster.(*Renderer).getBuf", m + "internal/gfx/raster.(*Renderer).filterBackdrop"}, CatRasterBuf},
		{"backdrop cache beats the rasterizer that calls it", []string{"runtime.makeslice", m + "internal/gfx/raster.(*BackdropCache).store", m + "internal/gfx/raster.(*Renderer).filterBackdrop"}, CatBackdrop},
		{"font parsing", []string{"runtime.makeslice", "github.com/go-text/typesetting/font/opentype/tables.ParseGlyf", "github.com/go-text/typesetting/font.NewFont", m + "internal/text.newTypeface"}, CatFonts},
		{"fonts beat text layout when layout triggers the parse", []string{"github.com/go-text/typesetting/font.NewFace", m + "internal/text.(*Face).ShapingFace", m + "internal/text.NewLayout"}, CatFonts},
		{"glyph slots of a face", []string{"runtime.makeslice", m + "internal/text.(*Typeface).Face", m + "internal/text.instance", m + "internal/text.NewLayout"}, CatGlyphCache},
		{"glyph outlines", []string{m + "internal/gfx.(*PathBuilder).LineTo", m + "internal/text.(*Face).loadGlyph", m + "internal/gfx/raster.(*Renderer).drawGlyph"}, CatGlyphCache},
		{"rendered glyph masks", []string{"runtime.makeslice", m + "internal/gfx/raster.(*Renderer).glyphMask", m + "internal/gfx/raster.(*Renderer).drawGlyph"}, CatGlyphCache},
		{"edge buffer grown while a glyph is rendered", []string{"runtime.growslice", m + "internal/gfx/raster.(*pathFiller).addFixed", m + "internal/gfx/raster.(*Renderer).glyphMask"}, CatRasterBuf},
		{"decoded image", []string{"runtime.makeslice", m + "internal/gfx.NewImage", m + "internal/core.decodeAsset"}, CatImages},
		{"paragraph shaping", []string{"github.com/go-text/typesetting/shaping.(*HarfbuzzShaper).Shape", m + "internal/text.NewLayout", m + "internal/core.(*Tree).commitParagraph"}, CatTextLayout},
		{"display list command", []string{"runtime.growslice", m + "internal/gfx.(*DisplayList).FillRect", m + "internal/core.(*painter).paintBackground"}, CatDisplayList},
		{"element", []string{"runtime.newobject", m + "h.Div", "examples/hello.hello", m + "internal/core.(*Tree).build"}, CatTrees},
		{"nothing known", []string{"runtime.main"}, CatOther},
		{"no frames", nil, CatOther},
		{"the testing frames do not decide", []string{"testing.tRunner", "runtime.goexit"}, CatOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(DefaultRules, tt.stack); got != tt.want {
				t.Errorf("Classify = %q, want %q", got, tt.want)
			}
		})
	}
}

// Every category a rule names is listed in Categories, so the report shows
// it.
func TestRulesUseKnownCategories(t *testing.T) {
	for _, r := range DefaultRules {
		if !slices.Contains(Categories, r.Category) {
			t.Errorf("rule category %q is not in Categories", r.Category)
		}
		if len(r.Prefixes) == 0 {
			t.Errorf("rule %q has no prefixes", r.Category)
		}
	}
}

func TestBreakdownOf(t *testing.T) {
	const m = "github.com/kuronosu/tela/"
	recs := []record{
		{bytes: 100, objects: 1, funcs: []string{m + "internal/core.resizeFramebuffer"}, where: []string{"a"}},
		{bytes: 50, objects: 2, funcs: []string{m + "internal/gfx/raster.(*Renderer).getBuf"}, where: []string{"b"}},
		{bytes: 30, objects: 3, funcs: []string{m + "internal/gfx/raster.(*Renderer).getBuf"}, where: []string{"c"}},
		{bytes: 7, objects: 1, funcs: []string{"runtime.main"}, where: []string{"d"}},
	}
	got := breakdownOf(DefaultRules, recs)
	if got.Total != 187 {
		t.Errorf("Total = %d, want 187", got.Total)
	}
	by := map[string]Category{}
	for _, c := range got.Categories {
		by[c.Name] = c
	}
	if c := by[CatRasterBuf]; c.Bytes != 80 || c.Objects != 5 {
		t.Errorf("raster scratch = %+v, want 80 bytes in 5 objects", c)
	}
	if c := by[CatFramebuffer]; c.Bytes != 100 {
		t.Errorf("framebuffers = %+v, want 100 bytes", c)
	}
	if len(got.Categories) != len(Categories) {
		t.Errorf("%d categories, want all %d", len(got.Categories), len(Categories))
	}
	if got.Top[0].Bytes != 100 || got.Top[0].Category != CatFramebuffer {
		t.Errorf("top site = %+v, want the framebuffer first", got.Top[0])
	}
}

func TestParseStatus(t *testing.T) {
	const status = "Name:\tx\nVmHWM:\t  2048 kB\nVmRSS:\t  1024 kB\nRssAnon:\t   512 kB\nRssFile:\t   256 kB\nRssShmem:\t   128 kB\nBogus\nVmSwap:\t 1 kB\n"
	var s Stats
	parseStatus(status, &s)
	want := Stats{VmHWM: 2048 << 10, VmRSS: 1024 << 10, RssAnon: 512 << 10, RssFile: 256 << 10, RssShmem: 128 << 10}
	if s != want {
		t.Errorf("parseStatus = %+v, want %+v", s, want)
	}
}

func TestSnapshotAndProbe(t *testing.T) {
	p := newProbe()
	buf := make([]byte, 8<<20)
	buf[0] = 1
	p.Sample()
	p.Mark("big")
	s := Snapshot()
	if s.HeapAlloc < 8<<20 || p.peakAlloc < 8<<20 || len(p.marks) != 1 {
		t.Errorf("an 8 MiB buffer is alive: heapAlloc %d, peak %d, marks %d", s.HeapAlloc, p.peakAlloc, len(p.marks))
	}
	if s.TotalAlloc-p.start.TotalAlloc < 8<<20 {
		t.Errorf("TotalAlloc grew by %d, want at least 8 MiB", s.TotalAlloc-p.start.TotalAlloc)
	}
	runtime.KeepAlive(buf)
}
