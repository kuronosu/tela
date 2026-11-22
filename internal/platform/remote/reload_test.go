package remote

import (
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Tests of the desktop services across reloads that protocol 19 fixes
// (ADR-310): the drop's final effect, refused shortcuts, windows opened
// after a promotion, dialogs opened at startup, reloads under a window
// that gets no frames, and the client-area backdrop.

// swapTo freezes the host, dials a new child tagged tag, runs setup on it
// (its windows, its startup calls), waits for its first FrameEvent,
// presents its main window and promotes it. It returns the app, its Child
// and its main window.
func (p *pair) swapTo(tag string, n int, setup func(a *app) platform.Window) (*app, *Child, platform.Window) {
	p.t.Helper()
	p.ui(p.host.Freeze)
	a := p.dial(tag).start()
	w := setup(a)
	c := p.child(n)
	a.waitEvent(tag+"'s frame", isType[platform.FrameEvent])
	a.presentNow(w, byte(n+1), nil)
	waitChan(p.t, c.FirstFrame(), tag+"'s first frame")
	p.promote(c)
	return a, c, w
}

// TestDropEffectSetInOnDrop checks that the effect a child sets while it
// handles the drop (OnDrop) is the drop's outcome on the real window and
// the source's DragDoneEvent, not the answer it gave before the drop.
func TestDropEffectSetInOnDrop(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	var final atomic.Int32 // the effect OnDrop sets
	final.Store(int32(platform.DropMove))
	data := platform.TextData("x")
	started := make(chan error, 1)
	a := p.dial("a")
	a.react = func(a *app, w platform.Window, e platform.Event) {
		if pe, ok := e.(platform.PointerEvent); ok && pe.Action == platform.PointerDown {
			started <- w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy | platform.DropMove})
			return
		}
		de, ok := e.(platform.DragEvent)
		if !ok {
			return
		}
		switch de.Action {
		case platform.DragEnter, platform.DragOver:
			w.SetDropEffect(platform.DropCopy)
		case platform.DragDrop:
			w.SetDropEffect(platform.DropEffect(final.Load()))
		}
	}
	a.start()
	a.newWindow(platform.WindowOptions{Title: "drop", Size: platform.Size{W: 200, H: 100}})
	a.configured()
	hw := p.hostWindow("drop")

	drop := func(want platform.DropEffect, n int) {
		t.Helper()
		p.hb.StartExternalDrag(data, platform.DropCopy|platform.DropMove)
		hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 10, Y: 20}})
		waitFor(t, "the answer before the drop", dropAnswer(hw, platform.DropCopy))
		hw.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 10, Y: 20}})
		waitFor(t, "the drop's outcome", func() bool { return len(hw.Drops()) == n })
		if got := hw.Drops()[n-1]; got != want {
			t.Errorf("drop %d's outcome %v, want %v (the effect set in OnDrop)", n, got, want)
		}
	}
	drop(platform.DropMove, 1)
	final.Store(int32(platform.DropNone)) // OnDrop refuses the data
	drop(platform.DropNone, 2)

	// The app's own drag: the source's DragDoneEvent has the final effect.
	final.Store(int32(platform.DropMove))
	hw.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 5, Y: 6}})
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("StartDrag: %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("the press never reached the child")
	}
	waitFor(t, "the answer to its own drag", dropAnswer(hw, platform.DropCopy))
	hw.Inject(platform.PointerEvent{Action: platform.PointerUp, Button: platform.ButtonPrimary, Pos: platform.Point{X: 50, Y: 40}})
	done := a.waitEvent("DragDoneEvent", isType[platform.DragDoneEvent]).(platform.DragDoneEvent)
	if done.Effect != platform.DropMove {
		t.Errorf("DragDoneEvent %+v, want the move set in OnDrop", done)
	}
}

// TestRefusedShortcutRegisteredAgain checks that a shortcut whose
// registration was taken or refused is registered again by the next build
// (at its promotion), instead of handing it the old answer until tela dev
// restarts; a bound one is still taken over without a new registration.
func TestRefusedShortcutRegisteredAgain(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	var registered, next atomic.Int32
	next.Store(int32(platform.ShortcutTaken))
	p.hb.SetShortcutResponder(func(s platform.Shortcut) platform.ShortcutStatusEvent {
		registered.Add(1)
		return platform.ShortcutStatusEvent{Status: platform.ShortcutStatus(next.Load())}
	})
	show := platform.Shortcut{ID: "show", Accelerator: platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt, Key: "k"}, Description: "Show"}
	register := func(a *app) {
		t.Helper()
		a.ui(func() {
			if err := a.b.Desktop().RegisterShortcut(show); err != nil {
				t.Errorf("RegisterShortcut: %v", err)
			}
		})
	}
	// noStatus fails if a got a status before its promotion (the old
	// answer, cached).
	noStatus := func(a *app, c *Child, what string) {
		t.Helper()
		p.roundTrip(a, c)
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, e := range a.events {
			if se, ok := e.e.(platform.ShortcutStatusEvent); ok {
				t.Errorf("%s got the cached status %v before its promotion", what, se.Status)
			}
		}
	}

	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "keys"})
	a.configured()
	a.presentNow(wa, 1, nil)
	register(a)
	a.waitEvent("A's status", isStatus("show", platform.ShortcutTaken))

	// B registers it again: taken is not an answer to keep.
	next.Store(int32(platform.ShortcutRefused))
	b, cb, _ := p.swapTo("b", 1, func(b *app) platform.Window {
		w := b.newWindow(platform.WindowOptions{Title: "keys"})
		b.waitEvent("B's configure", isType[platform.ConfigureEvent])
		register(b)
		noStatus(b, p.child(1), "B")
		return w
	})
	b.waitEvent("B's status", isStatus("show", platform.ShortcutRefused))
	if n := registered.Load(); n != 2 {
		t.Errorf("%d registrations after B, want 2", n)
	}
	_ = cb

	// C again: refused is not kept either; this time it binds.
	next.Store(int32(platform.ShortcutBound))
	c, _, _ := p.swapTo("c", 2, func(c *app) platform.Window {
		w := c.newWindow(platform.WindowOptions{Title: "keys"})
		c.waitEvent("C's configure", isType[platform.ConfigureEvent])
		register(c)
		noStatus(c, p.child(2), "C")
		return w
	})
	c.waitEvent("C's status", isStatus("show", platform.ShortcutBound))
	if n := registered.Load(); n != 3 {
		t.Errorf("%d registrations after C, want 3", n)
	}

	// D takes the bound one over, with its status at once.
	d, _, _ := p.swapTo("d", 3, func(d *app) platform.Window {
		w := d.newWindow(platform.WindowOptions{Title: "keys"})
		register(d)
		// Not waitEvent, which would take the FrameEvent swapTo waits for
		// if it came first.
		waitFor(t, "D's status, at once", func() bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return slices.ContainsFunc(d.events, func(e appEvent) bool { return isStatus("show", platform.ShortcutBound)(e.e) })
		})
		return w
	})
	_ = d
	if n := registered.Load(); n != 3 {
		t.Errorf("%d registrations after D, want 3 (a bound shortcut is taken over)", n)
	}
}

// TestLateWindowKeepsRealWindow checks that a window the new build opens
// only after its promotion (from an effect) takes over the real window of
// the old build's window with its key, which waits for it, instead of
// closing at the promotion and opening anew. A window the new build
// opened anew (its creation options changed) closes at once, and a close
// request on a window waiting for a build closes it.
func TestLateWindowKeepsRealWindow(t *testing.T) {
	p := newPair(t, HostOptions{ReloadGrace: time.Minute}, headless.Options{})
	a := p.dial("a").start()
	ma := a.newWindow(platform.WindowOptions{Title: "main", Size: platform.Size{W: 100, H: 80}})
	a.configured()
	a.presentNow(ma, 1, nil)
	a.keyed(platform.WindowOptions{Title: "prefs", Parent: ma}, "w:prefs")
	a.keyed(platform.WindowOptions{Title: "gone", Parent: ma}, "w:gone")
	a.keyed(platform.WindowOptions{Title: "framed", Parent: ma}, "w:framed")
	hPrefs, hGone, hFramed := p.hostWindow("prefs"), p.hostWindow("gone"), p.hostWindow("framed")

	b, _, mb := p.swapTo("b", 1, func(b *app) platform.Window {
		w := b.newWindow(platform.WindowOptions{Title: "main 2"})
		b.keyed(platform.WindowOptions{Title: "framed 2", Parent: w, Frameless: true}, "w:framed")
		return w
	})
	waitFor(t, "the window opened anew closed", hFramed.Closed)
	p.ui(func() {})
	if hPrefs.Closed() || hGone.Closed() {
		t.Fatalf("the promotion closed a window the new build may still open")
	}
	b.keyed(platform.WindowOptions{Title: "prefs 2", Parent: mb}, "w:prefs")
	waitFor(t, "the late window on its real window", func() bool { return hPrefs.Title() == "prefs 2" })
	if hPrefs.Closed() {
		t.Errorf("the late window's real window closed")
	}
	if n := len(p.hb.Windows()); n != 4 {
		t.Errorf("%d real windows, want 4 (main, prefs, gone, framed 2)", n)
	}

	hGone.RequestClose()
	waitFor(t, "the waiting window closed by its close request", hGone.Closed)
	p.ui(func() {})
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.unhandled {
		if _, ok := e.(platform.CloseRequestedEvent); ok {
			t.Errorf("the close request of a waiting window reached OnUnhandled (tela dev would quit)")
		}
	}
}

// TestLateWindowGraceEnds checks that a window no build opened again
// closes once the grace after the promotion is over.
func TestLateWindowGraceEnds(t *testing.T) {
	p := newPair(t, HostOptions{ReloadGrace: 50 * time.Millisecond}, headless.Options{})
	a := p.dial("a").start()
	ma := a.newWindow(platform.WindowOptions{Title: "main"})
	a.configured()
	a.presentNow(ma, 1, nil)
	a.keyed(platform.WindowOptions{Title: "gone", Parent: ma}, "w:gone")
	hGone := p.hostWindow("gone")
	p.swapTo("b", 1, func(b *app) platform.Window {
		return b.newWindow(platform.WindowOptions{Title: "main"})
	})
	waitFor(t, "the window no build opened again closed", hGone.Closed)
}

// TestStartupDialogNotShownAgain checks that a dialog a build opens at its
// start is not shown again by the next builds, whose start asks for the
// same one: while it is open the new build takes it over (the answer goes
// to it), and once answered the new build gets the same answer at once.
// Another dialog, or the same one asked again, is shown.
func TestStartupDialogNotShownAgain(t *testing.T) {
	p := newPair(t, HostOptions{ReloadGrace: 50 * time.Millisecond}, headless.Options{})
	box := platform.MessageBoxOptions{Title: "Welcome", Text: "Open the last notes?", Buttons: platform.ButtonsYesNo}
	ask := func(a *app, opts platform.MessageBoxOptions) chan platform.MessageAnswer {
		t.Helper()
		ch := make(chan platform.MessageAnswer, 1)
		a.ui(func() {
			if err := a.b.Desktop().MessageBox(opts, func(ans platform.MessageAnswer, err error) {
				if err != nil {
					t.Errorf("MessageBox answer: %v", err)
				}
				ch <- ans
			}); err != nil {
				t.Errorf("MessageBox: %v", err)
			}
		})
		return ch
	}
	answer := func(ch chan platform.MessageAnswer, what string, want platform.MessageAnswer) {
		t.Helper()
		select {
		case got := <-ch:
			if got != want {
				t.Errorf("%s: answer %v, want %v", what, got, want)
			}
		case <-time.After(waitTimeout):
			t.Fatalf("%s: no answer", what)
		}
	}
	shown := func(want int) {
		t.Helper()
		p.ui(func() {})
		if n := len(p.hb.Dialogs()); n != want {
			t.Errorf("%d dialogs shown, want %d", n, want)
		}
	}

	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "notes"})
	a.configured()
	ansA := ask(a, box) // at its start
	waitFor(t, "the startup dialog", func() bool { return len(p.hb.OpenDialogs()) == 1 })
	a.presentNow(wa, 1, nil)

	// A reload while it is open: B's start takes it over.
	var ansB chan platform.MessageAnswer
	p.swapTo("b", 1, func(b *app) platform.Window {
		w := b.newWindow(platform.WindowOptions{Title: "notes"})
		ansB = ask(b, box)
		return w
	})
	shown(1)
	p.hb.AnswerDialog(p.hb.OpenDialogs()[0].ID, headless.DialogResponse{Answer: platform.AnswerYes})
	answer(ansB, "B, which took the open dialog over", platform.AnswerYes)
	select {
	case <-ansA:
		t.Errorf("the replaced build got the answer")
	default:
	}

	// The next reload's start gets the answer at once; a changed dialog,
	// and the same one asked a second time, are shown.
	var ansC, ansC2 chan platform.MessageAnswer
	other := box
	other.Text = "Open the notes of today?"
	p.swapTo("c", 2, func(c *app) platform.Window {
		w := c.newWindow(platform.WindowOptions{Title: "notes"})
		ansC = ask(c, box)
		answer(ansC, "C, at its start", platform.AnswerYes)
		shown(1)
		ask(c, other)
		shown(2)
		ansC2 = ask(c, box)
		shown(3)
		return w
	})
	_ = ansC2

	// After its start, a build's dialog is shown, even one the previous
	// build opened at its start.
	d, cd, _ := p.swapTo("d", 3, func(d *app) platform.Window {
		return d.newWindow(platform.WindowOptions{Title: "notes"})
	})
	time.Sleep(100 * time.Millisecond) // past the grace
	p.roundTrip(d, cd)
	ask(d, box)
	shown(4)
}

// TestReloadWhileSuspended checks that a reload under a window the OS
// sends no frames to while it reports it (Wayland's suspended state: a
// minimized window, a hidden workspace) gives the new build its first
// frame offscreen instead of waiting for the window: at once, not after
// the fallback of a window that gets no frames without saying why.
func TestReloadWhileSuspended(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	p.ui(func() { p.host.frameFallback = time.Hour })
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "away", Size: platform.Size{W: 80, H: 60}})
	a.configured()
	hw := p.hostWindow("away")
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	hw.Suspend(true)
	a.waitEvent("A suspended", func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State.Suspended
	})
	p.swapTo("b", 1, func(b *app) platform.Window {
		return b.newWindow(platform.WindowOptions{Title: "away"})
	})
}

// TestReloadWithoutFrames checks the same for a window that gets no frames
// without saying why (a minimized Wayland window on a compositor without
// xdg-shell's suspended state): the host gives a starting build its frame
// after a moment.
func TestReloadWithoutFrames(t *testing.T) {
	var fx *fxBackend
	p := newPairWrapped(t, HostOptions{}, headless.Options{}, func(hb *headless.Backend) platform.Backend {
		fx = newFxBackend(hb)
		return fx
	})
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "away"})
	a.configured()
	hw := p.hostWindow("away")
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	fx.window(hw).noFrames.Store(true)
	p.swapTo("b", 1, func(b *app) platform.Window {
		return b.newWindow(platform.WindowOptions{Title: "away"})
	})
}

// TestBackdropAlphaForwarded checks that a child window presents alpha
// (platform.AlphaPresenter) when its real window does, a Windows 11 or
// macOS window with a client-area backdrop, and is told again with a
// WindowStateEvent when that changes: core then renders a transparent
// canvas, and the host presents its pixels with their alpha.
func TestBackdropAlphaForwarded(t *testing.T) {
	p := newPairWrapped(t, HostOptions{}, headless.Options{}, func(hb *headless.Backend) platform.Backend {
		return newFxBackend(hb)
	})
	alpha := func(w platform.Window) bool {
		t.Helper()
		ap, ok := w.(platform.AlphaPresenter)
		if !ok {
			t.Fatalf("%T does not implement platform.AlphaPresenter", w)
		}
		return ap.PresentsAlpha()
	}
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "glass", Size: platform.Size{W: 40, H: 30}, Backdrop: platform.BackdropMica})
	a.configured()
	a.waitEvent("the state that says the window presents alpha", isType[platform.WindowStateEvent])
	if !alpha(wa) {
		t.Errorf("a window with Mica does not present alpha")
	}
	hw := p.hostWindow("glass")
	clear := image.NewRGBA(image.Rect(0, 0, 40, 30)) // transparent
	a.ui(func() {
		if err := wa.Present(clear, nil); err != nil {
			t.Errorf("Present: %v", err)
		}
	})
	waitFrame(t, hw, "the transparent frame, alpha kept", clear)

	a.ui(func() { wa.SetBackdrop(platform.BackdropNone) })
	a.waitEvent("the state after the backdrop went", isType[platform.WindowStateEvent])
	if alpha(wa) {
		t.Errorf("a window without a backdrop presents alpha")
	}
	a.ui(func() { wa.SetBackdrop(platform.BackdropAcrylic) })
	a.waitEvent("the state after Acrylic", isType[platform.WindowStateEvent])
	if !alpha(wa) {
		t.Errorf("a window with Acrylic does not present alpha")
	}

	// A starting build with another backdrop is told no alpha until its
	// promotion gives the real window its backdrop.
	var wb platform.Window
	b, _, _ := p.swapTo("b", 1, func(b *app) platform.Window {
		wb = b.newWindow(platform.WindowOptions{Title: "glass", Backdrop: platform.BackdropMicaAlt})
		b.configured()
		p.roundTrip(b, p.child(1))
		if alpha(wb) {
			t.Errorf("a starting build presents alpha with a backdrop the real window does not have yet")
		}
		return wb
	})
	b.waitEvent("B's state at its promotion", isType[platform.WindowStateEvent])
	if !alpha(wb) || hw.Backdrop() != platform.BackdropMicaAlt {
		t.Errorf("after the promotion: alpha %v, real backdrop %v", alpha(wb), hw.Backdrop())
	}
}

// fxBackend wraps the headless backend with what headless does not play:
// windows that present alpha with a client-area backdrop
// (platform.AlphaPresenter, as Win32 on Windows 11 and Cocoa), and windows
// that stop getting FrameEvents without a state that says so.
type fxBackend struct {
	*headless.Backend
	mu   sync.Mutex
	wins map[platform.Window]*fxWindow
}

type fxWindow struct {
	platform.Window
	hw       *headless.Window
	noFrames atomic.Bool
}

func (w *fxWindow) PresentsAlpha() bool { return w.hw.Backdrop().ClientBackdrop() }

func newFxBackend(hb *headless.Backend) *fxBackend {
	return &fxBackend{Backend: hb, wins: map[platform.Window]*fxWindow{}}
}

func (b *fxBackend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	if fw, ok := opts.Parent.(*fxWindow); ok {
		opts.Parent = fw.hw
	}
	w, err := b.Backend.NewWindow(opts)
	if err != nil {
		return nil, err
	}
	fw := &fxWindow{Window: w, hw: w.(*headless.Window)}
	b.mu.Lock()
	b.wins[w] = fw
	b.mu.Unlock()
	return fw, nil
}

// window returns the wrapper of a headless window.
func (b *fxBackend) window(hw *headless.Window) *fxWindow {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.wins[hw]
}

func (b *fxBackend) Run(h platform.Handler) error {
	return b.Backend.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		b.mu.Lock()
		fw := b.wins[w]
		b.mu.Unlock()
		if fw == nil {
			h.HandleEvent(w, e)
			return
		}
		if _, ok := e.(platform.FrameEvent); ok && fw.noFrames.Load() {
			return
		}
		h.HandleEvent(fw, e)
	}))
}
