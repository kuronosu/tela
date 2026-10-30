package h_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleOverscan() {
	t := &exampleT{}
	defer t.close()
	// Three rows fit in the list; Overscan adds items beyond each side of
	// the visible ones (five by default), so a short scroll finds them
	// already built.
	for _, overscan := range []int{0, 2, 5} {
		rows := VirtualList(1_000_000,
			func(int) float32 { return 40 },
			func(i int) tela.Node { return Div(Class("h-10"), Textf("Row %d", i)) },
			Class("h-[120px]"), Overscan(overscan))
		app := telatest.Mount(t, rows, telatest.Size(200, 120))
		fmt.Println(overscan, app.Count(telatest.ByRole("listitem", "")))
	}
	// Output:
	// 0 3
	// 2 5
	// 5 8
}
