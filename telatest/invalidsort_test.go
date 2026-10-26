package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// formPage has an email field the app validates (AriaInvalid), one whose
// own type makes it invalid, and a sortable column header (ADR-131).
func formPage() tela.Node {
	email, setEmail := tela.UseState("")
	order, setOrder := tela.UseState("none")
	return Div(Class("flex flex-col gap-2"),
		Input(AriaLabel("Email"), Bind(email, setEmail), AriaInvalid(email != "" && !strings.Contains(email, "@"))),
		Input(AriaLabel("Website"), Type("url"), DefaultValue("not a url")),
		Div(Role("row"),
			Button(Role("columnheader"), AriaSort(order), OnClick(func() {
				setOrder.Update(func(o string) string {
					if o == "ascending" {
						return "descending"
					}
					return "ascending"
				})
			}), Text("Name"))),
	)
}

func TestExpectInvalidAndSort(t *testing.T) {
	app := telatest.MountFunc(t, formPage, telatest.Size(400, 300))
	email := telatest.ByRole("textbox", "Email")
	header := telatest.ByRole("columnheader", "Name")
	app.Expect(email, telatest.Invalid(false))
	app.Fill(email, "ada")
	app.Expect(email, telatest.Invalid(true))
	app.Fill(email, "ada@example.com")
	app.Expect(email, telatest.Invalid(false))
	// The field's own validity, without AriaInvalid.
	app.Expect(telatest.ByRole("textbox", "Website"), telatest.Invalid(true))

	app.Expect(header, telatest.Sort("none"))
	app.Click(header)
	app.Expect(header, telatest.Sort("ascending"))
	app.Click(header)
	app.Expect(header, telatest.Sort("descending"))
}

// A read-only or disabled field is barred from constraint validation
// (HTML): its bad value neither matches invalid: nor makes it invalid
// for assistive technologies, while h.AriaInvalid still does.
func TestInvalidSkipsReadOnlyAndDisabledFields(t *testing.T) {
	field := func(label string, extra ...tela.Child) tela.Node {
		c := []tela.Child{AriaLabel(label), Type("url"), DefaultValue("not a url"),
			Class("block w-40 h-8 bg-white invalid:bg-red-500")}
		return Input(append(c, extra...)...)
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("flex flex-col gap-2 p-2 bg-white"),
			field("Editable"),
			field("Read-only", ReadOnly(true)),
			field("Disabled", Disabled(true)),
			Div(Disabled(true), field("In a disabled group")),
			field("Flagged", ReadOnly(true), AriaInvalid(true)),
		)
	}, telatest.Size(300, 300))
	for label, want := range map[string]bool{
		"Editable": true, "Read-only": false, "Disabled": false, "In a disabled group": false, "Flagged": true,
	} {
		l := telatest.ByRole("textbox", label)
		app.Expect(l, telatest.Invalid(want))
		b := app.Bounds(l)
		px := app.PixelAt(b.X+b.Width-4, b.Y+b.Height/2)
		if red := px.R > 200 && px.G < 100; red != (label == "Editable") {
			t.Errorf("%s: pixel %v, invalid: applied %v", label, px, red)
		}
	}
}

func TestExpectInvalidAndSortFailures(t *testing.T) {
	mount := func(tb testing.TB) *telatest.App {
		return telatest.MountFunc(tb, formPage, telatest.Size(400, 300), telatest.Timeout(100*time.Millisecond))
	}
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("textbox", "Email"), telatest.Invalid(true))
	}, `expected textbox "Email" to have invalid=true, but it has invalid=false (from h.AriaInvalid(false))`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("columnheader", "Name"), telatest.Invalid(true))
	}, "invalid=false (h.AriaInvalid sets it)")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("textbox", "Website"), telatest.Invalid(false))
	}, "invalid=true (the field's own value is invalid; h.AriaInvalid(false) overrides it)", "its states:", "invalid=true")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("columnheader", "Name"), telatest.Sort("ascending"))
	}, `to have sort="ascending", but it has sort="none" (h.AriaSort sets it)`)
	mustFail(t, func(tb testing.TB) {
		app := mount(tb)
		app.Click(telatest.ByRole("columnheader", "Name"))
		app.Expect(telatest.ByRole("columnheader", "Name"), telatest.Sort("descending"))
	}, `but it has sort="ascending"`, `its states: `, `sort="ascending"`)
	defer func() {
		if r, _ := recover().(string); !strings.Contains(r, `Sort("up"): not an aria-sort value`) {
			t.Errorf("panic %q", r)
		}
	}()
	telatest.Sort("up")
}
