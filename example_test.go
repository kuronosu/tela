package tela_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

func ExamplePreviews() {
	login := func() tela.Node { return core.Text("Login") }
	settings := func() tela.Node { return core.Text("Settings") }

	previews := tela.Previews{"Settings": settings, "Login": login}
	// `tela snapshot` renders the first name unless --preview says otherwise.
	fmt.Println(previews.Names())
	// Output: [Login Settings]
}

func ExampleHex() {
	c := tela.Hex("#1e1e2e")
	fmt.Println(c.R == tela.RGB(30, 30, 46).R, c)
	// Output: true #1e1e2e
}
