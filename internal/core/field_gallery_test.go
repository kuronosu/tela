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

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The field gallery: text fields as Tailwind's Preflight leaves them and
// styled with classes, rendered here and, as HTML with Tailwind's compiled
// CSS, by headless Chromium. TestChromiumFieldGallery compares the border
// box of every measured element (getBoundingClientRect) and the pixels of
// each cell; TestFieldGallery is its golden. testdata/chromium/fields
// holds the page (gallery.html, with the measuring script), its CSS,
// Chromium's geometry (geometry.json) and screenshots (x1.png, x2.png).
// Regenerate them with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumFieldGallery ./internal/core/
//
// Chromium lays a textarea out with its 15 px classic scrollbar, so the
// gallery is rendered with classic scrollbars (no textarea in it
// overflows). Chromium paints its own focus ring on a focused field
// (outline: auto): the focused cell sets outline-none on both sides.

// fnode is a node of the field gallery: an element (tag, classes,
// attributes, children; measured when id is set) or a text.
type fnode struct {
	tag, class, text string
	attrs            [][2]string
	kids             []fnode
	measure          bool
}

func fel(tag, class string, kids ...fnode) fnode { return fnode{tag: tag, class: class, kids: kids} }

func ftext(s string) fnode { return fnode{text: s} }

// fm is a measured element with attributes.
func fm(tag, class string, attrs ...string) fnode {
	n := fnode{tag: tag, class: class, measure: true}
	for i := 0; i+1 < len(attrs); i += 2 {
		n.attrs = append(n.attrs, [2]string{attrs[i], attrs[i+1]})
	}
	return n
}

var fieldKinds = map[string]style.Kind{
	"div": style.KindDiv, "p": style.KindP, "input": style.KindInput, "textarea": style.KindTextarea,
	"label": style.KindLabel, "span": style.KindSpan,
}

// node converts n to a Tela node; measured elements get keys in document
// order.
func (n fnode) node(id *int) Node {
	if n.tag == "" {
		return Text(n.text)
	}
	var c []Child
	if n.measure {
		c = append(c, Key(*id))
		*id++
	}
	if n.class != "" {
		c = append(c, Class(n.class))
	}
	for _, a := range n.attrs {
		switch a[0] {
		case "value":
			c = append(c, DefaultValue(a[1]))
		case "placeholder":
			c = append(c, Placeholder(a[1]))
		case "type":
			c = append(c, FieldType(a[1]))
		case "rows":
			var r int
			fmt.Sscan(a[1], &r)
			c = append(c, Rows(r))
		}
	}
	for _, k := range n.kids {
		c = append(c, k.node(id))
	}
	return Primitive(fieldKinds[n.tag], c...)
}

// html writes n; a textarea's value is its content.
func (n fnode) html(sb *strings.Builder, id *int) {
	if n.tag == "" {
		sb.WriteString(html.EscapeString(n.text))
		return
	}
	fmt.Fprintf(sb, "<%s", n.tag)
	if n.measure {
		fmt.Fprintf(sb, " id=\"e%d\"", *id)
		*id++
	}
	if n.class != "" {
		fmt.Fprintf(sb, " class=\"%s\"", html.EscapeString(n.class))
	}
	content := ""
	for _, a := range n.attrs {
		if n.tag == "textarea" && a[0] == "value" {
			content = a[1]
			continue
		}
		fmt.Fprintf(sb, " %s=\"%s\"", a[0], html.EscapeString(a[1]))
	}
	sb.WriteString(">")
	if n.tag == "input" {
		return
	}
	sb.WriteString(html.EscapeString(content))
	for _, k := range n.kids {
		k.html(sb, id)
	}
	fmt.Fprintf(sb, "</%s>", n.tag)
}

type fieldGalleryCase struct {
	name   string
	height int
	root   fnode
	// sel selects [sel[0], sel[1]) of the measured field of the case and
	// focuses it.
	sel []int
}

const (
	fieldGalleryWidth = 360
	twInput           = "w-full rounded-md border border-slate-300 px-3 py-2 placeholder:text-slate-400"
)

var fieldGalleryCases = []fieldGalleryCase{
	{"input", 40, fm("input", "", "value", "Hello world"), nil},
	{"placeholder", 40, fm("input", "", "placeholder", "Email address"), nil},
	{"tailwind", 56, fel("div", "w-[300px]", fm("input", twInput, "placeholder", "you@example.com")), nil},
	{"tailwind-value", 56, fel("div", "w-[300px]", fm("input", twInput+" text-slate-900", "value", "ana@example.com")), nil},
	{"tall", 64, fm("input", "h-12 border border-slate-400 px-2", "value", "Centered Ag"), nil},
	{"small", 40, fel("div", "text-sm", fm("input", "border border-slate-300", "value", "Small text")), nil},
	{"large", 48, fel("div", "text-2xl font-semibold", fm("input", "w-72 border-b-2 border-indigo-500", "value", "Large")), nil},
	{"password", 40, fm("input", "border border-slate-300 px-1", "type", "password", "value", "secret"), nil},
	{"centered", 40, fm("input", "w-48 border border-slate-300 text-center", "value", "middle"), nil},
	{"right", 40, fm("input", "w-48 border border-slate-300 text-right", "placeholder", "0.00"), nil},
	{"textarea", 64, fm("textarea", "", "value", "A textarea wraps its text over several lines"), nil},
	{"textarea-rows", 128, fm("textarea", "w-72 rounded-md border border-slate-300 p-2", "rows", "4", "value", "Rows 4\nwith a second line"), nil},
	{"textarea-placeholder", 64, fm("textarea", "border border-slate-300", "placeholder", "Write a message"), nil},
	{"fs-input", 40, fm("input", "field-sizing-content border border-slate-300", "value", "Fits its text"), nil},
	{"fs-textarea", 64, fm("textarea", "field-sizing-content border border-slate-300", "value", "Two\nlines"), nil},
	{"baseline", 40, fel("p", "text-slate-700", ftext("Name "), fm("input", "w-32 border border-slate-300 px-1", "value", "inline"), ftext(" end")), nil},
	{"selection", 40, fm("input", "border border-slate-300 outline-none", "value", "Selected text"), []int{0, 8}},
}

func fieldGalleryRoot() fnode {
	var cells []fnode
	for _, c := range fieldGalleryCases {
		cells = append(cells, fel("div", fmt.Sprintf("h-[%dpx] px-2 pt-2", c.height), c.root))
	}
	return fel("div", fmt.Sprintf("w-[%dpx]", fieldGalleryWidth), cells...)
}

func fieldGallerySize() (w, h float32) {
	for _, c := range fieldGalleryCases {
		h += float32(c.height)
	}
	return fieldGalleryWidth, h
}

// fieldGallerySelections returns, per measured id, the selection to make.
func fieldGallerySelections() map[int][]int {
	out := map[int][]int{}
	id := 0
	var walk func(n fnode, sel []int)
	walk = func(n fnode, sel []int) {
		if n.tag == "" {
			return
		}
		if n.measure {
			if sel != nil && (n.tag == "input" || n.tag == "textarea") {
				out[id] = sel
			}
			id++
		}
		for _, k := range n.kids {
			walk(k, sel)
		}
	}
	for _, c := range fieldGalleryCases {
		walk(c.root, c.sel)
	}
	return out
}

func fieldGalleryHTML() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"gallery.css\">\n</head>\n<body class=\"bg-white\">\n")
	id := 0
	fieldGalleryRoot().html(&sb, &id)
	sels, _ := json.Marshal(fieldGallerySelections())
	fmt.Fprintf(&sb, `
<pre id="out" style="display: none"></pre>
<script>
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  for (const [k, s] of Object.entries(%s)) {
    const e = document.getElementById("e" + k);
    e.focus({preventScroll: true});
    e.setSelectionRange(s[0], s[1]);
  }
  const out = [];
  for (let i = 0; i < %d; i++) {
    const r = document.getElementById("e" + i).getBoundingClientRect();
    out.push([r.left, r.top, r.width, r.height]);
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script>
</body>
</html>
`, sels, id)
	return sb.String()
}

func regenerateFieldGallery(t *testing.T, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath, err := filepath.Abs(filepath.Join(dir, "gallery.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(fieldGalleryHTML()), 0o644); err != nil {
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
	css := fmt.Sprintf("@import %q;\n@source %q;\n@theme { --font-sans: \"Inter\"; }\n%s",
		filepath.Join(modules, "tailwindcss", "index.css"), htmlPath, inlineGalleryFonts)
	if err := os.WriteFile(input, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tw, "-i", input, "-o", filepath.Join(dir, "gallery.css")).CombinedOutput(); err != nil {
		t.Fatalf("tailwindcss: %v\n%s", err, out)
	}
	w, h := fieldGallerySize()
	geom := map[string][][4]float64{}
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
		var rects [][4]float64
		if err := json.Unmarshal([]byte(html.UnescapeString(s[i+5:j])), &rects); err != nil {
			t.Fatal(err)
		}
		geom[fmt.Sprintf("x%d", scale)] = rects
		shot, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command("chromium", "--headless", "--disable-gpu", "--hide-scrollbars", "--allow-file-access-from-files",
			"--virtual-time-budget=2000", "--force-color-profile=srgb", fmt.Sprintf("--force-device-scale-factor=%d", scale),
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

// renderFieldGallery renders the gallery at scale and returns the image and
// the border box of every measured element in document order.
func renderFieldGallery(t *testing.T, scale float32) (*image.RGBA, []gfx.Rect) {
	t.Helper()
	var logs []string
	cfg := Config{Fonts: testFonts, RootClass: "bg-white overflow-hidden", IDPrefix: "r-", ClassicScrollbars: true,
		Log: func(m string) { logs = append(logs, m) }}
	w, h := fieldGallerySize()
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.SetScale(scale)
	id := 0
	tr.Mount(fieldGalleryRoot().node(&id))
	tr.Frame()
	for k, s := range fieldGallerySelections() {
		el := keyed(tr, k)
		tr.focusElement(el)
		tr.scr.reveal = nil // as focus({preventScroll: true})
		f := el.box.fld
		f.ed.Select(s[0], s[1])
		f.markPaint()
	}
	dl, _ := tr.Frame()
	img := tr.Rasterize(dl, scale)
	for _, m := range logs {
		t.Errorf("dev message: %s", m)
	}
	rects := make([]gfx.Rect, id)
	for k := range id {
		el := keyed(tr, k)
		if el == nil {
			t.Fatalf("no element %d", k)
		}
		rects[k] = tr.boxWindowRect(el.box)
	}
	tr.Unmount()
	return img, rects
}

// fieldGalleryCase returns the case of measured element k.
func fieldGalleryCaseOf(k int) string {
	id := 0
	for _, c := range fieldGalleryCases {
		n := 0
		var walk func(f fnode)
		walk = func(f fnode) {
			if f.measure {
				n++
			}
			for _, x := range f.kids {
				walk(x)
			}
		}
		walk(c.root)
		if k < id+n {
			return c.name
		}
		id += n
	}
	return "?"
}

// fieldGalleryMean bounds the mean pixel difference of a cell (glyph
// anti-aliasing, and at scale 2 Blink's line metrics in device pixels,
// ADR-048).
var fieldGalleryMean = [2]float64{2, 7}

func TestChromiumFieldGallery(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "fields")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateFieldGallery(t, dir)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(h) != fieldGalleryHTML() {
		t.Fatal("testdata/chromium/fields/gallery.html is out of date: the gallery changed; regenerate with TELA_CHROMIUM=1")
	}
	data, err := os.ReadFile(filepath.Join(dir, "geometry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var geom map[string][][4]float64
	if err := json.Unmarshal(data, &geom); err != nil {
		t.Fatal(err)
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	for _, scale := range []int{1, 2} {
		img, rects := renderFieldGallery(t, float32(scale))
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("fields_x%d_tela.png", scale)), img)
		}
		want := geom[fmt.Sprintf("x%d", scale)]
		if len(want) != len(rects) {
			t.Fatalf("x%d: %d elements here, %d in Chromium", scale, len(rects), len(want))
		}
		worst, worstAt := 0.0, ""
		for k, r := range rects {
			g := [4]float64{float64(r.X0), float64(r.Y0), float64(r.Width()), float64(r.Height())}
			w := want[k]
			d := 0.0
			for c := range 2 {
				d = math.Max(d, math.Max(math.Abs(g[c]-w[c]), math.Abs(g[c]+g[c+2]-w[c]-w[c+2])))
			}
			if d > worst {
				worst, worstAt = d, fmt.Sprintf("element %d (%s): %v, Chromium %v", k, fieldGalleryCaseOf(k), g, w)
			}
			if d > fieldGeometryTolerance(scale) {
				t.Errorf("x%d element %d (%s): %.3f %.3f %.3f %.3f, Chromium %.3f %.3f %.3f %.3f", scale, k, fieldGalleryCaseOf(k),
					g[0], g[1], g[2], g[3], w[0], w[1], w[2], w[3])
			}
		}
		fmt.Fprintf(&report, "x%d: largest edge difference %.3f px (%s)\n", scale, worst, worstAt)
		shot := readRGBA(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if shot.Rect != img.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, img.Rect, shot.Rect)
		}
		y := 0
		for _, c := range fieldGalleryCases {
			d := diffRegion(img, shot, image.Rect(0, y*scale, fieldGalleryWidth*scale, (y+c.height)*scale))
			fmt.Fprintf(&report, "  %-20s max %3d mean %.3f >16 %d\n", c.name, d.max, d.mean, d.over)
			if d.mean > fieldGalleryMean[scale-1] {
				t.Errorf("x%d %s: pixels differ from Chromium by max %d mean %.3f", scale, c.name, d.max, d.mean)
			}
			y += c.height
		}
	}
	t.Log("difference from Chromium:\n" + report.String())
}

// fieldGeometryTolerance is the largest edge difference accepted: at scale
// 2 Blink rounds line metrics in device pixels (ADR-048).
func fieldGeometryTolerance(scale int) float64 {
	if scale == 2 {
		return 1.01
	}
	return 0.1
}

func TestFieldGallery(t *testing.T) {
	for _, scale := range []int{1, 2} {
		img, _ := renderFieldGallery(t, float32(scale))
		golden.Image(t, fmt.Sprintf("field_gallery_x%d", scale), img)
	}
}
