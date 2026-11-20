package telatest_test

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// OpenedURLs lists the URLs the app opened, in order, and leaves paths out.
func TestOpenedURLs(t *testing.T) {
	page := func() tela.Node {
		return Div(
			Button(OnClick(func() { _ = tela.OpenURL("https://example.com/a") }), Text("A")),
			Button(OnClick(func() { _ = tela.OpenPath("/tmp") }), Text("Folder")),
			Button(OnClick(func() { _ = tela.OpenURL("mailto:ana@example.com") }), Text("Mail")),
		)
	}
	app := telatest.Mount(t, tela.C(page))
	if got := app.OpenedURLs(); len(got) != 0 {
		t.Fatalf("OpenedURLs before any click = %q, want none", got)
	}
	app.Click(telatest.ByRole("button", "A"))
	app.Click(telatest.ByRole("button", "Folder"))
	app.Click(telatest.ByRole("button", "Mail"))
	want := []string{"https://example.com/a", "mailto:ana@example.com"}
	if got := app.OpenedURLs(); !slices.Equal(got, want) {
		t.Errorf("OpenedURLs = %q, want %q", got, want)
	}
}
