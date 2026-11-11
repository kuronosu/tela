package text

import (
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The hyphenation golden draws paragraphs with soft hyphens and hyphens:
// auto (English, Spanish, French; alignments, decorations, spans, right to
// left, balance, line-clamp) here and in Chromium, at scales 1 and 2,
// and compares each paragraph's pixels; Tela's pictures are also goldens
// of their own (testdata/hyphens_x1.png, hyphens_x2.png).
//
// The screenshots are committed. To regenerate them (with Chromium's
// hyphenation dictionaries, see chromium_hyphen_test.go):
//
//	TELA_CHROMIUM=1 TELA_CHROMIUM_HYPHEN=~/.config/chromium/hyphen-data/120.0.6050.0 \
//	  go test -run TestChromiumHyphenPixels ./internal/text/

const hyPageW, hyPageH = 780, 420

type hyCell struct {
	x, y, w float32
	para    ParagraphStyle
	spans   []chromiumSpan
}

var (
	hyInk    = gfx.RGB8(0x11, 0x18, 0x27)
	hyAccent = gfx.RGB8(0x4f, 0x46, 0xe5)
	hyRed    = gfx.RGB8(0xdc, 0x26, 0x26)
)

// hyText is a style of the golden: 16 px Inter (and the test fallbacks),
// line height 24, ink color.
func hyText(h Hyphens, lang string, mods ...func(*TextStyle)) TextStyle {
	return hyStyle(16, h, lang, append([]func(*TextStyle){func(s *TextStyle) { s.Paint = hyInk }}, mods...)...)
}

func hyCells() []hyCell {
	shy := hyText(HyphensManual, "")
	en := hyText(HyphensAuto, "en")
	under := func(s *TextStyle) {
		s.Decoration = TextDecoration{Lines: DecorationUnderline, Paint: hyAccent}
	}
	return []hyCell{
		{x: 16, y: 16, w: 150, spans: one1(shyExtra+" and soft\u00adhyphen\u00adated words", shy)},
		{x: 200, y: 16, w: 130, spans: one1(autoText, hyText(HyphensAuto, "en", under))},
		{x: 364, y: 16, w: 170, para: ParagraphStyle{Align: AlignJustify}, spans: one1(autoES, hyText(HyphensAuto, "es"))},
		{x: 568, y: 16, w: 190, para: ParagraphStyle{Align: AlignCenter}, spans: one1(autoFR, hyText(HyphensAuto, "fr"))},
		{x: 16, y: 230, w: 150, spans: []chromiumSpan{
			{text: "Ex\u00adtra\u00ador", style: shy},
			{text: "\u00addi\u00adnary in\u00adter", style: hyText(HyphensManual, "", func(s *TextStyle) { s.Weight = 700; s.Paint = hyRed })},
			{text: "\u00adna\u00adtion\u00adal", style: hyText(HyphensManual, "", func(s *TextStyle) { s.Paint = hyAccent })},
			{text: "\u00adiza\u00adtion of type", style: shy},
		}},
		{x: 200, y: 230, w: 90, para: ParagraphStyle{Direction: DirectionRTL}, spans: one1("العربية\u00adهي لغة سا\u00adمية يتحدث\u00adبها أكثر من أربعمائة", shy)},
		{x: 364, y: 230, w: 200, para: ParagraphStyle{WrapStyle: TextWrapBalance}, spans: one1(autoText, en)},
		{x: 600, y: 230, w: 140, para: ParagraphStyle{LineClamp: 3}, spans: one1(autoText, hyText(HyphensAuto, "en", func(s *TextStyle) { s.LetterSpacing = 1 }))},
	}
}

func (c *hyCell) paragraph(scale float32) *Paragraph {
	p := &Paragraph{Style: c.para, Default: c.spans[0].style, Scale: scale}
	p.Default.Decoration = TextDecoration{}
	for _, s := range c.spans {
		p.AddText(s.text, s.style)
	}
	return p
}

// cssColor returns a CSS color of a solid paint.
func cssColor(p gfx.Paint) string {
	c, _ := p.(gfx.Color)
	r, g, b, _ := c.Clamp().NRGBA8().RGBA()
	return fmt.Sprintf("rgb(%d, %d, %d)", r>>8, g>>8, b>>8)
}

func hyPixelsHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(chromiumFonts)
	fmt.Fprintf(&b, "\nhtml, body { margin: 0; padding: 0; overflow: hidden; background: #fff; width: %dpx; height: %dpx; }\n.p { position: absolute; }\n</style></head><body>\n", hyPageW, hyPageH)
	for _, c := range hyCells() {
		base := c.spans[0].style
		lang := ""
		if base.Language != "" {
			lang = fmt.Sprintf(" lang=%q", base.Language)
		}
		css := fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; %s %s%s color: %s;", c.x, c.y, c.w, cssPara(c.para), cssStyle(base), cssHyphens(base.Hyphens), cssColor(base.Paint))
		css += [...]string{"", " text-wrap-style: balance;", " text-wrap-style: pretty;"}[c.para.WrapStyle]
		if k := c.para.LineClamp; k > 0 {
			css += fmt.Sprintf(" overflow: hidden; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: %d;", k)
		}
		fmt.Fprintf(&b, "<div class=\"p\"%s style=\"%s\">", lang, html.EscapeString(css))
		for _, s := range c.spans {
			st := cssStyle(s.style) + cssHyphens(s.style.Hyphens) + " color: " + cssColor(s.style.Paint) + ";"
			if d := s.style.Decoration; d.Lines&DecorationUnderline != 0 {
				st += " text-decoration: underline " + cssColor(d.Paint) + ";"
			}
			fmt.Fprintf(&b, "<span style=\"%s\">%s</span>", html.EscapeString(st), strings.ReplaceAll(html.EscapeString(s.text), "\u00ad", "&shy;"))
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</body></html>\n")
	return b.String()
}

// renderHyphenScene draws the cells at scale, and returns their rects in
// logical px.
func renderHyphenScene(t testing.TB, scale float32) (*image.RGBA, []image.Rectangle) {
	coll := hyphenCollection(t)
	b := gfx.NewBuilder()
	var rects []image.Rectangle
	for _, c := range hyCells() {
		l := NewLayout(coll, c.paragraph(scale))
		l.BreakLines(c.w)
		l.PaintScaled(b, gfx.Pt(c.x, c.y), scale)
		rects = append(rects, image.Rect(int(c.x)-6, int(c.y)-2, int(c.x+c.w)+6, int(c.y+l.Height())+4))
	}
	img := image.NewRGBA(image.Rect(0, 0, int(hyPageW*scale), int(hyPageH*scale)))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{Scale: scale})
	return img, rects
}

// regenerateHyphenPixels writes hyphen_pixels.html and Chromium's
// screenshots of it at scales 1 and 2.
func regenerateHyphenPixels(dir, hyphenDir string) error {
	htmlPath := filepath.Join(dir, "hyphen_pixels.html")
	if err := os.WriteFile(htmlPath, []byte(hyPixelsHTML()), 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(htmlPath)
	for _, scale := range []int{1, 2} {
		profile, err := os.MkdirTemp("", "tela-chromium-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(profile)
		if hyphenDir != "" {
			if err := os.CopyFS(filepath.Join(profile, "hyphen-data", filepath.Base(hyphenDir)), os.DirFS(hyphenDir)); err != nil {
				return err
			}
		}
		shot, _ := filepath.Abs(filepath.Join(dir, fmt.Sprintf("hyphen_pixels_x%d.png", scale)))
		cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--hide-scrollbars", "--force-color-profile=srgb",
			fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", hyPageW, hyPageH),
			"--user-data-dir="+profile, "--allow-file-access-from-files", "--virtual-time-budget=10000",
			"--screenshot="+shot, "file://"+abs)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("chromium: %v\n%s", err, out)
		}
	}
	return nil
}

// Tolerances of a paragraph's pixels: the mean channel difference, the
// share of channels more than 32 levels off and the ink, relative to
// Chromium's. The paragraphs are dense text, and the differences are
// those of glyph rendering: Skia samples coverage and FreeType hints the
// outlines (at scale 2 it snaps horizontal stems, crossbars and the
// hyphen to whole pixels), raster computes exact areas of unhinted
// outlines. Measured with Chromium 155: means of 2.0 to 3.2 at scale 1
// and 2.3 to 4.1 at scale 2, 2.2% to 4.6% of the channels more than 32
// levels off, ink within 2.0%; line breaks and positions are the same
// (TestChromiumHyphens).
var (
	hyPixelsMaxMean = map[float32]float64{1: 3.5, 2: 4.6}
	hyPixelsMaxFar  = 0.055
	hyPixelsMaxInk  = 0.03
)

func TestChromiumHyphenPixels(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := regenerateHyphenPixels(dir, os.Getenv("TELA_CHROMIUM_HYPHEN")); err != nil {
			t.Fatal(err)
		}
	}
	if h, err := os.ReadFile(filepath.Join(dir, "hyphen_pixels.html")); err != nil || string(h) != hyPixelsHTML() {
		t.Fatal("testdata/chromium/hyphen_pixels.html is out of date: the cells changed; regenerate with TELA_CHROMIUM=1")
	}
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			fh, err := os.Open(filepath.Join(dir, fmt.Sprintf("hyphen_pixels_x%g.png", scale)))
			if err != nil {
				t.Fatal(err)
			}
			ref, err := png.Decode(fh)
			fh.Close()
			if err != nil {
				t.Fatal(err)
			}
			got, rects := renderHyphenScene(t, scale)
			golden.Image(t, fmt.Sprintf("hyphens_x%g", scale), got)
			for i, r := range rects {
				r = image.Rect(int(float32(r.Min.X)*scale), int(float32(r.Min.Y)*scale), int(float32(r.Max.X)*scale), int(float32(r.Max.Y)*scale))
				st := compareRegion(got, ref, r)
				gi, ri := ink(got, r), ink(ref, r)
				rel := (gi - ri) / ri
				t.Logf("cell %d: max %3d, mean %.2f, >32: %.2f%%, ink %+.1f%%", i, st.max, st.mean, 100*st.far, 100*rel)
				if st.mean > hyPixelsMaxMean[scale] || st.far > hyPixelsMaxFar {
					t.Errorf("cell %d: mean channel difference %.2f (allowed %.2f), %.2f%% of the channels off by more than 32 (allowed %.1f%%)",
						i, st.mean, hyPixelsMaxMean[scale], 100*st.far, 100*hyPixelsMaxFar)
				}
				if rel > hyPixelsMaxInk || rel < -hyPixelsMaxInk {
					t.Errorf("cell %d: ink %.1f, Chromium %.1f (%+.1f%%, allowed ±%.0f%%)", i, gi, ri, 100*rel, 100*hyPixelsMaxInk)
				}
			}
		})
	}
}
