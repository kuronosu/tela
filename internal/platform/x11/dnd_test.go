//go:build linux

package x11

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

const xdndSource = 600

// dndRig is a data rig whose window 7 is at (100, 50) on the root, with
// another client's XDND source window.
func dndRig() *dataRig {
	g := newDataRig()
	g.x.origin[7] = image.Pt(100, 50)
	return g
}

func (g *dataRig) xdnd(typ xproto.Atom, data ...uint32) string {
	return g.feed(clientMsg(7, typ, data...))
}

// position sends an XdndPosition from the source with the pointer at
// (x, y) on the root.
func (g *dataRig) position(x, y int, action xproto.Atom) string {
	g.x.pointer = image.Pt(x, y)
	return g.xdnd(g.b.atoms.xdndPosition, xdndSource, 0, uint32(x)<<16|uint32(y), 20, uint32(action))
}

func TestXdndTarget(t *testing.T) {
	g := dndRig()
	a := &g.b.atoms
	utf8, uris := g.x.id("UTF8_STRING"), g.x.id("text/uri-list")
	answer := platform.DropCopy
	var want []string
	g.r.react = func(w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && de.Action != platform.DragLeave {
			w.SetDropEffect(answer, want...)
		}
	}

	if got := g.xdnd(a.xdndEnter, xdndSource, 5<<24, uint32(utf8), uint32(uris), 0); got != "" {
		t.Errorf("XdndEnter delivered %q", got)
	}
	want = []string{platform.MIMEURIList}
	g.x.mask = xproto.KeyButMaskControl
	if got, w := g.position(130, 70, a.xdndActionCopy), "drag enter 30,20 [text/plain;charset=utf-8 text/uri-list] allowed=copy mods=10"; got != w {
		t.Errorf("first position = %q, want %q", got, w)
	}
	if got, w := g.x.take(), "msg XdndStatus to=600 win=600 [7 3 0 0 XdndActionCopy]"; got != w {
		t.Errorf("status = %q, want %q", got, w)
	}
	// The same position: no event, but a status.
	if got := g.position(130, 70, a.xdndActionCopy); got != "" {
		t.Errorf("repeated position delivered %q", got)
	}
	if got, w := g.x.take(), "msg XdndStatus to=600 win=600 [7 3 0 0 XdndActionCopy]"; got != w {
		t.Errorf("repeated status = %q", got)
	}
	// Move asks the app (Shift): DragOver; the app's copy is not allowed,
	// so the status refuses.
	g.x.mask = xproto.KeyButMaskShift
	if got, w := g.position(140, 70, a.xdndActionMove), "drag over 40,20 [text/plain;charset=utf-8 text/uri-list] allowed=move mods=1"; got != w {
		t.Errorf("move = %q, want %q", got, w)
	}
	if got, w := g.x.take(), "msg XdndStatus to=600 win=600 [7 2 0 0 None]"; got != w {
		t.Errorf("refusing status = %q", got)
	}
	answer = platform.DropMove
	g.position(141, 70, a.xdndActionMove)
	g.x.take()

	// The drop reads the wanted type with the drop's time, then delivers
	// it, then reports the effect the window holds.
	if got := g.xdnd(a.xdndDrop, xdndSource, 0, 30); got != "" {
		t.Errorf("XdndDrop delivered %q before the data", got)
	}
	if got, w := g.x.take(), "create selection window; convert XdndSelection text/uri-list to TELA_SELECTION t=30"; got != w {
		t.Errorf("drop conversion = %q, want %q", got, w)
	}
	g.r.react = nil
	got := g.answer(a.xdndSelection, uris, uris, 8, []byte("file:///a\r\n"))
	got += g.pass()
	if w := "drag drop 41,20 [text/plain;charset=utf-8 text/uri-list] allowed=move mods=1 data=[text/uri-list]"; got != w {
		t.Errorf("drop = %q, want %q", got, w)
	}
	if d := g.r.data[0]; len(d) != 1 || string(d[0].Bytes) != "file:///a\r\n" {
		t.Errorf("drop data = %v", d)
	}
	if got, w := g.x.take(), "read+delete TELA_SELECTION win=900; msg XdndFinished to=600 win=600 [7 1 XdndActionMove 0 0]"; got != w {
		t.Errorf("finish = %q, want %q", got, w)
	}
	// Late messages of the finished session are ignored.
	if got := g.xdnd(a.xdndLeave, xdndSource); got != "" {
		t.Errorf("leave after the drop = %q", got)
	}
}

// With XKB, a session queries the pointer once, at its first position:
// later positions are translated with the window's origin it read, and
// the modifiers follow XKB's StateNotify (the pointer's mask is not read
// again). Without XKB each position queries it, so the modifiers stay
// current.
func TestXdndTargetPositionsWithoutRoundTrips(t *testing.T) {
	g := dndRig()
	g.b.xkb = xkbExt{extension: extension{present: true, opcode: 135, firstEvent: xkbFirstEvent}, ok: true}
	a := &g.b.atoms
	png := uint32(g.x.id("image/png"))
	g.xdnd(a.xdndEnter, xdndSource, 5<<24, png)
	g.x.mask = xproto.KeyButMaskControl
	if got, w := g.position(130, 70, a.xdndActionCopy), "drag enter 30,20 [image/png] allowed=copy mods=10"; got != w {
		t.Errorf("first position = %q, want %q", got, w)
	}
	// The mask changes without a StateNotify: not read.
	g.x.mask = xproto.KeyButMaskShift
	if got, w := g.position(140, 75, a.xdndActionCopy), "drag over 40,25 [image/png] allowed=copy mods=10"; got != w {
		t.Errorf("second position = %q, want %q", got, w)
	}
	// Shift pressed, Control released: StateNotify gives the modifiers
	// of the next position.
	g.feed(stateNotify(byte(xproto.KeyButMaskShift), 0, 0, 50))
	if got, w := g.position(140, 75, a.xdndActionCopy), "drag over 40,25 [image/png] allowed=copy mods=1"; got != w {
		t.Errorf("after StateNotify = %q, want %q", got, w)
	}
	g.feed(stateNotify(0, 0, 0, 50))
	if got, w := g.position(160, 90, a.xdndActionCopy), "drag over 60,40 [image/png] allowed=copy mods=0"; got != w {
		t.Errorf("after the release = %q, want %q", got, w)
	}
	if g.x.queries != 1 {
		t.Errorf("%d QueryPointer round trips in the session, want 1", g.x.queries)
	}
	// A new session reads the origin again (the window moved meanwhile).
	g.xdnd(a.xdndLeave, xdndSource)
	g.x.origin[7] = image.Pt(200, 100)
	g.xdnd(a.xdndEnter, xdndSource, 5<<24, png)
	if got, w := g.position(230, 120, a.xdndActionCopy), "drag enter 30,20 [image/png] allowed=copy mods=1"; got != w {
		t.Errorf("new session = %q, want %q", got, w)
	}
	if g.x.queries != 2 {
		t.Errorf("%d QueryPointer round trips after two sessions, want 2", g.x.queries)
	}

	// Without XKB, every position reads the mask.
	g.b.xkb = xkbExt{}
	g.x.mask = xproto.KeyButMaskControl
	if got, w := g.position(231, 120, a.xdndActionCopy), "drag over 31,20 [image/png] allowed=copy mods=10"; got != w {
		t.Errorf("without XKB = %q, want %q", got, w)
	}
	if g.x.queries != 3 {
		t.Errorf("%d QueryPointer round trips, want 3", g.x.queries)
	}
}

func TestXdndTargetRefusedAndLeave(t *testing.T) {
	g := dndRig()
	a := &g.b.atoms
	png := g.x.id("image/png")

	// A drop the app refuses: DragLeave, XdndFinished not accepted.
	g.xdnd(a.xdndEnter, xdndSource, 5<<24, uint32(png))
	g.position(110, 60, a.xdndActionCopy)
	g.x.take()
	got := g.xdnd(a.xdndDrop, xdndSource, 0, 30) + g.pass()
	if w := "drag leave 10,10 [image/png] allowed=copy mods=0"; got != w {
		t.Errorf("refused drop = %q, want %q", got, w)
	}
	if got, w := g.x.take(), "msg XdndFinished to=600 win=600 [7 0 None 0 0]"; got != w {
		t.Errorf("refused finish = %q", got)
	}

	// XdndLeave ends the session.
	g.xdnd(a.xdndEnter, xdndSource, 5<<24, uint32(png))
	g.position(110, 60, a.xdndActionCopy)
	if got, w := g.xdnd(a.xdndLeave, xdndSource), "drag leave 10,10 [image/png] allowed=copy mods=0"; got != w {
		t.Errorf("leave = %q", got)
	}
	// A leave before any position delivers nothing.
	g.xdnd(a.xdndEnter, xdndSource, 5<<24, uint32(png))
	if got := g.xdnd(a.xdndLeave, xdndSource); got != "" {
		t.Errorf("leave without enter = %q", got)
	}
	// Messages of another source, or an old version, are ignored.
	g.xdnd(a.xdndEnter, xdndSource, 2<<24, uint32(png))
	if got := g.position(110, 60, a.xdndActionCopy); got != "" {
		t.Errorf("version 2 source = %q", got)
	}
}

func TestXdndTargetTypeListAndAsk(t *testing.T) {
	g := dndRig()
	a := &g.b.atoms
	var list []xproto.Atom
	for _, n := range []string{"text/html", "STRING", "UTF8_STRING", "image/png", "TARGETS"} {
		list = append(list, g.x.id(n))
	}
	g.x.setProp(xdndSource, a.xdndTypeList, xproto.AtomAtom, 32, atomList(list...))
	g.x.setProp(xdndSource, a.xdndActionList, xproto.AtomAtom, 32, atomList(a.xdndActionCopy, a.xdndActionLink))
	g.xdnd(a.xdndEnter, xdndSource, 5<<24|1, uint32(list[0]), uint32(list[1]), uint32(list[2]))
	if got, w := g.position(110, 60, a.xdndActionAsk), "drag enter 10,10 [text/html text/plain;charset=utf-8 image/png] allowed=copy|link mods=0"; got != w {
		t.Errorf("ask = %q, want %q", got, w)
	}
	// The app's own drag over its window is FromApp; another window of
	// the app as the source is too.
	g.xdnd(a.xdndLeave, xdndSource)
	g.xdnd(a.xdndEnter, 7, 5<<24, uint32(list[0]))
	if got := g.feed(clientMsg(7, a.xdndPosition, 7, 0, 110<<16|60, 20, uint32(a.xdndActionCopy))); got != "drag enter 10,10 [text/html] allowed=copy mods=0 app" {
		t.Errorf("own source = %q", got)
	}
}

func TestXdndTargetWindowCloses(t *testing.T) {
	g := dndRig()
	a := &g.b.atoms
	g.r.react = func(w platform.Window, e platform.Event) { w.SetDropEffect(platform.DropCopy) }
	g.xdnd(a.xdndEnter, xdndSource, 5<<24, uint32(g.x.id("UTF8_STRING")))
	g.position(110, 60, a.xdndActionCopy)
	g.xdnd(a.xdndDrop, xdndSource, 0, 30)
	g.x.take()
	g.w.Close()
	g.answer(a.xdndSelection, g.x.id("UTF8_STRING"), g.x.id("UTF8_STRING"), 8, []byte("x"))
	if got := g.x.take(); got != "msg XdndFinished to=600 win=600 [7 0 None 0 0]; read+delete TELA_SELECTION win=900" &&
		got != "read+delete TELA_SELECTION win=900; msg XdndFinished to=600 win=600 [7 0 None 0 0]" {
		t.Errorf("close during the drop: %q", got)
	}
}
