package core

import (
	"fmt"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// inputBaselineCases are single-line inputs measured in Chromium 154
// (headless, Inter through @font-face, display: block, box-sizing:
// border-box, no border, 12 px of horizontal padding, 120 px wide, in a
// 50 px tall container, the value "HHH"): the device row
// of the bottom of an "H" of the value, counted from the input's top (the
// baseline is the row below it), and the input's height in CSS px, at
// display scales 1, 1.5 and 2. Chromium's inner editor drops the input's
// line-height (line-height: normal) when the input's height is a
// percentage or a length larger than the line height, and when the font
// size is at least the line height; its line is centered in the content
// box (field.textStyle, ADR-159).
var inputBaselineCases = []struct {
	size   int
	lh     string // the leading-* class
	height string // the h-* class ("" for auto)
	py     int
	row    [3]int
	h      [3]float32
}{
	{14, "leading-[20px]", "h-[36px]", 4, [3]int{23, 34, 45}, [3]float32{36, 36, 36}}, // kit's Input: h-9 py-1 md:text-sm
	{14, "leading-[20px]", "h-[37px]", 4, [3]int{23, 34, 46}, [3]float32{37, 37, 37}},
	{14, "leading-[20px]", "h-[40px]", 8, [3]int{25, 37, 49}, [3]float32{40, 40, 40}},
	{14, "leading-none", "h-[36px]", 4, [3]int{23, 34, 45}, [3]float32{36, 36, 36}},
	{16, "leading-[24px]", "h-[36px]", 4, [3]int{23, 35, 47}, [3]float32{36, 36, 36}},
	{16, "leading-[24px]", "h-[37px]", 4, [3]int{24, 35, 48}, [3]float32{37, 37, 37}},
	{13, "leading-[20px]", "h-[36px]", 4, [3]int{22, 33, 45}, [3]float32{36, 36, 36}},
	{12, "leading-[16px]", "h-[36px]", 4, [3]int{22, 33, 44}, [3]float32{36, 36, 36}},
	{16, "leading-none", "h-[37px]", 4, [3]int{24, 35, 48}, [3]float32{37, 37, 37}},
	{14, "leading-[30px]", "", 4, [3]int{23, 35, 47}, [3]float32{38, 38, 38}},
	{14, "leading-[20px]", "", 4, [3]int{18, 27, 37}, [3]float32{28, 28, 28}},
	{16, "leading-[24px]", "", 4, [3]int{21, 31, 42}, [3]float32{32, 32, 32}},
	{14, "leading-[30px]", "h-[30px]", 0, [3]int{19, 29, 39}, [3]float32{30, 30, 30}}, // not taller: kept
	{14, "leading-[30px]", "h-[31px]", 0, [3]int{20, 30, 40}, [3]float32{31, 31, 31}},
	{14, "leading-[30px]", "h-[38px]", 4, [3]int{24, 35, 47}, [3]float32{38, 38, 38}},
	{14, "leading-[30px]", "h-1/2", 4, [3]int{17, 25, 34}, [3]float32{25, 25, 25}},
	{14, "leading-[30px]", "h-[calc(100%-2px)]", 4, [3]int{29, 43, 57}, [3]float32{48, 48, 48}},
	{14, "leading-none", "", 4, [3]int{17, 25, 34}, [3]float32{25, 24.6667, 25}},     // font size >= line height
	{14, "leading-[14.5px]", "", 4, [3]int{17, 25, 31}, [3]float32{25, 24.6667, 22.5}}, // 14.5 px: 14 at x1, 21.75 at x1.5, but 29 at x2
	{14, "leading-[15px]", "", 4, [3]int{16, 23, 32}, [3]float32{23, 23, 23}},
	{16, "leading-[16px]", "", 4, [3]int{19, 28, 38}, [3]float32{28, 27.3333, 27.5}},
	{16, "leading-[17px]", "", 4, [3]int{17, 26, 35}, [3]float32{25, 25, 25}},
}

func TestChromiumInputBaselines(t *testing.T) {
	for _, c := range inputBaselineCases {
		for si, s := range []float32{1, 1.5, 2} {
			name := fmt.Sprintf("x%g text-[%dpx] %s %s py-[%dpx]", s, c.size, c.lh, c.height, c.py)
			cls := fmt.Sprintf("block box-border border-0 w-[120px] px-3 py-[%dpx] text-[%dpx] %s %s", c.py, c.size, c.lh, c.height)
			tr := NewTree(Config{Fonts: testFonts})
			tr.SetViewport(gfx.Size{W: 200, H: 100})
			tr.SetScale(s)
			tr.Mount(div(Class("h-[50px] w-[150px]"), input(Key("in"), Class(cls), DefaultValue("HHH"))))
			tr.Frame()
			f := fieldOf(t, tr, "in")
			h := f.bx.ld.Layout().Size.H
			_, oy := f.textOrigin()
			base := float64(oy+f.layout().Line(0).Baseline) * float64(s)
			// The glyphs' baseline snaps to the nearest device row, a half
			// rounding down, as in Skia (raster, ADR-029). Chromium's
			// positions are LayoutUnits, Tela's float32 logical px: at
			// scale 1.5 a baseline Chromium has at 35.5 device px is
			// 35.49999 here, so the row is accepted within 1e-3 px.
			row := func(eps float64) int { return int(math.Floor(base+0.5+eps)) - 1 }
			if (row(1e-3) != c.row[si] && row(-1e-3) != c.row[si]) || math.Abs(float64(h-c.h[si])) > 0.001 {
				t.Errorf("%s: baseline at %.4f device px (H bottom row %d), height %.4f; Chromium's row %d, height %.4f", name, base, row(0), h, c.row[si], c.h[si])
			}
		}
	}
}

// TestSetScaleRelaysText checks that a new display scale lays paragraphs
// and fields out again with line boxes quantized for it (ADR-159): the
// baseline of 14 px Inter with leading-snug (19.25 px) is at 15 px at
// scale 1, 21 device px (14 px) at scale 1.5 and 29 device px (14.5 px)
// at scale 2; an Input with leading-[14.5px] lays its line out with
// line-height: normal at scales 1 and 1.5 (17 px, 25 device px) but keeps
// it at scale 2 (29 device px, more than its 28 px font).
func TestSetScaleRelaysText(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 300, H: 200})
	tr.Mount(div(
		div(Key("p"), Class("text-[14px] leading-snug"), Text("Hxgy")),
		input(Key("in"), Class("block box-border border-0 p-0 text-[14px] leading-[14.5px]"), DefaultValue("Hxgy")),
	))
	tr.Frame()
	height := func(k string) float32 { return keyed(tr, k).box.ld.Layout().Size.H }
	for _, c := range []struct {
		scale  float32
		p, in  float32
		pBase  float32
		inBase float32
	}{
		{1, 19.25, 17, 15, 14},
		{1.5, 19.25, 25 / 1.5, 14, 20 / 1.5},
		{2, 19.25, 14.5, 14.5, 12},
		{1, 19.25, 17, 15, 14},
	} {
		tr.SetScale(c.scale)
		tr.Frame()
		para := keyed(tr, "p").box.para
		f := fieldOf(t, tr, "in")
		_, oy := f.textOrigin()
		got := [4]float32{height("p"), height("in"), para.lay.Line(0).Baseline, oy + f.layout().Line(0).Baseline}
		want := [4]float32{c.p, c.in, c.pBase, c.inBase}
		for i := range got {
			if math.Abs(float64(got[i]-want[i])) > 1e-4 {
				t.Errorf("scale %g: paragraph height, input height, paragraph baseline, input baseline %v, want %v", c.scale, got, want)
				break
			}
		}
	}
}
