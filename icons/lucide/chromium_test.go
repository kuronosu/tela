package lucide

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The Chromium oracle for an icon set (ADR-102, phase 8's done criterion:
// "an SVG icon set renders within a few levels of Chromium at scales 1 and
// 2"). Every distinct Lucide icon (aliases have the same drawing and are
// compared once) is shown in a 32 px cell of a grid, centered at 16 and at
// 24 px, in one color, through h.Icon and telatest; headless Chromium shows
// the same SVG markup inline with the same CSS:
//
//	.g  { display: flex; flex-wrap: wrap; width: 2048px }
//	.c  { flex: none; width: 32px; height: 32px; display: flex; align-items: center; justify-content: center }
//	svg { display: block; width: 16px (or 24px); height: ...; color: #1e293b }
//
// With TELA_CHROMIUM=1 the test rebuilds the pages in a temporary
// directory and takes the screenshots (TELA_CHROMIUM_BIN, default
// "chromium"):
//
//	chromium --headless --disable-gpu --hide-scrollbars --force-device-scale-factor=N \
//	  --window-size=W,H --screenshot=testdata/chromium/<size>_x<N>.png page.html
//
// at scales 1 and 2, and records the digest of the cells in
// testdata/chromium/cells.sha256, so that the test notices a regenerated
// icon set whose screenshots are old. The pages are not committed (they are
// 600 KiB each and made from icons_gen.go). The test compares each cell:
// the mean difference per RGB channel over the cell, and the share of the
// ink (the reference's darkness) that differs. TELA_CHROMIUM_DUMP=dir writes
// Tela's renderings and the 40 worst cells side by side with Chromium's.

const (
	gridCols = 64
	cellPx   = 32
	inkColor = "#1e293b" // slate-800
)

var gridSizes = []int{16, 24} // icon sizes in px

// distinct returns the icons with their first name (sorted), dropping the
// ones that repeat an earlier SVG: Lucide's renamed icons keep the old name
// as an alias with the same drawing.
func distinct(t testing.TB) []entry {
	t.Helper()
	all := append([]entry(nil), allIcons(t)...)
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	seen := map[string]bool{}
	var out []entry
	for _, e := range all {
		if !seen[e.SVG] {
			seen[e.SVG] = true
			out = append(out, e)
		}
	}
	return out
}

func gridSize(n int) (w, h int) {
	rows := (n + gridCols - 1) / gridCols
	return gridCols * cellPx, rows * cellPx
}

func cellsDigest(cells []entry) string {
	sum := sha256.New()
	for _, c := range cells {
		fmt.Fprintf(sum, "%s\n%s\n", c.Name, c.SVG)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func galleryHTML(cells []entry, size int) string {
	var sb strings.Builder
	w, _ := gridSize(len(cells))
	fmt.Fprintf(&sb, "<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n"+
		"html, body { margin: 0; background: #fff; }\n"+
		".g { display: flex; flex-wrap: wrap; width: %dpx; }\n"+
		".c { flex: none; width: %dpx; height: %dpx; display: flex; align-items: center; justify-content: center; }\n"+
		"svg { display: block; width: %dpx; height: %dpx; color: %s; }\n"+
		"</style></head><body><div class=\"g\">\n", w, cellPx, cellPx, size, size, inkColor)
	for _, c := range cells {
		sb.WriteString("<div class=\"c\">" + c.SVG + "</div>\n")
	}
	sb.WriteString("</div></body></html>\n")
	return sb.String()
}

// renderGrid shows the cells through h.Icon and returns the window.
func renderGrid(t testing.TB, cells []entry, size int, scale float64) *image.RGBA {
	t.Helper()
	w, hgt := gridSize(len(cells))
	iconClass := fmt.Sprintf("size-[%dpx] text-[%s]", size, inkColor)
	page := func() tela.Node {
		kids := make([]tela.Child, 0, len(cells)+1)
		kids = append(kids, h.Class(fmt.Sprintf("flex flex-wrap w-[%dpx]", w)))
		for _, c := range cells {
			kids = append(kids, h.Div(h.Class("flex size-8 flex-none items-center justify-center"),
				h.Icon(tela.Icon{SVG: c.SVG}, h.Class(iconClass))))
		}
		return h.Div(kids...)
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(float32(w), float32(hgt)), telatest.Scale(scale), telatest.RootClass("bg-white"))
	return app.Screenshot()
}

type cellStat struct {
	name      string
	mean, rel float64
	max       int
}

// compare measures the cell i of a against b.
func compareCell(a, b *image.RGBA, r image.Rectangle) (mean, rel float64, max int) {
	var sum, ink float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pa, pb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			for _, d := range [3]int{int(pa.R) - int(pb.R), int(pa.G) - int(pb.G), int(pa.B) - int(pb.B)} {
				if d < 0 {
					d = -d
				}
				sum += float64(d)
				max = maxInt(max, d)
			}
			ink += float64(255*3 - int(pb.R) - int(pb.G) - int(pb.B))
		}
	}
	n := float64(r.Dx() * r.Dy() * 3)
	mean = sum / n
	if ink > 0 {
		rel = sum / ink
	}
	return
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func readPNG(t testing.TB, path string) *image.RGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

func writePNG(t testing.TB, path string, img image.Image) {
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

// regenerate rebuilds the Chromium screenshots.
func regenerate(t *testing.T, cells []entry, dir string) {
	t.Helper()
	bin := os.Getenv("TELA_CHROMIUM_BIN")
	if bin == "" {
		bin = "chromium"
	}
	tmp := t.TempDir()
	w, hgt := gridSize(len(cells))
	for _, size := range gridSizes {
		page := filepath.Join(tmp, fmt.Sprintf("gallery_%d.html", size))
		if err := os.WriteFile(page, []byte(galleryHTML(cells, size)), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, scale := range []int{1, 2} {
			raw := filepath.Join(tmp, fmt.Sprintf("%d_x%d.png", size, scale))
			cmd := exec.Command(bin, "--headless", "--disable-gpu", "--hide-scrollbars",
				fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", w, hgt),
				"--screenshot="+raw, "file://"+page)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", bin, err, b)
			}
			// Re-encode with the best compression: the screenshots are
			// committed.
			writePNG(t, filepath.Join(dir, fmt.Sprintf("%d_x%d.png", size, scale)), readPNG(t, raw))
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cells.sha256"), []byte(cellsDigest(cells)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Bounds: the measured worst values (ADR-102) plus a margin. The averages
// are 0.5-0.9 levels per channel and 1.8-6.7% of the ink; the worst icons
// are circles and curves at scale 1, where Skia's coverage of anti-aliased
// edges differs from exact area coverage.
const (
	boundMean = 3.5  // mean difference of a cell, levels per channel (measured worst 3.10)
	boundRel  = 0.25 // share of a cell's ink that differs (measured worst 0.21)
	boundAvg  = 1.05 // mean of the cells' means (measured worst 0.93)
)

func TestChromiumLucide(t *testing.T) {
	cells := distinct(t)
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		regenerate(t, cells, dir)
	}
	want, err := os.ReadFile(filepath.Join(dir, "cells.sha256"))
	if err != nil || strings.TrimSpace(string(want)) != cellsDigest(cells) {
		t.Fatalf("the screenshots in %s are not of these icons; regenerate them with TELA_CHROMIUM=1 (needs chromium)", dir)
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	for _, size := range gridSizes {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("%dpx_x%d", size, scale), func(t *testing.T) {
				ref := readPNG(t, filepath.Join(dir, fmt.Sprintf("%d_x%d.png", size, scale)))
				got := renderGrid(t, cells, size, float64(scale))
				if got.Rect != ref.Rect {
					t.Fatalf("render %v, Chromium %v", got.Rect, ref.Rect)
				}
				stats := make([]cellStat, len(cells))
				var sumMean, sumRel float64
				for i, c := range cells {
					x, y := (i%gridCols)*cellPx*scale, (i/gridCols)*cellPx*scale
					r := image.Rect(x, y, x+cellPx*scale, y+cellPx*scale)
					m, rel, mx := compareCell(got, ref, r)
					stats[i] = cellStat{c.Name, m, rel, mx}
					sumMean += m
					sumRel += rel
				}
				report(t, stats, sumMean/float64(len(cells)), sumRel/float64(len(cells)))
				if dump != "" {
					dumpWorst(t, dump, fmt.Sprintf("%dpx_x%d", size, scale), got, ref, stats, scale)
				}
				if avg := sumMean / float64(len(cells)); avg > boundAvg {
					t.Errorf("mean of the cells' means %.3f, bound %.2f", avg, boundAvg)
				}
				for _, s := range stats {
					if s.mean > boundMean || s.rel > boundRel {
						t.Errorf("%s: mean %.3f (bound %.2f), %.1f%% of the ink differs (bound %.0f%%), max %d", s.name, s.mean, boundMean, s.rel*100, boundRel*100, s.max)
					}
				}
			})
		}
	}
}

func report(t *testing.T, stats []cellStat, avgMean, avgRel float64) {
	means := make([]float64, len(stats))
	rels := make([]float64, len(stats))
	for i, s := range stats {
		means[i], rels[i] = s.mean, s.rel
	}
	sort.Float64s(means)
	sort.Float64s(rels)
	q := func(v []float64, p float64) float64 { return v[int(math.Round(p*float64(len(v)-1)))] }
	byMean := append([]cellStat(nil), stats...)
	sort.Slice(byMean, func(i, j int) bool { return byMean[i].mean > byMean[j].mean })
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%d icons: mean level per channel: average %.3f, median %.3f, p90 %.3f, p99 %.3f, max %.3f; share of ink that differs: average %.1f%%, median %.1f%%, p90 %.1f%%, p99 %.1f%%, max %.1f%%\nworst by mean:",
		len(stats), avgMean, q(means, .5), q(means, .9), q(means, .99), means[len(means)-1],
		avgRel*100, q(rels, .5)*100, q(rels, .9)*100, q(rels, .99)*100, rels[len(rels)-1]*100)
	for _, s := range byMean[:min(10, len(byMean))] {
		fmt.Fprintf(&sb, "\n  %-28s mean %.3f  ink %.1f%%  max %d", s.name, s.mean, s.rel*100, s.max)
	}
	t.Log(sb.String())
}

// dumpWorst writes Tela's rendering and, for the worst cells, Tela | Chromium
// | difference side by side, magnified.
func dumpWorst(t *testing.T, dir, name string, got, ref *image.RGBA, stats []cellStat, scale int) {
	writePNG(t, filepath.Join(dir, "lucide_"+name+"_tela.png"), got)
	idx := make([]int, len(stats))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return stats[idx[a]].mean > stats[idx[b]].mean })
	const n = 40
	mag := 8 / scale // output pixels per device pixel keeps every cell 256 px
	cell := cellPx * scale * mag
	cols := 8
	rows := (n + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, cols*(3*cell+6), rows*(cell+4)))
	for i := range sheet.Pix {
		sheet.Pix[i] = 0x80
	}
	for k := 0; k < n && k < len(idx); k++ {
		i := idx[k]
		ox := (k % cols) * (3*cell + 6)
		oy := (k / cols) * (cell + 4)
		sx, sy := (i%gridCols)*cellPx*scale, (i/gridCols)*cellPx*scale
		for y := 0; y < cellPx*scale; y++ {
			for x := 0; x < cellPx*scale; x++ {
				a, b := got.RGBAAt(sx+x, sy+y), ref.RGBAAt(sx+x, sy+y)
				d := color.RGBA{R: 255 - uint8(absInt(int(a.G)-int(b.G))*4%256), G: 255 - uint8(absInt(int(a.G)-int(b.G))*4%256), B: 255 - uint8(absInt(int(a.G)-int(b.G))*4%256), A: 255}
				for dy := 0; dy < mag; dy++ {
					for dx := 0; dx < mag; dx++ {
						sheet.SetRGBA(ox+x*mag+dx, oy+y*mag+dy, a)
						sheet.SetRGBA(ox+cell+2+x*mag+dx, oy+y*mag+dy, b)
						sheet.SetRGBA(ox+2*cell+4+x*mag+dx, oy+y*mag+dy, d)
					}
				}
			}
		}
	}
	writePNG(t, filepath.Join(dir, "lucide_"+name+"_worst.png"), sheet)
	var sb strings.Builder
	for k := 0; k < n && k < len(idx); k++ {
		fmt.Fprintf(&sb, "%d %s mean %.3f\n", k, stats[idx[k]].name, stats[idx[k]].mean)
	}
	if err := os.WriteFile(filepath.Join(dir, "lucide_"+name+"_worst.txt"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
