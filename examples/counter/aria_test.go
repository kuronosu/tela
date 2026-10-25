package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestAriaSnapshot pins what `tela inspect --aria ./examples/counter`
// prints (testdata/counter.aria.yml).
func TestAriaSnapshot(t *testing.T) {
	app := telatest.Mount(t, Counter(), telatest.Size(800, 600))
	app.AriaGolden("counter.aria.yml")
}

// TestClickByRole finds the Increment button through the semantics tree,
// as an assistive technology or tela mcp does, and clicks it.
func TestClickByRole(t *testing.T) {
	app := telatest.Mount(t, Counter(), telatest.Size(800, 600))
	app.Click(telatest.ByRole("button", "Increment"))
	app.ExpectAria(`
		- 'heading "Count: 1" [level=1]'
		- button "Increment"
	`)
}
