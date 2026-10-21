package svg

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func mustParse(t testing.TB, src string) *Doc {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, src)
	}
	return d
}

// dump records src with opts and returns the list's Dump text.
func dump(t testing.TB, src string, opts Options) string {
	t.Helper()
	l := mustParse(t, src).Record(opts)
	if l == nil {
		return ""
	}
	return l.String()
}

func svgDoc(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="100" height="100">` + body + `</svg>`
}

func TestScanNumber(t *testing.T) {
	cases := []struct {
		in   string
		want []float64
	}{
		{"1.5.5", []float64{1.5, 0.5}},
		{"-1-2", []float64{-1, -2}},
		{"1e2", []float64{100}},
		{"1E-2+.5e1", []float64{0.01, 5}},
		{".5.5", []float64{0.5, 0.5}},
		{"5.", []float64{5}},
		{"+3", []float64{3}},
	}
	for _, c := range cases {
		var got []float64
		i := 0
		for i < len(c.in) {
			v, j, ok := scanNumber(c.in, i)
			if !ok {
				t.Fatalf("%q: no number at %d", c.in, i)
			}
			got = append(got, v)
			i = j
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%q: %v, want %v", c.in, got, c.want)
		}
	}
	// "1em": the number stops before the unit.
	if v, j, ok := scanNumber("1em", 0); !ok || v != 1 || j != 1 {
		t.Errorf("1em: %v %d %v", v, j, ok)
	}
	for _, bad := range []string{"", "-", ".", "e5", "1e999999", "2e9"} {
		if _, ok := parseNumber(bad); ok {
			t.Errorf("parseNumber(%q) accepted", bad)
		}
	}
}

func pathString(p gfx.Path) string {
	var sb strings.Builder
	for el := range p.Elements() {
		sb.WriteString(el.Verb.String())
		for i := 0; i < el.Verb.NumPoints(); i++ {
			fmt.Fprintf(&sb, " %g,%g", el.Pts[i].X, el.Pts[i].Y)
		}
		sb.WriteString("; ")
	}
	return strings.TrimSpace(sb.String())
}

func TestPathData(t *testing.T) {
	cases := []struct{ in, want string }{
		{"M10 10 L20 10 L20 20 Z", "MoveTo 10,10; LineTo 20,10; LineTo 20,20; Close;"},
		{"m10 10 20 0 0 10z", "MoveTo 10,10; LineTo 30,10; LineTo 30,20; Close;"},
		{"M10,10h5v5H0V0", "MoveTo 10,10; LineTo 15,10; LineTo 15,15; LineTo 0,15; LineTo 0,0;"},
		{"M0 0C1 1 2 2 3 3S5 5 6 6", "MoveTo 0,0; CubicTo 1,1 2,2 3,3; CubicTo 4,4 5,5 6,6;"},
		{"M0 0Q1 1 2 0T4 0t2 0", "MoveTo 0,0; QuadTo 1,1 2,0; QuadTo 3,-1 4,0; QuadTo 5,1 6,0;"},
		{"M0 0S1 1 2 0", "MoveTo 0,0; CubicTo 0,0 1,1 2,0;"},
		{"M0 0T2 0", "MoveTo 0,0; QuadTo 0,0 2,0;"},
		// After Z, drawing starts at the subpath's start.
		{"M5 5 L10 5 Z l5 0", "MoveTo 5,5; LineTo 10,5; Close; MoveTo 5,5; LineTo 10,5;"},
		// Errors keep what came before.
		{"M0 0 L10 10 L20", "MoveTo 0,0; LineTo 10,10;"},
		{"M0 0 L10 10 X 5 5", "MoveTo 0,0; LineTo 10,10;"},
		{"M0 0 L Z", "MoveTo 0,0;"},
		{"L10 10", ""},
		{"M0,0,L10,10", "MoveTo 0,0;"},
		{"M1.5.5", "MoveTo 1.5,0.5;"},
		// Arcs: zero radius is a line; equal end points draw nothing.
		{"M0 0A0 5 0 0 1 10 0", "MoveTo 0,0; LineTo 10,0;"},
		{"M0 0A5 5 0 0 1 0 0", "MoveTo 0,0;"},
		{"M0 0A5 5 0 2 1 10 0", "MoveTo 0,0;"},
	}
	for _, c := range cases {
		if got := pathString(parsePathData(c.in)); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}

func TestPathArcs(t *testing.T) {
	// Compact flags: "a10 10 0 1020 0" is large=1 sweep=0 to (+20, 0).
	a := parsePathData("M0 0 a10 10 0 1020 0")
	b := parsePathData("M0 0 a10 10 0 1 0 20 0")
	if pathString(a) != pathString(b) {
		t.Errorf("compact flags:\n%s\n%s", pathString(a), pathString(b))
	}
	// A half circle from (0,0) to (20,0) with sweep 1 goes through
	// (10,-10) (clockwise on screen from the left point is up).
	p := parsePathData("M0 0 A10 10 0 0 1 20 0")
	bb := p.Bounds()
	if math.Abs(float64(bb.Y0)+10) > 1e-3 || math.Abs(float64(bb.Y1)) > 1e-3 || math.Abs(float64(bb.X0)) > 1e-4 || bb.X1 != 20 {
		t.Errorf("half circle bounds %v", bb)
	}
	last, _ := p.CurrentPoint()
	if last != gfx.Pt(20, 0) {
		t.Errorf("arc end %v, want exactly (20,0)", last)
	}
	// Radii too small are scaled up: the same half circle.
	q := parsePathData("M0 0 A1 1 0 0 1 20 0")
	if pathString(q) != pathString(p) {
		t.Errorf("scaled radii:\n%s\n%s", pathString(q), pathString(p))
	}
	// A rotated elliptical arc ends exactly at its end point.
	r := parsePathData("M10 10 A20 10 30 1 1 30 20")
	if end, _ := r.CurrentPoint(); end != gfx.Pt(30, 20) || r.Len() < 3 {
		t.Errorf("rotated arc: %s", pathString(r))
	}
}

func TestEllipseAccuracy(t *testing.T) {
	p := ellipsePath(50, 40, 30, 20)
	var maxErr float64
	p.Flatten(0.0001, func(v gfx.Verb, pt gfx.Point) {
		if v == gfx.Close {
			return
		}
		x, y := (float64(pt.X)-50)/30, (float64(pt.Y)-40)/20
		maxErr = max(maxErr, math.Abs(math.Hypot(x, y)-1))
	})
	if maxErr > 2e-5 {
		t.Errorf("ellipse error %g of the radius", maxErr)
	}
	// The start point and direction (dashes depend on them).
	first := p.Points()[0]
	if first != gfx.Pt(80, 40) {
		t.Errorf("ellipse starts at %v, want (80,40)", first)
	}
	if p.Points()[3].Y <= 40 {
		t.Errorf("ellipse does not turn toward +y first: %v", p.Points()[3])
	}
}

func TestTransforms(t *testing.T) {
	cases := []struct {
		in   string
		want gfx.Affine
		ok   bool
	}{
		{"translate(10)", gfx.Translate(10, 0), true},
		{"translate(10, 20) scale(2)", gfx.Affine{2, 0, 0, 2, 10, 20}, true},
		{"scale(2 3)", gfx.Scale(2, 3), true},
		{"rotate(90)", gfx.Affine{0, 1, -1, 0, 0, 0}, true},
		{"rotate(-90 10 10)", gfx.Affine{0, -1, 1, 0, 0, 20}, true},
		{"rotate(180)", gfx.Affine{-1, 0, 0, -1, 0, 0}, true},
		{"matrix(1,2,3,4,5,6)", gfx.Affine{1, 2, 3, 4, 5, 6}, true},
		{"  translate(1,1)  ,  translate(1 1) ", gfx.Translate(2, 2), true},
		{"skewX(45)", gfx.Affine{1, 0, 1, 1, 0, 0}, true},
		{"translate(1,1) bogus(1)", gfx.Identity, false},
		{"rotate(1 2)", gfx.Identity, false},
		{"translate(1,1", gfx.Identity, false},
		{"Translate(1)", gfx.Identity, false},
	}
	for _, c := range cases {
		got, ok := parseTransform(c.in)
		if ok != c.ok {
			t.Errorf("%q: ok %v", c.in, ok)
			continue
		}
		for i := range got {
			if math.Abs(float64(got[i]-c.want[i])) > 1e-6 {
				t.Errorf("%q: %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestColors(t *testing.T) {
	if len(namedColors) != 148 {
		t.Errorf("%d named colors, want 148", len(namedColors))
	}
	cases := []struct {
		in   string
		want string // gfx.Color.String; "current"; "" for invalid
	}{
		{"red", "#ff0000"}, {"RebeccaPurple", "#663399"}, {"transparent", "#00000000"},
		{"currentColor", "current"}, {"CURRENTCOLOR", "current"},
		{"#abc", "#aabbcc"}, {"#abcd", "#aabbccdd"}, {"#a1b2c3", "#a1b2c3"}, {"#a1b2c380", "#a1b2c380"},
		{"rgb(255, 0, 0)", "#ff0000"}, {"rgba(0,0,255,.5)", "#0000ff80"}, {"rgb(0 128 255 / 50%)", "#0080ff80"},
		{"rgb(100%, 50%, 0%)", "#ff8000"}, {"rgb(300, -1, 0)", "#ff0000"}, {"RGB(1 2 3)", "#010203"},
		{"rgb(none 0 0)", "#000000"},
		{"hsl(120, 100%, 25%)", "#008000"}, {"hsla(0, 100%, 50%, 0.3)", "#ff00004d"},
		{"hsl(0.5turn 100% 50%)", "#00ffff"}, {"hsl(200deg 0% 50%)", "#808080"}, {"hsl(300 100 50)", "#ff00ff"},
		{"", ""}, {"notacolor", ""}, {"#ab", ""}, {"rgb(1,2)", ""}, {"rgb(1, 2%, 3)", ""}, {"rgb(1 2 3 / )", ""},
		{"hsl(120, 100, 25)", ""}, {"rgb(1,2,3", ""},
	}
	for _, c := range cases {
		v, ok := parseColor(c.in)
		got := ""
		switch {
		case !ok:
		case v.current:
			got = "current"
		default:
			got = v.c.String()
			if v.c.A >= 1 {
				got = v.c.String()
			}
		}
		if got != c.want {
			t.Errorf("parseColor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLengths(t *testing.T) {
	vp := viewport{200, 100}
	f := fonts{em: 10, rem: 20}
	cases := []struct {
		in   string
		ax   axis
		want float64
	}{
		{"5", axisX, 5}, {"5px", axisX, 5}, {"1in", axisX, 96}, {"2.54cm", axisX, 96}, {"25.4mm", axisX, 96},
		{"72pt", axisX, 96}, {"6pc", axisX, 96}, {"2em", axisX, 20}, {"2ex", axisX, 10}, {"2rem", axisX, 40},
		{"10%", axisX, 20}, {"10%", axisY, 10}, {"10%", axisDiag, 10 * math.Sqrt((200*200+100*100)/2) / 100},
		{"1E1PX", axisX, 10},
	}
	for _, c := range cases {
		l, ok := parseLength(c.in)
		if !ok {
			t.Errorf("%q rejected", c.in)
			continue
		}
		if got := float64(l.resolve(c.ax, vp, f)); math.Abs(got-c.want) > 1e-4 {
			t.Errorf("%q: %g, want %g", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"5 px", "5qq", "px", "5%%"} {
		if _, ok := parseLength(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCascade(t *testing.T) {
	src := svgDoc(`
		<g fill="#ff0000" stroke-width="3">
			<rect id="a" width="10" height="10" style="fill:#00ff00"/>
			<rect id="b" width="10" height="10" fill="#0000ff" style="fill: nonsense"/>
			<rect id="c" width="10" height="10" style="fill: inherit; stroke: #000; stroke-width: 2 !important"/>
			<rect id="d" width="10" height="10" fill="inherit" style="/* x */ stroke:#123456;FILL-OPACITY: 50%"/>
		</g>`)
	got := dump(t, src, Options{})
	for _, want := range []string{
		"FillPath path={M0,0 L10,0 L10,10 L0,10 Z} rule=nonzero paint=#00ff00",
		"paint=#0000ff",
		"paint=#ff0000 bounds",
		"StrokePath path={M0,0 L10,0 L10,10 L0,10 Z} width=2",
		"paint=#ff000080",
		"width=3 join=miter cap=butt paint=#123456",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

// TestInvalidStyleKeepsAttribute checks that an invalid declaration in the
// style attribute leaves the valid presentation attribute in force, for
// every kind of property.
func TestInvalidStyleKeepsAttribute(t *testing.T) {
	src := svgDoc(`<path d="M0 0 L10 0 L10 10 Z" fill="#ff0000" fill-opacity=".5" fill-rule="evenodd" stroke="#00ff00"
		stroke-width="3" stroke-opacity=".25" stroke-linecap="round" stroke-linejoin="bevel" stroke-miterlimit="7"
		stroke-dasharray="2 1" stroke-dashoffset="1" opacity=".5"
		style="fill-opacity: x; fill-rule: x; stroke-width: -1; stroke-opacity: x; stroke-linecap: x; stroke-linejoin: arcs;
		stroke-miterlimit: .5; stroke-dasharray: 1 -1; stroke-dashoffset: x; opacity: x; fill: x"/>`)
	got := dump(t, src, Options{})
	for _, want := range []string{
		"rule=evenodd paint=#ff000080", // fill-opacity .5; opacity .5 needs a layer
		"PushLayer opacity=0.5",
		"width=3 join=bevel cap=round miter=7 dashes=[2,1] dashOffset=1 paint=#00ff0040",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestDisplayVisibility(t *testing.T) {
	src := svgDoc(`
		<g display="none"><rect width="10" height="10" fill="#ff0000"/></g>
		<g visibility="hidden"><rect width="10" height="10" fill="#00ff00"/><rect width="20" height="20" fill="#0000ff" visibility="visible"/></g>
		<text>ignored</text><image href="x.png" width="10" height="10"/><foreignObject><div/></foreignObject>
		<unknown><rect width="10" height="10" fill="#ff0000"/></unknown>`)
	got := dump(t, src, Options{})
	if strings.Count(got, "FillPath") != 1 || !strings.Contains(got, "#0000ff") {
		t.Errorf("only the visible blue rect must be drawn:\n%s", got)
	}
}

func TestSizes(t *testing.T) {
	cases := []struct {
		attrs        string
		w, h, ratio  float32
		size         gfx.Size
		transformDst gfx.Rect
		want         gfx.Affine
	}{
		{`width="24" height="24" viewBox="0 0 24 24"`, 24, 24, 1, gfx.Size{W: 24, H: 24}, gfx.RectXYWH(0, 0, 48, 48), gfx.Scale(2, 2)},
		{`viewBox="0 0 20 10"`, 0, 0, 2, gfx.Size{W: 20, H: 10}, gfx.RectXYWH(0, 0, 40, 40), gfx.Affine{2, 0, 0, 2, 0, 10}},
		{`width="40" viewBox="0 0 20 10"`, 40, 0, 2, gfx.Size{W: 40, H: 20}, gfx.RectXYWH(10, 0, 40, 40), gfx.Affine{2, 0, 0, 2, 10, 10}},
		{`width="50%" height="2in"`, 0, 192, 0, gfx.Size{W: 300, H: 192}, gfx.RectXYWH(5, 5, 10, 10), gfx.Translate(5, 5)},
		{`width="50" height="25"`, 50, 25, 2, gfx.Size{W: 50, H: 25}, gfx.RectXYWH(0, 0, 100, 50), gfx.Scale(2, 2)},
		{``, 0, 0, 0, gfx.Size{W: 300, H: 150}, gfx.RectXYWH(3, 4, 10, 10), gfx.Translate(3, 4)},
		{`viewBox="0 0 20 10" preserveAspectRatio="xMaxYMin slice"`, 0, 0, 2, gfx.Size{W: 20, H: 10}, gfx.RectXYWH(0, 0, 40, 40), gfx.Affine{4, 0, 0, 4, -40, 0}},
		{`viewBox="0 0 20 10" preserveAspectRatio="none"`, 0, 0, 2, gfx.Size{W: 20, H: 10}, gfx.RectXYWH(0, 0, 40, 40), gfx.Scale(2, 4)},
		{`viewBox="10 10 20 10" preserveAspectRatio="xMinYMax meet"`, 0, 0, 2, gfx.Size{W: 20, H: 10}, gfx.RectXYWH(0, 0, 40, 40), gfx.Affine{2, 0, 0, 2, -20, 0}},
	}
	for _, c := range cases {
		d := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" `+c.attrs+`><rect width="1" height="1"/></svg>`)
		w, h, r := d.Intrinsic()
		if w != c.w || h != c.h || r != c.ratio {
			t.Errorf("%s: Intrinsic %g %g %g, want %g %g %g", c.attrs, w, h, r, c.w, c.h, c.ratio)
		}
		if s := d.Size(); s != c.size {
			t.Errorf("%s: Size %v, want %v", c.attrs, s, c.size)
		}
		if m := d.Transform(c.transformDst); m != c.want {
			t.Errorf("%s: Transform %v, want %v", c.attrs, m, c.want)
		}
	}
	// A zero-size viewBox disables rendering.
	d := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 10"><rect width="1" height="1"/></svg>`)
	b := gfx.NewBuilder()
	d.Draw(b, gfx.RectXYWH(0, 0, 10, 10), Options{})
	if b.Finish().Len() != 0 {
		t.Error("a zero-size viewBox drew something")
	}
}

func TestClipToViewport(t *testing.T) {
	for _, c := range []struct {
		attr string
		clip bool
	}{{"", true}, {` overflow="hidden"`, true}, {` overflow="visible"`, false}, {` style="overflow: auto"`, false}} {
		d := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"`+c.attr+`><rect width="20" height="20"/></svg>`)
		if d.Clips() != c.clip {
			t.Errorf("%q: Clips %v", c.attr, d.Clips())
		}
		b := gfx.NewBuilder()
		d.Draw(b, gfx.RectXYWH(0, 0, 10, 10), Options{})
		got := b.Finish().String()
		if strings.HasPrefix(got, "PushClipRect") != c.clip {
			t.Errorf("%q:\n%s", c.attr, got)
		}
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{
		``, `not xml`, `<html/>`, `<svg xmlns="http://www.w3.org/2000/svg"><rect></svg>`,
		`<x:svg xmlns:x="urn:other"/>`,
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q) succeeded", src)
		}
	}
	// No namespace is accepted (hand-written icons).
	mustParse(t, `<svg><rect width="1" height="1"/></svg>`)
}

func TestLimits(t *testing.T) {
	// Nesting.
	deep := strings.Repeat("<g>", maxDepth) + strings.Repeat("</g>", maxDepth)
	if _, err := Parse([]byte(svgDoc(deep))); !errors.Is(err, ErrTooComplex) {
		t.Errorf("deep nesting: %v", err)
	}
	// Element count.
	many := strings.Repeat("<g/>", maxElements)
	if _, err := Parse([]byte(svgDoc(many))); !errors.Is(err, ErrTooComplex) {
		t.Errorf("many elements: %v", err)
	}
	// Exponential use expansion ("billion laughs" with use).
	var sb strings.Builder
	sb.WriteString(`<defs><rect id="l0" width="1" height="1"/>`)
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, `<g id="l%d"><use href="#l%d"/><use href="#l%d"/><use href="#l%d"/></g>`, i, i-1, i-1, i-1)
	}
	sb.WriteString(`</defs><use href="#l20"/>`)
	if _, err := Parse([]byte(svgDoc(sb.String()))); !errors.Is(err, ErrTooComplex) {
		t.Errorf("use expansion: %v", err)
	}
	// Path points over all use instances.
	pts := strings.Repeat(" 1 1", 2000)
	sb.Reset()
	sb.WriteString(`<defs><polyline id="p0" points="0 0` + pts + `"/>`)
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&sb, `<g id="p%d"><use href="#p%d"/><use href="#p%d"/><use href="#p%d"/></g>`, i, i-1, i-1, i-1)
	}
	sb.WriteString(`</defs><use href="#p8"/>`)
	if _, err := Parse([]byte(svgDoc(sb.String()))); !errors.Is(err, ErrTooComplex) {
		t.Errorf("points: %v", err)
	}
	// Entities: too many, or too long.
	var ents strings.Builder
	ents.WriteString(`<!DOCTYPE svg [`)
	for i := 0; i <= maxEntities; i++ {
		fmt.Fprintf(&ents, `<!ENTITY e%d "x">`, i)
	}
	ents.WriteString(`]><svg/>`)
	if _, err := Parse([]byte(ents.String())); !errors.Is(err, ErrTooComplex) {
		t.Errorf("entities: %v", err)
	}
	long := `<!DOCTYPE svg [<!ENTITY e "` + strings.Repeat("x", maxEntityValue+1) + `">]><svg/>`
	if _, err := Parse([]byte(long)); !errors.Is(err, ErrTooComplex) {
		t.Errorf("long entity: %v", err)
	}
	// Huge numbers are invalid, not infinite.
	d := mustParse(t, svgDoc(`<rect width="1e30" height="10"/><path d="M0 0 L1e300 0 L5 5 Z"/><circle r="5" cx="1e10"/>`))
	b := gfx.NewBuilder()
	d.Draw(b, gfx.RectXYWH(0, 0, 100, 100), Options{})
	b.Finish()
}

func TestUseCycles(t *testing.T) {
	src := svgDoc(`
		<use id="self" href="#self"/>
		<g id="g1"><use href="#g1"/><rect width="5" height="5" fill="#ff0000"/></g>
		<use id="u1" href="#u2"/><use id="u2" href="#u1"/>
		<use href="#nothing"/><use href="http://example.com/x.svg#a"/>`)
	got := dump(t, src, Options{})
	if strings.Count(got, "FillPath") != 1 {
		t.Errorf("cycles must draw nothing but the plain rect:\n%s", got)
	}
}

func TestEntities(t *testing.T) {
	src := `<?xml version="1.0"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [
	<!ENTITY ns_svg "http://www.w3.org/2000/svg">
	<!ENTITY % param "ignored">
	<!ENTITY ext SYSTEM "file:///etc/passwd">
	<!ENTITY c "#123456">
]>
<svg xmlns="&ns_svg;" width="10" height="10"><rect width="10" height="10" fill="&c;"/></svg>`
	got := dump(t, src, Options{})
	if !strings.Contains(got, "paint=#123456") {
		t.Errorf("entity not expanded:\n%s", got)
	}
	// An external entity is not declared: using it is an error.
	if _, err := Parse([]byte(strings.Replace(src, "&c;", "&ext;", 1))); err == nil {
		t.Error("an external entity was expanded")
	}
}

func TestOverrides(t *testing.T) {
	src := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" color="#ff0000">
		<path d="M0 0L10 0L10 1"/>
		<path d="M0 5L10 5" stroke-width="4" stroke="#00ff00"/>
		<g color="#0000ff"><path d="M0 10L10 10"/></g>
		<rect width="4" height="4" fill="currentColor" stroke="none"/>
	</svg>`
	base := dump(t, src, Options{CurrentColor: gfx.Black})
	for _, want := range []string{"width=2 join=miter cap=butt paint=#ff0000", "width=4 join=miter cap=butt paint=#00ff00", "paint=#0000ff"} {
		if !strings.Contains(base, want) {
			t.Errorf("base: missing %q\n%s", want, base)
		}
	}
	// The root's color attribute beats the inherited CurrentColor; the
	// Root override beats both.
	ov := dump(t, src, Options{CurrentColor: gfx.Black, Root: Root{
		StrokeWidth: 1.5, HasStrokeWidth: true, Color: gfx.MustParseHex("#123456"), HasColor: true,
	}})
	for _, want := range []string{"width=1.5 join=miter cap=butt paint=#123456", "width=4 join=miter cap=butt paint=#00ff00",
		"width=1.5 join=miter cap=butt paint=#0000ff", "FillPath path={M0,0 L4,0 L4,4 L0,4 Z} rule=nonzero paint=#123456"} {
		if !strings.Contains(ov, want) {
			t.Errorf("overrides: missing %q\n%s", want, ov)
		}
	}
	// Fill and stroke overrides apply only where the paint is inherited
	// from the root.
	ov2 := dump(t, src, Options{Root: Root{Fill: Paint{Kind: PaintColor, Color: gfx.MustParseHex("#abcdef")}, Stroke: Paint{Kind: PaintNone}}})
	if strings.Count(ov2, "StrokePath") != 1 || !strings.Contains(ov2, "paint=#00ff00") {
		t.Errorf("stroke none must keep only the explicit stroke:\n%s", ov2)
	}
	if strings.Count(ov2, "#abcdef") != 1 || !strings.Contains(ov2, "M0,0 L10,0 L10,1} rule=nonzero paint=#abcdef") {
		t.Errorf("the fill override must fill the first path only (the rect has its own fill):\n%s", ov2)
	}
	// Without a root color, CurrentColor is used.
	src2 := strings.Replace(src, ` color="#ff0000"`, "", 1)
	if got := dump(t, src2, Options{CurrentColor: gfx.MustParseHex("#445566")}); !strings.Contains(got, "paint=#445566") {
		t.Errorf("CurrentColor not used:\n%s", got)
	}
	if d := mustParse(t, src2); !d.UsesCurrentColor() {
		t.Error("UsesCurrentColor is false")
	}
	if d := mustParse(t, svgDoc(`<rect width="1" height="1" fill="#000"/>`)); d.UsesCurrentColor() {
		t.Error("UsesCurrentColor is true")
	}
}

func TestOpacityFolding(t *testing.T) {
	got := dump(t, svgDoc(`<rect width="10" height="10" fill="#ff0000" opacity=".5"/>`), Options{})
	if strings.Contains(got, "PushLayer") || !strings.Contains(got, "paint=#ff000080") {
		t.Errorf("a single solid paint folds its opacity:\n%s", got)
	}
	got = dump(t, svgDoc(`<rect width="10" height="10" fill="#ff0000" stroke="#000" opacity=".5"/>`), Options{})
	if !strings.Contains(got, "PushLayer opacity=0.5") {
		t.Errorf("fill and stroke need a layer:\n%s", got)
	}
	got = dump(t, svgDoc(`<g opacity=".5"><rect width="10" height="10"/></g>`), Options{})
	if !strings.Contains(got, "PushLayer opacity=0.5") {
		t.Errorf("group opacity needs a layer:\n%s", got)
	}
	// An override that adds a stroke turns the folded shape into a layer.
	got = dump(t, svgDoc(`<rect width="10" height="10" fill="#ff0000" opacity=".5"/>`), Options{Root: Root{Stroke: Paint{Kind: PaintColor, Color: gfx.Black}}})
	if !strings.Contains(got, "PushLayer opacity=0.5") {
		t.Errorf("override stroke:\n%s", got)
	}
}

func TestPaintFallbacks(t *testing.T) {
	src := svgDoc(`
		<linearGradient id="empty"/>
		<linearGradient id="one"><stop offset=".3" stop-color="#112233" stop-opacity=".5"/></linearGradient>
		<pattern id="pat" width="2" height="2"><rect width="1" height="1"/></pattern>
		<linearGradient id="g"><stop stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
		<rect width="1" height="1" fill="url(#empty) #ff0000"/>
		<rect width="2" height="2" fill="url(#one)"/>
		<rect width="3" height="3" fill="url(#pat) #00ff00"/>
		<rect width="4" height="4" fill="url(#missing)"/>
		<rect width="5" height="5" fill="url(#missing) currentColor" color="#0000ff"/>
		<line x2="10" stroke="url(#g) #00ffff"/>
		<line x2="10" stroke="url(#g)"/>`)
	got := dump(t, src, Options{})
	for _, want := range []string{
		"paint=#11223380",
		"paint=#00ff00",
		"paint=#0000ff",
		"paint=#00ffff",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// No stops: none, even with a fallback; a missing reference without
	// one: none; a bounding-box gradient on a line: the fallback or none.
	if strings.Contains(got, "L1,1 L0,1") || strings.Contains(got, "L4,4 L0,4") || strings.Count(got, "StrokePath") != 1 {
		t.Errorf("unexpected paints:\n%s", got)
	}
}

func TestGradientResolution(t *testing.T) {
	src := svgDoc(`
		<linearGradient id="base" x1="10%" spreadMethod="reflect"><stop stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
		<linearGradient id="child" href="#base" x2="50%" gradientUnits="userSpaceOnUse"/>
		<radialGradient id="rad" href="#child" cx="20"/>
		<rect width="10" height="10" fill="url(#child)"/>
		<rect width="10" height="10" fill="url(#rad)"/>`)
	got := dump(t, src, Options{})
	// child: x1 = 10% of the 100-px viewport, x2 = 50%, userSpaceOnUse,
	// reflect and the stops of base.
	if !strings.Contains(got, "linear((10,0)->(50,0) srgb reflect [0:#000000 1:#ffffff])") {
		t.Errorf("linear inheritance:\n%s", got)
	}
	// rad: radial attributes only from radial gradients; units, spread and
	// stops from the chain.
	if !strings.Contains(got, "radial(c=(20,50) r=(") || !strings.Contains(got, "reflect") {
		t.Errorf("radial inheritance:\n%s", got)
	}
	// Cyclic href chains terminate.
	mustParse(t, svgDoc(`<linearGradient id="a" href="#b"/><linearGradient id="b" href="#a"/><rect width="1" height="1" fill="url(#a)"/>`))
}

func TestUnpremultipliedStops(t *testing.T) {
	tm := []stopTmpl{{off: 0, c: gfx.Color{R: 1, G: 1, B: 1, A: 1}}, {off: 1, c: gfx.Color{A: 0}}}
	stops := finishStops(tm, gfx.Black)
	if len(stops) < 3 {
		t.Fatalf("no intermediate stops: %v", stops)
	}
	// The premultiplied piecewise-linear ramp stays within half a level of
	// the unpremultiplied interpolation.
	for k := 0; k <= 1000; k++ {
		u := float64(k) / 1000
		want := (1 - u) * (1 - u) // premultiplied white channel: a(u)·c(u)
		var got float64
		for i := 0; i+1 < len(stops); i++ {
			s0, s1 := stops[i], stops[i+1]
			if u >= float64(s0.Offset) && u <= float64(s1.Offset) {
				v := (u - float64(s0.Offset)) / float64(s1.Offset-s0.Offset)
				p0, p1 := float64(s0.Color.R*s0.Color.A), float64(s1.Color.R*s1.Color.A)
				got = p0 + (p1-p0)*v
				break
			}
		}
		if math.Abs(got-want)*255 > 0.5 {
			t.Fatalf("u=%g: %g, want %g", u, got, want)
		}
	}
	// Equal alphas or equal colors need nothing.
	if n := len(finishStops([]stopTmpl{{off: 0, c: gfx.Color{R: 1, A: 1}}, {off: 1, c: gfx.Color{B: 1, A: 1}}}, gfx.Black)); n != 2 {
		t.Errorf("%d stops for equal alphas", n)
	}
}

func TestClipAndMaskReferences(t *testing.T) {
	src := svgDoc(`
		<clipPath id="one"><rect width="5" height="5"/></clipPath>
		<clipPath id="two"><rect width="5" height="5"/><circle r="3"/></clipPath>
		<clipPath id="empty"/>
		<mask id="m"><rect width="5" height="5" fill="#fff"/></mask>
		<mask id="m0"/>
		<rect id="a" width="10" height="10" clip-path="url(#one)" fill="#000001"/>
		<rect width="10" height="10" clip-path="url(#two)" fill="#000002"/>
		<rect width="10" height="10" clip-path="url(#empty)" fill="#000003"/>
		<rect width="10" height="10" clip-path="url(#missing)" fill="#000004"/>
		<rect width="10" height="10" mask="url(#m)" fill="#000005"/>
		<rect width="10" height="10" mask="url(#m0)" fill="#000006"/>
		<rect width="10" height="10" mask="url(#missing)" fill="#000007"/>
		<rect width="10" height="10" clip-path="url(#m)" fill="#000008"/>
		<clipPath id="bb" clipPathUnits="objectBoundingBox"><rect width=".5" height=".5"/></clipPath>
		<line x2="10" stroke="#000" clip-path="url(#bb)" stroke-width="9"/>`)
	got := dump(t, src, Options{})
	for _, want := range []string{"PushClipPath", "PushLayer opacity=1 blend=normal mask=alpha(2 cmds", "mask=luminance(", "#000001", "#000002", "#000004", "#000005", "#000007", "#000008"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	for _, bad := range []string{"#000003", "#000006", "StrokePath"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q must not be drawn:\n%s", bad, got)
		}
	}
}

func TestMaskFollowsOverrides(t *testing.T) {
	src := svgDoc(`<mask id="m"><rect width="10" height="10"/></mask><rect width="10" height="10" mask="url(#m)" fill="#ff0000"/>`)
	d := mustParse(t, src)
	maskOf := func(l *gfx.DisplayList) string {
		for _, c := range l.All() {
			if pl, ok := c.(gfx.PushLayer); ok && pl.Mask.List != nil {
				return pl.Mask.List.String()
			}
		}
		t.Fatalf("no mask layer:\n%s", l)
		return ""
	}
	a := maskOf(d.Record(Options{}))
	b := maskOf(d.Record(Options{Root: Root{Fill: Paint{Kind: PaintColor, Color: gfx.White}}}))
	if !strings.Contains(a, "paint=#000000") || !strings.Contains(b, "paint=#ffffff") {
		t.Errorf("the mask content does not follow the root fill override:\n%s\n%s", a, b)
	}
}

func TestRecordMatchesDraw(t *testing.T) {
	d := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" stroke="currentColor" fill="none"><circle cx="12" cy="12" r="9"/></svg>`)
	opts := Options{CurrentColor: gfx.MustParseHex("#336699")}
	dst := gfx.RectXYWH(4, 4, 48, 48)
	b1 := gfx.NewBuilder()
	d.Draw(b1, dst, opts)
	b2 := gfx.NewBuilder()
	d.DrawRecorded(b2, dst, d.Record(opts))
	l1, l2 := b1.Finish(), b2.Finish()
	if l1.Len() != l2.Len() {
		t.Fatalf("Draw:\n%s\nDrawRecorded:\n%s", l1, l2)
	}
	for i := 0; i < l1.Len(); i++ {
		if !gfx.Equal(l1.At(i), l2.At(i)) || l1.BoundsAt(i) != l2.BoundsAt(i) {
			t.Errorf("command %d differs: %v / %v", i, l1.At(i), l2.At(i))
		}
	}
}
