package remote

import (
	"errors"
	"image"
	"image/color"
	"io/fs"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Tests of protocol 5 (phase 13, ADR-214): every phase-13 piece between a
// child and a host whose real backend is headless, which fakes drags,
// menus, shortcuts, launches, secrets, associations and the clipboard.

func TestSettingsV2Forwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "settings"})
	a.configured()
	s := platform.SystemSettings{DarkMode: true, DoubleClickInterval: 300 * time.Millisecond,
		AccentColor: color.NRGBA{R: 53, G: 132, B: 228, A: 255}, HasAccentColor: true, HighContrast: true, TextScale: 1.5, TitleBarButtons: "appmenu:minimize,close",
		UIFontFamily: "Adwaita Sans", MonoFontFamily: "Adwaita Mono"}
	p.hb.SetSettings(s)
	got := a.waitEvent("settings", isType[platform.SettingsEvent]).(platform.SettingsEvent)
	if got.Settings != s {
		t.Errorf("SettingsEvent %+v, want %+v", got.Settings, s)
	}
	if b := p.dial("b"); b.b.Settings() != s {
		t.Errorf("a new child's welcome settings %+v, want %+v", b.b.Settings(), s)
	} else {
		b.b.Close()
	}
}

func TestClipboardData(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "clip"})
	a.configured()
	cb := a.b.Clipboard()

	data := platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<b>x</b>")}, {Type: platform.MIMEText, Bytes: []byte("x")}}
	a.ui(func() {
		if err := cb.Write(data); err != nil {
			t.Errorf("Write: %v", err)
		}
	})
	if got := p.hb.ClipboardData(); !reflect.DeepEqual(got, data) {
		t.Errorf("the host's clipboard holds %+v, want %+v", got, data)
	}

	other := platform.Data{{Type: "application/x-tela", Bytes: []byte{}}, {Type: platform.MIMEText, Bytes: []byte("other")}}
	p.hb.SetClipboardData(other)
	type answer struct {
		types []string
		data  []byte
		err   error
	}
	got := make(chan answer, 3)
	inCall := false
	a.ui(func() {
		inCall = true
		defer func() { inCall = false }()
		if err := cb.ReadTypes(func(types []string, err error) {
			if inCall {
				t.Errorf("ReadTypes called back synchronously")
			}
			got <- answer{types: types, err: err}
		}); err != nil {
			t.Errorf("ReadTypes: %v", err)
		}
		for _, mime := range []string{"application/x-tela", platform.MIMEPNG} {
			if err := cb.Read(mime, func(p []byte, err error) { got <- answer{data: p, err: err} }); err != nil {
				t.Errorf("Read: %v", err)
			}
		}
	})
	want := []answer{{types: []string{"application/x-tela", platform.MIMEText}}, {data: []byte{}}, {}}
	for i, w := range want {
		select {
		case r := <-got:
			if !slices.Equal(r.types, w.types) || (r.data == nil) != (w.data == nil) || string(r.data) != string(w.data) || r.err != nil {
				t.Errorf("answer %d: %+v, want %+v", i, r, w)
			}
		case <-time.After(waitTimeout):
			t.Fatalf("clipboard answer %d never came", i)
		}
	}

	p.hb.SetUnsupported(headless.ServiceClipboard)
	a.ui(func() {
		if err := cb.Write(data); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Write = %v, want ErrUnsupported", err)
		}
		if err := cb.ReadTypes(func([]string, error) { t.Errorf("done called after an error") }); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("ReadTypes = %v, want ErrUnsupported", err)
		}
		if err := cb.Read(platform.MIMEText, func([]byte, error) { t.Errorf("done called after an error") }); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Read = %v, want ErrUnsupported", err)
		}
	})
}

// accepter answers every DragEnter and DragOver of its windows with
// effect and types, as core does.
func accepter(effect platform.DropEffect, types ...string) func(a *app, w platform.Window, e platform.Event) {
	return func(a *app, w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && de.Action != platform.DragLeave {
			w.SetDropEffect(effect, types...)
		}
	}
}

func dropAnswer(hw *headless.Window, effect platform.DropEffect, types ...string) func() bool {
	return func() bool {
		e, ts := hw.DropAnswer()
		return e == effect && slices.Equal(ts, types)
	}
}

func isDrag(action platform.DragAction) func(platform.Event) bool {
	return func(e platform.Event) bool {
		de, ok := e.(platform.DragEvent)
		return ok && de.Action == action
	}
}

// TestDropTarget checks the target side: the real window's drag session
// reaches the child, the child's answer reaches the real window (sticky,
// one round trip later), and the drop carries the data the host's backend
// read for the answered types.
func TestDropTarget(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a")
	var refuse atomic.Bool
	accept := accepter(platform.DropCopy, platform.MIMEText)
	a.react = func(a *app, w platform.Window, e platform.Event) {
		if !refuse.Load() {
			accept(a, w, e)
		}
	}
	a.start()
	a.newWindow(platform.WindowOptions{Title: "drop", Size: platform.Size{W: 200, H: 100}})
	a.configured()
	hw := p.hostWindow("drop")

	data := platform.Data{{Type: platform.MIMEURIList, Bytes: []byte("file:///tmp/a\r\n")}, {Type: platform.MIMEText, Bytes: []byte("a")}}
	p.hb.StartExternalDrag(data, platform.DropCopy|platform.DropMove)
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 10, Y: 20}})
	enter := a.waitEvent("DragEnter", isDrag(platform.DragEnter)).(platform.DragEvent)
	if !slices.Equal(enter.Types, data.Types()) || enter.Allowed != platform.DropCopy|platform.DropMove || enter.Pos != (platform.Point{X: 10, Y: 20}) || enter.FromApp {
		t.Errorf("DragEnter %+v", enter)
	}
	waitFor(t, "the child's answer on the real window", dropAnswer(hw, platform.DropCopy, platform.MIMEText))
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 30, Y: 40}})
	a.waitEvent("DragOver", isDrag(platform.DragOver))
	hw.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 30, Y: 40}})
	drop := a.waitEvent("DragDrop", isDrag(platform.DragDrop)).(platform.DragEvent)
	if want := data.Select([]string{platform.MIMEText}); !reflect.DeepEqual(drop.Data, want) {
		t.Errorf("drop data %+v, want %+v", drop.Data, want)
	}
	waitFor(t, "the drop's outcome", func() bool { return slices.Equal(hw.Drops(), []platform.DropEffect{platform.DropCopy}) })

	// A refusing child: a release before (or without) an accepting answer
	// is a DragLeave.
	refuse.Store(true)
	p.hb.StartExternalDrag(data, platform.DropCopy)
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 5, Y: 5}})
	a.waitEvent("second DragEnter", isDrag(platform.DragEnter))
	hw.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 5, Y: 5}})
	a.waitEvent("DragLeave of the refused drop", isDrag(platform.DragLeave))
	if _, ok := p.hb.Drag(); ok {
		t.Errorf("the refused drag is still in progress")
	}
}

// TestDropAcrossSwap checks that a child promoted while a drag is over the
// window joins the session with a DragEnter, and that its answer replaces
// the old child's.
func TestDropAcrossSwap(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a")
	a.react = accepter(platform.DropCopy)
	a.start()
	wa := a.newWindow(platform.WindowOptions{Title: "swap", Size: platform.Size{W: 100, H: 80}})
	a.configured()
	hw := p.hostWindow("swap")
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	p.hb.StartExternalDrag(platform.TextData("x"), platform.DropCopy|platform.DropMove)
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 1, Y: 1}})
	waitFor(t, "A's answer", dropAnswer(hw, platform.DropCopy))

	p.ui(p.host.Freeze)
	b := p.dial("b")
	b.react = accepter(platform.DropMove)
	b.start()
	wb := b.newWindow(platform.WindowOptions{Title: "swap"})
	cb := p.child(1)
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	b.presentNow(wb, 2, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	p.promote(cb)
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 2, Y: 2}})
	b.waitEvent("B's DragEnter", isDrag(platform.DragEnter))
	b.waitEvent("B's DragOver", isDrag(platform.DragOver))
	waitFor(t, "B's answer", dropAnswer(hw, platform.DropMove))
	hw.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 2, Y: 2}})
	b.waitEvent("B's drop", isDrag(platform.DragDrop))
	for _, e := range a.take() {
		if de, ok := e.e.(platform.DragEvent); ok && de.Action != platform.DragEnter {
			t.Errorf("the replaced child got %v", de.Action)
		}
	}
}

// TestDragSource checks the source side: StartDrag from the child's
// handler of a press starts the drag on the real window under that press,
// and its outcome comes back as DragDoneEvent.
func TestDragSource(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Pix[0] = 200
	data := platform.Data{{Type: platform.MIMEText, Bytes: []byte("dragged")}}
	started := make(chan error, 1)
	a := p.dial("a")
	a.react = func(a *app, w platform.Window, e platform.Event) {
		if pe, ok := e.(platform.PointerEvent); ok && pe.Action == platform.PointerDown {
			started <- w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy | platform.DropMove, Image: img, Hotspot: image.Pt(2, 1)})
			return
		}
		accepter(platform.DropMove)(a, w, e)
	}
	a.start()
	w := a.newWindow(platform.WindowOptions{Title: "source", Size: platform.Size{W: 100, H: 80}})
	a.configured()
	hw := p.hostWindow("source")

	a.ui(func() {
		if err := w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
			t.Errorf("StartDrag without a press = %v, want ErrNoGrab", err)
		}
	})
	hw.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 5, Y: 6}})
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("StartDrag under the press: %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("the press never reached the child")
	}
	info, ok := p.hb.Drag()
	if !ok || info.Source != hw || !reflect.DeepEqual(info.Data, data) || info.Allowed != platform.DropCopy|platform.DropMove ||
		info.Image == nil || !bytes2(info.Image.Pix, img.Pix) || info.Hotspot != image.Pt(2, 1) {
		t.Fatalf("the real drag %+v (in progress %v)", info, ok)
	}
	a.waitEvent("PointerCancel", func(e platform.Event) bool {
		pe, ok := e.(platform.PointerEvent)
		return ok && pe.Action == platform.PointerCancel
	})
	enter := a.waitEvent("DragEnter of its own drag", isDrag(platform.DragEnter)).(platform.DragEvent)
	if !enter.FromApp {
		t.Errorf("the app's own drag entered with FromApp false")
	}
	waitFor(t, "the answer", dropAnswer(hw, platform.DropMove))
	hw.Inject(platform.PointerEvent{Action: platform.PointerUp, Button: platform.ButtonPrimary, Pos: platform.Point{X: 50, Y: 40}})
	a.waitEvent("its own drop", isDrag(platform.DragDrop))
	done := a.waitEvent("DragDoneEvent", isType[platform.DragDoneEvent]).(platform.DragDoneEvent)
	if done.Effect != platform.DropMove {
		t.Errorf("DragDoneEvent %+v, want move", done)
	}
}

func bytes2(a, b []byte) bool { return slices.Equal(a, b) }

func TestMenus(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	p.hb.SetDialogResponder(func(d headless.Dialog) headless.DialogResponse {
		return headless.DialogResponse{Item: 3}
	})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "menus"})
	a.configured()
	hw := p.hostWindow("menus")
	menu := []platform.MenuItem{{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "Open", Accelerator: "Ctrl+O"}, {ID: 2, Label: "Quit"}}}}
	a.ui(func() {
		if err := w.SetMenu(menu); err != nil {
			t.Errorf("SetMenu: %v", err)
		}
	})
	if got := hw.Menu(); !reflect.DeepEqual(got, menu) {
		t.Errorf("the real window's menu %+v, want %+v", got, menu)
	}
	hw.Inject(platform.MenuEvent{Item: 1})
	if got := a.waitEvent("MenuEvent", isType[platform.MenuEvent]); got != (platform.MenuEvent{Item: 1}) {
		t.Errorf("MenuEvent %+v", got)
	}
	chosen := make(chan platform.MenuItemID, 1)
	a.ui(func() {
		popup := []platform.MenuItem{{ID: 3, Label: "Copy"}, {ID: 4, Label: "Paste"}}
		if err := w.PopupMenu(platform.Point{X: 4, Y: 5}, popup, func(item platform.MenuItemID, err error) {
			if err != nil {
				t.Errorf("PopupMenu answer: %v", err)
			}
			chosen <- item
		}); err != nil {
			t.Errorf("PopupMenu: %v", err)
		}
	})
	select {
	case item := <-chosen:
		if item != 3 {
			t.Errorf("popup menu choice %d, want 3", item)
		}
	case <-time.After(waitTimeout):
		t.Fatal("the popup menu never answered")
	}
	if dl := p.hb.Dialogs(); len(dl) != 1 || dl[0].Popup.Window != hw || dl[0].Popup.At != (platform.Point{X: 4, Y: 5}) {
		t.Errorf("popup dialogs %+v", dl)
	}

	// A frameless window has no menu bar (Win32's rule, which headless
	// follows).
	f := a.newWindow(platform.WindowOptions{Title: "frameless", Frameless: true})
	a.ui(func() {
		if err := f.SetMenu(menu); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("SetMenu on a frameless window = %v, want ErrUnsupported", err)
		}
	})

	// A reload's menu bar is set at its promotion; a starting build learns
	// whether the window has menus without changing it.
	changes := hw.MenuChanges()
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "menus"})
	fb := b.newWindow(platform.WindowOptions{Title: "frameless", Frameless: true})
	cb := p.child(1)
	menu2 := []platform.MenuItem{{ID: 9, Label: "Help"}}
	b.ui(func() {
		if err := wb.SetMenu(menu2); err != nil {
			t.Errorf("a starting build's SetMenu: %v", err)
		}
		if err := fb.SetMenu(menu2); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("a starting build's SetMenu on a frameless window = %v, want ErrUnsupported", err)
		}
		if err := wb.PopupMenu(platform.Point{}, menu2, func(platform.MenuItemID, error) { t.Errorf("done called after an error") }); err == nil {
			t.Errorf("a starting build's PopupMenu succeeded")
		}
	})
	if got := hw.Menu(); !reflect.DeepEqual(got, menu) || hw.MenuChanges() != changes {
		t.Errorf("a starting build changed the shown menu: %+v (%d changes, was %d)", got, hw.MenuChanges(), changes)
	}
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	b.presentNow(wb, 2, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	p.promote(cb)
	if got := hw.Menu(); !reflect.DeepEqual(got, menu2) {
		t.Errorf("the menu after the promotion %+v, want %+v", got, menu2)
	}
	hw.Inject(platform.MenuEvent{Item: 9})
	b.waitEvent("B's MenuEvent", isType[platform.MenuEvent])

	// A build without a menu bar removes it.
	p.ui(p.host.Freeze)
	c := p.dial("c").start()
	wc := c.newWindow(platform.WindowOptions{Title: "menus"})
	cc := p.child(2)
	c.waitEvent("C's frame", isType[platform.FrameEvent])
	c.presentNow(wc, 3, nil)
	waitChan(t, cc.FirstFrame(), "C's first frame")
	p.promote(cc)
	if got := hw.Menu(); got != nil {
		t.Errorf("the menu after a build without one %+v, want none", got)
	}
}

func TestActivateWithToken(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "token"})
	a.configured()
	hw := p.hostWindow("token")
	a.ui(func() { w.ActivateWithToken("tok_TIME42") })
	waitFor(t, "the token", func() bool { return slices.Equal(hw.ActivationTokens(), []string{"tok_TIME42"}) })
}

func isStatus(id string, status platform.ShortcutStatus) func(platform.Event) bool {
	return func(e platform.Event) bool {
		se, ok := e.(platform.ShortcutStatusEvent)
		return ok && se.ID == id && se.Status == status
	}
}

// TestShortcutsAcrossReloads checks that shortcuts are held by the host:
// a reload that registers the same shortcut takes it over without a new
// registration and gets its status at once; presses go to the active
// child; a changed shortcut is registered at the promotion; one the new
// build dropped is unregistered then; and a crash keeps them for the next
// build.
func TestShortcutsAcrossReloads(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	var registered atomic.Int32
	p.hb.SetShortcutResponder(func(s platform.Shortcut) platform.ShortcutStatusEvent {
		registered.Add(1)
		return platform.ShortcutStatusEvent{Status: platform.ShortcutBound, Trigger: "bound " + s.Accelerator.String()}
	})
	show := platform.Shortcut{ID: "show", Accelerator: platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt, Key: "k"}, Description: "Show"}
	extra := platform.Shortcut{ID: "extra", Accelerator: platform.Accelerator{Mods: platform.ModSuper, Key: "e"}, Description: "Extra"}
	register := func(a *app, s platform.Shortcut) {
		t.Helper()
		a.ui(func() {
			if err := a.b.Desktop().RegisterShortcut(s); err != nil {
				t.Errorf("RegisterShortcut(%q): %v", s.ID, err)
			}
		})
	}
	ids := func() []string {
		var out []string
		for _, r := range p.hb.Shortcuts() {
			out = append(out, r.Shortcut.ID+"="+r.Shortcut.Accelerator.String())
		}
		slices.Sort(out)
		return out
	}
	press := platform.ShortcutEvent{ID: "show", Action: platform.ShortcutPressed}

	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "keys"})
	a.configured()
	hw := p.hostWindow("keys")
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	register(a, show)
	st := a.waitEvent("A's status", isStatus("show", platform.ShortcutBound)).(platform.ShortcutStatusEvent)
	if st.Trigger != "bound Ctrl+Alt+K" {
		t.Errorf("status %+v", st)
	}
	p.hb.InjectApp(press)
	if got := a.waitEvent("A's press", isType[platform.ShortcutEvent]).(platform.ShortcutEvent); got.ID != "show" || got.Action != platform.ShortcutPressed {
		t.Errorf("press %+v", got)
	}

	// A reload that registers the same shortcut and a new one.
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "keys"})
	cb := p.child(1)
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	register(b, show)
	b.waitEvent("B's status of the held shortcut", isStatus("show", platform.ShortcutBound))
	register(b, extra)
	b.waitEvent("B's status of its new shortcut", isStatus("extra", platform.ShortcutBound))
	if n := registered.Load(); n != 2 {
		t.Errorf("%d registrations after the reload's, want 2 (show once, extra)", n)
	}
	p.hb.InjectApp(press) // held while input is frozen
	b.presentNow(wb, 2, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	p.promote(cb)
	b.waitEvent("B's press, queued during the swap", isType[platform.ShortcutEvent])
	// The replaced build's cleanup does not unregister B's shortcut.
	a.ui(func() {
		if err := a.b.Desktop().UnregisterShortcut("show"); err != nil {
			t.Errorf("UnregisterShortcut from the replaced build: %v", err)
		}
	})
	p.roundTrip(a, p.child(0))
	if got := ids(); !slices.Equal(got, []string{"extra=Super+E", "show=Ctrl+Alt+K"}) {
		t.Errorf("shortcuts after the promotion %v", got)
	}
	for _, e := range a.take() {
		if isType[platform.ShortcutEvent](e.e) {
			t.Errorf("the replaced build got a press")
		}
	}

	// A reload that changes show's trigger and drops extra.
	show2 := show
	show2.Accelerator.Key = "j"
	p.ui(p.host.Freeze)
	c := p.dial("c").start()
	wc := c.newWindow(platform.WindowOptions{Title: "keys"})
	cc := p.child(2)
	c.waitEvent("C's frame", isType[platform.FrameEvent])
	register(c, show2)
	if n := registered.Load(); n != 2 {
		t.Errorf("a starting build registered its changed shortcut before its promotion (%d registrations)", n)
	}
	c.presentNow(wc, 3, nil)
	waitChan(t, cc.FirstFrame(), "C's first frame")
	p.promote(cc)
	st = c.waitEvent("C's status", isStatus("show", platform.ShortcutBound)).(platform.ShortcutStatusEvent)
	if st.Trigger != "bound Ctrl+Alt+J" {
		t.Errorf("C's status %+v", st)
	}
	if got := ids(); !slices.Equal(got, []string{"show=Ctrl+Alt+J"}) {
		t.Errorf("shortcuts after the second promotion %v", got)
	}

	// A crash keeps them for the next build, which takes them over.
	cc.Close()
	waitChan(t, cc.Done(), "C disconnected")
	p.ui(func() {})
	if got := ids(); !slices.Equal(got, []string{"show=Ctrl+Alt+J"}) {
		t.Errorf("shortcuts after the crash %v", got)
	}
	d := p.dial("d").start()
	d.newWindow(platform.WindowOptions{Title: "keys"})
	register(d, show2)
	d.waitEvent("D's status", isStatus("show", platform.ShortcutBound))
	if n := registered.Load(); n != 3 {
		t.Errorf("%d registrations, want 3 (the build after the crash takes the shortcut over)", n)
	}
	p.hb.InjectApp(press)
	d.waitEvent("D's press", isType[platform.ShortcutEvent])
}

// TestSingleInstance checks that the host claims the app ID for its
// children and keeps it across reloads, that launches of other processes
// reach the active child, and that a second tela dev of the app is a
// second instance.
func TestSingleInstance(t *testing.T) {
	appID := "dev.telaui.RemoteTest" + t.Name()
	p := newPair(t, HostOptions{}, headless.Options{})
	claim := func(a *app, launch platform.Launch) bool {
		t.Helper()
		primary, err := a.b.Desktop().ClaimInstance(appID, launch) // before any window, before Run
		if err != nil {
			t.Fatalf("ClaimInstance: %v", err)
		}
		return primary
	}
	a := p.dial("a")
	if !claim(a, platform.Launch{Args: []string{"first"}}) {
		t.Fatalf("the first build is not the primary instance")
	}
	a.start()
	a.newWindow(platform.WindowOptions{Title: "inst"})
	a.configured()
	if got := p.hb.Instances(); !slices.Equal(got, []string{appID}) {
		t.Errorf("the host's claims %v", got)
	}

	b := p.dial("b")
	if !claim(b, platform.Launch{}) {
		t.Errorf("a reload is not the primary instance")
	}
	b.b.Close()

	// Another process of the app.
	other := headless.New(headless.Options{})
	t.Cleanup(other.ReleaseInstances)
	launch := platform.Launch{Args: []string{"notes:open?id=3"}, Dir: "/tmp", Token: "tok"}
	if primary, err := other.Desktop().ClaimInstance(appID, launch); primary || err != nil {
		t.Fatalf("another process's claim: %v, %v", primary, err)
	}
	if got := a.waitEvent("LaunchEvent", isType[platform.LaunchEvent]).(platform.LaunchEvent); !reflect.DeepEqual(got.Launch, launch) {
		t.Errorf("LaunchEvent %+v, want %+v", got.Launch, launch)
	}

	// A second tela dev of the same app.
	var mu sync.Mutex
	var seconds []string
	p2 := newPair(t, HostOptions{OnSecondInstance: func(_ *Child, id string) {
		mu.Lock()
		seconds = append(seconds, id)
		mu.Unlock()
	}}, headless.Options{})
	a2 := p2.dial("a2")
	if claim(a2, platform.Launch{Args: []string{"second"}}) {
		t.Errorf("a second tela dev of the app is the primary instance")
	}
	a2.b.Close()
	if got := a.waitEvent("the second tela dev's launch", isType[platform.LaunchEvent]).(platform.LaunchEvent); !slices.Equal(got.Launch.Args, []string{"second"}) {
		t.Errorf("forwarded launch %+v", got.Launch)
	}
	p2.ui(func() {})
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(seconds, []string{appID}) {
		t.Errorf("OnSecondInstance calls %v", seconds)
	}
}

// TestSecretsAndAssociations checks that both are served by the host's
// backend, from any goroutine of the child, with their sentinel errors.
func TestSecretsAndAssociations(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	d := a.b.Desktop() // kept and used off the UI thread
	const id = "com.example.Notes"

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := string(rune('a' + i))
			if err := d.SetSecret(id, name, []byte{byte(i)}); err != nil {
				t.Errorf("SetSecret: %v", err)
			}
		}()
	}
	wg.Wait()
	for i := range 4 {
		name := string(rune('a' + i))
		if v, ok := p.hb.StoredSecret(id, name); !ok || !slices.Equal(v, []byte{byte(i)}) {
			t.Errorf("stored secret %s = %v, %v", name, v, ok)
		}
		if v, err := d.Secret(id, name); err != nil || !slices.Equal(v, []byte{byte(i)}) {
			t.Errorf("Secret(%s) = %v, %v", name, v, err)
		}
	}
	if err := d.SetSecret(id, "empty", nil); err != nil {
		t.Errorf("SetSecret of an empty secret: %v", err)
	}
	if v, err := d.Secret(id, "empty"); err != nil || v == nil || len(v) != 0 {
		t.Errorf("Secret of an empty secret = %v, %v; want an empty slice", v, err)
	}
	if err := d.DeleteSecret(id, "a"); err != nil {
		t.Errorf("DeleteSecret: %v", err)
	}
	if _, err := d.Secret(id, "a"); !errors.Is(err, platform.ErrSecretNotFound) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Secret of a deleted secret = %v, want ErrSecretNotFound", err)
	}
	locked := errors.New("the keyring is locked")
	p.hb.SetSecretError(locked)
	if _, err := d.Secret(id, "b"); err == nil || err.Error() != locked.Error() {
		t.Errorf("Secret with a locked keyring = %v", err)
	}
	p.hb.SetSecretError(nil)

	app := platform.HandlerApp{ID: id, Name: "Notes", Exec: "/usr/bin/notes", Args: []string{"--open"}}
	scheme := platform.Association{Scheme: "notes"}
	as := d.Associations()
	if changed, err := as.Register(app, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register = %v, %v", changed, err)
	}
	if changed, err := as.Register(app, scheme, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("Register again = %v, %v; want no change", changed, err)
	}
	if st, err := as.Status(app, scheme); !st.Registered || err != nil {
		t.Errorf("Status = %+v, %v", st, err)
	}
	if regs := p.hb.Associations().Registrations(); len(regs) != 1 || regs[0].Association != "notes:" || !reflect.DeepEqual(regs[0].App, app) {
		t.Errorf("registrations %+v", regs)
	}
	if res, err := as.MakeDefault(app, scheme); res != platform.DefaultSet || err != nil {
		t.Errorf("MakeDefault = %v, %v", res, err)
	}
	file := platform.Association{Extension: ".note", MIMEType: "application/x-note", Description: "Note"}
	p.hb.Associations().SetConflict(file, "other.desktop")
	if _, err := as.Register(app, file, platform.RegisterOptions{}); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("Register of a taken type = %v, want ErrAssociationTaken", err)
	}
	// The role crosses the wire: the host registers a declaration.
	p.hb.Associations().SetConflict(file, "")
	declared := file
	declared.Role = platform.HandlerNone
	if changed, err := as.Register(app, declared, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register with HandlerNone = %v, %v", changed, err)
	}
	if regs := p.hb.Associations().Registrations(); len(regs) != 2 || regs[0].Association != ".note" || regs[0].Role != platform.HandlerNone {
		t.Errorf("registrations after HandlerNone %+v", regs)
	}
	if st, err := as.Status(app, file); st.Registered || st.Role != platform.HandlerNone || err != nil {
		t.Errorf("Status as a handler of a declared type = %+v, %v; want not registered", st, err)
	}
	if st, err := as.Status(app, declared); !st.Registered || err != nil {
		t.Errorf("Status of the declaration = %+v, %v", st, err)
	}
	if changed, err := as.Unregister(app, scheme); !changed || err != nil {
		t.Errorf("Unregister = %v, %v", changed, err)
	}

	p.hb.SetUnsupported(headless.ServiceSecrets, headless.ServiceAssociations)
	if err := d.SetSecret(id, "x", []byte("x")); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetSecret without a keyring = %v, want ErrUnsupported", err)
	}
	if _, err := as.Status(app, scheme); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Status without associations = %v, want ErrUnsupported", err)
	}
	if res, err := as.MakeDefault(app, scheme); !errors.Is(err, platform.ErrUnsupported) || res != platform.DefaultNone {
		t.Errorf("MakeDefault without associations = %v, %v; want DefaultNone and ErrUnsupported", res, err)
	}
}
