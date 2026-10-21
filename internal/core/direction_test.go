package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// dirRect returns the window rect of the primitive with key k.
func dirRect(t *testing.T, tr *Tree, k any) gfx.Rect {
	t.Helper()
	el := keyed(tr, k)
	if el == nil {
		t.Fatalf("no element %v", k)
	}
	return tr.boxWindowRect(el.box)
}

func TestDirMirrorsFlexAndLogicalUtilities(t *testing.T) {
	row := func(dir string) Node {
		return div(Key("row"), Dir(dir), Class("flex w-[300px] gap-2 ps-4 pe-1"),
			div(Key("a"), Class("w-10 h-4 me-3")),
			div(Key("b"), Class("w-20 h-4")),
		)
	}
	for _, c := range []struct {
		dir        string
		ax, bx     float32
		padL, padR float32
	}{
		{"", 16, 16 + 40 + 12 + 8, 16, 4},
		{"ltr", 16, 16 + 40 + 12 + 8, 16, 4},
		{"rtl", 300 - 16 - 40, 300 - 16 - 40 - 12 - 8 - 80, 4, 16},
		{"RTL", 300 - 16 - 40, 300 - 16 - 40 - 12 - 8 - 80, 4, 16},
	} {
		tr := newTestTree(t, 400, 100, row(c.dir), nil)
		tr.Frame()
		if a := dirRect(t, tr, "a"); a.X0 != c.ax {
			t.Errorf("dir %q: a at x=%v, want %v", c.dir, a.X0, c.ax)
		}
		if b := dirRect(t, tr, "b"); b.X0 != c.bx {
			t.Errorf("dir %q: b at x=%v, want %v", c.dir, b.X0, c.bx)
		}
		cs := keyed(tr, "row").box.cs
		if cs.Padding.Left.Px != c.padL || cs.Padding.Right.Px != c.padR {
			t.Errorf("dir %q: padding %v/%v, want %v/%v", c.dir, cs.Padding.Left.Px, cs.Padding.Right.Px, c.padL, c.padR)
		}
	}
}

func TestDirInheritedAndSwitched(t *testing.T) {
	var setDir Setter[string]
	app := Component(func() Node {
		d, set := UseState("ltr")
		setDir = set
		return div(Key("root"), Dir(d),
			div(Key("mid"), Class("flex w-[200px] space-x-4"),
				div(Key("a"), Class("w-10 h-4 rtl:h-8")),
				div(Key("b"), Class("w-10 h-4")),
			),
			div(Key("ltr"), Dir("ltr"), Class("flex w-[200px]"), div(Key("c"), Class("size-4"))),
		)
	})
	tr := newTestTree(t, 400, 200, app, nil)
	tr.Frame()
	if a, b := dirRect(t, tr, "a"), dirRect(t, tr, "b"); a.X0 != 0 || b.X0 != 56 || a.Height() != 16 {
		t.Fatalf("ltr: a %v b %v", a, b)
	}
	setDir.Set("rtl")
	tr.Frame()
	a, b := dirRect(t, tr, "a"), dirRect(t, tr, "b")
	// The 200px row sits at the right of the 400px root (block layout
	// in rtl), and space-x-4 is margin-inline-end: on the left of a.
	if a.X0 != 360 || b.X0 != 304 {
		t.Errorf("rtl: a at %v, b at %v; want 360 and 304", a.X0, b.X0)
	}
	if a.Height() != 32 {
		t.Errorf("rtl: variant not applied to a descendant: height %v", a.Height())
	}
	if c := dirRect(t, tr, "c"); c.X0 != 200 {
		t.Errorf("Dir(ltr) inside rtl: c at %v, want 200 (the start of its ltr row at the right)", c.X0)
	}
	if d := keyed(tr, "mid").box.cs.Text.Direction; d != text.DirectionRTL {
		t.Errorf("inherited direction %v", d)
	}
	setDir.Set("")
	tr.Frame()
	if a := dirRect(t, tr, "a"); a.X0 != 0 || a.Height() != 16 {
		t.Errorf("back to inherited ltr: a %v", a)
	}
}

func TestDirParagraphs(t *testing.T) {
	root := div(Class("w-[300px]"),
		p(Key("rtl"), Dir("rtl"), Class("text-base"), Text("Hello")),
		p(Key("ltr"), Class("text-base"), Text("Hello")),
		p(Key("start"), Dir("rtl"), Class("text-start"), Text("Hello")),
		p(Key("left"), Dir("rtl"), Class("text-left"), Text("Hello")),
		p(Key("end"), Dir("rtl"), Class("text-end"), Text("Hello")),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	ink := func(k string) float32 {
		para := keyed(tr, k).box.para
		if para == nil || para.lay == nil {
			t.Fatalf("%s has no paragraph", k)
		}
		if para.lay.RTL() != (k != "ltr") {
			t.Errorf("%s: paragraph RTL %v", k, para.lay.RTL())
		}
		return para.lay.Caret(text.Cursor{}).X
	}
	// The caret before "H": at the start of the line, which a
	// right-to-left paragraph aligns to the right.
	if x := ink("rtl"); x < 200 {
		t.Errorf("rtl paragraph starts at %v, want right aligned", x)
	}
	if x := ink("start"); x < 200 {
		t.Errorf("rtl text-start starts at %v, want right aligned", x)
	}
	if x := ink("ltr"); x != 0 {
		t.Errorf("ltr paragraph starts at %v", x)
	}
	if x := ink("left"); x != 0 {
		t.Errorf("rtl text-left starts at %v, want 0", x)
	}
	if x := ink("end"); x != 0 {
		t.Errorf("rtl text-end starts at %v, want 0", x)
	}
}

func TestDirAuto(t *testing.T) {
	var setText Setter[string]
	app := Component(func() Node {
		s, set := UseState("مرحبا world")
		setText = set
		return div(Class("w-[300px]"),
			div(Key("auto"), Dir("auto"), Class("flex rtl:pt-2"),
				span(Text(s)),
				div(Key("box"), Class("size-4")),
			),
			// Text below an element with its own Dir does not count.
			div(Key("skip"), Dir("auto"), div(Dir("rtl"), Text("שלום")), Text("hi")),
			input(Key("in"), Dir("auto"), DefaultValue("abc")),
		)
	})
	x := newKeys(t, app, 400, 200)
	tr := x.tree()
	dirOf := func(k string) text.Direction { return keyed(tr, k).box.cs.Text.Direction }
	if dirOf("auto") != text.DirectionRTL {
		t.Errorf("auto with Arabic first: %v", dirOf("auto"))
	}
	if dirOf("skip") != text.DirectionLTR {
		t.Errorf("auto skipping a Dir subtree: %v", dirOf("skip"))
	}
	setText.Set("hello مرحبا")
	x.frame()
	if dirOf("auto") != text.DirectionLTR {
		t.Errorf("auto after the text changed to Latin first: %v", dirOf("auto"))
	}
	if keyed(tr, "auto").box.cs.Padding.Top.Px != 0 {
		t.Error("rtl: variant still applied after auto turned ltr")
	}
	setText.Set("١٢٣ عربي")
	x.frame()
	if dirOf("auto") != text.DirectionRTL {
		t.Errorf("auto back to Arabic: %v", dirOf("auto"))
	}
	if dirOf("in") != text.DirectionLTR {
		t.Errorf("Input auto with Latin value: %v", dirOf("in"))
	}
	// Typing Arabic at the start of the field turns it right to left.
	x.tab()
	x.press("Home", 0, 0)
	x.typeText("س")
	x.frame()
	if dirOf("in") != text.DirectionRTL {
		t.Errorf("Input auto after typing Arabic first: %v (value %q)", dirOf("in"), x.value("in"))
	}
}

// TestDirAutoRechecksOnlyOnTextChange: the text below a Dir("auto")
// element is walked again only when something below it changed (a text, a
// TextFn's Value, children, a child's Dir), not at every style phase.
func TestDirAutoRechecksOnlyOnTextChange(t *testing.T) {
	var (
		setCls      Setter[string]
		setInnerDir Setter[string]
		setShowHeb  Setter[bool]
		v           *Value[string]
	)
	page := func() Node {
		cls, sc := UseState("size-4")
		innerDir, sd := UseState("rtl")
		showHeb, sh := UseState(false)
		val := UseValue("hello")
		setCls, setInnerDir, setShowHeb, v = sc, sd, sh, val
		var heb Node
		if showHeb {
			heb = Text("שלום ")
		}
		return div(
			div(Key("auto"), Dir("auto"), heb,
				div(Dir(innerDir), Text("עברית")),
				span(TextFn(func() string { return val.Get() })),
			),
			div(Key("other"), Class(cls)),
		)
	}
	tr := newTestTree(t, 400, 200, Component(page), nil)
	tr.Frame()
	dir := func() text.Direction { return keyed(tr, "auto").box.cs.Text.Direction }
	step := func(name string, want text.Direction, maxWalks int) {
		t.Helper()
		before := tr.autoWalks
		tr.Frame()
		if got := dir(); got != want {
			t.Errorf("%s: direction %v, want %v", name, got, want)
		}
		if n := tr.autoWalks - before; n > maxWalks {
			t.Errorf("%s: %d walks of the text, want at most %d", name, n, maxWalks)
		}
	}
	step("settled", text.DirectionLTR, 0)
	// A frame that changes nothing below the element walks nothing.
	setCls.Set("size-6")
	step("unrelated restyle", text.DirectionLTR, 0)
	setCls.Set("size-8 bg-red-500")
	step("unrelated restyle again", text.DirectionLTR, 0)
	// A TextFn's Value.
	v.Set("مرحبا")
	step("TextFn turned Arabic", text.DirectionRTL, 2)
	v.Set("hello")
	step("TextFn back to Latin", text.DirectionLTR, 2)
	// A child's Dir: the Hebrew below it counts once it has none.
	setInnerDir.Set("")
	step("inner Dir removed", text.DirectionRTL, 2)
	setInnerDir.Set("ltr")
	step("inner Dir ltr", text.DirectionLTR, 2)
	// A child inserted before the rest.
	setShowHeb.Set(true)
	step("Hebrew inserted first", text.DirectionRTL, 2)
	setCls.Set("size-4")
	step("unrelated restyle after", text.DirectionRTL, 0)
}

func TestDirInlineBlockAlignment(t *testing.T) {
	// Inline-blocks in a block flow next to block siblings are placed by
	// emulated text-align: the inline start of the parent's direction.
	root := div(Class("w-[300px]"),
		div(Key("rtl"), Dir("rtl"), div(Class("h-2")), button(Key("b1"), Class("w-10 h-4 absolute"))),
		div(Dir("rtl"), Class("text-left"), div(Class("h-2")), div(Key("b2"), Class("inline-block w-10 h-4"))),
		div(Dir("rtl"), div(Class("h-2")), div(Key("b3"), Class("inline-block w-10 h-4"))),
		div(Dir("rtl"), Class("text-end"), div(Class("h-2")), div(Key("b4"), Class("inline-block w-10 h-4"))),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	if r := dirRect(t, tr, "b2"); r.X0 != 0 {
		t.Errorf("rtl text-left inline-block at %v, want 0", r.X0)
	}
	if r := dirRect(t, tr, "b3"); r.X0 != 260 {
		t.Errorf("rtl inline-block at %v, want 260", r.X0)
	}
	if r := dirRect(t, tr, "b4"); r.X0 != 0 {
		t.Errorf("rtl text-end inline-block at %v, want 0", r.X0)
	}
	if r := dirRect(t, tr, "b1"); r.X0 != 260 {
		t.Errorf("rtl absolute static position at %v, want 260", r.X0)
	}
}

func TestDirInput(t *testing.T) {
	// A right-to-left Input aligns its line right and scrolls it to the
	// left (negative offsets) to keep the caret in view.
	long := "هذا نص عربي طويل جدا لا يتسع في الحقل الضيق"
	root := div(input(Key("in"), Dir("rtl"), Class("w-[120px] px-2"), DefaultValue(long)),
		input(Key("short"), Dir("rtl"), Class("w-[120px] px-2"), DefaultValue("نص")))
	x := newKeys(t, root, 400, 200)
	tr := x.tree()
	short := keyed(tr, "short").box.fld
	if lay := short.layout(); !lay.RTL() || lay.Caret(text.Cursor{}).X < 50 {
		t.Errorf("a short rtl value: RTL %v, start caret at %v (want right aligned)", lay.RTL(), lay.Caret(text.Cursor{}).X)
	}
	x.tab()
	x.press("End", 0, 0)
	x.frame()
	f := keyed(tr, "in").box.fld
	lay := f.layout()
	c := lay.Caret(f.ed.Selection().Focus)
	w := f.textWidth()
	if f.scrollX >= 0 {
		t.Fatalf("End in a long rtl value: scrollX %v, want negative (scrolled toward the left); caret %+v w %v max %v sel %+v", f.scrollX, c, w, lay.MaxContentWidth(), f.ed.Selection())
	}
	if c.X < f.scrollX || c.X > f.scrollX+w {
		t.Errorf("caret at %v outside the visible [%v, %v]", c.X, f.scrollX, f.scrollX+w)
	}
	x.press("Home", 0, 0)
	x.frame()
	if f.scrollX != 0 {
		t.Errorf("Home: scrollX %v, want 0 (the start of an rtl line is at the right)", f.scrollX)
	}
}

func TestDirChecks(t *testing.T) {
	devAny.Store(true)
	func() {
		defer func() {
			r := recover()
			if r == nil || !strings.Contains(fmt.Sprint(r), `Dir("right")`) {
				t.Errorf("invalid Dir: panic %v", r)
			}
		}()
		Dir("right")
	}()
	func() {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "Portal") {
				t.Errorf("Dir on a Portal: panic %v", r)
			}
		}()
		Portal(Dir("rtl"))
	}()
	devAny.Store(false)
	defer devAny.Store(true)
	tr := newTestTree(t, 100, 100, div(Key("x"), Dir("sideways"), Class("w-4 h-4")), nil)
	tr.Frame()
	if keyed(tr, "x").box.cs.Text.Direction != text.DirectionLTR {
		t.Error("an invalid Dir outside dev mode is not ignored")
	}
}
