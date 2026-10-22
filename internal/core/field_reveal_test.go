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

// focusEndApp is an Input whose effect focuses it and puts the caret at
// the end of its long value (the usual "focus and go to the end"),
// changing nothing else, so the layout stays current. The effect runs on
// mount, or when start is not nil, after start is called.
func focusEndApp(long string, start *func()) Node {
	return Component(func() Node {
		run, setRun := UseState(start == nil)
		if start != nil {
			*start = func() { setRun.Set(true) }
		}
		ref := UseElementRef()
		UseEffect(func() func() {
			if run {
				ref.Focus()
				ref.SetSelectionRange(len(long), len(long))
			}
			return nil
		}, run)
		return div(Class("p-4"), input(Key("in"), Class("w-32"), AttachRef(ref), DefaultValue(long)))
	})
}

// wantRevealed fails unless the field's scroll is the one a reveal with
// its current layout gives, and that one scrolls (the caret at the end of
// a value that overflows). It leaves that scroll.
func wantRevealed(t *testing.T, what string, f *field) {
	t.Helper()
	got := f.scrollX
	f.scrollX = 0
	f.revealNow()
	if f.scrollX <= 0 {
		t.Fatalf("%s: reference scrollX %v, want the end revealed", what, f.scrollX)
	}
	if got != f.scrollX {
		t.Errorf("%s: scrollX %v, want %v", what, got, f.scrollX)
	}
}

// wantCaretArea fails unless the IME's caret area is inside the field.
func wantCaretArea(t *testing.T, what string, st *platform.TextInputState, f *field) {
	t.Helper()
	if st == nil {
		t.Fatalf("%s: no text input state", what)
	}
	r := windowRect(f.bx)
	if x := st.CursorArea.X; x < r.X0 || x > r.X1 {
		t.Errorf("%s: caret area x %v outside the field [%v, %v]", what, x, r.X0, r.X1)
	}
}

// A caret reveal a passive effect asks for while leaving the layout
// current (no frame work, no update queued) runs right after the effects
// (ADR-122): the field is scrolled when FlushEffects returns, so a tree
// that runs no other frame (a one-shot render, a test reading the state)
// does not lose it, and a host gives the IME the revealed caret area at
// once instead of a stale one first. Before, it waited for the next
// frame: the caret area sent after the effects was outside the window.
func TestFieldRevealEffectAtOnce(t *testing.T) {
	long := strings.Repeat("word ", 10) // overflows w-32
	t.Run("tree", func(t *testing.T) {
		// No frame request (SetRequestFrame): one frame, then its effects.
		tr := NewTree(Config{Fonts: testFonts})
		tr.SetViewport(gfx.Size{W: 400, H: 100})
		tr.Mount(focusEndApp(long, nil))
		tr.Frame()
		tr.FlushEffects()
		f := fieldOf(t, tr, "in")
		if tr.foc.el != f.el || f.ed.Selection().Focus.Offset != len(long) {
			t.Fatalf("the effect: focused %v, caret %d", tr.foc.el == f.el, f.ed.Selection().Focus.Offset)
		}
		if len(tr.frev.q) != 0 {
			t.Errorf("%d reveals wait after the effects, want none", len(tr.frev.q))
		}
		_, st, _ := tr.TextInput()
		wantCaretArea(t, "after the effects", st, f)
		wantRevealed(t, "after the effects", f)
	})
	t.Run("host", func(t *testing.T) {
		var start func()
		a := newKeys(t, focusEndApp(long, &start), 400, 100)
		a.settle()
		f := fieldOf(t, a.tree(), "in")
		s0, ime0 := a.tree().Stats(), a.w.TextInputChanges()
		start()
		a.b.Drain(a.h) // the update
		a.frame()      // its build; the effect runs after the present
		if a.tree().foc.el != f.el {
			t.Fatal("the effect did not focus the field")
		}
		wantCaretArea(t, "after the effects", a.w.TextInput(), f)
		wantRevealed(t, "after the effects", f)
		a.settle()
		s1 := a.tree().Stats()
		// The frame of the build and the one that paints the focus, as
		// before; the IME gets one state, not a stale one first.
		if n := s1.Frames - s0.Frames; n != 2 {
			t.Errorf("%d frames, want 2", n)
		}
		if n := a.w.TextInputChanges() - ime0; n != 1 {
			t.Errorf("%d text input states sent, want 1", n)
		}
		wantRevealed(t, "settled", f)
	})
}

// A caret reveal a passive effect asks for while changing the layout (it
// narrows the field) waits for the next frame, also in a tree with no
// frame request: Dirty reports the work, and the frame its driver runs
// reveals with the narrowed width.
func TestFieldRevealEffectWaitsNoRequest(t *testing.T) {
	long := strings.Repeat("word ", 5) // fits w-80, not w-32
	var start func()
	app := Component(func() Node {
		narrow, setNarrow := UseState(false)
		run, setRun := UseState(false)
		start = func() { setRun.Set(true) }
		ref := UseElementRef()
		UseEffect(func() func() {
			if run {
				ref.Focus()
				ref.SetSelectionRange(len(long), len(long))
				setNarrow.Set(true)
			}
			return nil
		}, run)
		w := "w-80"
		if narrow {
			w = "w-32"
		}
		return div(Class("p-4"), input(Key("in"), Class(w), AttachRef(ref), DefaultValue(long)))
	})
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 400, H: 100})
	tr.Mount(app)
	tr.Frame()
	tr.FlushEffects()
	start()
	tr.Frame() // builds; its effect waits
	tr.FlushEffects()
	f := fieldOf(t, tr, "in")
	if f.scrollX != 0 || len(tr.frev.q) != 1 {
		t.Fatalf("after the effects: scrollX %v, %d reveals waiting; want 0 and 1 (the wide layout is stale)", f.scrollX, len(tr.frev.q))
	}
	if !tr.Dirty() {
		t.Fatal("Dirty is false with a reveal and an update waiting")
	}
	for i := 0; i < 10 && tr.Dirty(); i++ {
		tr.Frame()
	}
	if len(tr.frev.q) != 0 {
		t.Errorf("%d reveals still wait", len(tr.frev.q))
	}
	wantRevealed(t, "narrowed", f)
}

// A caret reveal held at the start of a frame (asked for by a passive
// effect the frame flushes or a function posted to it) whose frame has
// nothing to build or lay out runs with the current layout at the frame's
// early-return check: one that scrolls nothing keeps the frame idle (no
// paint pass), one that scrolls is painted by that frame. Before, a held
// reveal always forced a paint pass.
func TestFieldRevealHeldIdleFrame(t *testing.T) {
	long := strings.Repeat("word ", 10) // overflows w-32
	for _, via := range []string{"effect", "post"} {
		t.Run(via, func(t *testing.T) {
			var fld *field
			var setTick Setter[int]
			app := Component(func() Node {
				tick, st := UseState(0)
				setTick = st
				UseEffect(func() func() {
					if tick > 0 && fld != nil {
						fld.reveal()
					}
					return nil
				}, tick)
				return div(Class("p-4"), input(Key("in"), Class("w-32"), DefaultValue(long)))
			})
			tr := NewTree(Config{Fonts: testFonts})
			tr.SetViewport(gfx.Size{W: 400, H: 100})
			tr.Mount(app)
			tr.Frame()
			fld = fieldOf(t, tr, "in")
			stop := tr.ReceivePosts()
			defer stop()
			// ask queues a reveal for the start of the next frame.
			n := 0
			ask := func() {
				switch via {
				case "effect":
					n++
					setTick.Set(n)
					tr.Frame() // builds; its effect waits for the next frame
				case "post":
					Post(fld.reveal)
				}
			}
			for _, c := range []struct {
				name   string
				caret  int
				paints int
			}{
				{"in view", 0, 0},
				{"scrolls", len(long), 1},
			} {
				fld.ed.Select(c.caret, c.caret)
				ask()
				s0 := tr.Stats()
				_, changed := tr.Frame()
				s1 := tr.Stats()
				if changed != (c.paints > 0) || s1.Frames-s0.Frames != c.paints || s1.Paints-s0.Paints != c.paints {
					t.Errorf("%s: changed %v, %d frames, %d paints; want %d", c.name, changed, s1.Frames-s0.Frames, s1.Paints-s0.Paints, c.paints)
				}
				if len(tr.frev.q) != 0 || tr.Dirty() {
					t.Errorf("%s: %d reveals wait, dirty %v", c.name, len(tr.frev.q), tr.Dirty())
				}
			}
			wantRevealed(t, "scrolled", fld)
		})
	}
}

// A caret reveal still held when what holds it ends early (a posted
// function at a frame's start, or a passive effect run after a frame,
// panicked) asks for the next frame, which runs it: it is never stranded.
func TestFieldRevealHeldAfterPanic(t *testing.T) {
	long := strings.Repeat("word ", 10) // overflows w-32
	for _, via := range []string{"frame", "effects"} {
		t.Run(via, func(t *testing.T) {
			var fld *field
			var setRun Setter[bool]
			app := Component(func() Node {
				run, sr := UseState(false)
				setRun = sr
				UseEffect(func() func() {
					if run {
						fld.reveal()
						panic("boom")
					}
					return nil
				}, run)
				return div(Class("p-4"), input(Key("in"), Class("w-32"), DefaultValue(long)))
			})
			tr := NewTree(Config{Fonts: testFonts})
			tr.SetViewport(gfx.Size{W: 400, H: 100})
			tr.Mount(app)
			tr.Frame()
			tr.FlushEffects()
			fld = fieldOf(t, tr, "in")
			fld.ed.Select(len(long), len(long))
			stop := tr.ReceivePosts()
			defer stop()
			var panics func()
			switch via {
			case "frame":
				Post(fld.reveal)
				Post(func() { panic("boom") })
				panics = func() { tr.Frame() }
			case "effects":
				setRun.Set(true)
				tr.Frame() // builds; its effect waits
				panics = tr.FlushEffects
			}
			requests := 0
			tr.SetRequestFrame(func() { requests++ })
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("no panic")
					}
				}()
				panics()
			}()
			if requests != 1 {
				t.Errorf("%d frame requests after the panic, want 1 (the frame that runs the reveal)", requests)
			}
			tr.Frame()
			if len(tr.frev.q) != 0 {
				t.Errorf("%d reveals still wait", len(tr.frev.q))
			}
			wantRevealed(t, "next frame", fld)
		})
	}
}

// A handler or a posted function run outside a frame that resizes a field
// (a state update the next frame applies) and then focuses it or moves its
// caret reveals the caret with the new width: with an update queued the
// layout is not current, so the reveal waits for the frame that applies
// it, as after the passive effects (ADR-122), and that frame, the only one
// as before, runs it. Before, it ran at once with the old width (the value
// fit, nothing scrolled) and the frame that narrowed the field did not
// reveal again: the caret at the end was clipped away.
func TestFieldRevealHandlerResizes(t *testing.T) {
	long := strings.Repeat("word ", 5) // fits w-80, not w-32
	for _, via := range []string{"click", "key", "post"} {
		t.Run(via, func(t *testing.T) {
			var setNarrow Setter[bool]
			var ref ElementRef
			app := Component(func() Node {
				narrow, sn := UseState(false)
				setNarrow = sn
				ref = UseElementRef()
				w := "w-80"
				if narrow {
					w = "w-32"
				}
				return div(Class("p-4 flex flex-col items-start gap-2"),
					input(Key("in"), Class(w), AttachRef(ref), DefaultValue(long),
						// A key handler that narrows the focused field and
						// moves its caret to the end.
						On("keydown", func(e KeyboardEvent) {
							if e.Key == KeyValue(platform.KeyF2) {
								setNarrow.Set(true)
								ref.SetSelectionRange(len(long), len(long))
							}
						})),
					// A click handler that narrows the field and focuses it.
					button(Key("go"), Class("h-6 w-20"), On("click", func(MouseEvent) {
						setNarrow.Set(true)
						ref.Focus()
					})))
			})
			x := newKeys(t, app, 400, 120)
			x.settle()
			f := fieldOf(t, x.tree(), "in")
			var act func()
			switch via {
			case "click":
				f.ed.Select(len(long), len(long)) // the caret at the end, not focused
				act = func() { x.click(x.center("go")) }
			case "key":
				x.tree().focusElement(f.el)
				x.settle()
				if f.ed.Selection().Focus.Offset != 0 || f.scrollX != 0 {
					t.Fatalf("setup: caret %d, scrollX %v", f.ed.Selection().Focus.Offset, f.scrollX)
				}
				act = func() { x.press(platform.KeyF2, platform.CodeF2, 0) }
			case "post":
				stop := x.tree().ReceivePosts()
				defer stop()
				f.ed.Select(len(long), len(long))
				act = func() {
					Post(func() {
						setNarrow.Set(true)
						ref.Focus()
					})
					x.b.Drain(x.h) // the wake: ApplyUpdates runs the function
				}
			}
			s0 := x.tree().Stats()
			act()
			x.settle()
			s1 := x.tree().Stats()
			if x.tree().foc.el != f.el || f.ed.Selection().Focus.Offset != len(long) {
				t.Fatalf("focused %v, caret %d", x.tree().foc.el == f.el, f.ed.Selection().Focus.Offset)
			}
			if w := f.textWidth(); w > 128 {
				t.Fatalf("content width %v: the field was not narrowed", w)
			}
			if n, p := s1.Frames-s0.Frames, s1.Paints-s0.Paints; n != 1 || p != 1 {
				t.Errorf("%d frames, %d paints; want 1 and 1 (the frame of the update)", n, p)
			}
			if len(x.tree().frev.q) != 0 {
				t.Errorf("%d reveals still wait", len(x.tree().frev.q))
			}
			wantRevealed(t, "narrowed", f)
		})
	}
}
