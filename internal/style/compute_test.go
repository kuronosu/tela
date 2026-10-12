package style

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

func TestRootTextStyle(t *testing.T) {
	s := Compute(testCx, Input{Kind: KindRoot})
	eq(t, "Size", s.Text.FontSize, 16)
	eq(t, "Weight", s.Text.FontWeight, 400)
	eq(t, "Color", s.Text.Color, gfx.Black)
	eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1.5))
	eq(t, "Tab", s.Text.TabSize, 4)
	if len(s.Text.FontFamily) != 1 || s.Text.FontFamily[0] != "Inter" {
		t.Errorf("FontFamily = %v, want [Inter]", s.Text.FontFamily)
	}
	eq(t, "Display", s.Display, DisplayBlock)
	eq(t, "Opacity", s.Opacity, 1)
	eq(t, "FlexShrink", s.FlexShrink, 1)
	if !s.Transform.IsIdentity() {
		t.Error("the default transform is not the identity")
	}
}

func TestPreflightResets(t *testing.T) {
	s := style(t, "")
	eq(t, "Margin", s.Margin, AllSides(DimPx(0)))
	eq(t, "Padding", s.Padding, AllSides(LP{}))
	for _, b := range borderSides(s) {
		eq(t, "border width", b.Width, 0)
		eq(t, "border style", b.Style, BorderSolid)
		eq(t, "border color", b.Color, s.Text.Color)
	}
}

func TestPrecedence(t *testing.T) {
	// Defaults, then classes in order, then typed props: last wins.
	cl1, cl2 := mustCompile(t, "w-4 p-2 bg-red-500"), mustCompile(t, "w-8 bg-blue-500")
	s := Compute(testCx, Input{Kind: KindButton, Classes: []*Classes{cl1, cl2}, Props: []Op{Width(100), Padding(7)}})
	eq(t, "Width (prop)", s.Width, DimPx(100))
	eq(t, "Padding (prop)", s.Padding, AllSides(PxLP(7)))
	eq(t, "Bg (second class string)", s.BackgroundColor, DefaultTheme().Colors["blue-500"])

	s = Compute(testCx, Input{Kind: KindButton, Classes: []*Classes{cl1, cl2}})
	eq(t, "Width (second class string)", s.Width, DimPx(32))
	eq(t, "Padding (first class string)", s.Padding, AllSides(PxLP(8)))

	// A class overrides a default.
	s = Compute(testCx, Input{Kind: KindButton, Classes: []*Classes{mustCompile(t, "cursor-default block")}})
	eq(t, "Cursor", s.Cursor, CursorDefault)
	eq(t, "Display", s.Display, DisplayBlock)
	// ... and the default holds when nothing overrides it.
	s = Compute(testCx, Input{Kind: KindButton})
	eq(t, "Cursor", s.Cursor, CursorPointer)
	eq(t, "Display", s.Display, DisplayInlineBlock)
}

func TestInheritance(t *testing.T) {
	parent := style(t, "text-red-500 text-xl font-bold italic tracking-wide leading-7 text-center uppercase underline whitespace-nowrap font-mono tabular-nums cursor-pointer pointer-events-none select-none invisible "+
		"bg-white p-4 w-32 border-2 opacity-50 rotate-45 shadow-lg")
	child := Compute(testCx, Input{Kind: KindDiv, Parent: parent})

	// Inherited: text properties (and cursor, visibility, pointer-events, user-select).
	eq(t, "Color", child.Text.Color, parent.Text.Color)
	eq(t, "Size", child.Text.FontSize, parent.Text.FontSize)
	eq(t, "Weight", child.Text.FontWeight, 700)
	eq(t, "Style", child.Text.FontStyle, text.StyleItalic)
	eq(t, "Spacing", child.Text.LetterSpacing, parent.Text.LetterSpacing)
	eq(t, "LH", child.Text.LineHeight, text.LineHeightPixels(28))
	eq(t, "Align", child.Text.TextAlign, text.AlignCenter)
	eq(t, "Transform", child.Text.TextTransform, text.TransformUppercase)
	eq(t, "Deco", child.Text.Decoration, parent.Text.Decoration)
	eq(t, "WS", child.Text.WhiteSpace, text.WhiteSpaceNowrap)
	eq(t, "Numeric", child.Text.Numeric, text.NumericTabular)
	eq(t, "Family", child.Text.FontFamily[0], "JetBrains Mono")
	eq(t, "Cursor", child.Cursor, CursorPointer)
	eq(t, "PE", child.PointerEvents, PointerEventsNone)
	eq(t, "US", child.UserSelect, UserSelectNone)
	eq(t, "Visibility", child.Visibility, VisibilityHidden)

	// Not inherited: boxes, backgrounds, borders, effects, transforms.
	eq(t, "Bg", child.BackgroundColor, gfx.Transparent)
	eq(t, "Padding", child.Padding, AllSides(LP{}))
	eq(t, "Width", child.Width, Auto)
	eq(t, "Border", child.Border.Top.Width, 0)
	eq(t, "Opacity", child.Opacity, 1)
	eq(t, "Rotate", child.Transform.Rotate, 0)
	eq(t, "Shadows", len(child.BoxShadows), 0)
	eq(t, "TextOverflow", child.TextOverflow, text.TextOverflowClip)
}

func TestCurrentColor(t *testing.T) {
	// Borders and outlines default to the text color of the element itself.
	p := style(t, "text-red-500")
	c := Compute(testCx, Input{Parent: p, Classes: []*Classes{mustCompile(t, "border text-blue-500 outline ring-2")}})
	blue := DefaultTheme().Colors["blue-500"]
	eq(t, "Border", c.Border.Left.Color, blue)
	eq(t, "Outline", c.Outline.Color, blue)
	eq(t, "Ring", c.BoxShadows[0].Color, blue)

	// text-current keeps the inherited color.
	d := Compute(testCx, Input{Parent: p, Classes: []*Classes{mustCompile(t, "text-current")}})
	eq(t, "text-current", d.Text.Color, p.Text.Color)
	// text-current with an opacity modifier.
	e := Compute(testCx, Input{Parent: p, Classes: []*Classes{mustCompile(t, "text-current/50")}})
	eq(t, "text-current/50", e.Text.Color, p.Text.Color.MulAlpha(0.5))

	// Gradient stops can be current.
	g := style(t, "text-red-500 bg-linear-to-r from-current to-transparent")
	eq(t, "from", g.Backgrounds[0].Gradient.Stops[0].Color, DefaultTheme().Colors["red-500"])
}

func TestEmUnits(t *testing.T) {
	// em is relative to the element's own font size, even when the class
	// that sets the size comes later; font-size itself is relative to the parent.
	s := style(t, "p-[1.5em] text-xl")
	eq(t, "Padding", s.Padding.Top, PxLP(30))
	s = style(t, "text-xl p-[1.5em]")
	eq(t, "Padding", s.Padding.Top, PxLP(30))

	parent := style(t, "text-[20px]")
	child := Compute(testCx, Input{Parent: parent, Classes: []*Classes{mustCompile(t, "text-[1.5em] w-[2em]")}})
	eq(t, "child size", child.Text.FontSize, 30)
	eq(t, "child width", child.Width, DimPx(60))

	// rem follows the theme's root size.
	th := DefaultTheme()
	th.RootFontSize = 20
	cx := &Context{Theme: th, Viewport: testCx.Viewport}
	got := Compute(cx, Input{Classes: []*Classes{mustCompile(t, "w-[2rem] p-4 text-base")}})
	eq(t, "rem width", got.Width, DimPx(40))
	eq(t, "spacing is rem-based", got.Padding.Top, PxLP(20))
	eq(t, "root size", got.Text.FontSize, 20)

	// vw/vh use the context's viewport.
	v := Compute(&Context{Viewport: gfx.Size{W: 500, H: 300}}, Input{Classes: []*Classes{mustCompile(t, "w-[50vw] h-screen max-h-[10vh]")}})
	eq(t, "vw", v.Width, DimPx(250))
	eq(t, "screen", v.Height, DimPx(300))
	eq(t, "vh", v.MaxHeight, DimPx(30))
}

func TestElementDefaults(t *testing.T) {
	parent := style(t, "text-red-500 text-xl font-bold")
	of := func(k Kind) *ComputedStyle { return Compute(testCx, Input{Kind: k, Parent: parent}) }

	for _, k := range []Kind{KindH1, KindH3, KindH6} {
		h := of(k)
		eq(t, k.String()+" size inherits", h.Text.FontSize, parent.Text.FontSize)
		eq(t, k.String()+" weight inherits", h.Text.FontWeight, parent.Text.FontWeight)
		eq(t, k.String()+" display", h.Display, DisplayBlock)
		eq(t, k.String()+" margin", h.Margin, AllSides(DimPx(0)))
	}
	eq(t, "p display", of(KindP).Display, DisplayBlock)
	eq(t, "span display", of(KindSpan).Display, DisplayInline)
	eq(t, "text display", of(KindText).Display, DisplayInline)

	b := of(KindButton)
	eq(t, "button display", b.Display, DisplayInlineBlock)
	eq(t, "button cursor", b.Cursor, CursorPointer)
	eq(t, "button bg", b.BackgroundColor, gfx.Transparent)
	eq(t, "button font", b.Text.FontSize, parent.Text.FontSize)
	eq(t, "button color", b.Text.Color, parent.Text.Color)
	eq(t, "button weight", b.Text.FontWeight, 700)
	eq(t, "button align", b.Text.TextAlign, text.AlignCenter)
	eq(t, "button border", b.Border.Top.Width, 0)
	eq(t, "button radius", b.Radius.TopLeft, PxLP(0))

	img := of(KindImg)
	eq(t, "img display", img.Display, DisplayBlock)
	eq(t, "img max-width", img.MaxWidth, DimPct(100))
	eq(t, "img height", img.Height, Auto)

	eq(t, "strong weight", of(KindStrong).Text.FontWeight, 900) // bolder than 700
	eq(t, "strong from 400", Compute(testCx, Input{Kind: KindStrong, Parent: style(t, "")}).Text.FontWeight, 700)
	eq(t, "em style", of(KindEm).Text.FontStyle, text.StyleItalic)
	eq(t, "code family", of(KindCode).Text.FontFamily[0], "JetBrains Mono")
	eq(t, "code size", of(KindCode).Text.FontSize, parent.Text.FontSize)
	eq(t, "a color", of(KindA).Text.Color, parent.Text.Color)

	if got := len(KindDefaults(KindImg)); got == 0 {
		t.Error("KindDefaults(KindImg) is empty")
	}
	// Every kind has a display default.
	for k := Kind(0); k < numKinds; k++ {
		if len(KindDefaults(k)) == 0 {
			t.Errorf("KindDefaults(%v) is empty", k)
		}
	}
}

func TestTypedProps(t *testing.T) {
	red := DefaultTheme().Colors["red-500"]
	props := []Op{
		Width(10), Height(20), MinWidth(1), MinHeight(2), MaxWidth(30), MaxHeight(40),
		Padding(3), Margin(4), Gap(5), Left(6), Top(7), Right(8), Bottom(9),
		Bg(red), Color(gfx.White), BorderColor(red), BorderWidth(2), Radius(11), Opacity(0.25),
		ZIndexOf(5), Translate(1, 2), Scale(2, 3), Rotate(30), FontSize(33), FontWeight(600),
	}
	s := Compute(testCx, Input{Props: props})
	eq(t, "Width", s.Width, DimPx(10))
	eq(t, "Height", s.Height, DimPx(20))
	eq(t, "MinWidth", s.MinWidth, DimPx(1))
	eq(t, "MaxHeight", s.MaxHeight, DimPx(40))
	eq(t, "Padding", s.Padding, AllSides(PxLP(3)))
	eq(t, "Margin", s.Margin.Left, DimPx(4))
	eq(t, "Gap", s.RowGap, PxLP(5))
	eq(t, "Inset", s.Inset, Sides[Dim]{DimPx(7), DimPx(8), DimPx(9), DimPx(6)})
	eq(t, "Bg", s.BackgroundColor, red)
	eq(t, "Color", s.Text.Color, gfx.White)
	eq(t, "Border color", s.Border.Right.Color, red)
	eq(t, "Border width", s.Border.Right.Width, 2)
	eq(t, "Radius", s.Radius.BottomLeft, PxLP(11))
	eq(t, "Opacity", s.Opacity, 0.25)
	eq(t, "Z", s.ZIndex, ZIndex{true, 5})
	eq(t, "Translate", s.Transform.TranslateY, PxLP(2))
	eq(t, "Scale", s.Transform.ScaleY, 3)
	eq(t, "Rotate", s.Transform.Rotate, 30)
	eq(t, "FontSize", s.Text.FontSize, 33)
	eq(t, "FontWeight", s.Text.FontWeight, 600)

	// Props go through the same finalize step: currentColor borders use the prop color.
	b := Compute(testCx, Input{Props: []Op{Color(red), BorderWidth(1)}})
	eq(t, "border follows color", b.Border.Top.Color, red)

	g := Gradient{Kind: GradientLinear, Angle: 90, Stops: []ColorStop{{Color: gfx.White}, {Color: gfx.Black}}}
	gs := Compute(testCx, Input{Props: []Op{BackgroundGradient(g)}})
	eq(t, "gradient layers", len(gs.Backgrounds), 1)
	sh := Compute(testCx, Input{Props: []Op{BoxShadow(Shadow{OffsetY: 2, Blur: 4, Color: gfx.Black}, Shadow{Spread: 1, Color: gfx.White, Inset: true})}})
	eq(t, "shadows", len(sh.BoxShadows), 2)
	eq(t, "inset first", sh.BoxShadows[0].Inset, true)

	custom := Compute(testCx, Input{Props: []Op{Prop(func(s *ComputedStyle) { s.AspectRatio = 2 })}})
	eq(t, "Prop", custom.AspectRatio, 2)
}

func TestDeferredProps(t *testing.T) {
	x, y := float32(10), float32(20)
	op := TranslateFn(func() (float32, float32) { return x, y })
	eq(t, "phase", op.Phase(), PhasePaint)
	eq(t, "layout phase", WidthFn(func() float32 { return 1 }).Phase(), PhaseLayout)
	eq(t, "plain phase", Width(1).Phase(), PhaseStyle)

	s := Compute(testCx, Input{Props: []Op{op, WidthFn(func() float32 { return x * 3 }), Opacity(0.5)}})
	// The closures are not evaluated while the style is computed.
	eq(t, "TranslateX before", s.Transform.TranslateX, PxLP(0))
	eq(t, "Width before", s.Width, Auto)
	eq(t, "deferred recorded", len(s.Deferred), 2)

	// A paint pass evaluates the paint-phase closures, a layout pass the layout ones.
	p := s.Resolved(PhasePaint)
	eq(t, "TranslateX paint", p.Transform.TranslateX, PxLP(10))
	eq(t, "Width paint pass", p.Width, Auto)
	l := s.Resolved(PhaseLayout)
	eq(t, "Width layout", l.Width, DimPx(30))
	eq(t, "TranslateX layout pass", l.Transform.TranslateX, PxLP(0))
	// The source style is untouched, and a later call sees new values.
	eq(t, "source", s.Transform.TranslateX, PxLP(0))
	x = 50
	eq(t, "re-evaluated", s.Resolved(PhasePaint).Transform.TranslateX, PxLP(50))
	// No deferred props of the phase: the same pointer comes back.
	plain := style(t, "p-4")
	if plain.Resolved(PhasePaint) != plain {
		t.Error("Resolved copied a style without deferred props")
	}
}

func TestTransformMatrix(t *testing.T) {
	box := gfx.RectXYWH(100, 50, 200, 100)
	// Rotation about the center.
	s := style(t, "rotate-90")
	m := s.Transform.Matrix(box)
	c := box.Center()
	if p := m.Apply(c); !near(p.X, c.X, 1e-3) || !near(p.Y, c.Y, 1e-3) {
		t.Errorf("center moved to %v", p)
	}
	if p := m.Apply(gfx.Pt(100, 50)); !near(p.X, 250, 1e-3) || !near(p.Y, 0, 1e-3) {
		t.Errorf("top-left corner went to %v, want (250, 0)", p)
	}
	// Percent translate refers to the box.
	s = style(t, "-translate-x-1/2 translate-y-full")
	m = s.Transform.Matrix(box)
	if p := m.Apply(gfx.Pt(0, 0)); !near(p.X, -100, 1e-4) || !near(p.Y, 100, 1e-4) {
		t.Errorf("origin went to %v", p)
	}
	// Scale about the top-left origin.
	s = style(t, "scale-50 origin-top-left")
	m = s.Transform.Matrix(box)
	if p := m.Apply(gfx.Pt(300, 150)); !near(p.X, 200, 1e-4) || !near(p.Y, 100, 1e-4) {
		t.Errorf("bottom-right went to %v, want (200, 100)", p)
	}
	// Composition order: translate, rotate, scale, then skew (CSS).
	s = style(t, "translate-x-4 scale-x-200 origin-top-left")
	if p := s.Transform.Matrix(box).Apply(gfx.Pt(110, 50)); !near(p.X, 100+16+20, 1e-3) {
		t.Errorf("scale then translate: got %v", p)
	}
	// Identity transforms give the identity.
	if !style(t, "p-4").Transform.Matrix(box).IsIdentity() {
		t.Error("identity transform produced a matrix")
	}
}
