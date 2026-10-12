package style

import (
	"fmt"
	"regexp"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

func TestInterning(t *testing.T) {
	p := NewParser(DefaultTheme())
	a := p.Compile("flex p-4 hover:bg-red-500")
	b := p.Compile("flex p-4 hover:bg-red-500")
	if a != b {
		t.Error("equal strings compiled twice")
	}
	if c := p.Compile("flex  p-4 hover:bg-red-500"); c == a {
		t.Error("strings that differ in spacing share a result (the cache is keyed by the exact string)")
	}
	// Errors are interned too.
	e1, e2 := p.Compile("rounded-xxl"), p.Compile("rounded-xxl")
	if e1 != e2 || e1.Err == nil {
		t.Error("an invalid string was not interned with its error")
	}
	// The empty string is valid.
	if c := p.Compile(""); c.Err != nil || len(c.Rules) != 0 {
		t.Errorf("empty string: %+v", c)
	}
}

func TestInterningIsBounded(t *testing.T) {
	p := NewParser(DefaultTheme())
	p.limit = 8
	first := p.Compile("p-1")
	for i := 0; i < 100; i++ {
		p.Compile(fmt.Sprintf("w-[%dpx]", i))
	}
	p.mu.RLock()
	n := len(p.cur) + len(p.old)
	p.mu.RUnlock()
	if n > 16 {
		t.Errorf("the cache holds %d entries with a limit of 8", n)
	}
	// An evicted entry compiles again to an equivalent result.
	again := p.Compile("p-1")
	if again.Err != nil || len(again.Rules) != len(first.Rules) {
		t.Errorf("recompiled p-1 = %+v", again)
	}
	// The newest entries are still cached.
	last := p.Compile("w-[99px]")
	if p.Compile("w-[99px]") != last {
		t.Error("a recent entry was not kept")
	}
}

// A string in use stays cached however many other strings pass through:
// hits in the old generation move to the current one.
func TestInterningKeepsHotEntries(t *testing.T) {
	p := NewParser(DefaultTheme())
	p.limit = 8
	hot := p.Compile("flex gap-2")
	for i := 0; i < 100; i++ {
		p.Compile(fmt.Sprintf("w-[%dpx]", i))
		if got := p.Compile("flex gap-2"); got != hot {
			t.Fatalf("after %d other strings the hot entry was compiled again", i+1)
		}
	}
}

func TestConcurrentCompile(t *testing.T) {
	p := NewParser(DefaultTheme())
	classes := []string{
		counterClasses,
		"grid grid-cols-[200px_1fr] gap-4 dark:bg-slate-900 md:grid-cols-3",
		"text-sm/6 text-[14px] font-['Open_Sans'] truncate",
		"rounded-xxl p-4", // with an error
		"w-[calc(100%-2rem)] -translate-x-1/2 shadow-indigo-500/50 shadow-lg",
	}
	results := make([][]*Classes, 32)
	var wg sync.WaitGroup
	for g := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				for _, c := range classes {
					results[g] = append(results[g], p.Compile(c))
				}
			}
			// Also compute and suggest concurrently.
			cx := &Context{Viewport: gfx.Size{W: 1000, H: 600}}
			Compute(cx, Input{Classes: []*Classes{p.Compile(classes[0])}})
			p.Suggest("rounded-xxl")
		}()
	}
	wg.Wait()
	for g := range results {
		for i, r := range results[g] {
			if want := results[0][i%len(classes)]; r != want {
				t.Fatalf("goroutine %d result %d is not the shared *Classes", g, i)
			}
		}
	}
	for i, c := range classes {
		r := results[0][i]
		if (i == 3) != (r.Err != nil) {
			t.Errorf("%q: Err = %v", c, r.Err)
		}
	}
}

func TestComputeIsDeterministic(t *testing.T) {
	cl := mustCompile(t, counterClasses+" ring-2 ring-offset-2 text-sm/6 grid-cols-[1fr_2fr]")
	first := fmt.Sprintf("%+v", *Compute(testCx, Input{Classes: []*Classes{cl}}))
	for i := 0; i < 20; i++ {
		got := fmt.Sprintf("%+v", *Compute(testCx, Input{Classes: []*Classes{cl}}))
		// Gradient pointers differ between runs; compare the rest.
		if strip(got) != strip(first) {
			t.Fatalf("run %d differs", i)
		}
	}
}

var ptrRE = regexp.MustCompile(`0x[0-9a-f]+`)

func strip(s string) string { return ptrRE.ReplaceAllString(s, "PTR") }

func TestBridgeToText(t *testing.T) {
	s := style(t, "text-lg font-semibold italic tracking-wide leading-7 text-red-500 text-center uppercase underline decoration-2 "+
		"tabular-nums font-mono truncate line-clamp-2 break-words whitespace-pre-wrap")
	ts := s.TextStyle()
	if len(ts.Families) != 1 || ts.Families[0] != "JetBrains Mono" {
		t.Errorf("Families = %v", ts.Families)
	}
	eq(t, "Size", ts.Size, 18)
	eq(t, "Weight", ts.Weight, 600)
	eq(t, "Style", ts.Style, text.StyleItalic)
	if !near(ts.LetterSpacing, 0.025*18, 1e-5) {
		t.Errorf("LetterSpacing = %v", ts.LetterSpacing)
	}
	eq(t, "LineHeight", ts.LineHeight, text.LineHeightPixels(28))
	eq(t, "Paint", ts.Paint, gfx.Paint(DefaultTheme().Colors["red-500"]))
	eq(t, "Transform", ts.Transform, text.TransformUppercase)
	eq(t, "Numeric", ts.Numeric, text.NumericTabular)
	eq(t, "Decoration lines", ts.Decoration.Lines, text.DecorationUnderline)
	eq(t, "Decoration thickness", ts.Decoration.Thickness, 2)
	eq(t, "Decoration paint", ts.Decoration.Paint, gfx.Paint(DefaultTheme().Colors["red-500"]))

	ps := s.ParagraphStyle()
	eq(t, "Align", ps.Align, text.AlignCenter)
	eq(t, "WhiteSpace", ps.WhiteSpace, text.WhiteSpacePreWrap)
	eq(t, "OverflowWrap", ps.OverflowWrap, text.OverflowWrapBreakWord)
	eq(t, "LineClamp", ps.LineClamp, 2)
	eq(t, "TextOverflow", ps.TextOverflow, text.TextOverflowEllipsis)
	eq(t, "TabSize", ps.TabSize, 4)

	// A style without a decoration leaves the span's decoration empty.
	plain := style(t, "text-sm").TextStyle()
	eq(t, "no decoration", plain.Decoration.Lines, 0)
	if plain.Decoration.Paint != nil {
		t.Error("a decoration paint without lines")
	}
}
