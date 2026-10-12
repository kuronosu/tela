package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func readTheme(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("tailwind/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// TestGeneratedFileIsUpToDate fails when theme_gen.go does not match what
// the generator produces from the vendored theme.css.
func TestGeneratedFileIsUpToDate(t *testing.T) {
	want, err := build(readTheme(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../theme_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("internal/style/theme_gen.go is stale: run \"go generate ./internal/style\" and commit the result")
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	src := readTheme(t)
	a, err := build(src)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		b, err := build(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatal("two runs of the generator gave different output")
		}
	}
	if !bytes.HasPrefix(a, []byte("// Code generated ")) || !bytes.Contains(a, []byte("DO NOT EDIT.")) {
		t.Error("generated file lacks the standard header")
	}
}

// TestUnknownVariableIsRejected checks that the generator rejects a variable
// it does not know instead of dropping it.
func TestUnknownVariableIsRejected(t *testing.T) {
	src := readTheme(t)
	bad := strings.Replace(src, "--spacing: 0.25rem;", "--spacing: 0.25rem;\n  --frobnicate-x: 3px;", 1)
	_, err := build(bad)
	if err == nil || !strings.Contains(err.Error(), "--frobnicate-x") {
		t.Errorf("build error = %v, want it to name --frobnicate-x", err)
	}
}

func TestBuildErrors(t *testing.T) {
	src := readTheme(t)
	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"unsupported unit", "--breakpoint-sm: 40rem;", "--breakpoint-sm: 40vw;", "unit"},
		{"bad color", "--color-white: #fff;", "--color-white: hsl(0 0% 100%);", "only oklch()"},
		{"new font", "--font-serif:", "--font-slab: x;\n  --font-serif:", "fontStacks"},
		{"duplicate", "--color-black: #000;", "--color-black: #000;\n  --color-black: #111;", "twice"},
		{"line height only", "--text-xs: 0.75rem;", "", "--text-xs--line-height has no --text-xs"},
		{"size only", "--text-xs--line-height: calc(1 / 0.75);", "", "no --text-xs--line-height"},
		{"shadow in em", "--shadow-2xs: 0 1px rgb(0 0 0 / 0.05);", "--shadow-2xs: 0 1em rgb(0 0 0 / 0.05);", "must be in px"},
		{"no spacing", "--spacing: 0.25rem;", "", "--spacing is missing"},
		{"stray rule", "--spacing: 0.25rem;", "--spacing: 0.25rem; }\n.x { color: red;", "unsupported top-level rule"},
	} {
		bad := strings.Replace(src, tc.from, tc.to, 1)
		if bad == src {
			t.Fatalf("%s: %q not found in theme.css", tc.name, tc.from)
		}
		_, err := build(bad)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: build error = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}

func TestParseColor(t *testing.T) {
	for _, tc := range []struct {
		in         string
		r, g, b, a float32
	}{
		{"#fff", 1, 1, 1, 1},
		{"#000", 0, 0, 0, 1},
		{"rgb(0 0 0 / 0.25)", 0, 0, 0, 0.25},
		{"rgb(255, 0, 255)", 1, 0, 1, 1},
		{"rgb(0 0 0 / 50%)", 0, 0, 0, 0.5},
		{"oklch(100% 0 none)", 1, 1, 1, 1},
		{"oklch(0% 0 0)", 0, 0, 0, 1},
	} {
		c, err := parseColor(tc.in)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if c.R != tc.r || c.G != tc.g || c.B != tc.b || c.A != tc.a {
			t.Errorf("%s = %+v", tc.in, c)
		}
	}
	for _, bad := range []string{"red", "oklch(50% 0.1)", "rgb(1 2)", "oklch(x 0 0)"} {
		if _, err := parseColor(bad); err == nil {
			t.Errorf("parseColor(%q) succeeded", bad)
		}
	}
}

func TestParseValues(t *testing.T) {
	check := func(got string, err error, want string) {
		t.Helper()
		if err != nil || got != want {
			t.Errorf("got %q, %v; want %q", got, err, want)
		}
	}
	got, err := lengthExpr("-0.025em")
	check(got, err, "Em(-0.025)")
	got, err = lengthExpr("0")
	check(got, err, "Px(0)")
	got, err = numberExpr("calc(1.25 / 0.875)")
	check(got, err, "1.4285715")
	got, err = numberExpr("16 / 9")
	check(got, err, "1.7777778")
	got, err = durationExpr("0.15s")
	check(got, err, "150 * time.Millisecond")
	got, err = shadowExpr("inset 0 2px 4px rgb(0 0 0 / 0.05), 0 1px #000")
	check(got, err, "{{OffsetY: 2, Blur: 4, Color: gfx.Color{R: 0, G: 0, B: 0, A: 0.05}, Inset: true}, {OffsetY: 1, Color: gfx.Color{R: 0, G: 0, B: 0, A: 1}}}")
	if _, err := lengthExpr("3"); err == nil {
		t.Error("a unitless nonzero length was accepted")
	}
	if _, err := easeExpr("ease-in"); err == nil {
		t.Error("a keyword easing was accepted")
	}
}
