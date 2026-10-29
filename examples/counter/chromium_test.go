package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/chromium"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The Counter is compared with Chromium's rendering of the same markup in
// HTML, styled by Tailwind's own compiled CSS (compare_test.go). The files
// in testdata/chromium are:
//
//	counter.html            the markup (counterMarkup), directly in <body>
//	counter.css             compiled from it by the Tailwind CLI v4.3.3,
//	                        with Inter as the sans font
//	counter_x1.png,
//	counter_x2.png          Chromium's screenshots at device scale 1 and 2
//	layout.html             the same markup plus a script that measures it
//	layout.json             what the script measured (getBoundingClientRect)
//
// Regenerate them with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestRegenerateChromium ./examples/counter/
//
// which needs chromium in PATH and the Tailwind CLI (TELA_TAILWIND, or
// tailwindcss in PATH, installed in a node_modules directory). TestCounterHTML
// checks, without Chromium, that counter.html still has the classes of
// Counter.

const (
	// counterW and counterH are the viewport, as in the App of the example
	// (800x600).
	counterW, counterH = 800, 600

	// counterMarkup is Counter's tree as HTML, with the same classes and
	// text (count 0).
	counterMarkup = `<div class="flex flex-col items-center gap-6 p-10 rounded-3xl bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"><h1 class="text-5xl font-bold text-white tracking-tight">Count: 0</h1><button class="px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur hover:bg-white/25 active:scale-95 transition">Increment</button></div>`

	// chromiumFonts loads the embedded Inter in the page, relative to
	// testdata/chromium.
	chromiumFonts = `@font-face { font-family: "Inter"; src: url("../../../../internal/assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }
@font-face { font-family: "Inter"; src: url("../../../../internal/assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
`
)

const chromiumDir = "testdata/chromium"

// counterHTML is the page screenshotted by Chromium.
func counterHTML() string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"counter.css\">\n</head>\n<body>\n" +
		counterMarkup + "\n</body>\n</html>\n"
}

// layoutHTML is counter.html plus a script that writes the boxes into a
// hidden element (after the card, so it does not move it): the card, the
// heading, the button, the rectangles of the text of both, and the ascent
// of their fonts as Blink quantizes it, to place the baselines.
func layoutHTML() string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"counter.css\">\n</head>\n<body>\n" +
		counterMarkup + `
<pre id="out" hidden></pre>
<script>
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const rect = r => [r.left, r.top, r.width, r.height];
  const card = document.querySelector("div");
  const h1 = document.querySelector("h1");
  const button = document.querySelector("button");
  // Blink quantizes the font metrics in device pixels: measure at the
  // device size.
  const dpr = window.devicePixelRatio;
  const ctx = document.createElement("canvas").getContext("2d");
  const textOf = el => {
    const range = document.createRange();
    range.selectNodeContents(el);
    const cs = getComputedStyle(el);
    ctx.font = cs.fontWeight + " " + (parseFloat(cs.fontSize) * dpr) + "px " + cs.fontFamily;
    const m = ctx.measureText(el.textContent);
    return {rect: rect(range.getBoundingClientRect()), ascent: m.fontBoundingBoxAscent / dpr};
  };
  const text = textOf(h1), buttonText = textOf(button);
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify({
    dpr: dpr,
    card: rect(card.getBoundingClientRect()),
    h1: rect(h1.getBoundingClientRect()),
    text: text.rect,
    ascent: text.ascent,
    button: rect(button.getBoundingClientRect()),
    buttonText: buttonText.rect,
    buttonAscent: buttonText.ascent,
  }) + "END";
});
</script>
</body>
</html>
`
}

// chromiumLayout is what layoutHTML measures, in CSS pixels. Rectangles are
// [left, top, width, height]. Text is the range of the heading's text and
// ButtonText the button's; Ascent and ButtonAscent are the quantized font
// ascents (canvas fontBoundingBoxAscent), so that the baseline of a text is
// the top of its rectangle plus its ascent.
type chromiumLayout struct {
	DPR          float64    `json:"dpr"`
	Card         [4]float64 `json:"card"`
	H1           [4]float64 `json:"h1"`
	Text         [4]float64 `json:"text"`
	Ascent       float64    `json:"ascent"`
	Button       [4]float64 `json:"button"`
	ButtonText   [4]float64 `json:"buttonText"`
	ButtonAscent float64    `json:"buttonAscent"`
}

// chromiumFile is layout.json: the measures at device scale 1 and 2.
type chromiumFile struct {
	X1 chromiumLayout `json:"x1"`
	X2 chromiumLayout `json:"x2"`
}

// TestRegenerateChromium rewrites testdata/chromium with Chromium (only with
// TELA_CHROMIUM=1).
func TestRegenerateChromium(t *testing.T) {
	if os.Getenv("TELA_CHROMIUM") != "1" {
		t.Skip("set TELA_CHROMIUM=1 (and TELA_TAILWIND) to regenerate testdata/chromium with Chromium")
	}
	dir, err := filepath.Abs(chromiumDir)
	if err != nil {
		t.Fatal(err)
	}
	htmlPath, layoutPath := filepath.Join(dir, "counter.html"), filepath.Join(dir, "layout.html")
	for path, src := range map[string]string{htmlPath: counterHTML(), layoutPath: layoutHTML()} {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	chromium.Tailwind(t, htmlPath, filepath.Join(dir, "counter.css"),
		"@theme { --font-sans: \"Inter\"; }\n"+chromiumFonts)

	var layouts [2]chromiumLayout
	for i, scale := range []int{1, 2} {
		shot := filepath.Join(dir, fmt.Sprintf("counter_x%d.png", scale))
		chromium.Screenshot(t, "file://"+htmlPath, shot, counterW, counterH, scale)
		// Chromium compresses fast; keep the committed file smaller.
		golden.WriteBestPNG(t, shot, golden.ReadRGBA(t, shot))
		chromium.DumpJSON(t, "file://"+layoutPath, counterW, counterH, scale, &layouts[i])
	}
	// One scale per line: compact, but diffs show which one changed.
	var out strings.Builder
	out.WriteString("{\n")
	for i, l := range layouts {
		js, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		sep := ","
		if i == len(layouts)-1 {
			sep = ""
		}
		fmt.Fprintf(&out, "%q: %s%s\n", fmt.Sprintf("x%d", i+1), js, sep)
	}
	out.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
