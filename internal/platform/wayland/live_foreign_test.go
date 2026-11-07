//go:build linux

package wayland

import (
	"encoding/json"
	"image"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Live checks of foreign.go and the suspended state (ADR-233). The window
// opens on a workspace nobody looks at, so it never takes the focus:
// TELA_LIVE_WORKSPACE (default "7") through a Hyprland window rule that
// matches only this test's app id; other compositors open it where they
// open windows.

const liveForeignAppID = "dev.telaui.liveforeign"

// placeLiveWindow adds a Hyprland rule that opens the test's window on
// the hidden workspace, silently, and disables it at the end.
func placeLiveWindow(t *testing.T) {
	t.Helper()
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return
	}
	ws := os.Getenv("TELA_LIVE_WORKSPACE")
	if ws == "" {
		ws = "7"
	}
	rule := `tela_live_foreign = hl.window_rule({ name = "tela-live-foreign", match = { class = "` + liveForeignAppID + `" }, workspace = "` + ws + ` silent" })`
	if out, err := exec.Command("hyprctl", "eval", rule).CombinedOutput(); err != nil || strings.Contains(strings.ToLower(string(out)), "error") {
		t.Logf("hyprctl eval (window rule): %v %s; the window opens on the current workspace", err, out)
		return
	}
	t.Cleanup(func() {
		exec.Command("hyprctl", "eval", `if tela_live_foreign then tela_live_foreign:set_enabled(false) end`).Run()
	})
}

// logWorkspace logs the Hyprland workspace of this process's window.
func logWorkspace(t *testing.T) {
	out, err := exec.Command("hyprctl", "clients", "-j").Output()
	if err != nil {
		return
	}
	var clients []struct {
		Pid       int `json:"pid"`
		Workspace struct {
			Name string `json:"name"`
		} `json:"workspace"`
	}
	if json.Unmarshal(out, &clients) != nil {
		return
	}
	for _, c := range clients {
		if c.Pid == os.Getpid() {
			t.Logf("the window is on Hyprland workspace %q", c.Workspace.Name)
		}
	}
}

// TestLiveForeign exports a window and gets activation tokens from the
// compositor; on a hidden workspace it also reports whether the window is
// suspended (Minimized) and that frames pause then. No portal is called.
func TestLiveForeign(t *testing.T) {
	b := liveBackend(t)
	placeLiveWindow(t)
	if b.exporter == nil {
		t.Log("the compositor has no zxdg_exporter_v2: portal dialogs get no parent")
	}
	t.Logf("xdg_wm_base version %d (6 has the suspended state)", b.wm.Version())
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live foreign", AppID: liveForeignAppID, Size: platform.Size{W: 240, H: 160}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	type answer struct{ parent, token string }
	var (
		answers   []answer
		minimized bool
		framesMin int
		step      int
	)
	ask := func(token bool) {
		b.preparePortal(w, token, func(p, tk string) {
			answers = append(answers, answer{p, tk})
			b.Wake()
		})
	}
	time.AfterFunc(15*time.Second, b.Quit)
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.WindowStateEvent:
			if e.State.Minimized && !minimized {
				t.Logf("suspended: WindowStateEvent %v", e.State)
				minimized = true
				w.RequestFrame()
				time.AfterFunc(500*time.Millisecond, b.Wake)
			}
		case platform.FrameEvent:
			if minimized {
				framesMin++
			}
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			quadrants(fb)
			if err := w.Present(fb, nil); err != nil {
				t.Fatalf("Present: %v", err)
			}
			if step == 0 {
				step = 1
				ask(true) // mapped now
			}
		case platform.WakeEvent:
			switch {
			case step == 1 && len(answers) == 1:
				step = 2
				ask(false) // the kept handle, at once
			case step == 2 && len(answers) == 2:
				step = 3
				w.SetVisible(false)
				ask(false) // hidden: none
			case step == 3 && len(answers) == 3:
				step = 4
				w.SetVisible(true) // exported anew at the next request
				time.AfterFunc(time.Second, b.Wake)
			case step == 4:
				step = 5
				ask(false)
			case step == 5 && len(answers) == 4:
				step = 6
				logWorkspace(t)
				time.AfterFunc(1500*time.Millisecond, b.Quit) // time for the suspended state
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(answers) < 4 {
		t.Fatalf("answers %+v: the sequence did not complete", answers)
	}
	t.Logf("answers: %+v", answers)
	if b.exporter != nil {
		a := answers[0]
		if !strings.HasPrefix(a.parent, "wayland:") || len(a.parent) <= len("wayland:") {
			t.Errorf("parent %q, want wayland:<handle>", a.parent)
		}
		if answers[1].parent != a.parent {
			t.Errorf("second parent %q, want the kept %q", answers[1].parent, a.parent)
		}
		if answers[2].parent != "" {
			t.Errorf("parent of a hidden window %q, want none", answers[2].parent)
		}
		if p := answers[3].parent; !strings.HasPrefix(p, "wayland:") || p == a.parent {
			t.Errorf("parent after showing %q, want a new handle (was %q)", p, a.parent)
		}
	}
	if b.activation != nil && answers[0].token == "" {
		t.Error("no activation token from xdg_activation_v1")
	}
	if minimized {
		t.Logf("suspended: %d frames delivered while suspended (want 0)", framesMin)
		if framesMin != 0 {
			t.Errorf("%d FrameEvents while suspended", framesMin)
		}
	} else {
		t.Log("the window was never suspended (on a visible workspace, or the compositor sends no suspended state)")
	}
}
