package core

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
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// writeBestPNG writes img to path with the best compression (Chromium's
// screenshots are compressed fast).
func writeBestPNG(t *testing.T, path string, img image.Image) {
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

// The inline gallery: paragraphs with decorated inline elements (code
// chips, links, wrapped and nested spans, bidi text, truncation) and
// atomic inlines (badges, buttons, inline-flex icons, tall inline-blocks
// with each vertical-align), rendered here and, as HTML with Tailwind's
// compiled CSS, by headless Chromium. TestChromiumInlineGallery compares
// the geometry of every element (Element.getClientRects: the border box of
// every fragment of an inline element, the border box of other boxes) at
// device scales 1 and 2, and the pixels of each cell; TestInlineGallery
// is its golden. testdata/chromium/inline holds the page (gallery.html,
// which also holds the measuring script), its CSS, Chromium's geometry
// (geometry.json) and screenshots (x1.png, x2.png). Regenerate them with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumInlineGallery ./internal/core/
//
// Set TELA_CHROMIUM_DUMP to a directory to get Tela's renders there.

// inode is a node of an inline case: an element (tag, classes, children)
// or, without a tag, a text.
type inode struct {
	tag, class, text string
	kids             []inode
	// noGeom leaves the element out of the geometry comparison (see the
	// case).
	noGeom bool
	// dark gives the element the dark scheme: SchemeProp here, the
	// class "dark" in the page (theme_gallery_test.go).
	dark bool
	// dir is the element's dir attribute (Dir prop), if any.
	dir string
	// Media (media_gallery_test.go): src is an img's path (SrcPath here,
	// the src attribute in the page), tint its Tint (in the page, a div
	// with that background color and the image as a mask), and svg the
	// markup of an svg element (an Icon here, inline in the page, which
	// gets attrs on its root too: width="1em" height="1em" models the
	// Icon's default size).
	src, tint, svg, attrs string
}

func itext(s string) inode { return inode{text: s} }

// idir returns n with the dir attribute d.
func idir(d string, n inode) inode { n.dir = d; return n }

func iel(tag, class string, kids ...inode) inode { return inode{tag: tag, class: class, kids: kids} }

var inodeKinds = map[string]style.Kind{
	"div": style.KindDiv, "p": style.KindP, "span": style.KindSpan, "code": style.KindCode, "a": style.KindA,
	"strong": style.KindStrong, "em": style.KindEm, "button": style.KindButton, "img": style.KindImg, "svg": style.KindSVG,
}

// node converts n to a Tela node; elements get keys in document order.
func (n inode) node(id *int) Node {
	if n.tag == "" {
		return Text(n.text)
	}
	c := []Child{Key(*id)}
	*id++
	if n.class != "" {
		c = append(c, Class(n.class))
	}
	if n.dark {
		c = append(c, SchemeProp(ColorSchemeDark))
	}
	if n.dir != "" {
		c = append(c, Dir(n.dir))
	}
	if n.src != "" {
		c = append(c, SrcPath(n.src))
	}
	if n.tint != "" {
		c = append(c, Tint(gfx.MustParseHex(n.tint)))
	}
	if n.svg != "" {
		c = append(c, IconSrc(n.svg))
	}
	for _, k := range n.kids {
		c = append(c, k.node(id))
	}
	return Primitive(inodeKinds[n.tag], c...)
}

// html writes n without white space between the tags (it would be text).
func (n inode) html(sb *strings.Builder, id *int) {
	if n.tag == "" {
		sb.WriteString(html.EscapeString(n.text))
		return
	}
	switch {
	case n.tint != "":
		// A tinted image: the color masked by the image's alpha.
		fmt.Fprintf(sb, "<div id=\"e%d\" class=\"%s\"></div>", *id,
			html.EscapeString(strings.TrimSpace(n.class+fmt.Sprintf(" bg-[%s] mask-[url(%s)] mask-size-[100%%_100%%] mask-no-repeat", n.tint, n.src))))
		*id++
		return
	case n.tag == "img":
		fmt.Fprintf(sb, "<img id=\"e%d\" class=\"%s\" src=\"%s\">", *id, html.EscapeString(n.class), n.src)
		*id++
		return
	case n.tag == "svg":
		head, rest, _ := strings.Cut(n.svg, "<svg")
		fmt.Fprintf(sb, "%s<svg id=\"e%d\" class=\"%s\" %s%s", head, *id, html.EscapeString(n.class), n.attrs, rest)
		*id++
		return
	}
	fmt.Fprintf(sb, "<%s id=\"e%d\"", n.tag, *id)
	*id++
	if cls := n.class; cls != "" || n.dark {
		if n.dark {
			cls = strings.TrimSpace("dark " + cls)
		}
		fmt.Fprintf(sb, " class=\"%s\"", html.EscapeString(cls))
	}
	if n.dir != "" {
		fmt.Fprintf(sb, " dir=\"%s\"", n.dir)
	}
	sb.WriteString(">")
	for _, k := range n.kids {
		k.html(sb, id)
	}
	fmt.Fprintf(sb, "</%s>", n.tag)
}

// skipped appends the ids of the elements left out of the geometry
// comparison.
func (n inode) skipped(id *int, out map[int]bool) {
	if n.tag == "" {
		return
	}
	if n.noGeom {
		out[*id] = true
	}
	*id++
	for _, k := range n.kids {
		k.skipped(id, out)
	}
}

type inlineGalleryCase struct {
	name   string
	height int // of the cell, logical px
	root   inode
}

const (
	inlineGalleryWidth = 440
	chip               = "rounded bg-slate-100 px-1 py-0.5 font-mono text-sm text-slate-800"
	badge              = "inline-block rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700"
	btn                = "rounded-md bg-indigo-600 px-2 py-1 text-sm font-medium text-white"
)

var inlineGalleryCases = []inlineGalleryCase{
	{"code-chip", 40, iel("p", "text-slate-700", itext("Run "), iel("code", chip, itext("go test ./...")), itext(" now, then "), iel("code", chip, itext("tela snapshot")), itext("."))},
	{"link", 40, iel("p", "text-slate-700", itext("Read the "), iel("a", "rounded px-0.5 text-indigo-600 underline", itext("documentation")), itext(" and the "), iel("a", "rounded bg-indigo-50 px-1 font-medium text-indigo-700 underline decoration-2", itext("guide")), itext(" first."))},
	{"wrap-3", 96, iel("p", "w-[230px] text-slate-700", itext("Some "), iel("span", "rounded-md border border-amber-400 bg-amber-100 px-2", itext("decorated text that wraps over three lines of this narrow paragraph")), itext(" end."))},
	{"gradient-wrap", 72, iel("p", "w-[250px] font-semibold", itext("A "), iel("span", "rounded-lg bg-linear-to-r from-pink-500 to-violet-500 px-1.5 text-white shadow-md", itext("gradient highlight that continues on the next line")), itext(" here."))},
	{"nested", 40, iel("p", "text-slate-700", itext("Call "), iel("a", "rounded border border-sky-300 bg-sky-50 px-1 text-sky-700", itext("the "), iel("code", "rounded bg-white px-1 font-mono text-sm", itext("Render()")), itext(" "), iel("strong", "", itext("function"))), itext(" twice."))},
	{"nested-wrap", 72, iel("p", "w-[170px] text-slate-700", itext("Call "), iel("a", "rounded border border-sky-300 bg-sky-50 px-1 text-sky-700", itext("the "), iel("code", "rounded bg-white px-1 font-mono text-sm", itext("Render()")), itext(" "), iel("strong", "", itext("function"))), itext(" twice."))},
	{"sides", 40, iel("p", "text-slate-700", itext("Note "), iel("span", "border-y border-l-4 border-y-slate-300 border-l-amber-500 bg-amber-50 py-1 pr-1 pl-2", itext("left accent")), itext(" and "), iel("em", "mx-2 rounded-sm bg-yellow-200 px-1", itext("margins")), itext(" end."))},
	{"empty", 40, iel("p", "text-slate-700", itext("Dot "), iel("span", "rounded-full bg-rose-400 px-1.5"), itext(" and "), iel("span", "border-r-2 border-r-slate-900 pl-3"), itext("caret."))},
	{"bidi", 72, iel("p", "w-[400px] text-slate-700", itext("Price: "), iel("span", "rounded bg-teal-100 px-1.5", itext("120 دولار أمريكي")), itext(" today. Split: abc "), iel("span", "rounded border border-teal-500 bg-teal-50 px-1", itext("xyz عر")), itext("بي def"))},
	{"arabic", 40, iel("p", "text-slate-700", itext("مرحبا "), iel("span", "rounded bg-orange-100 px-2", itext("بكم في")), itext(" عالم النصوص"))},
	{"vertical-padding", 72, iel("p", "w-[200px] text-slate-700", iel("span", "bg-sky-500/30 px-1 py-1.5", itext("padding overlaps the line above and below it")))},
	{"truncate", 40, iel("p", "w-[260px] truncate text-slate-700", itext("Truncated "), inode{tag: "span", class: "rounded bg-violet-100 px-2 text-violet-700", kids: []inode{itext("label that runs past the end")}, noGeom: true}, itext(" of the box"))},
	{"line-clamp", 64, iel("p", "line-clamp-2 w-[220px] text-slate-700", itext("Clamped "), inode{tag: "span", class: "rounded bg-lime-200 px-1", kids: []inode{itext("text whose highlight goes past the second line of the clamp")}, noGeom: true}, itext(" and more."))},
	{"badge", 40, iel("p", "text-slate-700", itext("Release "), iel("span", badge, itext("New")), itext(" is out "), iel("span", "inline-block rounded bg-slate-900 px-1.5 text-xs leading-5 text-white", itext("v0.5")), itext(" today."))},
	{"buttons", 56, iel("p", "text-slate-700", itext("Click "), iel("button", btn, itext("Save")), itext(" or "), iel("button", btn+" align-middle", itext("Middle")), itext(" or "), iel("button", btn+" align-top", itext("Top")), itext(" or "), iel("button", btn+" align-text-bottom", itext("Bottom")), itext("."))},
	{"inline-flex", 40, iel("p", "text-slate-700", itext("Status "), iel("span", "inline-flex items-center gap-1 rounded bg-slate-100 px-1.5 text-sm", iel("span", "inline-block size-2.5 rounded-full bg-emerald-500"), itext("Online")), itext(" and "), iel("span", "inline-flex items-baseline gap-1 rounded bg-slate-100 px-1.5 text-sm", iel("span", "inline-block size-2.5 rounded-full bg-amber-500"), itext("Away")), itext("."))},
	{"tall", 72, iel("p", "text-slate-700", itext("Tall "), iel("span", "inline-block h-12 w-10 rounded bg-amber-300"), itext(" base "), iel("span", "inline-block h-12 w-10 rounded bg-amber-400 align-top"), itext(" top "), iel("span", "inline-block h-12 w-10 rounded bg-amber-500 align-bottom"), itext(" bottom "), iel("span", "inline-block h-10 w-10 rounded bg-amber-600 align-middle"), itext(" mid"))},
	{"inline-block-text", 64, iel("p", "text-slate-700", itext("Box "), iel("span", "inline-block w-24 rounded border border-slate-300 px-1 text-xs", itext("two lines of text inside it")), itext(" baseline "), iel("span", "inline-block overflow-hidden rounded bg-slate-200 px-1 text-xs", itext("clipped")), itext(" end"))},
	{"block-button", 48, iel("div", "", iel("button", "rounded bg-slate-800 px-3 py-1 text-xs text-white", itext("Lone button")))},
	{"centered", 40, iel("p", "w-[400px] text-center text-slate-700", itext("Centered "), iel("span", badge, itext("badge")), itext(" and "), iel("code", chip, itext("code")))},
	{"space-x", 40, iel("p", "space-x-3 text-slate-700", iel("span", "rounded bg-pink-100 px-1", itext("one")), iel("span", "rounded bg-pink-200 px-1", itext("two")), iel("button", "rounded bg-pink-300 px-1", itext("three")))},
	{"wrap-atoms", 72, iel("p", "w-[200px] text-slate-700", itext("Tags: "), iel("span", badge, itext("design")), itext(" "), iel("span", badge, itext("systems")), itext(" "), iel("span", badge, itext("typography")), itext(" "), iel("span", badge, itext("layout")))},
}

func inlineGalleryRoot() inode {
	var cells []inode
	for _, c := range inlineGalleryCases {
		cells = append(cells, iel("div", fmt.Sprintf("h-[%dpx] overflow-visible px-2 pt-2", c.height), c.root))
	}
	return iel("div", fmt.Sprintf("w-[%dpx]", inlineGalleryWidth), cells...)
}

func inlineGallerySize() (w, h float32) {
	for _, c := range inlineGalleryCases {
		h += float32(c.height)
	}
	return inlineGalleryWidth, h
}

// inlineGalleryFonts loads the embedded fonts in the page.
const inlineGalleryFonts = `@font-face { font-family: "Inter"; src: url("../../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }
@font-face { font-family: "Inter"; src: url("../../../../assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
@font-face { font-family: "JetBrains Mono"; src: url("../../../../assets/fonts/jetbrainsmono/JetBrainsMono-wght.ttf"); font-weight: 100 800; }
@font-face { font-family: "Noto Sans Arabic"; src: url("../../../../assets/fonts/notosansarabic/NotoSansArabic-wdth-wght.ttf"); font-weight: 100 900; font-stretch: 62.5% 100%; }
`

func inlineGalleryHTML() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"gallery.css\">\n</head>\n<body class=\"bg-white\">\n")
	id := 0
	inlineGalleryRoot().html(&sb, &id)
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

// inlineGeometry is Chromium's geometry: per scale ("x1", "x2"), per
// element (document order), its rects [left, top, width, height].
type inlineGeometry map[string][][][4]float64

func regenerateInlineGallery(t *testing.T, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath, err := filepath.Abs(filepath.Join(dir, "gallery.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(inlineGalleryHTML()), 0o644); err != nil {
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
	tmp := t.TempDir()
	input := filepath.Join(tmp, "input.css")
	css := fmt.Sprintf("@import %q;\n@source %q;\n@theme { --font-sans: \"Inter\", \"Noto Sans Arabic\"; --font-mono: \"JetBrains Mono\"; }\n%s",
		filepath.Join(modules, "tailwindcss", "index.css"), htmlPath, inlineGalleryFonts)
	if err := os.WriteFile(input, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tw, "-i", input, "-o", filepath.Join(dir, "gallery.css")).CombinedOutput(); err != nil {
		t.Fatalf("tailwindcss: %v\n%s", err, out)
	}
	w, h := inlineGallerySize()
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

// renderInlineGallery renders the gallery at scale and returns the image
// and the rects of every element (window coordinates; the fragments of
// inline elements, the border box of the others), in document order.
func renderInlineGallery(t *testing.T, scale float32) (*image.RGBA, [][]gfx.Rect) {
	t.Helper()
	var logs []string
	cfg := Config{Fonts: testFonts, RootClass: "bg-white", IDPrefix: "r-", Log: func(m string) { logs = append(logs, m) }}
	w, h := inlineGallerySize()
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.SetScale(scale)
	id := 0
	tr.Mount(inlineGalleryRoot().node(&id))
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

// inlineGalleryCells returns each case's cell (logical px).
func inlineGalleryCells() [][4]int {
	var out [][4]int
	y := 0
	for _, c := range inlineGalleryCases {
		out = append(out, [4]int{0, y, inlineGalleryWidth, y + c.height})
		y += c.height
	}
	return out
}

// inlineGalleryMean bounds the mean pixel difference of a cell, per
// scale: text anti-aliasing differs from Skia's on every glyph edge (see
// chromiumBounds "counter"; single pixels differ by up to 255 levels, so
// the maximum is not bounded), and at scale 2 Blink rounds line metrics in
// device pixels, which moves most lines of text, and the fragments and
// atomic inlines on them, by one device pixel (ADR-048).
var inlineGalleryMean = [2]float64{1.5, 7}

func TestChromiumInlineGallery(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "inline")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateInlineGallery(t, dir)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(h) != inlineGalleryHTML() {
		t.Fatal("testdata/chromium/inline/gallery.html is out of date: the gallery changed; regenerate with TELA_CHROMIUM=1")
	}
	data, err := os.ReadFile(filepath.Join(dir, "geometry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var geom inlineGeometry
	if err := json.Unmarshal(data, &geom); err != nil {
		t.Fatal(err)
	}
	skip := map[int]bool{}
	id := 0
	inlineGalleryRoot().skipped(&id, skip)
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	cells := inlineGalleryCells()
	var report strings.Builder
	for _, scale := range []int{1, 2} {
		img, rects := renderInlineGallery(t, float32(scale))
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("inline_x%d_tela.png", scale)), img)
		}
		want := geom[fmt.Sprintf("x%d", scale)]
		if len(want) != len(rects) {
			t.Fatalf("x%d: %d elements here, %d in Chromium", scale, len(rects), len(want))
		}
		worst, worstX, worstAt := 0.0, 0.0, ""
		for k := range rects {
			if skip[k] {
				continue
			}
			got := rects[k]
			w := want[k]
			// Chromium lists empty rects for some fragments that hold
			// nothing visible; so do we. Compare sorted by top, then left.
			g := make([][4]float64, len(got))
			for i, r := range got {
				g[i] = [4]float64{float64(r.X0), float64(r.Y0), float64(r.Width()), float64(r.Height())}
			}
			sortRects(g)
			ws := slices.Clone(w)
			sortRects(ws)
			if len(g) != len(ws) {
				t.Errorf("x%d element %d (%s): %d rects %v, Chromium %d %v", scale, k, caseOf(k), len(g), g, len(ws), ws)
				continue
			}
			for i := range g {
				var dc [2]float64 // horizontal and vertical edges
				for c := range 2 {
					dc[c] = math.Max(math.Abs(g[i][c]-ws[i][c]), math.Abs(g[i][c]+g[i][c+2]-ws[i][c]-ws[i][c+2]))
				}
				d := math.Max(dc[0], dc[1])
				if d > worst {
					worst, worstAt = d, fmt.Sprintf("element %d (%s) rect %d: %v, Chromium %v", k, caseOf(k), i, g[i], ws[i])
				}
				worstX = math.Max(worstX, dc[0])
				if tx, ty := inlineGeometryTolerance(scale); dc[0] > tx || dc[1] > ty {
					t.Errorf("x%d element %d (%s) rect %d: %.3f %.3f %.3f %.3f, Chromium %.3f %.3f %.3f %.3f", scale, k, caseOf(k), i,
						g[i][0], g[i][1], g[i][2], g[i][3], ws[i][0], ws[i][1], ws[i][2], ws[i][3])
				}
			}
		}
		fmt.Fprintf(&report, "x%d: largest edge difference %.3f px (%s); horizontal edges %.3f px\n", scale, worst, worstAt, worstX)
		shot := readRGBA(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if shot.Rect != img.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, img.Rect, shot.Rect)
		}
		for i, c := range cells {
			d := diffRegion(img, shot, image.Rect(c[0]*scale, c[1]*scale, c[2]*scale, c[3]*scale))
			fmt.Fprintf(&report, "  %-18s max %3d mean %.3f >16 %d\n", inlineGalleryCases[i].name, d.max, d.mean, d.over)
			if d.mean > inlineGalleryMean[scale-1] {
				t.Errorf("x%d %s: pixels differ from Chromium by max %d mean %.3f", scale, inlineGalleryCases[i].name, d.max, d.mean)
			}
		}
	}
	t.Log("difference from Chromium:\n" + report.String())
}

// inlineGeometryTolerance is the largest difference accepted for
// horizontal and vertical edges: text positions agree within hundredths
// of a pixel (TestChromiumInline in internal/text); at scale 2 Blink
// rounds font metrics and line boxes in device pixels, which moves
// baselines (and the atomic inlines on them) by up to half a logical
// pixel and the ascent and descent of fragments' content areas by up to
// half a pixel more (ADR-048: Tela quantizes them in logical pixels).
func inlineGeometryTolerance(scale int) (x, y float64) {
	if scale == 2 {
		return 0.1, 1.01
	}
	return 0.1, 0.1
}

// caseOf returns the case that element k is in.
func caseOf(k int) string {
	id := 1 // the gallery's root div
	for _, c := range inlineGalleryCases {
		n := countElements(c.root) + 1 // the cell
		if k < id+n {
			return c.name
		}
		id += n
	}
	return "?"
}

func countElements(n inode) int {
	if n.tag == "" {
		return 0
	}
	k := 1
	for _, c := range n.kids {
		k += countElements(c)
	}
	return k
}

func sortRects(rs [][4]float64) {
	slices.SortFunc(rs, func(a, b [4]float64) int {
		if d := math.Round(a[1]) - math.Round(b[1]); d != 0 {
			if d < 0 {
				return -1
			}
			return 1
		}
		switch {
		case a[0] < b[0]:
			return -1
		case a[0] > b[0]:
			return 1
		}
		return 0
	})
}

func TestInlineGallery(t *testing.T) {
	for _, scale := range []int{1, 2} {
		img, _ := renderInlineGallery(t, float32(scale))
		golden.Image(t, fmt.Sprintf("inline_gallery_x%d", scale), img)
	}
}
