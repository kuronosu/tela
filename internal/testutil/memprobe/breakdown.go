package memprobe

import (
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Rule puts the allocations whose stack has a frame with one of Prefixes in
// Category. Prefixes are compared with the start of the frame's function
// name after the module path "github.com/kuronosu/tela/" is removed
// (so "internal/gfx/raster.(*Renderer).getBuf" and "telatest." work; a
// third-party name keeps its full path).
type Rule struct {
	Category string
	Prefixes []string
}

// modulePrefix is removed from function names before matching.
const modulePrefix = "github.com/kuronosu/tela/"

// Categories, in the order of the report.
const (
	CatFonts       = "fonts"
	CatGlyphCache  = "glyph cache"
	CatFramebuffer = "framebuffers and present copies"
	CatRasterBuf   = "raster scratch"
	CatBackdrop    = "backdrop cache"
	CatDisplayList = "display lists"
	CatImages      = "images"
	CatTextLayout  = "text layout"
	CatTrees       = "trees"
	CatOther       = "other"
)

// Categories lists the categories in report order.
var Categories = []string{CatFonts, CatGlyphCache, CatFramebuffer, CatRasterBuf, CatBackdrop, CatDisplayList, CatImages, CatTextLayout, CatTrees, CatOther}

// Classify returns the category of an allocation from the function names of
// its stack. The rules are tried in table order and the first one that
// matches any frame of the stack decides, so a specific rule (the backdrop
// cache, the glyph cache) goes before the general ones (the rasterizer, the
// trees) and an image.NewRGBA called by the rasterizer is raster scratch
// while the same call from the host is a framebuffer. Frames no rule names
// (the runtime, the standard library, the testing package, the test
// harness) decide nothing. Allocations no rule matches are "other".
func Classify(rules []Rule, funcs []string) string {
	for _, r := range rules {
		for _, fn := range funcs {
			fn = strings.TrimPrefix(fn, modulePrefix)
			for _, p := range r.Prefixes {
				if strings.HasPrefix(fn, p) {
					return r.Category
				}
			}
		}
	}
	return CatOther
}

// Category is the live heap of one category.
type Category struct {
	Name    string `json:"name"`
	Bytes   uint64 `json:"bytes"`
	Objects int64  `json:"objects"`
}

// StackBytes is one allocation site of the live heap.
type StackBytes struct {
	Category string   `json:"category"`
	Bytes    uint64   `json:"bytes"`
	Objects  int64    `json:"objects"`
	Frames   []string `json:"frames"` // innermost first, function names with file:line, trimmed
}

// BreakdownResult is the live heap split by category.
type BreakdownResult struct {
	Rate       int          `json:"rate"`       // runtime.MemProfileRate when taken
	Total      uint64       `json:"total"`      // bytes in all records
	Categories []Category   `json:"categories"` // in report order
	Top        []StackBytes `json:"top"`        // the largest allocation sites
}

// topStacks is how many allocation sites a result lists (TELA_MEM_TOP
// changes it, to design the rules).
var topStacks = func() int {
	if n, err := strconv.Atoi(os.Getenv("TELA_MEM_TOP")); err == nil && n > 0 {
		return n
	}
	return 40
}()

// maxFrames is how many frames of a site a result keeps.
const maxFrames = 12

// Breakdown splits the live heap by category using rules. It needs
// runtime.MemProfileRate to be 1 before the allocations happen; otherwise
// the records are samples and the numbers are estimates.
func Breakdown(rules []Rule) BreakdownResult {
	runtime.GC() // the profile is as of the last completed GC
	records := liveRecords()
	return breakdownOf(rules, records)
}

// record is a heap profile record with the stack symbolized.
type record struct {
	bytes   uint64
	objects int64
	funcs   []string // function names, innermost first
	where   []string // function names with file:line
}

func liveRecords() []record {
	// MemProfile fails when the slice is too small, and the profile grows
	// while it is read: ask again with room to spare.
	n, _ := runtime.MemProfile(nil, true)
	var p []runtime.MemProfileRecord
	for {
		p = make([]runtime.MemProfileRecord, n+50)
		var ok bool
		if n, ok = runtime.MemProfile(p, true); ok {
			p = p[:n]
			break
		}
	}
	out := make([]record, 0, len(p))
	for i := range p {
		r := &p[i]
		b := r.InUseBytes()
		if b <= 0 {
			continue
		}
		rec := record{bytes: uint64(b), objects: r.InUseObjects()}
		frames := runtime.CallersFrames(r.Stack())
		for {
			f, more := frames.Next()
			if f.Function != "" {
				rec.funcs = append(rec.funcs, f.Function)
				rec.where = append(rec.where, shorten(f.Function)+" "+shortFile(f.File)+":"+strconv.Itoa(f.Line))
			}
			if !more {
				break
			}
		}
		out = append(out, rec)
	}
	return out
}

func shorten(fn string) string { return strings.TrimPrefix(fn, modulePrefix) }

// shortFile keeps the last two elements of a path.
func shortFile(f string) string {
	i := strings.LastIndexByte(f, '/')
	if i < 0 {
		return f
	}
	j := strings.LastIndexByte(f[:i], '/')
	return f[j+1:]
}

// breakdownOf groups records; it is separate from the runtime for tests.
func breakdownOf(rules []Rule, records []record) BreakdownResult {
	res := BreakdownResult{Rate: runtime.MemProfileRate}
	byCat := map[string]*Category{}
	for _, c := range Categories {
		byCat[c] = &Category{Name: c}
	}
	var sites []StackBytes
	for _, r := range records {
		cat := Classify(rules, r.funcs)
		c := byCat[cat]
		if c == nil {
			c = &Category{Name: cat}
			byCat[cat] = c
		}
		c.Bytes += r.bytes
		c.Objects += r.objects
		res.Total += r.bytes
		frames := r.where
		if len(frames) > maxFrames {
			frames = frames[:maxFrames]
		}
		sites = append(sites, StackBytes{Category: cat, Bytes: r.bytes, Objects: r.objects, Frames: frames})
	}
	for _, c := range Categories {
		res.Categories = append(res.Categories, *byCat[c])
	}
	slices.SortFunc(sites, func(a, b StackBytes) int {
		switch {
		case a.Bytes > b.Bytes:
			return -1
		case a.Bytes < b.Bytes:
			return 1
		}
		return 0
	})
	res.Top = sites[:min(topStacks, len(sites))]
	return res
}
