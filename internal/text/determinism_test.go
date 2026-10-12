package text

import (
	"hash"
	"hash/fnv"
	"math"
	"testing"

	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/gfx"
)

// The digests below were recorded on amd64 (GOAMD64=v1, where Go never
// fuses multiply-adds). Every architecture must reproduce them (ADR-027);
// check with
//
//	GOARCH=arm64 go test -short -exec qemu-aarch64-static -run Deterministic ./internal/text/
//	GOAMD64=v3 go test -run Deterministic ./internal/text/
//
// go-text's own variable outlines fail this check (its gvar code fuses
// under GOAMD64=v3), which is why glyf.go and vardelta.go exist.

type digest struct{ h hash.Hash64 }

func newDigest() *digest { return &digest{fnv.New64a()} }

func (d *digest) f64(vs ...float64) {
	var b [8]byte
	for _, v := range vs {
		u := math.Float64bits(v)
		for i := range b {
			b[i] = byte(u >> (8 * i))
		}
		d.h.Write(b[:])
	}
}

func (d *digest) f32(vs ...float32) {
	for _, v := range vs {
		d.f64(float64(math.Float32frombits(math.Float32bits(v))))
	}
}

func (d *digest) path(p gfx.Path) {
	for _, v := range p.Verbs() {
		d.f64(float64(v))
	}
	for _, q := range p.Points() {
		d.f32(q.X, q.Y)
	}
}

func (d *digest) paint(p gfx.Paint) {
	stops := func(s []gfx.GradientStop) {
		for _, st := range s {
			d.f32(st.Offset, st.Color.R, st.Color.G, st.Color.B, st.Color.A)
		}
	}
	switch v := p.(type) {
	case gfx.Color:
		d.f32(v.R, v.G, v.B, v.A)
	case gfx.LinearGradient:
		d.f32(v.Start.X, v.Start.Y, v.End.X, v.End.Y)
		stops(v.Stops)
	case gfx.RadialGradient:
		d.f32(v.Center.X, v.Center.Y, v.Radius.X, v.Radius.Y)
		stops(v.Stops)
	case gfx.ConicGradient:
		d.f32(v.Center.X, v.Center.Y, v.StartAngle)
		stops(v.Stops)
	}
}

func (d *digest) list(l *gfx.DisplayList) {
	for _, c := range l.All() {
		d.h.Write([]byte(c.Name()))
		switch v := c.(type) {
		case gfx.FillRect:
			d.f32(v.Rect.X0, v.Rect.Y0, v.Rect.X1, v.Rect.Y1)
			d.paint(v.Paint)
		case gfx.FillPath:
			d.path(v.Path)
			d.paint(v.Paint)
		case gfx.PushClipRect:
			d.f32(v.Rect.X0, v.Rect.Y0, v.Rect.X1, v.Rect.Y1)
		case gfx.PushClipPath:
			d.path(v.Path)
		case gfx.PushTransform:
			d.f32(v.Affine[:]...)
		case gfx.PushLayer:
			d.f32(v.Opacity)
			d.f64(float64(v.Blend))
		}
	}
}

// outlineDigest hashes, for faces at non-default coordinates, the float64
// glyf points (before their rounding to float32, which would hide most
// last-bit differences), the em-space outlines, the advances, the
// unrounded HVAR deltas and the metrics.
func outlineDigest(t testing.TB) uint64 {
	c := NewDefaultCollection()
	d := newDigest()
	faces := []*Face{
		c.Face(Query{Families: []string{"Inter"}, Weight: 650}, 20),
		c.Face(Query{Families: []string{"Inter"}, Weight: 333, Style: StyleItalic}, 31),
		c.Face(Query{Families: []string{"Noto Sans Arabic"}, Weight: 650, Stretch: 80}, 16),
		c.Face(Query{Families: []string{"JetBrains Mono"}, Weight: 333}, 16),
	}
	for _, f := range faces {
		if f.coords == nil {
			t.Fatalf("%s: default instance; the digest needs variations", f.FontName())
		}
		m := f.Metrics()
		d.f32(m.Ascent, m.Descent, m.LineGap, m.CapHeight, m.XHeight, m.UnderlineOffset, m.UnderlineThickness, m.StrikeoutOffset, m.StrikeoutThickness)
		tf := f.tf
		for g := 0; g < tf.nGlyphs; g++ {
			for _, p := range tf.glyf.points(tables.GlyphID(g), f.coords) {
				d.f64(p.x, p.y)
			}
			if tf.hvar != nil {
				d.f64(varStoreDelta(&tf.hvar.ItemVariationStore, advanceIndex(&tf.hvar.AdvanceWidthMapping, tables.GlyphID(g)), f.coords))
			}
			p, _ := f.GlyphOutline(gfx.GlyphID(g))
			d.path(p)
			b := f.GlyphBounds(gfx.GlyphID(g))
			d.f32(f.Advance(gfx.GlyphID(g)), b.X0, b.Y0, b.X1, b.Y1)
		}
	}
	return d.h.Sum64()
}

// colorDigest hashes the display lists of color glyphs: the golden emoji,
// the emoji that use two-point radial gradients and three flags (blend
// composites).
func colorDigest(t testing.TB) uint64 {
	f := NewDefaultCollection().Face(Query{Families: []string{"emoji"}}, 64)
	d := newDigest()
	seqs := append(append([]string(nil), goldenEmoji...), "⛈", "🌁", "🌄", "🏙", "💟", "🗻", "🦟", "🇧🇷", "🇩🇪")
	for _, s := range seqs {
		g := shapedGlyph(t, f, s)
		l := f.ColorGlyph(g, gfx.RGB8(0x11, 0x18, 0x27))
		if l == nil {
			t.Fatalf("%s: not a color glyph", s)
		}
		d.list(l)
		b := f.GlyphBounds(g)
		d.f32(b.X0, b.Y0, b.X1, b.Y1)
	}
	return d.h.Sum64()
}

const (
	wantOutlineDigest uint64 = 0x2d8d5f094f0b0d96
	wantColorDigest   uint64 = 0x5e5f32fd34bd8708
)

func TestOutlinesDeterministic(t *testing.T) {
	if got := outlineDigest(t); got != wantOutlineDigest {
		t.Errorf("outlineDigest = %#x, want %#x (the same on every architecture)", got, wantOutlineDigest)
	}
}

func TestColorGlyphsDeterministic(t *testing.T) {
	if got := colorDigest(t); got != wantColorDigest {
		t.Errorf("colorDigest = %#x, want %#x (the same on every architecture)", got, wantColorDigest)
	}
}
