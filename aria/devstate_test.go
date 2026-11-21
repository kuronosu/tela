//go:build teladev

package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/telatest"
)

// The values of package aria that hold their state in unexported fields
// (Selection, and the state of the hooks) survive a tela dev reload when
// they are kept in hook state (ADR-113, ADR-309). These tests set the
// process-wide seed of the next mount, so they do not run in parallel. Run
// with go test -tags teladev -run DevState ./aria/.

func devReload(t *testing.T, app *telatest.App, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	var data []byte
	done := false
	core.CaptureAppDevState(func(d []byte, rep core.DevStateReport, err error) {
		if err != nil {
			t.Errorf("CaptureAppDevState: %v", err)
		}
		for _, r := range rep.Reset {
			if !r.Silent {
				t.Errorf("a slot resets with a warning: %s #%d of %s: %s", r.Hook, r.Index, r.Component, r.Reason)
			}
		}
		data, done = d, true
	})
	app.WaitFor("the dev state", func() bool { return done })
	app.Close()
	core.SetDevSeed(data)
	return telatest.Mount(t, root, opts...)
}

// A Selection in the app's own UseState is kept: it was the known limit of
// ADR-113 (a Tela type with unexported fields reset silently).
func TestDevStateKeepsASelectionInState(t *testing.T) {
	root := tela.C(func() tela.Node {
		sel, setSel := tela.UseState(aria.SelectionOf("b"))
		return Div(
			Button(OnClick(func() { setSel.Set(aria.SelectionOf("a", "c")) }), Text("Pick")),
			Span(Textf("selected: %s", strings.Join(sel.Keys(), ","))),
		)
	})
	app := telatest.Mount(t, root)
	app.Click(telatest.ByRole("button", "Pick"))
	app.ExpectText("selected: a,c")
	app = devReload(t, app, root)
	app.ExpectText("selected: a,c")
}

// A list box's uncontrolled selection and its focused item are the state of
// its collection hook (a UseOnce store), kept by its codec.
func TestDevStateKeepsAListBoxSelection(t *testing.T) {
	root := tela.C(listPage(aria.ListBoxOptions[string]{SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("banana")}, &events{}))
	size := telatest.Size(300, 400)
	app := telatest.Mount(t, root, size)
	app.Click(option("Cherry"))
	app.Expect(option("Cherry"), telatest.Selected(true))
	app = devReload(t, app, root, size)
	app.Expect(option("Banana"), telatest.Selected(true))
	app.Expect(option("Cherry"), telatest.Selected(true))
	app.Expect(option("Apple"), telatest.Selected(false))
	// The focused item is the collection's too: the arrows go on from it.
	app.ExpectFocused(option("Cherry"))
	app.Press("ArrowDown")
	app.ExpectFocused(option("Date"))
}
