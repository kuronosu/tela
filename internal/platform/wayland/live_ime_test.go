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
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// TestLiveIME composes Japanese with fcitx5 and Mozc through
// zwp_text_input_v3: it switches fcitx5 to Mozc with fcitx5-remote (and
// back to the previous input method at the end; the configuration is not
// touched), types "nihongo" with wtype, converts with Space and commits
// with Return, and checks that preedits and then a commit of non-ASCII
// text arrive as IMEEvents. Run it with
//
//	TELA_LIVE=1 CGO_ENABLED=0 go test -run TestLiveIME -v ./internal/platform/wayland/
//
// with fcitx5 running and nothing else grabbing the keyboard.
// TELA_LIVE_TYPE=manual waits 20 s for the user to type instead and only
// logs the events. The app side is a minimal field: the value is the
// committed text with the caret at its end, sent back as surrounding text
// after every IMEEvent.
func TestLiveIME(t *testing.T) {
	skipUnlessLive(t)
	manual := strings.EqualFold(os.Getenv("TELA_LIVE_TYPE"), "manual")
	if !manual {
		for _, tool := range []string{"wtype", "fcitx5-remote"} {
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s is not installed (or set TELA_LIVE_TYPE=manual to type by hand)", tool)
			}
		}
	}
	b := liveBackend(t)
	if b.textInputManager == nil {
		t.Skip("the compositor has no zwp_text_input_manager_v3")
	}
	skipUnlessSeatKeyboard(t, b)
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live IME", AppID: "dev.telaui.livetest", Size: platform.Size{W: 360, H: 160}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)

	var (
		mu      sync.Mutex
		log     []string
		imes    []platform.IMEEvent
		value   string
		active  = make(chan struct{})
		once    sync.Once
		textKey int // KeyEvents with Text while a preedit was shown
		preedit string
	)
	state := func() *platform.TextInputState {
		return &platform.TextInputState{
			CursorArea:  platform.Rect{X: 20, Y: 40, H: 22},
			Surrounding: &platform.SurroundingText{Text: value, Cursor: len(value), Anchor: len(value)},
		}
	}
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Errorf("%s %v: %v: %s", name, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	go func() {
		defer b.Quit()
		select {
		case <-active:
		case <-time.After(5 * time.Second):
			t.Error("no IMESessionEvent: the text input never entered the window (is fcitx5 running on this compositor?)")
			return
		}
		time.Sleep(200 * time.Millisecond)
		if manual {
			t.Log("type now with the IME (20 s)…")
			time.Sleep(20 * time.Second)
			return
		}
		prev := run("fcitx5-remote", "-n")
		run("fcitx5-remote", "-s", "mozc")
		defer func() {
			if prev != "" {
				run("fcitx5-remote", "-s", prev)
			}
		}()
		time.Sleep(300 * time.Millisecond)
		run("wtype", "nihongo")
		time.Sleep(500 * time.Millisecond)
		run("wtype", "-k", "space")
		time.Sleep(500 * time.Millisecond)
		run("wtype", "-k", "Return")
		time.Sleep(500 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		preedits, commit := 0, ""
		for _, e := range imes {
			if e.Preedit != "" {
				preedits++
			}
			commit += e.Commit
		}
		if preedits == 0 {
			t.Errorf("no preedit arrived (is fcitx5 running with Mozc, and does the compositor route wtype through the IME?)")
		}
		if commit == "" || !utf8.ValidString(commit) || commit == "nihongo" {
			t.Errorf("committed %q, want the converted text", commit)
		}
		if textKey > 0 {
			t.Logf("%d KeyEvents with Text arrived while a preedit was shown (the IME passed them through)", textKey)
		}
		t.Logf("value %q", value)
	}()

	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		mu.Lock()
		defer mu.Unlock()
		switch e := e.(type) {
		case platform.FrameEvent:
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			quadrants(fb)
			if err := w.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
			}
		case platform.FocusEvent:
			log = append(log, "focus "+map[bool]string{true: "in", false: "out"}[e.Focused])
			if e.Focused {
				w.SetTextInput(state())
			}
		case platform.IMESessionEvent:
			log = append(log, "session "+map[bool]string{true: "on", false: "off"}[e.Active])
			if e.Active {
				once.Do(func() { close(active) })
			}
		case platform.IMEEvent:
			imes = append(imes, e)
			t.Logf("IMEEvent del=%d,%d commit=%q preedit=%q cursor=%d,%d", e.DeleteBefore, e.DeleteAfter, e.Commit, e.Preedit, e.CursorStart, e.CursorEnd)
			value = value[:max(0, len(value)-e.DeleteBefore)] + e.Commit
			preedit = e.Preedit
			w.SetTextInput(state())
		case platform.KeyEvent:
			t.Logf("KeyEvent %v %v key=%q text=%q", e.Action, e.Code, e.Key, e.Text)
			if e.Text != "" && preedit != "" {
				textKey++
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Logf("events: %s", strings.Join(log, ", "))
}
