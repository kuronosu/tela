package main

import (
	"fmt"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the text fields and of Field, which ties a label, a control,
// its help text and its error together.

func textareaPage() tela.Node {
	msg, setMsg := tela.UseState("")
	const limit = 140
	return grid(
		demo("Basic", "It grows with its text from a minimum height of 64 px.", col,
			kit.Textarea{Class: "max-w-sm", Placeholder: "Type your message here.", AriaLabel: "Message"}),
		demo("With a label and a button", "", col,
			Div(Class("flex w-full max-w-sm flex-col gap-3"),
				kit.Field{Label: "Your message", Description: "Your message will be copied to the support team.",
					Children: kit.Textarea{Placeholder: "Type your message here."}},
				kit.Button{Children: Text("Send message")})),
		demo("Counter", fmt.Sprintf("MaxLength stops typing at %d characters; the count is the app's.", limit), col,
			kit.Textarea{Class: "max-w-sm", Value: &msg, OnValueChange: setMsg.Set, MaxLength: limit, Rows: 3,
				Placeholder: "What are you working on?", AriaLabel: "Status"},
			readout(fmt.Sprintf("%d / %d", utf8.RuneCountInString(msg), limit))),
		demo("States", "Disabled and invalid.", col,
			kit.Textarea{Class: "max-w-sm", Disabled: true, Placeholder: "Disabled", AriaLabel: "Disabled"},
			kit.Textarea{Class: "max-w-sm", Invalid: true, DefaultValue: "Too short.", AriaLabel: "Invalid"}),
	)
}

func fieldSetPage() tela.Node {
	plan, setPlan := tela.UseState("pro")
	return grid(
		demo("Fields under a legend", "A FieldSet groups fields under a legend and a description; the group is named by the legend.", col,
			kit.FieldSet{Class: "max-w-sm", Legend: "Address", Description: "Where we send your invoices.",
				Children: Fragment(
					kit.Field{Label: "Street", Children: kit.Input{Placeholder: "123 Main St"}},
					kit.Field{Label: "City", Children: kit.Input{Placeholder: "Springfield"}},
				)}),
		demo("A radio group", "A RadioGroup or ToggleGroup in a FieldSet takes its name from the legend: no AriaLabel needed.", col,
			kit.FieldSet{Class: "max-w-sm", Legend: "Plan", Description: "Choose how you are billed.",
				Children: kit.RadioGroup[string]{Value: &plan, OnValueChange: setPlan.Set, Items: []kit.RadioItem[string]{
					{Value: "free", Label: "Free"}, {Value: "pro", Label: "Pro"}, {Value: "team", Label: "Team"}}}}),
		demo("Label variant", "LegendVariantLabel draws the legend the size of a field's label, for a group of settings rows.", col,
			kit.FieldSet{Class: "max-w-sm", Legend: "Notifications", LegendVariant: kit.LegendVariantLabel,
				Children: Fragment(
					kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center", Label: "Push notifications",
						Children: kit.Switch{DefaultChecked: true}},
					kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center", Label: "Email digest",
						Description: "Once a week.", Children: kit.Switch{}},
				)}),
	)
}
