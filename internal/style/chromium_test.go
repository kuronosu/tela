package style

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestChromiumPalette compares the generated palette, painted as solid
// colors (extended sRGB clipped per channel), with the pixels that Chromium
// draws for the same oklch() source, and with an independent float64
// per-channel clip of the OKLCH conversion. A difference must be one level
// at most, on a channel whose exact value is within 0.05 of a rounding
// boundary, where Chromium's own conversion rounding decides. It needs a
// Chromium binary and runs only with TELA_CHROMIUM=1 (TELA_CHROMIUM_BIN
// overrides the binary, default "chromium").
func TestChromiumPalette(t *testing.T) {
	if os.Getenv("TELA_CHROMIUM") != "1" {
		t.Skip("set TELA_CHROMIUM=1 to compare the palette with Chromium")
	}
	bin := os.Getenv("TELA_CHROMIUM_BIN")
	if bin == "" {
		bin = "chromium"
	}

	css, err := os.ReadFile("gen/tailwind/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`--color-([a-z]+-\d+): (oklch\(([\d.]+)% ([\d.]+) ([\d.]+|none)\));`)
	type swatch struct {
		name, css string
		l, c, h   float64
	}
	var swatches []swatch
	for _, m := range re.FindAllStringSubmatch(string(css), -1) {
		l, _ := strconv.ParseFloat(m[3], 64)
		c, _ := strconv.ParseFloat(m[4], 64)
		h := 0.0
		if m[5] != "none" {
			h, _ = strconv.ParseFloat(m[5], 64)
		}
		swatches = append(swatches, swatch{m[1], m[2], l / 100, c, h})
	}
	if len(swatches) != 26*11 {
		t.Fatalf("found %d oklch colors in theme.css, want %d", len(swatches), 26*11)
	}

	const size, cols = 20, 11
	rows := (len(swatches) + cols - 1) / cols
	var html strings.Builder
	html.WriteString(`<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;background:#fff}
div{position:absolute;width:20px;height:20px}
</style>`)
	for i, s := range swatches {
		fmt.Fprintf(&html, `<div style="left:%dpx;top:%dpx;background:%s"></div>`,
			i%cols*size, i/cols*size, s.css)
	}
	dir := t.TempDir()
	page := filepath.Join(dir, "palette.html")
	shot := filepath.Join(dir, "palette.png")
	if err := os.WriteFile(page, []byte(html.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "--headless", "--disable-gpu", "--no-sandbox",
		"--hide-scrollbars", "--force-color-profile=srgb", "--force-device-scale-factor=1",
		fmt.Sprintf("--window-size=%d,%d", cols*size, rows*size),
		"--screenshot="+shot, "file://"+page).CombinedOutput()
	if err != nil {
		t.Fatalf("running %s: %v\n%s", bin, err, out)
	}
	f, err := os.Open(shot)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	th := DefaultTheme()
	type stat struct {
		n, maxDiff int
		worst      string
	}
	var generated, clipped stat
	modelsDiffer, outOfGamut := 0, 0
	for i, s := range swatches {
		px := img.At(i%cols*size+size/2, i/cols*size+size/2)
		r16, g16, b16, _ := px.RGBA()
		chrome := [3]int{int(r16 >> 8), int(g16 >> 8), int(b16 >> 8)}

		gen := th.Colors[s.name].NRGBA8()
		clip := clipOKLCH(s.l, s.c, s.h)
		genRGB := [3]int{int(gen.R), int(gen.G), int(gen.B)}
		if genRGB != chrome {
			g := th.Colors[s.name].Clamp()
			t.Logf("%-13s differs from chromium: chromium %v  generated %v  (unrounded %.3f %.3f %.3f)",
				s.name, chrome, genRGB, g.R*255, g.G*255, g.B*255)
			for k, v := range [3]float32{g.R, g.G, g.B} {
				if genRGB[k] == chrome[k] {
					continue
				}
				x := float64(v) * 255
				if abs(genRGB[k]-chrome[k]) > 1 || math.Abs(x-math.Floor(x)-0.5) > 0.05 {
					t.Errorf("%s: channel %d is %.3f (%d), Chromium paints %d: not a rounding tie", s.name, k, x, genRGB[k], chrome[k])
				}
			}
		}
		sin, cos := math.Sincos(s.h * math.Pi / 180)
		r, g, b := gfx.OklabToLinearSRGB(float32(s.l), float32(s.c*cos), float32(s.c*sin))
		for _, v := range []float32{r, g, b} {
			if v < -0.0005 || v > 1.0005 {
				outOfGamut++
				break
			}
		}
		if s.name == "fuchsia-600" {
			t.Logf("%s: chromium %v generated %v clipped %v linear before clipping %.3f %.3f %.3f",
				s.name, chrome, genRGB, clip, r, g, b)
		}
		if genRGB != clip {
			modelsDiffer++
			t.Logf("%-13s chromium %v  generated %v  float64 clip %v", s.name, chrome, genRGB, clip)
		}
		for _, m := range []struct {
			st  *stat
			got [3]int
		}{
			{&generated, [3]int{int(gen.R), int(gen.G), int(gen.B)}},
			{&clipped, clip},
		} {
			d := 0
			for k := range 3 {
				d = max(d, abs(m.got[k]-chrome[k]))
			}
			if d > 0 {
				m.st.n++
			}
			if d > m.st.maxDiff {
				m.st.maxDiff = d
				m.st.worst = fmt.Sprintf("%s: chromium %v, model %v", s.name, chrome, m.got)
			}
		}
	}
	t.Logf("generated (extended sRGB, clipped) vs Chromium: %d of %d colors differ, max channel difference %d (%s)",
		generated.n, len(swatches), generated.maxDiff, generated.worst)
	t.Logf("%d of %d colors are outside the sRGB gamut", outOfGamut, len(swatches))
	t.Logf("generated and float64 per-channel clip differ on %d of %d colors", modelsDiffer, len(swatches))
	t.Logf("float64 per-channel clip vs Chromium: %d of %d colors differ, max channel difference %d (%s)",
		clipped.n, len(swatches), clipped.maxDiff, clipped.worst)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// clipOKLCH converts OKLCH to 8-bit sRGB by clamping each channel, with no
// gamut mapping.
func clipOKLCH(l, c, h float64) [3]int {
	sin, cos := math.Sincos(h * math.Pi / 180)
	r, g, b := gfx.OklabToLinearSRGB(float32(l), float32(c*cos), float32(c*sin))
	var out [3]int
	for i, v := range []float32{r, g, b} {
		v = min(max(v, 0), 1)
		out[i] = int(math.Round(float64(gfx.LinearToSRGB(v)) * 255))
	}
	return out
}
