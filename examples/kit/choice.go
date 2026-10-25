package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the controls that choose: Checkbox, Switch, RadioGroup,
// Toggle and ToggleGroup.

func checkboxPage() tela.Node {
	items := []string{"Recents", "Home", "Applications", "Desktop", "Downloads", "Documents"}
	on, setOn := tela.UseState(map[string]bool{"Recents": true, "Home": true, "Desktop": true})
	all := len(on) == len(items)
	for _, it := range items {
		if !on[it] {
			all = false
		}
	}
	toggle := func(it string, v bool) {
		next := map[string]bool{}
		for k, b := range on {
			next[k] = b
		}
		if v {
			next[it] = true
		} else {
			delete(next, it)
		}
		setOn.Set(next)
	}
	setAll := func(v bool) {
		next := map[string]bool{}
		if v {
			for _, it := range items {
				next[it] = true
			}
		}
		setOn.Set(next)
	}
	var rows []tela.Child
	for _, it := range items {
		rows = append(rows, kit.Field{Props: Key(it), Orientation: kit.Horizontal, Label: it,
			Children: kit.Checkbox{Checked: ptr(on[it]), OnCheckedChange: func(v bool) { toggle(it, v) }}})
	}
	return grid(
		demo("States", "Unchecked, checked, disabled and invalid. A Label with the checkbox's ID names it.", col,
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-off"}, kit.Label{For: "cb-off", Children: Text("Unchecked")}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-on", DefaultChecked: true}, kit.Label{For: "cb-on", Children: Text("Checked")}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-dis", Disabled: true}, kit.Label{For: "cb-dis", Disabled: true, Children: Text("Disabled")}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-bad", Invalid: true}, kit.Label{For: "cb-bad", Children: Text("Invalid")}),
		),
		demo("With text", "A Field in the horizontal orientation puts the description under the label.", col,
			kit.Field{Orientation: kit.Horizontal, Class: "max-w-sm", Label: "Accept terms and conditions",
				Description: "By clicking this checkbox, you agree to the terms and conditions.",
				Children:    kit.Checkbox{DefaultChecked: true}}),
		demo("A list", "The first row selects every item; its state follows the others (there is no indeterminate state yet).", col,
			Div(Class("flex w-64 flex-col gap-3"),
				kit.Field{Orientation: kit.Horizontal, Label: "Select all", Class: "font-medium",
					Children: kit.Checkbox{Checked: &all, OnCheckedChange: setAll}},
				kit.Separator{},
				Fragment(rows...)),
			readout(fmt.Sprintf("%d of %d selected", len(on), len(items)))),
	)
}

// ptr is a pointer to a copy of v: the controlled value of a row.
func ptr[T any](v T) *T { return &v }

func switchPage() tela.Node {
	wifi, setWifi := tela.UseState(true)
	return grid(
		demo("Basic", "A Label with the switch's ID names it and toggles it when clicked.", "",
			Div(Class("flex items-center gap-2"),
				kit.Switch{ID: "airplane"}, kit.Label{For: "airplane", Children: Text("Airplane mode")})),
		demo("Sizes and states", "The default size, Sm, on, disabled, disabled and on.", "",
			kit.Switch{AriaLabel: "Default"}, kit.Switch{Size: kit.Sm, AriaLabel: "Small"},
			kit.Switch{DefaultChecked: true, AriaLabel: "On"}, kit.Switch{Disabled: true, AriaLabel: "Disabled"},
			kit.Switch{Disabled: true, DefaultChecked: true, AriaLabel: "Disabled and on"}),
		demo("Controlled", "Checked points to your state.", "",
			Div(Class("flex items-center gap-2"),
				kit.Switch{ID: "wifi", Checked: &wifi, OnCheckedChange: setWifi.Set},
				kit.Label{For: "wifi", Children: Text("Wi-Fi")}),
			readout(map[bool]string{true: "Connected to Home-5G", false: "Not connected"}[wifi])),
		demo("Settings rows", "A Field with flex-row-reverse puts the switch at the end of the row.", col,
			kit.Card{Class: "w-full max-w-md", TitleLevel: 2, Title: Text("Notifications"), Description: Text("Choose what you want to be told about."),
				Children: Div(Class("flex flex-col gap-5"),
					switchRow("Comments", "When someone replies to you.", true),
					switchRow("Mentions", "When someone @mentions you.", true),
					switchRow("Weekly digest", "A summary of the week, on Mondays.", false))}),
	)
}

func switchRow(title, hint string, on bool) tela.Node {
	return Fragment(kit.Field{Props: Key(title), Orientation: kit.Horizontal, Label: title, Description: hint,
		Class: "flex-row-reverse items-center justify-between", Children: kit.Switch{DefaultChecked: on}})
}

func radioGroupPage() tela.Node {
	plan, setPlan := tela.UseState("pro")
	size, setSize := tela.UseState("m")
	return grid(
		demo("Default", "Arrow keys move and check, Tab enters at the checked option. Label and Description are each option's text.", col,
			kit.RadioGroup[string]{AriaLabel: "Plan", Value: &plan, OnValueChange: setPlan.Set, Items: []kit.RadioItem[string]{
				{Value: "free", Label: "Free", Description: "For personal projects."},
				{Value: "pro", Label: "Pro", Description: "For small teams, with support."},
				{Value: "team", Label: "Team", Description: "Unlimited seats and SSO.", Disabled: true},
			}},
			readout("Plan: "+plan)),
		demo("Horizontal", "Orientation Horizontal also changes which arrow keys it listens to.", "",
			kit.RadioGroup[string]{AriaLabel: "T-shirt size", Orientation: kit.Horizontal, Class: "gap-6", Value: &size, OnValueChange: setSize.Set,
				Items: []kit.RadioItem[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}}}),
		demo("Custom options", "Render draws each option; the group keeps the behavior. Props go on the one button, Indicator is the familiar mark.", "",
			kit.RadioGroup[string]{AriaLabel: "Billing", DefaultValue: ptr("yearly"), Orientation: kit.Horizontal, Class: "gap-3", Render: billingCard,
				Items: []kit.RadioItem[string]{
					{Value: "monthly", Label: "Monthly", Description: "$12 / month"},
					{Value: "yearly", Label: "Yearly", Description: "$120 / year"},
				}}),
		demo("Invalid and disabled", "", "",
			kit.RadioGroup[string]{AriaLabel: "Required", Invalid: true, Items: []kit.RadioItem[string]{{Value: "a", Label: "Option A"}, {Value: "b", Label: "Option B"}}},
			kit.RadioGroup[string]{AriaLabel: "Locked", Disabled: true, DefaultValue: ptr("a"), Items: []kit.RadioItem[string]{{Value: "a", Label: "Option A"}, {Value: "b", Label: "Option B"}}}),
	)
}

// billingCard is one option of the custom RadioGroup: a card that takes the
// accent border when checked.
func billingCard(o kit.RadioItem[string], st kit.RadioState) tela.Node {
	return Button(Key(o.Value), st.Props,
		Class("flex w-44 items-start gap-3 rounded-lg border border-border bg-card p-4 text-left text-sm text-card-foreground shadow-xs outline-none transition-colors",
			"hover:bg-accent focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
			"aria-checked:border-primary aria-checked:bg-accent/50"),
		Span(Class("mt-0.5"), st.Indicator),
		Div(Class("flex flex-col gap-0.5"), Span(Class("font-medium"), Text(o.Label)), Span(Class("text-muted-foreground"), Text(o.Description))))
}

func togglePage() tela.Node {
	bold, setBold := tela.UseState(false)
	return grid(
		demo("Variants", "Default and Outline, with an icon, a label or both.", "",
			kit.Toggle{Icon: icons.Bold, AriaLabel: "Toggle bold"},
			kit.Toggle{Variant: kit.Outline, Icon: icons.Italic, AriaLabel: "Toggle italic"},
			kit.Toggle{Variant: kit.Outline, DefaultPressed: true, Icon: icons.Italic, Children: Text("Italic")},
			kit.Toggle{Variant: kit.Outline, Children: Text("Text only")}),
		demo("Sizes", "Sm, the default and Lg.", "",
			kit.Toggle{Variant: kit.Outline, Size: kit.Sm, Icon: icons.Underline, AriaLabel: "Underline, small"},
			kit.Toggle{Variant: kit.Outline, Icon: icons.Underline, AriaLabel: "Underline"},
			kit.Toggle{Variant: kit.Outline, Size: kit.Lg, Icon: icons.Underline, AriaLabel: "Underline, large"}),
		demo("Disabled", "", "",
			kit.Toggle{Disabled: true, Icon: icons.Bold, AriaLabel: "Bold, disabled"},
			kit.Toggle{Variant: kit.Outline, Disabled: true, DefaultPressed: true, Icon: icons.Bold, AriaLabel: "Bold, disabled and pressed"}),
		demo("Controlled", "Pressed points to your state.", "",
			kit.Toggle{Variant: kit.Outline, Pressed: &bold, OnPressedChange: setBold.Set, Icon: icons.Bold, Children: Text("Bold")},
			Span(Class("text-sm font-medium", map[bool]string{true: "font-bold", false: "font-normal"}[bold]), Text("The quick brown fox"))),
	)
}

func toggleGroupPage() tela.Node {
	view, setView := tela.UseState("list")
	align, setAlign := tela.UseState("start")
	return grid(
		demo("Segmented control", "One choice, always one on (DisallowEmptySelection). The group is one tab stop; the arrows move through it.", col,
			kit.ToggleGroup[string]{AriaLabel: "View", Variant: kit.Outline, DisallowEmptySelection: true, Value: &view, OnValueChange: setView.Set,
				Items: []kit.ToggleGroupItem[string]{
					{Value: "list", Label: "List", Icon: icons.List},
					{Value: "grid", Label: "Grid", Icon: icons.LayoutGrid},
				}},
			readout("View: "+view)),
		demo("Multiple", "Any number on: the aria.Selection of the group is Selected, DefaultSelected and OnSelectionChange.", "",
			kit.ToggleGroup[string]{AriaLabel: "Text formatting", Multiple: true, Variant: kit.Outline, DefaultSelected: aria.SelectionOf("bold", "italic"),
				Items: []kit.ToggleGroupItem[string]{
					{Value: "bold", Icon: icons.Bold, AriaLabel: "Toggle bold"},
					{Value: "italic", Icon: icons.Italic, AriaLabel: "Toggle italic"},
					{Value: "underline", Icon: icons.Underline, AriaLabel: "Toggle underline"},
				}}),
		demo("Alignment", "Icons only, default variant.", col,
			kit.ToggleGroup[string]{AriaLabel: "Text alignment", DisallowEmptySelection: true, Value: &align, OnValueChange: setAlign.Set,
				Items: []kit.ToggleGroupItem[string]{
					{Value: "start", Icon: icons.TextAlignStart, AriaLabel: "Align left"},
					{Value: "center", Icon: icons.TextAlignCenter, AriaLabel: "Align center"},
					{Value: "end", Icon: icons.TextAlignEnd, AriaLabel: "Align right"},
				}},
			P(Class("w-64 text-sm", map[string]string{"start": "text-left", "center": "text-center", "end": "text-right"}[align]),
				Text("The quick brown fox jumps over the lazy dog."))),
		demo("Spacing, sizes and disabled", "Spacing 2 separates the items (4 px steps); Sm and Lg; one item disabled.", col,
			kit.ToggleGroup[string]{AriaLabel: "Spaced", Variant: kit.Outline, Spacing: 2, DefaultValue: ptr("a"),
				Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: "Top"}, {Value: "b", Label: "Bottom"}, {Value: "c", Label: "Left"}}},
			kit.ToggleGroup[string]{AriaLabel: "Small", Variant: kit.Outline, Size: kit.Sm, DefaultValue: ptr("a"),
				Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: "Day"}, {Value: "b", Label: "Week"}, {Value: "c", Label: "Month", Disabled: true}}},
			kit.ToggleGroup[string]{AriaLabel: "Large", Size: kit.Lg, DefaultValue: ptr("a"),
				Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: "One"}, {Value: "b", Label: "Two"}}}),
	)
}
