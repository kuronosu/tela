package text

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The baseline comparison lays out one line of Inter in every font size
// and line height of baselineSizes and baselineHeights at the display
// scales of baselineScales (Paragraph.Scale), here and in Chromium (the
// same font file through @font-face, --force-device-scale-factor), and
// compares in device pixels the line box's height, its baseline and the
// content area of the text (the quantized ascent and descent of its font):
// ADR-159. Chromium's results (getBoundingClientRect of a div, of the
// text's span and of an empty inline-block on the baseline) are committed
// in testdata/chromium/baselines.json, so the test runs without Chromium.
// To regenerate them:
//
//	TELA_CHROMIUM=1 go test -run TestChromiumBaselines ./internal/text/
//
// which writes testdata/chromium/baselines.html and runs chromium
// --headless --dump-dom on it once per scale.

var baselineSizes = []float32{11, 12, 13, 14, 15, 16, 17, 18, 20, 24, 30, 36, 48}

// baselineHeights are line heights as CSS writes them and as Tela gets
// them: Tailwind's leading-* multiples, normal, pixel values and the
// default line heights of Tailwind's text-* sizes (calc() in its CSS,
// float32 multiples in Tela's theme, internal/style/theme_gen.go).
var baselineHeights = []struct {
	css string
	lh  LineHeight
}{
	{"1", LineHeightMultiple(1)},         // leading-none
	{"1.25", LineHeightMultiple(1.25)},   // leading-tight
	{"1.375", LineHeightMultiple(1.375)}, // leading-snug
	{"1.5", LineHeightMultiple(1.5)},     // leading-normal
	{"1.625", LineHeightMultiple(1.625)}, // leading-relaxed
	{"2", LineHeightMultiple(2)},         // leading-loose
	{"0.8", LineHeightMultiple(0.8)},
	{"normal", LineHeight{}},
	{"16px", LineHeightPixels(16)},
	{"20px", LineHeightPixels(20)},
	{"24px", LineHeightPixels(24)},
	{"28px", LineHeightPixels(28)},
	{"calc(1 / 0.75)", LineHeightMultiple(1.3333334)},     // text-xs, text-2xl
	{"calc(1.25 / 0.875)", LineHeightMultiple(1.4285715)}, // text-sm
	{"calc(1.75 / 1.125)", LineHeightMultiple(1.5555556)}, // text-lg
	{"calc(1.75 / 1.25)", LineHeightMultiple(1.4)},        // text-xl
	{"calc(2.25 / 1.875)", LineHeightMultiple(1.2)},       // text-3xl
	{"calc(2.5 / 2.25)", LineHeightMultiple(1.1111112)},   // text-4xl
}

var baselineScales = []float32{1, 1.25, 1.5, 2, 3}

// baselineKnown are the cases where Chromium's baseline is one device
// pixel above Tela's: Blink computes a unitless line height in float32
// (the number as a percentage, times the font size, over 100) and
// truncates it to LayoutUnits, so 4/3 of a 30 or 60 px device font is
// 39.984 or 79.984 px there, and the leading, an even number of device
// pixels in exact arithmetic, loses one when halved and rounded down.
// Tela takes the nearest LayoutUnit (40, 80), which agrees with Blink in
// more cases than imitating its float32 steps with Tela's float32
// multiples would (ADR-159). The content area moves with the baseline.
var baselineKnown = map[string]bool{
	"x1 30/calc(1 / 0.75)":    true,
	"x1.25 24/calc(1 / 0.75)": true,
	"x1.25 48/calc(1 / 0.75)": true,
	"x1.5 20/calc(1 / 0.75)":  true,
	"x2 15/calc(1 / 0.75)":    true,
	"x2 30/calc(1 / 0.75)":    true,
	"x3 20/calc(1 / 0.75)":    true,
}

func baselineKey(size float32, css string) string { return fmt.Sprintf("%g/%s", size, css) }

func baselineScaleKey(s float32) string { return fmt.Sprintf("x%g", s) }

func chromiumBaselinesHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(`@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }`)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; background: #fff; }\n")
	b.WriteString(".c { font-family: \"Inter\"; width: 300px; margin: 0 0 10px 0; }\n")
	b.WriteString(".m { display: inline-block; width: 0; height: 0; }\n")
	b.WriteString("</style></head><body>\n")
	for _, s := range baselineSizes {
		for _, h := range baselineHeights {
			st := html.EscapeString(fmt.Sprintf("font-size: %gpx; line-height: %s;", s, h.css))
			k := html.EscapeString(baselineKey(s, h.css))
			// The first div has the marker on the baseline (which can
			// grow a line box shorter than the baseline), the second
			// gives the line box's height.
			fmt.Fprintf(&b, "<div class=\"c\" data-k=\"%s\" style=\"%s\"><span class=\"t\">Hxgy</span><span class=\"m\"></span></div>", k, st)
			fmt.Fprintf(&b, "<div class=\"c h\" style=\"%s\">Hxgy</div>\n", st)
		}
	}
	b.WriteString(`<pre id="out"></pre>
<script>
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = {};
  for (const d of document.querySelectorAll("[data-k]")) {
    const r = d.getBoundingClientRect();
    const t = d.querySelector(".t").getBoundingClientRect();
    const m = d.querySelector(".m").getBoundingClientRect();
    const h = d.nextElementSibling.getBoundingClientRect();
    out[d.dataset.k] = [h.height, t.top - r.top, t.height, m.top - r.top];
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`)
	return b.String()
}

// chromiumBaselines is baselines.json: per scale ("x1.5"), per case
// ("14/1.375"), Chromium's [line box height, text top, text height,
// baseline] in CSS px, relative to the line box's top.
type chromiumBaselines map[string]map[string][4]float64

func regenerateChromiumBaselines(t *testing.T, dir string) {
	page := filepath.Join(dir, "baselines.html")
	if err := os.WriteFile(page, []byte(chromiumBaselinesHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(page)
	all := chromiumBaselines{}
	for _, s := range baselineScales {
		out, err := exec.Command("chromium", "--headless", "--disable-gpu", fmt.Sprintf("--force-device-scale-factor=%g", s),
			"--allow-file-access-from-files", "--virtual-time-budget=10000", "--window-size=400,800", "--dump-dom", "file://"+abs).Output()
		if err != nil {
			t.Fatalf("chromium: %v", err)
		}
		str := string(out)
		i, j := strings.Index(str, "BEGIN"), strings.Index(str, "END</pre>")
		if i < 0 || j < 0 {
			t.Fatalf("chromium output has no result:\n%s", str[:min(len(str), 2000)])
		}
		var res map[string][4]float64
		if err := json.Unmarshal([]byte(html.UnescapeString(str[i+5:j])), &res); err != nil {
			t.Fatal(err)
		}
		all[baselineScaleKey(s)] = res
	}
	// One case per line, so that a regeneration's diff is readable.
	var b strings.Builder
	b.WriteString("{\n")
	for si, s := range baselineScales {
		fmt.Fprintf(&b, "%q: {\n", baselineScaleKey(s))
		n := 0
		for _, size := range baselineSizes {
			for _, h := range baselineHeights {
				k := baselineKey(size, h.css)
				v, _ := json.Marshal(all[baselineScaleKey(s)][k])
				n++
				sep := ","
				if n == len(baselineSizes)*len(baselineHeights) {
					sep = ""
				}
				fmt.Fprintf(&b, "  %q: %s%s\n", k, v, sep)
			}
		}
		sep := ","
		if si == len(baselineScales)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "}%s\n", sep)
	}
	b.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "baselines.json"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChromiumBaselines(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateChromiumBaselines(t, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "baselines.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	var ref chromiumBaselines
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	coll := layoutCollection(t)
	known := 0
	for _, s := range baselineScales {
		k := float64(s)
		sk := baselineScaleKey(s)
		for _, size := range baselineSizes {
			for _, h := range baselineHeights {
				key := baselineKey(size, h.css)
				r, ok := ref[sk][key]
				if !ok {
					t.Fatalf("%s %s: no Chromium result; regenerate with TELA_CHROMIUM=1", sk, key)
				}
				st := TextStyle{Families: []string{"Inter"}, Size: size, LineHeight: h.lh}
				l := NewLayout(coll, &Paragraph{Text: "Hxgy", Default: st, Scale: s})
				ln := l.Line(0)
				c := l.Caret(Cursor{})
				// Device pixels: Tela's, then Chromium's.
				got := [4]float64{float64(l.Height()), float64(ln.Baseline - c.Ascent), float64(c.Ascent + c.Descent), float64(ln.Baseline)}
				var d [4]float64
				for i := range got {
					d[i] = got[i]*k - r[i]*k
				}
				name := sk + " " + key
				if baselineKnown[name] {
					known++
					if math.Abs(d[3]-1) > 0.01 || math.Abs(d[1]-1) > 0.01 {
						t.Errorf("%s: baseline %+.3f, text top %+.3f device px from Chromium's, want +1 (a known difference; remove it from baselineKnown if it is gone)", name, d[3], d[1])
					}
					continue
				}
				// Chromium truncates line heights to LayoutUnits (1/64
				// device px); the rest is whole device pixels.
				if math.Abs(d[0]) > 1.0/64+1e-3 || math.Abs(d[1]) > 0.01 || math.Abs(d[2]) > 0.01 || math.Abs(d[3]) > 0.01 {
					t.Errorf("%s: device px: line box height %.3f (Chromium %.3f), text top %.3f (%.3f), text height %.3f (%.3f), baseline %.3f (%.3f)",
						name, got[0]*k, r[0]*k, got[1]*k, r[1]*k, got[2]*k, r[2]*k, got[3]*k, r[3]*k)
				}
			}
		}
	}
	if known != len(baselineKnown) {
		t.Errorf("%d of the %d known differences were checked: baselineKnown names a case that is not in the table", known, len(baselineKnown))
	}
}

// TestSetScaleMatchesNewLayout checks that Layout.SetScale gives the line
// boxes of a layout made at that scale, back and forth, with spans in
// several sizes, a fallback font with line-height normal and inline
// boxes.
func TestSetScaleMatchesNewLayout(t *testing.T) {
	coll := layoutCollection(t)
	para := func(scale float32) *Paragraph {
		p := &Paragraph{Default: TextStyle{Families: allFamilies, Size: 14, LineHeight: LineHeightMultiple(1.375)}, Scale: scale}
		p.AddText("Small ", TextStyle{Families: allFamilies, Size: 13})
		p.AddText("normal مرحبا ", TextStyle{Families: []string{"Inter", "Noto Sans Arabic"}, Size: 17})
		p.AddBox(InlineBox{Width: 12, Height: 12, Align: BoxTextTop})
		p.AddText(" tight text that wraps onto a second line", TextStyle{Families: allFamilies, Size: 15, LineHeight: LineHeightMultiple(1.25)})
		return p
	}
	type lineBox struct{ top, height, baseline, ascent, descent float32 }
	boxes := func(l *Layout) []lineBox {
		var out []lineBox
		for i := range l.NumLines() {
			ln := l.Line(i)
			out = append(out, lineBox{ln.Top, ln.Height, ln.Baseline, ln.Ascent, ln.Descent})
		}
		return out
	}
	l := NewLayout(coll, para(1))
	l.BreakLines(180)
	for _, s := range []float32{2, 1.5, 1.25, 3, 1} {
		l.SetScale(s)
		if l.Scale() != s {
			t.Fatalf("Scale() = %g after SetScale(%g)", l.Scale(), s)
		}
		want := NewLayout(coll, para(s))
		want.BreakLines(180)
		if got, w := boxes(l), boxes(want); fmt.Sprint(got) != fmt.Sprint(w) {
			t.Errorf("scale %g: SetScale gives line boxes %v, NewLayout %v", s, got, w)
		}
		if l.Height() != want.Height() {
			t.Errorf("scale %g: height %v after SetScale, %v from NewLayout", s, l.Height(), want.Height())
		}
	}
	for _, s := range []float32{0, -1, float32(math.Inf(1)), float32(math.NaN())} {
		if l := NewLayout(coll, para(s)); l.Scale() != 1 {
			t.Errorf("Paragraph.Scale %g: Scale() = %g, want 1", s, l.Scale())
		}
	}
}
