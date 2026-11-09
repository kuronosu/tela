//go:build linux

package x11

import (
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// TestTransparentXFixesSelection: a compositing manager that takes
// _NET_WM_CM_S0 without the MANAGER message is seen through the XFixes
// selection notification, and so is its end; the selection is watched
// when the tracking starts.
func TestTransparentXFixesSelection(t *testing.T) {
	g := newRig(t)
	x := newFakeX(g.b)
	cmSel := x.id("_NET_WM_CM_S0")
	x.take()
	g.b.xfixes = xfixesExt{checked: true, ok: true, extension: extension{present: true, opcode: 140, firstEvent: 87}}
	w := fakeWindow(g.b, 10, image.Pt(100, 80), platform.WindowOptions{Frameless: true, Transparent: true, Size: platform.Size{W: 100, H: 80}})
	w.initTransparent(true)
	g.b.loop.UpdateState(w, w.osState)
	g.feed()
	if w.osState.Transparent {
		t.Fatal("transparent without a manager")
	}
	if got := x.take(); !strings.Contains(got, "watch selection _NET_WM_CM_S0") {
		t.Errorf("requests %q: want the selection watched", got)
	}
	notify := func(owner xproto.Window, subtype byte, sel xproto.Atom) xgb.RawEvent {
		raw := make([]byte, 32)
		raw[0], raw[1] = 87, subtype
		le.PutUint32(raw[4:], uint32(g.b.root))
		le.PutUint32(raw[8:], uint32(owner))
		le.PutUint32(raw[12:], uint32(sel))
		return raw
	}
	x.owners[cmSel] = 99
	events, _ := g.feed(notify(99, 0, cmSel))
	if events != "state {csd transparent}" {
		t.Errorf("events %q after a silent manager took the selection, want the transparent state", events)
	}
	if got := x.take(); got != "select win=99 mask=0x20000" {
		t.Errorf("requests %q, want StructureNotify on the manager's window", got)
	}
	if events, _ := g.feed(notify(5, 0, 12345)); events != "" {
		t.Errorf("events %q for another selection", events)
	}
	delete(x.owners, cmSel)
	if events, _ := g.feed(notify(0, 2, cmSel)); events != "state {csd}" {
		t.Errorf("events %q after the manager's client closed, want the opaque state", events)
	}
}

// The request's encoding.
func TestSelectSelectionInputRequest(t *testing.T) {
	req := selectSelectionInputRequest(140, 0x1234, 0x56, 7)
	want := []byte{140, 2, 4, 0, 0x34, 0x12, 0, 0, 0x56, 0, 0, 0, 7, 0, 0, 0}
	if string(req) != string(want) {
		t.Errorf("request % x, want % x", req, want)
	}
	if _, ok := decodeSelectionNotify(make([]byte, 31)); ok {
		t.Error("a short event decoded")
	}
}
