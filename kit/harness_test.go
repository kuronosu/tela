package kit_test

import (
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The test harness every component of the kit uses (ADR-129, K3 and K11).
//
// A sheet is the gallery of one component: a list of cells, each the same
// thing twice. Tela is given a tela.Component (kit.Button{...}); Chromium
// is given the markup shadcn/ui's own component renders, with the classes
// of its registry (the pinned commit, kit.ShadcnCommit). Both are shown in
// a light panel and in a dark one. A sheet is used three ways:
//
//   - sheet.golden: the sheet in light and in dark, at scales 1 and 2, as
//     testdata/<name>_{light,dark}_x{1,2}.png (look at them: the goldens are
//     how the look is reviewed);
//   - sheet.chromium: the sheet against Chromium. With TELA_CHROMIUM=1 it
//     writes the page, compiles its CSS with the Tailwind CLI 4.3.3
//     (TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss) and takes
//     Chromium's geometry and screenshots at scales 1 and 2 into
//     testdata/chromium/<name>/; without it, it compares Tela with what is
//     committed, and fails when the committed page is not the one the sheet
//     makes now. It compares the boxes marked as probes (a cell's own box
//     and the ones its markup marks) and the pixels around each cell;
//   - the tests of the component, which use the same cells.
//
// A cell marks an element as a probe on both sides with the same name:
// Props: h.TestID("name") on Tela's and data-probe="name" in the markup. A
// name is used once per panel, light first, so it appears twice on a page.
//
// Regenerating needs chromium in PATH (or TELA_CHROMIUM_BIN), Node and
// the Tailwind CLI:
//
//	mkdir /tmp/tw && cd /tmp/tw && npm i @tailwindcss/cli@4.3.3 tailwindcss@4.3.3
//	TELA_CHROMIUM=1 TELA_TAILWIND=/tmp/tw/node_modules/.bin/tailwindcss go test -run Chromium ./kit/
//
// TELA_CHROMIUM_DUMP=dir writes Tela's rendering of every sheet there.

// cell is one entry of a sheet.
type cell struct {
	// name identifies the cell; it is the probe of its box.
	name string
	// tela is the component under test.
	tela tela.Component
	// html is shadcn/ui's markup for the same thing.
	html string
	// width is the cell's width in px; 0 shrinks it to its content.
	width int
	// probes names the extra probes the cell's two sides mark.
	probes []string
}

// sheet is the gallery of a component.
type sheet struct {
	// name is the component's file name: "button".
	name  string
	cells []cell
	// width is the page's width in px (default 800: above Tailwind's md
	// breakpoint, where shadcn's text-sm applies).
	width int
	// slot is the width of the fixed slot each cell sits in, in px; 0 lets
	// the cells flow by their own widths. Use it for sheets of small cells.
	slot int
	// css is extra CSS for the Chromium page, for what cannot be the same
	// in a screenshot, such as a running animation.
	css string
	// edge is the largest difference accepted between a probe's edges and
	// Chromium's, in logical px, at scales 1 and 2 (default 0.06, 0.5).
	edge [2]float64
	// mean is the largest mean difference per channel accepted around a
	// cell, in levels, at scales 1 and 2 (default 3.5 at both scales: the text is
	// anti-aliased by Skia in Chromium and by Tela's rasterizer here, which
	// differ by about a level per channel over a cell of text).
	mean [2]float64
	// theme is the tree's theme (nil: kit.Theme()).
	theme *tela.Theme
}

func (s *sheet) pageWidth() int {
	if s.width > 0 {
		return s.width
	}
	return 800
}

func (s *sheet) edgeBound(si int) float64 {
	if s.edge[si] > 0 {
		return s.edge[si]
	}
	return [2]float64{0.06, 0.5}[si]
}

func (s *sheet) meanBound(si int) float64 {
	if s.mean[si] > 0 {
		return s.mean[si]
	}
	return [2]float64{3.5, 3.5}[si]
}

// panelClass is the classes of a panel; the two sides write it the same.
const panelClass = "flex flex-wrap items-start gap-6 bg-background p-6 text-foreground"

func (c cell) wrapperClass() string {
	if c.width > 0 {
		return fmt.Sprintf("flex w-[%dpx]", c.width)
	}
	return "flex"
}

// slotClass is the class of the fixed-width slot a cell sits in when the
// sheet has a slot width ("" without one). Slots keep a cell where it is
// whatever the text widths before it are, which Chromium rounds to 1/64 px
// and Tela does not: without them the positions drift along a row by up to
// 0.1 px, and a box that straddles a device pixel is snapped to another
// one by each side.
func (s *sheet) slotClass() string {
	if s.slot > 0 {
		return fmt.Sprintf("flex-none w-[%dpx]", s.slot)
	}
	return ""
}

// cellNode is the Tela side of a cell: its wrapper, in a slot when the
// sheet has them; probe adds the cell's probe id.
func (s *sheet) cellNode(c cell, probe bool) tela.Node {
	kids := []tela.Child{h.Class(c.wrapperClass())}
	if probe {
		kids = append(kids, h.TestID(c.name))
	}
	kids = append(kids, c.tela)
	w := h.Div(kids...)
	if s.slot > 0 {
		return h.Div(h.Class(s.slotClass()), w)
	}
	return w
}

// cellHTML is the Chromium side of a cell.
func (s *sheet) cellHTML(c cell) string {
	w := fmt.Sprintf("<div data-probe=%q class=%q>\n%s\n</div>\n", c.name, c.wrapperClass(), strings.TrimSpace(c.html))
	if s.slot > 0 {
		return fmt.Sprintf("<div class=%q>\n%s</div>\n", s.slotClass(), w)
	}
	return w
}

// panel is the Tela side of a panel; dark puts it under a dark Scheme.
func (s *sheet) panel(dark bool) tela.Node {
	kids := []tela.Child{h.Class(panelClass), h.TestID("panel")}
	if dark {
		kids = append(kids, h.Scheme(tela.Dark))
	}
	for _, c := range s.cells {
		kids = append(kids, s.cellNode(c, true))
	}
	return h.Div(kids...)
}

// themeOf returns the theme the sheet is mounted with.
func (s *sheet) themeOf() tela.Theme {
	if s.theme != nil {
		return *s.theme
	}
	return kit.Theme()
}

// heights returns the heights of the light and dark panels in logical
// px, rounded up, measured on Tela's layout.
func (s *sheet) heights(t testing.TB) (light, dark int) {
	t.Helper()
	w := s.pageWidth()
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class(fmt.Sprintf("w-[%dpx]", w)), s.panel(false), s.panel(true))
	}, telatest.Size(float32(w), 4000), telatest.Theme(s.themeOf()))
	light = int(math.Ceil(float64(app.Bounds(telatest.ByTestID("panel").Nth(0)).Height)))
	dark = int(math.Ceil(float64(app.Bounds(telatest.ByTestID("panel").Nth(1)).Height)))
	return light, dark
}

// golden checks the sheet's golden images: light and dark, scales 1 and 2.
func (s *sheet) golden(t *testing.T) {
	t.Helper()
	w := s.pageWidth()
	light, _ := s.heights(t)
	for _, scheme := range []string{"light", "dark"} {
		opts := []telatest.Option{telatest.Size(float32(w), float32(light)), telatest.Theme(s.themeOf()),
			telatest.RootClass("bg-background text-foreground")}
		if scheme == "dark" {
			opts = append(opts, telatest.Dark())
		}
		app := telatest.MountFunc(t, func() tela.Node {
			kids := []tela.Child{h.Class(panelClass)}
			for _, c := range s.cells {
				kids = append(kids, s.cellNode(c, false))
			}
			return h.Div(kids...)
		}, opts...)
		app.GoldenScales(s.name+"_"+scheme, 1, 2)
	}
}

// html returns the Chromium page: the sheet's cells in a light panel and a
// dark one, the measuring script, and the sheet's extra CSS.
func (s *sheet) html() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"gallery.css\">\n")
	if s.css != "" {
		sb.WriteString("<style>\n" + s.css + "\n</style>\n")
	}
	sb.WriteString("</head>\n<body class=\"bg-white\">\n")
	fmt.Fprintf(&sb, "<div data-probe=\"page\" class=\"w-[%dpx]\">\n", s.pageWidth())
	for _, dark := range []bool{false, true} {
		cls := panelClass
		if dark {
			cls = "dark " + cls
		}
		fmt.Fprintf(&sb, "<div data-probe=\"panel\" class=%q>\n", cls)
		for _, c := range s.cells {
			sb.WriteString(s.cellHTML(c))
		}
		sb.WriteString("</div>\n")
	}
	sb.WriteString(`</div>
<pre id="out" style="display: none"></pre>
<script>
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = [];
  for (const e of document.querySelectorAll("[data-probe]")) {
    const r = e.getBoundingClientRect();
    out.push([e.dataset.probe, r.left + scrollX, r.top + scrollY, r.width, r.height]);
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script>
</body>
</html>
`)
	return sb.String()
}

// geometry is Chromium's boxes: per scale ("x1", "x2"), per probe name, the
// rectangles [left, top, width, height] in document order.
type geometry map[string]map[string][][4]float64

func (s *sheet) dir() string { return filepath.Join("testdata", "chromium", s.name) }

// shadcnCSS is shadcn/ui's v4 theme for the neutral base color (the
// theming docs, apps/v4) with its base layer: every border takes the
// border color, outlines the ring's.
const shadcnCSS = `@custom-variant dark (&:is(.dark *));
@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-card: var(--card);
  --color-card-foreground: var(--card-foreground);
  --color-popover: var(--popover);
  --color-popover-foreground: var(--popover-foreground);
  --color-primary: var(--primary);
  --color-primary-foreground: var(--primary-foreground);
  --color-secondary: var(--secondary);
  --color-secondary-foreground: var(--secondary-foreground);
  --color-muted: var(--muted);
  --color-muted-foreground: var(--muted-foreground);
  --color-accent: var(--accent);
  --color-accent-foreground: var(--accent-foreground);
  --color-destructive: var(--destructive);
  --color-border: var(--border);
  --color-input: var(--input);
  --color-ring: var(--ring);
  --color-chart-1: var(--chart-1);
  --color-chart-2: var(--chart-2);
  --color-chart-3: var(--chart-3);
  --color-chart-4: var(--chart-4);
  --color-chart-5: var(--chart-5);
  --radius-sm: calc(var(--radius) * 0.6);
  --radius-md: calc(var(--radius) * 0.8);
  --radius-lg: var(--radius);
  --radius-xl: calc(var(--radius) * 1.4);
  --radius-2xl: calc(var(--radius) * 1.8);
  --radius-3xl: calc(var(--radius) * 2.2);
  --radius-4xl: calc(var(--radius) * 2.6);
}
:root {
  --radius: 0.625rem;
  --background: oklch(1 0 0);
  --foreground: oklch(0.145 0 0);
  --card: oklch(1 0 0);
  --card-foreground: oklch(0.145 0 0);
  --popover: oklch(1 0 0);
  --popover-foreground: oklch(0.145 0 0);
  --primary: oklch(0.205 0 0);
  --primary-foreground: oklch(0.985 0 0);
  --secondary: oklch(0.97 0 0);
  --secondary-foreground: oklch(0.205 0 0);
  --muted: oklch(0.97 0 0);
  --muted-foreground: oklch(0.556 0 0);
  --accent: oklch(0.97 0 0);
  --accent-foreground: oklch(0.205 0 0);
  --destructive: oklch(0.577 0.245 27.325);
  --border: oklch(0.922 0 0);
  --input: oklch(0.922 0 0);
  --ring: oklch(0.708 0 0);
  --chart-1: oklch(0.646 0.222 41.116);
  --chart-2: oklch(0.6 0.118 184.704);
  --chart-3: oklch(0.398 0.07 227.392);
  --chart-4: oklch(0.828 0.189 84.429);
  --chart-5: oklch(0.769 0.188 70.08);
}
.dark {
  --background: oklch(0.145 0 0);
  --foreground: oklch(0.985 0 0);
  --card: oklch(0.205 0 0);
  --card-foreground: oklch(0.985 0 0);
  --popover: oklch(0.205 0 0);
  --popover-foreground: oklch(0.985 0 0);
  --primary: oklch(0.922 0 0);
  --primary-foreground: oklch(0.205 0 0);
  --secondary: oklch(0.269 0 0);
  --secondary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.269 0 0);
  --muted-foreground: oklch(0.708 0 0);
  --accent: oklch(0.269 0 0);
  --accent-foreground: oklch(0.985 0 0);
  --destructive: oklch(0.704 0.191 22.216);
  --border: oklch(1 0 0 / 10%);
  --input: oklch(1 0 0 / 15%);
  --ring: oklch(0.556 0 0);
  --chart-1: oklch(0.488 0.243 264.376);
  --chart-2: oklch(0.696 0.17 162.48);
  --chart-3: oklch(0.769 0.188 70.08);
  --chart-4: oklch(0.627 0.265 303.9);
  --chart-5: oklch(0.645 0.246 16.439);
}
@layer base {
  * {
    @apply border-border outline-ring/50;
  }
}
`

// fontCSS loads the embedded Inter, relative to testdata/chromium/<name>.
const fontCSS = `@theme { --font-sans: "Inter"; }
@font-face { font-family: "Inter"; src: url("../../../../internal/assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }
@font-face { font-family: "Inter"; src: url("../../../../internal/assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
`

func chromiumBin() string {
	if b := os.Getenv("TELA_CHROMIUM_BIN"); b != "" {
		return b
	}
	return "chromium"
}

// regenerate writes the page and takes Chromium's geometry and screenshots.
func (s *sheet) regenerate(t *testing.T) {
	t.Helper()
	dir := s.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	page, err := filepath.Abs(filepath.Join(dir, "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte(s.html()), 0o644); err != nil {
		t.Fatal(err)
	}
	tw := os.Getenv("TELA_TAILWIND")
	if tw == "" {
		if tw, err = exec.LookPath("tailwindcss"); err != nil {
			t.Fatal("the Tailwind CLI 4.3.3 is needed: set TELA_TAILWIND to node_modules/.bin/tailwindcss")
		}
	}
	bin, err := filepath.EvalSymlinks(tw)
	if err != nil {
		t.Fatal(err)
	}
	modules := bin
	for filepath.Base(modules) != "node_modules" {
		next := filepath.Dir(modules)
		if next == modules {
			t.Fatalf("%s is not inside a node_modules directory", tw)
		}
		modules = next
	}
	input := filepath.Join(t.TempDir(), "input.css")
	css := fmt.Sprintf("@import %q;\n@source %q;\n%s%s", filepath.Join(modules, "tailwindcss", "index.css"), page, fontCSS, shadcnCSS)
	if err := os.WriteFile(input, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tw, "-i", input, "-o", filepath.Join(dir, "gallery.css")).CombinedOutput(); err != nil {
		t.Fatalf("tailwindcss: %v\n%s", err, out)
	}
	light, dark := s.heights(t)
	w, hgt := s.pageWidth(), light+dark
	geo := geometry{}
	for _, scale := range []int{1, 2} {
		dom, err := exec.Command(chromiumBin(), "--headless", "--disable-gpu", "--allow-file-access-from-files", "--virtual-time-budget=10000",
			fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", w, hgt), "--dump-dom", "file://"+page).Output()
		if err != nil {
			t.Fatalf("%s: %v", chromiumBin(), err)
		}
		str := string(dom)
		i, j := strings.Index(str, "BEGIN"), strings.Index(str, "END</pre>")
		if i < 0 || j < 0 {
			t.Fatalf("chromium output has no result:\n%s", str[:min(len(str), 2000)])
		}
		var rows [][]any
		if err := json.Unmarshal([]byte(html.UnescapeString(str[i+5:j])), &rows); err != nil {
			t.Fatal(err)
		}
		g := map[string][][4]float64{}
		for _, r := range rows {
			g[r[0].(string)] = append(g[r[0].(string)], [4]float64{r[1].(float64), r[2].(float64), r[3].(float64), r[4].(float64)})
		}
		geo[fmt.Sprintf("x%d", scale)] = g
		shot := filepath.Join(dir, fmt.Sprintf("x%d.png", scale))
		abs, err := filepath.Abs(shot)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(chromiumBin(), "--headless", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
			"--virtual-time-budget=10000", "--force-color-profile=srgb", fmt.Sprintf("--force-device-scale-factor=%d", scale),
			fmt.Sprintf("--window-size=%d,%d", w, hgt), "--screenshot="+abs, "file://"+page).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", chromiumBin(), err, out)
		}
		writeBestPNG(t, shot, readPNG(t, shot))
	}
	// One probe per line, so that the diff of a regeneration is readable.
	var sb strings.Builder
	sb.WriteString("{\n")
	for k, key := range []string{"x1", "x2"} {
		fmt.Fprintf(&sb, "%q: {\n", key)
		names := make([]string, 0, len(geo[key]))
		for n := range geo[key] {
			names = append(names, n)
		}
		sort.Strings(names)
		for i, n := range names {
			b, _ := json.Marshal(geo[key][n])
			sep := ","
			if i == len(names)-1 {
				sep = ""
			}
			fmt.Fprintf(&sb, "  %q: %s%s\n", n, b, sep)
		}
		sep := ","
		if k == 1 {
			sep = ""
		}
		fmt.Fprintf(&sb, "}%s\n", sep)
	}
	sb.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "geometry.json"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// chromium compares the sheet with Chromium.
func (s *sheet) chromium(t *testing.T) {
	t.Helper()
	dir := s.dir()
	if os.Getenv("TELA_CHROMIUM") == "1" {
		s.regenerate(t)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "page.html")); err != nil || string(got) != s.html() {
		t.Fatalf("%s/page.html is not the page of this sheet: regenerate it with TELA_CHROMIUM=1 (see harness_test.go)", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "geometry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var geo geometry
	if err := json.Unmarshal(data, &geo); err != nil {
		t.Fatal(err)
	}
	light, dark := s.heights(t)
	w, hgt := s.pageWidth(), light+dark
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	for si, scale := range []int{1, 2} {
		app := telatest.MountFunc(t, func() tela.Node {
			return h.Div(h.Class(fmt.Sprintf("w-[%dpx]", w)), s.panel(false), s.panel(true))
		}, telatest.Size(float32(w), float32(hgt)), telatest.Scale(float64(scale)), telatest.Theme(s.themeOf()))
		img := app.Screenshot()
		if dump != "" {
			writeBestPNG(t, filepath.Join(dump, fmt.Sprintf("kit_%s_x%d_tela.png", s.name, scale)), img)
		}
		want := geo[fmt.Sprintf("x%d", scale)]
		shot := readPNG(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if shot.Rect != img.Rect {
			t.Fatalf("x%d: Tela's render is %v, Chromium's %v", scale, img.Rect, shot.Rect)
		}
		// Probes: every name Chromium measured must be on Tela's page too.
		names := make([]string, 0, len(want))
		for n := range want {
			names = append(names, n)
		}
		sort.Strings(names)
		worst, worstAt := 0.0, ""
		for _, n := range names {
			if n == "page" {
				continue
			}
			for k, r := range want[n] {
				got := app.Bounds(telatest.ByTestID(n).Nth(k))
				d := edgeDiff([4]float64{float64(got.X), float64(got.Y), float64(got.Width), float64(got.Height)}, r)
				scheme := [2]string{"light", "dark"}[min(k, 1)]
				if n == "panel" {
					scheme = [2]string{"light", "dark"}[k]
				}
				if d > worst {
					worst, worstAt = d, fmt.Sprintf("%s (%s): Tela %.3f %.3f %.3f %.3f, Chromium %.3f %.3f %.3f %.3f", n, scheme, got.X, got.Y, got.Width, got.Height, r[0], r[1], r[2], r[3])
				}
				if d > s.edgeBound(si) {
					t.Errorf("x%d %s (%s): an edge is %.3f px from Chromium's (bound %.2f): Tela %.3f %.3f %.3f %.3f, Chromium %.3f %.3f %.3f %.3f",
						scale, n, scheme, d, s.edgeBound(si), got.X, got.Y, got.Width, got.Height, r[0], r[1], r[2], r[3])
				}
			}
		}
		fmt.Fprintf(&report, "x%d: largest edge difference %.3f px (%s)\n", scale, worst, worstAt)
		// Pixels: the area of each cell, grown by 8 px for its shadow and
		// ring.
		for k, scheme := range []string{"light", "dark"} {
			for _, c := range s.cells {
				b := app.Bounds(telatest.ByTestID(c.name).Nth(k))
				r := image.Rect(int(math.Floor(float64(b.X)-8))*scale, int(math.Floor(float64(b.Y)-8))*scale,
					int(math.Ceil(float64(b.X+b.Width)+8))*scale, int(math.Ceil(float64(b.Y+b.Height)+8))*scale).Intersect(img.Rect)
				d := diffRegion(img, shot, r)
				fmt.Fprintf(&report, "  %-24s %-5s max %3d mean %.3f\n", c.name, scheme, d.max, d.mean)
				if d.mean > s.meanBound(si) {
					t.Errorf("x%d %s (%s): the pixels differ from Chromium's by %.3f levels per channel on average (bound %.2f), at most %d", scale, c.name, scheme, d.mean, s.meanBound(si), d.max)
				}
			}
		}
	}
	t.Log("difference from Chromium:\n" + report.String())
}

// edgeDiff is the largest difference between the edges of two rectangles
// given as left, top, width, height.
func edgeDiff(a, b [4]float64) float64 {
	d := 0.0
	for c := range 2 {
		d = math.Max(d, math.Max(math.Abs(a[c]-b[c]), math.Abs(a[c]+a[c+2]-b[c]-b[c+2])))
	}
	return d
}

type regionDiff struct {
	max  int
	mean float64
}

// diffRegion compares the pixels of two images inside r.
func diffRegion(a, b *image.RGBA, r image.Rectangle) regionDiff {
	var d regionDiff
	sum, n := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pa, pb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			for _, v := range [3]int{int(pa.R) - int(pb.R), int(pa.G) - int(pb.G), int(pa.B) - int(pb.B)} {
				v = max(v, -v)
				sum += v
				d.max = max(d.max, v)
			}
			n += 3
		}
	}
	if n > 0 {
		d.mean = float64(sum) / float64(n)
	}
	return d
}

func readPNG(t testing.TB, path string) *image.RGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// writeBestPNG writes img with the best compression: the screenshots are
// committed.
func writeBestPNG(t testing.TB, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// cn is shadcn's cn(): the classes joined and merged by tailwind-merge, so
// that a later class replaces an earlier one that sets the same property
// (a size's gap-1 drops the button's gap-2). It is what makes the markup of
// a cell the one a browser gets from shadcn's component. This is a small
// version of tailwind-merge that knows the groups shadcn's components
// conflict on; add a group when a component needs one.
func cn(classes ...string) string {
	var toks []string
	for _, c := range classes {
		toks = append(toks, splitClasses(c)...)
	}
	keep := make([]bool, len(toks))
	seen := map[string]bool{}
	for i := len(toks) - 1; i >= 0; i-- {
		mods, util := splitModifiers(toks[i])
		keep[i] = true
		groups := mergeGroups(util)
		if len(groups) == 0 {
			continue
		}
		conflict := false
		for _, g := range groups {
			if seen[mods+"|"+g] {
				conflict = true
			}
		}
		if conflict {
			keep[i] = false
			continue
		}
		seen[mods+"|"+groups[0]] = true
		// size-* also replaces an earlier w-* and h-*.
		if groups[0] == "size" {
			seen[mods+"|w"] = true
			seen[mods+"|h"] = true
		}
	}
	var out []string
	for i, t := range toks {
		if keep[i] {
			out = append(out, t)
		}
	}
	return strings.Join(out, " ")
}

// splitClasses splits a class string at spaces that are outside brackets.
func splitClasses(s string) []string {
	var out []string
	depth, start := 0, -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || (s[i] == ' ' && depth == 0) {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		}
	}
	return out
}

// splitModifiers splits "dark:hover:bg-x" into "dark:hover:" and "bg-x",
// ignoring the colons inside brackets.
func splitModifiers(tok string) (mods, util string) {
	depth, last := 0, -1
	for i := 0; i < len(tok); i++ {
		switch tok[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ':':
			if depth == 0 {
				last = i
			}
		}
	}
	return tok[:last+1], tok[last+1:]
}

var mergeGroupPatterns = []struct {
	group string
	re    *regexp.Regexp
}{
	{"gap", regexp.MustCompile(`^gap-[\d.\[]`)},
	{"px", regexp.MustCompile(`^px-`)},
	{"py", regexp.MustCompile(`^py-`)},
	{"pt", regexp.MustCompile(`^pt-`)},
	{"pb", regexp.MustCompile(`^pb-`)},
	{"h", regexp.MustCompile(`^h-`)},
	{"w", regexp.MustCompile(`^w-`)},
	{"size", regexp.MustCompile(`^size-`)},
	{"rounded", regexp.MustCompile(`^rounded(-|$)`)},
	{"font-size", regexp.MustCompile(`^text-(xs|sm|base|lg|xl|[2-9]xl)$`)},
	{"font-weight", regexp.MustCompile(`^font-(thin|light|normal|medium|semibold|bold)$`)},
	{"leading", regexp.MustCompile(`^leading-`)},
	{"min-h", regexp.MustCompile(`^min-h-`)},
	{"min-w", regexp.MustCompile(`^min-w-`)},
	{"bg", regexp.MustCompile(`^bg-`)},
	{"border-color", regexp.MustCompile(`^border-(transparent|current|white|black|border|input|ring|background|foreground|primary|secondary|destructive|muted|accent|card|popover|[a-z]+-\d{2,3})(/\d+)?$`)},
	{"text-color", regexp.MustCompile(`^text-(white|black|current|foreground|background|primary|secondary|destructive|muted|accent|card|popover|[a-z]+-\d{2,3})(-foreground)?(/\d+)?$`)},
}

// mergeGroups returns the property groups a utility sets, or none.
func mergeGroups(util string) []string {
	// A group is written on the utility without a leading "-" (negative
	// values) or "!" (important).
	util = strings.TrimLeft(util, "-!")
	for _, p := range mergeGroupPatterns {
		if p.re.MatchString(util) {
			return []string{p.group}
		}
	}
	return nil
}
