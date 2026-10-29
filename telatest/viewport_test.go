package telatest_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestAppLayoutJSONOf(t *testing.T) {
	app := telatest.MountFunc(t, Responsive, telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Clicked 0"))
	type box struct {
		Kind     string `json:"kind"`
		Text     string `json:"text"`
		Rect     any    `json:"rect"`
		Children []box  `json:"children"`
	}
	var whole, sub box
	if err := json.Unmarshal(app.LayoutJSON(), &whole); err != nil {
		t.Fatal(err)
	}
	js := app.LayoutJSONOf(telatest.ByRole("button", "Clicked 1"))
	if !strings.HasSuffix(string(js), "}\n") || !strings.HasPrefix(string(js), "{\n  \"kind\": \"button\"") {
		t.Fatalf("LayoutJSONOf is not the indented JSON of the button's box:\n%.200s", js)
	}
	if err := json.Unmarshal(js, &sub); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(js), "Open menu") || !strings.Contains(string(js), "Clicked 1") {
		t.Errorf("the subtree is not the button's alone:\n%s", js)
	}
	// It is the node the whole tree has at that place, rect included.
	var same func(b box) bool
	same = func(b box) bool {
		if b.Kind == sub.Kind && fmt.Sprint(b.Rect) == fmt.Sprint(sub.Rect) && len(b.Children) == len(sub.Children) {
			return true
		}
		for _, c := range b.Children {
			if same(c) {
				return true
			}
		}
		return false
	}
	if !same(whole) {
		t.Errorf("no node of LayoutJSON matches the subtree:\n%s", js)
	}
}

func TestAppLayoutJSONOfPaintedBounds(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("p-4"), Button(Class("h-10 w-20 scale-50"), TestID("scaled"), Text("Small")))
	}, telatest.Size(200, 160))
	js := string(app.LayoutJSONOf(telatest.ByTestID("scaled")))
	if !strings.Contains(js, `"bounds"`) {
		t.Errorf("the painted bounds of a scaled box are missing:\n%s", js)
	}
}

func TestAppLayoutJSONOfFailures(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, Responsive, telatest.Size(400, 300), telatest.Timeout(50*time.Millisecond))
		app.LayoutJSONOf(telatest.ByRole("button", "Nope"))
	}, "Nope")
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, Responsive, telatest.Size(400, 300), telatest.Timeout(50*time.Millisecond))
		app.LayoutJSONOf(telatest.ByRole("button", ""))
	}, "button")
}
