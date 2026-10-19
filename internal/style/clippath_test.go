package style

import (
	"errors"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func TestClipPathArbitraryProperty(t *testing.T) {
	px, pct := PxLP, PctLP
	tests := []struct {
		class string
		want  ClipPath
	}{
		{"[clip-path:circle(50%)]", ClipPath{Shape: ClipCircle, Values: []LP{pct(50), pct(50), pct(50)}}},
		{"[clip-path:circle(40px_at_25%_75%)]", ClipPath{Shape: ClipCircle, Values: []LP{px(40), pct(25), pct(75)}}},
		{"[clip-path:circle()]", ClipPath{Shape: ClipCircle, Values: []LP{{}, pct(50), pct(50)},
			Radius: [2]ShapeRadius{RadiusClosestSide}}},
		{"[clip-path:circle(farthest-side_at_left_top)]", ClipPath{Shape: ClipCircle, Values: []LP{{}, pct(0), pct(0)},
			Radius: [2]ShapeRadius{RadiusFarthestSide}}},
		{"[clip-path:ellipse(50%_30%_at_center)]", ClipPath{Shape: ClipEllipse, Values: []LP{pct(50), pct(30), pct(50), pct(50)}}},
		{"[clip-path:ellipse()]", ClipPath{Shape: ClipEllipse, Values: []LP{{}, {}, pct(50), pct(50)},
			Radius: [2]ShapeRadius{RadiusClosestSide, RadiusClosestSide}}},
		{"[clip-path:inset(10px)]", ClipPath{Shape: ClipInset, Values: append([]LP{px(10), px(10), px(10), px(10)}, make([]LP, 8)...)}},
		{"[clip-path:inset(0_round_1rem)]", ClipPath{Shape: ClipInset, Values: []LP{{}, {}, {}, {},
			px(16), px(16), px(16), px(16), px(16), px(16), px(16), px(16)}}},
		{"[clip-path:inset(5%_10px_round_4px_8px_/_2px)]", ClipPath{Shape: ClipInset, Values: []LP{pct(5), px(10), pct(5), px(10),
			px(4), px(2), px(8), px(2), px(4), px(2), px(8), px(2)}}},
		{"[clip-path:polygon(0_0,100%_0,100%_80%,0_100%)]", ClipPath{Shape: ClipPolygon,
			Values: []LP{{}, {}, pct(100), {}, pct(100), pct(80), {}, pct(100)}}},
		{"[clip-path:polygon(evenodd,50%_0,100%_100%,0_100%)]", ClipPath{Shape: ClipPolygon, Rule: gfx.EvenOdd,
			Values: []LP{pct(50), {}, pct(100), pct(100), {}, pct(100)}}},
		{"[clip-path:circle(50%)_border-box]", ClipPath{Shape: ClipCircle, Values: []LP{pct(50), pct(50), pct(50)}}},
		{"[clip-path:none]", ClipPath{}},
	}
	for _, tt := range tests {
		got := style(t, tt.class).ClipPath
		if !got.Equal(&tt.want) {
			t.Errorf("%s: %+v, want %+v", tt.class, got, tt.want)
		}
	}
	// Variants apply to it, and the last class wins.
	c := mustCompile(t, "[clip-path:circle(50%)] hover:[clip-path:circle(75%)]")
	hov := Compute(testCx, Input{Kind: KindDiv, State: StateHover, Classes: []*Classes{c}})
	if v := hov.ClipPath.Values; len(v) != 3 || v[0] != PctLP(75) {
		t.Errorf("hover: %+v", hov.ClipPath)
	}
	if v := style(t, "[clip-path:circle(50%)] hover:[clip-path:circle(75%)]").ClipPath.Values; v[0] != PctLP(50) {
		t.Errorf("not hovered: %+v", v)
	}
	// em resolves against the element's font size.
	if v := style(t, "text-[20px] [clip-path:circle(2em)]").ClipPath.Values; v[0] != PxLP(40) {
		t.Errorf("em: %+v", v)
	}
}

func TestClipPathErrors(t *testing.T) {
	tests := []struct {
		class, mention string
		unsupported    bool
	}{
		{"[mask-type:luminance]", "arbitrary properties", true},
		{"[color:red]", "except [clip-path", true},
		{"[clip-path:path('M0_0')]", "path()", true},
		{"[clip-path:url(#c)]", "url()", true},
		{"[clip-path:circle(50%)_padding-box]", "border box", true},
		{"[clip-path:circle(var(--r))]", "CSS variables", true},
		{"[clip-path:star(5)]", "unknown clip path shape", false},
		{"[clip-path:circle(1px_2px)]", "one radius", false},
		{"[clip-path:polygon(0_0,50%)]", "two lengths", false},
		{"[clip-path:inset()]", "one to four", false},
		{"[clip-path:inset(1px_round)]", "radii", false},
		{"[clip-path:circle(10px_at)]", "position", false},
		{"[clip-path:blob]", "inset(), circle()", false},
		{"clip-path", "[clip-path:circle(50%)]", true},
	}
	for _, tt := range tests {
		c := Compile(tt.class)
		if c.Err == nil || len(c.Rules) != 0 {
			t.Errorf("%q: no error (rules %d)", tt.class, len(c.Rules))
			continue
		}
		var u *UnsupportedClassError
		var in *InvalidClassError
		switch {
		case tt.unsupported && errors.As(c.Err, &u):
			if !strings.Contains(u.Reason, tt.mention) {
				t.Errorf("%q: reason %q does not mention %q", tt.class, u.Reason, tt.mention)
			}
		case !tt.unsupported && errors.As(c.Err, &in):
			if !strings.Contains(in.Reason, tt.mention) {
				t.Errorf("%q: reason %q does not mention %q", tt.class, in.Reason, tt.mention)
			}
		default:
			t.Errorf("%q: Err = %v (%T)", tt.class, c.Err, c.Err)
		}
	}
}

func TestClipPathResolve(t *testing.T) {
	box := gfx.Rect{X0: 10, Y0: 20, X1: 110, Y1: 70} // 100 x 50
	near := func(a, b float32) bool { return a-b < 1e-3 && b-a < 1e-3 }
	g := style(t, "[clip-path:circle(50%)]").ClipPath.Resolve(box)
	// 50% of sqrt(100² + 50²) / sqrt(2) = 39.528.
	if r := g.RRect.Rect; !near(r.Width(), 79.0569) || !near(r.X0+r.X1, 120) || !near(r.Y0+r.Y1, 90) {
		t.Errorf("circle: %+v", g.RRect)
	}
	g = style(t, "[clip-path:circle()]").ClipPath.Resolve(box)
	if r := g.RRect.Rect; !near(r.Width(), 50) || !near(r.Height(), 50) {
		t.Errorf("closest-side: %+v", r)
	}
	g = style(t, "[clip-path:ellipse(farthest-side_closest-side_at_20%_50%)]").ClipPath.Resolve(box)
	if r := g.RRect.Rect; !near(r.X0, 10+20-80) || !near(r.X1, 10+20+80) || !near(r.Height(), 50) {
		t.Errorf("ellipse: %+v", r)
	}
	g = style(t, "[clip-path:inset(10px_20%_round_50%)]").ClipPath.Resolve(box)
	if r := g.RRect.Rect; r != (gfx.Rect{X0: 30, Y0: 30, X1: 90, Y1: 60}) || !near(g.RRect.Radii[0].X, 30) || !near(g.RRect.Radii[0].Y, 15) {
		t.Errorf("inset: %+v", g.RRect)
	}
	// Insets larger than the box are reduced in proportion.
	g = style(t, "[clip-path:inset(0_80px_0_120px)]").ClipPath.Resolve(box)
	if r := g.RRect.Rect; !near(r.X0, 70) || !near(r.X1, 70) {
		t.Errorf("large inset: %+v", r)
	}
	g = style(t, "[clip-path:polygon(50%_0,100%_100%,0_100%)]").ClipPath.Resolve(box)
	want := []gfx.Point{{X: 60, Y: 20}, {X: 110, Y: 70}, {X: 10, Y: 70}}
	if !g.IsPoly || !slices.Equal(g.Poly, want) || g.Bounds() != box {
		t.Errorf("polygon: %+v", g)
	}
}

func TestClipPathInterpolable(t *testing.T) {
	a := style(t, "[clip-path:circle(10%)]").ClipPath
	b := style(t, "[clip-path:circle(60%_at_0_0)]").ClipPath
	c := style(t, "[clip-path:circle()]").ClipPath
	d := style(t, "[clip-path:polygon(0_0,1px_1px,2px_0)]").ClipPath
	e := style(t, "[clip-path:polygon(0_0,1px_1px,2px_0,0_5px)]").ClipPath
	if !a.Interpolable(&b) || a.Interpolable(&c) || d.Interpolable(&e) || a.Interpolable(&d) {
		t.Error("interpolable pairs")
	}
	var none ClipPath
	if none.Interpolable(&none) {
		t.Error("none interpolates")
	}
	if c := style(t, "transition-[clip-path]").Transition.Properties; c != TransitionClipPath {
		t.Errorf("transition-[clip-path]: %v", c)
	}
}

func TestBackgroundLayersAndBlend(t *testing.T) {
	s := style(t, "bg-[linear-gradient(to_bottom,transparent,rgb(0_0_0/0.6)),url(img/photo.png)] bg-cover bg-center bg-no-repeat")
	if len(s.Backgrounds) != 2 {
		t.Fatalf("layers: %+v", s.Backgrounds)
	}
	top, bottom := s.Backgrounds[0], s.Backgrounds[1]
	if top.Gradient == nil || top.URL != "" || bottom.URL != "img/photo.png" || bottom.Gradient != nil {
		t.Errorf("layers: %+v", s.Backgrounds)
	}
	for _, l := range s.Backgrounds {
		if l.Size.Kind != SizeCover || l.PosX != PctLP(50) || l.Repeat != RepeatNone {
			t.Errorf("layer settings: %+v", l)
		}
	}
	if s := style(t, "bg-[url(a_b.png)]"); len(s.Backgrounds) != 1 || s.Backgrounds[0].URL != "a_b.png" {
		t.Errorf("underscores in url(): %+v", s.Backgrounds)
	}
	if s := style(t, "bg-blend-multiply"); s.BackgroundBlend != gfx.Multiply {
		t.Errorf("bg-blend-multiply: %v", s.BackgroundBlend)
	}
	if c := Compile("bg-blend-plus-lighter"); c.Err == nil {
		t.Error("bg-blend-plus-lighter accepted")
	}
	if c := Compile("bg-[url(a.png),bogus]"); c.Err == nil {
		t.Error("an invalid layer in a list is accepted")
	}
	// A decoded image takes the layer settings of the classes.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	s = styleIn(t, testCx, KindDiv, nil, "bg-contain bg-top", BgImage(img))
	if len(s.Backgrounds) != 1 || s.Backgrounds[0].Image != image.Image(img) || s.Backgrounds[0].Size.Kind != SizeContain ||
		s.Backgrounds[0].PosY != PctLP(0) {
		t.Errorf("BgImage: %+v", s.Backgrounds)
	}
}
