package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's ToggleGroup and ToggleGroupItem (new-york-v4/ui/toggle-group.tsx
// at kit.ShadcnCommit), verbatim.
const (
	shadcnToggleGroup     = "group/toggle-group flex w-fit items-center gap-[--spacing(var(--gap))] rounded-md data-[spacing=default]:data-[variant=outline]:shadow-xs"
	shadcnToggleGroupItem = "w-auto min-w-0 shrink-0 px-3 focus:z-10 focus-visible:z-10"
	shadcnToggleGroupJoin = "data-[spacing=0]:rounded-none data-[spacing=0]:shadow-none data-[spacing=0]:first:rounded-l-md data-[spacing=0]:last:rounded-r-md data-[spacing=0]:data-[variant=outline]:border-l-0 data-[spacing=0]:data-[variant=outline]:first:border-l"
)

// tgItem is one item of a toggle group cell.
type tgItem struct {
	key, label string
	icon       tela.Icon
	on         bool
	disabled   bool
}

// tgSpec describes a toggle group cell.
type tgSpec struct {
	variant  kit.Variant
	size     kit.Size
	spacing  int
	multiple bool
	items    []tgItem
}

func tgCell(name string, sp tgSpec) cell {
	items := make([]kit.ToggleGroupItem[string], len(sp.items))
	var on []string
	var markup strings.Builder
	variantAttr := string(sp.variant)
	if variantAttr == "" {
		variantAttr = "default"
	}
	sizeAttr := string(sp.size)
	if sizeAttr == "" {
		sizeAttr = "default"
	}
	for i, it := range sp.items {
		items[i] = kit.ToggleGroupItem[string]{Value: it.key, Label: it.label, Icon: it.icon, Disabled: it.disabled}
		if it.label == "" {
			items[i].AriaLabel = it.key
		}
		if it.on {
			on = append(on, it.key)
		}
		inner := ""
		if it.icon != (tela.Icon{}) {
			inner = svgHTML(it.icon)
		}
		inner += it.label
		state := "off"
		if it.on {
			state = "on"
		}
		var role string
		if sp.multiple {
			role = fmt.Sprintf(`aria-pressed="%t"`, it.on)
		} else {
			role = fmt.Sprintf(`role="radio" aria-checked="%t"`, it.on)
		}
		dis := ""
		if it.disabled {
			dis = ` disabled data-disabled=""`
		}
		cls := cn(toggleClasses(sp.variant, sp.size), shadcnToggleGroupItem, shadcnToggleGroupJoin)
		fmt.Fprintf(&markup, `<button type="button" %s data-state=%q%s data-variant=%q data-size=%q data-spacing="%d" data-slot="toggle-group-item" class="%s">%s</button>`,
			role, state, dis, variantAttr, sizeAttr, sp.spacing, cls, inner)
	}
	g := kit.ToggleGroup[string]{Items: items, Variant: sp.variant, Size: sp.size, Spacing: sp.spacing,
		Multiple: sp.multiple, AriaLabel: name, Props: h.TestID(name + "-group")}
	if len(on) > 0 {
		if sp.multiple {
			g.DefaultSelected = aria.SelectionOf(on...)
		} else {
			g.DefaultValue = &on[0]
		}
	}
	return cell{name: name, probes: []string{name + "-group"}, tela: g,
		html: fmt.Sprintf(`<div role="group" dir="ltr" data-slot="toggle-group" data-variant=%q data-size=%q data-spacing="%d" style="--gap: %d" data-probe="%s-group" class="%s">%s</div>`,
			variantAttr, sizeAttr, sp.spacing, sp.spacing, name, cn(shadcnToggleGroup), markup.String())}
}

func toggleGroupSheet() *sheet {
	formats := []tgItem{{key: "Bold", icon: iconBold}, {key: "Italic", icon: iconItalic}, {key: "Underline", icon: iconUnderline}}
	on := func(items []tgItem, i int) []tgItem {
		out := append([]tgItem(nil), items...)
		out[i].on = true
		return out
	}
	tgViews := []tgItem{{key: "list", label: "List"}, {key: "grid", label: "Grid"}, {key: "map", label: "Map"}}
	return &sheet{name: "togglegroup", slot: 280, cells: []cell{
		tgCell("default", tgSpec{items: on(formats, 1)}),
		tgCell("outline", tgSpec{variant: kit.Outline, items: on(formats, 0)}),
		tgCell("multiple", tgSpec{variant: kit.Outline, multiple: true, items: []tgItem{
			{key: "Bold", icon: iconBold, on: true}, {key: "Italic", icon: iconItalic}, {key: "Underline", icon: iconUnderline, on: true}}}),
		tgCell("text", tgSpec{variant: kit.Outline, items: on(tgViews, 0)}),
		tgCell("text-default", tgSpec{items: on(tgViews, 1)}),
		tgCell("sm", tgSpec{variant: kit.Outline, size: kit.Sm, items: on(formats, 2)}),
		tgCell("lg", tgSpec{variant: kit.Outline, size: kit.Lg, items: on(formats, 1)}),
		tgCell("spacing", tgSpec{variant: kit.Outline, spacing: 2, items: on(formats, 0)}),
		tgCell("spacing-default", tgSpec{spacing: 1, items: on(tgViews, 2)}),
		tgCell("disabled-item", tgSpec{variant: kit.Outline, items: []tgItem{
			{key: "Bold", icon: iconBold, on: true}, {key: "Italic", icon: iconItalic, disabled: true}, {key: "Underline", icon: iconUnderline}}}),
		tgCell("two", tgSpec{variant: kit.Outline, items: []tgItem{{key: "light", label: "Light", on: true}, {key: "dark", label: "Dark"}}}),
	}}
}

func TestToggleGroupGolden(t *testing.T)   { toggleGroupSheet().golden(t) }
func TestChromiumToggleGroup(t *testing.T) { toggleGroupSheet().chromium(t) }

var tgViews = []kit.ToggleGroupItem[string]{
	{Value: "list", Label: "List"}, {Value: "grid", Label: "Grid"}, {Value: "map", Label: "Map", Disabled: true}, {Value: "table", Label: "Table"},
}

func tgRadio(name string) telatest.Locator { return telatest.ByRole("radio", name) }

func TestToggleGroupSingleUncontrolled(t *testing.T) {
	var got []string
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.ToggleGroup[string]{Items: tgViews, AriaLabel: "View", DefaultValue: new("list"),
			OnValueChange: func(k string) { got = append(got, k) }})
	})
	app.Expect(tgRadio("List"), telatest.Checked(true))
	app.Expect(tgRadio("Grid"), telatest.Checked(false))
	app.Click(tgRadio("Grid"))
	app.Expect(tgRadio("Grid"), telatest.Checked(true))
	app.Expect(tgRadio("List"), telatest.Checked(false))
	// Pressing the item that is on turns it off (nothing disallows empty).
	app.Click(tgRadio("Grid"))
	app.Expect(tgRadio("Grid"), telatest.Checked(false))
	if fmt.Sprint(got) != "[grid]" {
		t.Errorf("OnValueChange got %v, want [grid] (it reports the key turned on)", got)
	}
}

func TestToggleGroupDisallowEmptyAndKeyboard(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.ToggleGroup[string]{Items: tgViews, AriaLabel: "View", DefaultValue: new("list"), DisallowEmptySelection: true})
	})
	app.Click(tgRadio("List"))
	app.Expect(tgRadio("List"), telatest.Checked(true))
	// One tab stop: the arrows move the focus without selecting, skipping
	// the disabled item, wrapping around.
	app.Focus(tgRadio("List"))
	app.Press("ArrowRight")
	app.Expect(tgRadio("Grid"), telatest.Focused(true), telatest.Checked(false))
	app.Press("ArrowRight")
	app.Expect(tgRadio("Table"), telatest.Focused(true))
	app.Press("ArrowRight")
	app.Expect(tgRadio("List"), telatest.Focused(true))
	app.Press("End")
	app.Expect(tgRadio("Table"), telatest.Focused(true))
	app.Press("Home")
	app.Press("ArrowRight")
	app.Press("Space")
	app.Expect(tgRadio("Grid"), telatest.Checked(true))
	app.Press("Enter")
	app.Expect(tgRadio("Grid"), telatest.Checked(true)) // the only item on stays on
}

// controlledGroup keeps its value in the caller.
type controlledGroup struct{}

func (controlledGroup) Build() tela.Node {
	view, setView := tela.UseState("grid")
	return h.Div(kit.ToggleGroup[string]{Items: tgViews, AriaLabel: "View", Value: &view, OnValueChange: setView.Set, DisallowEmptySelection: true},
		h.Span(h.Class("sr-only"), h.Text("view is "+view)))
}

func TestToggleGroupControlled(t *testing.T) {
	app := telatest.Mount(t, controlledGroup{})
	app.ExpectText("view is grid")
	app.Expect(tgRadio("Grid"), telatest.Checked(true))
	app.Click(tgRadio("Table"))
	app.ExpectText("view is table")
	app.Expect(tgRadio("Table"), telatest.Checked(true))
	app.Expect(tgRadio("Grid"), telatest.Checked(false))
}

// multipleGroup keeps a Selection in the caller.
type multipleGroup struct{}

func (multipleGroup) Build() tela.Node {
	sel, setSel := tela.UseState(aria.SelectionOf("bold"))
	return h.Div(kit.ToggleGroup[string]{Multiple: true, Variant: kit.Outline, AriaLabel: "Format", Selected: sel, OnSelectionChange: setSel.Set,
		Items: []kit.ToggleGroupItem[string]{
			{Value: "bold", Icon: iconBold, AriaLabel: "Bold"},
			{Value: "italic", Icon: iconItalic, AriaLabel: "Italic"},
			{Value: "underline", Icon: iconUnderline, AriaLabel: "Underline"}}},
		h.Span(h.Class("sr-only"), h.Text(fmt.Sprintf("on: %v", sel.Keys()))))
}

func TestToggleGroupMultiple(t *testing.T) {
	app := telatest.Mount(t, multipleGroup{})
	app.ExpectText("on: [bold]")
	app.Expect(toggleButton("Bold"), telatest.Pressed(true))
	app.Click(toggleButton("Underline"))
	app.ExpectText("on: [bold underline]")
	app.Click(toggleButton("Bold"))
	app.ExpectText("on: [underline]")
	app.Expect(toggleButton("Bold"), telatest.Pressed(false))
	snap := app.AriaSnapshot()
	for _, want := range []string{`toolbar "Format"`, `button "Underline" [pressed]`, `button "Italic"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, snap)
		}
	}
}

func TestToggleGroupSingleSnapshot(t *testing.T) {
	got, err := telatest.AriaSnapshot(kit.ToggleGroup[string]{Items: tgViews[:2], AriaLabel: "View", DefaultValue: new("grid")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`radiogroup "View"`, `radio "List"`, `radio "Grid" [checked]`} {
		if !strings.Contains(got, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, got)
		}
	}
}

func TestToggleGroupSpacingAndDisabled(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.ToggleGroup[string]{Items: tgViews[:2], AriaLabel: "Joined", Props: h.TestID("joined")},
			kit.ToggleGroup[string]{Items: tgViews[:2], AriaLabel: "Apart", Spacing: 2, Props: h.TestID("apart")},
			kit.ToggleGroup[string]{Items: tgViews[:2], AriaLabel: "Off", Disabled: true})
	})
	if n := app.Count(telatest.ByClass("rounded-none")); n != 4 {
		t.Errorf("%d items have square corners, want the 4 of the two joined groups", n)
	}
	if n := app.Count(telatest.ByClass("gap-2")); n < 1 {
		t.Errorf("the group with spacing has no gap")
	}
	snap := app.AriaSnapshot()
	if strings.Count(snap, "[disabled]") != 3 {
		t.Errorf("the disabled group has the wrong number of disabled items:\n%s", snap)
	}
}

func TestToggleGroupBothValueStylesPanic(t *testing.T) {
	_, err := telatest.AriaSnapshot(kit.ToggleGroup[string]{Items: tgViews, Value: new("list"), Selected: aria.SelectionOf("grid")})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("error %v, want the Value and Selected conflict reported", err)
	}
}

func ExampleToggleGroup() {
	t := &exampleT{}
	defer t.close()
	theme := "system"
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ToggleGroup[string]{
			AriaLabel: "Theme", Variant: kit.Outline, DefaultValue: &theme, DisallowEmptySelection: true,
			OnValueChange: func(k string) { theme = k },
			Items: []kit.ToggleGroupItem[string]{
				{Value: "light", Label: "Light"}, {Value: "dark", Label: "Dark"}, {Value: "system", Label: "System"},
			}})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("radio", "Dark"))
	fmt.Println("theme", theme)
	// Output: theme dark
}
