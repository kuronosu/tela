package text

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font/opentype/tables"
)

// TestCOLRSurvey measures which COLR features Noto Color Emoji uses: paint
// formats, composite modes, gradient kinds and extend modes, counted once
// per glyph that uses them and once per visit (a shared layer counts at
// every use). It is opt-in: TELA_COLR_SURVEY=1 go test -run COLRSurvey -v.
func TestCOLRSurvey(t *testing.T) {
	if os.Getenv("TELA_COLR_SURVEY") == "" {
		t.Skip("set TELA_COLR_SURVEY=1 to survey Noto Color Emoji")
	}
	f := NewDefaultCollection().Face(Query{Families: []string{"emoji"}}, 64)
	colr := f.tf.ft.COLR
	visits := map[string]int{}
	glyphs := map[string]int{}
	var walk func(p tables.PaintTable, seen map[string]bool, depth int)
	note := func(k string, seen map[string]bool) {
		visits[k]++
		seen[k] = true
	}
	walk = func(p tables.PaintTable, seen map[string]bool, depth int) {
		if depth > maxPaintDepth {
			return
		}
		note(strings.TrimPrefix(fmt.Sprintf("%T", p), "tables."), seen)
		switch v := p.(type) {
		case tables.PaintColrLayers:
			ls, _ := colr.LayerList.Resolve(v)
			for _, l := range ls {
				walk(l, seen, depth+1)
			}
		case tables.PaintGlyph:
			walk(v.Paint, seen, depth+1)
		case tables.PaintColrGlyph:
			if pt, ok := colr.Search(tables.GlyphID(v.GlyphID)); ok {
				walk(pt, seen, depth+1)
			}
		case tables.PaintComposite:
			note(fmt.Sprintf("composite mode %d", v.CompositeMode), seen)
			walk(v.SourcePaint, seen, depth+1)
			walk(v.BackdropPaint, seen, depth+1)
		case tables.PaintSolid:
			if v.PaletteIndex == 0xFFFF {
				note("solid foreground", seen)
			}
		case tables.PaintLinearGradient:
			note(fmt.Sprintf("linear extend %d", v.ColorLine.Extend), seen)
		case tables.PaintRadialGradient:
			note(fmt.Sprintf("radial extend %d", v.ColorLine.Extend), seen)
			if v.X0 != v.X1 || v.Y0 != v.Y1 {
				note("radial two-point", seen)
			} else {
				note("radial concentric", seen)
			}
		case tables.PaintSweepGradient:
			note(fmt.Sprintf("sweep extend %d", v.ColorLine.Extend), seen)
		default:
			if _, child, ok := (&painter{f: f, colr: colr}).transformOf(p); ok {
				walk(child, seen, depth+1)
			}
		}
	}
	n, clip := 0, 0
	for g := 0; g < f.tf.nGlyphs; g++ {
		pt, ok := colr.Search(tables.GlyphID(g))
		if !ok {
			continue
		}
		n++
		if _, ok := f.clipBox(tables.GlyphID(g)); ok {
			clip++
		}
		seen := map[string]bool{}
		walk(pt, seen, 0)
		for k := range seen {
			glyphs[k]++
		}
	}
	t.Logf("%d color glyphs, %d with a clip box, CPAL %d palette(s) of %d colors", n, clip, len(f.tf.ft.CPAL), len(f.tf.ft.CPAL[0]))
	keys := make([]string, 0, len(visits))
	for k := range visits {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-36s %6d glyphs %7d visits", k, glyphs[k], visits[k])
	}
}
