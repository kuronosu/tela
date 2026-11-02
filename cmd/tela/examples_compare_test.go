package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"
)

// The OS-independent half of TestExamplesCocoa (examples_cocoa_e2e_test.go):
// finding the examples and the window size each one declares, and comparing
// a window's pixels with the headless reference. TestExamplesCompare checks
// it on every OS.

// exampleDirs returns the names of the directories under root that hold a
// package main: the examples.
func exampleDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(root, e.Name(), "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			pf, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.PackageClauseOnly)
			if err == nil && pf.Name.Name == "main" {
				names = append(names, e.Name())
				break
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

// declaredSizes returns the distinct "WxH" of the tela.App composite
// literals in the package's non-test files that set both Width and Height
// to constants: the window size main asks for (from `var App` or the
// literal main passes to tela.Main).
func declaredSizes(dir string, files []string) ([]string, error) {
	var sizes []string
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isTelaApp(lit.Type) {
				return true
			}
			var w, h string
			for _, e := range lit.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				val, isLit := kv.Value.(*ast.BasicLit)
				if !ok || !isLit {
					continue
				}
				switch key.Name {
				case "Width":
					w = val.Value
				case "Height":
					h = val.Value
				}
			}
			if w != "" && h != "" {
				if s := w + "x" + h; !slices.Contains(sizes, s) {
					sizes = append(sizes, s)
				}
			}
			return true
		})
	}
	return sizes, nil
}

// isTelaApp reports whether a composite literal's type is tela.App (any
// import name).
func isTelaApp(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "App"
}

// Where a differing pixel lies: the window server rounds the window's
// corners and may draw a rim on its edges, so the comparison counts the
// differing pixels by place.
const (
	cornerBand = 32 // physical pixels from two edges
	edgeBand   = 2  // physical pixels from an edge
)

// pixelDiff is the comparison of a window read back with the reference.
type pixelDiff struct {
	size      image.Point // of the reference
	sizeErr   string      // the sizes differ: nothing else is set
	total     int
	identical int
	differing int // not masked
	maxDiff   int // the largest channel difference of a differing pixel
	// hist counts the differing pixels by their largest channel
	// difference: 1, 2-4, 5-16, 17-64, 65-255.
	hist                   [5]int
	corner, edge, interior int // differing pixels by place
	masked                 []maskStat
	regions                []diffRegion // sorted by size, largest first
}

// maskStat is what one mask reason excluded.
type maskStat struct {
	reason    string
	n         int
	differing int // of the n, the pixels that differ from the reference
	box       image.Rectangle
	outside   int // pixels outside the corner bands (for the window shape)
}

// diffRegion is a connected group of differing pixels (8-connected at
// regionCell granularity).
type diffRegion struct {
	box       image.Rectangle
	n         int
	maxDiff   int
	at        image.Point // the pixel with maxDiff
	got, want color.RGBA
}

const regionCell = 8

func (d pixelDiff) ok() bool { return d.sizeErr == "" && d.differing == 0 }

// summary is one line: the counts and the largest difference.
func (d pixelDiff) summary() string {
	if d.sizeErr != "" {
		return d.sizeErr
	}
	s := fmt.Sprintf("%dx%d: %d of %d pixels identical, %d differing", d.size.X, d.size.Y, d.identical, d.total, d.differing)
	if d.differing > 0 {
		s += fmt.Sprintf(" (max channel difference %d; by difference 1:%d 2-4:%d 5-16:%d 17-64:%d 65+:%d; corners %d, edges %d, interior %d; %d region(s))",
			d.maxDiff, d.hist[0], d.hist[1], d.hist[2], d.hist[3], d.hist[4], d.corner, d.edge, d.interior, len(d.regions))
	}
	for _, m := range d.masked {
		s += fmt.Sprintf("; masked %s: %d pixels in %v, %d of them differing", m.reason, m.n, m.box, m.differing)
		if m.outside > 0 {
			s += fmt.Sprintf(", %d outside the corners", m.outside)
		}
	}
	return s
}

// details lists the largest regions, at most n.
func (d pixelDiff) details(n int) []string {
	var lines []string
	for i, r := range d.regions {
		if i == n {
			lines = append(lines, fmt.Sprintf("... and %d more region(s)", len(d.regions)-n))
			break
		}
		lines = append(lines, fmt.Sprintf("region %d: %v (%dx%d), %d pixels, max channel difference %d at (%d,%d): window %s, reference %s",
			i+1, r.box, r.box.Dx(), r.box.Dy(), r.n, r.maxDiff, r.at.X, r.at.Y, hexRGBA(r.got), hexRGBA(r.want)))
	}
	return lines
}

func hexRGBA(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A) }

// channelDiff is the largest difference of the four channels.
func channelDiff(a, b color.RGBA) int {
	d := 0
	for _, v := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
		x := int(v[0]) - int(v[1])
		d = max(d, x, -x)
	}
	return d
}

// inCorner reports whether p is within cornerBand of two edges of size.
func inCorner(p, size image.Point) bool {
	return (p.X < cornerBand || p.X >= size.X-cornerBand) && (p.Y < cornerBand || p.Y >= size.Y-cornerBand)
}

// comparePixels compares got (a window read back) with want (the
// reference). mask returns why a pixel is not compared ("" to compare it);
// it may be nil.
func comparePixels(got, want *image.RGBA, mask func(x, y int, got color.RGBA) string) pixelDiff {
	size := want.Rect.Size()
	d := pixelDiff{size: size}
	if got.Rect.Size() != size {
		d.sizeErr = fmt.Sprintf("the window read back is %dx%d pixels, the reference %dx%d", got.Rect.Dx(), got.Rect.Dy(), size.X, size.Y)
		return d
	}
	type cell struct {
		n, maxDiff int
		box        image.Rectangle
		at         image.Point
		got, want  color.RGBA
	}
	cw, ch := (size.X+regionCell-1)/regionCell, (size.Y+regionCell-1)/regionCell
	cells := make([]cell, cw*ch)
	masks := map[string]*maskStat{}
	for y := range size.Y {
		for x := range size.X {
			g := got.RGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
			w := want.RGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
			diff := channelDiff(g, w)
			d.total++
			p := image.Pt(x, y)
			if mask != nil {
				if reason := mask(x, y, g); reason != "" {
					m := masks[reason]
					if m == nil {
						m = &maskStat{reason: reason}
						masks[reason] = m
					}
					m.n++
					m.box = m.box.Union(image.Rectangle{p, p.Add(image.Pt(1, 1))})
					if diff > 0 {
						m.differing++
					}
					if !inCorner(p, size) {
						m.outside++
					}
					continue
				}
			}
			if diff == 0 {
				d.identical++
				continue
			}
			d.differing++
			d.maxDiff = max(d.maxDiff, diff)
			switch {
			case diff == 1:
				d.hist[0]++
			case diff <= 4:
				d.hist[1]++
			case diff <= 16:
				d.hist[2]++
			case diff <= 64:
				d.hist[3]++
			default:
				d.hist[4]++
			}
			switch {
			case inCorner(p, size):
				d.corner++
			case x < edgeBand || y < edgeBand || x >= size.X-edgeBand || y >= size.Y-edgeBand:
				d.edge++
			default:
				d.interior++
			}
			c := &cells[(y/regionCell)*cw+x/regionCell]
			c.n++
			c.box = c.box.Union(image.Rectangle{p, p.Add(image.Pt(1, 1))})
			if diff > c.maxDiff {
				c.maxDiff, c.at, c.got, c.want = diff, p, g, w
			}
		}
	}
	for _, m := range masks {
		d.masked = append(d.masked, *m)
	}
	slices.SortFunc(d.masked, func(a, b maskStat) int { return strings.Compare(a.reason, b.reason) })

	// Regions: the 8-connected components of the cells with differences.
	seen := make([]bool, len(cells))
	for i := range cells {
		if cells[i].n == 0 || seen[i] {
			continue
		}
		var r diffRegion
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			c := cells[j]
			r.n += c.n
			r.box = r.box.Union(c.box)
			if c.maxDiff > r.maxDiff {
				r.maxDiff, r.at, r.got, r.want = c.maxDiff, c.at, c.got, c.want
			}
			cx, cy := j%cw, j/cw
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := cx+dx, cy+dy
					if nx < 0 || ny < 0 || nx >= cw || ny >= ch {
						continue
					}
					k := ny*cw + nx
					if cells[k].n > 0 && !seen[k] {
						seen[k] = true
						stack = append(stack, k)
					}
				}
			}
		}
		d.regions = append(d.regions, r)
	}
	slices.SortStableFunc(d.regions, func(a, b diffRegion) int { return b.n - a.n })
	return d
}

// diffPanels returns the reference, the window and a map of the
// differences side by side (a 4-pixel gap between them): in the map the
// reference is dimmed, differing pixels are red (brighter for larger
// differences) and masked pixels blue.
func diffPanels(got, want *image.RGBA, mask func(x, y int, got color.RGBA) string) *image.RGBA {
	const gap = 4
	size := want.Rect.Size()
	gs := got.Rect.Size()
	h := max(size.Y, gs.Y)
	out := image.NewRGBA(image.Rect(0, 0, size.X+gap+gs.X+gap+size.X, h))
	for i := range out.Pix {
		out.Pix[i] = 0x80 // the gaps and any size mismatch: mid gray
	}
	for y := range h {
		for x := range max(size.X, gs.X) {
			var w, g color.RGBA
			inW := x < size.X && y < size.Y
			inG := x < gs.X && y < gs.Y
			if inW {
				w = want.RGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
				out.SetRGBA(x, y, w)
			}
			if inG {
				g = got.RGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
				out.SetRGBA(size.X+gap+x, y, g)
			}
			if !inW {
				continue
			}
			mx := size.X + gap + gs.X + gap + x
			lum := uint8((int(w.R)*3 + int(w.G)*6 + int(w.B)) / 10 / 3)
			c := color.RGBA{lum, lum, lum, 0xff}
			switch {
			case !inG:
			case mask != nil && mask(x, y, g) != "":
				c = color.RGBA{0x20, 0x40, 0xff, 0xff}
			default:
				if diff := channelDiff(g, w); diff > 0 {
					c = color.RGBA{uint8(min(255, 128+diff*2)), 0, 0, 0xff}
				}
			}
			out.SetRGBA(mx, y, c)
		}
	}
	return out
}

// TestExamplesCompare checks the helpers of TestExamplesCocoa: the examples
// and their declared sizes, and the pixel comparison.
func TestExamplesCompare(t *testing.T) {
	root := filepath.Join(repoRoot(t), "examples")
	names, err := exampleDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"counter", "hello", "kit", "notes"} {
		if !slices.Contains(names, want) {
			t.Errorf("exampleDirs found %v, without %s", names, want)
		}
	}
	for name, want := range map[string]string{"counter": "800x600", "hello": "800x450", "kit": "1280x800", "notes": "960x640"} {
		dir := filepath.Join(root, name)
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for i, f := range files {
			files[i] = filepath.Base(f)
		}
		sizes, err := declaredSizes(dir, files)
		if err != nil {
			t.Fatal(err)
		}
		if len(sizes) != 1 || sizes[0] != want {
			t.Errorf("%s declares %v, want [%s]", name, sizes, want)
		}
	}

	// A 100x60 reference; the window differs in a 3x2 patch by 20 levels,
	// in one pixel by 1 level, and its top-left corner pixel is transparent.
	want := image.NewRGBA(image.Rect(0, 0, 100, 60))
	for i := range want.Pix {
		want.Pix[i] = 0xff
	}
	got := image.NewRGBA(image.Rect(10, 10, 110, 70)) // bounds need not start at 0
	copy(got.Pix, want.Pix)
	for y := 40; y < 42; y++ {
		for x := 50; x < 53; x++ {
			got.SetRGBA(10+x, 10+y, color.RGBA{235, 255, 255, 255})
		}
	}
	got.SetRGBA(10+40, 10+30, color.RGBA{255, 254, 255, 255})
	got.SetRGBA(10, 10, color.RGBA{})
	shape := func(_, _ int, c color.RGBA) string {
		if c.A != 0xff {
			return "window shape"
		}
		return ""
	}
	d := comparePixels(got, want, shape)
	if d.ok() || d.differing != 7 || d.maxDiff != 20 || d.hist != [5]int{1, 0, 0, 6, 0} || d.interior != 7 {
		t.Errorf("comparison: %s", d.summary())
	}
	if len(d.regions) != 2 || d.regions[0].box != image.Rect(50, 40, 53, 42) || d.regions[0].n != 6 || d.regions[0].at != image.Pt(50, 40) {
		t.Errorf("regions: %v", d.details(5))
	}
	if len(d.masked) != 1 || d.masked[0].n != 1 || d.masked[0].differing != 1 || d.masked[0].outside != 0 {
		t.Errorf("masked: %+v", d.masked)
	}
	t.Logf("%s; %s", d.summary(), strings.Join(d.details(5), "; "))
	if p := diffPanels(got, want, shape); p.Rect.Size() != image.Pt(308, 60) || p.RGBAAt(208+51, 41).R < 128 || p.RGBAAt(208, 0) != (color.RGBA{0x20, 0x40, 0xff, 0xff}) {
		t.Errorf("diffPanels: %v, diff pixel %v, masked pixel %v", p.Rect, p.RGBAAt(208+51, 41), p.RGBAAt(208, 0))
	}
	if d := comparePixels(want, want, nil); !d.ok() || d.identical != 6000 {
		t.Errorf("identical images: %s", d.summary())
	}
	if d := comparePixels(image.NewRGBA(image.Rect(0, 0, 99, 60)), want, nil); d.ok() || d.sizeErr == "" {
		t.Errorf("sizes 99x60 and 100x60: %s", d.summary())
	}

	// The macos-latest runner: a 1024x677 visible frame, a 32-point title
	// bar and 1024x642 of content for a window that asks for more.
	for _, c := range []struct {
		declared, content, screen, extra float64
		ok, clamped                      bool
	}{
		{1440, 1024, 1024, 0, true, true},
		{900, 642, 677, 32, true, true},
		{720, 645, 677, 32, true, true},
		{720, 500, 677, 32, false, true},  // far less than the screen
		{720, 650, 677, 32, false, true},  // a frame larger than the screen
		{600, 600, 677, 32, true, false},  // fits: exactly the declared size
		{600, 599, 677, 32, false, false}, // fits, but not the declared size
		{760, 760, 0, 32, true, false},    // no screen known
	} {
		if ok, clamped := axisSize(c.declared, c.content, c.screen, c.extra); ok != c.ok || clamped != c.clamped {
			t.Errorf("axisSize(%g, %g, %g, %g) = %v, %v, want %v, %v", c.declared, c.content, c.screen, c.extra, ok, clamped, c.ok, c.clamped)
		}
	}

	// The reference's generated file is Go for both contracts it serves.
	for _, c := range []struct {
		k      contract
		hasApp bool
	}{{contractApp, true}, {contractPreview, false}, {contractPreview, true}} {
		src := exampleReferenceSource("main", c.k, c.hasApp)
		if _, err := parser.ParseFile(token.NewFileSet(), generatedFile, src, 0); err != nil {
			t.Errorf("exampleReferenceSource(%v, App %v): %v\n%s", c.k, c.hasApp, err, src)
		}
	}
}

// clampShare is the least share of the screen's visible frame that a
// window's frame takes on an axis its declared size does not fit.
const clampShare = 0.9

// axisSize checks a window's content length on one axis against the
// declared one, for a screen whose visible frame is screen points long
// there and a window frame that adds extra points (the title bar). AppKit
// keeps a window's frame inside the visible frame, and not exactly at its
// edge (on the macos-latest runner the frame ends 3 points short), so no
// exact clamp is computed: where the declared size fits (declared + extra
// <= screen) the content must be the declared size; where it does not,
// the frame must fit in the visible frame and take at least clampShare of
// it, and clamped reports it. A zero screen (unknown) clamps nothing.
func axisSize(declared, content, screen, extra float64) (ok, clamped bool) {
	if screen <= 0 || declared+extra <= screen {
		return content == declared, false
	}
	frame := content + extra
	return frame <= screen && frame >= clampShare*screen, true
}

// The environment of the reference test (exampleReferenceSource).
const (
	exrefW        = "TELA_EXREF_W"
	exrefH        = "TELA_EXREF_H"
	exrefScale    = "TELA_EXREF_SCALE"
	exrefDark     = "TELA_EXREF_DARK"     // "true": the system is dark
	exrefReduced  = "TELA_EXREF_REDUCED"  // "true": reduced motion
	exrefContrast = "TELA_EXREF_CONTRAST" // "true": increased contrast
	exrefAccent   = "TELA_EXREF_ACCENT"   // #rrggbb, or empty
	exrefAdvance  = "TELA_EXREF_ADVANCE"  // a time.Duration
	exrefOut      = "TELA_EXREF_OUT"      // the PNG
	// exrefControls is the rectangle the window's standard buttons cover
	// in a TitleBarOverlay window, "x,y,w,h" in points (the window's
	// TitleBarControls), or empty: the reference's window reports the
	// same, so the app lays its title bar out around the same buttons
	// (without it the headless window acts as a frameless one and the
	// app draws its own controls).
	exrefControls = "TELA_EXREF_CONTROLS"
)

// exampleReferenceTest is the test function of exampleReferenceSource.
const exampleReferenceTest = "TestTelaExampleReference"

// exampleReferenceSource returns the generated test file that renders the
// reference of TestExamplesCocoa in a package called pkgName: the screen of
// contract c (telaBuild, from tela snapshot's contract code, with telaApp
// when hasApp), mounted with telatest.Mount and FromApp in the window's
// size, scale and system settings, then advanced by as long as the window
// was shown, so that idle timers (an overlay scrollbar's fade, a caret's
// blink) end as they did in the window. Dev errors are logged, not fatal:
// the window runs in dev mode too, and fails on none.
func exampleReferenceSource(pkgName string, c contract, hasApp bool) []byte {
	var buf strings.Builder
	err := exampleReferenceTemplate.Execute(&buf, map[string]any{
		"Package": pkgName, "Kind": c.String(), "HasApp": hasApp, "Test": exampleReferenceTest,
		"W": exrefW, "H": exrefH, "Scale": exrefScale, "Dark": exrefDark, "Reduced": exrefReduced,
		"Contrast": exrefContrast, "Accent": exrefAccent, "Advance": exrefAdvance, "Out": exrefOut,
		"Controls": exrefControls,
	})
	if err != nil {
		panic(err) // the template is a constant
	}
	return []byte(buf.String())
}

var exampleReferenceTemplate = template.Must(template.New("ref").Parse(`// Code generated by TestExamplesCocoa; DO NOT EDIT.

package {{.Package}}

import (
	telafmt "fmt"
	telapng "image/png"
	telaos "os"
	telasort "sort"
	telastrconv "strconv"
	telastrings "strings"
	telatesting "testing"
	telatime "time"

	telatela "github.com/kuronosu/tela"
	telatest "github.com/kuronosu/tela/telatest"
)

var (
	_ = telasort.Strings
	_ = telastrings.Join
	_ = telafmt.Errorf
)

{{template "contract" .}}
func {{.Test}}(t *telatesting.T) {
	num := func(name string) float64 {
		v, err := telastrconv.ParseFloat(telaos.Getenv(name), 64)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return v
	}
	w, h, scale := float32(num({{printf "%q" .W}})), float32(num({{printf "%q" .H}})), num({{printf "%q" .Scale}})
	advance, err := telatime.ParseDuration(telaos.Getenv({{printf "%q" .Advance}}))
	if err != nil {
		t.Fatal(err)
	}
	root, _, _, err := telaBuild("")
	if err != nil {
		t.Fatal(err)
	}
	opts := []telatest.Option{telatest.Size(w, h), telatest.Scale(scale), telatest.AllowDev("")}
	if telaos.Getenv({{printf "%q" .Dark}}) == "true" {
		opts = append(opts, telatest.Dark())
	}
	if telaos.Getenv({{printf "%q" .Reduced}}) == "true" {
		opts = append(opts, telatest.ReducedMotion())
	}
	if telaos.Getenv({{printf "%q" .Contrast}}) == "true" {
		opts = append(opts, telatest.HighContrast())
	}
	if a := telaos.Getenv({{printf "%q" .Accent}}); a != "" {
		opts = append(opts, telatest.AccentColor(telatela.Hex(a)))
	}
	if c := telaos.Getenv({{printf "%q" .Controls}}); c != "" {
		var r [4]float32
		for i, s := range telastrings.Split(c, ",") {
			v, err := telastrconv.ParseFloat(s, 32)
			if err != nil || i >= 4 {
				t.Fatalf("%s=%q: want x,y,w,h", {{printf "%q" .Controls}}, c)
			}
			r[i] = float32(v)
		}
		opts = append(opts, telatest.TitleBarControls(telatela.Rect{X: r[0], Y: r[1], Width: r[2], Height: r[3]}))
	}
{{- if .HasApp}}
	if telaHasApp {
		opts = append(opts, telatest.FromApp(telaApp), telatest.Size(w, h)) // the window's size
	}
{{- end}}
	app := telatest.Mount(t, root, opts...)
	app.Advance(advance)
	img := app.Screenshot()
	f, err := telaos.Create(telaos.Getenv({{printf "%q" .Out}}))
	if err != nil {
		t.Fatal(err)
	}
	if err := telapng.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
` + contractTemplate))
