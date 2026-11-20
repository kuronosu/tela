package devtools

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// TestGoldens renders the panels over the made-up session at the size of
// the window, in both schemes (the previews of `tela snapshot`).
func TestGoldens(t *testing.T) {
	cases := []struct {
		name    string
		preview string
		dark    bool
	}{
		{"elements_dark", "Elements", true},
		{"elements_light", "Elements", false},
		{"box_model_dark", "ElementsBox", true},
		{"accessibility_dark", "Accessibility", true},
		{"profiler_dark", "Profiler", true},
		{"profiler_light", "Profiler", false},
		{"logs_dark", "Logs", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scheme := tela.Light
			if c.dark {
				scheme = tela.Dark
			}
			app := telatest.Mount(t, Previews[c.preview].Root, telatest.FromApp(App), telatest.ColorScheme(scheme))
			app.Golden("devtools_" + c.name + ".png")
		})
	}
}
