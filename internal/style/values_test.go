package style

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// evalLen evaluates a length expression in a 16px/1280x800 environment with
// a 20px font.
func evalLen(t *testing.T, s string, allowPct bool) (LP, error) {
	t.Helper()
	e, err := parseLengthExpr(s, allowPct)
	if err != nil {
		return LP{}, err
	}
	ev := env{fontSize: 20, theme: DefaultTheme(), viewport: gfx.Size{W: 1280, H: 800}}
	return e.eval(&ev), nil
}

func TestLengthExpressions(t *testing.T) {
	tests := []struct {
		in   string
		want LP
	}{
		{"10px", PxLP(10)},
		{"-10px", PxLP(-10)},
		{"1.5rem", PxLP(24)},
		{"2em", PxLP(40)},
		{"50%", PctLP(50)},
		{"10vw", PxLP(128)},
		{"50vh", PxLP(400)},
		{"10dvh", PxLP(80)},
		{"0", PxLP(0)},
		{"65ch", PxLP(65 * 20 * ChPerEm)},
		{"calc(1rem+2px)", PxLP(18)},
		{"calc(1rem + 2px)", PxLP(18)},
		{"calc(100% - 2rem)", LP{Px: -32, Pct: 100}},
		{"calc(100%-2rem)", LP{Px: -32, Pct: 100}},
		{"calc(50% + 10px - 4px)", LP{Px: 6, Pct: 50}},
		{"calc(100% / 3)", PctLP(100.0 / 3)},
		{"calc(2 * 1rem)", PxLP(32)},
		{"calc(1rem * 2)", PxLP(32)},
		{"calc((1rem + 2px) * 2)", PxLP(36)},
		{"calc(10px - -2px)", PxLP(12)},
		{"min(10px, 20px)", PxLP(10)},
		{"max(10px, 2rem)", PxLP(32)},
		{"max(1rem, 5vw)", PxLP(64)},
		{"clamp(10px, 50vw, 100px)", PxLP(100)},
		{"clamp(2rem, 1vw, 100px)", PxLP(32)},
		{"calc(min(1rem, 10px) + 4px)", PxLP(14)},
		{"min(calc(1rem + 2px), 30px)", PxLP(18)},
	}
	for _, tt := range tests {
		got, err := evalLen(t, tt.in, true)
		if err != nil {
			t.Errorf("%s: %v", tt.in, err)
			continue
		}
		if !near(got.Px, tt.want.Px, 1e-3) || !near(got.Pct, tt.want.Pct, 1e-3) {
			t.Errorf("%s = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestLengthExpressionErrors(t *testing.T) {
	tests := []struct {
		in       string
		allowPct bool
		mention  string
	}{
		{"min(10%, 20px)", true, "percentage"},
		{"clamp(1rem, 50%, 40rem)", true, "percentage"},
		{"max(1rem, 2%)", true, "max()"},
		{"10%", false, "percentage"},
		{"calc(10% + 1px)", false, "percentage"},
		{"10cm", true, "cm"},
		{"5pt", true, "pt"},
		{"10vmin", true, "vmin"},
		{"calc(1rem + 2)", true, "number"},
		{"calc(1rem * 2px)", true, "multiply"},
		{"calc(1rem / 0)", true, "divided"},
		{"calc(1rem +)", true, ""},
		{"calc(1rem", true, ""},
		{"clamp(1rem, 2rem)", true, "three"},
		{"min(1rem)", true, "two"},
		{"var(--x)", true, ""},
		{"calc(var(--x) + 1px)", true, "variables"},
		{"sin(1px)", true, ""},
	}
	for _, tt := range tests {
		_, err := evalLen(t, tt.in, tt.allowPct)
		if err == nil {
			t.Errorf("%s: no error", tt.in)
			continue
		}
		if tt.mention != "" && !containsFold(err.Error(), tt.mention) {
			t.Errorf("%s: error %q does not mention %q", tt.in, err, tt.mention)
		}
	}
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 32
		}
		if y >= 'A' && y <= 'Z' {
			y += 32
		}
		if x != y {
			return false
		}
	}
	return true
}

// TestParseColorIsStrict: rgb() and hsl() follow CSS Color 4 (ADR-193), so
// the lax forms the class parser once took are errors with an actionable
// message.
func TestParseColorIsStrict(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"hsl(120,100,25)", "with commas takes percentages"},
		{"hsl(120, 100%, 25)", "with commas takes percentages"},
		{"rgb(1, 2 3)", "not both"},
		{"rgb(1 2 3, 0.5)", "not both"},
		{"rgba(1,2,3 / 0.5)", "not both"},
		{"rgb(1 2 3 /)", `rgb() takes exactly one alpha after the "/"`},
		{"hsl(1 2% 3% / 1 2)", `hsl() takes exactly one alpha after the "/"`},
		{"rgb(1, 2%, 3)", "not a mix"},
		{"rgb(1 2)", "rgb() takes three components"},
		{"hsl(1)", "hsl() takes hue"},
		{"hsl(x 1% 1%)", `invalid hue "x"`},
		{"rgb(1 2 z)", `invalid color component "z"`},
		{"rgb(1 2 3 / z)", `invalid color component "z"`},
		{"rgb(none, 0, 0)", `invalid color component "none"`},
	} {
		_, err := parseColor(tt.in)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("parseColor(%q) error = %v, want one with %q", tt.in, err, tt.want)
		}
	}
}

func TestParseColor(t *testing.T) {
	near3 := func(got, want gfx.Color) bool { return colorNear(got, want, 1e-4) }
	tests := []struct {
		in   string
		want gfx.Color
	}{
		{"#f00", gfx.Color{R: 1, A: 1}},
		{"#ff000080", gfx.RGBA8(255, 0, 0, 128)},
		{"rgb(255 0 0)", gfx.Color{R: 1, A: 1}},
		{"rgb(255, 0, 0)", gfx.Color{R: 1, A: 1}},
		{"rgba(255,0,0,0.5)", gfx.Color{R: 1, A: 0.5}},
		{"rgb(255 0 0 / 50%)", gfx.Color{R: 1, A: 0.5}},
		{"rgb(100% 0% 0%)", gfx.Color{R: 1, A: 1}},
		{"hsl(120 100% 50%)", gfx.Color{G: 1, A: 1}},
		{"hsl(120 100 50)", gfx.Color{G: 1, A: 1}},
		{"hsl(none 100% 50%)", gfx.Color{R: 1, A: 1}},
		{"rgb(none 0 0 / none)", gfx.Color{}},
		{"hsl(240deg, 100%, 50%)", gfx.Color{B: 1, A: 1}},
		{"hsla(0, 100%, 50%, 0.25)", gfx.Color{R: 1, A: 0.25}},
		{"oklch(0.637 0.237 25.331)", gfx.OKLCH(0.637, 0.237, 25.331, 1)},
		{"oklch(63.7% 0.237 25.331 / 0.5)", gfx.OKLCH(0.637, 0.237, 25.331, 0.5)},
		{"oklab(0.7 0.1 -0.1)", gfx.Oklab(0.7, 0.1, -0.1, 1)},
		{"transparent", gfx.Transparent},
	}
	for _, tt := range tests {
		got, err := parseColor(tt.in)
		if err != nil {
			t.Errorf("%s: %v", tt.in, err)
			continue
		}
		if !near3(got.c, tt.want) {
			t.Errorf("%s = %v, want %v", tt.in, got.c, tt.want)
		}
	}
	if c, err := parseColor("currentColor"); err != nil || !c.current {
		t.Errorf("currentColor = %+v, %v", c, err)
	}
	for _, bad := range []string{"#12", "rgb(1,2)", "rgb(a b c)", "hsl(1)", "oklch(1 2)", "color-mix(in srgb, red, blue)", "red", "var(--x)"} {
		if _, err := parseColor(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestParseGradientArgs(t *testing.T) {
	c := &pctx{th: DefaultTheme()}
	g, err := parseGradientList(c, GradientLinear, "to right, #000, #fff 20%, rgb(0 0 0 / 0.5) 40% 60%, indigo-500")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "angle", g.Angle, 90)
	if len(g.Stops) != 5 {
		t.Fatalf("Stops = %+v", g.Stops)
	}
	eq(t, "auto first", g.Stops[0].Pos.Set, false)
	eq(t, "second", g.Stops[1].Pos.Pct, 20)
	eq(t, "two positions: first", g.Stops[2].Pos.Pct, 40)
	eq(t, "two positions: second", g.Stops[3].Pos.Pct, 60)
	eq(t, "theme color", g.Stops[4].Color, DefaultTheme().Colors["indigo-500"])

	// No prelude: the first argument is a color stop.
	g, err = parseGradientList(c, GradientLinear, "#000,#fff")
	if err != nil || g.Angle != 180 || len(g.Stops) != 2 {
		t.Errorf("no prelude: %+v, %v", g, err)
	}
	for _, bad := range []string{"to sideways,#000", "45,#000", "#000 10% 20% 30%", "nonsense"} {
		if _, err := parseGradientList(c, GradientLinear, bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	// Radial and conic preludes.
	r, err := parseGradientList(c, GradientRadial, "ellipse farthest-side at 10% 20%,#000,#fff")
	if err != nil || r.Size != SizeFarthestSide || r.CenterX != PctLP(10) || r.CenterY != PctLP(20) {
		t.Errorf("radial: %+v, %v", r, err)
	}
	r, err = parseGradientList(c, GradientRadial, "circle 50px at bottom right,#000,#fff")
	if err != nil || r.Shape != ShapeCircle || r.Size != SizeExplicit || r.Radius[0] != PxLP(50) || r.CenterX != PctLP(100) {
		t.Errorf("radial circle: %+v, %v", r, err)
	}
	k, err := parseGradientList(c, GradientConic, "from 0.25turn at left,#000,#fff")
	if err != nil || k.Angle != 90 || k.CenterX != PctLP(0) || k.CenterY != PctLP(50) {
		t.Errorf("conic: %+v, %v", k, err)
	}
}

func TestParseTrackList(t *testing.T) {
	l, err := parseTrackList("repeat(2, 1fr 100px) minmax(10px, auto) fit-content(20rem) min-content", 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 4 || l.Entries[0].Count != 2 || len(l.Entries[0].Tracks) != 2 {
		t.Fatalf("list = %+v", l)
	}
	if n := len(l.Expand()); n != 4+3 {
		t.Errorf("Expand = %d tracks, want 7", n)
	}
	eq(t, "fit-content", l.Entries[2].Tracks[0].Max, TrackBreadth{Kind: BreadthFitContent, LP: PxLP(320)})
	named, err := parseTrackList("[a] 20px [b c] repeat(auto-fit, [d] 10px) [e]", 16)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "named", named.String(), "[a] 20px [b c] repeat(auto-fit, [d] 10px) [e]")
	for _, bad := range []string{"", "1fr [a", "[a] [b] 1fr", "repeat(0, 1fr)", "repeat(2)", "minmax(1fr, 2fr)", "3em", "abc",
		"calc(1px + 2vw)", "[1a] 1fr", "[auto] 1fr", "1fr repeat(auto-fit, 10px)"} {
		if _, err := parseTrackList(bad, 16); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
