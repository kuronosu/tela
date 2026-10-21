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
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// A cssGallery is a stack of rows of Tailwind elements rendered here and,
// as HTML with Tailwind's compiled CSS, by headless Chromium (the same
// scheme as the inline gallery, inline_gallery_test.go). Each gallery has a
// directory under testdata/chromium with its page (gallery.html, which
// also holds the measuring script), its CSS, Chromium's geometry
// (geometry.json: Element.getClientRects of every element at device
// scales 1 and 2) and screenshots (x1.png, x2.png). The test compares the
// geometry of every element and the pixels of each row; a second test is
// the gallery's golden. Regenerate Chromium's files with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run 'TestChromium<Gallery>' ./internal/core/
//
// Set TELA_CHROMIUM_DUMP to a directory to get Tela's renders there.
type cssGallery struct {
	// name is the directory under testdata/chromium and the golden's name.
	name  string
	width int
	rows  []cssRow
	// mean is the largest mean pixel difference of a row per scale; maxd
	// the largest single-channel difference (0: not bounded).
	mean [2]float64
	maxd [2]int
	// geom is the largest edge difference accepted, per scale, in logical
	// pixels.
	geom [2]float64
	// fontCSS replaces the page's @theme font and @font-face rules
	// (cssGalleryFonts with Inter as the sans font when empty).
	fontCSS string
	// theme is the tree's theme (nil: the default).
	theme *Theme
}

// cssRow is one row of a gallery: height logical pixels tall, its
// elements laid out by flex with class (added to the row's own).
type cssRow struct {
	name   string
	height int
	class  string
	kids   []inode
}

// cssGalleryFonts loads the embedded fonts in the page, relative to
// testdata/chromium/<name>.
const cssGalleryFonts = `@font-face { font-family: "Inter"; src: url("../../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }
@font-face { font-family: "Inter"; src: url("../../../../assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
`

func (g *cssGallery) root() inode {
	var rows []inode
	for _, r := range g.rows {
		cls := fmt.Sprintf("flex h-[%dpx] shrink-0 items-center px-4", r.height)
		if r.class != "" {
			cls += " " + r.class
		}
		rows = append(rows, iel("div", cls, r.kids...))
	}
	return iel("div", fmt.Sprintf("w-[%dpx]", g.width), rows...)
}

func (g *cssGallery) size() (w, h float32) {
	for _, r := range g.rows {
		h += float32(r.height)
	}
	return float32(g.width), h
}

func (g *cssGallery) html() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"gallery.css\">\n</head>\n<body class=\"bg-white\">\n")
	id := 0
	root := g.root()
	root.html(&sb, &id)
	fmt.Fprintf(&sb, `
<pre id="out" style="display: none"></pre>
<script>
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = [];
  for (let i = 0; i < %d; i++) {
    const e = document.getElementById("e" + i);
    out.push([...e.getClientRects()].map(q => [q.left, q.top, q.width, q.height]));
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script>
</body>
</html>
`, id)
	return sb.String()
}

func (g *cssGallery) dir() string { return filepath.Join("testdata", "chromium", g.name) }

func (g *cssGallery) regenerate(t *testing.T) {
	dir := g.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath, err := filepath.Abs(filepath.Join(dir, "gallery.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(g.html()), 0o644); err != nil {
		t.Fatal(err)
	}
	tw := os.Getenv("TELA_TAILWIND")
	if tw == "" {
		if tw, err = exec.LookPath("tailwindcss"); err != nil {
			t.Fatal("the Tailwind CLI is needed: set TELA_TAILWIND to node_modules/.bin/tailwindcss")
		}
	}
	bin, err := filepath.EvalSymlinks(tw)
	if err != nil {
		t.Fatal(err)
	}
	modules := bin
	for filepath.Base(modules) != "node_modules" {
		if next := filepath.Dir(modules); next != modules {
			modules = next
			continue
		}
		t.Fatalf("%s is not inside a node_modules directory", tw)
	}
	input := filepath.Join(t.TempDir(), "input.css")
	fonts := g.fontCSS
	if fonts == "" {
		fonts = "@theme { --font-sans: \"Inter\"; }\n" + cssGalleryFonts
	}
	css := fmt.Sprintf("@import %q;\n@source %q;\n%s",
		filepath.Join(modules, "tailwindcss", "index.css"), htmlPath, fonts)
	if err := os.WriteFile(input, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tw, "-i", input, "-o", filepath.Join(dir, "gallery.css")).CombinedOutput(); err != nil {
		t.Fatalf("tailwindcss: %v\n%s", err, out)
	}
	w, h := g.size()
	geom := inlineGeometry{}
	for _, scale := range []int{1, 2} {
		cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--allow-file-access-from-files", "--virtual-time-budget=10000",
			fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", int(w), int(h)), "--dump-dom", "file://"+htmlPath)
		outb, err := cmd.Output()
		if err != nil {
			t.Fatalf("chromium: %v", err)
		}
		s := string(outb)
		i, j := strings.Index(s, "BEGIN"), strings.Index(s, "END</pre>")
		if i < 0 || j < 0 {
			t.Fatalf("chromium output has no result:\n%s", s[:min(len(s), 2000)])
		}
		var rects [][][4]float64
		if err := json.Unmarshal([]byte(html.UnescapeString(s[i+5:j])), &rects); err != nil {
			t.Fatal(err)
		}
		geom[fmt.Sprintf("x%d", scale)] = rects
		shot, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command("chromium", "--headless", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
			"--virtual-time-budget=10000", // fonts and background images loaded
			"--force-color-profile=srgb", fmt.Sprintf("--force-device-scale-factor=%d", scale),
			fmt.Sprintf("--window-size=%d,%d", int(w), int(h)), "--screenshot="+shot, "file://"+htmlPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("chromium: %v\n%s", err, out)
		}
		writeBestPNG(t, shot, readRGBA(t, shot))
	}
	var sb strings.Builder
	sb.WriteString("{\n")
	for k, key := range []string{"x1", "x2"} {
		fmt.Fprintf(&sb, "%q: [\n", key)
		for i, r := range geom[key] {
			b, _ := json.Marshal(r)
			sep := ","
			if i == len(geom[key])-1 {
				sep = ""
			}
			fmt.Fprintf(&sb, "  %s%s\n", b, sep)
		}
		sep := ","
		if k == 1 {
			sep = ""
		}
		fmt.Fprintf(&sb, "]%s\n", sep)
	}
	sb.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "geometry.json"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// render renders the gallery at scale and returns the image and the
// rects of every element (window coordinates, document order).
func (g *cssGallery) render(t *testing.T, scale float32) (*image.RGBA, [][]gfx.Rect) {
	t.Helper()
	var logs []string
	cfg := Config{Fonts: testFonts, RootClass: "bg-white", IDPrefix: "r-", Theme: g.theme, Log: func(m string) { logs = append(logs, m) }}
	w, h := g.size()
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.SetScale(scale)
	id := 0
	tr.Mount(g.root().node(&id))
	dl, _ := tr.Frame()
	img := tr.Rasterize(dl, scale)
	for _, m := range logs {
		t.Errorf("dev message: %s", m)
	}
	byKey := map[int]*Element{}
	var walk func(el *Element)
	walk = func(el *Element) {
		if k, ok := el.key.(int); ok && el.keyed && el.kind == elemPrimitive {
			byKey[k] = el
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(tr.root)
	rects := make([][]gfx.Rect, id)
	for k := range id {
		el := byKey[k]
		if el == nil {
			t.Fatalf("no element %d", k)
		}
		if p := el.box.inPara; p != nil {
			l := p.ld.Layout()
			o := tr.semOrigin(p).Add(gfx.Point{X: l.Border.Left + l.Padding.Left, Y: l.Border.Top + l.Padding.Top})
			for _, r := range p.para.inlineRects(el, nil) {
				rects[k] = append(rects[k], r.Translate(o.X, o.Y))
			}
			continue
		}
		rects[k] = []gfx.Rect{tr.boxWindowRect(el.box)}
	}
	tr.Unmount()
	return img, rects
}

// cells returns each row's cell (logical px).
func (g *cssGallery) cells() [][4]int {
	var out [][4]int
	y := 0
	for _, r := range g.rows {
		out = append(out, [4]int{0, y, g.width, y + r.height})
		y += r.height
	}
	return out
}

// rowOf returns the row that element k is in.
func (g *cssGallery) rowOf(k int) string {
	id := 1 // the root
	for _, r := range g.rows {
		n := 1
		for _, c := range r.kids {
			n += countElements(c)
		}
		if k < id+n {
			return r.name
		}
		id += n
	}
	return "?"
}

// compare runs the comparison with Chromium, regenerating its files first
// when TELA_CHROMIUM=1.
func (g *cssGallery) compare(t *testing.T) {
	dir := g.dir()
	if os.Getenv("TELA_CHROMIUM") == "1" {
		g.regenerate(t)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(h) != g.html() {
		t.Fatalf("%s/gallery.html is out of date: the gallery changed; regenerate with TELA_CHROMIUM=1", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "geometry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var geom inlineGeometry
	if err := json.Unmarshal(data, &geom); err != nil {
		t.Fatal(err)
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	cells := g.cells()
	var report strings.Builder
	for si, scale := range []int{1, 2} {
		img, rects := g.render(t, float32(scale))
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("%s_x%d_tela.png", g.name, scale)), img)
		}
		want := geom[fmt.Sprintf("x%d", scale)]
		if len(want) != len(rects) {
			t.Fatalf("x%d: %d elements here, %d in Chromium", scale, len(rects), len(want))
		}
		worst, worstAt := 0.0, ""
		for k := range rects {
			got := make([][4]float64, len(rects[k]))
			for i, r := range rects[k] {
				got[i] = [4]float64{float64(r.X0), float64(r.Y0), float64(r.Width()), float64(r.Height())}
			}
			sortRects(got)
			ws := slices.Clone(want[k])
			sortRects(ws)
			if len(ws) == 0 && len(got) == 1 && got[0] == [4]float64{} {
				continue // display: none: Chromium has no rects, we have an empty one
			}
			if len(got) != len(ws) {
				t.Errorf("x%d element %d (%s): %d rects %v, Chromium %d %v", scale, k, g.rowOf(k), len(got), got, len(ws), ws)
				continue
			}
			for i := range got {
				d := 0.0
				for c := range 2 {
					d = math.Max(d, math.Max(math.Abs(got[i][c]-ws[i][c]), math.Abs(got[i][c]+got[i][c+2]-ws[i][c]-ws[i][c+2])))
				}
				if d > worst {
					worst, worstAt = d, fmt.Sprintf("element %d (%s): %v, Chromium %v", k, g.rowOf(k), got[i], ws[i])
				}
				if d > g.geom[si] {
					t.Errorf("x%d element %d (%s) rect %d: %.3f %.3f %.3f %.3f, Chromium %.3f %.3f %.3f %.3f", scale, k, g.rowOf(k), i,
						got[i][0], got[i][1], got[i][2], got[i][3], ws[i][0], ws[i][1], ws[i][2], ws[i][3])
				}
			}
		}
		fmt.Fprintf(&report, "x%d: largest edge difference %.3f px (%s)\n", scale, worst, worstAt)
		shot := readRGBA(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if shot.Rect != img.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, img.Rect, shot.Rect)
		}
		for i, c := range cells {
			d := diffRegion(img, shot, image.Rect(c[0]*scale, c[1]*scale, c[2]*scale, c[3]*scale))
			fmt.Fprintf(&report, "  %-18s max %3d mean %.3f >16 %d\n", g.rows[i].name, d.max, d.mean, d.over)
			if d.mean > g.mean[si] || (g.maxd[si] > 0 && d.max > g.maxd[si]) {
				t.Errorf("x%d %s: pixels differ from Chromium by max %d mean %.3f (bounds %d, %.3f)", scale, g.rows[i].name, d.max, d.mean, g.maxd[si], g.mean[si])
			}
		}
	}
	t.Log("difference from Chromium:\n" + report.String())
}

// golden checks the gallery's render against its golden images.
func (g *cssGallery) golden(t *testing.T) {
	for _, scale := range []int{1, 2} {
		img, _ := g.render(t, float32(scale))
		golden.Image(t, fmt.Sprintf("%s_x%d", g.name, scale), img)
	}
}
