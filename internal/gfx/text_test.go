package gfx

import (
	"math"
	"strings"
	"sync/atomic"
	"testing"
)

// boxFont is a minimal Font: glyph g is a square of side g/10 em sitting on
// the baseline; glyph 0 is empty; glyph 99 is a color glyph.
type boxFont struct {
	key    uint64
	bounds atomic.Int64 // GlyphBounds calls
}

func newBoxFont() *boxFont { return &boxFont{key: NewFontKey()} }

func (f *boxFont) FontKey() uint64  { return f.key }
func (f *boxFont) FontName() string { return "Box Sans" }

func (f *boxFont) GlyphOutline(g GlyphID) (Path, bool) {
	if g == 0 || g == 99 {
		return Path{}, false
	}
	return RectPath(f.GlyphBounds(g)), true
}

func (f *boxFont) GlyphBounds(g GlyphID) Rect {
	f.bounds.Add(1)
	switch g {
	case 0:
		return Rect{}
	case 99:
		return Rect{0, -1, 1, 0}
	}
	s := float32(g) / 10
	return Rect{0, -s, s, 0}
}

func (f *boxFont) ColorGlyph(g GlyphID, fg Color) *DisplayList {
	if g != 99 {
		return nil
	}
	b := NewBuilder()
	b.FillRect(Rect{0, -1, 1, 0}, fg)
	return b.Finish()
}

func TestNewFontKey(t *testing.T) {
	a, b := NewFontKey(), NewFontKey()
	if a == 0 || b == 0 || a == b {
		t.Errorf("NewFontKey: %d, %d", a, b)
	}
}

func TestDrawGlyphRunBounds(t *testing.T) {
	f := newBoxFont()
	dl := build(func(b *Builder) {
		b.DrawGlyphRun(DrawGlyphRun{Font: f, Size: 20, Origin: Pt(10, 50), Paint: Black,
			Glyphs: []Glyph{{5, 0, 0}, {0, 10, 0}, {10, 30, -5}}})
	})
	if dl.Len() != 1 {
		t.Fatalf("%d commands, want 1", dl.Len())
	}
	// Glyph 5: [10, 40, 20, 50]; glyph 0: nothing; glyph 10: [40, 25, 60, 45].
	if got, want := dl.BoundsAt(0), (Rect{10, 25, 60, 50}); got != want {
		t.Errorf("bounds %v, want %v", got, want)
	}
	// Under a transform and a clip.
	dl = build(func(b *Builder) {
		b.PushClipRect(Rect{0, 0, 100, 60})
		b.PushTransform(Scale(2, 2))
		b.DrawGlyphRun(DrawGlyphRun{Font: f, Size: 20, Origin: Pt(10, 50), Paint: Black, Glyphs: []Glyph{{5, 0, 0}, {10, 30, -5}}})
		b.PopTransform()
		b.PopClip()
	})
	if got, want := dl.BoundsAt(2), (Rect{20, 50, 100, 60}); got != want {
		t.Errorf("transformed bounds %v, want %v", got, want)
	}
}

func TestDrawGlyphRunCopiesAndDrops(t *testing.T) {
	f := newBoxFont()
	gs := []Glyph{{5, 0, 0}, {7, 10, 0}}
	dl := build(func(b *Builder) {
		b.DrawGlyphRun(DrawGlyphRun{Font: f, Size: 10, Paint: Black, Glyphs: gs})
	})
	gs[0].ID, gs[1].X = 1, 99
	run := dl.At(0).(DrawGlyphRun)
	if run.Glyphs[0].ID != 5 || run.Glyphs[1].X != 10 {
		t.Errorf("the run shares the caller's glyph slice: %v", run.Glyphs)
	}

	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	dropped := []DrawGlyphRun{
		{Font: f, Size: 0, Paint: Black, Glyphs: []Glyph{{5, 0, 0}}},
		{Font: f, Size: -3, Paint: Black, Glyphs: []Glyph{{5, 0, 0}}},
		{Font: f, Size: nan, Paint: Black, Glyphs: []Glyph{{5, 0, 0}}},
		{Font: f, Size: 10, Origin: Pt(inf, 0), Paint: Black, Glyphs: []Glyph{{5, 0, 0}}},
		{Font: f, Size: 10, Paint: Black, Glyphs: []Glyph{{5, 0, 0}, {6, nan, 0}}},
		{Font: f, Size: 10, Paint: Color{nan, 0, 0, 1}, Glyphs: []Glyph{{5, 0, 0}}},
		{Font: f, Size: 10, Paint: Black, Glyphs: nil},
		{Font: f, Size: 10, Paint: Black, Glyphs: []Glyph{{0, 0, 0}, {0, 5, 0}}},            // only empty glyphs
		{Font: f, Size: 10, Origin: Pt(500, 500), Paint: Black, Glyphs: []Glyph{{5, 0, 0}}}, // clipped away
	}
	for i, run := range dropped {
		dl := build(func(b *Builder) {
			b.PushClipRect(Rect{0, -100, 100, 100})
			b.DrawGlyphRun(run)
			b.PopClip()
		})
		if dl.Len() != 0 {
			t.Errorf("run %d: recorded %s", i, dl)
		}
	}

	// A transparent paint is kept (color glyphs paint themselves), clamped.
	for _, p := range []Paint{Transparent, Color{2, -1, 0.5, 0}, LinearGradient{End: Pt(1, 0), Stops: []GradientStop{{0, Transparent}}}} {
		dl := build(func(b *Builder) {
			b.DrawGlyphRun(DrawGlyphRun{Font: f, Size: 10, Paint: p, Glyphs: []Glyph{{99, 0, 0}}})
		})
		if dl.Len() != 1 {
			t.Errorf("paint %v: run dropped", p)
			continue
		}
		if c, ok := dl.At(0).(DrawGlyphRun).Paint.(Color); !ok || c.A != 0 || c.R > 1 || c.G < 0 {
			t.Errorf("paint %v stored as %v", p, dl.At(0).(DrawGlyphRun).Paint)
		}
	}
}

func TestDrawGlyphRunPanics(t *testing.T) {
	f := newBoxFont()
	for name, run := range map[string]DrawGlyphRun{
		"Font is nil":  {Size: 10, Paint: Black},
		"paint is nil": {Font: f, Size: 10},
	} {
		func() {
			defer func() {
				v := recover()
				if s, _ := v.(string); !strings.Contains(s, name) {
					t.Errorf("panic %v, want one mentioning %q", v, name)
				}
			}()
			NewBuilder().DrawGlyphRun(run)
		}()
	}
}

func TestDrawGlyphRunDump(t *testing.T) {
	f := newBoxFont()
	dl := build(func(b *Builder) {
		b.DrawGlyphRun(DrawGlyphRun{Font: f, Size: 14, Origin: Pt(10.5, 20), Paint: MustParseHex("#336699"),
			Glyphs: []Glyph{{5, 0, 0}, {7, 8.25, -1}, {0, 15, 0}}})
	})
	want := `DrawGlyphRun font="Box Sans" size=14 origin=(10.5,20) paint=#336699 glyphs=3[5@(0,0) 7@(8.25,-1) 0@(15,0)] bounds=[10.5,9.2,28.55,20]` + "\n"
	if got := dl.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
