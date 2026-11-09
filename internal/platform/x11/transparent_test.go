//go:build linux

package x11

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// TestTransparentFollowsCompositingManager: an ARGB window is transparent
// while a compositing manager owns _NET_WM_CM_S0; its input shape is the
// mask of its frames then, and the whole window without a manager.
func TestTransparentFollowsCompositingManager(t *testing.T) {
	g := newRig(t)
	x := newFakeX(g.b)
	cmSel := x.id("_NET_WM_CM_S0")
	x.owners[cmSel] = 77
	x.take()
	w := fakeWindow(g.b, 10, image.Pt(100, 80), platform.WindowOptions{Frameless: true, Transparent: true, Size: platform.Size{W: 100, H: 80}})
	w.initTransparent(true)
	g.b.loop.UpdateState(w, w.osState) // as NewWindow, after initTransparent
	g.feed()                            // the window's first configure, state and frame
	if !w.osState.Transparent {
		t.Fatal("a window with a compositing manager is not transparent")
	}
	if got := x.take(); !strings.Contains(got, "select win=1 mask=0x420000") || !strings.Contains(got, "select win=77 mask=0x20000") {
		t.Errorf("requests %q: want StructureNotify on the root and on the manager's window", got)
	}
	if e := w.edgeAt(platform.Point{X: 1, Y: 1}); e != 0 {
		t.Errorf("a transparent window has resize edge %v", e)
	}

	// The input shape follows the frame: the card, not the shadow.
	fb := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 10; y < 60; y++ {
		for x := 20; x < 80; x++ {
			c := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if y >= 50 {
				c = color.RGBA{A: 20}
			}
			fb.SetRGBA(x, y, c)
		}
	}
	w.syncInputShape(fb, []image.Rectangle{fb.Rect})
	if got, want := x.take(), "input shape win=10 [{20 10 60 40}]"; got != want {
		t.Errorf("requests %q, want %q", got, want)
	}
	w.syncInputShape(fb, []image.Rectangle{image.Rect(0, 0, 10, 10)})
	if got := x.take(); got != "" {
		t.Errorf("an unchanged mask sent %q", got)
	}

	// The manager quits: the window is opaque and takes input everywhere.
	delete(x.owners, cmSel)
	g.f.take()
	events, _ := g.feed(xproto.DestroyNotifyEvent{Event: 77, Window: 77})
	if events != "state {csd}" || w.osState.Transparent {
		t.Errorf("events %q after the manager quit, want the opaque state", events)
	}
	if got, want := x.take(), "input shape win=10 [{0 0 65535 65535}]"; got != want {
		t.Errorf("requests %q, want %q", got, want)
	}
	// Opaque frames set no input shape.
	w.syncInputShape(fb, []image.Rectangle{fb.Rect})
	if got := x.take(); got != "" {
		t.Errorf("an opaque window sent %q", got)
	}

	// A new manager announces itself: transparent again; its window is
	// followed.
	x.owners[cmSel] = 88
	manager := xproto.ClientMessageEvent{Format: 32, Window: g.b.root, Type: g.b.cm.manager,
		Data: xproto.ClientMessageDataUnionData32New([]uint32{0, uint32(cmSel), 88, 0, 0})}
	events, _ = g.feed(manager)
	if events != "state {csd transparent}" || !w.osState.Transparent {
		t.Errorf("events %q after a manager started, want the transparent state", events)
	}
	if got := x.take(); got != "select win=88 mask=0x20000" {
		t.Errorf("requests %q, want StructureNotify on the new manager's window", got)
	}
	w.syncInputShape(fb, []image.Rectangle{fb.Rect})
	if got, want := x.take(), "input shape win=10 [{20 10 60 40}]"; got != want {
		t.Errorf("requests %q, want %q", got, want)
	}
}

// A window without the ARGB visual, or without a manager, is opaque.
func TestTransparentWithoutManager(t *testing.T) {
	g := newRig(t)
	newFakeX(g.b)
	w := fakeWindow(g.b, 10, image.Pt(100, 80), platform.WindowOptions{Frameless: true, Transparent: true})
	w.initTransparent(true)
	if w.osState.Transparent {
		t.Error("a window without a compositing manager is transparent")
	}
	v := fakeWindow(g.b, 11, image.Pt(100, 80), platform.WindowOptions{Frameless: true, Transparent: true})
	v.initTransparent(false)
	if v.osState.Transparent {
		t.Error("a window without the ARGB visual is transparent")
	}
}
