package core

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// A Textarea whose lines do not wrap (text-nowrap, whitespace-pre,
// whitespace-nowrap) scrolls horizontally, as in Chromium: its extents
// come from its widest line, the wheel (Shift, or a horizontal delta)
// scrolls it, the caret is revealed horizontally while typing and moving,
// a selection drag autoscrolls, and its text paints and hit-tests moved by
// the offset. Before, fieldExtents always returned a horizontal extent of
// 0, so long lines ran past the right edge, clipped.

const hscrollLong = "The quick brown fox jumps over the lazy dog and keeps on running far away"

// newScrollKeys is newKeys with a configuration (newScrollHost, the
// scroll sets checked at the end).
func newScrollKeys(t *testing.T, root Node, w, h float32, cfg Config) *inputHarness {
	t.Helper()
	x := newScrollHost(t, root, w, h, cfg)
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	return x
}

// caretInView fails unless the caret of f is inside its scroll
// container's client area horizontally (its left edge only in a
// right-to-left field, where a caret at the right edge of the text has no
// room after it).
func caretInView(t *testing.T, what string, f *field) {
	t.Helper()
	s := f.bx.scr
	l := f.bx.ld.Layout()
	c := f.layout().Caret(f.ed.Selection().Focus)
	x := l.Padding.Left + c.X
	px := s.flip(s.pos).X
	w := float32(caretWidth)
	if s.rtl {
		w = 0
	}
	if x < px-0.01 || x+w > px+s.client.W+0.01 {
		t.Errorf("%s: caret at %v not in view [%v, %v]", what, x, px, px+s.client.W)
	}
}

func TestTextareaNowrapExtents(t *testing.T) {
	for _, c := range []struct {
		class string
		wraps bool
		pad   float32 // padding-right
		lines int
	}{
		{"text-nowrap", false, 0, 2},
		{"whitespace-pre", false, 0, 2},
		{"whitespace-nowrap", false, 0, 1}, // collapses the newline too
		{"text-nowrap px-3", false, 12, 2},
		{"", true, 0, 0},
		{"break-all", true, 0, 0},
	} {
		t.Run(c.class, func(t *testing.T) {
			root := div(Class("p-2"), textarea(Key("ta"), Class("h-16 w-40 "+c.class), DefaultValue(hscrollLong+"\nshort")))
			tr := fieldTree(t, root, 400, 200, Config{Fonts: testFonts})
			f := fieldOf(t, tr, "ta")
			s := f.bx.scr
			lay := f.layout()
			if c.wraps {
				if s.max.X != 0 {
					t.Errorf("a wrapping textarea scrolls horizontally: max %v", s.max)
				}
				return
			}
			if lay.NumLines() != c.lines {
				t.Errorf("%d lines, want %d", lay.NumLines(), c.lines)
			}
			want := lay.Width() + max(caretWidth-c.pad, 0) - f.textWidth()
			if want <= 0 {
				t.Fatalf("the long line fits (%v in %v)", lay.Width(), f.textWidth())
			}
			if s.max.X != want {
				t.Errorf("max.X %v, want %v (widest line %v, content width %v)", s.max.X, want, lay.Width(), f.textWidth())
			}
			if !s.userX {
				t.Error("overflow-x auto does not scroll by the user")
			}
		})
	}
	// An empty field with a long placeholder scrolls to show it.
	root := div(textarea(Key("ta"), Class("h-16 w-40 text-nowrap"), Placeholder(hscrollLong)))
	tr := fieldTree(t, root, 400, 200, Config{Fonts: testFonts})
	if s := fieldOf(t, tr, "ta").bx.scr; s.max.X <= 0 {
		t.Errorf("a long placeholder: max %v", s.max)
	}
}

func TestTextareaNowrapWheel(t *testing.T) {
	root := div(Class("p-2"), textarea(Key("ta"), Class("h-16 w-40 text-nowrap"), DefaultValue(hscrollLong)))
	x := newKeys(t, root, 400, 200)
	x.frame()
	f := fieldOf(t, x.tree(), "ta")
	r := windowRect(f.bx)
	cx, cy := r.Center().X, r.Center().Y
	// Shift turns a vertical notch into a horizontal one.
	e := notch(cx, cy, 1)
	e.Mods = platform.ModShift
	x.wheel(e)
	x.settle()
	s := f.bx.scr
	if s.pos.X <= 0 || s.pos.Y != 0 {
		t.Fatalf("Shift+wheel: offset %v, want a horizontal scroll", s.pos)
	}
	// A horizontal notch scrolls back.
	x.wheel(platform.ScrollEvent{Pos: platform.Point{X: cx, Y: cy}, Steps: platform.Point{X: -5}})
	x.settle()
	if s.pos.X != 0 {
		t.Errorf("horizontal notches back: offset %v", s.pos)
	}
	// A touchpad's horizontal delta follows the fingers, to the extent.
	x.wheel(platform.ScrollEvent{Pos: platform.Point{X: cx, Y: cy}, Delta: platform.Point{X: 1000}, Phase: platform.ScrollPhaseUpdate})
	x.settle()
	if s.pos.X != s.max.X {
		t.Errorf("a touchpad delta past the end: offset %v, max %v", s.pos, s.max)
	}
}

func TestTextareaNowrapRevealsCaret(t *testing.T) {
	for _, class := range []string{"", "px-3 py-2"} {
		t.Run(class, func(t *testing.T) {
			root := div(Class("p-2"), textarea(Key("ta"), Class("h-16 w-40 text-nowrap "+class)))
			x := newKeys(t, root, 400, 200)
			x.tab()
			x.typeText(hscrollLong)
			x.settle()
			f := fieldOf(t, x.tree(), "ta")
			s := f.bx.scr
			if s.max.X <= 0 || s.pos.X <= 0 {
				t.Fatalf("typing a long line: offset %v max %v, want a horizontal scroll", s.pos, s.max)
			}
			if s.pos.X != s.max.X {
				t.Errorf("the caret at the end of the longest line: offset %v, want the extent %v", s.pos.X, s.max.X)
			}
			caretInView(t, "typing", f)
			x.key(platform.KeyHome, 0)
			x.settle()
			if s.pos.X != 0 {
				t.Errorf("Home: offset %v, want 0", s.pos.X)
			}
			x.key(platform.KeyEnd, 0)
			x.settle()
			if s.pos.X != s.max.X {
				t.Errorf("End: offset %v, want %v", s.pos.X, s.max.X)
			}
			// A short line below: ArrowDown goes to its end, and the view
			// follows the caret back to the start.
			x.enter()
			x.typeText("ab")
			x.settle()
			caretInView(t, "a short line", f)
			if s.pos.X >= s.max.X {
				t.Errorf("a short line: offset %v did not follow the caret back", s.pos.X)
			}
			// Arrow keys move the caret and the view.
			x.key(platform.KeyArrowUp, 0)
			x.key(platform.KeyEnd, 0)
			x.settle()
			for range 12 {
				x.key(platform.KeyArrowLeft, platform.ModCtrl)
			}
			x.settle()
			caretInView(t, "Ctrl+ArrowLeft", f)
			if s.pos.X == s.max.X {
				t.Error("Ctrl+ArrowLeft did not scroll")
			}
		})
	}
}

func TestTextareaNowrapRTL(t *testing.T) {
	root := div(Class("p-2"), textarea(Key("ta"), Dir("rtl"), Class("h-16 w-40 text-nowrap")))
	x := newKeys(t, root, 400, 200)
	x.tab()
	x.typeText(hscrollLong)
	x.settle()
	f := fieldOf(t, x.tree(), "ta")
	s := f.bx.scr
	lay := f.layout()
	if want := lay.Width() - f.textWidth(); s.max.X != want {
		t.Errorf("max.X %v, want %v (no caret room in a right-to-left field)", s.max.X, want)
	}
	// The line overflows on the left (start-aligned: its right edge at
	// the right of the field), and the Latin text's end, where the caret
	// is, is at its right: no scroll.
	if s.pos.X != 0 {
		t.Errorf("typing: offset %v, want 0", s.pos.X)
	}
	caretInView(t, "typing", f)
	// Home goes to the start of the text, on the left: scrolled all the
	// way toward the left (scrollLeft negative).
	x.key(platform.KeyHome, 0)
	x.settle()
	if s.pos.X != s.max.X || s.flip(s.pos).X >= 0 {
		t.Errorf("Home: offset %v (physical %v), want the extent %v", s.pos.X, s.flip(s.pos).X, s.max.X)
	}
	caretInView(t, "Home", f)
	x.key(platform.KeyEnd, 0)
	x.settle()
	if s.pos.X != 0 {
		t.Errorf("End: offset %v, want 0", s.pos.X)
	}
}

func TestTextareaNowrapPaintAndHitTest(t *testing.T) {
	root := div(Class("bg-white p-2"), textarea(Key("ta"), Class("h-16 w-40 text-nowrap bg-white text-black"), DefaultValue(hscrollLong)))
	x := newScrollKeys(t, root, 400, 200, Config{InstantScroll: true})
	x.frame()
	tr := x.tree()
	f := fieldOf(t, tr, "ta")
	s := f.bx.scr
	before := cloneFrame(x)
	const dx = 37
	tr.scrollDirect(s, gfx.Point{X: dx})
	x.settle()
	if s.pos.X != dx || s.soff.X != dx {
		t.Fatalf("offset %v painted %v, want %v", s.pos, s.soff, dx)
	}
	after := x.w.Frame()
	// The text moved left by dx: inside the client area (away from its
	// edges, where the old frame had no text or the new one is clipped,
	// and above the overlay scrollbar shown by the scroll), each pixel is
	// the one dx to its right before.
	r := windowRect(f.bx)
	diff, ink := 0, 0
	for py := int(r.Y0) + 2; py < int(r.Y1)-16; py++ {
		for px := int(r.X0) + 2; px < int(r.X1)-2-dx; px++ {
			a, b := after.RGBAAt(px, py), before.RGBAAt(px+dx, py)
			if a != b {
				diff++
			}
			if a.R < 128 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("no text painted")
	}
	if diff != 0 {
		t.Errorf("%d pixels differ from the frame before shifted by %d px", diff, dx)
	}
	// A click places the caret under the pointer, in the scrolled text.
	lay := f.layout()
	l := f.bx.ld.Layout()
	cx := r.X0 + l.Border.Left + l.Padding.Left + lay.Caret(textCursor(20)).X - dx
	x.click(cx+1, r.Center().Y)
	if s0, e0 := x.selection("ta"); s0 != 20 || e0 != 20 {
		t.Errorf("a click in the scrolled text put the caret at %d..%d, want 20", s0, e0)
	}
	if s.pos.X != dx {
		t.Errorf("the click scrolled to %v", s.pos.X)
	}
}

// cloneFrame returns a copy of the window's last frame.
func cloneFrame(x *inputHarness) *image.RGBA {
	x.t.Helper()
	f := x.w.Frame()
	if f == nil {
		x.t.Fatal("no frame")
	}
	c := *f
	c.Pix = append([]byte(nil), f.Pix...)
	return &c
}

func TestTextareaNowrapAutoscroll(t *testing.T) {
	root := div(Class("p-4"), textarea(Key("ta"), Class("h-16 w-40 text-nowrap"), DefaultValue(hscrollLong)))
	x := newKeys(t, root, 400, 200)
	x.frame()
	f := fieldOf(t, x.tree(), "ta")
	s := f.bx.scr
	r := windowRect(f.bx)
	cy := r.Y0 + 10
	x.send(move(r.X0+5, cy), down(r.X0+5, cy), moveHeld(r.X1+40, cy))
	for range 10 {
		x.frame()
	}
	if s.pos.X <= 0 {
		t.Fatalf("no horizontal autoscroll: offset %v", s.pos)
	}
	if _, e := x.selection("ta"); e < 30 {
		t.Errorf("the selection did not follow the scroll: end %d", e)
	}
	x.send(up(r.X1+40, cy))
	x.settle()
	if x.tree().fieldAutoscrolling() {
		t.Error("still autoscrolling after the release")
	}
	// A wrapping textarea never autoscrolls horizontally.
	y := newKeys(t, div(Class("p-4"), textarea(Key("ta"), Class("h-40 w-40"), DefaultValue(hscrollLong))), 400, 300)
	y.frame()
	g := fieldOf(t, y.tree(), "ta")
	r = windowRect(g.bx)
	y.send(move(r.X0+5, r.Y0+5), down(r.X0+5, r.Y0+5), moveHeld(r.X1+40, r.Y0+5))
	if y.tree().fieldAutoscrolling() {
		t.Error("a wrapping textarea autoscrolls past its right edge")
	}
	y.send(up(r.X1+40, r.Y0+5))
}

func TestTextareaNowrapClassicGutter(t *testing.T) {
	// Laid out with a long line: the horizontal bar's gutter is reserved.
	root := div(Class("p-2"), textarea(Key("ta"), Class("h-24 w-40 text-nowrap"), DefaultValue(hscrollLong)))
	tr := fieldTree(t, root, 400, 200, Config{Fonts: testFonts, ClassicScrollbars: true})
	f := fieldOf(t, tr, "ta")
	if l := f.bx.ld.Layout(); l.ScrollbarSize.H != classicGutter || l.ScrollbarSize.W != 0 {
		t.Errorf("scrollbar size %v, want a %d px horizontal gutter only", l.ScrollbarSize, classicGutter)
	}
	if s := f.bx.scr; s.client.H != 96-classicGutter {
		t.Errorf("client height %v, want %v", s.client.H, 96-classicGutter)
	}

	// Typing: the keystroke that makes the line overflow lays the field
	// out once to add the gutter; the others lay nothing out.
	x := newScrollKeys(t, div(Class("p-2"), textarea(Key("ta"), Class("h-24 w-40 text-nowrap"))), 400, 200, Config{ClassicScrollbars: true})
	x.tab()
	x.settle()
	g := fieldOf(t, x.tree(), "ta")
	s := g.bx.scr
	layouts, flips := x.tree().Stats().Layouts, 0
	had := false
	for _, r := range hscrollLong {
		x.typeText(string(r))
		x.settle()
		now := g.bx.ld.Layout().ScrollbarSize.H == classicGutter
		if now != (s.max.X > 0) {
			t.Fatalf("after %q: gutter %v with extent %v", g.ed.RawValue(), now, s.max.X)
		}
		if now != had {
			flips++
			had = now
		}
	}
	if flips != 1 {
		t.Errorf("the gutter changed %d times, want once", flips)
	}
	if n := x.tree().Stats().Layouts - layouts; n == 0 || n > 3 {
		t.Errorf("%d layouts while typing, want the gutter's only", n)
	}
	caretInView(t, "classic", g)
	// Clearing the field drops it.
	x.ctrl("a", 0)
	x.key(platform.KeyBackspace, 0)
	x.settle()
	if h := g.bx.ld.Layout().ScrollbarSize.H; h != 0 || s.max.X != 0 {
		t.Errorf("after clearing: gutter %v, extent %v", h, s.max.X)
	}
}

func TestTextareaNowrapDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		for _, cfg := range []Config{{}, {ClassicScrollbars: true, InstantScroll: true}} {
			t.Run(fmt.Sprintf("x%v_classic=%v", scale, cfg.ClassicScrollbars), func(t *testing.T) {
				root := div(Class("bg-slate-50 p-3"),
					textarea(Key("ta"), Class("h-24 w-48 rounded-md border border-slate-300 bg-white px-3 py-2 text-nowrap"),
						DefaultValue("first\n"+strings.Repeat("abc ", 4))))
				x := newDamageHarness(t, root, 320, 180, scale, cfg, 7)
				x.w.SetFocused(true)
				x.b.Drain(x.h)
				key := func(k platform.Key, c platform.KeyCode, mods platform.Modifiers, what string) {
					x.inject(keyEv(platform.KeyPress, k, c, mods))
					x.inject(keyEv(platform.KeyRelease, k, c, mods))
					x.frame(what)
				}
				key(platform.KeyTab, platform.CodeTab, 0, "tab")
				key(platform.KeyEnd, platform.CodeEnd, platform.ModCtrl, "ctrl+end")
				for i, r := range hscrollLong[:40] {
					k := platform.Key(string(r))
					x.inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeUnidentified, Key: k, Text: string(r)})
					x.inject(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeUnidentified, Key: k})
					x.frame(fmt.Sprintf("type %d", i))
				}
				for range 20 {
					x.frame("settle")
				}
				f := fieldOf(t, x.tree(), "ta")
				if f.bx.scr.pos.X <= 0 {
					t.Fatalf("not scrolled horizontally: %v", f.bx.scr.pos)
				}
				key(platform.KeyHome, platform.CodeHome, 0, "home")
				key(platform.KeyEnd, platform.CodeEnd, 0, "end")
				r := windowRect(f.bx)
				x.move(r.Center().X, r.Center().Y)
				for i := range 6 {
					e := platform.ScrollEvent{Pos: platform.Point{X: r.Center().X, Y: r.Center().Y}, Steps: platform.Point{Y: float32(1 - 2*(i%2))}, Mods: platform.ModShift}
					x.inject(e)
					for range 12 {
						x.frame(fmt.Sprintf("shift-wheel %d", i))
					}
				}
				x.inject(platform.ScrollEvent{Pos: platform.Point{X: r.Center().X, Y: r.Center().Y}, Delta: platform.Point{X: 23}, Phase: platform.ScrollPhaseUpdate})
				x.frame("touchpad")
				x.inject(platform.ScrollEvent{Pos: platform.Point{X: r.Center().X, Y: r.Center().Y}, Phase: platform.ScrollPhaseEnd})
				for range 30 {
					x.frame("fling")
				}
				key(platform.KeyBackspace, platform.CodeBackspace, platform.ModCtrl, "ctrl+backspace")
				x.inject(keyEv(platform.KeyPress, "a", platform.CodeKeyA, platform.ModCtrl))
				x.inject(keyEv(platform.KeyRelease, "a", platform.CodeKeyA, platform.ModCtrl))
				x.frame("select all")
				key(platform.KeyBackspace, platform.CodeBackspace, 0, "clear")
				for range 10 {
					x.frame("settle")
				}
				if x.frames == 0 {
					t.Error("no frame checked")
				}
			})
		}
	}
}

// A selection drag past the right edge level with a line shorter than the
// longest scrolls the field toward the pointer, as Blink's selection
// autoscroll does (it scrolls the point under the pointer into view, then
// extends the selection there), and stops requesting frames at the
// extent. Before, the autoscroll only revealed the caret, which stayed at
// the end of the short line, already visible: nothing scrolled, and a
// frame was requested every refresh until the release.
func TestTextareaNowrapAutoscrollShortLine(t *testing.T) {
	root := div(Class("p-4"), textarea(Key("ta"), Class("h-16 w-40 text-nowrap"), DefaultValue(hscrollLong+"\nab")))
	x := newKeys(t, root, 400, 200)
	x.frame()
	f := fieldOf(t, x.tree(), "ta")
	s := f.bx.scr
	r := windowRect(f.bx)
	l := f.bx.ld.Layout()
	lay := f.layout()
	cy := r.Y0 + l.Border.Top + l.Padding.Top + lay.Caret(textCursor(len(hscrollLong)+1)).Top + 2 // the "ab" row
	x.send(move(r.X0+1, cy), down(r.X0+1, cy), moveHeld(r.X1+40, cy))
	for i := 0; i < 100 && x.w.FrameRequested(); i++ {
		x.frame()
	}
	if s.pos.X != s.max.X {
		t.Errorf("dragging right of a short line: offset %v, want the extent %v", s.pos.X, s.max.X)
	}
	if s0, e := x.selection("ta"); s0 != len(hscrollLong)+1 || e != len(hscrollLong)+3 {
		t.Errorf("selection %d..%d, want %d..%d", s0, e, len(hscrollLong)+1, len(hscrollLong)+3)
	}
	if x.tree().fieldAutoscrolling() {
		t.Error("still autoscrolling at the extent")
	}
	x.settle() // the overlay bar's fade only, while the button is held
	// Back past the left edge: the move changes no selection (the text
	// under the pointer is past the end of "ab" still) and starts the
	// frames itself; the field scrolls back to 0.
	x.send(moveHeld(r.X0-12, cy))
	for i := 0; i < 100 && x.w.FrameRequested(); i++ {
		x.frame()
	}
	if s.pos.X != 0 {
		t.Errorf("dragging left of the field: offset %v, want 0", s.pos.X)
	}
	x.send(up(r.X0-12, cy))
	x.settle()
}

// The vertical autoscroll reaches the extent of a padded Textarea: past
// the bottom, the last line is visible before the bottom padding is.
func TestTextareaAutoscrollReachesPadding(t *testing.T) {
	lines := make([]string, 12)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	root := div(Class("p-4"), textarea(Key("ta"), Class("h-16 w-40 py-3"), DefaultValue(strings.Join(lines, "\n"))))
	x := newKeys(t, root, 400, 300)
	x.frame()
	tr := x.tree()
	f := fieldOf(t, tr, "ta")
	s := f.bx.scr
	l := f.bx.ld.Layout()
	if s.max.Y <= l.Padding.Bottom {
		t.Fatalf("extent %v", s.max)
	}
	// The last line ends at the client's bottom edge.
	tr.scrollDirect(s, gfx.Point{Y: s.max.Y - l.Padding.Bottom})
	x.settle()
	r := windowRect(f.bx)
	cx := r.Center().X
	x.send(move(cx, r.Center().Y), down(cx, r.Center().Y), moveHeld(cx, r.Y1+30))
	for i := 0; i < 100 && x.w.FrameRequested(); i++ {
		x.frame()
	}
	if s.pos.Y != s.max.Y {
		t.Errorf("dragging below: offset %v, want the extent %v", s.pos.Y, s.max.Y)
	}
	if tr.fieldAutoscrolling() {
		t.Error("still autoscrolling at the extent")
	}
	x.send(up(cx, r.Y1+30))
	x.settle()
}

// A click in a horizontally scrolled, padded Textarea leaves the text
// under the pointer where it is: the caret it places is under the
// pointer. Before, the reveal counted the padding on both sides of the
// caret, so a click near the left edge scrolled by up to the padding.
// Home and End still show the padding at the ends of the overflow
// (TestTextareaNowrapRevealsCaret).
func TestTextareaNowrapClickKeepsScroll(t *testing.T) {
	root := div(Class("p-2"), textarea(Key("ta"), Class("h-16 w-40 px-3 text-nowrap"), DefaultValue(hscrollLong)))
	x := newScrollKeys(t, root, 400, 200, Config{InstantScroll: true})
	x.frame()
	tr := x.tree()
	f := fieldOf(t, tr, "ta")
	s := f.bx.scr
	r := windowRect(f.bx)
	l := f.bx.ld.Layout()
	lay := f.layout()
	cy := r.Center().Y
	for _, c := range []struct {
		name   string
		scroll float32
		min    float32 // the caret at least this far into the client area
		last   bool    // the last caret in view, not the first
	}{
		{"near the left edge", 100, 2, false},
		{"the first glyph", 5, 0, false},
		{"near the right edge", 100, 0, true},
	} {
		tr.scrollDirect(s, gfx.Point{X: c.scroll})
		x.settle()
		off, at := -1, float32(0)
		for i := 0; i <= len(hscrollLong); i++ {
			cx := l.Padding.Left + lay.Caret(textCursor(i)).X - c.scroll
			if cx < c.min || cx+caretWidth > s.client.W {
				continue
			}
			if off < 0 || c.last {
				off, at = i, cx
			}
		}
		if off < 0 {
			t.Fatalf("%s: no caret in view", c.name)
		}
		px := r.X0 + l.Border.Left + at + 1
		if c.last {
			px = r.X0 + l.Border.Left + at - 1 // the right half of the glyph before
		}
		x.b.Advance(time.Second) // not a double click
		x.click(px, cy)
		x.settle()
		if s0, e := x.selection("ta"); s0 != off || e != off {
			t.Errorf("%s: caret %d..%d, want %d", c.name, s0, e, off)
		}
		if s.pos.X != c.scroll {
			t.Errorf("%s: the click scrolled from %v to %v", c.name, c.scroll, s.pos.X)
		}
	}
}

// A focus by a pointer that does not place the caret (here set directly;
// a label's press is the other) reveals the caret the field has; only the
// press on the field itself, which puts the caret under the pointer, does
// not reveal the old one.
func TestTextareaNowrapMouseFocusReveals(t *testing.T) {
	root := div(Class("p-2"),
		label(Key("lb"), Class("block"), For("ta"), Text("Notes")),
		textarea(Key("ta"), ID("ta"), Class("h-16 w-40 text-nowrap"), DefaultValue(hscrollLong)),
		input(Key("other")))
	x := newKeys(t, root, 400, 200)
	x.tab()
	x.key(platform.KeyEnd, 0)
	x.settle()
	tr := x.tree()
	f := fieldOf(t, tr, "ta")
	s := f.bx.scr
	if s.pos.X != s.max.X || s.max.X <= 0 {
		t.Fatalf("End: offset %v, extent %v", s.pos.X, s.max.X)
	}
	away := func() {
		t.Helper()
		x.tab() // to "other"
		tr.scrollDirect(s, gfx.Point{})
		x.settle()
		if tr.foc.el == f.el || s.pos.X != 0 {
			t.Fatalf("focus %v, offset %v", tr.foc.el == f.el, s.pos.X)
		}
	}
	away()
	tr.setFocus(f.el, focusMouse)
	x.settle()
	if s.pos.X != s.max.X {
		t.Errorf("a pointer focus without a press on the field: offset %v, want the caret's %v", s.pos.X, s.max.X)
	}
	away()
	x.click(x.center("lb"))
	x.settle()
	if tr.foc.el != f.el || s.pos.X != s.max.X {
		t.Errorf("a label's press: focused %v, offset %v, want %v", tr.foc.el == f.el, s.pos.X, s.max.X)
	}
	away()
	// A press on the field puts the caret under the pointer, at the
	// start: the view stays.
	r := windowRect(f.bx)
	x.click(r.X0+3, r.Center().Y)
	x.settle()
	if tr.foc.el != f.el || s.pos.X != 0 {
		t.Errorf("a press on the field: focused %v, offset %v, want 0", tr.foc.el == f.el, s.pos.X)
	}
}
