package tela_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleElementRef_ScrollToIndex() {
	t := &exampleT{}
	defer t.close()
	var list tela.ElementRef
	page := func() tela.Node {
		list = tela.UseElementRef()
		return VirtualList(100_000,
			func(int) float32 { return 40 },
			func(i int) tela.Node { return Div(Class("h-10"), Textf("Row %d", i)) },
			Ref(list), Class("h-[400px]"))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(200, 400))
	// Item 50,000 ends up in the middle of the viewport (200 px from its
	// top, a row of 40 px: its top is at 180). The list keeps it there while
	// the items around it are measured.
	list.ScrollToIndex(50_000, tela.AlignCenter)
	fmt.Println(app.Bounds(telatest.ByText("Row 50000", telatest.Exact())).Y)
	// AlignStart puts it at the top, AlignEnd at the bottom, and AlignAuto
	// scrolls only when it is not fully visible.
	list.ScrollToIndex(50_000, tela.AlignStart)
	fmt.Println(app.Bounds(telatest.ByText("Row 50000", telatest.Exact())).Y)
	list.ScrollToIndex(50_000, tela.AlignEnd)
	fmt.Println(app.Bounds(telatest.ByText("Row 50000", telatest.Exact())).Y)
	// Output:
	// 180
	// 0
	// 360
}
