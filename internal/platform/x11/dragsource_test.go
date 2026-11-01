//go:build linux

package x11

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// sourceRig is a data rig with a scene on the root (1): the app's window
// 7 at (100, 50) 300x200 and window 8 at (100, 300) 300x200 (not
// reparented), and another client's frame 800 at (500, 0) holding its
// XdndAware client window 801.
func sourceRig(t *testing.T) *dataRig {
	g := newDataRig()
	fakeWindow(g.b, 8, image.Pt(300, 200), platform.WindowOptions{})
	g.feed()
	g.r.take()
	x := g.x
	x.origin[7], x.origin[8], x.origin[800], x.origin[801] = image.Pt(100, 50), image.Pt(100, 300), image.Pt(500, 0), image.Pt(510, 20)
	x.children[1] = []fakeChild{
		{7, image.Rect(100, 50, 400, 250)},
		{8, image.Rect(100, 300, 400, 500)},
		{800, image.Rect(500, 0, 900, 400)},
	}
	x.children[800] = []fakeChild{{801, image.Rect(510, 20, 890, 390)}}
	x.setProp(801, g.b.atoms.xdndAware, xproto.AtomAtom, 32, card32s(5))
	// The pointer enters window 7 and presses there.
	x.pointer, x.mask = image.Pt(130, 70), xproto.KeyButMaskButton1
	got := g.feed(xproto.EnterNotifyEvent{Event: 7, EventX: 30, EventY: 20, RootX: 130, RootY: 70},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 30, EventY: 20, RootX: 130, RootY: 70})
	if got != "pointer enter; pointer down" {
		t.Fatalf("press = %q", got)
	}
	return g
}

func (g *dataRig) motion(x, y int) string {
	g.x.pointer = image.Pt(x, y)
	return g.feed(xproto.MotionNotifyEvent{Event: 7, RootX: int16(x), RootY: int16(y), State: xproto.KeyButMaskButton1, Time: 40})
}

func (g *dataRig) releaseButton() string {
	g.x.mask = 0
	return g.feed(xproto.ButtonReleaseEvent{Event: 7, Detail: 1, RootX: int16(g.x.pointer.X), RootY: int16(g.x.pointer.Y),
		State: xproto.KeyButMaskButton1, Time: 50})
}

var dragData = platform.Data{{Type: platform.MIMEText, Bytes: []byte("drag")}, {Type: "image/png", Bytes: []byte("PNG")}}

func TestDragSourceToXdndTarget(t *testing.T) {
	g := sourceRig(t)
	a := &g.b.atoms
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy | platform.DropMove}); err != nil {
		t.Fatal(err)
	}
	want := "grab pointer win=7 grabbing; own XdndSelection win=7; " +
		"prop XdndTypeList win=7 ATOM = [text/plain;charset=utf-8 UTF8_STRING text/plain TEXT STRING image/png]; " +
		"prop XdndActionList win=7 ATOM = [XdndActionCopy XdndActionMove]; grab keyboard win=7; cursor no-drop"
	if got := g.x.take(); got != want {
		t.Errorf("StartDrag sent:\n got %s\nwant %s", got, want)
	}
	// The source window loses the pointer, then the drag enters it.
	if got, w := g.pass(), "pointer cancel; pointer leave; drag enter 30,20 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app"; got != w {
		t.Errorf("start = %q, want %q", got, w)
	}
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
		t.Errorf("second StartDrag = %v", err)
	}

	// Over the other client's window: leave ours, XdndEnter (more than
	// three types: XdndTypeList) and a position with the default action.
	if got, w := g.motion(600, 100), "drag leave 30,20 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app"; got != w {
		t.Errorf("leaving = %q", got)
	}
	want = "msg XdndEnter to=801 win=801 [7 83886081 text/plain;charset=utf-8 UTF8_STRING text/plain]; " +
		"msg XdndPosition to=801 win=801 [7 0 600,100 40 XdndActionCopy]"
	if got := g.x.take(); got != want {
		t.Errorf("entering the target:\n got %s\nwant %s", got, want)
	}
	// One position in flight: the next waits for the status.
	g.motion(610, 100)
	if got := g.x.take(); got != "" {
		t.Errorf("position while waiting: %s", got)
	}
	g.feed(clientMsg(7, a.xdndStatus, 801, 3, 0, 0, uint32(a.xdndActionCopy)))
	if got, w := g.x.take(), "cursor copy; msg XdndPosition to=801 win=801 [7 0 610,100 40 XdndActionCopy]"; got != w {
		t.Errorf("after the status: %s", got)
	}
	g.feed(clientMsg(7, a.xdndStatus, 801, 3, 0, 0, uint32(a.xdndActionCopy)))

	// The release drops; the target converts, then finishes.
	g.releaseButton()
	if got, w := g.x.take(), "msg XdndDrop to=801 win=801 [7 0 50 0 0]; ungrab pointer; ungrab keyboard"; got != w {
		t.Errorf("drop: %s", got)
	}
	g.feed(xproto.SelectionRequestEvent{Owner: 7, Requestor: 801, Selection: a.xdndSelection, Target: g.x.id("image/png"), Property: g.x.id("XdndData"), Time: 50})
	if got, w := g.x.take(), `prop XdndData win=801 image/png = "PNG"; notify win=801 XdndSelection image/png prop=XdndData`; got != w {
		t.Errorf("serving the drop: %s", got)
	}
	if got, w := g.feed(clientMsg(7, a.xdndFinished, 801, 1, uint32(a.xdndActionCopy))), "done copy"; got != w {
		t.Errorf("finish = %q", got)
	}
	if got, w := g.x.take(), "own XdndSelection win=0"; got != w {
		t.Errorf("after the drag: %s", got)
	}
}

func TestDragSourceToOwnWindow(t *testing.T) {
	g := sourceRig(t)
	g.r.react = func(w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && w == platform.Window(g.b.windows[8]) && de.Action != platform.DragLeave {
			w.SetDropEffect(platform.DropMove, "image/png")
		}
	}
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy | platform.DropMove}); err != nil {
		t.Fatal(err)
	}
	g.pass()
	got := g.motion(200, 400)
	if w := "drag leave 30,20 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app; drag enter 100,100 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app"; got != w {
		t.Errorf("into window 8 = %q, want %q", got, w)
	}
	g.motion(201, 400)
	g.x.take()
	got = strings.Trim(g.releaseButton()+"; "+g.pass(), "; ")
	if w := "drag drop 101,100 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app data=[image/png]; done move"; got != w {
		t.Errorf("drop on window 8 = %q, want %q", got, w)
	}
	if got, w := g.x.take(), "ungrab pointer; ungrab keyboard; own XdndSelection win=0"; got != w {
		t.Errorf("server: %s", got)
	}
}

func TestDragSourceCancelAndRefusal(t *testing.T) {
	// Escape cancels.
	g := sourceRig(t)
	a := &g.b.atoms
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.x.take()
	got := g.feed(xproto.KeyPressEvent{Event: 7, Detail: 9})
	if w := "done none"; got != w {
		t.Errorf("Escape = %q", got)
	}
	if got, w := g.x.take(), "msg XdndLeave to=801 win=801 [7 0 0 0 0]; ungrab pointer; ungrab keyboard; own XdndSelection win=0"; got != w {
		t.Errorf("Escape sent %s", got)
	}

	// A release before the status: the drop waits for it, and a refusal
	// leaves.
	g = sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.releaseButton()
	g.x.take()
	got = g.feed(clientMsg(7, a.xdndStatus, 801, 2, 0, 0, 0))
	if w := "done none"; got != w {
		t.Errorf("refused = %q", got)
	}
	if got := g.x.take(); !strings.HasPrefix(got, "msg XdndLeave to=801") {
		t.Errorf("refused sent %s", got)
	}

	// No status at all: the timeout leaves.
	g = sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.releaseButton()
	g.x.fire(dragStatusTimeout)
	if got := g.pass(); got != "done none" {
		t.Errorf("status timeout = %q", got)
	}

	// Released over nothing.
	g = sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(950, 450)
	if got := g.releaseButton(); got != "drag leave 30,20 [text/plain;charset=utf-8] allowed=copy mods=0 app; done none" &&
		got != "done none" {
		t.Errorf("drop over nothing = %q", got)
	}

	// Without a held press: ErrNoGrab.
	g = sourceRig(t)
	g.releaseButton()
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
		t.Errorf("StartDrag without a press = %v", err)
	}
}

func TestDragSourceWindowCloses(t *testing.T) {
	g := sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.x.take()
	g.w.Close()
	if got := g.x.take(); !strings.HasPrefix(got, "msg XdndLeave to=801 win=801 [7 0 0 0 0]; ungrab pointer") {
		t.Errorf("close sent %s", got)
	}
	if g.b.integ.drag != nil {
		t.Errorf("the drag outlived its window")
	}
}
