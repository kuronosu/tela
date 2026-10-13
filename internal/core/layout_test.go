package core

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// textWidth returns the max-content width of s in the default style.
func textWidth(t *testing.T, s string) float32 {
	t.Helper()
	tr := newTestTree(t, 2000, 100, div(Class("flex"), div(Key("t"), Text(s))), nil)
	tr.Frame()
	return mustFind(t, tr, "t").Rect.Width()
}

func wantRect(t *testing.T, n *InspectNode, x, y, w, h float32) {
	t.Helper()
	const eps = 0.01
	r := n.Rect
	if !near(r.X0, x, eps) || !near(r.Y0, y, eps) || !near(r.Width(), w, eps) || !near(r.Height(), h, eps) {
		t.Errorf("%s %q: rect (%g,%g %gx%g), want (%g,%g %gx%g)", n.Kind, n.Key, r.X0, r.Y0, r.Width(), r.Height(), x, y, w, h)
	}
}

func TestFlexRowWithText(t *testing.T) {
	wa := textWidth(t, "Hello")
	root := div(Key("row"), Class("flex gap-2 p-4"),
		div(Key("a"), Text("Hello")),
		div(Key("b"), Class("grow"), Text("World")),
		span(Key("c"), Class("w-10"), Text("blockified span")),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	// A Span is a flex item (blockified) with its own paragraph, which
	// wraps in its 40px width.
	c := mustFind(t, tr, "c")
	if c.Kind != "span" || c.Rect.Width() != 40 || c.Lines < 2 || c.Rect.Height() != float32(24*c.Lines) {
		t.Errorf("span flex item: %+v", c)
	}
	// The items stretch to the row's height (align-items: normal).
	h := c.Rect.Height()
	wantRect(t, mustFind(t, tr, "a"), 16, 16, wa, h)
	// b grows into what a, c and the gaps leave.
	wantRect(t, mustFind(t, tr, "b"), 16+wa+8, 16, 400-32-wa-8-8-40, h)
	if row := mustFind(t, tr, "row"); row.Rect.Height() != 32+c.Rect.Height() {
		t.Errorf("row height %g, want padding + tallest item %g", row.Rect.Height(), 32+c.Rect.Height())
	}
}

func TestFlexColumnCentered(t *testing.T) {
	// The Counter of ARCHITECTURE section 1, without paint effects.
	root := div(Key("card"), Class("flex flex-col items-center gap-6 p-10"),
		h1(Key("title"), Class("text-5xl font-bold tracking-tight"), Text("Count: 0")),
		button(Key("btn"), Class("px-6 py-3"), Text("Increment")),
	)
	tr := newTestTree(t, 800, 600, root, nil)
	tr.Frame()
	card := mustFind(t, tr, "card")
	title := mustFind(t, tr, "title")
	btn := mustFind(t, tr, "btn")
	// text-5xl is 48px with line-height 1.
	if title.Rect.Height() != 48 || title.Rect.Y0 != 40 {
		t.Errorf("title %v, want y 40 height 48", title.Rect)
	}
	// Items are centered: fit-content widths around the card's center.
	for _, n := range []*InspectNode{title, btn} {
		if c := (n.Rect.X0 + n.Rect.X1) / 2; !near(c, 400, 0.01) {
			t.Errorf("%s center %g, want 400", n.Key, c)
		}
	}
	if btn.Rect.Y0 != 40+48+24 || btn.Rect.Height() != 48 {
		t.Errorf("button %v, want y 112 height 48 (py-3 + 24px line)", btn.Rect)
	}
	if card.Rect.Height() != 40+48+24+48+40 {
		t.Errorf("card height %g", card.Rect.Height())
	}
}

func TestInlineBlockButtonFitContent(t *testing.T) {
	w := textWidth(t, "OK")
	tr := newTestTree(t, 400, 300, div(
		button(Key("b1"), Class("px-4"), Text("OK")),
		div(Class("text-center"), button(Key("b2"), Class("px-4"), Text("OK"))),
		div(Class("text-right"), button(Key("b3"), Class("w-20"), Text("OK"))),
	), nil)
	tr.Frame()
	// Block flow: the button takes its fit-content width, not the
	// container's.
	wantRect(t, mustFind(t, tr, "b1"), 0, 0, w+32, 24)
	// text-align of the parent places it like an inline-block.
	wantRect(t, mustFind(t, tr, "b2"), (400-w-32)/2, 24, w+32, 24)
	wantRect(t, mustFind(t, tr, "b3"), 400-80, 48, 80, 24)
	// The button's own label is centered (browser default).
	if b3 := mustFind(t, tr, "b3"); b3.Style.TextAlign != "center" {
		t.Errorf("button text-align %q", b3.Style.TextAlign)
	}
}

func TestAnonymousParagraphs(t *testing.T) {
	tr := newTestTree(t, 400, 300, div(Key("d"),
		Text("before "), strong(Text("bold")),
		div(Key("blk"), Class("h-10")),
		Text("after"),
		Text("   "),
	), nil)
	tr.Frame()
	d := mustFind(t, tr, "d")
	if len(d.Children) != 3 {
		t.Fatalf("children:\n%s", dump(d))
	}
	a0, blk, a1 := &d.Children[0], &d.Children[1], &d.Children[2]
	if a0.Kind != "anonymous" || a0.Text != "before bold" || len(a0.Inline) != 1 || a0.Inline[0] != "strong" {
		t.Errorf("first run: %+v", a0)
	}
	wantRect(t, a0, 0, 0, 400, 24)
	wantRect(t, blk, 0, 24, 400, 40)
	if a1.Kind != "anonymous" || a1.Text != "after   " {
		t.Errorf("second run: %+v", a1)
	}
	wantRect(t, a1, 0, 64, 400, 24)
	wantRect(t, d, 0, 0, 400, 88)
}

func TestWhiteSpaceRunsDropped(t *testing.T) {
	tr := newTestTree(t, 400, 300, div(Key("d"),
		Text("  "), div(Class("h-4")), Text("\n "), div(Class("h-4")),
	), nil)
	tr.Frame()
	if d := mustFind(t, tr, "d"); len(d.Children) != 2 || d.Rect.Height() != 32 {
		t.Errorf("white-space runs make boxes:\n%s", dump(d))
	}
	// An empty block has no line box.
	tr = newTestTree(t, 400, 300, div(Key("e"), Text("")), nil)
	tr.Frame()
	if h := mustFind(t, tr, "e").Rect.Height(); h != 0 {
		t.Errorf("empty paragraph height %g, want 0", h)
	}
}

func TestOneParagraphForInlineContent(t *testing.T) {
	tr := newTestTree(t, 400, 300, p(Key("p"), Class("text-sm"),
		Text("a "), strong(Text("b "), em(Text("c"))), Text(" "), code(Text("d")),
	), nil)
	tr.Frame()
	n := mustFind(t, tr, "p")
	if n.Text != "a b c d" || len(n.Children) != 0 {
		t.Errorf("paragraph: %+v", n)
	}
	if want := []string{"strong", "em", "code"}; len(n.Inline) != 3 || n.Inline[0] != want[0] || n.Inline[1] != want[1] || n.Inline[2] != want[2] {
		t.Errorf("inline elements %v, want %v", n.Inline, want)
	}
	// text-sm: 14px / 20px.
	if n.Rect.Height() != 20 {
		t.Errorf("height %g, want 20", n.Rect.Height())
	}
	// A block inside an inline element makes it a block.
	tr = newTestTree(t, 400, 300, div(Key("d"), Text("x"), span(Key("s"), div(Class("h-4")))), nil)
	tr.Frame()
	if d := mustFind(t, tr, "d"); len(d.Children) != 2 || d.Children[1].Kind != "span" {
		t.Errorf("span with a block child:\n%s", dump(d))
	}
}

func TestFlexTextRuns(t *testing.T) {
	tr := newTestTree(t, 400, 300, div(Key("f"), Class("flex"),
		Text("a"), Text("b"), span(Text("c")), Text(" "),
	), nil)
	tr.Frame()
	f := mustFind(t, tr, "f")
	if len(f.Children) != 2 || f.Children[0].Kind != "anonymous" || f.Children[0].Text != "ab" || f.Children[1].Kind != "span" {
		t.Errorf("flex items:\n%s", dump(f))
	}
}

func TestSpaceUtilities(t *testing.T) {
	block := div(Key("b"), Class("space-y-4"),
		div(Key("b0"), Class("h-10")), div(Key("b1"), Class("h-10 mb-2")),
		div(Class("absolute")), div(Key("b2"), Class("h-10")))
	col := div(Key("c"), Class("flex flex-col space-y-4"),
		div(Key("c0"), Class("h-10")), div(Key("c1"), Class("h-10")))
	row := div(Class("space-x-4"), div(Key("r0"), Class("h-4")), div(Key("r1"), Class("h-4")))
	tr := newTestTree(t, 400, 600, div(block, col, row), nil)
	tr.Frame()
	wantRect(t, mustFind(t, tr, "b0"), 0, 0, 400, 40)
	wantRect(t, mustFind(t, tr, "b1"), 0, 56, 400, 40)
	// Margins and the gap add up (b1 has mb-2, 8px); the absolute child
	// does not take part.
	wantRect(t, mustFind(t, tr, "b2"), 0, 56+40+8+16, 400, 40)
	wantRect(t, mustFind(t, tr, "b"), 0, 0, 400, 160)
	wantRect(t, mustFind(t, tr, "c1"), 0, 160+56, 400, 40)
	// space-x in block flow: the end margin of every child but the last.
	wantRect(t, mustFind(t, tr, "r0"), 0, 256, 384, 16)
	wantRect(t, mustFind(t, tr, "r1"), 0, 272, 400, 16)

	// Between inline elements of a paragraph it is an inline space.
	wa, wb := textWidth(t, "a"), textWidth(t, "b")
	tr = newTestTree(t, 400, 600, div(Class("flex"), div(Key("p"), Class("space-x-4"), span(Text("a")), span(Text("b")))), nil)
	tr.Frame()
	if w := mustFind(t, tr, "p").Rect.Width(); !near(w, wa+wb+16, 0.01) {
		t.Errorf("space-x between spans: width %g, want %g", w, wa+wb+16)
	}
}

func TestDisplayContentsAndNone(t *testing.T) {
	tr := newTestTree(t, 400, 300, div(Key("f"), Class("flex"),
		div(Class("contents"), div(Key("c1"), Class("w-10 h-10")), div(Key("c2"), Class("w-10 h-10"))),
		div(Key("gone"), Class("hidden w-10 h-10")),
		div(Key("c3"), Class("w-10 h-10")),
	), nil)
	tr.Frame()
	f := mustFind(t, tr, "f")
	if len(f.Children) != 3 {
		t.Fatalf("flex items:\n%s", dump(f))
	}
	wantRect(t, mustFind(t, tr, "c1"), 0, 0, 40, 40)
	wantRect(t, mustFind(t, tr, "c2"), 40, 0, 40, 40)
	wantRect(t, mustFind(t, tr, "c3"), 80, 0, 40, 40)
	root := tr.Inspect()
	if find(&root, "gone") != nil {
		t.Error("a display: none element has a box")
	}

	// A hidden inline element does not split its paragraph, and showing
	// it again restores its text.
	var setHidden func(bool)
	app := func() Node {
		hidden, set := UseState(true)
		setHidden = set
		cls := "inline"
		if hidden {
			cls = "hidden"
		}
		return div(Key("d"), Text("a"), span(Class(cls), Text("b"), div(Key("inner"), Class("h-4"))), Text("c"))
	}
	tr = newTestTree(t, 400, 300, Component(app), nil)
	tr.Frame()
	if d := mustFind(t, tr, "d"); d.Text != "ac" || len(d.Children) != 0 {
		t.Errorf("hidden span:\n%s", dump(d))
	}
	setHidden(false)
	tr.Frame()
	// Shown, the span holds a block: it is a block-level box.
	d := mustFind(t, tr, "d")
	if len(d.Children) != 3 || d.Children[1].Kind != "span" || len(d.Children[1].Children) != 2 || d.Children[1].Children[1].Key != "inner" {
		t.Errorf("shown span:\n%s", dump(d))
	}
	setHidden(true)
	tr.Frame()
	if d := mustFind(t, tr, "d"); d.Text != "ac" || len(d.Children) != 0 {
		t.Errorf("hidden again:\n%s", dump(d))
	}
}

func TestDisplayChangeRestructures(t *testing.T) {
	var setInline func(bool)
	app := func() Node {
		inline, set := UseState(true)
		setInline = set
		cls := "inline"
		if !inline {
			cls = "block"
		}
		return div(Key("d"), Text("a"), span(Key("s"), Class(cls), Text("b")), Text("c"))
	}
	tr := newTestTree(t, 400, 300, Component(app), nil)
	tr.Frame()
	if d := mustFind(t, tr, "d"); d.Text != "abc" || len(d.Children) != 0 {
		t.Fatalf("inline span:\n%s", dump(d))
	}
	setInline(false)
	tr.Frame()
	d := mustFind(t, tr, "d")
	if len(d.Children) != 3 || d.Children[1].Key != "s" || d.Children[1].Text != "b" {
		t.Fatalf("block span:\n%s", dump(d))
	}
	setInline(true)
	tr.Frame()
	if d := mustFind(t, tr, "d"); d.Text != "abc" || len(d.Children) != 0 {
		t.Errorf("inline again:\n%s", dump(d))
	}
}

func TestImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	src.Set(0, 0, color.RGBA{255, 0, 0, 255})
	tr := newTestTree(t, 400, 300, div(
		img(Key("natural"), Src(src)),
		img(Key("w32"), Class("w-32"), Src(src)),
		div(Class("w-10"), img(Key("max"), Src(src))),
	), nil)
	dl, _ := tr.Frame()
	wantRect(t, mustFind(t, tr, "natural"), 0, 0, 40, 20)
	wantRect(t, mustFind(t, tr, "w32"), 0, 20, 128, 64)
	// Preflight's max-width: 100% keeps the image inside its container.
	wantRect(t, mustFind(t, tr, "max"), 0, 84, 40, 20)
	draws := 0
	for _, c := range dl.All() {
		if _, ok := c.(gfx.DrawImage); ok {
			draws++
		}
	}
	if draws != 3 {
		t.Errorf("%d DrawImage commands, want 3", draws)
	}
}

func TestParagraphReshapeVersusRebreak(t *testing.T) {
	var setText func(string)
	var setCls func(string)
	app := func() Node {
		s, st := UseState("The quick brown fox jumps over the lazy dog, again and again.")
		cls, sc := UseState("text-slate-900")
		setText, setCls = st, sc
		return p(Key("p"), Class(cls), Text(s))
	}
	tr := newTestTree(t, 400, 300, Component(app), nil)
	tr.Frame()
	s0 := tr.Stats()
	if s0.Shapes != 1 {
		t.Fatalf("%d shapes after mount, want 1", s0.Shapes)
	}
	lines := mustFind(t, tr, "p").Lines

	// A narrower viewport breaks the lines again without shaping.
	tr.SetViewport(gfx.Size{W: 150, H: 300})
	tr.Frame()
	s1 := tr.Stats()
	if s1.Shapes != s0.Shapes {
		t.Errorf("a width change shaped the paragraph again")
	}
	if s1.Breaks == s0.Breaks {
		t.Errorf("a width change did not break the lines again")
	}
	if n := mustFind(t, tr, "p").Lines; n <= lines {
		t.Errorf("%d lines at 150px, %d at 400px", n, lines)
	}

	// A frame with nothing dirty does nothing.
	tr.Frame()
	if s := tr.Stats(); s != s1 {
		t.Errorf("an idle frame did work: %+v -> %+v", s1, s)
	}

	// A background change neither shapes nor breaks.
	setCls("text-slate-900 bg-white")
	tr.Frame()
	s2 := tr.Stats()
	if s2.Shapes != s1.Shapes || s2.Breaks != s1.Breaks {
		t.Errorf("a background change: shapes %d -> %d, breaks %d -> %d", s1.Shapes, s2.Shapes, s1.Breaks, s2.Breaks)
	}

	// New text, or a new text style, shapes once.
	setText("Short.")
	tr.Frame()
	s3 := tr.Stats()
	if s3.Shapes != s2.Shapes+1 {
		t.Errorf("a text change: %d shapes, want 1", s3.Shapes-s2.Shapes)
	}
	setCls("text-red-500")
	tr.Frame()
	if s := tr.Stats(); s.Shapes != s3.Shapes+1 {
		t.Errorf("a color change: %d shapes, want 1", s.Shapes-s3.Shapes)
	}
}

func TestInspectJSON(t *testing.T) {
	tr := newTestTree(t, 200, 100, div(Key("a"), Class("m-2 w-20 h-10 bg-red-500"), Text("a & b <c>")), nil)
	tr.Frame()
	root := tr.Inspect()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // as tela inspect does
	if err := enc.Encode(root); err != nil {
		t.Fatal(err)
	}
	js := buf.Bytes()
	var got struct {
		Kind     string `json:"kind"`
		Children []struct {
			Key  string `json:"key"`
			Rect struct{ X, Y, Width, Height float32 }
		} `json:"children"`
	}
	if err := json.Unmarshal(js, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "root" || len(got.Children) != 1 {
		t.Fatalf("unexpected JSON: %s", js)
	}
	a := got.Children[0]
	if a.Key != "a" || a.Rect.X != 8 || a.Rect.Y != 8 || a.Rect.Width != 80 || a.Rect.Height != 40 {
		t.Errorf("rect of a = %+v in %s, want x 8 y 8 width 80 height 40", a, js)
	}
	// Field order is part of the contract: kind first, rect before the
	// edges, children last. Zero edges are left out (the root has them
	// all zero, "a" has a margin).
	s := string(js)
	if !(strings.Index(s, `"kind"`) < strings.Index(s, `"rect"`) && strings.Index(s, `"rect"`) < strings.Index(s, `"margin"`) && strings.Index(s, `"style"`) < strings.Index(s, `"children"`)) {
		t.Errorf("unexpected field order: %s", s)
	}
	if strings.Count(s, `"margin"`) != 1 || strings.Contains(s, `"padding"`) || strings.Contains(s, `"border"`) {
		t.Errorf("zero edges must be omitted: %s", s)
	}
	if !strings.Contains(s, `"text":"a & b <c>"`) {
		t.Errorf("text must not be HTML-escaped: %s", s)
	}
}

// Lengths with a pixel and a percentage part reach layout as calc values:
// sizes, margins, padding, insets, and a percentage space utility added to
// a margin.
func TestCalcLengths(t *testing.T) {
	tr := newTestTree(t, 400, 600, div(
		div(Key("w"), Class("h-4 w-[calc(100%-2rem)] ml-[calc(5%+2px)]")),
		div(Key("p"), Class("h-4 pl-[calc(10%-4px)]"), div(Key("pc"), Class("h-2"))),
		div(Key("rel"), Class("relative h-20"),
			div(Key("abs"), Class("absolute left-[calc(25%-10px)] top-[calc(50%+1px)] size-4"))),
		div(Key("sp"), Class("space-y-[5%]"), div(Key("s0"), Class("h-4 mb-[3px]")), div(Key("s1"), Class("h-4"))),
	), nil)
	tr.Frame()
	wantRect(t, mustFind(t, tr, "w"), 22, 0, 368, 16)
	wantRect(t, mustFind(t, tr, "pc"), 36, 16, 364, 8)
	wantRect(t, mustFind(t, tr, "abs"), 90, 32+41, 16, 16)
	// space-y-[5%] adds 5% of the container's width (20px) to s0's 3px.
	wantRect(t, mustFind(t, tr, "s1"), 0, 112+16+3+20, 400, 16)
}
