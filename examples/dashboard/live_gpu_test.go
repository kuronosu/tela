//go:build telagpu && linux

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/core/gpulive"
	"github.com/kuronosu/tela/internal/stdfonts"
)

// TestLiveGPU runs the dashboard in a real window, drawn by the GPU and
// presented without a copy where the display server takes its buffers
// (internal/core/gpulive has the rig and how to run it):
//
//	go test -c -tags telagpu -o bin/dashboard.test ./examples/dashboard
//	TELA_LIVE=1 TELA_BACKEND=x11 DISPLAY=:0 bin/dashboard.test -test.run TestLiveGPU -test.v
//
// its first frames, the dark theme and back. On X11 the pixels of each
// step are read back from the server and compared with the CPU
// rasterizer's rendering of the same frame, within ADR-282.
func TestLiveGPU(t *testing.T) {
	steps := []gpulive.Step{
		{Name: "dark", Do: func(h *core.Host) { h.Tree().SetColorScheme(core.ColorSchemeDark) }},
		{Name: "light", Do: func(h *core.Host) { h.Tree().SetColorScheme(core.ColorSchemeLight) }},
	}
	res := gpulive.Run(t, gpulive.Config{
		Class: "dev.telaui.livegpu",
		Options: core.Options{
			Config: core.Config{RootClass: rootClass, Theme: (*core.Theme)(&theme), Fonts: stdfonts.NewCollection()},
			Window: core.WindowSpec{Title: "tela live gpu dashboard", Width: 1440, Height: 900},
			Root:   Dashboard(),
		},
		Steps: steps,
	})
	st := res.Window.Stats
	t.Logf("%s, %s: direct %v; %d presents of dmabufs, %d found every buffer busy, %d dropped; %d buffers created, %d destroyed, %d busy at the end",
		res.Backend, res.Device, res.Window.Direct, st.Shown, st.Busy, st.Dropped, st.Created, st.Destroyed, res.Window.Busy)
	if res.Device == "" {
		t.Fatalf("no GPU was opened; log: %s", res.Log)
	}
	if res.Backend == "wayland" {
		t.Logf("the pixels were NOT looked at: nothing reads a Wayland window back; %d frames were paced by the rig's timer (the compositor sends a window on a hidden workspace no frame callbacks)", res.Paced)
	}
	if res.Backend == "x11" {
		t.Logf("%d steps read back from the X server; against the CPU rasterizer at most %d levels (at %v), %.3f%% of the pixels over 2", res.Checked, res.Worst.Max, res.Worst.Worst, 100*res.Worst.Over2)
		if res.Checked < len(steps)+1 {
			t.Errorf("%d steps were read back and compared, want %d", res.Checked, len(steps)+1)
		}
	}
	if os.Getenv("TELA_GPU_DEVICE") == "" && os.Getenv(core.EnvPresent) != "copy" {
		if !res.Window.Direct || st.Shown == 0 {
			t.Errorf("the dashboard was not presented without a copy (direct %v, %d presents)", res.Window.Direct, st.Shown)
		}
		if strings.TrimSpace(res.Log) != "" {
			t.Errorf("the host logged: %s", res.Log)
		}
	} else if res.Log != "" {
		t.Logf("the host logged: %s", res.Log)
	}
}
