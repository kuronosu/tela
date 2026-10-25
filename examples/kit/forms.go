package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the text fields and of Field, which ties a label, a control,
// its help text and its error together.

func inputPage() tela.Node {
	name, setName := tela.UseState("")
	return grid(
		demo("Types", "Type is the HTML one: text, email, password, search, number...", col,
			kit.Input{Class: "max-w-xs", Placeholder: "Full name", Props: AriaLabel("Full name")},
			kit.Input{Class: "max-w-xs", Type: "email", Placeholder: "Email", Props: AriaLabel("Email")},
			kit.Input{Class: "max-w-xs", Type: "password", DefaultValue: "correct horse", Props: AriaLabel("Password")},
			kit.Input{Class: "max-w-xs", Type: "search", Placeholder: "Search components...", Props: AriaLabel("Search")},
		),
		demo("States", "A value, disabled, read-only and invalid (aria-invalid draws the red border and ring).", col,
			kit.Input{Class: "max-w-xs", DefaultValue: "mei@example.com", Props: AriaLabel("With a value")},
			kit.Input{Class: "max-w-xs", Disabled: true, Placeholder: "Disabled", Props: AriaLabel("Disabled")},
			kit.Input{Class: "max-w-xs", ReadOnly: true, DefaultValue: "Read only", Props: AriaLabel("Read only")},
			kit.Input{Class: "max-w-xs", Invalid: true, DefaultValue: "not-an-email", Props: AriaLabel("Invalid")},
		),
		demo("With a button", "An input and a button in a row, as in a newsletter form.", "",
			Div(Class("flex w-full max-w-sm items-center gap-2"),
				kit.Input{Type: "email", Placeholder: "Email", Props: AriaLabel("Email")},
				kit.Button{Children: Text("Subscribe")})),
		demo("Controlled", "Value points to your state; OnValueChange is how the field asks for a change.", col,
			kit.Input{Class: "max-w-xs", Value: &name, OnValueChange: setName.Set, Placeholder: "Your name", Props: AriaLabel("Your name")},
			readout(map[bool]string{true: "Type something.", false: fmt.Sprintf("Hello, %s!", name)}[name == ""]),
		),
	)
}

func textareaPage() tela.Node {
	msg, setMsg := tela.UseState("")
	const limit = 140
	return grid(
		demo("Basic", "It grows with its text from a minimum height of 64 px.", col,
			kit.Textarea{Class: "max-w-sm", Placeholder: "Type your message here.", Props: AriaLabel("Message")}),
		demo("With a label and a button", "", col,
			Div(Class("flex w-full max-w-sm flex-col gap-3"),
				kit.Field{Label: "Your message", Description: "Your message will be copied to the support team.",
					Children: kit.Textarea{Placeholder: "Type your message here."}},
				kit.Button{Children: Text("Send message")})),
		demo("Counter", fmt.Sprintf("MaxLength stops typing at %d characters; the count is the app's.", limit), col,
			kit.Textarea{Class: "max-w-sm", Value: &msg, OnValueChange: setMsg.Set, MaxLength: limit, Rows: 3,
				Placeholder: "What are you working on?", Props: AriaLabel("Status")},
			readout(fmt.Sprintf("%d / %d", utf8.RuneCountInString(msg), limit))),
		demo("States", "Disabled and invalid.", col,
			kit.Textarea{Class: "max-w-sm", Disabled: true, Placeholder: "Disabled", Props: AriaLabel("Disabled")},
			kit.Textarea{Class: "max-w-sm", Invalid: true, DefaultValue: "Too short.", Props: AriaLabel("Invalid")}),
	)
}

func fieldPage() tela.Node {
	email, setEmail := tela.UseState("mei@")
	valid := strings.Contains(email, "@") && strings.Contains(email[strings.Index(email, "@")+1:], ".")
	errText := ""
	if !valid {
		errText = "Enter a valid email address."
	}
	return grid(
		demo("Label, description and error", "The label names the control and focuses it when clicked; the description or the error describes it. Edit the email to clear the error.", col,
			kit.Field{Class: "max-w-sm", Label: "Email", Description: "We never share it.", Error: errText,
				Children: kit.Input{Type: "email", Value: &email, OnValueChange: setEmail.Set}}),
		demo("Other controls", "Select, Textarea, DatePicker and the choice controls take the field's id the same way.", col,
			Div(Class("flex w-full max-w-sm flex-col gap-5"),
				kit.Field{Label: "Role", Description: "What you do in the team.",
					Children: kit.Select[string]{Placeholder: "Select a role", Items: []kit.SelectItem[string]{
						{Value: "design", Label: "Design"}, {Value: "eng", Label: "Engineering"}, {Value: "ops", Label: "Operations"}}}},
				kit.Field{Label: "Bio", Children: kit.Textarea{Placeholder: "Tell us a little about yourself", Rows: 3}},
			)),
		demo("Horizontal", "Orientation Horizontal puts the label beside a checkbox; flex-row-reverse puts the control after the text.", col,
			Div(Class("flex w-full max-w-sm flex-col gap-5"),
				kit.Field{Orientation: kit.Horizontal, Label: "Remember me", Description: "Stay signed in on this device.",
					Children: kit.Checkbox{DefaultChecked: true}},
				kit.Field{Orientation: kit.Horizontal, Class: "flex-row-reverse items-center justify-between", Label: "Marketing emails",
					Description: "News about the product, once a month.", Children: kit.Switch{}},
			)),
		demo("Invalid with no message", "Invalid marks the control and the label when the message is shown somewhere else.", col,
			kit.Field{Class: "max-w-sm", Label: "Username", Invalid: true,
				Children: kit.Input{DefaultValue: "mei tanaka"}},
			kit.Field{Class: "max-w-sm", Label: "Password", Error: "Use at least 8 characters.",
				Children: kit.Input{Type: "password", DefaultValue: "short"}}),
	)
}
