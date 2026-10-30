//go:build linux

package wayland

import (
	"os/exec"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// TestLiveClipboard copies between a Tela window and wl-clipboard
// (wl-copy, wl-paste) through the real compositor, in both directions.
// It needs the window to get keyboard focus (compositors only accept
// selections from, and only announce them to, the focused client), and
// it replaces the session's clipboard; the previous text, if any, is put
// back at the end.
func TestLiveClipboard(t *testing.T) {
	skipUnlessLive(t)
	if _, err := exec.LookPath("wl-paste"); err != nil {
		t.Skip("wl-clipboard (wl-copy, wl-paste) is not installed")
	}
	if _, err := exec.LookPath("wl-copy"); err != nil {
		t.Skip("wl-clipboard (wl-copy, wl-paste) is not installed")
	}
	if prev, err := exec.Command("wl-paste", "--no-newline", "--type", "text").Output(); err == nil {
		t.Cleanup(func() { exec.Command("wl-copy", "--", string(prev)).Run() })
	}

	const ours = "copied from Tela ✓ ñ"
	const theirs = "pasted into Tela ✓ €"
	b := liveBackend(t)
	skipUnlessSeatKeyboard(t, b)
	if _, err := b.NewWindow(platform.WindowOptions{Title: "tela live clipboard", Size: platform.Size{W: 240, H: 160}}); err != nil {
		t.Fatal(err)
	}

	type result struct {
		out string
		err error
	}
	results := make(chan result, 1)
	// External commands run on their own goroutines (they talk to the
	// compositor while the loop serves our selection) and report back
	// through a channel and Wake.
	// wl-copy forks a server that keeps running, so its output is not
	// captured (Output would wait for the fork's stdout to close).
	run := func(name string, args ...string) {
		go func() {
			var r result
			if name == "wl-copy" {
				r.err = exec.Command(name, args...).Run()
			} else {
				out, err := exec.Command(name, args...).Output()
				r = result{string(out), err}
			}
			results <- r
			b.Wake()
		}()
	}

	step := 0
	presented := false
	deadline := time.AfterFunc(10*time.Second, func() { b.Wake() })
	defer deadline.Stop()
	start := time.Now()
	err := b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		if time.Since(start) > 10*time.Second {
			t.Errorf("timed out at step %d", step)
			b.Quit()
			return
		}
		switch e := e.(type) {
		case platform.FrameEvent:
			if !presented {
				presented = true
				w.Present(solid(w, 0x20, 0x60, 0xa0), nil)
			}
		case platform.FocusEvent:
			if e.Focused && step == 0 {
				step = 1
				// Ours → wl-paste.
				if err := b.Clipboard().WriteText(ours); err != nil {
					t.Errorf("WriteText: %v", err)
					b.Quit()
					return
				}
				run("wl-paste", "--no-newline", "--type", "text/plain;charset=utf-8")
			}
		case platform.WakeEvent:
			select {
			case r := <-results:
				switch step {
				case 1:
					if r.err != nil || r.out != ours {
						t.Errorf("wl-paste = %q, %v; want %q", r.out, r.err, ours)
					}
					step = 2
					run("wl-copy", "--", theirs)
				case 2:
					if r.err != nil {
						t.Errorf("wl-copy: %v", r.err)
					}
					step = 3
					// Give the compositor time to announce the new
					// selection to the focused window.
					time.AfterFunc(300*time.Millisecond, b.Wake)
				}
			default:
				if step == 3 {
					step = 4
					b.Clipboard().ReadText(func(text string, err error) {
						if err != nil || text != theirs {
							t.Errorf("ReadText = %q, %v; want %q", text, err, theirs)
						}
						b.Quit()
					})
				}
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if step < 4 && !t.Failed() {
		t.Errorf("stopped at step %d (did the window get keyboard focus?)", step)
	}
}
