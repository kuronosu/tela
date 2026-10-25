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
