package core

import (
	"encoding/json"
	"fmt"
	"html"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Fixed and sticky positioning compared with Chromium (ADR-077). Each
// scene is a page (a tree of Tailwind elements, the body's content) shown
// at several scroll states: the window's and elements' offsets set with
// scrollTo by the page's script before Chromium measures every element
// (getBoundingClientRect) and takes the screenshot, and with ScrollTo
// here. testdata/chromium/positioned/<scene> holds the page, its CSS,
// the geometry (geometry.json: per scale and step) and the screenshots
// (x<scale>_s<step>.png). Chromium's --screenshot ignores the window's
// scroll offset (it draws the document from its top, with the fixed
// boxes moved), so the page's body is the scroller instead: html is
// overflow-hidden and body h-screen overflow-auto, which looks and
// measures like a scrolled window. The test compares the boxes of every element
// and the pixels of every state, so Chromium is not needed to run it.
// Regenerate with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumPositioned ./internal/core/
//
// (one Chromium instance at a time). TELA_CHROMIUM_DUMP=dir writes
// Tela's renders there.

// chromiumDecorations is the height that Chromium 152's headless window
// takes from --window-size in --dump-dom mode (its innerHeight is 87 px
// smaller; --screenshot is not affected).
const chromiumDecorations = 87

// posScroll scrolls the window (id -1) or element e<id> to (x, y).
type posScroll struct {
	id   int
	x, y float32
}

type posScene struct {
	name string
	w, h int
	root inode
	// steps are the scroll states, each a list of scrolls applied in
	// order.
	steps [][]posScroll
	// geom is the largest edge difference accepted (logical px); mean
	// and maxd bound the pixel differences of a state per scale.
	geom float64
	mean [2]float64
	maxd [2]int
	// geomY2 is a larger bound for vertical edges at scale 2, where
	// Blink rounds line boxes in device pixels (ADR-048); 0 is geom.
	geomY2 float64
	// arabic adds the embedded Noto Sans Arabic to the page's fonts (Tela
	// falls back to it for Arabic script).
	arabic bool
}

func (s *posScene) dir() string { return filepath.Join("testdata", "chromium", "positioned", s.name) }

func (s *posScene) html() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html class=\"overflow-hidden\">\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"gallery.css\">\n</head>\n<body class=\"h-screen overflow-auto bg-white\">\n")
	id := 0
	s.root.html(&sb, &id)
	steps, _ := json.Marshal(s.scrollList())
	fmt.Fprintf(&sb, `
<pre id="out" style="display: none"></pre>
<script>
const steps = %s;
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const k = +(location.hash.slice(1) || 0);
  for (const [id, x, y] of steps[k]) {
    if (id < 0) document.body.scrollTo(x, y); else document.getElementById("e" + id).scrollTo(x, y);
  }
  const out = [];
  for (let i = 0; i < %d; i++) {
    const q = document.getElementById("e" + i).getBoundingClientRect();
    out.push([q.left, q.top, q.width, q.height]);
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script>
</body>
</html>
`, steps, id)
	return sb.String()
}

func (s *posScene) scrollList() [][][3]float32 {
	out := make([][][3]float32, len(s.steps))
	for i, st := range s.steps {
		out[i] = [][3]float32{}
		for _, sc := range st {
			out[i] = append(out[i], [3]float32{float32(sc.id), sc.x, sc.y})
		}
	}
	return out
}

// posGeometry is the geometry of a scene: "x<scale>" to one list of
// element rectangles (x, y, width, height) per step.
type posGeometry map[string][][][4]float64

func (s *posScene) regenerate(t *testing.T) {
	dir := s.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath, err := filepath.Abs(filepath.Join(dir, "gallery.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(s.html()), 0o644); err != nil {
		t.Fatal(err)
	}
	tw := os.Getenv("TELA_TAILWIND")
	if tw == "" {
		t.Fatal("the Tailwind CLI is needed: set TELA_TAILWIND to node_modules/.bin/tailwindcss")
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
	fonts := strings.ReplaceAll(cssGalleryFonts, "../../../../", "../../../../../")
	sans := `"Inter"`
	if s.arabic {
		fonts += `@font-face { font-family: "Noto Sans Arabic"; src: url("../../../../../assets/fonts/notosansarabic/NotoSansArabic-wdth-wght.ttf"); font-weight: 100 900; font-stretch: 62.5% 100%; }` + "\n"
		sans = `"Inter", "Noto Sans Arabic"`
	}
	css := fmt.Sprintf("@import %q;\n@source %q;\n@theme { --font-sans: %s; }\n%s",
		filepath.Join(modules, "tailwindcss", "index.css"), htmlPath, sans, fonts)
	if err := os.WriteFile(input, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tw, "-i", input, "-o", filepath.Join(dir, "gallery.css")).CombinedOutput(); err != nil {
		t.Fatalf("tailwindcss: %v\n%s", err, out)
	}
	geom := posGeometry{}
	for _, scale := range []int{1, 2} {
		key := fmt.Sprintf("x%d", scale)
		for k := range s.steps {
			url := fmt.Sprintf("file://%s#%d", htmlPath, k)
			cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
				"--virtual-time-budget=10000", fmt.Sprintf("--force-device-scale-factor=%d", scale),
				fmt.Sprintf("--window-size=%d,%d", s.w, s.h+chromiumDecorations), "--dump-dom", url)
			outb, err := cmd.Output()
			if err != nil {
				t.Fatalf("chromium: %v", err)
			}
			o := string(outb)
			i, j := strings.Index(o, "BEGIN"), strings.Index(o, "END</pre>")
			if i < 0 || j < 0 {
				t.Fatalf("chromium output has no result:\n%s", o[:min(len(o), 2000)])
			}
			var rects [][4]float64
			if err := json.Unmarshal([]byte(html.UnescapeString(o[i+5:j])), &rects); err != nil {
				t.Fatal(err)
			}
			geom[key] = append(geom[key], rects)
			shot, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("x%d_s%d.png", scale, k)))
			if err != nil {
				t.Fatal(err)
			}
			cmd = exec.Command("chromium", "--headless", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
				"--virtual-time-budget=10000", "--force-color-profile=srgb", fmt.Sprintf("--force-device-scale-factor=%d", scale),
				fmt.Sprintf("--window-size=%d,%d", s.w, s.h), "--screenshot="+shot, url)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("chromium: %v\n%s", err, out)
			}
			writeBestPNG(t, shot, readRGBA(t, shot))
		}
	}
	data, err := json.Marshal(geom)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geometry.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// render renders the scene at scale in every scroll state: the images
// and every element's bounds (window coordinates, document order).
func (s *posScene) render(t *testing.T, scale float32) ([]*image.RGBA, [][]gfx.Rect) {
	t.Helper()
	var logs []string
	tr := NewTree(Config{Fonts: testFonts, RootClass: "bg-white", IDPrefix: "r-", Log: func(m string) { logs = append(logs, m) }})
	tr.SetViewport(gfx.Size{W: float32(s.w), H: float32(s.h)})
	tr.SetScale(scale)
	id := 0
	tr.Mount(s.root.node(&id))
	tr.Frame()
	var imgs []*image.RGBA
	var all [][]gfx.Rect
	for _, step := range s.steps {
		// Each state from the top, like a fresh page.
		for _, sc := range tr.scr.list {
			if sc != nil {
				tr.scrollScripted(sc, gfx.Point{})
			}
		}
		tr.Frame()
		for _, sc := range step {
			var target *scroller
			if sc.id < 0 {
				target = tr.rootBox.scr
			} else if el := keyed(tr, sc.id); el != nil {
				target = el.box.scr
			}
			if target == nil {
				t.Fatalf("scroll target %d is not a scroll container", sc.id)
			}
			tr.scrollScripted(target, target.flip(gfx.Point{X: sc.x, Y: sc.y})) // scrollTo's coordinates
			tr.Frame()
		}
		// Let the overlay scrollbars fade out (Chromium's are hidden).
		for i := range 3 {
			tr.Animate(time.Duration(len(imgs)*10+i+1) * 5 * time.Second)
			tr.Frame()
		}
		dl, _ := tr.Frame()
		imgs = append(imgs, tr.Rasterize(dl, scale))
		rects := make([]gfx.Rect, id)
		for k := range id {
			el := keyed(tr, k)
			if el == nil {
				t.Fatalf("no element %d", k)
			}
			rects[k] = tr.boxWindowRect(el.box)
		}
		all = append(all, rects)
	}
	for _, m := range logs {
		t.Errorf("dev message: %s", m)
	}
	tr.Unmount()
	return imgs, all
}

func (s *posScene) compare(t *testing.T) {
	dir := s.dir()
	if os.Getenv("TELA_CHROMIUM") == "1" {
		s.regenerate(t)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(h) != s.html() {
		t.Fatalf("%s/gallery.html is out of date: the scene changed; regenerate with TELA_CHROMIUM=1", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "geometry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var geom posGeometry
	if err := json.Unmarshal(data, &geom); err != nil {
		t.Fatal(err)
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	for si, scale := range []int{1, 2} {
		imgs, rects := s.render(t, float32(scale))
		want := geom[fmt.Sprintf("x%d", scale)]
		if len(want) != len(s.steps) {
			t.Fatalf("x%d: %d steps in geometry.json, %d in the scene", scale, len(want), len(s.steps))
		}
		for k := range s.steps {
			if dump != "" {
				writePNG(t, filepath.Join(dump, fmt.Sprintf("%s_x%d_s%d_tela.png", s.name, scale, k)), imgs[k])
			}
			worst, worstAt, worstX := 0.0, "", 0.0
			skip := map[int]bool{}
			s.root.skipped(new(int), skip)
			for e, r := range rects[k] {
				if skip[e] {
					continue
				}
				w := want[k][e]
				got := [4]float64{float64(r.X0), float64(r.Y0), float64(r.Width()), float64(r.Height())}
				var dc [2]float64
				for c := range 2 {
					dc[c] = math.Max(math.Abs(got[c]-w[c]), math.Abs(got[c]+got[c+2]-w[c]-w[c+2]))
				}
				d := math.Max(dc[0], dc[1])
				if d > worst {
					worst, worstAt = d, fmt.Sprintf("element %d: %v, Chromium %v", e, got, w)
				}
				worstX = math.Max(worstX, dc[0])
				yBound := s.geom
				if scale == 2 {
					yBound = math.Max(yBound, s.geomY2)
				}
				if dc[0] > s.geom || dc[1] > yBound {
					t.Errorf("x%d step %d element %d: %.3f %.3f %.3f %.3f, Chromium %.3f %.3f %.3f %.3f", scale, k, e,
						got[0], got[1], got[2], got[3], w[0], w[1], w[2], w[3])
				}
			}
			shot := readRGBA(t, filepath.Join(dir, fmt.Sprintf("x%d_s%d.png", scale, k)))
			if shot.Rect != imgs[k].Rect {
				t.Fatalf("x%d step %d: render %v, Chromium %v", scale, k, imgs[k].Rect, shot.Rect)
			}
			d := diffRegion(imgs[k], shot, shot.Rect)
			fmt.Fprintf(&report, "x%d step %d: largest edge difference %.3f px (%s), horizontal %.3f px; pixels max %d mean %.3f >16 %d\n",
				scale, k, worst, worstAt, worstX, d.max, d.mean, d.over)
			if d.mean > s.mean[si] || (s.maxd[si] > 0 && d.max > s.maxd[si]) {
				t.Errorf("x%d step %d: pixels differ from Chromium by max %d mean %.3f (bounds %d, %.3f)", scale, k, d.max, d.mean, s.maxd[si], s.mean[si])
			}
		}
	}
	t.Log("difference from Chromium:\n" + report.String())
}

func (s *posScene) golden(t *testing.T) {
	imgs, _ := s.render(t, 1)
	for k, img := range imgs {
		golden.Image(t, fmt.Sprintf("positioned_%s_s%d", s.name, k), img)
	}
}

// The scenes. Text is kept short: its anti-aliasing differs from
// Chromium's by design (ADR-029, ADR-048).

func rowsOf(n int, class, label string) []inode {
	out := make([]inode, n)
	for i := range out {
		out[i] = iel("div", class, itext(fmt.Sprintf("%s %d", label, i)))
	}
	return out
}

var posScenes = []*posScene{
	{
		// A page taller than the window: a fixed header and button, a
		// fixed badge inside a clipped positioned box (not clipped: the
		// box is not its containing block), a fixed box inside a
		// transformed card (its containing block) and one with auto
		// insets (its static position).
		name: "fixed", w: 500, h: 240,
		root: iel("div", "relative px-4 pt-14 pb-[400px]",
			iel("div", "fixed inset-x-0 top-0 z-20 h-10 bg-slate-800 px-3 text-sm/10 text-white", itext("Header")),
			iel("div", "relative h-16 w-24 overflow-hidden rounded-lg bg-amber-100",
				iel("div", "fixed top-3 right-3 z-30 size-6 rounded-full bg-rose-500")),
			iel("div", "mt-4 h-20 w-40 translate-x-4 rounded-lg bg-sky-100",
				iel("div", "fixed right-2 bottom-2 size-6 rounded bg-sky-600")),
			iel("div", "mt-4 ml-8 h-12 w-48 bg-emerald-100",
				iel("div", "fixed h-4 w-10 bg-emerald-600")),
			iel("div", "fixed right-4 bottom-4 z-20 size-10 rounded-full bg-indigo-600 shadow-lg"),
		),
		steps: [][]posScroll{{}, {{-1, 0, 90}}, {{-1, 0, 300}}},
		geom:  0.02,
		mean:  [2]float64{0.5, 0.5}, maxd: [2]int{255, 255},
	},
	{
		// Sticky headers in a scroll container: stuck, pushed out by
		// the end of their section, a footer stuck to the bottom, and a
		// sticky first column in a horizontal strip.
		name: "sticky", w: 500, h: 300,
		root: iel("div", "p-3",
			iel("div", "h-[180px] overflow-y-auto rounded-lg border border-slate-300 bg-white",
				iel("div", "", append([]inode{iel("div", "sticky top-0 z-10 h-8 bg-slate-800 px-2 text-sm/8 text-white", itext("Section A"))},
					rowsOf(5, "h-7 border-b border-slate-100 px-2 text-xs/7", "Row")...)...),
				iel("div", "", append([]inode{iel("div", "sticky top-0 z-10 h-8 bg-indigo-700 px-2 text-sm/8 text-white", itext("Section B"))},
					rowsOf(5, "h-7 border-b border-slate-100 px-2 text-xs/7", "Row")...)...),
				iel("div", "", append(append([]inode{iel("div", "sticky top-2 z-10 h-8 bg-rose-700 px-2 text-sm/8 text-white", itext("Section C"))},
					rowsOf(5, "h-7 border-b border-slate-100 px-2 text-xs/7", "Row")...),
					iel("div", "sticky bottom-0 h-6 bg-sky-600 px-2 text-xs/6 text-white", itext("Footer")))...),
			),
			iel("div", "mt-3 flex overflow-x-auto rounded-lg bg-slate-100",
				append([]inode{iel("div", "sticky left-0 z-10 h-12 w-16 shrink-0 bg-amber-300 p-2 text-xs", itext("Name"))},
					rowsOf(8, "h-12 w-20 shrink-0 border-r border-slate-300 p-2 text-xs", "Col")...)...),
		),
		// e1 is the list; e24 the strip, after 3 sections of a header
		// and 5 rows, and the footer.
		steps: [][]posScroll{
			{},
			{{1, 0, 60}, {24, 120, 0}},
			{{1, 0, 190}},
			{{1, 0, 330}, {24, 400, 0}},
			{{1, 0, 1000}},
		},
		geom: 0.02,
		mean: [2]float64{0.5, 0.5}, maxd: [2]int{255, 255},
	},
	{
		// The window scrolls: a sticky page header and a sticky sidebar
		// card inside a padded section.
		name: "stickypage", w: 500, h: 240,
		root: iel("div", "",
			iel("div", "sticky top-0 z-20 h-10 bg-white/90 px-3 text-sm/10 shadow", itext("Page")),
			iel("div", "flex items-start gap-3 p-3",
				iel("div", "sticky top-12 h-24 w-24 shrink-0 rounded-lg bg-indigo-100"),
				iel("div", "h-[360px] grow rounded-lg bg-slate-100"),
			),
			iel("div", "h-[300px] bg-slate-200"),
		),
		steps: [][]posScroll{{}, {{-1, 0, 100}}, {{-1, 0, 300}}},
		geom:  0.02,
		mean:  [2]float64{0.5, 0.5}, maxd: [2]int{255, 255},
	},
}

func TestChromiumPositioned(t *testing.T) {
	for _, s := range posScenes {
		t.Run(s.name, s.compare)
	}
}

func TestPositionedGolden(t *testing.T) {
	for _, s := range posScenes {
		t.Run(s.name, s.golden)
	}
}
