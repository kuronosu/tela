package text

import (
	"testing"

	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
)

// Layout digests, recorded on amd64 (GOAMD64=v1). Every architecture must
// reproduce layoutDigest (ADR-027); check with
//
//	GOARCH=arm64 go test -short -exec qemu-aarch64-static -run Deterministic ./internal/text/
//	GOAMD64=v3 go test -run Deterministic ./internal/text/
//
// The corpus uses variable instances away from the defaults, where
// go-text computes HVAR advances and GPOS variation deltas in float32
// (fusable). Layout takes the nominal advances from Face.Advance
// (fusion-safe) and keeps HarfBuzz's GPOS adjustments, which HarfBuzz
// rounds to whole font units because it shapes at units-per-em scale.
// rawShapingDigest hashes HarfBuzz's own output at that scale; it is
// logged, not checked, to tell whether go-text's float32 code changes
// shaping results on another architecture.

type corpusItem struct {
	text  string
	style TextStyle
	width float32
}

func determinismCorpus() []corpusItem {
	latin := "Typography: AVATAR To Ty Wa, office ﬁ fl -> => 0123456789 (affine) «quotes» — dashes."
	arabic := "العربية هي لغة سامية يتحدث بها أكثر من أربعمائة مليون شخص ١٢٣٤."
	mixed := "Mixed: مرحبا 2024 بكم, 日本語の文章、中文, emoji 😀👨‍👩‍👧‍👦🇯🇵 and more."
	st := func(fams []string, size, weight, stretch float32, ls float32) TextStyle {
		return TextStyle{Families: fams, Size: size, Weight: weight, Stretch: stretch, LetterSpacing: ls, LineHeight: LineHeightMultiple(1.4)}
	}
	inter := []string{"Inter", "Noto Sans Arabic", "Noto Sans JP", "Noto Color Emoji"}
	return []corpusItem{
		{latin, st(inter, 14.5, 333, 0, 0), 180},
		{latin, st(inter, 23, 650, 0, 0.5), 260},
		{latin, st([]string{"Inter"}, 17, 777, 0, 0), 1e6},
		{latin, TextStyle{Families: []string{"Inter"}, Size: 19, Weight: 450, Style: StyleItalic}, 220},
		{arabic, st([]string{"Noto Sans Arabic", "Inter"}, 17, 650, 80, 0), 200},
		{arabic, st([]string{"Noto Sans Arabic", "Inter"}, 13, 333, 93, 0), 150},
		{mixed, st(inter, 16, 560, 0, 0), 240},
		{"func main() { x := []int{1, 2}; return x[0] != 0 } // ->", st([]string{"JetBrains Mono"}, 13.5, 333, 0, 0), 200},
	}
}

func layoutDigest(t testing.TB) uint64 {
	d := newDigest()
	for _, it := range determinismCorpus() {
		ps := ParagraphStyle{Align: AlignJustify}
		if it.text[0] >= 0x80 && []rune(it.text)[0] >= 0x600 && []rune(it.text)[0] < 0x700 {
			ps.Direction = DirectionRTL
		}
		l := simple(t, it.text, ps, it.style, it.width)
		for gi := range l.glyphs {
			g := l.Glyph(gi)
			d.f64(float64(g.ID))
			d.f32(g.Advance, g.XOffset, g.YOffset)
		}
		for ci := range l.NumClusters() {
			c := l.Cluster(ci)
			d.f64(float64(c.TextStart), float64(c.Line))
			d.f32(c.X, c.Advance)
		}
		for li := range l.NumLines() {
			ln := l.Line(li)
			d.f32(ln.Top, ln.Height, ln.Baseline, ln.X, ln.Width, ln.Hanging)
		}
		d.f32(l.MinContentWidth(), l.MaxContentWidth(), l.Width(), l.Height())
	}
	return d.h.Sum64()
}

// rawShapingDigest hashes go-text's HarfBuzz output for the corpus at
// units-per-em scale: glyphs, advances, offsets and the nominal advances
// HarfBuzz reports (go-text's float32 HVAR code).
func rawShapingDigest(t testing.TB) uint64 {
	c := layoutCollection(t)
	d := newDigest()
	shapeMu.Lock()
	defer shapeMu.Unlock()
	buf := harfbuzz.NewBuffer()
	for _, it := range determinismCorpus() {
		rs := []rune(it.text)
		for _, fr := range c.Itemize(rs, 0, len(rs), it.style.query(), it.style.size(), language.Common, nil) {
			hb := fr.Face.hbFont()
			buf.Clear()
			buf.Props.Direction = harfbuzz.LeftToRight
			buf.Props.Script = language.LookupScript(rs[fr.Start])
			if buf.Props.Script == language.Arabic {
				buf.Props.Direction = harfbuzz.RightToLeft
			}
			buf.AddRunes(rs, fr.Start, fr.End-fr.Start)
			buf.Shape(hb, nil)
			for k := range buf.Info {
				p := buf.Pos[k]
				d.f64(float64(buf.Info[k].Glyph), float64(p.XAdvance), float64(p.XOffset), float64(p.YOffset), float64(hb.GlyphHAdvance(buf.Info[k].Glyph)))
			}
		}
	}
	return d.h.Sum64()
}

const (
	wantLayoutDigest     uint64 = 0xc14955264514c21e
	wantRawShapingDigest uint64 = 0x18ac1e3cbb92b19b
)

func TestLayoutDeterministic(t *testing.T) {
	if got := layoutDigest(t); got != wantLayoutDigest {
		t.Errorf("layoutDigest = %#x, want %#x (the same on every architecture)", got, wantLayoutDigest)
	}
	if got := rawShapingDigest(t); got != wantRawShapingDigest {
		t.Logf("go-text's raw HarfBuzz output differs from amd64: digest %#x, recorded %#x", got, wantRawShapingDigest)
	} else {
		t.Logf("go-text's raw HarfBuzz output matches amd64")
	}
}
