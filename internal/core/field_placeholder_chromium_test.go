package core

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/chromium"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The textarea placeholder comparison: textareas with single-line,
// multi-line and wrapping placeholders, a kit (shadcn) Textarea among them,
// rendered here and by Chromium with Tailwind's CSS, at scales 1, 1.5 and
// 2. The placeholder is black ("HHH" lines: flat tops and bottoms), and
// the test compares the runs of rows of each textarea that hold ink: the
// top and bottom of every placeholder line, so a line one device pixel
// off, a different line height or a different wrap fails. ADR-265 measured
// the Input's placeholder rule; a Textarea's placeholder starts at the top
// of the content box like its value and keeps the textarea's line height.
// testdata/chromium/placeholders holds the page, its CSS and Chromium's
// screenshots; regenerate them with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumTextareaPlaceholders ./internal/core/

// kitTextarea is kit's Textarea recipe (shadcn's textarea.tsx at
// kit.ShadcnCommit) with the placeholder in black.
const kitTextarea = "resize-none flex field-sizing-content min-h-16 w-full rounded-md border border-slate-300 bg-transparent px-3 py-2 text-base shadow-xs md:text-sm placeholder:text-black"

var placeholderCases = []struct {
	name   string
	height int // the cell's height
	n      fnode
}{
	{"plain", 72, fm("textarea", "w-60 resize-none border border-slate-300 placeholder:text-black", "placeholder", "HHH one line")},
	{"kit", 88, fel("div", "w-72", fm("textarea", kitTextarea, "placeholder", "HHH Type your message here."))},
	{"kit-lines", 104, fel("div", "w-72", fm("textarea", kitTextarea, "placeholder", "HHH first line\nHHH second line\nHHH"))},
	{"lines", 104, fm("textarea", "h-24 w-60 resize-none border border-slate-300 px-3 py-2 text-sm placeholder:text-black", "placeholder", "HHH\nHHH H\nHH")},
	{"wrap", 120, fm("textarea", "h-28 w-40 resize-none border border-slate-300 px-2 py-1 text-sm placeholder:text-black", "placeholder", "HHH HHH HHH HHH HHH HHH HHH HHH HHH HHH")},
	{"leading", 104, fm("textarea", "h-24 w-60 resize-none border border-slate-300 px-3 py-2 text-sm leading-[30px] placeholder:text-black", "placeholder", "HHH\nHHH")},
	{"ph-size", 104, fm("textarea", "h-24 w-60 resize-none border border-slate-300 px-3 py-2 text-sm leading-5 placeholder:text-[20px] placeholder:text-black", "placeholder", "HHH\nHHH")},
	{"rows", 88, fm("textarea", "w-60 resize-none border border-slate-300 px-3 py-1 text-[15px] placeholder:text-black", "rows", "3", "placeholder", "HHH\nHHH")},
}

var placeholderScales = []float32{1, 1.5, 2}

const placeholderW = 320

func placeholderRoot() fnode {
	var kids []fnode
	for _, c := range placeholderCases {
		kids = append(kids, fel("div", fmt.Sprintf("h-[%dpx] p-2", c.height), c.n))
	}
	return fel("div", fmt.Sprintf("w-[%dpx]", placeholderW), kids...)
}

func placeholderSize() (w, h float32) {
	for _, c := range placeholderCases {
		h += float32(c.height)
	}
	return placeholderW, h
}

func placeholderHTML() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<link rel=\"stylesheet\" href=\"gallery.css\">\n</head>\n<body class=\"bg-white\">\n")
	id := 0
	placeholderRoot().html(&sb, &id)
	sb.WriteString("\n</body>\n</html>\n")
	return sb.String()
}

// renderPlaceholders renders the page at scale and returns the image and
// the border box of every textarea.
func renderPlaceholders(t *testing.T, scale float32) (*image.RGBA, []gfx.Rect) {
	t.Helper()
	var logs []string
	cfg := Config{Fonts: testFonts, RootClass: "bg-white overflow-hidden", IDPrefix: "r-", ClassicScrollbars: true,
		Log: func(m string) { logs = append(logs, m) }}
	w, h := placeholderSize()
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.SetScale(scale)
	id := 0
	tr.Mount(placeholderRoot().node(&id))
	dl, _ := tr.Frame()
	img := tr.Rasterize(dl, scale)
	for _, m := range logs {
		t.Errorf("dev message: %s", m)
	}
	rects := make([]gfx.Rect, id)
	for k := range id {
		rects[k] = tr.boxWindowRect(keyed(tr, k).box)
	}
	tr.Unmount()
	return img, rects
}

// inkRuns returns the runs of rows of r in img that hold a dark pixel, as
// [first, end) pairs.
func inkRuns(img *image.RGBA, r image.Rectangle) [][2]int {
	var runs [][2]int
	in := false
	for y := r.Min.Y; y < r.Max.Y; y++ {
		dark := false
		for x := r.Min.X; x < r.Max.X && !dark; x++ {
			dark = img.RGBAAt(x, y).G < 128
		}
		switch {
		case dark && !in:
			runs = append(runs, [2]int{y, y + 1})
		case dark:
			runs[len(runs)-1][1] = y + 1
		}
		in = dark
	}
	return runs
}

func TestChromiumTextareaPlaceholders(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "placeholders")
	w, h := placeholderSize()
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		page, _ := filepath.Abs(filepath.Join(dir, "gallery.html"))
		if err := os.WriteFile(page, []byte(placeholderHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
		chromium.Tailwind(t, page, filepath.Join(dir, "gallery.css"), "@theme { --font-sans: \"Inter\"; }\n"+inlineGalleryFonts)
		for _, s := range placeholderScales {
			shot, _ := filepath.Abs(filepath.Join(dir, fmt.Sprintf("x%g.png", s)))
			chromium.ScreenshotAt(t, "file://"+page, shot, int(w), int(h), float64(s), "--allow-file-access-from-files", "--virtual-time-budget=2000")
			golden.WriteBestPNG(t, shot, golden.ReadRGBA(t, shot))
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(b) != placeholderHTML() {
		t.Fatalf("%s/gallery.html is out of date; regenerate with TELA_CHROMIUM=1", dir)
	}
	for _, s := range placeholderScales {
		img, rects := renderPlaceholders(t, s)
		if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
			golden.WritePNG(t, filepath.Join(d, fmt.Sprintf("placeholders_x%g_tela.png", s)), img)
		}
		ref := golden.ReadRGBA(t, filepath.Join(dir, fmt.Sprintf("x%g.png", s)))
		if ref.Rect != img.Rect {
			t.Fatalf("x%g: render %v, Chromium %v", s, img.Rect, ref.Rect)
		}
		for i, c := range placeholderCases {
			r := rects[i]
			dr := image.Rect(int(r.X0*s), int(r.Y0*s), int(r.X1*s), int(r.Y1*s))
			// The bottom of each line's "H"s is its baseline, on whole
			// device pixels; their top is the cap height, anti-aliased
			// differently by Skia (which hints Inter's horizontal stems):
			// one row apart at most.
			got, want := inkRuns(img, dr), inkRuns(ref, dr)
			ok := len(got) == len(want)
			for j := 0; ok && j < len(got); j++ {
				ok = got[j][1] == want[j][1] && got[j][0]-want[j][0] <= 1 && want[j][0]-got[j][0] <= 1
			}
			if !ok {
				t.Errorf("x%g %s: ink rows %v, Chromium's %v", s, c.name, got, want)
			}
		}
	}
}
