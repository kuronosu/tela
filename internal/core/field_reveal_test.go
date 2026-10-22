package core

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// A text field reveals its caret (scrolls it into view) with the width of
// its laid-out content box. A reveal asked for in a frame before the
// layout, such as the focus an AutoFocus field takes after the style phase
// or the caret a controlled Value moves in the build, waits for the
// frame's layout: before the fix the AutoFocus Input of a value that fits
// was scrolled by the caret's 1 px (its content width was still zero), and
// a caret moved to the end of a long value before the layout showed none
// of the text.

// revealHarness mounts root and then, before the first frame (when then is
// not nil), updates the tree to then, as a build of the first frame would.
func revealHarness(t *testing.T, root, then Node, w, h float32) *inputHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if then.d != nil {
		host.Tree().Mount(then)
	}
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	t.Cleanup(host.stopTimer)
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	return x
}

// samePixels fails when the last frames of a and b differ.
func samePixels(t *testing.T, what string, a, b *inputHarness) {
	t.Helper()
	fa, fb := a.w.Frame(), b.w.Frame()
	if fa == nil || fb == nil {
		t.Fatalf("%s: no frame", what)
	}
	if fa.Rect != fb.Rect || !bytes.Equal(fa.Pix, fb.Pix) {
		t.Errorf("%s: the pixels differ from the field focused after its layout", what)
	}
}

func TestFieldAutoFocusRevealAfterLayout(t *testing.T) {
	long := strings.Repeat("word ", 30)
	for _, c := range []struct {
		name, value string
	}{
		{"fits", "hello"},
		{"overflows", long},
	} {
		t.Run(c.name, func(t *testing.T) {
			field := func(auto bool) Node {
				return div(Class("p-4"), input(Key("in"), Class("w-40"), AutoFocus(auto), DefaultValue(c.value)))
			}
			a := revealHarness(t, field(true), Node{}, 300, 100)
			a.settle()
			fa := fieldOf(t, a.tree(), "in")
			if a.tree().foc.el != fa.el {
				t.Fatal("the AutoFocus field is not focused")
			}
			if fa.scrollX != 0 {
				t.Errorf("AutoFocus: scrollX %v, want 0 (the caret is at the start)", fa.scrollX)
			}
			// The same Input focused by a click at its left edge after its
			// layout: the caret at the start, nothing scrolled.
			b := revealHarness(t, field(false), Node{}, 300, 100)
			b.settle()
			fb := fieldOf(t, b.tree(), "in")
			r := windowRect(fb.bx)
			b.click(r.X0+1, r.Center().Y)
			b.settle()
			if b.tree().foc.el != fb.el || fb.ed.Selection().Focus.Offset != 0 {
				t.Fatalf("click: focused %v, caret %d", b.tree().foc.el == fb.el, fb.ed.Selection().Focus.Offset)
			}
			if fb.scrollX != fa.scrollX {
				t.Errorf("scrollX: AutoFocus %v, click %v", fa.scrollX, fb.scrollX)
			}
			samePixels(t, "AutoFocus", a, b)
		})
	}
}

// A caret moved to the end of a long value before the first layout (a
// controlled Value set by the first frame's build) of an AutoFocus field is
// revealed as when the field is focused after its layout.
func TestFieldAutoFocusRevealEndBeforeLayout(t *testing.T) {
	long := strings.Repeat("word ", 30)
	for _, c := range []struct {
		name  string
		field func(auto bool, v string) Node
	}{
		{"input", func(auto bool, v string) Node {
			return input(Key("in"), Class("w-40"), AutoFocus(auto), FieldValue(v))
		}},
		{"textarea", func(auto bool, v string) Node {
			return textarea(Key("in"), Class("w-40"), Rows(2), AutoFocus(auto), FieldValue(v))
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := long
			if c.name == "textarea" {
				v = strings.Repeat("line\n", 12) + "end"
			}
			root := func(auto bool, v string) Node { return div(Class("p-4"), c.field(auto, v)) }
			a := revealHarness(t, root(true, ""), root(true, v), 300, 120)
			a.settle()
			fa := fieldOf(t, a.tree(), "in")
			if a.tree().foc.el != fa.el {
				t.Fatal("the AutoFocus field is not focused")
			}
			if fa.ed.Selection().Focus.Offset != len(v) {
				t.Fatalf("caret %d, want %d (the end)", fa.ed.Selection().Focus.Offset, len(v))
			}
			b := revealHarness(t, root(false, ""), root(false, v), 300, 120)
			b.settle()
			fb := fieldOf(t, b.tree(), "in")
			b.tree().focusElement(fb.el)
			b.settle()
			if b.tree().foc.el != fb.el {
				t.Fatal("the reference field is not focused")
			}
			if c.name == "input" {
				if fb.scrollX <= 0 {
					t.Fatalf("reference: scrollX %v, want the end revealed", fb.scrollX)
				}
				if fa.scrollX != fb.scrollX {
					t.Errorf("scrollX: AutoFocus %v, focused after layout %v", fa.scrollX, fb.scrollX)
				}
			} else {
				pa, pb := fa.bx.scr.pos, fb.bx.scr.pos
				if pb.Y <= 0 {
					t.Fatalf("reference: offset %v, want the end revealed", pb)
				}
				if pa != pb {
					t.Errorf("offset: AutoFocus %v, focused after layout %v", pa, pb)
				}
			}
			samePixels(t, "AutoFocus", a, b)
		})
	}
}

// A controlled Value that moves the caret to the end in the same build
// that narrows the field reveals it with the new width.
func TestFieldRevealStaleWidth(t *testing.T) {
	long := strings.Repeat("word ", 30)
	var set func(bool)
	app := func() Node {
		narrow, setNarrow := UseState(false)
		set = setNarrow.Set
		w, v := "w-80", ""
		if narrow {
			w, v = "w-32", long
		}
		return div(Class("p-4"), input(Key("in"), Class(w), FieldValue(v)))
	}
	a := newKeys(t, Component(app), 400, 100)
	a.settle()
	fa := fieldOf(t, a.tree(), "in")
	a.tree().focusElement(fa.el)
	a.settle()
	set(true)
	a.settle()
	// The reference: the narrow field laid out first, then the value.
	var setV func(string)
	ref := func() Node {
		v, setVal := UseState("")
		setV = setVal.Set
		return div(Class("p-4"), input(Key("in"), Class("w-32"), FieldValue(v)))
	}
	b := newKeys(t, Component(ref), 400, 100)
	b.settle()
	fb := fieldOf(t, b.tree(), "in")
	b.tree().focusElement(fb.el)
	b.settle()
	setV(long)
	b.settle()
	if fb.scrollX <= 0 {
		t.Fatalf("reference: scrollX %v, want the end revealed", fb.scrollX)
	}
	if fa.scrollX != fb.scrollX {
		t.Errorf("scrollX: %v, want %v (revealed with the new width)", fa.scrollX, fb.scrollX)
	}
	samePixels(t, "narrowed", a, b)
}

// A passive effect that narrows a field and focuses it, in either order,
// reveals its caret with the narrowed width: the effect's state update
// applies in the next frame, and the reveal waits for that frame's layout,
// as when the field is focused after it. Before the fix the reveal ran in
// the effect, with the wide layout (the value fit, nothing scrolled), and
// the caret at the end of the narrowed field was clipped away.
func TestFieldRevealFromEffect(t *testing.T) {
	long := strings.Repeat("word ", 5) // fits w-80, not w-32
	app := func(focusFirst bool, start *func()) Node {
		return Component(func() Node {
			narrow, setNarrow := UseState(false)
			run, setRun := UseState(false)
			*start = func() { setRun.Set(true) }
			ref := UseElementRef()
			UseEffect(func() func() {
				if run {
					if focusFirst {
						ref.Focus()
						setNarrow.Set(true)
					} else {
						setNarrow.Set(true)
						ref.Focus()
					}
				}
				return nil
			}, run)
			w := "w-80"
			if narrow {
				w = "w-32"
			}
			return div(Class("p-4"), input(Key("in"), Class(w), AttachRef(ref), DefaultValue(long)))
		})
	}
	// caretToEnd puts the caret of the field at the end and blurs it.
	caretToEnd := func(x *inputHarness, f *field) {
		x.tree().focusElement(f.el)
		x.key(platform.KeyEnd, 0)
		x.settle()
		x.tree().setFocus(nil, focusScript)
		x.settle()
		if x.tree().foc.el != nil || f.ed.Selection().Focus.Offset != len(long) {
			t.Fatalf("setup: focused %v, caret %d", x.tree().foc.el != nil, f.ed.Selection().Focus.Offset)
		}
	}
	// The reference: the narrow field laid out, then focused.
	var refStart func()
	b := newKeys(t, app(false, &refStart), 400, 100)
	b.settle()
	refStart()
	b.settle()
	fb := fieldOf(t, b.tree(), "in")
	caretToEnd(b, fb)
	b.tree().focusElement(fb.el)
	b.settle()
	if b.tree().foc.el != fb.el || fb.scrollX <= 0 {
		t.Fatalf("reference: focused %v, scrollX %v, want the end revealed", b.tree().foc.el == fb.el, fb.scrollX)
	}
	for _, focusFirst := range []bool{false, true} {
		name := "narrow then focus"
		if focusFirst {
			name = "focus then narrow"
		}
		t.Run(name, func(t *testing.T) {
			var start func()
			a := newKeys(t, app(focusFirst, &start), 400, 100)
			a.settle()
			fa := fieldOf(t, a.tree(), "in")
			caretToEnd(a, fa)
			if fa.scrollX != 0 {
				t.Fatalf("setup: scrollX %v in the wide field", fa.scrollX)
			}
			start()
			a.settle()
			if a.tree().foc.el != fa.el {
				t.Fatal("the effect did not focus the field")
			}
			if fa.scrollX != fb.scrollX {
				t.Errorf("scrollX %v, want %v (revealed with the narrowed width)", fa.scrollX, fb.scrollX)
			}
			samePixels(t, name, a, b)
		})
	}
}

// The same at the start of a frame, before its build: a passive effect the
// frame flushes (a Tree driven by Frame alone) and a posted function its
// ApplyUpdates runs. The caret is revealed as by a focus after the layout.
func TestFieldRevealAtFrameStart(t *testing.T) {
	long := strings.Repeat("word ", 5) // fits w-80, not w-32
	for _, via := range []string{"effect", "post"} {
		t.Run(via, func(t *testing.T) {
			var setRun, setNarrow Setter[bool]
			var ref ElementRef
			app := func() Node {
				narrow, sn := UseState(false)
				run, sr := UseState(false)
				setNarrow, setRun = sn, sr
				ref = UseElementRef()
				UseEffect(func() func() {
					if run {
						setNarrow.Set(true)
						ref.Focus()
					}
					return nil
				}, run)
				w := "w-80"
				if narrow {
					w = "w-32"
				}
				return div(Class("p-4"), input(Key("in"), Class(w), AttachRef(ref), DefaultValue(long)))
			}
			tr := NewTree(Config{Fonts: testFonts})
			tr.SetViewport(gfx.Size{W: 400, H: 100})
			tr.Mount(Component(app))
			tr.Frame()
			f := fieldOf(t, tr, "in")
			f.ed.Select(len(long), len(long)) // the caret at the end, not focused
			switch via {
			case "effect":
				setRun.Set(true)
				tr.Frame() // builds; its passive effect waits
				tr.Frame() // flushes the effect at its start
			case "post":
				stop := tr.ReceivePosts()
				defer stop()
				Post(func() {
					setNarrow.Set(true)
					ref.Focus()
				})
				tr.Frame()
			}
			for i := 0; i < 10 && tr.Dirty(); i++ {
				tr.Frame()
			}
			if tr.foc.el != f.el {
				t.Fatal("the field is not focused")
			}
			got := f.scrollX
			f.scrollX = 0
			f.revealNow() // the reveal of a focus after the layout
			if f.scrollX <= 0 {
				t.Fatalf("reference: scrollX %v, want the end revealed", f.scrollX)
			}
			if got != f.scrollX {
				t.Errorf("scrollX %v, want %v (revealed with the narrowed width)", got, f.scrollX)
			}
		})
	}
}

// Typing into a field sized to its content (field-sizing-content) widens
// it in the next layout: the reveal waits for that layout, so the text
// that fits is not scrolled away by the width before the key (it was by
// 36.75 px after typing "hello").
func TestFieldRevealSizingContent(t *testing.T) {
	field := func(v string) Node {
		return div(Class("p-4"), input(Key("in"), Class("field-sizing-content"), DefaultValue(v)))
	}
	a := newKeys(t, field("abc"), 400, 100)
	a.settle()
	fa := fieldOf(t, a.tree(), "in")
	a.tree().focusElement(fa.el)
	a.key(platform.KeyEnd, 0)
	a.settle()
	a.typeText("hello")
	a.settle()
	b := newKeys(t, field("abchello"), 400, 100)
	b.settle()
	fb := fieldOf(t, b.tree(), "in")
	b.tree().focusElement(fb.el)
	b.key(platform.KeyEnd, 0)
	b.settle()
	if lo, hi := fa.scrollXRange(); fa.scrollX < lo || fa.scrollX > hi {
		t.Errorf("scrollX %v outside [%v, %v]", fa.scrollX, lo, hi)
	}
	if fa.scrollX != fb.scrollX {
		t.Errorf("scrollX after typing %v, want %v (the value typed before the layout)", fa.scrollX, fb.scrollX)
	}
	samePixels(t, "typed", a, b)
}
