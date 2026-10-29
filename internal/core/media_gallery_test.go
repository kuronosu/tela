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

// The media gallery (ADR-098) compares Img sources (SVG and raster, with
// object-fit and object-position), tinted images, icons (inline <svg> in
// the page) and CSS masks (mask-[url()] and Tailwind v4's gradient
// masks) with Chromium, cell by cell, at scales 1 and 2, like the effects
// gallery (cssfx_gallery_test.go). Its files are in
// testdata/chromium/media, with the images in img/ (photo.png and
// tile.png from the effects gallery; glyph.png, a 64 px ring crossed by
// a diagonal bar whose alpha is a 4x4 supersampled coverage, over a color
// ramp; and hand-written SVG files), which the test
// registers as assets. A tinted Img is, in the page, a div with the tint
// as background and the image as mask-image; an Icon is inline SVG markup
// whose width="1em" height="1em" attributes model the Icon's default
// size. Regenerate Chromium's files with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumMedia ./internal/core/

// Icons of the gallery, in the shape of Lucide's (24 x 24, stroked with
// currentColor), and a star whose root has no paint of its own, so that
// inherited fill and stroke reach it in the page as well.
const (
	galleryCheck = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></svg>`
	galleryHeart = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/></svg>`
	galleryStar  = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" stroke-linejoin="round"><path d="M12 2l2.9 6.6 7.1.6-5.4 4.7 1.6 7L12 17.3 5.8 20.9l1.6-7L2 9.2l7.1-.6z"/></svg>`
)

func imgc(name, class, src string) fxCell {
	return fxCell{name, inode{tag: "img", class: class, src: src}}
}

func tintc(name, class, src, tint string) fxCell {
	return fxCell{name, inode{tag: "img", class: class, src: src, tint: tint}}
}

// isvg is an Icon of the gallery.
func isvg(markup, class string) inode {
	return inode{tag: "svg", class: class, svg: markup, attrs: `width="1em" height="1em"`}
}

const photo = "img/photo.png"

var mediaRows = []fxRow{
	{"img-svg", 88, []fxCell{
		imgc("svg-natural", "", "img/badge.svg"),
		imgc("svg-h-only", "h-16", "img/badge.svg"),
		imgc("svg-fill", "w-20 h-16 bg-slate-100", "img/badge.svg"),
		imgc("svg-contain", "w-20 h-16 bg-slate-100 object-contain object-bottom", "img/badge.svg"),
		imgc("svg-cover", "w-20 h-16 object-cover", "img/badge.svg"),
		imgc("svg-none", "w-12 h-16 bg-slate-100 object-none object-left", "img/badge.svg"),
		imgc("svg-scale-down", "w-24 h-16 bg-slate-100 object-scale-down", "img/badge.svg"),
		imgc("svg-viewbox", "w-16", "img/square.svg"),
		imgc("svg-stretch", "w-24 h-12", "img/stretch.svg"),
		imgc("svg-rounded", "size-16 rounded-full object-cover", "img/square.svg"),
	}},
	{"img-raster", 88, []fxCell{
		imgc("png-cover", "w-28 h-16 object-cover", photo),
		imgc("png-contain", "w-28 h-16 bg-slate-100 object-contain object-bottom", photo),
		imgc("png-down", "w-24", photo),
		imgc("png-none", "w-20 h-16 object-none object-[25%_75%]", photo),
		imgc("png-up", "size-16", "img/tile.png"),
		imgc("png-scale-down", "w-28 h-16 bg-slate-100 object-scale-down", "img/tile.png"),
		imgc("png-rounded", "size-16 rounded-xl object-cover", photo),
	}},
	{"tint", 72, []fxCell{
		tintc("tint-png", "size-12", "img/glyph.png", "#d97757"),
		tintc("tint-png-small", "size-6", "img/glyph.png", "#4f46e5"),
		tintc("tint-svg", "size-12", "img/star.svg", "#2563eb"),
		tintc("tint-photo", "w-18 h-12", photo, "#059669"),
	}},
	{"icons", 72, []fxCell{
		fxc("icon-text", "text-slate-700", iel("p", "", itext("Saved "), isvg(galleryCheck, "inline"), itext(" now"))),
		fxc("icon-text-2xl", "text-2xl text-emerald-600", iel("p", "", isvg(galleryCheck, "inline"), itext(" Done"))),
		fxc("icon-button", "", iel("button", "inline-flex items-center gap-2 rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white",
			isvg(galleryCheck, "size-4"), itext("Save"))),
		fxc("icon-stroke", "", iel("button", "inline-flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm text-red-600",
			isvg(galleryHeart, "size-5 stroke-[1.5]"), itext("Like"))),
		fxc("icon-inherit", "fill-current stroke-sky-700 stroke-[1.5] text-amber-400", isvg(galleryStar, "size-10")),
		fxc("icon-own", "", isvg(galleryHeart, "size-10 text-rose-500 fill-rose-200 stroke-[2.5]")),
		fxc("icon-fill-none", "", isvg(galleryCheck, "size-10 text-indigo-600 fill-indigo-100")),
	}},
	{"mask-image", 88, []fxCell{
		fxc("mask-png", "size-16 bg-[#d97757] mask-[url(img/glyph.png)] mask-contain mask-no-repeat mask-center"),
		fxc("mask-svg-tiles", "size-16 bg-linear-to-br from-sky-400 to-violet-600 mask-[url(img/star.svg)] mask-size-[24px_24px]"),
		fxc("mask-svg-right", "w-24 h-16 bg-emerald-500 mask-[url(img/star.svg)] mask-contain mask-no-repeat mask-right"),
		fxc("mask-luminance", "size-16 bg-indigo-500 mask-[url(img/photo.png)] mask-cover mask-luminance"),
		fxc("mask-space", "w-24 h-16 bg-[url(img/photo.png)] bg-cover mask-[url(img/glyph.png)] mask-repeat-space mask-size-[20px_20px]"),
		fxc("mask-round", "size-16 bg-amber-400 mask-[url(img/star.svg)] mask-repeat-round mask-size-[20px_20px]"),
		fxc("mask-origin", "size-16 border-4 border-sky-700 bg-sky-400 p-2 mask-[url(img/glyph.png)] mask-origin-content mask-no-repeat mask-contain"),
	}},
	{"mask-linear", 88, []fxCell{
		fxc("mask-t", "w-24 h-16 bg-indigo-500 mask-t-from-50%"),
		fxc("mask-b-to", "w-24 h-16 bg-indigo-500 mask-b-to-80%"),
		fxc("mask-x", "w-24 h-16 bg-indigo-500 mask-x-from-70% mask-x-to-90%"),
		fxc("mask-y-spacing", "w-24 h-16 bg-indigo-500 mask-y-from-4"),
		fxc("mask-two-edges", "w-24 h-16 bg-indigo-500 mask-l-from-[30px] mask-t-from-60%"),
		fxc("mask-linear", "w-24 h-16 bg-[url(img/photo.png)] bg-cover mask-linear-from-20% mask-linear-to-80%"),
		fxc("mask-linear-135", "w-24 h-16 bg-indigo-500 mask-linear-135 mask-linear-from-30% mask-linear-to-90%"),
		fxc("mask-r-color", "w-24 h-16 bg-indigo-500 mask-r-from-black/40 mask-r-to-80%"),
	}},
	{"mask-radial", 88, []fxCell{
		fxc("radial", "size-16 bg-rose-500 mask-radial-from-40% mask-radial-to-70%"),
		fxc("radial-circle", "w-24 h-16 bg-rose-500 mask-radial-from-0% mask-circle mask-radial-closest-side"),
		fxc("radial-at", "w-24 h-16 bg-rose-500 mask-radial-from-20% mask-radial-at-top-left"),
		fxc("radial-size", "w-24 h-16 bg-rose-500 mask-radial-from-50% mask-radial-[40%_80%]"),
		fxc("radial-photo", "w-24 h-16 bg-[url(img/photo.png)] bg-cover mask-radial-from-60% mask-radial-to-61% mask-circle mask-radial-farthest-side"),
	}},
	{"mask-conic", 88, []fxCell{
		fxc("conic", "size-16 bg-teal-500 mask-conic-from-75%"),
		fxc("conic-90", "size-16 bg-teal-500 mask-conic-90 mask-conic-from-25% mask-conic-to-60%"),
		fxc("conic-deg", "size-16 bg-teal-500 mask-conic-from-[90deg] mask-conic-to-[270deg]"),
	}},
	{"mask-composite", 88, []fxCell{
		fxc("intersect", "w-24 h-16 bg-violet-500 mask-t-from-50% mask-radial-from-40%"),
		fxc("add", "size-16 bg-violet-500 mask-[url(img/star.svg),radial-gradient(black_40%,transparent_41%)] mask-add"),
		fxc("subtract", "size-16 bg-violet-500 mask-[url(img/star.svg),radial-gradient(black_40%,transparent_41%)] mask-subtract"),
		fxc("intersect-list", "size-16 bg-violet-500 mask-[url(img/star.svg),radial-gradient(black_40%,transparent_41%)] mask-intersect"),
		fxc("exclude", "size-16 bg-violet-500 mask-[url(img/star.svg),radial-gradient(black_40%,transparent_41%)] mask-exclude"),
		fxc("clip-children", "relative size-16 rounded-2xl bg-sky-200 mask-[linear-gradient(black,black)]",
			iel("div", "absolute -right-4 top-4 size-8 bg-red-500")),
		fxc("with-opacity", "size-16 bg-violet-500 opacity-60 mask-radial-from-30% mask-radial-to-70%"),
	}},
}

// mediaBounds holds the largest max and mean channel differences measured
// per cell (x1 max, x1 mean, x2 max, x2 mean), rounded up, for the cells
// above mediaDefaultBound, with the reason.
var mediaBounds = map[string][4]float64{
	// Downscaled photos: Chromium's image filter against the mip level
	// and bilinear sampling (ADR-079); a rounded clip's anti-aliasing.
	"png-cover":   {65, 1.15, 65, 0.7},
	"png-contain": {5, 0.8, 5, 0.5},
	"png-down":    {5, 0.9, 5, 0.55},
	"png-rounded": {35, 0.9, 30, 0.55},
	"svg-rounded": {65, 0.6, 55, 0.2},
	// Text: glyph anti-aliasing (Skia's), and at scale 2 the line half a
	// device pixel apart (Blink rounds line metrics, ADR-048).
	"icon-text":     {80, 2.1, 210, 8.4},
	"icon-text-2xl": {145, 1.7, 255, 4.7},
	"icon-button":   {160, 2.1, 175, 1.3},
	"icon-stroke":   {210, 1.1, 185, 0.6},
	"icon-inherit":  {50, 0.65, 40, 0.3},
	// A 64 px mask image tiled at 20 px: Chromium's downscaling filter
	// against the mip level, on every edge of the glyph.
	"mask-space": {145, 4.3, 125, 2.1},
	"mask-round": {100, 2.1, 90, 1.05},
}

// mediaDefaultBound applies to the cells not in mediaBounds.
var mediaDefaultBound = [4]float64{80, 0.5, 80, 0.5}

func mediaGallery() *cssGallery {
	// Box edges: within 0.05 px at scale 1 (text widths before the cells
	// of a row); at scale 2 inline icons sit up to half a device pixel
	// lower or higher, where Blink rounds the font metrics of the line
	// (ADR-066, ADR-048).
	g := &cssGallery{name: "media", width: 1000, fontCSS: fxFontCSS,
		mean: [2]float64{100, 100}, geom: [2]float64{0.05, 0.55}}
	for _, r := range mediaRows {
		row := cssRow{name: r.name, height: r.height, class: "gap-6"}
		for _, c := range r.cells {
			row.kids = append(row.kids, c.n)
		}
		g.rows = append(g.rows, row)
	}
	return g
}

func registerMediaAssets(t *testing.T) {
	resetAssets()
	RegisterAssets(os.DirFS(filepath.Join("testdata", "chromium", "media")))
	t.Cleanup(resetAssets)
}

func TestChromiumMedia(t *testing.T) {
	g := mediaGallery()
	registerMediaAssets(t)
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
	var cellIDs []int
	var names []string
	id := 1
	for _, r := range mediaRows {
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
	worst := [2]float64{}
	for si, scale := range []int{1, 2} {
		img, rects := g.render(t, float32(scale))
		if dump != "" {
			golden.WritePNG(t, filepath.Join(dump, fmt.Sprintf("media_x%d_tela.png", scale)), img)
		}
		want := geom[fmt.Sprintf("x%d", scale)]
		if len(want) != len(rects) {
			t.Fatalf("x%d: %d elements here, %d in Chromium", scale, len(rects), len(want))
		}
		// Every element's box (the inline icons have one rect each).
		for k := range rects {
			if len(rects[k]) != 1 || len(want[k]) != 1 {
				if len(want[k]) == len(rects[k]) {
					continue // the text elements of a paragraph: compared by pixels
				}
				t.Errorf("x%d element %d: rects %v, Chromium %v", scale, k, rects[k], want[k])
				continue
			}
			r, w := rects[k][0], want[k][0]
			d := math.Max(math.Max(math.Abs(float64(r.X0)-w[0]), math.Abs(float64(r.Y0)-w[1])),
				math.Max(math.Abs(float64(r.Width())-w[2]), math.Abs(float64(r.Height())-w[3])))
			worst[si] = math.Max(worst[si], d)
			if d > g.geom[si] {
				t.Errorf("x%d element %d (%s): %v, Chromium %v", scale, k, g.rowOf(k), r, w)
			}
		}
		shot := golden.ReadRGBA(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if shot.Rect != img.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, img.Rect, shot.Rect)
		}
		for i, k := range cellIDs {
			r := rects[k][0].Outset(6)
			cell := image.Rect(int(math.Floor(float64(r.X0)))*scale, int(math.Floor(float64(r.Y0)))*scale,
				int(math.Ceil(float64(r.X1)))*scale, int(math.Ceil(float64(r.Y1)))*scale).Intersect(img.Rect)
			diffs[i][si] = diffRegion(img, shot, cell)
		}
	}
	fmt.Fprintf(&report, "largest box edge difference: x1 %.3f px, x2 %.3f px\n", worst[0], worst[1])
	for i, name := range names {
		b, ok := mediaBounds[name]
		if !ok {
			b = mediaDefaultBound
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

func TestMediaGolden(t *testing.T) {
	registerMediaAssets(t)
	g := mediaGallery()
	for _, scale := range []int{1, 2} {
		img, _ := g.render(t, float32(scale))
		golden.Image(t, fmt.Sprintf("media_x%d", scale), img)
	}
}
