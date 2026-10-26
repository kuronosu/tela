package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// Absolutely positioned and fixed inline elements are blockified and out
// of the inline flow (ADR-176): an absolute Span is laid out and painted
// like an absolute Div with the same classes, and neither splits the
// paragraph it is in.

// renderOOF renders root at 240x160 and scale 2.
func renderOOF(root Node) *image.RGBA {
	_, img := Render(root, RenderOptions{Config: Config{Fonts: testFonts}, Width: 240, Height: 160, Scale: 2})
	return img
}

// sameImages fails the test when a and b differ.
func sameImages(t *testing.T, what string, a, b *image.RGBA) {
	t.Helper()
	if p, ok := firstDiff(a, b); !ok {
		t.Errorf("%s: the images differ at %v", what, p)
	}
}

// keyedRect returns the border box of the element with key k after a
// frame of tr.
func keyedRect(t *testing.T, tr *Tree, k string) [4]float32 {
	t.Helper()
	r := mustFind(t, tr, k).Rect
	return [4]float32{r.X0, r.Y0, r.X1, r.Y1}
}

// TestAbsoluteInlineIsBlockified: a status dot on an avatar (a block, not
// a flex container, with text) is the same box and the same pixels as a
// Span, an A or a Strong as it is as a Div, absolute or fixed.
func TestAbsoluteInlineIsBlockified(t *testing.T) {
	avatar := func(kind style.Kind, dot string) Node {
		return div(Class("relative m-4 size-10 rounded-full bg-indigo-200 text-sm"), Text("MC"),
			Primitive(kind, Key("dot"), Class(dot)))
	}
	for _, dot := range []string{
		"absolute right-0 bottom-0 size-3 rounded-full bg-emerald-500 ring-2 ring-white",
		"fixed right-2 bottom-2 size-3 rounded-full bg-sky-500",
	} {
		want := renderOOF(avatar(style.KindDiv, dot))
		tr := newTestTree(t, 240, 160, avatar(style.KindDiv, dot), nil)
		tr.Frame()
		wantRect := keyedRect(t, tr, "dot")
		if wantRect[2]-wantRect[0] != 12 || wantRect[3]-wantRect[1] != 12 {
			t.Fatalf("%q: the Div dot is %v", dot, wantRect)
		}
		for _, kind := range []style.Kind{style.KindSpan, style.KindA, style.KindStrong} {
			what := fmt.Sprintf("%v %q", kind, dot)
			sameImages(t, what, renderOOF(avatar(kind, dot)), want)
			tr := newTestTree(t, 240, 160, avatar(kind, dot), nil)
			tr.Frame()
			if got := keyedRect(t, tr, "dot"); got != wantRect {
				t.Errorf("%s: %v, want %v", what, got, wantRect)
			}
		}
	}
}

// TestAbsoluteInFlexIsFlex: an absolute inline-flex Span is a flex
// container stretched between its insets, like an absolute flex Div.
func TestAbsoluteInFlexIsFlex(t *testing.T) {
	badge := func(kind style.Kind) Node {
		return div(Class("relative m-2 h-10 bg-slate-100 text-sm"), Text("Badge: "),
			Primitive(kind, Key("badge"), Class("absolute inline-flex inset-x-2 top-1 h-6 items-center justify-between bg-rose-500 px-1 text-white"),
				span(Key("a"), Text("A")), span(Key("b"), Text("B"))))
	}
	flexDiv := func() Node {
		return div(Class("relative m-2 h-10 bg-slate-100 text-sm"), Text("Badge: "),
			div(Key("badge"), Class("absolute flex inset-x-2 top-1 h-6 items-center justify-between bg-rose-500 px-1 text-white"),
				span(Key("a"), Text("A")), span(Key("b"), Text("B"))))
	}
	tr := newTestTree(t, 240, 160, badge(style.KindSpan), nil)
	tr.Frame()
	ref := newTestTree(t, 240, 160, flexDiv(), nil)
	ref.Frame()
	for _, k := range []string{"badge", "a", "b"} {
		if got, want := keyedRect(t, tr, k), keyedRect(t, ref, k); got != want {
			t.Errorf("%s: %v, want %v", k, got, want)
		}
	}
	if r := keyedRect(t, tr, "badge"); r[2]-r[0] != 240-16-16 {
		t.Errorf("the badge is %v: not stretched between its insets", r)
	}
	sameImages(t, "inline-flex Span", renderOOF(badge(style.KindSpan)), renderOOF(flexDiv()))
}

// TestOutOfFlowKeepsTheLine: an absolute or fixed element in a paragraph,
// directly or inside an inline element, leaves its lines as they are
// without it (here invisible, so that the pixels compare), truncated and
// clamped ones included.
func TestOutOfFlowKeepsTheLine(t *testing.T) {
	const para = "relative m-2 w-52 bg-slate-100 text-sm"
	const long = "A long conversation title that does not fit on two lines of this narrow box"
	plain := p(Class(para), Text("Hello "), span(Class("font-semibold"), Text("world")), Text(", a paragraph that wraps onto two lines."))
	for _, c := range []struct {
		name  string
		size  float32
		root  Node
		plain Node
	}{
		{"truncate", 8, div(Class("relative m-2 w-40 truncate text-sm"), Text(long), span(Key("x"), Class("invisible absolute top-0 right-0 size-2 bg-rose-500"))),
			div(Class("relative m-2 w-40 truncate text-sm"), Text(long))},
		{"line-clamp", 8, div(Class("relative m-2 w-40 line-clamp-2 text-sm"), div(Key("x"), Class("invisible absolute top-0 right-0 size-2 bg-rose-500")), Text(long)),
			div(Class("relative m-2 w-40 line-clamp-2 text-sm"), Text(long))},
		{"span", 12, p(Class(para), Text("Hello "), span(Key("x"), Class("invisible absolute top-0 right-0 size-3 bg-rose-500")),
			span(Class("font-semibold"), Text("world")), Text(", a paragraph that wraps onto two lines.")), plain},
		{"div", 12, p(Class(para), Text("Hello "), div(Key("x"), Class("invisible absolute top-0 right-0 size-3 bg-rose-500")),
			span(Class("font-semibold"), Text("world")), Text(", a paragraph that wraps onto two lines.")), plain},
		{"fixed", 12, p(Class(para), Text("Hello "), span(Key("x"), Class("invisible fixed right-2 bottom-2 size-3 bg-sky-500")),
			span(Class("font-semibold"), Text("world")), Text(", a paragraph that wraps onto two lines.")), plain},
		{"in an inline", 8, p(Class(para), Text("Hello "),
			span(Class("font-semibold"), Text("world"), span(Key("x"), Class("invisible absolute -top-1 -right-1 size-2 bg-rose-500"))),
			Text(", a paragraph that wraps onto two lines.")), plain},
	} {
		sameImages(t, c.name, renderOOF(c.root), renderOOF(c.plain))
		tr := newTestTree(t, 240, 160, c.root, nil)
		tr.Frame()
		if r := keyedRect(t, tr, "x"); r[2]-r[0] != c.size || r[3]-r[1] != c.size {
			t.Errorf("%s: the positioned element is %v, want %g px square", c.name, r, c.size)
		}
	}
}

// TestOutOfFlowToggles: an element that becomes absolute or static in a
// paragraph restructures it, as a fresh mount would lay it out.
func TestOutOfFlowToggles(t *testing.T) {
	var set Setter[int]
	scene := func(kind style.Kind, abs bool) Node {
		cls := "size-3 bg-rose-500"
		if abs {
			cls += " absolute right-0 top-0"
		}
		return p(Class("relative m-2 w-52 bg-slate-100 text-sm"), Text("Hello "), Primitive(kind, Class(cls)), Text("world, and more text."))
	}
	for _, kind := range []style.Kind{style.KindSpan, style.KindDiv} {
		comp := func() Node {
			v, s := UseState(0)
			set = s
			return scene(kind, v%2 == 1)
		}
		tr := newTestTree(t, 240, 160, Component(comp), nil)
		tr.Frame()
		for v := 1; v <= 3; v++ {
			set.Set(v)
			dl, _ := tr.Frame()
			sameImages(t, fmt.Sprintf("%v after %d toggles", kind, v), tr.Rasterize(dl, 1), renderOOFAt1(scene(kind, v%2 == 1)))
		}
	}
}

// renderOOFAt1 renders root at 240x160 and scale 1.
func renderOOFAt1(root Node) *image.RGBA {
	_, img := Render(root, RenderOptions{Config: Config{Fonts: testFonts}, Width: 240, Height: 160, Scale: 1})
	return img
}
