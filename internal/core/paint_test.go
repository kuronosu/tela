package core

import (
	"image"
	"regexp"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform/headless"
)

var boundsRE = regexp.MustCompile(` bounds=\[[^\]]*\]`)

// paintDump renders root in a w x h viewport at scale and returns the
// display list's dump without bounds and without the canvas fill.
func paintDump(t *testing.T, w, h, scale float32, root Node) string {
	t.Helper()
	tr := newTestTree(t, w, h, root, nil)
	tr.SetScale(scale)
	dl, _ := tr.Frame()
	s := boundsRE.ReplaceAllString(dl.String(), "")
	_, rest, _ := strings.Cut(s, "\n") // the canvas
	return rest
}

// order checks that the substrings appear in s in this order.
func order(t *testing.T, s string, subs ...string) {
	t.Helper()
	at := 0
	for _, sub := range subs {
		i := strings.Index(s[at:], sub)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", sub, s)
		}
		at += i + len(sub)
	}
}

func hex(class string) string {
	c, ok := testHex[class]
	if !ok {
		panic(class)
	}
	return c
}

var testHex = map[string]string{
	"bg-red-500": "#fb2c36", "bg-sky-500": "#00a6f4", "bg-amber-400": "#ffba00",
	"bg-pink-500": "#f6339a", "bg-slate-200": "#e2e8f0", "bg-white": "#ffffff",
}

func TestPaintStackingOrder(t *testing.T) {
	// Positioned boxes paint after in-flow ones: auto/0 in tree order,
	// then positive z-index ascending; negative ones first.
	s := paintDump(t, 200, 200, 1, div(Class("relative isolate"),
		div(Class("absolute z-20 size-4 bg-sky-500")),
		div(Class("absolute z-10 size-4 bg-pink-500")),
		div(Class("absolute -z-10 size-4 bg-amber-400")),
		div(Class("absolute size-4 bg-red-500")),
		div(Class("size-4 bg-slate-200")),
	))
	order(t, s, hex("bg-amber-400"), hex("bg-slate-200"), hex("bg-red-500"), hex("bg-pink-500"), hex("bg-sky-500"))

	// z-index applies to flex items without position, not to static
	// blocks.
	s = paintDump(t, 200, 200, 1, div(Class("flex"),
		div(Class("z-10 size-4 bg-sky-500")),
		div(Class("size-4 bg-pink-500")),
	))
	order(t, s, hex("bg-pink-500"), hex("bg-sky-500"))
	s = paintDump(t, 200, 200, 1, div(
		div(Class("z-10 size-4 bg-sky-500")),
		div(Class("size-4 bg-pink-500")),
	))
	order(t, s, hex("bg-sky-500"), hex("bg-pink-500"))

	// A positioned box without z-index does not contain the positioned
	// boxes inside it: a later z-10 sibling of it paints after its
	// child, and its z-20 child after that sibling.
	s = paintDump(t, 200, 200, 1, div(Class("relative isolate"),
		div(Class("relative size-8 bg-slate-200"), div(Class("absolute z-20 size-4 bg-sky-500"))),
		div(Class("absolute z-10 size-4 bg-pink-500")),
	))
	order(t, s, hex("bg-slate-200"), hex("bg-pink-500"), hex("bg-sky-500"))

	// A stacking context keeps its descendants: the z-20 child of a z-10
	// box paints below a later z-15 sibling.
	s = paintDump(t, 200, 200, 1, div(Class("relative isolate"),
		div(Class("relative z-10 size-8 bg-slate-200"), div(Class("absolute z-20 size-4 bg-sky-500"))),
		div(Class("absolute z-[15] size-4 bg-pink-500")),
	))
	order(t, s, hex("bg-slate-200"), hex("bg-sky-500"), hex("bg-pink-500"))
}

func TestPaintOverflowClip(t *testing.T) {
	// The clip is the padding box with the inner radii (16 - 4).
	s := paintDump(t, 200, 200, 1, div(Class("m-2 size-20 overflow-hidden rounded-2xl border-4 border-white"),
		div(Class("size-40 bg-sky-500"))))
	order(t, s, "PushClipRRect rect=[12,12,84,84] r=12", hex("bg-sky-500"), "PopClip")

	// An absolute child escapes the clip of a static ancestor between it
	// and its containing block, but not the clip of its containing block.
	s = paintDump(t, 200, 200, 1, div(Class("relative"),
		div(Class("size-20 overflow-hidden"), div(Class("absolute size-40 bg-sky-500")))))
	if strings.Contains(s, "PushClip") {
		t.Errorf("absolute child clipped by a static ancestor:\n%s", s)
	}
	s = paintDump(t, 200, 200, 1, div(Class("relative size-20 overflow-hidden"),
		div(div(Class("absolute size-40 bg-sky-500")))))
	order(t, s, "PushClipRect rect=[0,0,80,80]", hex("bg-sky-500"), "PopClip")

	// overflow-x: clip alone clips one axis.
	s = paintDump(t, 200, 200, 1, div(Class("size-20 overflow-x-clip"), div(Class("size-40 bg-sky-500"))))
	order(t, s, "PushClipRect rect=[0,-16777216,80,16777216]", hex("bg-sky-500"))
}

func TestPaintSnapping(t *testing.T) {
	// A box at x = 10.4 with a 1.5px border: at scale 1 its edges round
	// to whole pixels and the border floors to 1px; at scale 2 to half
	// pixels and 1.5px.
	root := div(Class("ml-[10.4px] mt-[0.25px] h-[10.6px] w-[20.3px] border-[1.5px] border-black bg-sky-500"))
	s := paintDump(t, 100, 100, 1, root)
	// Rounding is half up: y = 0.25 is 0 at scale 1 and 0.5 at scale 2.
	order(t, s, "FillRect rect=[10,0,31,11] paint="+hex("bg-sky-500"), "FillPath")
	if !strings.Contains(s, "M10,0 L31,0 L31,11 L10,11 Z M30,1 L11,1 L11,10 L30,10 Z") {
		t.Errorf("border at scale 1:\n%s", s)
	}
	s = paintDump(t, 100, 100, 2, root)
	order(t, s, "FillRect rect=[10.5,0.5,30.5,11] paint="+hex("bg-sky-500"))
	if !strings.Contains(s, "M10.5,0.5 L30.5,0.5 L30.5,11 L10.5,11 Z M29,2 L12,2 L12,9.5 L29,9.5 Z") {
		t.Errorf("border at scale 2:\n%s", s)
	}
}

func TestPaintScaleRepaints(t *testing.T) {
	tr := newTestTree(t, 100, 100, div(Class("ml-[10.4px] size-4 bg-sky-500")), nil)
	dl1, _ := tr.Frame()
	if _, changed := tr.Frame(); changed {
		t.Fatal("idle frame repainted")
	}
	tr.SetScale(2)
	if !tr.Dirty() {
		t.Fatal("a new scale does not dirty the tree")
	}
	dl2, changed := tr.Frame()
	if !changed || dl1.String() == dl2.String() {
		t.Errorf("a new scale did not repaint with new snapping:\n%s", dl2)
	}
}

func TestPaintEffects(t *testing.T) {
	s := paintDump(t, 200, 200, 1, div(Class("p-4 bg-white"),
		div(Class("size-10 rounded-lg bg-sky-500/50 opacity-50 rotate-90 backdrop-blur blur-sm shadow-md"))))
	// Transform, then backdrop, then one layer with opacity and the blur
	// filter, the shadow under the background.
	order(t, s, "PushTransform", "BackdropFilter", "PushLayer opacity=0.5", "DrawShadow", "FillRRect", "PopLayer", "PopTransform")

	// visibility: hidden skips the box but not its visible child.
	s = paintDump(t, 200, 200, 1, div(Class("invisible size-10 bg-red-500"), div(Class("visible size-4 bg-sky-500"))))
	if strings.Contains(s, hex("bg-red-500")) || !strings.Contains(s, hex("bg-sky-500")) {
		t.Errorf("visibility:\n%s", s)
	}
}

func TestPaintBackgrounds(t *testing.T) {
	// bg-clip-padding paints inside the border.
	s := paintDump(t, 200, 200, 1, div(Class("size-10 border-4 border-transparent bg-sky-500 bg-clip-padding")))
	order(t, s, "FillRect rect=[4,4,36,36] paint="+hex("bg-sky-500"))

	// A gradient's tile is the padding box (background-origin) and it
	// repeats under the border: nine clipped tiles.
	s = paintDump(t, 200, 200, 1, div(Class("size-10 border-4 border-transparent bg-linear-to-r from-sky-500 to-pink-500")))
	if n := strings.Count(s, "paint=linear("); n != 9 {
		t.Errorf("%d gradient tiles, want 9:\n%s", n, s)
	}
	if n := strings.Count(s, "PushClipRect"); n != 9 {
		t.Errorf("%d tile clips, want 9:\n%s", n, s)
	}
	order(t, s, "PushClipRect rect=[4,4,36,36]", "paint=linear((4,20)->(36,20)")
	// Without a border, one tile and no clip.
	s = paintDump(t, 200, 200, 1, div(Class("size-10 rounded-lg bg-linear-to-r from-sky-500 to-pink-500")))
	if strings.Count(s, "paint=linear(") != 1 || strings.Contains(s, "PushClip") {
		t.Errorf("single tile:\n%s", s)
	}
}

func TestPaintBordersAndOutline(t *testing.T) {
	// Sides of different colors are clipped per side; one color is one
	// fill.
	s := paintDump(t, 200, 200, 1, div(Class("size-10 border-4 border-sky-500 border-t-pink-500")))
	if n := strings.Count(s, "PushClipPath"); n != 4 {
		t.Errorf("%d side clips, want 4:\n%s", n, s)
	}
	s = paintDump(t, 200, 200, 1, div(Class("size-10 rounded-lg border-t-2 border-b-8 border-sky-500")))
	if strings.Count(s, "FillPath") != 1 || strings.Contains(s, "PushClip") {
		t.Errorf("one-color border:\n%s", s)
	}
	// The outline follows the radius: 8 + offset 2 inside, + 2 outside.
	s = paintDump(t, 200, 200, 1, div(Class("m-10 size-10 rounded-lg outline-2 outline-offset-2 outline-sky-500")))
	if !strings.Contains(s, "FillPath") {
		t.Fatalf("no outline:\n%s", s)
	}
	dl := func() *gfx.DisplayList {
		tr := newTestTree(t, 200, 200, div(Class("m-10 size-10 rounded-lg outline-2 outline-offset-2 outline-sky-500")), nil)
		dl, _ := tr.Frame()
		return dl
	}()
	var fp gfx.FillPath
	for _, c := range dl.All() {
		if f, ok := c.(gfx.FillPath); ok {
			fp = f
		}
	}
	if b := fp.Path.Bounds(); b != (gfx.Rect{X0: 36, Y0: 36, X1: 84, Y1: 84}) {
		t.Errorf("outline bounds %v, want [36,36,84,84]", b)
	}
}

func TestPaintImageRadius(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	// An avatar: the image is clipped to the content box's corners.
	s := paintDump(t, 100, 100, 1, img(Src(src), Class("size-10 rounded-full")))
	order(t, s, "PushClipRRect rect=[0,0,40,40] r=20", "DrawImage", "PopClip")
	// Square corners need no clip.
	s = paintDump(t, 100, 100, 1, img(Src(src), Class("size-10")))
	if strings.Contains(s, "PushClip") || !strings.Contains(s, "DrawImage") {
		t.Errorf("square image:\n%s", s)
	}
}

// TestHostScale checks that a window moving to another scale repaints with
// the new snapping.
func TestHostScale(t *testing.T) {
	b, h := newHeadlessHost(t, div(Class("ml-[10.25px] size-4 bg-sky-500")))
	win := h.Window().(*headless.Window)
	if s := h.Tree().Scale(); s != 1 {
		t.Fatalf("scale %v, want 1", s)
	}
	paints := h.Tree().Stats().Paints
	win.SetScale(2)
	b.Drain(h)
	if s := h.Tree().Scale(); s != 2 {
		t.Errorf("scale %v after SetScale(2), want 2", s)
	}
	if h.Tree().Stats().Paints == paints {
		t.Error("the new scale did not repaint")
	}
	if h.Presents() != 2 {
		t.Errorf("presents = %d, want 2", h.Presents())
	}
}
