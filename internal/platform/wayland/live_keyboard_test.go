//go:build linux

package wayland

import (
	"image"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// TestLiveKeyboard types into a real window with wtype (the
// virtual-keyboard protocol; wlroots compositors such as Hyprland and
// Sway) and checks the KeyEvents: text through the compositor's keymap,
// Ctrl and Shift shortcuts, key repeat with the compositor's repeat_info,
// and that the loop goes idle afterwards. Run it with
//
//	TELA_LIVE=1 CGO_ENABLED=0 go test -run TestLiveKeyboard -v ./internal/platform/wayland/
//
// with nothing else grabbing the keyboard: the new window must get focus.
// TELA_LIVE_TYPE=manual waits 15 s for the user to type instead and only
// logs the events (for checking the real keyboard, dead keys and
// Compose).
func TestLiveKeyboard(t *testing.T) {
	skipUnlessLive(t)
	manual := strings.EqualFold(os.Getenv("TELA_LIVE_TYPE"), "manual")
	if _, err := exec.LookPath("wtype"); err != nil && !manual {
		t.Skip("wtype is not installed (or set TELA_LIVE_TYPE=manual to type by hand)")
	}
	b := liveBackend(t)
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live keyboard", AppID: "dev.tela.livetest", Size: platform.Size{W: 320, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)

	var (
		mu      sync.Mutex
		events  []platform.KeyEvent
		focused = make(chan struct{})
		once    sync.Once
	)
	snapshot := func() []platform.KeyEvent {
		mu.Lock()
		defer mu.Unlock()
		out := events
		events = nil
		return out
	}
	text := func(evs []platform.KeyEvent) string {
		var s strings.Builder
		for _, e := range evs {
			s.WriteString(e.Text)
		}
		return s.String()
	}
	wtype := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("wtype", args...).CombinedOutput(); err != nil {
			t.Errorf("wtype %v: %v: %s", args, err, out)
		}
		time.Sleep(300 * time.Millisecond)
	}

	go func() {
		defer b.Quit()
		select {
		case <-focused:
		case <-time.After(5 * time.Second):
			t.Error("the window never got keyboard focus")
			return
		}
		time.Sleep(200 * time.Millisecond)
		snapshot()
		if manual {
			t.Log("type now (15 s)…")
			time.Sleep(15 * time.Second)
			for _, e := range snapshot() {
				t.Logf("%v %v key=%q text=%q mods=%v repeat=%v synthetic=%v", e.Action, e.Code, e.Key, e.Text, e.Mods, e.Repeat, e.Synthetic)
			}
			return
		}

		const typed = "Hello, wörld! 123 é"
		wtype(typed)
		if got := text(snapshot()); got != typed {
			t.Errorf("typed %q, got text %q", typed, got)
		}

		wtype("-M", "ctrl", "a", "-m", "ctrl")
		ok := false
		for _, e := range snapshot() {
			if e.Action == platform.KeyPress && e.Key == "a" && e.Mods == platform.ModCtrl {
				ok = true
			}
		}
		if !ok {
			t.Error("Ctrl+A: no press with Key \"a\" and Mods Ctrl")
		}

		// wtype uploads its own keymap with one keysym per key (type
		// ONE_LEVEL, arbitrary keycodes), so Shift selects no level: like
		// libxkbcommon, the key stays "a". Level selection is covered by the
		// libxkbcommon oracle tests of internal/xkb; here only Mods is checked.
		wtype("-M", "shift", "a", "-m", "shift")
		ok = false
		evs := snapshot()
		for _, e := range evs {
			if e.Action == platform.KeyPress && e.Key == "a" && e.Mods == platform.ModShift {
				ok = true
			}
		}
		if !ok {
			t.Error("Shift+A: no press with Key \"a\" and Mods Shift")
			for _, e := range evs {
				t.Logf("  %v %v key=%q text=%q mods=%v", e.Action, e.Code, e.Key, e.Text, e.Mods)
			}
		}

		// Hold a key for 1.5 s: client-side repeat.
		wtype("-P", "x", "-s", "1500", "-p", "x")
		repeats := 0
		var last time.Duration
		for _, e := range snapshot() {
			if e.Repeat {
				if e.Key != "x" || e.Text != "x" {
					t.Errorf("repeat %+v", e)
				}
				if repeats > 0 {
					t.Logf("repeat interval %v", e.Time-last)
				}
				last = e.Time
				repeats++
			}
		}
		t.Logf("%d repeats while x was held for 1.5 s", repeats)
		if repeats < 3 {
			t.Errorf("only %d repeats while a key was held for 1.5 s", repeats)
		}

		// Idle: no key held, no repeat timer: the process sleeps.
		start := cpuTime(t)
		time.Sleep(time.Second)
		if used := cpuTime(t) - start; used > 20*time.Millisecond {
			t.Errorf("CPU time %v over 1 s of keyboard idle; a repeat timer may still be armed", used)
		}
	}()

	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			quadrants(fb)
			if err := w.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
			}
		case platform.FocusEvent:
			if e.Focused {
				once.Do(func() { close(focused) })
			}
		case platform.KeyEvent:
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if b.keyboardErr != nil {
		t.Errorf("keyboard error: %v", b.keyboardErr)
	}
}
