//go:build telagpu && linux

package core_test

import (
	"os"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/core/gpulive"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/style"
)

// TestLiveGPU runs a small app in a real window, drawn by the GPU core
// opens for it and presented without a copy where the display server
// takes its buffers (gpulive has the rig and how to run it): partial
// frames, a transition's frames one after the other, hiding and showing
// with a change in between, and the theme. On X11 every step's pixels are
// read back from the server and compared with the CPU rasterizer's.
func TestLiveGPU(t *testing.T) {
	div := func(c ...core.Child) core.Node { return core.Primitive(style.KindDiv, c...) }
	var set core.Setter[int]
	root := core.Component(func() core.Node {
		n, setN := core.UseState(0)
		set = setN
		colors := []string{"bg-red-500", "bg-emerald-500", "bg-sky-500", "bg-amber-400"}
		return div(core.Class("relative h-screen bg-slate-100 p-6 dark:bg-slate-900"),
			div(core.Class("rounded-xl bg-white p-4 text-2xl text-slate-900 shadow-lg dark:bg-slate-800 dark:text-white"), core.Textf("Frame %d of the live test", n)),
			div(core.Class("mt-4 h-24 rounded-2xl bg-linear-to-r from-indigo-500 to-rose-500")),
			div(core.Class("mt-4 h-16 rounded-xl transition-colors duration-500 ease-linear "+colors[n%len(colors)])),
			div(core.Class("absolute bottom-6 right-6 size-20 rounded-full border-4 border-white shadow-xl "+colors[(n+1)%len(colors)])))
	})
	next := func(name string) gpulive.Step {
		return gpulive.Step{Name: name, Do: func(*core.Host) { set.Update(func(v int) int { return v + 1 }) }}
	}
	steps := []gpulive.Step{
		next("a partial frame"), next("a second one"), next("a third one"), next("a fourth one"),
		{Name: "hide", Do: func(h *core.Host) { h.Handle(h.Window()).Hide() }, Unchecked: true},
		{Name: "a change while hidden", Do: func(*core.Host) { set.Update(func(v int) int { return v + 1 }) }, Unchecked: true},
		{Name: "show", Do: func(h *core.Host) { h.Handle(h.Window()).Show() }},
		next("a frame after the show"),
		{Name: "dark", Do: func(h *core.Host) { h.Tree().SetColorScheme(core.ColorSchemeDark) }},
		next("a frame in the dark"),
		{Name: "light", Do: func(h *core.Host) { h.Tree().SetColorScheme(core.ColorSchemeLight) }},
	}
	res := gpulive.Run(t, gpulive.Config{
		Class: "dev.telaui.livegpu",
		Options: core.Options{
			Config: core.Config{Fonts: stdfonts.NewCollection()},
			Window: core.WindowSpec{Title: "tela live gpu", Width: 640, Height: 420},
			Root:   root,
		},
		Steps: steps,
	})
	report(t, res, len(steps)-2)
}

// report logs what a live run did and checks it: a GPU drew the window,
// without a copy unless the environment chose another device or path, the
// steps that could be read back were, and the Host logged nothing.
func report(t *testing.T, res gpulive.Result, checked int) {
	t.Helper()
	st := res.Window.Stats
	t.Logf("%s, %s: direct %v; %d presents of dmabufs, %d found every buffer busy, %d dropped; %d buffers created, %d destroyed, %d registered and %d busy at the end",
		res.Backend, res.Device, res.Window.Direct, st.Shown, st.Busy, st.Dropped, st.Created, st.Destroyed, st.Buffers, res.Window.Busy)
	if res.Device == "" {
		t.Fatalf("no GPU was opened; log: %s", res.Log)
	}
	if res.Backend == "wayland" {
		t.Logf("the pixels were NOT looked at: nothing reads a Wayland window back; %d frames were paced by the rig's timer (the compositor sends a window on a hidden workspace no frame callbacks)", res.Paced)
	}
	if res.Backend == "x11" {
		t.Logf("%d steps read back from the X server; against the CPU rasterizer at most %d levels (at %v), %.3f%% of the pixels over 2", res.Checked, res.Worst.Max, res.Worst.Worst, 100*res.Worst.Over2)
		if res.Checked < checked {
			t.Errorf("%d steps were read back and compared, want %d", res.Checked, checked)
		}
	}
	if os.Getenv("TELA_GPU_DEVICE") == "" && os.Getenv(core.EnvPresent) != "copy" {
		if !res.Window.Direct || st.Shown == 0 {
			t.Errorf("the window was not presented without a copy (direct %v, %d presents)", res.Window.Direct, st.Shown)
		}
		if res.Window.Busy > 2 {
			t.Errorf("%d buffers are still busy at the end: the display server releases none", res.Window.Busy)
		}
		if strings.TrimSpace(res.Log) != "" {
			t.Errorf("the host logged: %s", res.Log)
		}
	} else if res.Log != "" {
		t.Logf("the host logged: %s", res.Log)
	}
}
