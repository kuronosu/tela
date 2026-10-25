package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TitleLevel makes the title a heading of that level, with the classes of
// the div it is by default; a level outside 0 to 6 panics in dev mode and
// is clamped otherwise (ADR-154, D7).
func TestCardTitleLevel(t *testing.T) {
	for level := 1; level <= 6; level++ {
		app := telatest.Mount(t, kit.Card{TitleLevel: level, Title: h.Text("Notifications"), Description: h.Text("Choose.")})
		app.ExpectAria(fmt.Sprintf(`
            - heading "Notifications" [level=%d]
            - text: Choose.`, level))
	}
	app := telatest.Mount(t, kit.Card{Title: h.Text("Plain"), Description: h.Text("Choose.")})
	app.ExpectCount(telatest.ByRole("heading", ""), 0)

	// The same classes, on the heading.
	app = telatest.Mount(t, kit.Card{TitleLevel: 2, Title: h.Text("Styled"), TitleClass: "text-sm", Description: h.Text("x")})
	app.ExpectCount(telatest.ByRole("heading", "Styled"), 1)

	_, err := telatest.Render(kit.Card{TitleLevel: 7, Title: h.Text("x")})
	if err == nil || !strings.Contains(err.Error(), "TitleLevel is 7") {
		t.Errorf("TitleLevel 7: error %v, want it reported in dev mode", err)
	}
	app = telatest.Mount(t, kit.Card{TitleLevel: 7, Title: h.Text("Clamped")}, telatest.NoDev())
	app.ExpectAria(`- heading "Clamped" [level=6]`)
	app = telatest.Mount(t, kit.Card{TitleLevel: -1, Title: h.Text("Clamped")}, telatest.NoDev())
	app.ExpectCount(telatest.ByRole("heading", ""), 0)
}
