package style

import (
	"errors"
	"strings"
	"testing"
)

func declString(d Description) string {
	parts := make([]string, len(d.Declarations))
	for i, x := range d.Declarations {
		parts[i] = x.String()
	}
	return strings.Join(parts, "; ")
}

func TestDescribe(t *testing.T) {
	p := DefaultParser()
	tests := []struct{ class, want string }{
		{"p-4", "padding: 1rem"},
		{"px-2.5", "padding-inline: 0.625rem"},
		{"-mt-2", "margin-top: -0.5rem"},
		{"m-auto", "margin: auto"},
		{"p-px", "padding: 1px"},
		{"p-[10px]", "padding: 10px"},
		{"w-1/2", "width: 50%"},
		{"h-screen", "height: 100vh"},
		{"max-w-xl", "max-width: 36rem"},
		{"size-8", "width: 2rem; height: 2rem"},
		{"bg-red-500", "background-color: #fb2c36"},
		{"bg-red-500/50", "background-color: #fb2c3680"},
		{"bg-[#1e1e2e]", "background-color: #1e1e2e"},
		{"text-red-500", "color: #fb2c36"},
		{"text-white", "color: #ffffff"},
		{"text-sm", "font-size: 0.875rem; line-height: 1.429"},
		{"text-sm/6", "font-size: 0.875rem; line-height: 1.5rem"},
		{"text-[14px]", "font-size: 14px"},
		{"text-center", "text-align: center"},
		{"font-bold", "font-weight: 700"},
		{"rounded", "border-radius: 0.25rem"},
		{"rounded-t-xl", "border-top-left-radius: 0.75rem; border-top-right-radius: 0.75rem"},
		{"shadow-md", "box-shadow: 0 4px 6px -1px #0000001a, 0 2px 4px -2px #0000001a"},
		{"border", "border-width: 1px"},
		{"border-t-4", "border-top-width: 4px"},
		{"border-red-500", "border-color: #fb2c36"},
		{"flex", "display: flex"},
		{"flex-1", "flex: 1 1 0%"},
		{"flex-col", "flex-direction: column"},
		{"items-center", "align-items: center"},
		{"justify-between", "justify-content: space-between"},
		{"grid-cols-3", "grid-template-columns: repeat(3, minmax(0, 1fr))"},
		{"gap-x-2", "column-gap: 0.5rem"},
		{"opacity-50", "opacity: 0.5"},
		{"-translate-y-2", "transform: translateY(-0.5rem)"},
		{"scale-95", "transform: scale(0.95)"},
		{"duration-300", "transition-duration: 300ms"},
		{"ease-in-out", "transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1)"},
		{"animate-spin", "animation: spin 1s linear infinite"},
		{"cursor-pointer", "cursor: pointer"},
		{"blur-sm", "filter: blur(8px)"},
		{"bg-linear-to-r", "background-image: linear-gradient(to right, var(--tw-gradient-stops))"},
		{"truncate", "overflow: hidden; text-overflow: ellipsis; white-space: nowrap"},
		{"inset-0", "inset: 0"},
		{"top-1/2", "top: 50%"},

		// Masks (Tailwind's composition of custom properties).
		{"mask-add", "mask-composite: add"},
		{"mask-match", "mask-mode: match-source"},
		{"mask-none", "mask-image: none"},
		{"mask-cover", "mask-size: cover"},
		{"mask-bottom-right", "mask-position: bottom right"},
		{"mask-no-repeat", "mask-repeat: no-repeat"},
		{"mask-clip-fill", "mask-clip: fill-box"},
		{"mask-origin-padding", "mask-origin: padding-box"},
		{"mask-type-alpha", "mask-type: alpha"},
		{"mask-[url(/a.png)]", "mask-image: url(/a.png)"},
		{"mask-t-from-50%", "--tw-mask-top-from-position: 50%; mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},
		{"mask-x-to-4", "--tw-mask-left-to-position: 1rem; --tw-mask-right-to-position: 1rem; mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},
		{"mask-b-from-slate-600", "--tw-mask-bottom-from-color: #45556c; mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},
		{"-mask-linear-45", "--tw-mask-linear: linear-gradient(-45deg, var(--tw-mask-linear-stops)); mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},
		{"mask-conic-90", "--tw-mask-conic: conic-gradient(from 90deg, var(--tw-mask-conic-stops)); mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},
		{"mask-radial-at-center", "--tw-mask-radial-position: center; mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},
		{"mask-circle", "--tw-mask-radial-shape: circle; mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic); mask-composite: intersect"},

		// Backgrounds, sizes and the other keywords.
		{"bg-conic-45", "background-image: conic-gradient(from 45deg in oklab, var(--tw-gradient-stops))"},
		{"-bg-conic-45", "background-image: conic-gradient(from -45deg in oklab, var(--tw-gradient-stops))"},
		{"bg-left-bottom", "background-position: left bottom"},
		{"bg-position-top-right", "background-position: top right"},
		{"bg-blend-multiply", "background-blend-mode: multiply"},
		{"bg-clip-padding", "background-clip: padding-box"},
		{"bg-origin-content", "background-origin: content-box"},
		{"bg-repeat-round", "background-repeat: round"},
		{"container", "width: 100%; max-width: 40rem /* from sm up */; max-width: 48rem /* from md up */; max-width: 64rem /* from lg up */; max-width: 80rem /* from xl up */; max-width: 96rem /* from 2xl up */"},
		{"size-2xl", "width: 42rem; height: 42rem"},
		{"w-screen-md", "width: 48rem"},
		{"w-prose", "width: 65ch"},
		{"object-bottom-left", "object-position: bottom left"},
		{"object-cover", "object-fit: cover"},
		{"origin-left-top", "transform-origin: left top"},
		{"font-stretch-condensed", "font-stretch: 75%"},
		{"via-50%", "--tw-gradient-via-position: 50%"},
		{"repeat-3", "animation-iteration-count: 3"},
		{"repeat-infinite", "animation-iteration-count: infinite"},
		{"direction-alternate-reverse", "animation-direction: alternate-reverse"},
		{"fill-mode-both", "animation-fill-mode: both"},
		{"animation-duration-300", "animation-duration: 300ms"},
		{"-animation-delay-100", "animation-delay: -100ms"},
		{"-backdrop-hue-rotate-30", "backdrop-filter: hue-rotate(-30deg)"},
		{"auto-cols-fr", "grid-auto-columns: minmax(0, 1fr)"},
		{"overscroll-x-contain", "overscroll-behavior-x: contain"},
		{"divide-dashed", "border-style: dashed /* between the children */"},
		{"divide-x-reverse", "--tw-divide-x-reverse: 1 /* between the children */"},
		{"outline-hidden", "outline-style: none"},
		{"leading-none", "line-height: 1"},
		{"scroll-smooth", "scroll-behavior: smooth"},
		{"wrap-anywhere", "overflow-wrap: anywhere"},
		{"field-sizing-content", "field-sizing: content"},
		{"justify-baseline", "justify-content: baseline"},
		{"mix-blend-plus-lighter", "mix-blend-mode: plus-lighter"},
	}
	for _, tc := range tests {
		d, err := p.Describe(tc.class)
		if err != nil {
			t.Errorf("%s: %v", tc.class, err)
			continue
		}
		if got := declString(d); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.class, got, tc.want)
		}
		if d.Utility != "" {
			t.Errorf("%s: Utility %q set although the class is described", tc.class, d.Utility)
		}
	}
}

func TestDescribeTokenColor(t *testing.T) {
	d, err := DefaultParser().Describe("bg-primary")
	if err != nil {
		t.Fatal(err)
	}
	got := declString(d)
	for _, want := range []string{"background-color: #", "in light", "in dark", "primary"} {
		if !strings.Contains(got, want) {
			t.Errorf("bg-primary = %q, want it to contain %q", got, want)
		}
	}
}

func TestDescribeVariants(t *testing.T) {
	d, err := DefaultParser().Describe("md:dark:hover:p-4")
	if err != nil {
		t.Fatal(err)
	}
	if got := declString(d); got != "padding: 1rem" {
		t.Errorf("declarations = %q", got)
	}
	want := []string{"when the window is at least 48rem wide (md)", "in dark mode", "on hover"}
	if strings.Join(d.Variants, "|") != strings.Join(want, "|") {
		t.Errorf("variants = %q, want %q", d.Variants, want)
	}
	for v, want := range map[string]string{
		"max-md":          "narrower than 48rem",
		"group-hover":     "group ancestor",
		"aria-expanded":   "aria-expanded is true",
		"min-[600px]":     "at least 600px",
		"motion-reduce":   "reduced motion",
		"peer-focus":      "preceding peer",
		"aria-[sort=asc]": "aria-sort=asc",
	} {
		if got := describeVariant(DefaultTheme(), v); !strings.Contains(got, want) {
			t.Errorf("describeVariant(%q) = %q, want it to contain %q", v, got, want)
		}
	}
}

func TestDescribeAlias(t *testing.T) {
	p, err := NewParserAliases(DefaultTheme(), map[string]string{
		"btn":     "px-4 py-2 rounded-md",
		"btn-big": "btn text-lg",
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.Describe("hover:btn-big")
	if err != nil {
		t.Fatal(err)
	}
	if d.Expansion != "btn text-lg" || len(d.Parts) != 2 {
		t.Fatalf("expansion %q with %d parts, want \"btn text-lg\" with 2", d.Expansion, len(d.Parts))
	}
	if d.Parts[0].Class != "btn" || d.Parts[0].Expansion != "px-4 py-2 rounded-md" || len(d.Parts[0].Parts) != 3 {
		t.Errorf("nested alias not described: %+v", d.Parts[0])
	}
	if got := declString(d.Parts[1]); !strings.HasPrefix(got, "font-size: 1.125rem") {
		t.Errorf("text-lg part = %q", got)
	}
	if len(d.Variants) != 1 {
		t.Errorf("variants = %q", d.Variants)
	}
}

func TestDescribeFallbackAndErrors(t *testing.T) {
	p := DefaultParser()
	// Valid, with no CSS equivalent: described by its utility.
	d, err := p.Describe("scrollbar-overlay")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Declarations) != 0 || d.Utility != "scrollbar" {
		t.Errorf("scrollbar-overlay: declarations %v, utility %q; want none and \"scrollbar\"", d.Declarations, d.Utility)
	}
	// Markers have no CSS.
	if d, _ := p.Describe("group"); len(d.Declarations) != 0 || d.Utility != "group" {
		t.Errorf("group: %+v", d)
	}
	// An unknown class is an error with its suggestion.
	_, err = p.Describe("bg-rd-500")
	var unknown *UnknownClassError
	if !errors.As(err, &unknown) {
		t.Fatalf("bg-rd-500: err = %v, want an UnknownClassError", err)
	}
}

// Every class of the vocabulary that the tables describe must give a value
// without placeholders from a failed lookup, and Describe must not panic on
// any of them.
func TestDescribeVocabulary(t *testing.T) {
	p := DefaultParser()
	described := 0
	vocab := p.Vocabulary()
	for _, class := range vocab {
		d, err := p.Describe(class)
		if err != nil {
			continue // a vocabulary entry the parser needs a value for
		}
		if len(d.Declarations) > 0 {
			described++
		}
		for _, x := range d.Declarations {
			if x.Property == "" || x.Value == "" || strings.Contains(x.Value, "NaN") || strings.Contains(x.Value, "%!") {
				t.Errorf("%s: bad declaration %q", class, x)
			}
		}
	}
	if described < len(vocab)/2 {
		t.Errorf("the tables describe %d of %d vocabulary classes; they should cover most of it", described, len(vocab))
	}
}

// noCSSEquivalent are the classes of the vocabulary that Describe may show
// by their utility name only, each because CSS has nothing for them.
var noCSSEquivalent = map[string]string{
	"group":             "a marker that variants such as group-hover refer to",
	"peer":              "a marker that variants such as peer-focus refer to",
	"scrollbar-classic": "Tela's own: the look of the platform's scrollbars has no CSS property",
	"scrollbar-overlay": "Tela's own: the look of the platform's scrollbars has no CSS property",
}

// Every class of the vocabulary is described in CSS: none falls back to
// its utility name, but the few with no CSS equivalent.
func TestDescribeCoversVocabulary(t *testing.T) {
	p := DefaultParser()
	var missing []string
	for _, class := range p.Vocabulary() {
		d, err := p.Describe(class)
		if err != nil {
			t.Errorf("%s: %v", class, err)
			continue
		}
		if len(d.Declarations) > 0 || d.Expansion != "" {
			if _, ok := noCSSEquivalent[class]; ok {
				t.Errorf("%s is in the exceptions but has a description; remove it from the list", class)
			}
			continue
		}
		if _, ok := noCSSEquivalent[class]; !ok {
			missing = append(missing, class)
		}
	}
	if len(missing) > 0 {
		shown := missing
		if len(shown) > 40 {
			shown = shown[:40]
		}
		t.Errorf("%d classes of the vocabulary fall back to their utility name (first %d: %v); describe them in describe.go or, when CSS has nothing for them, list them in noCSSEquivalent", len(missing), len(shown), shown)
	}
}
