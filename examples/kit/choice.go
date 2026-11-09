package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of the controls that choose: Checkbox, Switch, RadioGroup and
// Toggle (ToggleGroup is in toggles.go). The stages are written with tr:
// English here, Spanish and Arabic in the catalogs.

func checkboxPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	// The items are named in English (their keys) and shown in the language.
	items := []string{"Recents", "Home", "Applications", "Desktop", "Downloads", "Documents"}
	names := map[string]string{
		"Recents":      tr("checkbox.recents", "Recents"),
		"Home":         tr("checkbox.home", "Home"),
		"Applications": tr("checkbox.applications", "Applications"),
		"Desktop":      tr("checkbox.desktop", "Desktop"),
		"Downloads":    tr("checkbox.downloads", "Downloads"),
		"Documents":    tr("checkbox.documents", "Documents"),
	}
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
		rows = append(rows, kit.Field{Props: Key(it), Orientation: kit.Horizontal, Label: names[it],
			Children: kit.Checkbox{Checked: ptr(on[it]), OnCheckedChange: func(v bool) { toggle(it, v) }}})
	}
	return grid(
		demo("States", "Unchecked, checked, disabled and invalid. A Label with the checkbox's ID names it.", col,
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-off"}, kit.Label{For: "cb-off", Children: Text(tr("checkbox.unchecked", "Unchecked"))}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-on", DefaultChecked: true}, kit.Label{For: "cb-on", Children: Text(tr("checkbox.checked", "Checked"))}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-dis", Disabled: true}, kit.Label{For: "cb-dis", Disabled: true, Children: Text(tr("checkbox.disabled", "Disabled"))}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{ID: "cb-bad", Invalid: true}, kit.Label{For: "cb-bad", Children: Text(tr("checkbox.invalid", "Invalid"))}),
		),
		demo("With text", "A Field in the horizontal orientation puts the description under the label.", col,
			kit.Field{Orientation: kit.Horizontal, Class: "max-w-sm", Label: tr("checkbox.terms", "Accept terms and conditions"),
				Description: tr("checkbox.termsText", "By clicking this checkbox, you agree to the terms and conditions."),
				Children:    kit.Checkbox{DefaultChecked: true}}),
		demo("A list", "The first row selects every item; its state follows the others (there is no indeterminate state yet).", col,
			Div(Class("flex w-64 flex-col gap-3"),
				kit.Field{Orientation: kit.Horizontal, Label: tr("checkbox.all", "Select all"), Class: "font-medium",
					Children: kit.Checkbox{Checked: &all, OnCheckedChange: setAll}},
				kit.Separator{},
				Fragment(rows...)),
			readout(tr("checkbox.count", "{n} of {total} selected", "n", len(on), "total", len(items)))),
	)
}

// ptr is a pointer to a copy of v: the controlled value of a row.
func ptr[T any](v T) *T { return &v }

func switchPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	wifi, setWifi := tela.UseState(true)
	return grid(
		demo("Basic", "A Label with the switch's ID names it and toggles it when clicked.", "",
			Div(Class("flex items-center gap-2"),
				kit.Switch{ID: "airplane"}, kit.Label{For: "airplane", Children: Text(tr("switch.airplane", "Airplane mode"))})),
		demo("Sizes and states", "The default size, Sm, on, disabled, disabled and on.", "",
			kit.Switch{AriaLabel: tr("switch.default", "Default")}, kit.Switch{Size: kit.Sm, AriaLabel: tr("switch.small", "Small")},
			kit.Switch{DefaultChecked: true, AriaLabel: tr("switch.on", "On")}, kit.Switch{Disabled: true, AriaLabel: tr("switch.disabled", "Disabled")},
			kit.Switch{Disabled: true, DefaultChecked: true, AriaLabel: tr("switch.disabledOn", "Disabled and on")}),
		demo("Controlled", "Checked points to your state.", "",
			Div(Class("flex items-center gap-2"),
				kit.Switch{ID: "wifi", Checked: &wifi, OnCheckedChange: setWifi.Set},
				kit.Label{For: "wifi", Children: Text(tr("switch.wifi", "Wi-Fi"))}),
			readout(map[bool]string{true: tr("switch.connected", "Connected to Home-5G"), false: tr("switch.notConnected", "Not connected")}[wifi])),
		demo("Settings rows", "A horizontal Field with ControlAfter puts the switch at the end of the row.", col,
			kit.Card{Class: "w-full max-w-md", TitleLevel: 2, Title: Text(tr("switch.notifications", "Notifications")), Description: Text(tr("switch.notificationsText", "Choose what you want to be told about.")),
				Children: Div(Class("flex flex-col gap-5"),
					switchRow(tr("switch.comments", "Comments"), tr("switch.commentsText", "When someone replies to you."), true),
					switchRow(tr("switch.mentions", "Mentions"), tr("switch.mentionsText", "When someone @mentions you."), true),
					switchRow(tr("switch.digest", "Weekly digest"), tr("switch.digestText", "A summary of the week, on Mondays."), false))}),
	)
}

func switchRow(title, hint string, on bool) tela.Node {
	return kit.Field{Props: Key(title), Orientation: kit.Horizontal, Label: title, Description: hint,
		ControlAfter: true, Class: "items-center", Children: kit.Switch{DefaultChecked: on}}
}

func radioGroupPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	plan, setPlan := tela.UseState("pro")
	size, setSize := tela.UseState("m")
	return grid(
		demo("Default", "Arrow keys move and check, Tab enters at the checked option. Label and Description are each option's text.", col,
			kit.RadioGroup[string]{AriaLabel: tr("radio.plan", "Plan"), Value: &plan, OnValueChange: setPlan.Set, Items: []kit.RadioItem[string]{
				{Value: "free", Label: tr("radio.free", "Free"), Description: tr("radio.freeText", "For personal projects.")},
				{Value: "pro", Label: tr("radio.pro", "Pro"), Description: tr("radio.proText", "For small teams, with support.")},
				{Value: "team", Label: tr("radio.team", "Team"), Description: tr("radio.teamText", "Unlimited seats and SSO."), Disabled: true},
			}},
			readout(tr("radio.planIs", "Plan: {plan}", "plan", map[string]string{"free": tr("radio.free", "Free"), "pro": tr("radio.pro", "Pro"), "team": tr("radio.team", "Team")}[plan]))),
		demo("Horizontal", "Orientation Horizontal also changes which arrow keys it listens to.", "",
			kit.RadioGroup[string]{AriaLabel: tr("radio.shirt", "T-shirt size"), Orientation: kit.Horizontal, Class: "gap-6", Value: &size, OnValueChange: setSize.Set,
				Items: []kit.RadioItem[string]{{Value: "s", Label: tr("radio.small", "Small")}, {Value: "m", Label: tr("radio.medium", "Medium")}, {Value: "l", Label: tr("radio.large", "Large")}}}),
		demo("Custom options", "Render draws each option; the group keeps the behavior. Props go on the one button, Indicator is the familiar mark.", "",
			kit.RadioGroup[string]{AriaLabel: tr("radio.billing", "Billing"), DefaultValue: ptr("yearly"), Orientation: kit.Horizontal, Class: "gap-3", Render: billingCard,
				Items: []kit.RadioItem[string]{
					{Value: "monthly", Label: tr("radio.monthly", "Monthly"), Description: tr("radio.perMonth", "$12 / month")},
					{Value: "yearly", Label: tr("radio.yearly", "Yearly"), Description: tr("radio.perYear", "$120 / year")},
				}}),
		demo("Invalid and disabled", "", "",
			kit.RadioGroup[string]{AriaLabel: tr("radio.required", "Required"), Invalid: true, Items: []kit.RadioItem[string]{{Value: "a", Label: tr("radio.optionA", "Option A")}, {Value: "b", Label: tr("radio.optionB", "Option B")}}},
			kit.RadioGroup[string]{AriaLabel: tr("radio.locked", "Locked"), Disabled: true, DefaultValue: ptr("a"), Items: []kit.RadioItem[string]{{Value: "a", Label: tr("radio.optionA", "Option A")}, {Value: "b", Label: tr("radio.optionB", "Option B")}}}),
	)
}

// billingCard is one option of the custom RadioGroup: a card that takes the
// accent border when checked.
func billingCard(o kit.RadioItem[string], st kit.RadioState) tela.Node {
	return Button(Key(o.Value), st.Props,
		Class("flex w-44 items-start gap-3 rounded-lg border border-border bg-card p-4 text-left text-sm text-card-foreground shadow-xs outline-none transition-colors",
			"hover:bg-accent focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
			"aria-checked:border-primary aria-checked:bg-accent/50"),
		// A flex box, not an inline span: inline, the indicator would sit on
		// its baseline, which moves when the checked dot appears.
		Span(Class("mt-0.5 flex"), st.Indicator),
		Div(Class("flex flex-col gap-0.5"), Span(Class("font-medium"), Text(o.Label)), Span(Class("text-muted-foreground"), Text(o.Description))))
}

func togglePage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	bold, setBold := tela.UseState(false)
	return grid(
		demo("Variants", "Default and Outline, with an icon, a label or both.", "",
			kit.Toggle{Icon: icons.Bold, AriaLabel: tr("toggle.bold", "Toggle bold")},
			kit.Toggle{Variant: kit.Outline, Icon: icons.Italic, AriaLabel: tr("toggle.italic", "Toggle italic")},
			kit.Toggle{Variant: kit.Outline, DefaultPressed: true, Icon: icons.Italic, Children: Text(tr("toggle.italicText", "Italic"))},
			kit.Toggle{Variant: kit.Outline, Children: Text(tr("toggle.textOnly", "Text only"))}),
		demo("Sizes", "Sm, the default and Lg.", "",
			kit.Toggle{Variant: kit.Outline, Size: kit.Sm, Icon: icons.Underline, AriaLabel: tr("toggle.underlineSm", "Underline, small")},
			kit.Toggle{Variant: kit.Outline, Icon: icons.Underline, AriaLabel: tr("toggle.underline", "Underline")},
			kit.Toggle{Variant: kit.Outline, Size: kit.Lg, Icon: icons.Underline, AriaLabel: tr("toggle.underlineLg", "Underline, large")}),
		demo("Disabled", "", "",
			kit.Toggle{Disabled: true, Icon: icons.Bold, AriaLabel: tr("toggle.boldDisabled", "Bold, disabled")},
			kit.Toggle{Variant: kit.Outline, Disabled: true, DefaultPressed: true, Icon: icons.Bold, AriaLabel: tr("toggle.boldDisabledPressed", "Bold, disabled and pressed")}),
		demo("Controlled", "Pressed points to your state.", "",
			kit.Toggle{Variant: kit.Outline, Pressed: &bold, OnPressedChange: setBold.Set, Icon: icons.Bold, Children: Text(tr("toggle.boldText", "Bold"))},
			Span(Class("text-sm font-medium", map[bool]string{true: "font-bold", false: "font-normal"}[bold]), Text(tr("toggle.fox", "The quick brown fox")))),
	)
}
