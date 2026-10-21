package h_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Counter is the example of ARCHITECTURE section 1, verbatim.
func Counter() tela.Node {
	count, setCount := tela.UseState(0)

	return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl "+
		"bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
		H1(Class("text-5xl font-bold text-white tracking-tight"),
			Textf("Count: %d", count)),
		Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur "+
			"hover:bg-white/25 active:scale-95 transition"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Text("Increment"),
		),
	)
}

func main() {
	tela.Run(tela.App{Title: "Demo", Width: 800, Height: 600, Root: tela.C(Counter)})
}

// ExampleCounter checks that the example compiles as an application.
func ExampleCounter() {
	_ = main // Run opens a window: compile it, do not call it.
	// Output:
}

func TestCounterGolden(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			_, img := core.Render(core.Component(Counter), core.RenderOptions{
				Config: core.Config{RootClass: "flex items-center justify-center bg-slate-900"},
				Width:  400, Height: 300, Scale: scale,
			})
			golden.Image(t, fmt.Sprintf("counter_x%g", scale), img)
		})
	}
}
