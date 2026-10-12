package style

import (
	"strings"
	"testing"
)

// TestNoPanicOnMalformedClasses feeds malformed classes to the parser: it
// must return errors, never panic.
func TestNoPanicOnMalformedClasses(t *testing.T) {
	nasty := []string{
		"", " ", "-", "--", "!", ":", "::", "a:", ":a", "hover:", "hover::flex", "[", "]", "[]", "()", "/", "//",
		"w-[", "w-]", "w-[]", "w-/", "w-1/", "w-/2", "w-1/0", "w-0/0", "w-[/]", "bg-[]", "bg-[", "bg-[#]", "bg-white/",
		"bg-white/[", "bg-white/[]", "bg-white/[abc]", "bg-white/-5", "text-[]", "text-[:]", "text-[length:]", "text-[color:]",
		"p-[calc(]", "p-[calc()]", "p-[)]", "p-[(]", "p-[calc(1px+)]", "p-[calc(+)]", "p-[min()]", "p-[min(,)]", "p-[clamp(1px,2px,)]",
		"grid-cols-[repeat(]", "grid-cols-[repeat()]", "grid-cols-[repeat(1)]", "grid-cols-[minmax(]", "grid-cols-[minmax()]",
		"grid-cols-[fit-content()]", "grid-cols-[,]", "col-[/]", "col-[span]", "col-[span_0]", "col-[1/2/3]",
		"bg-[url(]", "bg-[url()]", "bg-[linear-gradient(]", "bg-[linear-gradient()]", "bg-[linear-gradient(,)]",
		"bg-linear-[]", "bg-linear-[,]", "bg-linear-[to]", "bg-linear-[to_]", "bg-radial-[at]", "bg-radial-[at_]", "bg-radial-[circle_]",
		"bg-conic-[from]", "bg-conic-[at]", "bg-conic-[from_at]", "bg-linear-to-r/", "bg-linear-to-r/[", "from-[]", "via-[%]", "to-[,]",
		"shadow-[,]", "shadow-[]", "shadow-[1px]", "shadow-[inset]", "shadow-[1px_2px_3px_4px_5px]", "ring-[]", "outline-[]", "border-[]",
		"rounded-[]", "rounded-[x]", "blur-[]", "opacity-[]", "z-[]", "z-[x]", "order-[]", "flex-[]", "flex-[x]", "flex-[1_2_3_4]",
		"aspect-[]", "aspect-[/]", "aspect-[0/0]", "font-[]", "font-['']", "font-[,]", "leading-[]", "tracking-[]", "line-clamp-[]",
		"duration-[]", "ease-[]", "ease-[cubic-bezier()]", "ease-[cubic-bezier(1,2,3)]", "animate-[]", "origin-[]", "origin-[a_b_c]",
		"translate-x-[]", "rotate-[]", "scale-[]", "skew-x-[]", "transition-[]", "transition-[x]", "decoration-[]", "underline-offset-[]",
		"-", "-w-4", "--w-4", "-flex", "-bg-white", "w-4/5/6", "group-hover/", "group-hover/x:flex", "peer-/:flex",
		"\x00", "w-\x00", "bg-[é]", "w-[‮1px]", strings.Repeat("a", 5000), strings.Repeat("w-[", 200),
	}
	p := NewParser(DefaultTheme())
	for _, c := range nasty {
		cl := p.Compile(c)
		_ = Compute(testCx, Input{Classes: []*Classes{cl}})
		_ = p.Suggest(c)
	}
}

// TestNoPanicOnVocabularyMutations mutates every vocabulary class.
func TestNoPanicOnVocabularyMutations(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	p := NewParser(DefaultTheme())
	suffixes := []string{"/", "/50", "/[", "-[]", "-[x]", "-", "-1/2", "-[1px]", "-[#fff]", "-[calc(1px+2px)]", "!"}
	for i, c := range p.Vocabulary() {
		// Every error pays for a suggestion over the whole vocabulary, so
		// sample the vocabulary to keep the test fast.
		if i%40 != 0 {
			continue
		}
		for _, s := range suffixes {
			_ = p.Compile(c + s)
		}
		_ = p.Compile("hover:" + c)
		_ = p.Compile("-" + c)
	}
}

// TestComputeFiniteValues checks that no class of the vocabulary produces
// NaN or infinite lengths.
func TestComputeFiniteValues(t *testing.T) {
	p := NewParser(DefaultTheme())
	for _, c := range p.Vocabulary() {
		cl := p.Compile(c)
		if cl.Err != nil {
			continue
		}
		s := Compute(testCx, Input{Classes: []*Classes{cl}})
		for _, v := range []float32{s.Width.Px, s.Height.Px, s.Padding.Top.Px, s.Margin.Left.Px, s.Text.FontSize, s.Text.LetterSpacing, s.Opacity, s.Transform.ScaleX} {
			if v != v || v > 1e7 || v < -1e7 {
				t.Errorf("%s: non-finite or huge value %v", c, v)
				break
			}
		}
	}
}
