package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's RadioGroup and RadioGroupItem (new-york-v4/ui/radio-group.tsx
// at kit.ShadcnCommit), verbatim.
const (
	shadcnRadioGroup     = "grid gap-3"
	shadcnRadioItem      = "aspect-square size-4 shrink-0 rounded-full border border-input text-primary shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:bg-input/30 dark:aria-invalid:ring-destructive/40"
	shadcnRadioIndicator = "relative flex items-center justify-center"
	shadcnRadioDot       = "absolute top-1/2 left-1/2 size-2 -translate-x-1/2 -translate-y-1/2 fill-primary"
)

// radioHTML is the markup of shadcn's RadioGroupItem.
func radioHTML(id string, checked, disabled, invalid bool) string {
	inner := ""
	if checked {
		inner = fmt.Sprintf(`<span data-state="checked" data-slot="radio-group-indicator" class="%s">%s</span>`,
			shadcnRadioIndicator, svgWithClass(iconCircleTest, shadcnRadioDot))
	}
	attrs := radixAttrs("radio", checked, disabled, invalid)
	return fmt.Sprintf(`<button id=%q %s data-slot="radio-group-item" class="%s">%s</button>`, id, attrs, shadcnRadioItem, inner)
}

// iconCircleTest is Lucide's circle.
var iconCircleTest = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/></svg>`}

var plans = []kit.RadioItem[string]{{Value: "free", Label: "Free"}, {Value: "pro", Label: "Pro"}, {Value: "team", Label: "Team"}}

// rgSpec describes a radio group cell.
type rgSpec struct {
	items      []kit.RadioItem[string]
	value      string
	horizontal bool
	disabled   bool
	invalid    bool
}

func rgCell(name string, sp rgSpec) cell {
	g := kit.RadioGroup[string]{Items: sp.items, AriaLabel: name, Disabled: sp.disabled, Invalid: sp.invalid, Props: h.TestID(name + "-group")}
	if sp.value != "" {
		g.DefaultValue = &sp.value
	}
	root := shadcnRadioGroup
	if sp.horizontal {
		g.Orientation = kit.Horizontal
		root = cn(shadcnRadioGroup, "grid-flow-col auto-cols-max gap-6")
	}
	var rows strings.Builder
	for i, it := range sp.items {
		id := fmt.Sprintf("r-%s-%d", name, i)
		dis := sp.disabled || it.Disabled
		circle := radioHTML(id, it.Value == sp.value, dis, sp.invalid)
		if it.Description == "" {
			fmt.Fprintf(&rows, `<div class="flex items-center gap-3">%s<label for=%q class="%s">%s</label></div>`, circle, id, shadcnLabel, it.Label)
			continue
		}
		fmt.Fprintf(&rows, `<div role="group" data-slot="field" data-orientation="horizontal" class="%s">%s<div data-slot="field-content" class="%s"><label data-slot="field-label" for=%q class="%s">%s</label><p data-slot="field-description" class="%s">%s</p></div></div>`,
			cn(shadcnFieldHorizontal), strings.Replace(circle, `class="`, `class="peer `, 1), shadcnFieldContent+" peer-disabled:opacity-50", id, cn(shadcnLabel, shadcnFieldLabel), it.Label, cn(shadcnFieldDesc), it.Description)
	}
	attrs := ` aria-required="false" aria-orientation="vertical"`
	if sp.horizontal {
		attrs = ` aria-required="false" aria-orientation="horizontal"`
	}
	return cell{name: name, width: 300, probes: []string{name + "-group"}, tela: g,
		html: fmt.Sprintf(`<div role="radiogroup"%s dir="ltr" data-slot="radio-group" data-probe="%s-group" class="%s">%s</div>`, attrs, name, root, rows.String())}
}

// radioCardCell is a group whose options are cards, drawn by Render.
func radioCardCell(name string) cell {
	themes := []kit.RadioItem[string]{{Value: "light", Label: "Light"}, {Value: "dark", Label: "Dark"}}
	value := "dark"
	const card = "flex w-32 flex-col items-start gap-3 rounded-lg border border-border p-4 text-left text-sm transition-colors hover:bg-accent aria-checked:border-primary aria-checked:bg-primary/5"
	g := kit.RadioGroup[string]{Items: themes, AriaLabel: name, DefaultValue: &value, Orientation: kit.Horizontal, Props: h.TestID(name + "-group"),
		Render: func(it kit.RadioItem[string], st kit.RadioState) tela.Node {
			return h.Button(st.Props, h.Class(card), st.Indicator, h.Span(h.Class("font-medium"), h.Text(it.Label)))
		}}
	var rows strings.Builder
	for i, it := range themes {
		checked := it.Value == value
		dot := ""
		if checked {
			dot = fmt.Sprintf(`<span class="%s">%s</span>`, shadcnRadioIndicator, svgWithClass(iconCircleTest, shadcnRadioDot))
		}
		state := map[bool]string{true: "checked", false: "unchecked"}[checked]
		fmt.Fprintf(&rows, `<button type="button" role="radio" aria-checked="%t" data-state=%q id="r-%s-%d" class="%s"><span aria-hidden="true" class="inline-flex items-center justify-center %s">%s</span><span class="font-medium">%s</span></button>`,
			checked, state, name, i, strings.ReplaceAll(card, "aria-checked:", "data-[state=checked]:"), shadcnRadioItem, dot, it.Label)
	}
	return cell{name: name, width: 300, probes: []string{name + "-group"}, tela: g,
		html: fmt.Sprintf(`<div role="radiogroup" aria-orientation="horizontal" data-probe="%s-group" class="%s">%s</div>`, name, cn(shadcnRadioGroup, "grid-flow-col auto-cols-max gap-6"), rows.String())}
}

func radioGroupSheet() *sheet {
	described := []kit.RadioItem[string]{
		{Value: "free", Label: "Free", Description: "For personal projects."},
		{Value: "pro", Label: "Pro", Description: "For small teams."},
	}
	withDisabled := []kit.RadioItem[string]{{Value: "a", Label: "Comfortable"}, {Value: "b", Label: "Compact"}, {Value: "c", Label: "Spacious", Disabled: true}}
	return &sheet{name: "radiogroup", slot: 320, cells: []cell{
		rgCell("basic", rgSpec{items: plans, value: "pro"}),
		rgCell("none", rgSpec{items: plans}),
		rgCell("disabled-item", rgSpec{items: withDisabled, value: "b"}),
		rgCell("disabled", rgSpec{items: plans, value: "free", disabled: true}),
		rgCell("invalid", rgSpec{items: plans, value: "free", invalid: true}),
		rgCell("horizontal", rgSpec{items: plans[:2], value: "free", horizontal: true}),
		rgCell("description", rgSpec{items: described, value: "pro"}),
		radioCardCell("cards"),
	}}
}

func TestRadioGroupGolden(t *testing.T)   { radioGroupSheet().golden(t) }
func TestChromiumRadioGroup(t *testing.T) { radioGroupSheet().chromium(t) }

func rgRadio(name string) telatest.Locator { return telatest.ByRole("radio", name) }

func TestRadioGroupUncontrolled(t *testing.T) {
	var got []string
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.RadioGroup[string]{Items: plans, AriaLabel: "Plan", DefaultValue: new("pro"),
			OnValueChange: func(k string) { got = append(got, k) }})
	})
	app.Expect(rgRadio("Pro"), telatest.Checked(true))
	app.Expect(rgRadio("Free"), telatest.Checked(false))
	app.Click(rgRadio("Team"))
	app.Expect(rgRadio("Team"), telatest.Checked(true))
	app.Expect(rgRadio("Pro"), telatest.Checked(false))
	// The label checks its radio.
	app.Click(telatest.ByText("Free"))
	app.Expect(rgRadio("Free"), telatest.Checked(true))
	if fmt.Sprint(got) != "[team free]" {
		t.Errorf("OnValueChange got %v, want [team free]", got)
	}
}

func TestRadioGroupNothingCheckedAndKeyboard(t *testing.T) {
	items := []kit.RadioItem[string]{{Value: "a", Label: "A"}, {Value: "b", Label: "B", Disabled: true}, {Value: "c", Label: "C"}}
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.RadioGroup[string]{Items: items, AriaLabel: "Letters"})
	})
	for _, n := range []string{"A", "B", "C"} {
		app.Expect(rgRadio(n), telatest.Checked(false))
	}
	// Tab enters the group at the first enabled radio; an arrow moves the
	// focus to the next enabled one and checks it, going around the ends.
	app.Press("Tab")
	app.Expect(rgRadio("A"), telatest.Focused(true))
	app.Press("ArrowDown")
	app.Expect(rgRadio("C"), telatest.Focused(true), telatest.Checked(true))
	app.Press("ArrowRight")
	app.Expect(rgRadio("A"), telatest.Focused(true), telatest.Checked(true))
	app.Expect(rgRadio("C"), telatest.Checked(false))
	app.Press("ArrowUp")
	app.Expect(rgRadio("C"), telatest.Focused(true), telatest.Checked(true))
	// A disabled radio ignores a click.
	b := app.Bounds(rgRadio("B"))
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
	app.Expect(rgRadio("B"), telatest.Checked(false), telatest.Disabled(true))
}

// controlledRadios keeps its value in the caller.
type controlledRadios struct{ refuse bool }

func (c controlledRadios) Build() tela.Node {
	plan, setPlan := tela.UseState("free")
	return h.Div(kit.RadioGroup[string]{Items: plans, AriaLabel: "Plan", Value: &plan, OnValueChange: func(k string) {
		if !c.refuse {
			setPlan.Set(k)
		}
	}}, h.Span(h.Class("sr-only"), h.Text("plan is "+plan)))
}

func TestRadioGroupControlled(t *testing.T) {
	app := telatest.Mount(t, controlledRadios{})
	app.ExpectText("plan is free")
	app.Click(rgRadio("Team"))
	app.ExpectText("plan is team")
	app.Expect(rgRadio("Team"), telatest.Checked(true))
	refused := telatest.Mount(t, controlledRadios{refuse: true})
	refused.Click(rgRadio("Team"))
	refused.Expect(rgRadio("Team"), telatest.Checked(false))
	refused.Expect(rgRadio("Free"), telatest.Checked(true))
}

func TestRadioGroupDisabledReadOnlyInvalid(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.RadioGroup[string]{Items: plans, AriaLabel: "Off", Disabled: true, DefaultValue: new("free")},
			kit.RadioGroup[string]{Items: plans, AriaLabel: "Locked", ReadOnly: true, DefaultValue: new("pro")},
			kit.RadioGroup[string]{Items: plans, AriaLabel: "Wrong", Invalid: true, Required: true})
	})
	snap := app.AriaSnapshot()
	for _, want := range []string{`radiogroup "Off" [disabled]`, `radiogroup "Wrong" [invalid]`} {
		if !strings.Contains(snap, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, snap)
		}
	}
}

func TestRadioGroupRender(t *testing.T) {
	var states []kit.RadioState
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.RadioGroup[string]{Items: plans[:2], AriaLabel: "Plan", DefaultValue: new("pro"),
			Render: func(it kit.RadioItem[string], st kit.RadioState) tela.Node {
				states = append(states, st)
				return h.Button(st.Props, h.Class("rounded border p-2 aria-checked:bg-accent"), h.Text(it.Label), st.Indicator)
			}})
	})
	app.Expect(rgRadio("Pro"), telatest.Checked(true))
	app.Click(rgRadio("Free"))
	app.Expect(rgRadio("Free"), telatest.Checked(true))
	app.Expect(rgRadio("Pro"), telatest.Checked(false))
	last := states[len(states)-2:]
	if !last[0].Checked || last[1].Checked || last[0].ID == "" {
		t.Errorf("Render states %+v, want Free checked after the click", last)
	}
}

func TestRadioGroupFieldDescribesAndMarksInvalid(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.Field{Label: "Plan", Error: "Pick a plan.", Children: kit.RadioGroup[string]{Items: plans, AriaLabel: "Plan"}})
	})
	snap := app.AriaSnapshot()
	if !strings.Contains(snap, `radiogroup "Plan" [invalid]`) {
		t.Errorf("snapshot lacks the invalid radiogroup:\n%s", snap)
	}
	if n := app.Count(telatest.ByClass("aria-invalid:border-destructive")); n != 3 {
		t.Errorf("%d radios, want 3", n)
	}
}

func ExampleRadioGroup() {
	t := &exampleT{}
	defer t.close()
	plan := "free"
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.RadioGroup[string]{
			AriaLabel: "Plan", DefaultValue: &plan, OnValueChange: func(k string) { plan = k },
			Items: []kit.RadioItem[string]{
				{Value: "free", Label: "Free"},
				{Value: "pro", Label: "Pro", Description: "For small teams."},
			}})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("radio", "Pro"))
	fmt.Println("plan", plan)
	// Output: plan pro
}
