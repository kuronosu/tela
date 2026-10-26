package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// navPage has a link the app marks as the current page when it is clicked,
// and a box that aria-[current=page]: paints red (ADR-171).
func navPage() tela.Node {
	current, setCurrent := tela.UseState("false")
	return Div(Class("flex flex-col gap-2"),
		Div(Class("size-10 bg-blue-500 aria-[current=page]:bg-red-500 aria-[current=step]:bg-green-500"), AriaCurrent(current)),
		Button(Role("link"), AriaCurrent(current), OnClick(func() { setCurrent.Set("page") }), Text("Docs")),
		Button(OnClick(func() { setCurrent.Set("step") }), Text("Step")),
		Button(OnClick(func() { setCurrent.Set("") }), Text("Clear")),
	)
}

func TestExpectCurrent(t *testing.T) {
	app := telatest.MountFunc(t, navPage, telatest.Size(200, 200))
	link := telatest.ByRole("link", "Docs")
	app.Expect(link, telatest.Current("false"))
	app.Click(link)
	app.Expect(link, telatest.Current("page"))
	app.Click(telatest.ByRole("button", "Step"))
	app.Expect(link, telatest.Current("step"))
	app.Click(telatest.ByRole("button", "Clear"))
	app.Expect(link, telatest.Current("false"))
	// The aria snapshot does not print it, as Playwright's.
	app.Click(link)
	if snap := app.AriaSnapshot(); strings.Contains(snap, "current") {
		t.Errorf("the snapshot prints aria-current:\n%s", snap)
	}
}

func TestAriaCurrentVariantPaints(t *testing.T) {
	app := telatest.MountFunc(t, navPage, telatest.Size(200, 200))
	colors := func() (red, green, blue bool) {
		px := app.PixelAt(20, 20)
		return px.R > 200 && px.G < 100, px.G > 150 && px.R < 100, px.B > 200 && px.R < 100
	}
	if red, green, blue := colors(); red || green || !blue {
		t.Fatalf("the box is not blue before AriaCurrent is set: %v", app.PixelAt(20, 20))
	}
	app.Click(telatest.ByRole("link", "Docs"))
	if red, _, _ := colors(); !red {
		t.Errorf("aria-[current=page]:bg-red-500 did not apply: %v", app.PixelAt(20, 20))
	}
	app.Click(telatest.ByRole("button", "Step"))
	if red, green, _ := colors(); red || !green {
		t.Errorf("aria-[current=step]: did not replace page: %v", app.PixelAt(20, 20))
	}
	app.Click(telatest.ByRole("button", "Clear"))
	if _, _, blue := colors(); !blue {
		t.Errorf("the box did not return to blue: %v", app.PixelAt(20, 20))
	}
}

func TestExpectCurrentFailures(t *testing.T) {
	mount := func(tb testing.TB) *telatest.App {
		return telatest.MountFunc(tb, navPage, telatest.Size(200, 200), telatest.Timeout(100*time.Millisecond))
	}
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("link", "Docs"), telatest.Current("page"))
	}, `to have current="page", but it has current="false" (h.AriaCurrent sets it)`)
	mustFail(t, func(tb testing.TB) {
		app := mount(tb)
		app.Click(telatest.ByRole("link", "Docs"))
		app.Expect(telatest.ByRole("link", "Docs"), telatest.Current("step"))
	}, `but it has current="page"`, `its states: `)
	defer func() {
		if r, _ := recover().(string); !strings.Contains(r, `Current("up"): not an aria-current value`) {
			t.Errorf("panic %q", r)
		}
	}()
	telatest.Current("up")
}
