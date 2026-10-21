package tela_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseSize() {
	t := &exampleT{}
	defer t.close()
	var size *tela.Value[tela.Size]
	feed := func() tela.Node {
		card := tela.UseElementRef()
		size = tela.UseSize(card)
		s := size.Get() // a build reads it: the label follows the next frame
		return Div(TestID("feed"), Class("h-40 overflow-y-auto"),
			Div(Ref(card), Class("h-24 w-64 scale-150"), Textf("%.0f x %.0f", s.Width, s.Height)),
			Div(Class("h-96")),
		)
	}
	app := telatest.Mount(t, tela.C(feed))
	app.ExpectText("256 x 96") // the layout size: the scale does not count
	app.Scroll(telatest.ByTestID("feed"), 0, 80)
	fmt.Println(size.Get()) // scrolling does not change it either
	// Output: {256 96}
}
