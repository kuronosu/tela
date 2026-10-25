package kit_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// What the tests of the overlays (Dialog, AlertDialog, Popover and Tooltip)
// share.

// staticClasses turns the classes of shadcn's overlay content into those of
// its static rendering in a Chromium cell: the animation states
// (data-[state=...], data-[side=...]) and the transform origin that reads
// a Radix variable are left out, as the screenshots show the settled
// overlay, and so are the classes that fix the content to the window
// (a dialog adds relative, which positions its close button).
func staticClasses(class string) string {
	drop := map[string]bool{
		"fixed": true, "top-[50%]": true, "left-[50%]": true, "z-50": true,
		"translate-x-[-50%]": true, "translate-y-[-50%]": true,
	}
	var out []string
	for _, c := range strings.Fields(class) {
		if drop[c] || strings.HasPrefix(c, "data-[state=") || strings.HasPrefix(c, "data-[side=") || strings.HasPrefix(c, "origin-(") {
			continue
		}
		out = append(out, c)
	}
	return strings.Join(out, " ")
}

// pageUnder is a page for an overlay to open over: a heading, text and a
// button, so that the scrim and the inert page are visible.
func pageUnder(over ...tela.Child) func() tela.Node {
	return func() tela.Node {
		kids := []tela.Child{h.Class("flex flex-col items-start gap-4 p-6")}
		kids = append(kids,
			h.H1(h.Class("text-2xl font-semibold"), h.Text("Project settings")),
			h.P(h.Class("text-sm text-muted-foreground"), h.Text("Manage the name, the region and the members of your project.")),
			kit.Button{Variant: kit.Secondary, Children: h.Text("Save changes")})
		return h.Div(append(kids, over...)...)
	}
}

// openGolden checks the window with an overlay open: the whole window in
// light and dark at scales 1 and 2, testdata/<name>_open_<scheme>_x<scale>.png.
// open is called with the mounted app, to open the overlay when it is not
// open by default.
func openGolden(t *testing.T, name string, w, hgt float32, page func() tela.Node, open func(*telatest.App)) {
	t.Helper()
	for _, scheme := range []string{"light", "dark"} {
		opts := []telatest.Option{telatest.Size(w, hgt), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground")}
		if scheme == "dark" {
			opts = append(opts, telatest.Dark())
		}
		app := telatest.MountFunc(t, page, opts...)
		if open != nil {
			open(app)
		}
		app.GoldenScales(fmt.Sprintf("%s_open_%s", name, scheme), 1, 2)
	}
}

// expectIdle checks that the window asks for no frames once the entrance has
// ended: nothing is animating, no timer runs, and the floating element
// follows its anchor without frames of its own.
func expectIdle(t *testing.T, page func() tela.Node, open func(*telatest.App)) {
	t.Helper()
	app := telatest.MountFunc(t, page, telatest.NoSettle(), telatest.Theme(kit.Theme()))
	if open != nil {
		open(app)
	}
	app.Advance(3 * time.Second)
	before := app.Stats()
	app.Advance(2 * time.Second)
	after := app.Stats()
	if after.Frames != before.Frames {
		t.Errorf("an idle open overlay still requests frames: %d frames in 2 s (%+v then %+v)", after.Frames-before.Frames, before, after)
	}
}
