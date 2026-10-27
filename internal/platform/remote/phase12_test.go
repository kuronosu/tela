package remote

import (
	"errors"
	"image"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Tests of the phase-12 window control and desktop services over the
// remote backend (ADR-184).

// hostWindow returns the open headless window titled title, waiting for
// it.
func (p *pair) hostWindow(title string) *headless.Window {
	p.t.Helper()
	var w *headless.Window
	waitFor(p.t, "host window "+title, func() bool {
		for _, x := range p.hb.Windows() {
			if x.Title() == title {
				w = x
				return true
			}
		}
		return false
	})
	return w
}

// keyed creates a keyed window on the child's UI thread.
func (a *app) keyed(opts platform.WindowOptions, key string) platform.Window {
	a.t.Helper()
	var w platform.Window
	a.ui(func() {
		var err error
		if w, err = a.b.NewKeyedWindow(opts, key); err != nil {
			a.t.Errorf("NewKeyedWindow: %v", err)
		}
	})
	return w
}

// promote promotes c on the host's UI thread.
func (p *pair) promote(c *Child) {
	p.t.Helper()
	var err error
	p.ui(func() { err = p.host.Promote(c) })
	if err != nil {
		p.t.Fatalf("Promote: %v", err)
	}
}

// waitChan waits until ch is closed.
func waitChan(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func isState(want platform.WindowState) func(platform.Event) bool {
	return func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State == want
	}
}

func TestWindowOptionsForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	want := platform.WindowOptions{Title: "opts", AppID: "com.example.opts", Size: platform.Size{W: 200, H: 100},
		MinSize: platform.Size{W: 50, H: 40}, MaxSize: platform.Size{W: 900, H: 800}, Position: image.Pt(30, 40), HasPosition: true,
		AlwaysOnTop: true, Frameless: true, RestoreToken: "headless:9"}
	a.newWindow(want)
	hw := p.hostWindow("opts")
	if got := hw.Options(); got != want {
		t.Errorf("real window options\n got %+v\nwant %+v", got, want)
	}
	if !hw.AlwaysOnTop() {
		t.Errorf("AlwaysOnTop not applied")
	}
	a.waitEvent("frameless state", func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State.ClientSideDecorations && se.State.HasPosition && se.State.Position == image.Pt(30, 40)
	})
	var tok string
	a.ui(func() { tok = a.win.RestoreToken() })
	if tok != "headless:9" || tok != hw.RestoreToken() {
		t.Errorf("RestoreToken %q, real %q", tok, hw.RestoreToken())
	}
}

func TestWindowControlForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "ctl", Size: platform.Size{W: 120, H: 80}})
	a.configured()
	hw := p.hostWindow("ctl")

	a.ui(func() { w.SetMaximized(true) })
	a.waitEvent("maximized", isState(platform.WindowState{Maximized: true}))
	var st platform.WindowState
	a.ui(func() { st = w.State() })
	if !st.Maximized {
		t.Errorf("State() = %v after the maximized event", st)
	}
	a.ui(func() { w.SetMaximized(false); w.SetFullscreen(true) })
	a.waitEvent("fullscreen", isState(platform.WindowState{Fullscreen: true}))
	a.ui(func() { w.SetFullscreen(false); w.SetPosition(image.Pt(-5, 7)) })
	a.waitEvent("position", isState(platform.WindowState{HasPosition: true, Position: image.Pt(-5, 7)}))
	a.ui(func() { w.Minimize() })
	a.waitEvent("minimized", isState(platform.WindowState{Minimized: true, HasPosition: true, Position: image.Pt(-5, 7)}))
	a.ui(func() { w.Activate() })
	a.waitEvent("activated", isState(platform.WindowState{HasPosition: true, Position: image.Pt(-5, 7)}))
	if hw.Activations() != 1 {
		t.Errorf("Activations = %d", hw.Activations())
	}

	icon := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	icon.Pix[3] = 255
	regions := []platform.HitRegion{{Kind: platform.HitCaption, Rect: platform.Rect{W: 120, H: 24}},
		{Kind: platform.HitClose, Rect: platform.Rect{X: 96, W: 24, H: 24}}}
	a.ui(func() {
		w.SetAlwaysOnTop(true)
		w.SetMinSize(platform.Size{W: 60, H: 50})
		w.SetMaxSize(platform.Size{W: 600, H: 500})
		w.SetIcon([]image.Image{icon})
		w.SetHitRegions(regions)
		w.SetHitRegions(slices.Clone(regions)) // equal: not resent
	})
	waitFor(t, "state requests", func() bool {
		mn, mx := hw.SizeLimits()
		return hw.AlwaysOnTop() && mn == (platform.Size{W: 60, H: 50}) && mx == (platform.Size{W: 600, H: 500}) &&
			len(hw.Icon()) == 1 && platform.EqualHitRegions(hw.HitRegions(), regions)
	})
	if n := hw.HitRegionChanges(); n != 1 {
		t.Errorf("HitRegionChanges = %d, want 1", n)
	}
	if got := hw.Icon()[0].(*image.NRGBA); got.Pix[3] != 255 || got.Rect != icon.Rect {
		t.Errorf("icon not forwarded intact")
	}

	a.ui(func() { w.SetVisible(false) })
	waitFor(t, "hidden", func() bool { return !hw.Visible() })
	a.ui(func() { w.SetVisible(true) })
	waitFor(t, "shown", hw.Visible)
}

// TestStateKeptForNewChild checks that a child that maps onto a real window
// later (a reload) gets its current state right after its ConfigureEvent,
// that a pending child's actions do not move the window, and that its
// state requests apply at the promotion.
func TestStateKeptForNewChild(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "state", Size: platform.Size{W: 100, H: 80}})
	a.configured()
	hw := p.hostWindow("state")
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	maxed := platform.WindowState{Maximized: true}
	hw.SetState(maxed)
	a.waitEvent("A maximized", isState(maxed))

	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "state", Size: platform.Size{W: 100, H: 80}})
	cb := p.child(1)
	b.configured()
	b.waitEvent("B told the state", isState(maxed))
	var st platform.WindowState
	b.ui(func() { st = wb.State() })
	if st != maxed {
		t.Errorf("B's State() = %v", st)
	}
	// A pending child's actions are dropped; its state requests wait.
	b.ui(func() {
		wb.SetMaximized(false)
		wb.SetAlwaysOnTop(true)
	})
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	frameB := b.presentNow(wb, 2, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	p.ui(func() {})
	if !hw.OSState().Maximized || hw.AlwaysOnTop() {
		t.Errorf("a pending child changed the real window: %v, on top %v", hw.OSState(), hw.AlwaysOnTop())
	}
	p.promote(cb)
	waitFrame(t, hw, "B's frame", frameB)
	waitFor(t, "B's always-on-top applied", hw.AlwaysOnTop)
	if !hw.OSState().Maximized {
		t.Errorf("the promotion unmaximized the window")
	}
}

// TestKeyedWindowsAcrossSwap checks that the next build's windows take
// over the real windows with their keys, whatever their order, that a
// window it does not open closes, and that a window whose creation-only
// options changed opens anew.
func TestKeyedWindowsAcrossSwap(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	ma := a.newWindow(platform.WindowOptions{Title: "main", Size: platform.Size{W: 100, H: 80}})
	a.keyed(platform.WindowOptions{Title: "x", Parent: ma}, "w:x")
	a.keyed(platform.WindowOptions{Title: "y", Parent: ma, Size: platform.Size{W: 50, H: 40}}, "w:y")
	a.keyed(platform.WindowOptions{Title: "f", Parent: ma}, "w:f")
	hMain, hX, hY, hF := p.hostWindow("main"), p.hostWindow("x"), p.hostWindow("y"), p.hostWindow("f")
	if hY.Parent() != hMain {
		t.Errorf("the real child window's parent is %v, want the main window", hY.Parent())
	}

	b := p.dial("b").start()
	mb := b.newWindow(platform.WindowOptions{Title: "main 2", Size: platform.Size{W: 999, H: 999}})
	b.keyed(platform.WindowOptions{Title: "z", Parent: mb}, "w:z")
	yb := b.keyed(platform.WindowOptions{Title: "y 2", Parent: mb, Size: platform.Size{W: 1, H: 1}}, "w:y")
	b.keyed(platform.WindowOptions{Title: "f 2", Parent: mb, Frameless: true}, "w:f")
	cb := p.child(1)
	b.waitEvent("B's y configured", func(e platform.Event) bool {
		ce, ok := e.(platform.ConfigureEvent)
		return ok && ce.Size == platform.Size{W: 50, H: 40}
	})
	_ = yb
	p.promote(cb)
	waitFor(t, "x closed", hX.Closed)
	waitFor(t, "old f closed", hF.Closed)
	if hMain.Closed() || hY.Closed() {
		t.Fatalf("a window the new build kept was closed")
	}
	waitFor(t, "titles", func() bool { return hMain.Title() == "main 2" && hY.Title() == "y 2" })
	hZ, hF2 := p.hostWindow("z"), p.hostWindow("f 2")
	if hZ.Parent() != hMain || hF2.Parent() != hMain || !hF2.Options().Frameless {
		t.Errorf("new windows: z parent %v, f parent %v frameless %v", hZ.Parent(), hF2.Parent(), hF2.Options().Frameless)
	}
	if n := len(p.hb.Windows()); n != 4 {
		t.Errorf("%d real windows open, want 4", n)
	}
}

// TestCloseClosesChildren checks that closing a window closes its child
// windows first, on both sides.
func TestCloseClosesChildren(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	parent := a.newWindow(platform.WindowOptions{Title: "parent"})
	modal := a.newWindow(platform.WindowOptions{Title: "modal", Parent: parent, Modal: true})
	child := a.newWindow(platform.WindowOptions{Title: "child", Parent: modal})
	hp, hm, hc := p.hostWindow("parent"), p.hostWindow("modal"), p.hostWindow("child")
	if !hp.Blocked() || hm.Parent() != hp || hc.Parent() != hm {
		t.Errorf("parent links or modality not forwarded")
	}
	a.ui(func() { parent.Close() })
	waitFor(t, "every real window closed", func() bool { return hp.Closed() && hm.Closed() && hc.Closed() })
	a.ui(func() {
		for _, w := range []platform.Window{modal, child} {
			if w.(*Window).open() {
				t.Errorf("a child window is still open after its parent closed")
			}
		}
	})
}

// TestHiddenWindowSwap checks close-to-tray across a reload: a build whose
// windows are all hidden is ready without a frame, a build that takes over
// a hidden real window learns that it is hidden, and one that shows it
// gets its first frame although the real window is hidden.
func TestHiddenWindowSwap(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "tray app", Size: platform.Size{W: 90, H: 60}})
	a.configured()
	hw := p.hostWindow("tray app")
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	a.ui(func() { wa.SetVisible(false) })
	waitFor(t, "hidden", func() bool { return !hw.Visible() })

	// B adopts the hidden state (as tela dev's child runtime does) and is
	// ready without presenting.
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "tray app"})
	cb := p.child(1)
	hidden, ok := wb.(*Window).HostHidden(waitTimeout)
	if !ok || !hidden {
		t.Fatalf("HostHidden = %v, %v; want true, true", hidden, ok)
	}
	b.ui(func() { wb.SetVisible(false) })
	select {
	case <-cb.FirstFrame():
	case <-time.After(waitTimeout):
		t.Fatal("a build with every window hidden never became ready")
	}
	p.promote(cb)
	p.ui(func() {})
	if hw.Visible() {
		t.Errorf("the promotion showed a window the new build keeps hidden")
	}

	// C shows its window: it gets a frame offscreen, and the promotion
	// shows the real window with C's frame.
	p.ui(p.host.Freeze)
	c := p.dial("c").start()
	wc := c.newWindow(platform.WindowOptions{Title: "tray app"})
	cc := p.child(2)
	c.waitEvent("C's offscreen frame", isType[platform.FrameEvent])
	frameC := c.presentNow(wc, 3, nil)
	waitChan(t, cc.FirstFrame(), "C's first frame")
	p.promote(cc)
	waitFor(t, "shown at the promotion", hw.Visible)
	waitFrame(t, hw, "C's frame", frameC)
}

// TestAppEventsToActive checks that notification and tray events reach the
// active child only, held while input is frozen.
func TestAppEventsToActive(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "events"})
	a.configured()
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	var tray platform.TrayID
	var note platform.NotificationID
	a.ui(func() {
		var err error
		if tray, err = a.b.Desktop().NewTray(platform.TrayOptions{Icon: icon, Menu: []platform.MenuItem{{ID: 4, Label: "Quit"}}}); err != nil {
			t.Errorf("NewTray: %v", err)
		}
		if note, err = a.b.Desktop().Notify(platform.Notification{Title: "hi"}); err != nil {
			t.Errorf("Notify: %v", err)
		}
	})
	te := platform.TrayEvent{Tray: tray, Kind: platform.TrayMenuItem, Item: 4}
	p.hb.InjectApp(te)
	if got := a.waitEvent("tray event", isType[platform.TrayEvent]); got != te {
		t.Errorf("tray event %+v, want %+v", got, te)
	}
	ne := platform.NotificationEvent{ID: note, Kind: platform.NotificationActivated}
	p.hb.InjectApp(ne)
	if got := a.waitEvent("notification event", isType[platform.NotificationEvent]); got != ne {
		t.Errorf("notification event %+v, want %+v", got, ne)
	}

	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	b.newWindow(platform.WindowOptions{Title: "events"})
	cb := p.child(1)
	b.configured()
	p.hb.InjectApp(te)
	time.Sleep(50 * time.Millisecond)
	for _, e := range append(a.take(), b.take()...) {
		if isType[platform.TrayEvent](e.e) {
			t.Errorf("a tray event was delivered while input was frozen")
		}
	}
	p.promote(cb)
	b.waitEvent("queued tray event after the promotion", isType[platform.TrayEvent])
}

func TestDesktopForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	p.hb.SetDialogResponder(func(d headless.Dialog) headless.DialogResponse {
		switch d.Kind {
		case headless.DialogOpenFiles:
			return headless.DialogResponse{Paths: []string{"/a.txt", "/b.txt"}}
		case headless.DialogSaveFile:
			return headless.DialogResponse{Paths: []string{"/" + d.File.Name}}
		case headless.DialogPickFolder:
			return headless.DialogResponse{} // canceled
		}
		return headless.DialogResponse{Answer: platform.AnswerYes}
	})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "desk"})
	a.configured()
	hw := p.hostWindow("desk")
	d := a.b.Desktop()

	a.ui(func() {
		if err := d.OpenURI(w, "https://example.com"); err != nil {
			t.Errorf("OpenURI: %v", err)
		}
		if err := d.OpenPath(nil, "/tmp"); err != nil {
			t.Errorf("OpenPath: %v", err)
		}
	})
	if o := p.hb.Opened(); len(o) != 2 || o[0].Target != "https://example.com" || o[0].Parent != hw || !o[1].Path || o[1].Parent != nil {
		t.Errorf("opened %+v", o)
	}

	type result struct {
		paths  []string
		path   string
		answer platform.MessageAnswer
		err    error
	}
	got := make(chan result, 4)
	a.ui(func() {
		filters := []platform.FileFilter{{Name: "Text", Patterns: []string{"*.txt"}}}
		if err := d.OpenFiles(platform.FileDialogOptions{Parent: w, Multiple: true, Filters: filters}, func(p []string, err error) {
			got <- result{paths: p, err: err}
		}); err != nil {
			t.Errorf("OpenFiles: %v", err)
		}
		if err := d.SaveFile(platform.FileDialogOptions{Name: "notes.txt"}, func(p string, err error) { got <- result{path: p, err: err} }); err != nil {
			t.Errorf("SaveFile: %v", err)
		}
		if err := d.PickFolder(platform.FileDialogOptions{}, func(p string, err error) { got <- result{path: p, err: err} }); err != nil {
			t.Errorf("PickFolder: %v", err)
		}
		if err := d.MessageBox(platform.MessageBoxOptions{Buttons: platform.ButtonsYesNo}, func(a platform.MessageAnswer, err error) {
			got <- result{answer: a, err: err}
		}); err != nil {
			t.Errorf("MessageBox: %v", err)
		}
	})
	want := []result{{paths: []string{"/a.txt", "/b.txt"}}, {path: "/notes.txt"}, {}, {answer: platform.AnswerYes}}
	for i, w := range want {
		select {
		case r := <-got:
			if !slices.Equal(r.paths, w.paths) || r.path != w.path || r.answer != w.answer || r.err != nil {
				t.Errorf("answer %d: %+v, want %+v", i, r, w)
			}
		case <-time.After(waitTimeout):
			t.Fatalf("dialog answer %d never came", i)
		}
	}
	if dl := p.hb.Dialogs(); len(dl) != 4 || dl[0].File.Parent != hw || !dl[0].File.Multiple || dl[0].File.Filters[0].Patterns[0] != "*.txt" {
		t.Errorf("dialogs %+v", dl)
	}

	p.hb.SetUnsupported(headless.ServiceMessageBox, headless.ServiceTray)
	a.ui(func() {
		err := d.MessageBox(platform.MessageBoxOptions{}, func(platform.MessageAnswer, error) { t.Errorf("done called after an error") })
		if !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("MessageBox error %v, want ErrUnsupported", err)
		}
		if _, err := d.NewTray(platform.TrayOptions{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 8, 8))}}); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("NewTray error %v, want ErrUnsupported", err)
		}
		if err := d.UpdateTray(77, platform.TrayOptions{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 8, 8))}}); err == nil {
			t.Errorf("UpdateTray of an unknown tray succeeded")
		}
	})
}

// TestTraysAcrossSwap checks that a reload takes over the tray icons of
// the previous build (same IDs, no duplicate, its options at the
// promotion), that the old build's cleanup does not remove them, and that
// a failed reload's own trays go away.
func TestTraysAcrossSwap(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	newTray := func(a *app, tip string) platform.TrayID {
		t.Helper()
		var id platform.TrayID
		a.ui(func() {
			var err error
			if id, err = a.b.Desktop().NewTray(platform.TrayOptions{Icon: icon, Tooltip: tip}); err != nil {
				t.Errorf("NewTray: %v", err)
			}
		})
		return id
	}
	tooltips := func() []string {
		var out []string
		for _, r := range p.hb.Trays() {
			out = append(out, r.Options.Tooltip)
		}
		return out
	}

	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "trays"})
	a.configured()
	id1 := newTray(a, "one")
	id2 := newTray(a, "two")

	b := p.dial("b").start()
	b.newWindow(platform.WindowOptions{Title: "trays"})
	cb := p.child(1)
	if got := newTray(b, "one v2"); got != id1 {
		t.Errorf("the reload's first tray is %d, want the previous build's %d", got, id1)
	}
	if got := tooltips(); !slices.Equal(got, []string{"one", "two"}) {
		t.Errorf("trays before the promotion %v", got)
	}
	p.promote(cb)
	p.ui(func() {})
	if got := tooltips(); !slices.Equal(got, []string{"one v2"}) {
		t.Errorf("trays after the promotion %v, want [one v2] (the tray the new build did not add is removed)", got)
	}
	// The old build's cleanup at its quit does not remove B's tray.
	a.ui(func() {
		if err := a.b.Desktop().RemoveTray(id1); err != nil {
			t.Errorf("RemoveTray from the replaced build: %v", err)
		}
	})
	p.ui(func() {})
	if got := tooltips(); !slices.Equal(got, []string{"one v2"}) {
		t.Errorf("the replaced build removed the new build's tray: %v", got)
	}
	_ = id2

	// A reload that fails: its new tray goes, the shown build's stays.
	c := p.dial("c").start()
	c.newWindow(platform.WindowOptions{Title: "trays"})
	cc := p.child(2)
	newTray(c, "one v3")
	newTray(c, "extra")
	if got := tooltips(); !slices.Equal(got, []string{"one v2", "extra"}) {
		t.Errorf("trays during the failed reload %v", got)
	}
	cc.Close()
	waitChan(t, cc.Done(), "C disconnected")
	p.ui(func() {})
	if got := tooltips(); !slices.Equal(got, []string{"one v2"}) {
		t.Errorf("trays after the failed reload %v, want [one v2]", got)
	}
}

// TestNotificationsAcrossSwap checks that a reload that notifies what the
// previous build notified gets its notification instead of a new one.
func TestNotificationsAcrossSwap(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	notify := func(a *app, title string) platform.NotificationID {
		t.Helper()
		var id platform.NotificationID
		a.ui(func() {
			var err error
			if id, err = a.b.Desktop().Notify(platform.Notification{Title: title, Body: "b"}); err != nil {
				t.Errorf("Notify: %v", err)
			}
		})
		return id
	}
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "notes"})
	a.configured()
	n1 := notify(a, "started")
	b := p.dial("b").start()
	b.newWindow(platform.WindowOptions{Title: "notes"})
	if got := notify(b, "started"); got != n1 {
		t.Errorf("the reload's notification is %d, want the previous build's %d", got, n1)
	}
	if got := notify(b, "other"); got == n1 {
		t.Errorf("a different notification reused the previous build's")
	}
	if n := len(p.hb.Notifications()); n != 2 {
		t.Errorf("%d notifications shown, want 2", n)
	}
}
