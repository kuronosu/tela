package core

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The effects gallery (ADR-079) compares mix-blend-mode, clip-path,
// background images and background-clip: text with Chromium, cell by
// cell, at scales 1 and 2. It is a cssGallery (cssgallery_test.go): the
// page, Tailwind v4's CSS for it, Chromium's geometry and screenshots are
// in testdata/chromium/effects, with the synthetic images it uses in
// testdata/chromium/effects/img (generated, see the ADR), which the test
// registers as assets. Regenerate Chromium's files with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumEffects ./internal/core/
//
// Each cell is the border box of a top-level element of a row grown by 10
// px (shadows, outlines, rotations); fxBounds holds the largest mean and
// max channel differences measured per cell, rounded up.

// fxCell is one cell of the gallery.
type fxCell struct {
	name string
	n    inode
}

type fxRow struct {
	name   string
	height int
	cells  []fxCell
}

func fxc(name, class string, kids ...inode) fxCell {
	return fxCell{name, iel("div", class, kids...)}
}

// blendCell is a gradient square with a circle blended over it; isolate
// adds isolation: isolate to the square, and the circle overflows it on
// the right, over the row's background.
func blendCell(name, mode, extra string) fxCell {
	return fxc(name, "relative size-20 shrink-0 "+extra,
		iel("div", "absolute inset-y-0 left-0 w-14 bg-linear-to-br from-sky-400 to-fuchsia-600"),
		iel("div", "absolute top-3 left-5 size-14 rounded-full bg-amber-400 "+mode))
}

var fxRows = []fxRow{
	{"blend", 104, []fxCell{
		blendCell("multiply", "mix-blend-multiply", ""),
		blendCell("screen", "mix-blend-screen", ""),
		blendCell("overlay", "mix-blend-overlay", ""),
		blendCell("difference", "mix-blend-difference", ""),
		blendCell("color-dodge", "mix-blend-color-dodge", ""),
		blendCell("soft-light", "mix-blend-soft-light", ""),
		blendCell("hue", "mix-blend-hue", ""),
		blendCell("luminosity", "mix-blend-luminosity", ""),
	}},
	{"blend-isolate", 104, []fxCell{
		blendCell("iso-difference", "mix-blend-difference", "isolate"),
		blendCell("iso-multiply", "mix-blend-multiply", "isolate"),
		blendCell("z-multiply", "mix-blend-multiply", "z-10"),
		blendCell("opacity-screen", "mix-blend-screen", "opacity-90"),
		fxc("over-photo", "relative w-28 h-20 bg-[url(img/photo.png)] bg-cover bg-center",
			iel("div", "absolute inset-x-3 top-4 h-12 rounded-xl bg-fuchsia-500 mix-blend-multiply")),
		fxc("over-photo-screen", "relative isolate w-28 h-20 bg-[url(img/photo.png)] bg-cover bg-center",
			iel("div", "absolute inset-x-3 top-4 h-12 rounded-xl bg-sky-400 mix-blend-screen")),
		fxc("text-difference", "relative w-28 h-20 bg-linear-to-r from-black to-white",
			iel("p", "absolute inset-0 pt-5 text-center text-2xl font-black text-white mix-blend-difference", itext("Blend"))),
	}},
	{"clip", 112, []fxCell{
		fxc("circle", "size-24 shrink-0 bg-indigo-500 [clip-path:circle(50%)]"),
		fxc("circle-at", "size-24 shrink-0 bg-rose-500 [clip-path:circle(40%_at_30%_35%)]"),
		fxc("diamond", "size-24 shrink-0 bg-linear-to-r from-rose-500 to-orange-400 [clip-path:polygon(50%_0,100%_50%,50%_100%,0_50%)]"),
		fxc("slant", "w-28 h-24 shrink-0 bg-emerald-500 [clip-path:polygon(0_0,100%_0,100%_80%,0_100%)]"),
		fxc("inset-round", "size-24 shrink-0 bg-sky-500 [clip-path:inset(8px_round_1rem)]"),
		fxc("ellipse", "w-28 h-20 shrink-0 bg-amber-400 [clip-path:ellipse(50%_35%_at_50%_50%)]"),
		fxc("star-nonzero", "size-24 shrink-0 bg-violet-500 [clip-path:polygon(50%_0,79%_91%,2%_35%,98%_35%,21%_91%)]"),
		fxc("star-evenodd", "size-24 shrink-0 bg-violet-500 [clip-path:polygon(evenodd,50%_0,79%_91%,2%_35%,98%_35%,21%_91%)]"),
	}},
	{"clip-fx", 128, []fxCell{
		fxc("rotated-shadow", "size-24 shrink-0 rotate-12 rounded-xl bg-violet-500 shadow-xl [clip-path:inset(4px_round_12px)]"),
		fxc("shadow-outline", "size-20 shrink-0 rounded-lg bg-white shadow-lg outline-4 outline-offset-2 outline-rose-500 [clip-path:circle(60%)]"),
		fxc("scaled-poly", "size-24 shrink-0 scale-90 -rotate-6 bg-teal-500 [clip-path:polygon(0_0,100%_10%,90%_100%,10%_90%)]"),
		fxc("overflow-child", "relative size-24 shrink-0 bg-slate-800 [clip-path:circle(45%)]",
			iel("div", "absolute -right-6 top-6 h-12 w-20 bg-amber-400")),
		fxc("opacity", "size-24 shrink-0 bg-indigo-600 opacity-60 [clip-path:polygon(0_20%,60%_20%,60%_0,100%_50%,60%_100%,60%_80%,0_80%)]"),
		fxc("photo-circle", "w-32 h-24 shrink-0 bg-[url(img/photo.png)] bg-cover bg-center [clip-path:circle(45%_at_40%_50%)]"),
	}},
	{"bg-size", 112, []fxCell{
		fxc("cover", "w-32 h-24 shrink-0 bg-[url(img/photo.png)] bg-cover bg-center"),
		fxc("contain", "w-32 h-24 shrink-0 bg-slate-200 bg-[url(img/photo.png)] bg-contain bg-center bg-no-repeat"),
		fxc("contain-tall", "w-16 h-24 shrink-0 bg-slate-200 bg-[url(img/photo.png)] bg-contain bg-bottom bg-no-repeat"),
		fxc("length", "w-32 h-24 shrink-0 bg-[url(img/photo.png)] bg-[length:96px_auto] bg-[position:25%_75%] bg-no-repeat bg-slate-200"),
		fxc("auto-right", "w-32 h-24 shrink-0 bg-[url(img/tile.png)] bg-bottom-right bg-no-repeat bg-slate-100"),
		fxc("percent", "w-32 h-24 shrink-0 bg-[url(img/photo.png)] bg-[length:50%_50%] bg-slate-200"),
	}},
	{"bg-repeat", 104, []fxCell{
		fxc("repeat", "w-30 h-22 shrink-0 bg-[url(img/tile.png)] bg-[position:10px_6px] bg-amber-50"),
		fxc("repeat-x", "w-30 h-22 shrink-0 bg-[url(img/tile.png)] bg-repeat-x bg-center bg-amber-50"),
		fxc("repeat-y", "w-30 h-22 shrink-0 bg-[url(img/tile.png)] bg-repeat-y bg-center bg-amber-50"),
		fxc("round", "w-30 h-22 shrink-0 bg-[url(img/tile.png)] bg-repeat-round bg-amber-50"),
		fxc("space", "w-30 h-22 shrink-0 bg-[url(img/tile.png)] bg-repeat-space bg-amber-50"),
		fxc("no-repeat", "w-30 h-22 shrink-0 bg-[url(img/tile.png)] bg-no-repeat bg-center bg-amber-50"),
	}},
	{"bg-box", 112, []fxCell{
		fxc("origin-border", "w-32 h-24 shrink-0 border-8 border-dashed border-slate-500/50 p-3 bg-[url(img/tile.png)] bg-origin-border bg-clip-border"),
		fxc("origin-padding", "w-32 h-24 shrink-0 border-8 border-dashed border-slate-500/50 p-3 bg-[url(img/tile.png)] bg-origin-padding bg-clip-padding"),
		fxc("origin-content", "w-32 h-24 shrink-0 border-8 border-dashed border-slate-500/50 p-3 bg-[url(img/tile.png)] bg-origin-content bg-clip-content"),
		fxc("rounded-photo", "w-32 h-24 shrink-0 rounded-2xl border-4 border-white bg-[url(img/photo.png)] bg-cover bg-center shadow-lg"),
		fxc("no-repeat-content", "w-32 h-24 shrink-0 border-4 border-sky-200 p-4 bg-sky-50 bg-[url(img/tile.png)] bg-no-repeat bg-origin-content"),
	}},
	{"bg-layers", 112, []fxCell{
		fxc("gradient-over-photo", "w-32 h-24 shrink-0 bg-[linear-gradient(to_bottom,transparent,rgb(0_0_0/0.7)),url(img/photo.png)] bg-cover bg-center"),
		fxc("radial-over-tile", "w-32 h-24 shrink-0 bg-[radial-gradient(circle_at_30%_30%,rgb(255_255_255/0.9),transparent_60%),url(img/tile.png)] bg-indigo-200"),
		fxc("blend-multiply", "w-32 h-24 shrink-0 bg-sky-400 bg-[url(img/photo.png)] bg-cover bg-center bg-blend-multiply"),
		fxc("blend-luminosity", "w-32 h-24 shrink-0 bg-fuchsia-600 bg-[url(img/photo.png)] bg-cover bg-center bg-blend-luminosity"),
		fxc("jpeg", "w-32 h-24 shrink-0 bg-[url(img/photo.jpg)] bg-cover bg-center"),
		fxc("webp", "w-24 h-24 shrink-0 bg-[url(img/tile.webp)] bg-slate-100"),
	}},
	{"downscale", 96, []fxCell{
		fxc("zone-64", "size-16 shrink-0 bg-[url(img/zone.png)] bg-cover"),
		fxc("zone-40", "size-10 shrink-0 bg-[url(img/zone.png)] bg-cover"),
		fxc("zone-88", "size-22 shrink-0 bg-[url(img/zone.png)] bg-cover"),
		fxc("photo-48", "w-18 h-12 shrink-0 bg-[url(img/photo.png)] bg-cover"),
		fxc("photo-120", "w-30 h-20 shrink-0 bg-[url(img/photo.png)] bg-cover"),
	}},
	{"clip-text", 132, []fxCell{
		fxc("gradient-text", "shrink-0 bg-linear-to-r from-indigo-500 via-fuchsia-500 to-amber-400 bg-clip-text text-5xl font-extrabold text-transparent",
			itext("Gradient")),
		fxc("photo-text", "shrink-0 bg-[url(img/photo.png)] bg-cover bg-center bg-clip-text text-5xl font-black text-transparent",
			itext("Photo")),
		fxc("multi-line", "w-36 shrink-0 bg-linear-to-b from-sky-500 to-emerald-600 bg-clip-text text-xl leading-tight font-bold text-transparent",
			itext("Gradient text that wraps over three lines")),
		fxc("inline-span", "w-44 shrink-0 text-lg font-semibold text-slate-700",
			itext("Plain and "),
			iel("span", "bg-linear-to-r from-rose-500 to-violet-600 bg-clip-text text-transparent", itext("clipped words")),
			itext(" mixed.")),
		fxc("emoji", "shrink-0 bg-linear-to-r from-emerald-500 to-sky-500 bg-clip-text text-4xl font-bold text-transparent",
			itext("Hi 🎉")),
	}},
}

// fxBounds is the largest difference from Chromium accepted per cell (x1
// max, x1 mean, x2 max, x2 mean) where the default is too tight: the
// measured values (Chromium 152, Tailwind 4.3.3) × 1.1 + 3 and + 0.1,
// rounded up. Why they differ:
//
//   - text (text-difference, the bg-clip-text cells): glyph edges, Skia's
//     anti-aliasing against exact area coverage and Blink's baselines in
//     device pixels at x2 (ADR-048), as in internal/text's
//     TestChromiumClipText; the emoji's outlines differ a little too;
//   - repeated small images at x2 (repeat, round, origin-*, webp,
//     radial-over-tile): Chromium's repeating image shader interpolates
//     across tile seams (the last row of a tile with the first of the
//     next); Tela draws each tile on its own, clamped at its edges;
//   - downscaled and scaled photos (zone-*, photo-*, percent, jpeg,
//     contain, rounded-photo, over-photo): mip levels and bilinear
//     rounding; JPEG chroma upsampling (Go's image/jpeg is not libjpeg's
//     fancy upsampling);
//   - star-evenodd, overflow-child: single anti-aliased pixels on sharp
//     polygon tips and circle edges.
var fxBounds = map[string][4]float64{
	"over-photo":       {17, 0.65, 19, 0.45},
	"text-difference":  {123, 0.70, 229, 1.00},
	"star-evenodd":     {101, 0.30, 53, 0.15},
	"overflow-child":   {101, 0.65, 68, 0.30},
	"contain":          {18, 0.55, 17, 0.65},
	"percent":          {9, 0.90, 11, 0.55},
	"repeat":           {5, 0.15, 86, 1.40},
	"round":            {17, 0.40, 69, 1.15},
	"origin-border":    {14, 0.25, 86, 1.40},
	"origin-padding":   {14, 0.25, 86, 1.25},
	"origin-content":   {14, 0.25, 86, 0.80},
	"rounded-photo":    {29, 0.70, 32, 0.60},
	"radial-over-tile": {6, 0.25, 66, 0.90},
	"jpeg":             {67, 1.05, 59, 1.00},
	"webp":             {5, 0.20, 82, 1.30},
	"zone-64":          {6, 0.70, 5, 0.40},
	"zone-40":          {11, 0.80, 14, 0.70},
	"zone-88":          {14, 1.00, 22, 1.50},
	"photo-48":         {14, 0.75, 20, 0.50},
	"photo-120":        {6, 0.70, 5, 0.40},
	"gradient-text":    {214, 1.80, 255, 1.65},
	"photo-text":       {167, 2.30, 255, 1.75},
	"multi-line":       {161, 1.60, 255, 2.40},
	"inline-span":      {131, 1.30, 230, 2.20},
	"emoji":            {226, 3.75, 255, 1.20},
}

// fxDefaultBound applies to the cells not in fxBounds.
var fxDefaultBound = [4]float64{80, 0.5, 80, 0.5}

var fxFontCSS = `@theme { --font-sans: "Inter", "Noto Color Emoji"; }
` + cssGalleryFonts + `@font-face { font-family: "Noto Color Emoji"; src: url("../../../../assets/fonts/notocoloremoji/Noto-COLRv1.ttf"); }
`

func fxGallery() *cssGallery {
	g := &cssGallery{name: "effects", width: 1000, fontCSS: fxFontCSS,
		mean: [2]float64{100, 100}, geom: [2]float64{0.02, 0.02}}
	for _, r := range fxRows {
		row := cssRow{name: r.name, height: r.height, class: "gap-6"}
		for _, c := range r.cells {
			row.kids = append(row.kids, c.n)
		}
		if r.name == "blend-isolate" {
			row.class += " bg-emerald-200"
		}
		g.rows = append(g.rows, row)
	}
	return g
}

// registerFxAssets registers the gallery's images for the test.
func registerFxAssets(t *testing.T) {
	resetAssets()
	RegisterAssets(os.DirFS(filepath.Join("testdata", "chromium", "effects")))
	t.Cleanup(resetAssets)
}

func TestChromiumEffects(t *testing.T) {
	g := fxGallery()
	registerFxAssets(t)
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
	// The ids of the cells: the root is 0, each row one, then its
	// elements in document order.
	var cellIDs []int
	var names []string
	id := 1
	for _, r := range fxRows {
		id++ // the row
		for _, c := range r.cells {
			cellIDs = append(cellIDs, id)
			names = append(names, c.name)
			id += countElements(c.n)
		}
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	report.WriteString("cell                    x1 max   mean | x2 max   mean\n")
	diffs := make([][2]boxDiff, len(names))
	for si, scale := range []int{1, 2} {
		img, rects := g.render(t, float32(scale))
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("effects_x%d_tela.png", scale)), img)
		}
		want := geom[fmt.Sprintf("x%d", scale)]
		if len(want) != len(rects) {
			t.Fatalf("x%d: %d elements here, %d in Chromium", scale, len(rects), len(want))
		}
		for _, k := range cellIDs {
			if len(rects[k]) != 1 || len(want[k]) != 1 {
				t.Fatalf("x%d element %d: rects %v, Chromium %v", scale, k, rects[k], want[k])
			}
			r, w := rects[k][0], want[k][0]
			d := math.Max(math.Max(math.Abs(float64(r.X0)-w[0]), math.Abs(float64(r.Y0)-w[1])),
				math.Max(math.Abs(float64(r.Width())-w[2]), math.Abs(float64(r.Height())-w[3])))
			// Chromium's client rects include transforms; compare the
			// untransformed cells only.
			if d > g.geom[si] && !strings.Contains(fxClass(k), "rotate") && !strings.Contains(fxClass(k), "scale") {
				t.Errorf("x%d element %d: %v, Chromium %v", scale, k, r, w)
			}
		}
		shot := readRGBA(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if shot.Rect != img.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, img.Rect, shot.Rect)
		}
		for i, k := range cellIDs {
			r := rects[k][0].Outset(10)
			cell := image.Rect(int(math.Floor(float64(r.X0)))*scale, int(math.Floor(float64(r.Y0)))*scale,
				int(math.Ceil(float64(r.X1)))*scale, int(math.Ceil(float64(r.Y1)))*scale).Intersect(img.Rect)
			diffs[i][si] = diffRegion(img, shot, cell)
		}
	}
	for i, name := range names {
		b, ok := fxBounds[name]
		if !ok {
			b = fxDefaultBound
		}
		d1, d2 := diffs[i][0], diffs[i][1]
		fmt.Fprintf(&report, "%-22s %6d %6.3f | %6d %6.3f\n", name, d1.max, d1.mean, d2.max, d2.mean)
		for si, d := range diffs[i] {
			if float64(d.max) > b[2*si] || d.mean > b[2*si+1] {
				t.Errorf("x%d %s: max %d mean %.3f, bound %g %.2f", si+1, name, d.max, d.mean, b[2*si], b[2*si+1])
			}
		}
	}
	t.Log("difference from Chromium per cell (channel levels):\n" + report.String())
}

// fxClass returns the class of element k of the gallery (document
// order).
func fxClass(k int) string {
	var classes []string
	var walk func(n inode)
	walk = func(n inode) {
		if n.tag == "" {
			return
		}
		classes = append(classes, n.class)
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(fxGallery().root())
	return classes[k]
}

func TestEffectsGolden(t *testing.T) {
	registerFxAssets(t)
	g := fxGallery()
	for _, scale := range []int{1, 2} {
		img, _ := g.render(t, float32(scale))
		golden.Image(t, fmt.Sprintf("effects_x%d", scale), img)
	}
}
