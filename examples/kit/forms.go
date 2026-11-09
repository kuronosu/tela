package main

import (
	"fmt"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of the text fields and of Field, which ties a label, a control,
// its help text and its error together. The stages are written with tr:
// English here, Spanish and Arabic in the catalogs.

func textareaPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	msg, setMsg := tela.UseState("")
	const limit = 140
	return grid(
		demo("Basic", "It grows with its text from a minimum height of 64 px.", col,
			kit.Textarea{Class: "max-w-sm", Placeholder: tr("textarea.placeholder", "Type your message here."), AriaLabel: tr("textarea.message", "Message")}),
		demo("With a label and a button", "", col,
			Div(Class("flex w-full max-w-sm flex-col gap-3"),
				kit.Field{Label: tr("textarea.yourMessage", "Your message"), Description: tr("textarea.copied", "Your message will be copied to the support team."),
					Children: kit.Textarea{Placeholder: tr("textarea.placeholder", "Type your message here.")}},
				kit.Button{Children: Text(tr("textarea.send", "Send message"))})),
		demo("Counter", fmt.Sprintf("MaxLength stops typing at %d characters; the count is the app's.", limit), col,
			kit.Textarea{Class: "max-w-sm", Value: &msg, OnValueChange: setMsg.Set, MaxLength: limit, Rows: 3,
				Placeholder: tr("textarea.working", "What are you working on?"), AriaLabel: tr("textarea.status", "Status")},
			readout(fmt.Sprintf("%d / %d", utf8.RuneCountInString(msg), limit))),
		demo("States", "Disabled and invalid.", col,
			kit.Textarea{Class: "max-w-sm", Disabled: true, Placeholder: tr("textarea.disabled", "Disabled"), AriaLabel: tr("textarea.disabled", "Disabled")},
			kit.Textarea{Class: "max-w-sm", Invalid: true, DefaultValue: tr("textarea.tooShort", "Too short."), AriaLabel: tr("textarea.invalid", "Invalid")}),
	)
}

func fieldSetPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	plan, setPlan := tela.UseState("pro")
	return grid(
		demo("Fields under a legend", "A FieldSet groups fields under a legend and a description; the group is named by the legend.", col,
			kit.FieldSet{Class: "max-w-sm", Legend: tr("fieldset.address", "Address"), Description: tr("fieldset.addressText", "Where we send your invoices."),
				Children: Fragment(
					kit.Field{Label: tr("fieldset.street", "Street"), Children: kit.Input{Placeholder: tr("fieldset.streetSample", "123 Main St")}},
					kit.Field{Label: tr("fieldset.city", "City"), Children: kit.Input{Placeholder: tr("fieldset.citySample", "Springfield")}},
				)}),
		demo("A radio group", "A RadioGroup or ToggleGroup in a FieldSet takes its name from the legend: no AriaLabel needed.", col,
			kit.FieldSet{Class: "max-w-sm", Legend: tr("fieldset.plan", "Plan"), Description: tr("fieldset.planText", "Choose how you are billed."),
				Children: kit.RadioGroup[string]{Value: &plan, OnValueChange: setPlan.Set, Items: []kit.RadioItem[string]{
					{Value: "free", Label: tr("fieldset.free", "Free")}, {Value: "pro", Label: tr("fieldset.pro", "Pro")}, {Value: "team", Label: tr("fieldset.team", "Team")}}}}),
		demo("Label variant", "LegendVariantLabel draws the legend the size of a field's label, for a group of settings rows.", col,
			kit.FieldSet{Class: "max-w-sm", Legend: tr("fieldset.notifications", "Notifications"), LegendVariant: kit.LegendVariantLabel,
				Children: Fragment(
					kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center", Label: tr("fieldset.push", "Push notifications"),
						Children: kit.Switch{DefaultChecked: true}},
					kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center", Label: tr("fieldset.digest", "Email digest"),
						Description: tr("fieldset.weekly", "Once a week."), Children: kit.Switch{}},
				)}),
	)
}
