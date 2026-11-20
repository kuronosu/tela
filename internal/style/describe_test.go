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
	// Valid, but not in the tables: described by its utility.
	d, err := p.Describe("mask-t-from-50%")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Declarations) != 0 || d.Utility != "mask-t-from" {
		t.Errorf("mask-t-from-50%%: declarations %v, utility %q; want none and \"mask-t-from\"", d.Declarations, d.Utility)
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
