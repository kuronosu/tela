package style

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// child computes the style of a child Div with classes under parent;
// last says whether it is the last child.
func child(t *testing.T, cx *Context, parent *ComputedStyle, last bool, classes string, props ...Op) *ComputedStyle {
	t.Helper()
	in := Input{Kind: KindDiv, Parent: parent, Classes: []*Classes{mustCompile(t, classes)}, Props: props}
	if last {
		in.State |= StateLast
	} else {
		in.State |= StateFirst
	}
	return Compute(cx, in)
}

func widths(s *ComputedStyle) [4]float32 {
	b := s.Border
	return [4]float32{b.Top.Width, b.Right.Width, b.Bottom.Width, b.Left.Width}
}

// TestDivideWidths follows Tailwind v4 (tailwindcss 4.3.3): the lines are
// the end border (divide-x, in the inline direction) or the bottom border
// (divide-y) of every child but the last; -reverse moves them to the
// start or top side.
func TestDivideWidths(t *testing.T) {
	tests := []struct {
		class string
		want  [4]float32 // top, right, bottom, left of a child that is not the last
	}{
		{"divide-x", [4]float32{0, 1, 0, 0}},
		{"divide-y", [4]float32{0, 0, 1, 0}},
		{"divide-x-2", [4]float32{0, 2, 0, 0}},
		{"divide-y-4", [4]float32{0, 0, 4, 0}},
		{"divide-x-0", [4]float32{0, 0, 0, 0}},
		{"divide-x-[3px]", [4]float32{0, 3, 0, 0}},
		{"divide-y-[0.5rem]", [4]float32{0, 0, 8, 0}},
		{"divide-x-[length:2px]", [4]float32{0, 2, 0, 0}},
		{"divide-x divide-x-reverse", [4]float32{0, 0, 0, 1}},
		{"divide-x-reverse divide-x", [4]float32{0, 0, 0, 1}}, // class order does not matter
		{"divide-y divide-y-reverse", [4]float32{1, 0, 0, 0}},
		{"divide-x-3 divide-y-2", [4]float32{0, 3, 2, 0}},
		{"divide-x-2 divide-x-4", [4]float32{0, 4, 0, 0}}, // last wins
		{"divide-x-reverse", [4]float32{0, 0, 0, 0}},      // no width: no line
		{"divide-red-500", [4]float32{0, 0, 0, 0}},        // no width: no line
	}
	for _, tt := range tests {
		parent := style(t, "flex "+tt.class)
		if got := widths(child(t, testCx, parent, false, "")); got != tt.want {
			t.Errorf("%s: widths %v, want %v", tt.class, got, tt.want)
		}
		if got := widths(child(t, testCx, parent, true, "")); got != ([4]float32{}) {
			t.Errorf("%s: the last child has widths %v, want none", tt.class, got)
		}
	}
}

func TestDivideRTL(t *testing.T) {
	parent := style(t, "flex divide-x-2")
	rtl := func(classes string) *ComputedStyle {
		in := Input{Kind: KindDiv, Parent: parent, Classes: []*Classes{mustCompile(t, classes)}, State: StateFirst, Dir: DirRTL}
		return Compute(testCx, in)
	}
	if got, want := widths(rtl("")), [4]float32{0, 0, 0, 2}; got != want {
		t.Errorf("rtl divide-x: %v, want %v (the line is on the left, the inline end)", got, want)
	}
	parent = style(t, "flex divide-x-2 divide-x-reverse")
	if got, want := widths(rtl("")), [4]float32{0, 2, 0, 0}; got != want {
		t.Errorf("rtl divide-x-reverse: %v, want %v", got, want)
	}
}

// TestDivideColorAndStyle: the color and the style apply to the four
// borders of the child (border-color and border-style in Tailwind).
func TestDivideColorAndStyle(t *testing.T) {
	th := DefaultTheme()
	parent := style(t, "divide-y divide-red-500/50 divide-dashed")
	c := child(t, testCx, parent, false, "")
	if c.Border.Bottom.Width != 1 || c.Border.Bottom.Style != BorderDashed {
		t.Errorf("bottom border %+v, want 1px dashed", c.Border.Bottom)
	}
	want := th.Colors["red-500"].MulAlpha(0.5)
	for i, b := range []Border{c.Border.Top, c.Border.Right, c.Border.Bottom, c.Border.Left} {
		if b.Color != want || b.Style != BorderDashed {
			t.Errorf("border %d = %+v, want color %v dashed", i, b, want)
		}
	}
	// The default color is currentColor: the child's text color.
	parent = style(t, "divide-y")
	c = child(t, testCx, parent, false, "text-red-600")
	if c.Border.Bottom.Color != th.Colors["red-600"] {
		t.Errorf("default divide color = %v, want the text color %v", c.Border.Bottom.Color, th.Colors["red-600"])
	}
	// divide-none draws nothing.
	parent = style(t, "divide-y divide-none")
	if c := child(t, testCx, parent, false, ""); c.Border.Bottom.Width != 0 {
		t.Errorf("divide-none: bottom width %v", c.Border.Bottom.Width)
	}
	for _, st := range []string{"solid", "dotted", "double"} {
		parent = style(t, "divide-x divide-"+st)
		c := child(t, testCx, parent, false, "")
		if c.Border.Right.Style != divideStyles[st] || c.Border.Right.Width != 1 {
			t.Errorf("divide-%s: right border %+v", st, c.Border.Right)
		}
	}
}

// TestDivideTokens: a token color resolves in the scheme of the child, as
// border-border does, with an opacity modifier.
func TestDivideTokens(t *testing.T) {
	for _, cx := range []*Context{lightCx, darkCx} {
		parent := styleIn(t, cx, KindDiv, nil, "divide-y divide-border dark:divide-input/50")
		c := child(t, cx, parent, false, "")
		want := tok("border", cx.Dark)
		if cx.Dark {
			want = tok("input", true).MulAlpha(0.5)
		}
		if c.Border.Bottom.Color != want {
			t.Errorf("dark=%v: divide color %v, want %v", cx.Dark, c.Border.Bottom.Color, want)
		}
		via := child(t, cx, styleIn(t, cx, KindDiv, nil, "divide-y divide-[var(--color-border)]"), false, "")
		if via.Border.Bottom.Color != tok("border", cx.Dark) {
			t.Errorf("dark=%v: divide-[var(--color-border)] = %v", cx.Dark, via.Border.Bottom.Color)
		}
	}
	// A child under a Scheme override resolves the token for its own scheme.
	parent := style(t, "divide-y divide-border")
	in := Input{Kind: KindDiv, Parent: parent, State: StateFirst, Scheme: SchemeDark}
	if got := Compute(lightCx, in).Border.Bottom.Color; got != tok("border", true) {
		t.Errorf("scheme override: %v, want %v", got, tok("border", true))
	}
}

// TestDivideChildWins: the divide is under the child's own classes (it is
// :where() in Tailwind), property by property.
func TestDivideChildWins(t *testing.T) {
	parent := style(t, "divide-y-2 divide-red-500")
	th := DefaultTheme()
	tests := []struct {
		class string
		check func(s *ComputedStyle) bool
	}{
		{"border-b-4", func(s *ComputedStyle) bool { return s.Border.Bottom.Width == 4 }},
		{"border-b-0", func(s *ComputedStyle) bool { return s.Border.Bottom.Width == 0 }},
		{"border", func(s *ComputedStyle) bool { return widths(s) == [4]float32{1, 1, 1, 1} }},
		{"border-t", func(s *ComputedStyle) bool { return widths(s) == [4]float32{1, 0, 2, 0} }}, // both apply
		{"border-0", func(s *ComputedStyle) bool { return widths(s) == [4]float32{} }},
		{"border-blue-500", func(s *ComputedStyle) bool { return s.Border.Bottom.Color == th.Colors["blue-500"] }},
		{"border-b-green-500", func(s *ComputedStyle) bool {
			return s.Border.Bottom.Color == th.Colors["green-500"] && s.Border.Top.Color == th.Colors["red-500"]
		}},
		{"border-b-0 border-b-2", func(s *ComputedStyle) bool { return s.Border.Bottom.Width == 2 }},
		{"border-dotted", func(s *ComputedStyle) bool { return s.Border.Bottom.Style == BorderDotted }},
		{"border-none", func(s *ComputedStyle) bool { return s.Border.Bottom.Width == 0 }},
		{"border-red-500/25", func(s *ComputedStyle) bool { return s.Border.Bottom.Color == th.Colors["red-500"].MulAlpha(0.25) }},
	}
	for _, tt := range tests {
		if c := child(t, testCx, parent, false, tt.class); !tt.check(c) {
			t.Errorf("child %q under divide-y-2 divide-red-500: borders %+v", tt.class, c.Border)
		}
	}
	// A typed prop wins too.
	if c := child(t, testCx, parent, false, "", Prop(func(s *ComputedStyle) { s.Border.Bottom.Width = 7 })); c.Border.Bottom.Width != 7 {
		t.Errorf("prop: bottom width %v", c.Border.Bottom.Width)
	}
}

func TestDivideVariants(t *testing.T) {
	narrow := &Context{Viewport: gfx.Size{W: 500, H: 800}}
	wide := &Context{Viewport: gfx.Size{W: 1000, H: 800}}
	for _, tt := range []struct {
		cx   *Context
		want float32
	}{{narrow, 0}, {wide, 1}} {
		parent := styleIn(t, tt.cx, KindDiv, nil, "md:divide-y")
		if got := child(t, tt.cx, parent, false, "").Border.Bottom.Width; got != tt.want {
			t.Errorf("md:divide-y at %v px: bottom %v, want %v", tt.cx.Viewport.W, got, tt.want)
		}
	}
	// dark: and hover: on the parent.
	for _, dark := range []bool{false, true} {
		cx := lightCx
		if dark {
			cx = darkCx
		}
		parent := styleIn(t, cx, KindDiv, nil, "divide-y divide-zinc-200 dark:divide-zinc-800")
		want := DefaultTheme().Colors["zinc-200"]
		if dark {
			want = DefaultTheme().Colors["zinc-800"]
		}
		if got := child(t, cx, parent, false, "").Border.Bottom.Color; got != want {
			t.Errorf("dark=%v: %v, want %v", dark, got, want)
		}
	}
	in := Input{Kind: KindDiv, Classes: []*Classes{mustCompile(t, "hover:divide-x-4")}}
	if p := Compute(testCx, in); p.Divide != nil {
		t.Errorf("not hovered: Divide = %+v", p.Divide)
	}
	in.State = StateHover
	p := Compute(testCx, in)
	if p.Divide == nil || child(t, testCx, p, false, "").Border.Right.Width != 4 {
		t.Errorf("hovered: Divide = %+v", p.Divide)
	}
	// A variant on the child's side classes works as for any class.
	parent := style(t, "divide-y")
	if got := child(t, testCx, parent, false, "last:border-0 md:border-b-8").Border.Bottom.Width; got != 8 {
		t.Errorf("child md:border-b-8: %v", got)
	}
}

// TestDivideNotInherited: the divide of an element does not reach its
// grandchildren, and an element without divide has none.
func TestDivideNotInherited(t *testing.T) {
	parent := style(t, "divide-y")
	c := child(t, testCx, parent, false, "")
	if c.Divide != nil {
		t.Errorf("a child inherited the divide: %+v", c.Divide)
	}
	g := child(t, testCx, c, false, "")
	if widths(g) != ([4]float32{}) {
		t.Errorf("a grandchild got widths %v", widths(g))
	}
	if style(t, "flex").Divide != nil {
		t.Error("Divide is set without a divide class")
	}
	if !parent.Divide.Equal(style(t, "divide-y").Divide) || parent.Divide.Equal(style(t, "divide-x").Divide) || parent.Divide.Equal(nil) {
		t.Error("Divide.Equal")
	}
	// The root has no parent: nothing to apply.
	if c := Compute(testCx, Input{Kind: KindDiv}); widths(c) != ([4]float32{}) {
		t.Errorf("root widths %v", widths(c))
	}
}

func TestDivideClassFlag(t *testing.T) {
	for _, cl := range []string{"divide-y", "md:divide-x-2", "dark:hover:divide-zinc-800", "divide-dashed"} {
		if c := mustCompile(t, cl); !c.Divide {
			t.Errorf("%s: Classes.Divide is false", cl)
		}
	}
	for _, cl := range []string{"flex", "border-y", "space-y-2"} {
		if c := mustCompile(t, cl); c.Divide {
			t.Errorf("%s: Classes.Divide is true", cl)
		}
	}
}

func TestDivideErrors(t *testing.T) {
	for _, cl := range []string{"divide-x-2.5", "divide-x-[red]", "divide-x-foo", "divide-z", "divide-x-px", "-divide-x-2", "divide-x/50", "divide-x-reverse-2"} {
		c := Compile(cl)
		if c.Err == nil {
			t.Errorf("%s compiled", cl)
			continue
		}
		var u *UnknownClassError
		var i *InvalidClassError
		if !errors.As(c.Err, &u) && !errors.As(c.Err, &i) {
			t.Errorf("%s: Err = %v", cl, c.Err)
		}
	}
	c := Compile("divide-y-reverss")
	var u *UnknownClassError
	if !errors.As(c.Err, &u) || u.Suggestion != "divide-y-reverse" {
		t.Errorf("divide-y-reverss: Err = %v", c.Err)
	}
	c = Compile("divide-zinc-2OO")
	if c.Err == nil || strings.Contains(c.Err.Error(), "not supported") {
		t.Errorf("divide-zinc-2OO: Err = %v", c.Err)
	}
}

// TestDivideVocabulary: the classes are in the vocabulary of suggestions.
func TestDivideVocabulary(t *testing.T) {
	v := map[string]bool{}
	for _, c := range DefaultParser().Vocabulary() {
		v[c] = true
	}
	for _, c := range []string{"divide-x", "divide-y", "divide-x-2", "divide-y-reverse", "divide-x-reverse", "divide-dashed", "divide-border", "divide-zinc-200", "divide-transparent", "divide-current"} {
		if !v[c] {
			t.Errorf("%s is not in the vocabulary", c)
		}
	}
}
