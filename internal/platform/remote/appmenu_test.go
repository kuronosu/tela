package remote

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TestQuitRequestForwarded: the host's quit request reaches the active
// child (whose layers above run OnQuit), held while input is frozen.
func TestQuitRequestForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "quit"})
	a.configured()
	p.hb.RequestQuit()
	if got := a.waitEvent("quit request", isType[platform.QuitRequestedEvent]); got != (platform.QuitRequestedEvent{}) {
		t.Errorf("quit request %+v", got)
	}

	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	b.newWindow(platform.WindowOptions{Title: "quit"})
	cb := p.child(1)
	b.configured()
	p.hb.RequestQuit()
	time.Sleep(50 * time.Millisecond)
	for _, e := range append(a.take(), b.take()...) {
		if isType[platform.QuitRequestedEvent](e.e) {
			t.Errorf("a quit request was delivered while input was frozen")
		}
	}
	p.promote(cb)
	b.waitEvent("the queued quit request after the promotion", isType[platform.QuitRequestedEvent])
}

// TestAppMenuForwarded: the active child's app menu bar is the host
// backend's, its choices come back as AppMenuEvents, and a reload's bar
// is set at its promotion.
func TestAppMenuForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "menus"})
	a.configured()
	menu := []platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "New", Accelerator: "Cmd+N"}}},
		{Label: "Edit", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{{ID: 2, Label: "Copy", Role: platform.RoleCopy}}},
	}
	items := []platform.MenuItem{{ID: 3, Label: "Settings…", Accelerator: "Cmd+,"}}
	a.ui(func() {
		if err := a.b.Desktop().SetAppMenu(menu, items); err != nil {
			t.Errorf("SetAppMenu: %v", err)
		}
	})
	if m, it := p.hb.AppMenu(); !reflect.DeepEqual(m, menu) || !reflect.DeepEqual(it, items) {
		t.Errorf("the host's app menu %+v, %+v; want %+v, %+v", m, it, menu, items)
	}
	p.hb.ChooseAppMenu(3)
	if got := a.waitEvent("AppMenuEvent", isType[platform.AppMenuEvent]); got != (platform.AppMenuEvent{Item: 3}) {
		t.Errorf("AppMenuEvent %+v", got)
	}

	// A starting build's bar waits for its promotion.
	changes := p.hb.AppMenuChanges()
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "menus"})
	cb := p.child(1)
	menu2 := []platform.MenuItem{{Label: "Help", Role: platform.RoleHelpMenu, Submenu: []platform.MenuItem{{ID: 9, Label: "Guide"}}}}
	b.ui(func() {
		if err := b.b.Desktop().SetAppMenu(menu2, nil); err != nil {
			t.Errorf("a starting build's SetAppMenu: %v", err)
		}
	})
	if m, _ := p.hb.AppMenu(); !reflect.DeepEqual(m, menu) || p.hb.AppMenuChanges() != changes {
		t.Errorf("a starting build changed the shown app menu: %+v (%d changes, was %d)", m, p.hb.AppMenuChanges(), changes)
	}
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	b.presentNow(wb, 2, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	p.promote(cb)
	if m, it := p.hb.AppMenu(); !reflect.DeepEqual(m, menu2) || it != nil {
		t.Errorf("the app menu after the promotion %+v, %+v; want %+v", m, it, menu2)
	}
	p.hb.ChooseAppMenu(9)
	b.waitEvent("B's AppMenuEvent", isType[platform.AppMenuEvent])
	_ = wa

	// A build without an app menu bar restores the default one.
	p.ui(p.host.Freeze)
	c := p.dial("c").start()
	wc := c.newWindow(platform.WindowOptions{Title: "menus"})
	cc := p.child(2)
	c.waitEvent("C's frame", isType[platform.FrameEvent])
	c.presentNow(wc, 3, nil)
	waitChan(t, cc.FirstFrame(), "C's first frame")
	p.promote(cc)
	if m, it := p.hb.AppMenu(); m != nil || it != nil {
		t.Errorf("the app menu after a build without one %+v, %+v; want the default", m, it)
	}
}

// TestAppMenuUnsupported: a host backend without an app menu bar answers
// ErrUnsupported to active and starting builds alike.
func TestAppMenuUnsupported(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	p.hb.SetUnsupported(headless.ServiceAppMenu)
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "menus"})
	a.configured()
	menu := []platform.MenuItem{{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "New"}}}}
	a.ui(func() {
		if err := a.b.Desktop().SetAppMenu(menu, nil); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("SetAppMenu without an app menu bar = %v, want ErrUnsupported", err)
		}
	})
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	b.newWindow(platform.WindowOptions{Title: "menus"})
	b.ui(func() {
		if err := b.b.Desktop().SetAppMenu(menu, nil); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("a starting build's SetAppMenu without an app menu bar = %v, want ErrUnsupported", err)
		}
	})
	a.ui(func() {
		defer func() {
			if recover() == nil {
				t.Errorf("an invalid app menu did not panic in the child")
			}
		}()
		_ = a.b.Desktop().SetAppMenu(menu, []platform.MenuItem{{ID: 1, Label: "again"}})
	})
}

// TestTitleBarOverlayForwarded: the option reaches the real window, its
// controls' rectangle reaches the child, and a reload that changes the
// option opens a new real window.
func TestTitleBarOverlayForwarded(t *testing.T) {
	controls := platform.Rect{X: 12, Y: 10, W: 54, H: 16}
	p := newPair(t, HostOptions{}, headless.Options{TitleBarControls: controls})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "overlay", TitleBarOverlay: true})
	hw := p.hostWindow("overlay")
	if !hw.Options().TitleBarOverlay {
		t.Errorf("the real window has no TitleBarOverlay: %+v", hw.Options())
	}
	want := platform.WindowState{ClientSideDecorations: true, TitleBarControls: controls}
	a.waitEvent("the overlay's state", func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State == want
	})
	moved := platform.Rect{X: 20, Y: 18, W: 54, H: 16}
	hw.SetTitleBarControls(moved)
	a.waitEvent("the moved controls", func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State.TitleBarControls == moved
	})
	var st platform.WindowState
	a.ui(func() { st = w.State() })
	if st.TitleBarControls != moved {
		t.Errorf("the child window's state %v, want controls %v", st, moved)
	}

	// The option is fixed at creation: a reload without it gets a new real
	// window.
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "overlay"})
	cb := p.child(1)
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	b.presentNow(wb, 2, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	p.promote(cb)
	waitFor(t, "the old real window closes", hw.Closed)
	var stb platform.WindowState
	b.ui(func() { stb = wb.State() })
	if stb.TitleBarControls != (platform.Rect{}) || stb.ClientSideDecorations {
		t.Errorf("a window without the overlay has state %v", stb)
	}
}
