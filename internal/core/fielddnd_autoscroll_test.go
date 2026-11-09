package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// A drag that hovers near the edges of a text field scrolls it toward
// them after 0.2 s, as Chromium's drag and drop autoscroll does: the band
// is 20 px inside the box that scrolls, and every frame scrolls the point
// 20 px past the pointer into view, so a pointer 5 px inside an Input's
// right edge scrolls 16 px a frame and one 5 px inside its left edge 15
// px; the drop caret follows the text under the pointer; the delay is a
// deadline, and frames stop at the extent and away from the edges.
func TestDragAutoscrollInput(t *testing.T) {
	const long = "The quick brown fox jumps over the lazy dog, again and again and again"
	root := div(Class("p-4"), input(Key("in"), Class("w-40 border px-3"), DefaultValue(long)))
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	tr := x.h.Tree()
	f := fieldOf(t, tr, "in")
	// The clock moves only when the test says (winHarness.drain runs
	// frames).
	wait := func(d time.Duration) {
		x.b.Advance(d)
		if at, ok := x.h.TimerArmed(); ok && at <= x.b.Now() {
			x.b.Wake()
		}
		x.b.Drain(x.h)
	}
	frame := func() { wait(x.b.FrameInterval()) }
	r := windowRect(f.bx)
	l := f.bx.ld.Layout()
	right := r.X1 - l.Border.Right - l.Padding.Right
	left := r.X0 + l.Border.Left + l.Padding.Left
	cy := r.Center().Y
	x.b.StartExternalDrag(platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}, platform.DropCopy)
	w.Inject(move((left+right)/2, cy)) // the middle: no autoscroll
	wait(0)
	wait(time.Second)
	if f.scrollX != 0 || w.FrameRequested() {
		t.Fatalf("the middle of the field scrolls it: %v, frames %v", f.scrollX, w.FrameRequested())
	}
	w.Inject(move(right-5, cy))
	wait(0)
	if w.FrameRequested() {
		t.Error("frames run during the delay")
	}
	wait(199 * time.Millisecond)
	if f.scrollX != 0 {
		t.Fatalf("scrolled %v before the delay", f.scrollX)
	}
	wait(time.Millisecond)
	// The deadline asked for frames; the first may have run at once.
	base := f.scrollX
	if base != 0 && base != 16 || !w.FrameRequested() {
		t.Fatalf("after the delay: scroll %v, frames %v", base, w.FrameRequested())
	}
	frame()
	if f.scrollX != base+16 {
		t.Errorf("one step from 5 px inside the right edge scrolled %v, want 16", f.scrollX-base)
	}
	frame()
	if f.scrollX != base+32 {
		t.Errorf("two steps scrolled %v, want 32", f.scrollX-base)
	}
	_, hi := f.scrollXRange()
	for i := 0; i < 100 && w.FrameRequested(); i++ {
		frame()
	}
	if f.scrollX != hi || w.FrameRequested() {
		t.Errorf("at the extent: scroll %v (want %v), frames %v", f.scrollX, hi, w.FrameRequested())
	}
	if c, ok := tr.dropCaret(f); !ok || c.Offset < len(long)-3 {
		t.Errorf("drop caret %v %v, want under the pointer at the end of the text", c, ok)
	}
	// Back to the left edge: a new delay, then 15 px a frame.
	w.Inject(move(left+5, cy))
	wait(200 * time.Millisecond)
	base = f.scrollX
	frame()
	if f.scrollX != base-15 {
		t.Errorf("one step from 5 px inside the left edge: %v, want %v", f.scrollX, base-15)
	}
	for i := 0; i < 100 && w.FrameRequested(); i++ {
		frame()
	}
	if f.scrollX != 0 {
		t.Errorf("back at the start: scroll %v", f.scrollX)
	}
	if c, ok := tr.dropCaret(f); !ok || c.Offset > 1 {
		t.Errorf("drop caret %v %v, want at the start", c, ok)
	}
	x.b.CancelDrag()
	wait(0)
	for i := 0; i < 3 && w.FrameRequested(); i++ {
		frame() // the drop caret goes away
	}
	if w.FrameRequested() {
		t.Error("frames after the drag ended")
	}
}

// A Textarea scrolls vertically while a drag hovers near its bottom or
// top edge, to its extents.
func TestDragAutoscrollTextarea(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	root := div(Class("p-4"), textarea(Key("ta"), Class("h-24 w-48 border p-2"), DefaultValue(strings.Join(lines, "\n"))))
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	tr := x.h.Tree()
	f := fieldOf(t, tr, "ta")
	s := f.bx.scr
	// The clock moves only when the test says (winHarness.drain runs
	// frames).
	wait := func(d time.Duration) {
		x.b.Advance(d)
		if at, ok := x.h.TimerArmed(); ok && at <= x.b.Now() {
			x.b.Wake()
		}
		x.b.Drain(x.h)
	}
	frame := func() { wait(x.b.FrameInterval()) }
	r := windowRect(f.bx)
	x.b.StartExternalDrag(platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}, platform.DropCopy)
	w.Inject(move(r.X0+40, r.Y1-5))
	wait(200 * time.Millisecond)
	frame()
	if s.pos.Y <= 0 || s.pos.X != 0 {
		t.Fatalf("no vertical step near the bottom edge: %v", s.pos)
	}
	for i := 0; i < 200 && w.FrameRequested(); i++ {
		frame()
	}
	if s.pos.Y != s.max.Y {
		t.Errorf("at the bottom: %v (max %v)", s.pos, s.max)
	}
	if c, ok := tr.dropCaret(f); !ok || c.Offset < len(strings.Join(lines[:27], "\n")) {
		t.Errorf("drop caret %v %v, want on the last lines", c, ok)
	}
	w.Inject(move(r.X0+40, r.Y0+5))
	wait(200 * time.Millisecond)
	for i := 0; i < 200 && w.FrameRequested(); i++ {
		frame()
	}
	if s.pos.Y != 0 {
		t.Errorf("at the top: %v", s.pos)
	}
	x.b.CancelDrag()
	wait(0)
}
