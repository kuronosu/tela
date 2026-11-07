//go:build linux

package x11

import (
	"image"
	"os"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xim"
)

// TestLiveIME connects to the input method XMODIFIERS names (fcitx5's or
// ibus's XIM frontend) and creates an input context for a window with a
// text field. By default the window is never mapped, so it takes no
// focus and nothing is typed anywhere: the test checks the handshake
// (encoding, input style) and CREATE_IC. Run it with
//
//	TELA_LIVE=1 CGO_ENABLED=0 go test -run TestLiveIME -v ./internal/platform/x11/
//
// TELA_LIVE_TYPE=manual shows the window instead and waits 30 s for the
// user to click it and type (for example Japanese with fcitx5-mozc), and
// logs the events. TELA_LIVE_IME_FORWARD=1 shows the window and, once it
// has the keyboard focus and the session started, forwards a synthetic
// press and release of the A key (keycode 38) to the server for its own
// input context: the server consumes it (an active Japanese or Chinese
// engine: a preedit or a commit) or sends it back (a keyboard layout: the
// KeyEvent "a").
func TestLiveIME(t *testing.T) {
	if xim.IMName(os.Getenv("XMODIFIERS")) == "" {
		t.Skip("XMODIFIERS names no input method (@im=fcitx, @im=ibus)")
	}
	b := liveBackend(t)
	if b.ime.state == imeIdle {
		b.conn.Close()
		t.Skipf("the input method %q is not running (no owner of its selection in XIM_SERVERS)", b.ime.name)
	}
	manual := os.Getenv("TELA_LIVE_TYPE") == "manual"
	forward := os.Getenv("TELA_LIVE_IME_FORWARD") == "1"
	show := manual || forward
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela x11 ime", AppID: "dev.telaui.x11test", Size: platform.Size{W: 320, H: 120}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	state := &platform.TextInputState{CursorArea: platform.Rect{X: 20, Y: 30, W: 0, H: 18}}

	var log []string
	var icid uint16
	session, forwarded := false, false
	wait := 5 * time.Second
	if show {
		wait = 30 * time.Second
	}
	deadline := time.AfterFunc(wait, b.Quit)
	defer deadline.Stop()
	// Wake the loop now and then: the handler checks for the context.
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	go func() {
		for range tick.C {
			b.Wake()
		}
	}()
	h := handlerFunc(func(pw platform.Window, e platform.Event) {
		if w.ic != nil && w.ic.id != 0 && icid == 0 {
			icid = w.ic.id
			if !show {
				b.Quit()
			}
		}
		switch e := e.(type) {
		case platform.ConfigureEvent:
			pw.SetTextInput(state)
		case platform.FrameEvent:
			if !show {
				return
			}
			m := pw.Metrics()
			sz := platform.PhysicalSize(m.Size, m.Scale)
			fb := image.NewRGBA(image.Rect(0, 0, sz.X, sz.Y))
			for i := range fb.Pix {
				fb.Pix[i] = 0xe0
			}
			if err := pw.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
			}
		case platform.FocusEvent:
			log = append(log, "focus")
		case platform.IMESessionEvent:
			log = append(log, "session")
			session = e.Active
			if e.Active && manual {
				t.Log("type into the window now…")
			}
			if e.Active && forward && !forwarded {
				forwarded = true
				k := xproto.KeyPressEvent{Detail: 38, Root: b.root, Event: w.id, Time: 1, SameScreen: true}
				b.imeForward(w.ic, k)
				b.imeForward(w.ic, xproto.KeyReleaseEvent(k))
				time.AfterFunc(time.Second, b.Quit)
			}
		case platform.KeyEvent:
			t.Logf("key %v %v %q %q", e.Action, e.Code, e.Key, e.Text)
			log = append(log, "key")
		case platform.IMEEvent:
			t.Logf("ime commit=%q preedit=%q cursor=%d,%d", e.Commit, e.Preedit, e.CursorStart, e.CursorEnd)
			log = append(log, "ime")
		}
	})
	if err := b.Run(h); err != nil {
		t.Fatal(err)
	}
	ime := &b.ime
	t.Logf("input method %q: locale %q, UTF-8 %v, style %#x of %#x, boundary %d, IC %d, err %v",
		ime.name, ime.locale, ime.utf8, ime.style, ime.styles, ime.boundary, icid, ime.err)
	t.Logf("events: %v", log)
	if ime.state != imeOff {
		t.Errorf("state after Run = %d, want off", ime.state)
	}
	if ime.style == 0 {
		t.Fatalf("the handshake did not finish (error: %v)", ime.err)
	}
	if icid == 0 {
		t.Fatalf("no input context was created (error: %v)", ime.err)
	}
	if forward {
		if !session {
			t.Fatal("the window never got the keyboard focus, so no session started")
		}
		got := false
		for _, s := range log {
			got = got || s == "key" || s == "ime"
		}
		if !got {
			t.Error("the forwarded A key produced neither a KeyEvent nor an IMEEvent")
		}
	}
}

type handlerFunc func(platform.Window, platform.Event)

func (f handlerFunc) HandleEvent(w platform.Window, e platform.Event) { f(w, e) }
