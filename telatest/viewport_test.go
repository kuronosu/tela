package telatest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Responsive shows a menu button below md and a nav above, and counts
// clicks so the tests can see that state survives a resize.
func Responsive() tela.Node {
	n, setN := tela.UseState(0)
	return Div(Class("p-2"),
		Button(Class("md:hidden"), Text("Open menu")),
		Div(Role("navigation"), Class("hidden md:flex"), AriaLabel("Main"), Text("Links")),
		Button(OnClick(func() { setN.Set(n + 1) }), Textf("Clicked %d", n)),
	)
}

func TestResizeKeepsStateAndAppliesBreakpoints(t *testing.T) {
	app := telatest.MountFunc(t, Responsive, telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Clicked 0"))
	app.ExpectCount(telatest.ByRole("button", "Open menu"), 1)
	app.ExpectCount(telatest.ByRole("navigation", "Main"), 0)

	app.Resize(1024, 600)
	app.ExpectCount(telatest.ByRole("button", "Open menu"), 0)
	app.ExpectCount(telatest.ByRole("navigation", "Main"), 1)
	app.ExpectText("Clicked 1")
	if b := app.Screenshot().Bounds(); b.Dx() != 1024 || b.Dy() != 600 {
		t.Errorf("screenshot is %v after Resize(1024, 600)", b)
	}

	app.SetScale(2)
	if b := app.Screenshot().Bounds(); b.Dx() != 2048 || b.Dy() != 1200 {
		t.Errorf("screenshot is %v after SetScale(2), want 2048x1200", b)
	}
	app.ExpectText("Clicked 1")
}

func TestAppLayoutJSONShowsLiveState(t *testing.T) {
	app := telatest.MountFunc(t, Responsive, telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Clicked 0"))
	js := app.LayoutJSON()
	var tree struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(js, &tree); err != nil || tree.Kind != "root" {
		t.Fatalf("LayoutJSON is not the layout tree (%v):\n%.200s", err, js)
	}
	if !strings.Contains(string(js), "Clicked 1") {
		t.Errorf("LayoutJSON does not show the state after the click:\n%s", js)
	}
}
