package style

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSuggestions(t *testing.T) {
	tests := []struct{ in, want string }{
		{"rounded-xxl", "rounded-2xl"},
		{"bg-indgo-500", "bg-indigo-500"},
		{"jusitfy-center", "justify-center"},
		{"text-centre", "text-center"},
		{"hover:bg-indgo-500", "hover:bg-indigo-500"},
		{"dark:md:text-centre", "dark:md:text-center"},
		{"flex-colum", "flex-col"},
		{"itmes-center", "items-center"},
		{"font-bolld", "font-bold"},
		{"mx-auot", "mx-auto"},
		{"shadow-xxl", "shadow-2xl"},
		{"bg-fuschia-500", "bg-fuchsia-500"},
		{"text-gray-5000", "text-gray-500"},
		{"-tranlate-x-4", "-translate-x-4"},
		{"tracking-tigth", "tracking-tight"},
		{"over-flow-hidden", "overflow-hidden"},
		{"hiden", "hidden"},
		{"transtion", "transition"},
		{"cursor-poiner", "cursor-pointer"},
		{"w-ful", "w-full"},
		{"caret-pnik-500", "caret-pink-500"},
		{"field-sizing-contnet", "field-sizing-content"},
		{"placeholder-shwn:border-red-500", "placeholder-shown:"},
		{"read-onyl:bg-slate-100", "read-only:"},
		{"invalidd:text-red-600", "invalid:"},
		{"placeholdr:text-slate-400", "placeholder:"},
	}
	for _, tt := range tests {
		c := Compile(tt.in)
		var u *UnknownClassError
		if !errors.As(c.Err, &u) {
			t.Errorf("%q: Err = %v, want an UnknownClassError", tt.in, c.Err)
			continue
		}
		if u.Suggestion != tt.want {
			t.Errorf("%q: Suggestion = %q, want %q", tt.in, u.Suggestion, tt.want)
		}
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct{ in, want string }{
		{"rounded-xxl", `unknown class "rounded-xxl"; did you mean "rounded-2xl"?`},
		{"zzzzzzzz", `unknown class "zzzzzzzz"`},
	}
	for _, tt := range tests {
		if got := Compile(tt.in).Err.Error(); got != tt.want {
			t.Errorf("%q: %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestAllUnknownClassesReportedKnownApplied(t *testing.T) {
	c := Compile("flex rounded-xxl p-4 bg-indgo-500 jusitfy-center text-white")
	if len(c.Rules) != 3 {
		t.Errorf("Rules = %d, want the 3 valid classes", len(c.Rules))
	}
	var errs ClassErrors
	if !errors.As(c.Err, &errs) || len(errs) != 3 {
		t.Fatalf("Err = %v, want 3 errors", c.Err)
	}
	want := []string{"rounded-xxl", "bg-indgo-500", "jusitfy-center"}
	for i, e := range errs {
		var u *UnknownClassError
		if !errors.As(e, &u) || u.Class != want[i] {
			t.Errorf("error %d = %v, want unknown %q", i, e, want[i])
		}
	}
	for _, w := range []string{`"rounded-2xl"`, `"bg-indigo-500"`, `"justify-center"`} {
		if !strings.Contains(c.Err.Error(), w) {
			t.Errorf("message lacks %s: %v", w, c.Err)
		}
	}
	s := Compute(testCx, Input{Classes: []*Classes{c}})
	eq(t, "display", s.Display, DisplayFlex)
	eq(t, "padding", s.Padding.Top, PxLP(16))
	eq(t, "color", s.Text.Color.R, 1) // text-white still applied
}

func TestUnsupportedClasses(t *testing.T) {
	tests := []struct{ class, mention string }{
		{"!flex", "!important"},
		{"flex!", "!important"},
		{"hover:!p-4", "!important"},
		{"float-left", "flex or grid"},
		{"clear-both", "flex or grid"},
		{"table", "grid"},
		{"table-cell", "grid"},
		{"divide-x-2", "border"},
		{"space-x-reverse", "flex-row-reverse"},
		{"box-content", "border-box"},
		{"[mask-type:luminance]", "arbitrary properties"},
		{"content-['x']", "regular nodes"},
		{"before:flex", "pseudo-elements"},
		{"stroke-[10%]", "percent"},
		{"rotate-x-45", "2D"},
		{"w-(--my-width)", "CSS variables"},
		{"bg-[var(--x)]", "CSS variables"},
		{"bg-linear-to-r/oklch", "interpolation"},
		{"grid-cols-subgrid", "subgrid"},
		{"mix-blend-plus-darker", "multiply"},
		{"bg-fixed", "background-attachment"},
		{"scroll-mt-4", "scroll margins"},
		{"snap-x", "snapping"},
		{"resize", "resizable"},
		{"resize-y", "resizable"},
	}
	for _, tt := range tests {
		c := Compile(tt.class)
		var u *UnsupportedClassError
		if !errors.As(c.Err, &u) {
			t.Errorf("%q: Err = %v, want an UnsupportedClassError", tt.class, c.Err)
			continue
		}
		if !strings.Contains(u.Reason, tt.mention) {
			t.Errorf("%q: reason %q does not mention %q", tt.class, u.Reason, tt.mention)
		}
		if len(c.Rules) != 0 {
			t.Errorf("%q produced rules", tt.class)
		}
	}
}

// TestResizeNone checks that resize-none, which Tailwind users put on every
// Textarea, is accepted: Tela draws no resize grip.
func TestResizeNone(t *testing.T) {
	if c := Compile("resize-none rounded-md"); c.Err != nil {
		t.Errorf("resize-none: %v", c.Err)
	}
}

func TestInvalidValues(t *testing.T) {
	tests := []struct{ class, mention string }{
		{"w-[320]", "length"},
		{"w-[abc]", "length"},
		{"p-[10vmin]", "vmin"},
		{"w-[calc(1rem+)]", ""},
		{"w-[min(10%,20px)]", "percentage"},
		{"opacity-[x]", "number"},
		{"bg-[notacolor]", ""},
		{"text-[#12]", "hex"},
		{"duration-[abc]", "time"},
		{"ease-[nope]", "easing"},
		{"rotate-[abc]", "angle"},
		{"bg-white/150", "0 to 100"},
		{"bg-white/abc", "opacity"},
		{"grid-cols-[repeat(x,1fr)]", "repeat"},
		{"z-[1.5]", ""},
	}
	for _, tt := range tests {
		c := Compile(tt.class)
		if c.Err == nil {
			t.Errorf("%q compiled without error", tt.class)
			continue
		}
		var errs ClassErrors
		errors.As(c.Err, &errs)
		var inv *InvalidClassError
		var unk *UnknownClassError
		switch {
		case errors.As(c.Err, &inv):
			if !strings.Contains(strings.ToLower(inv.Reason), strings.ToLower(tt.mention)) {
				t.Errorf("%q: reason %q does not mention %q", tt.class, inv.Reason, tt.mention)
			}
		case errors.As(c.Err, &unk):
			// A value that fits no handler is an unknown class.
		default:
			t.Errorf("%q: unexpected error %T %v", tt.class, c.Err, c.Err)
		}
	}
}

func TestSuggestionSpeed(t *testing.T) {
	p := DefaultParser()
	p.Vocabulary() // build the vocabulary outside the timing
	start := time.Now()
	const n = 50
	for i := 0; i < n; i++ {
		if p.Suggest("hover:rounded-xxl") == "" {
			t.Fatal("no suggestion")
		}
	}
	if per := time.Since(start) / n; per > 50*time.Millisecond {
		t.Errorf("a suggestion takes %v over %d classes", per, len(p.Vocabulary()))
	}
}

func TestVocabulary(t *testing.T) {
	v := DefaultParser().Vocabulary()
	has := func(c string) bool {
		for _, x := range v {
			if x == c {
				return true
			}
		}
		return false
	}
	for _, c := range []string{"flex", "rounded", "rounded-2xl", "shadow", "shadow-2xl", "bg-indigo-500",
		"text-center", "text-5xl", "justify-between", "w-full", "p-4", "-mt-4", "bg-linear-to-br",
		"drop-shadow-lg", "backdrop-blur", "blur", "duration-300", "ease-in-out", "animate-spin", "-translate-x-1/2"} {
		if !has(c) {
			t.Errorf("vocabulary lacks %q", c)
		}
	}
	for _, c := range []string{"rounded-DEFAULT", "shadow-DEFAULT", "blur-DEFAULT", "drop-shadow-DEFAULT", "bg-DEFAULT"} {
		if has(c) {
			t.Errorf("vocabulary has %q", c)
		}
	}
	if len(v) < 3000 {
		t.Errorf("vocabulary has only %d classes", len(v))
	}
	// The vocabulary is sorted and free of duplicates.
	for i := 1; i < len(v); i++ {
		if v[i-1] >= v[i] {
			t.Fatalf("vocabulary not sorted at %d: %q, %q", i, v[i-1], v[i])
		}
	}
}

// Every class of the vocabulary must parse: the suggestions never name a
// class Tela cannot use (ADR-037).
func TestVocabularyParses(t *testing.T) {
	p := DefaultParser()
	var bad []string
	for _, c := range p.Vocabulary() {
		if cl := p.Compile(c); cl.Err != nil {
			bad = append(bad, c+": "+cl.Err.Error())
			if len(bad) > 25 {
				break
			}
		}
	}
	if len(bad) > 0 {
		t.Errorf("%d vocabulary classes fail to parse:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

func TestCustomThemeVocabulary(t *testing.T) {
	th := DefaultTheme()
	th.Colors["brand"] = th.Colors["red-500"]
	p := NewParser(th)
	c := p.Compile("bg-brand text-brand/50 border-brand")
	if c.Err != nil {
		t.Fatal(c.Err)
	}
	if err := p.Compile("bg-brnad").Err; err == nil || !strings.Contains(err.Error(), `"bg-brand"`) {
		t.Errorf("suggestion from a custom theme: %v", err)
	}
	// The default parser does not know the color.
	if Compile("bg-brand").Err == nil {
		t.Error("the default theme accepted bg-brand")
	}
}
