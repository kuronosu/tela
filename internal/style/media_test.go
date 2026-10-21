package style

import (
	"errors"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func TestSVGPaintUtilities(t *testing.T) {
	red := DefaultTheme().Colors["red-500"]
	s := style(t, "fill-red-500 stroke-current/50 stroke-[1.5]")
	if s.Fill != (SVGPaint{Kind: SVGPaintColor, Color: red}) {
		t.Errorf("fill = %+v", s.Fill)
	}
	if s.Stroke.Kind != SVGPaintCurrent || s.Stroke.Color.A != 0.5 {
		t.Errorf("stroke = %+v, want currentColor at 50%%", s.Stroke)
	}
	if !s.StrokeWidthSet || s.StrokeWidth != 1.5 {
		t.Errorf("stroke width = %v %v", s.StrokeWidth, s.StrokeWidthSet)
	}
	for cls, want := range map[string]float32{"stroke-0": 0, "stroke-2": 2, "stroke-10": 10, "stroke-[3px]": 3,
		"stroke-[length:0.5rem]": 8, "text-[20px] stroke-[0.1em]": 2, "stroke-[0.75]": 0.75} {
		if s := style(t, cls); !s.StrokeWidthSet || !near(s.StrokeWidth, want, 1e-5) {
			t.Errorf("%s: stroke width %v, want %v", cls, s.StrokeWidth, want)
		}
	}
	if s := style(t, "fill-none stroke-none"); s.Fill.Kind != SVGPaintNone || s.Stroke.Kind != SVGPaintNone {
		t.Errorf("none: %+v %+v", s.Fill, s.Stroke)
	}
	if s := style(t, "fill-none fill-[#123456]"); s.Fill.Kind != SVGPaintColor || s.Fill.Color != gfx.MustParseHex("#123456") {
		t.Errorf("last wins: %+v", s.Fill)
	}
	// Not stroke widths: stroke-1.5 is not a Tailwind class.
	for _, cls := range []string{"stroke-1.5", "stroke--1"} {
		if c := Compile(cls); c.Err == nil {
			t.Errorf("%s compiled", cls)
		}
	}
}

func TestSVGPaintInheritance(t *testing.T) {
	parent := style(t, "fill-current stroke-sky-500 stroke-[1.5] text-red-500")
	child := styleIn(t, testCx, KindDiv, parent, "text-blue-500")
	if child.Fill.Kind != SVGPaintCurrent || child.Stroke != parent.Stroke || child.StrokeWidth != 1.5 || !child.StrokeWidthSet {
		t.Errorf("inherited: %+v %+v %v", child.Fill, child.Stroke, child.StrokeWidth)
	}
	// currentColor is inherited as the keyword: the child's own color.
	if got := child.Fill.Resolve(child.Text.Color); got != DefaultTheme().Colors["blue-500"] {
		t.Errorf("fill-current resolves to %v, want the child's text color", got)
	}
	// Tokens bind for the element's scheme.
	dark := Compute(testCx, Input{Kind: KindDiv, Scheme: SchemeDark, Classes: []*Classes{mustCompile(t, "fill-primary")}})
	light := style(t, "fill-primary")
	if dark.Fill.Color == light.Fill.Color || dark.Fill.Kind != SVGPaintColor {
		t.Errorf("token not bound per scheme: %v %v", dark.Fill, light.Fill)
	}
}

func TestObjectFitUtilities(t *testing.T) {
	cases := map[string]struct {
		fit  ObjectFit
		x, y float32
	}{
		"object-contain":                     {ObjectContain, 50, 50},
		"object-cover object-top":            {ObjectCover, 50, 0},
		"object-none object-left-bottom":     {ObjectNone, 0, 100},
		"object-scale-down object-top-right": {ObjectScaleDown, 100, 0},
		"object-fill object-[25%_75%]":       {ObjectFill, 25, 75},
	}
	for cls, want := range cases {
		s := style(t, cls)
		if s.ObjectFit != want.fit || s.ObjectPosX != PctLP(want.x) || s.ObjectPosY != PctLP(want.y) {
			t.Errorf("%s: %v %v %v", cls, s.ObjectFit, s.ObjectPosX, s.ObjectPosY)
		}
	}
	if s := style(t, "object-[10px_2rem]"); s.ObjectPosX != PxLP(10) || s.ObjectPosY != PxLP(32) {
		t.Errorf("lengths: %v %v", s.ObjectPosX, s.ObjectPosY)
	}
}

func TestMaskImageUtilities(t *testing.T) {
	s := style(t, "mask-[url(icons/a.png)] mask-contain mask-no-repeat mask-center mask-luminance mask-origin-content mask-clip-padding")
	m := s.Mask
	if len(m.Layers) != 1 || m.Layers[0].URL != "icons/a.png" || m.Layers[0].Size.Kind != SizeContain ||
		m.Layers[0].Repeat != RepeatNone || m.Layers[0].PosX != PctLP(50) || m.Layers[0].Origin != BackgroundContentBox ||
		m.Mode != MaskModeLuminance || m.Clip != BackgroundPaddingBox || m.Composite != MaskAdd {
		t.Errorf("mask = %+v", m)
	}
	if s := style(t, "mask-[url(a.png)] mask-none"); !s.Mask.IsNone() {
		t.Errorf("mask-none: %+v", s.Mask)
	}
	if s := style(t, "mask-[url(a.svg),linear-gradient(black,transparent)] mask-size-[10px_20px] mask-position-[5px_6px] mask-exclude"); len(s.Mask.Layers) != 2 ||
		s.Mask.Layers[1].Gradient == nil || s.Mask.Layers[1].Size.Kind != SizeLength || s.Mask.Composite != MaskExclude ||
		s.Mask.Layers[0].PosY != PxLP(6) {
		t.Errorf("list: %+v", s.Mask)
	}
	if s := style(t, "mask-[url(a.png)] mask-no-clip mask-type-alpha"); !s.Mask.NoClip {
		t.Errorf("no-clip: %+v", s.Mask)
	}
	// Defaults: border-box origin and clip, repeat, 0% 0%.
	if s := style(t, "mask-[url(a.png)]"); s.Mask.Layers[0].Origin != BackgroundBorderBox || s.Mask.Layers[0].PosX != (LP{}) {
		t.Errorf("defaults: %+v", s.Mask.Layers[0])
	}
	for _, cls := range []string{"mask-[#fff]", "mask-[url:a.png]", "mask-size-cover"} {
		if c := Compile(cls); c.Err == nil {
			t.Errorf("%s compiled", cls)
		}
	}
}

func TestMaskGradientUtilities(t *testing.T) {
	// Edges: four linear layers (left, right, bottom, top), then the
	// opaque radial and conic ones, intersected.
	s := style(t, "mask-t-from-50% mask-x-to-80%")
	m := s.Mask
	if len(m.Layers) != 6 || m.Composite != MaskIntersect {
		t.Fatalf("edges: %d layers, composite %v", len(m.Layers), m.Composite)
	}
	top := m.Layers[3].Gradient
	if top.Angle != 0 || top.Stops[0].Pos.LP != PctLP(50) || top.Stops[0].Color != gfx.Black || top.Stops[1].Color != gfx.Transparent {
		t.Errorf("top edge: %+v", top)
	}
	left, bottom := m.Layers[0].Gradient, m.Layers[2].Gradient
	if left.Angle != 270 || left.Stops[1].Pos.LP != PctLP(80) || bottom.Stops[0].Color != gfx.White {
		t.Errorf("left %+v bottom %+v", left, bottom)
	}
	// Stops win over the edges, whatever the order.
	s = style(t, "mask-linear-from-20% mask-t-from-50% mask-linear-45")
	if g := s.Mask.Layers[0].Gradient; len(s.Mask.Layers) != 3 || g.Angle != 45 || g.Stops[0].Pos.LP != PctLP(20) {
		t.Errorf("linear stops: %+v", s.Mask.Layers)
	}
	// Spacing numbers and colors.
	s = style(t, "mask-b-from-20 mask-b-from-red-500/50")
	if g := s.Mask.Layers[2].Gradient; g.Stops[0].Pos.LP != PxLP(80) || !near(g.Stops[0].Color.A, 0.5, 1e-6) {
		t.Errorf("spacing and color: %+v", g.Stops)
	}
	// Radial with shape, size and position.
	s = style(t, "mask-radial-from-10% mask-radial-to-60% mask-circle mask-radial-closest-side mask-radial-at-top-left")
	if g := s.Mask.Layers[1].Gradient; g.Kind != GradientRadial || g.Shape != ShapeCircle || g.Size != SizeClosestSide ||
		g.CenterX != PctLP(0) || g.Stops[1].Pos.LP != PctLP(60) {
		t.Errorf("radial: %+v", g)
	}
	s = style(t, "mask-radial-from-0% mask-radial-[40%_60%]")
	if g := s.Mask.Layers[1].Gradient; g.Size != SizeExplicit || g.Radius[1] != PctLP(60) {
		t.Errorf("radial size: %+v", g)
	}
	// Conic: angle and stops; a degree position is a fraction of a turn.
	s = style(t, "mask-conic-90 mask-conic-from-[90deg] mask-conic-to-75%")
	if g := s.Mask.Layers[2].Gradient; g.Kind != GradientConic || g.Angle != 90 || g.Stops[0].Pos.LP != PctLP(25) {
		t.Errorf("conic: %+v", g)
	}
	// Invalid at computed-value time: no mask at all.
	for _, cls := range []string{"mask-linear-45", "mask-radial-[closest-side]", "mask-conic-45",
		"mask-radial-from-10% mask-radial-[3rem]", "mask-conic-from-50", "mask-t-from-10% mask-linear-30"} {
		if s := style(t, cls); !s.Mask.IsNone() {
			t.Errorf("%s: want no mask, got %d layers", cls, len(s.Mask.Layers))
		}
	}
	// An explicit composite wins over the gradients' intersect.
	if s := style(t, "mask-add mask-t-from-50%"); s.Mask.Composite != MaskAdd {
		t.Errorf("composite: %v", s.Mask.Composite)
	}
	// Tokens in stops.
	dark := Compute(testCx, Input{Kind: KindDiv, Scheme: SchemeDark, Classes: []*Classes{mustCompile(t, "mask-l-from-primary")}})
	light := style(t, "mask-l-from-primary")
	if dark.Mask.Layers[0].Gradient.Stops[0].Color == light.Mask.Layers[0].Gradient.Stops[0].Color {
		t.Error("token stops not bound per scheme")
	}
}

func TestMaskErrors(t *testing.T) {
	for _, cls := range []string{"mask-linear-[sideways]", "mask-radial-[circle_at_center]", "mask-conic-[left]"} {
		c := Compile(cls)
		var ie *InvalidClassError
		if !errors.As(c.Err, &ie) {
			t.Errorf("%s: err = %v, want invalid", cls, c.Err)
		}
	}
}
