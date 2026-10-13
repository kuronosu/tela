//go:build linux

package x11

import (
	"image"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// TestLiveKeyboard types into a real X11 window and checks the KeyEvents:
// text through the server's keyboard mapping, Ctrl and Shift shortcuts,
// server key repeat with detectable auto-repeat, and that the loop goes
// idle afterwards. It types with xdotool (XTEST; any X server) when it is
// installed, else with wtype through Xwayland (the compositor's virtual
// keyboard reaches the focused X11 window; wlroots compositors such as
// Hyprland and Sway). Run it with
//
//	TELA_LIVE=1 CGO_ENABLED=0 go test -run TestLiveKeyboard -v ./internal/platform/x11/
//
// with nothing else grabbing the keyboard: the new window must get focus.
// TELA_LIVE_TYPE=manual waits 15 s for the user to type instead and only
// logs the events (for checking the real keyboard, AltGr, dead keys and
// Compose).
func TestLiveKeyboard(t *testing.T) {
	b := liveBackend(t)
	manual := strings.EqualFold(os.Getenv("TELA_LIVE_TYPE"), "manual")
	tool := ""
	switch {
	case manual:
	case haveTool("xdotool"):
		tool = "xdotool"
	case haveTool("wtype") && os.Getenv("WAYLAND_DISPLAY") != "":
		tool = "wtype"
	default:
		b.conn.Close()
		t.Skip("neither xdotool nor wtype (with WAYLAND_DISPLAY) is available (or set TELA_LIVE_TYPE=manual to type by hand)")
	}
	t.Logf("typing with %s; XKB %v, detectable auto-repeat %v, XInput2 %v", tool, b.xkb.ok, b.xkb.detectable, b.xi.ok)
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela x11 keyboard", AppID: "dev.tela.x11test", Size: platform.Size{W: 320, H: 200}})
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
	run := func(name string, args ...string) {
		t.Helper()
		if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Errorf("%s %v: %v: %s", name, args, err, out)
		}
		time.Sleep(300 * time.Millisecond)
	}
	typeText := func(s string) {
		if tool == "xdotool" {
			run("xdotool", "type", "--delay", "30", s)
		} else {
			run("wtype", s)
		}
	}
	chord := func(mod, key string) {
		if tool == "xdotool" {
			run("xdotool", "key", mod+"+"+key)
		} else {
			run("wtype", "-M", mod, key, "-m", mod)
		}
	}
	hold := func(key string, d time.Duration) {
		if tool == "xdotool" {
			run("xdotool", "keydown", key)
			time.Sleep(d)
			run("xdotool", "keyup", key)
		} else {
			run("wtype", "-P", key, "-s", strconv.Itoa(int(d.Milliseconds())), "-p", key)
		}
	}
	logEvents := func(evs []platform.KeyEvent) {
		for _, e := range evs {
			t.Logf("  %v %v key=%q text=%q mods=%v repeat=%v synthetic=%v", e.Action, e.Code, e.Key, e.Text, e.Mods, e.Repeat, e.Synthetic)
		}
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
			logEvents(snapshot())
			return
		}

		const typed = "Hello, wörld! 123 é"
		typeText(typed)
		if evs := snapshot(); text(evs) != typed {
			t.Errorf("typed %q, got text %q", typed, text(evs))
			logEvents(evs)
		}

		chord("ctrl", "a")
		ok := false
		evs := snapshot()
		for _, e := range evs {
			if e.Action == platform.KeyPress && e.Key == "a" && e.Text == "a" && e.Mods == platform.ModCtrl {
				ok = true
			}
		}
		if !ok {
			t.Error("Ctrl+A: no press with Key \"a\", Text \"a\" and Mods Ctrl")
			logEvents(evs)
		}

		// With xdotool the server's mapping types "A"; wtype's own
		// keymap has one level per key (see the Wayland test), so only
		// Mods is checked strictly.
		chord("shift", "a")
		ok = false
		evs = snapshot()
		for _, e := range evs {
			if e.Action == platform.KeyPress && (e.Key == "A" || tool == "wtype" && e.Key == "a") && e.Mods == platform.ModShift {
				ok = true
			}
		}
		if !ok {
			t.Error("Shift+A: no press with Key \"A\" and Mods Shift")
			logEvents(evs)
		}

		// Hold a key for 1.5 s: the server repeats it.
		hold("x", 1500*time.Millisecond)
		repeats, releases := 0, 0
		var last time.Duration
		for _, e := range snapshot() {
			switch {
			case e.Repeat:
				if e.Key != "x" || e.Text != "x" {
					t.Errorf("repeat %+v", e)
				}
				if repeats > 0 {
					t.Logf("repeat interval %v", e.Time-last)
				}
				last = e.Time
				repeats++
			case e.Action == platform.KeyRelease && e.Key == "x":
				releases++
			}
		}
		t.Logf("%d repeats while x was held for 1.5 s, %d releases", repeats, releases)
		if repeats < 3 {
			t.Errorf("only %d repeats while a key was held for 1.5 s", repeats)
		}
		if releases != 1 {
			t.Errorf("%d releases of x, want 1 (repeats must not have releases in between)", releases)
		}

		// Idle: the server repeats, Tela arms nothing.
		start := cpuTime(t)
		time.Sleep(time.Second)
		if used := cpuTime(t) - start; used > 20*time.Millisecond {
			t.Errorf("CPU time %v over 1 s of keyboard idle", used)
		}
	}()

	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			fill(fb, 3, fb.Rect)
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
	if b.kbd.err != nil {
		t.Errorf("keyboard error: %v", b.kbd.err)
	}
}

func haveTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
