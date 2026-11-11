package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// dragScrollHarness mounts root in a 400x300 headless window, starts an
// external drag of text over it, and returns the harness with wait (moves
// the clock by d, running what is due) and frame (one frame interval).
func dragScrollHarness(t *testing.T, root Node) (x *winHarness, w interface{ Inject(platform.Event) }, wait func(time.Duration), frame func()) {
	t.Helper()
	x = newWinHarness(t, Options{Root: root})
	hw := x.windows()[0]
	// The clock moves only when the test says (winHarness.drain runs
	// frames).
	wait = func(d time.Duration) {
		x.b.Advance(d)
		if at, ok := x.h.TimerArmed(); ok && at <= x.b.Now() {
			x.b.Wake()
		}
		x.b.Drain(x.h)
	}
	frame = func() { wait(x.b.FrameInterval()) }
	x.b.StartExternalDrag(platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}, platform.DropCopy)
	return x, hw, wait, frame
}

// dragRows returns n keyed rows of 50 px, keys k0, k1...
func dragRows(k string, n int) []Child {
	out := make([]Child, n)
	for i := range n {
		out[i] = div(Key(fmt.Sprintf("%s%d", k, i)), Class("h-[50px] shrink-0"), Textf("Row %d", i))
	}
	return out
}

// A drag that hovers near the edges of a scroll container scrolls it as
// it scrolls a text field (ADR-265, ADR-272; Chromium's drag-and-drop
// AutoscrollController): from 0.2 s after it entered the 20 px band
// inside the container's border box, every frame reveals the point 20 px
// past the pointer, so a pointer 5 px inside the bottom edge scrolls 16
// px a frame and one 5 px inside the top edge 15 px; the middle scrolls
// nothing, and frames stop at the extents.
func TestDragAutoscrollScrollContainer(t *testing.T) {
	root := div(Class("p-4"), div(append([]Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}, dragRows("r", 20)...)...))
	x, w, wait, frame := dragScrollHarness(t, root)
	tr := x.h.Tree()
	s := scr(t, tr, "list")
	r := windowRect(s.box)
	cx := r.Center().X
	w.Inject(move(cx, r.Center().Y))
	wait(0)
	wait(time.Second)
	if s.pos.Y != 0 || x.windows()[0].FrameRequested() {
		t.Fatalf("the middle of the list scrolls it: %v, frames %v", s.pos, x.windows()[0].FrameRequested())
	}
	w.Inject(move(cx, r.Y1-5))
	wait(0)
	if x.windows()[0].FrameRequested() {
		t.Error("frames run during the delay")
	}
	wait(199 * time.Millisecond)
	if s.pos.Y != 0 {
		t.Fatalf("scrolled %v before the delay", s.pos.Y)
	}
	wait(time.Millisecond)
	base := s.pos.Y
	if base != 0 && base != 16 || !x.windows()[0].FrameRequested() {
		t.Fatalf("after the delay: scroll %v, frames %v", base, x.windows()[0].FrameRequested())
	}
	frame()
	if s.pos.Y != base+16 {
		t.Errorf("one step from 5 px inside the bottom edge scrolled %v, want 16", s.pos.Y-base)
	}
	for i := 0; i < 100 && x.windows()[0].FrameRequested(); i++ {
		frame()
	}
	if s.pos.Y != s.max.Y || s.max.Y != 800 || x.windows()[0].FrameRequested() {
		t.Errorf("at the extent: scroll %v (max %v), frames %v", s.pos.Y, s.max.Y, x.windows()[0].FrameRequested())
	}
	if s.pos.X != 0 {
		t.Errorf("scrolled sideways: %v", s.pos)
	}
	// Through the middle (the autoscroll stops) to the top edge: a new
	// delay, then 15 px a frame.
	w.Inject(move(cx, r.Center().Y))
	wait(0)
	w.Inject(move(cx, r.Y0+5))
	wait(199 * time.Millisecond)
	if s.pos.Y != 800 {
		t.Fatalf("scrolled to %v before the delay", s.pos.Y)
	}
	wait(time.Millisecond)
	base = s.pos.Y
	frame()
	if s.pos.Y != base-15 {
		t.Errorf("one step from 5 px inside the top edge: %v, want %v", s.pos.Y, base-15)
	}
	for i := 0; i < 100 && x.windows()[0].FrameRequested(); i++ {
		frame()
	}
	if s.pos.Y != 0 {
		t.Errorf("back at the start: scroll %v", s.pos.Y)
	}
	x.b.CancelDrag()
	wait(0)
	if x.windows()[0].FrameRequested() {
		t.Error("frames after the drag ended")
	}
}

// Nested containers: the innermost one that overflows under the drag
// scrolls (Chromium's FindAutoscrollable), also when the pointer is in the
// band of an outer one too. Once it is at its extent, the outer one
// scrolls only when the point the step reveals is past its own edge: the
// inner one sits at that edge (ScrollRectToVisible bubbles), not when it
// is inside.
func TestDragAutoscrollNested(t *testing.T) {
	for _, atEdge := range []bool{true, false} {
		t.Run(fmt.Sprintf("at edge %v", atEdge), func(t *testing.T) {
			above := "h-[150px] shrink-0"
			if !atEdge {
				above = "h-[50px] shrink-0"
			}
			inner := append([]Child{Key("inner"), Class("h-[150px] shrink-0 overflow-y-auto")}, dragRows("i", 10)...)
			root := div(Key("outer"), Class("flex flex-col h-[300px] w-[200px] overflow-y-auto"),
				div(Class(above)), div(inner...), div(Class("h-[400px] shrink-0")))
			x, w, wait, frame := dragScrollHarness(t, root)
			tr := x.h.Tree()
			in, out := scr(t, tr, "inner"), scr(t, tr, "outer")
			r := windowRect(in.box)
			w.Inject(move(r.Center().X, r.Y1-5))
			wait(0)
			wait(200 * time.Millisecond)
			for i := 0; i < 10 && x.windows()[0].FrameRequested(); i++ {
				frame()
			}
			if in.pos.Y == 0 || out.pos.Y != 0 {
				t.Fatalf("while the inner list moves: inner %v, outer %v; want only the inner one", in.pos.Y, out.pos.Y)
			}
			for i := 0; i < 200 && x.windows()[0].FrameRequested(); i++ {
				frame()
			}
			wantOut := float32(0)
			if atEdge {
				wantOut = out.max.Y
			}
			if in.pos.Y != in.max.Y || out.pos.Y != wantOut || x.windows()[0].FrameRequested() {
				t.Errorf("at rest: inner %v (max %v), outer %v (want %v), frames %v", in.pos.Y, in.max.Y, out.pos.Y, wantOut, x.windows()[0].FrameRequested())
			}
			x.b.CancelDrag()
			wait(0)
		})
	}
}

// What scrolls is found up the box tree from the element under the drag:
// an Input whose text fits is not autoscrollable, so its container is; a
// container the user cannot scroll (overflow hidden) is skipped for the
// one around it; nothing scrolls from inside a box fixed to the window,
// and the window (the root) scrolls like any container.
func TestDragAutoscrollFindsTheContainer(t *testing.T) {
	type tc struct {
		name   string
		root   Node
		target string // the key of the element the drag hovers near its bottom edge
		moves  string // the key of the container that scrolls, "" for none, "root" for the window
	}
	field := div(append([]Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")},
		append(dragRows("r", 3), input(Key("in"), Class("block h-[40px] w-full"), DefaultValue("short")), div(Class("h-[400px]")))...)...)
	hidden := div(append([]Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")},
		div(Class("h-[100px]")), div(append([]Child{Key("clip"), Class("h-[150px] overflow-hidden")}, dragRows("c", 10)...)...), div(Class("h-[400px]")))...)
	page := func(fixed bool) Node {
		cls := "absolute bottom-0 left-0 h-10 w-full"
		if fixed {
			cls = "fixed bottom-0 left-0 h-10 w-full"
		}
		return div(Class("relative"), div(Class("h-[1000px]")), div(Key("bar"), Class(cls)))
	}
	for _, c := range []tc{
		{"input that fits", field, "in", "list"},
		{"overflow hidden", hidden, "clip", "list"},
		{"fixed to the window", page(true), "bar", ""},
		{"window", page(false), "bar", "root"},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, w, wait, frame := dragScrollHarness(t, c.root)
			tr := x.h.Tree()
			var s *scroller
			switch c.moves {
			case "root":
				s = tr.rootBox.scr
			case "":
			default:
				s = scr(t, tr, c.moves)
			}
			b := tr.boxWindowRect(keyed(tr, c.target).box)
			// Near the bottom edge of the target and of what scrolls.
			edge := float32(300)
			if s != nil && s != tr.rootBox.scr {
				edge = windowRect(s.box).Y1
			}
			py := min(b.Y1, edge) - 5
			w.Inject(move(b.Center().X, py))
			wait(0)
			wait(200 * time.Millisecond)
			for i := 0; i < 5; i++ {
				frame()
			}
			if s == nil {
				if tr.rootBox.scr != nil && tr.rootBox.scr.pos.Y != 0 || x.windows()[0].FrameRequested() {
					t.Errorf("something scrolled from inside a fixed box: root %v, frames %v", tr.rootBox.scr.pos, x.windows()[0].FrameRequested())
				}
				return
			}
			if s.pos.Y == 0 {
				t.Errorf("%s did not scroll", c.moves)
			}
			if c.name == "overflow hidden" && scr(t, tr, "clip").pos.Y != 0 {
				t.Error("the overflow-hidden box scrolled")
			}
			x.b.CancelDrag()
			wait(0)
		})
	}
}
