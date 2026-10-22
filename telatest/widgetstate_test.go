package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// pickerPage is a hand-made combobox: an input that keeps the focus, a
// toggle button that opens a list box, and an active option that the
// arrows move (aria-activedescendant).
func pickerPage() tela.Node {
	open, setOpen := tela.UseState(false)
	active, setActive := tela.UseState(-1)
	list := tela.UseElementRef()
	opts := []tela.ElementRef{tela.UseElementRef(), tela.UseElementRef(), tela.UseElementRef()}
	names := []string{"Apollo", "Gemini", "Mercury"}
	input := []tela.Child{Role("combobox"), AriaLabel("Mission"), AriaExpanded(open), Class("h-8 w-40"),
		OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == "ArrowDown" {
				setOpen.Set(true)
				setActive.Update(func(i int) int { return min(i+1, len(names)-1) })
				e.PreventDefault()
			}
		})}
	if open {
		input = append(input, AriaControls(list))
	}
	if open && active >= 0 {
		input = append(input, AriaActiveDescendant(opts[active]))
	}
	var options []tela.Child
	for i, n := range names {
		options = append(options, Div(Key(n), Ref(opts[i]), Role("option"), AriaSelected(i == active), Text(n)))
	}
	return Div(
		Input(input...),
		Button(AriaLabel("Show missions"), AriaHasPopup("listbox"), AriaExpanded(open), OnClick(func() { setOpen.Set(!open) }), Text("v")),
		Button(AriaLabel("Plain"), AriaHasPopup("true"), Text("p")),
		If(open, Div(append([]tela.Child{Ref(list), Role("listbox"), AriaLabel("Missions")}, options...)...)))
}

func TestExpectWidgetReferences(t *testing.T) {
	app := telatest.MountFunc(t, pickerPage, telatest.Size(300, 300))
	box := telatest.ByRole("combobox", "Mission")
	toggle := telatest.ByRole("button", "Show missions")
	app.Expect(toggle, telatest.HasPopup("listbox"))
	app.Expect(telatest.ByRole("button", "Plain"), telatest.HasPopup("menu"), telatest.HasPopup("true"))
	app.Expect(box, telatest.HasPopup(""), telatest.HasPopup("false"), telatest.NoActiveDescendant())
	app.Focus(box)
	app.Press("ArrowDown")
	app.Expect(box, telatest.ActiveDescendant(telatest.ByRole("option", "Apollo")),
		telatest.Controls(telatest.ByRole("listbox", "Missions")), telatest.Focused(true))
	app.Press("ArrowDown")
	app.Expect(box, telatest.ActiveDescendant(telatest.ByRole("option", "Gemini")))
	app.Click(toggle) // closes: no reference any more
	app.Expect(box, telatest.NoActiveDescendant())
}

func TestExpectWidgetReferenceFailures(t *testing.T) {
	mount := func(tb testing.TB) *telatest.App {
		app := telatest.MountFunc(tb, pickerPage, telatest.Size(300, 300), telatest.Timeout(100*time.Millisecond))
		app.Focus(telatest.ByRole("combobox", "Mission"))
		app.Press("ArrowDown")
		return app
	}
	box := telatest.ByRole("combobox", "Mission")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(box, telatest.ActiveDescendant(telatest.ByRole("option", "Gemini")))
	}, `expected combobox "Mission" to have activedescendant=getByRole("option", name="Gemini"), but it has activedescendant=option "Apollo"`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(box, telatest.ActiveDescendant(telatest.ByRole("option", "Zeus")))
	}, `activedescendant=option "Apollo" (and getByRole("option", name="Zeus") matches no element)`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(box, telatest.ActiveDescendant(telatest.ByRole("option", "")))
	}, "ActiveDescendant(", "strict mode violation")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(box, telatest.ActiveDescendant(telatest.Locator{}))
	}, "telatest.NoActiveDescendant()")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(box, telatest.NoActiveDescendant())
	}, `to have no activedescendant, but it has activedescendant=option "Apollo"`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("button", "Show missions"), telatest.Controls(telatest.ByRole("listbox", "Missions")))
	}, `controls=none`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("button", "Show missions"), telatest.HasPopup("dialog"))
	}, `haspopup="dialog", but it has haspopup="listbox"`)
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), `HasPopup("popup")`) {
			t.Errorf("HasPopup with an unknown kind: %v", r)
		}
	}()
	telatest.HasPopup("popup")
}
