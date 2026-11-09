package main

import (
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of Input and Field (Textarea and FieldSet are in forms.go). The
// stages are written with tr: English here, Spanish and Arabic in the
// catalogs.

// RevealInput is a password field with a button inside its end (EndContent)
// that shows what was typed: an IconXs ghost button whose icon and accessible
// name follow the state, "Show password" and "Hide password".
type RevealInput struct {
	Label, Show, Hide string
	Value             string
	Class             string
}

func (r RevealInput) Build() tela.Node {
	shown, setShown := tela.UseState(false)
	typ, icon, name := "password", icons.Eye, r.Show
	if shown {
		typ, icon, name = "text", icons.EyeOff, r.Hide
	}
	return kit.Input{
		Class: r.Class, Type: typ, DefaultValue: r.Value, AriaLabel: r.Label,
		EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: icon, IconClass: "size-4",
			Class: "text-muted-foreground", AriaLabel: name, OnClick: func() { setShown.Set(!shown) }},
	}
}

func inputPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	name, setName := tela.UseState("")
	return grid(
		demo("Types", "Type is the HTML one: text, email, password, search, number...", col,
			kit.Input{Class: "max-w-xs", Placeholder: tr("input.fullName", "Full name"), AriaLabel: tr("input.fullName", "Full name")},
			kit.Input{Class: "max-w-xs", Type: "email", Placeholder: tr("input.email", "Email"), AriaLabel: tr("input.email", "Email")},
			kit.Input{Class: "max-w-xs", Type: "password", DefaultValue: "correct horse", AriaLabel: tr("input.password", "Password")},
			kit.Input{Class: "max-w-xs", Type: "search", Placeholder: tr("input.searchComponents", "Search components..."), AriaLabel: tr("input.search", "Search")},
		),
		demo("With icons", "Icon is drawn at the start of the field and IconEnd at its end (the left and the right under left-to-right, mirrored under right-to-left); with an icon Class is the wrapper's and InputClass the input's.", col,
			kit.Input{Class: "max-w-xs", Icon: icons.Search, Type: "search", Placeholder: tr("input.search", "Search"), AriaLabel: tr("input.search", "Search")},
			kit.Input{Class: "max-w-xs", Icon: icons.Mail, Type: "email", Placeholder: tr("input.email", "Email"), AriaLabel: tr("input.email", "Email")},
			kit.Input{Class: "max-w-xs", IconEnd: icons.CircleCheck, DefaultValue: "mei@example.com", AriaLabel: tr("input.verified", "Verified email")},
			kit.Input{Class: "max-w-xs", Icon: icons.Lock, IconEnd: icons.Eye, Type: "password", DefaultValue: "correct horse", AriaLabel: tr("input.password", "Password")},
		),
		demo("States", "A value, disabled, read-only and invalid (aria-invalid draws the red border and ring); an icon follows the state.", col,
			kit.Input{Class: "max-w-xs", DefaultValue: "mei@example.com", AriaLabel: tr("input.withValue", "With a value")},
			kit.Input{Class: "max-w-xs", Disabled: true, Placeholder: tr("input.disabled", "Disabled"), AriaLabel: tr("input.disabled", "Disabled")},
			kit.Input{Class: "max-w-xs", ReadOnly: true, DefaultValue: tr("input.readOnly", "Read only"), AriaLabel: tr("input.readOnly", "Read only")},
			kit.Input{Class: "max-w-xs", Invalid: true, DefaultValue: "not-an-email", AriaLabel: tr("input.invalid", "Invalid")},
			kit.Input{Class: "max-w-xs", Icon: icons.Mail, Disabled: true, Placeholder: tr("input.disabledIcon", "Disabled, with an icon"), AriaLabel: tr("input.disabledIcon", "Disabled, with an icon")},
			kit.Input{Class: "max-w-xs", Icon: icons.Mail, IconEnd: icons.CircleAlert, Invalid: true, DefaultValue: "not-an-email", AriaLabel: tr("input.invalidIcon", "Invalid, with an icon")},
		),
		demo("In a Field", "A Field names the input with its label and describes it; the icon is inside the wrapper, so the label still focuses the field.", col,
			kit.Field{Class: "max-w-xs", Label: tr("input.email", "Email"), Description: tr("input.neverShare", "We never share it."),
				Children: kit.Input{Icon: icons.Mail, Type: "email", Placeholder: "m@example.com"}},
			kit.Field{Class: "max-w-xs", Label: tr("input.search", "Search"),
				Children: kit.Input{Icon: icons.Search, IconEnd: icons.Check, Type: "search", Placeholder: tr("input.searchComponents", "Search components...")}},
		),
		demo("With a button", "An input and a button in a row, as in a newsletter form.", "",
			Div(Class("flex w-full max-w-sm items-center gap-2"),
				kit.Input{Type: "email", Placeholder: tr("input.email", "Email"), AriaLabel: tr("input.email", "Email")},
				kit.Button{Children: Text(tr("input.subscribe", "Subscribe"))})),
		demo("A button inside the field", "EndContent puts a component inside the end of the field, over its padding: here the button that reveals a password. Its icon and its accessible name follow the state (\"Show password\", \"Hide password\"); the field keeps room for a 24 px button, and with an EndContent the Class goes on the wrapper, as with an icon.", col,
			RevealInput{Class: "max-w-xs", Label: tr("input.yourPassword", "Your password"), Show: tr("input.showPassword", "Show password"),
				Hide: tr("input.hidePassword", "Hide password"), Value: "correct horse"}),
		demo("Controlled", "Value points to your state; OnValueChange is how the field asks for a change.", col,
			kit.Input{Class: "max-w-xs", Value: &name, OnValueChange: setName.Set, Placeholder: tr("input.yourName", "Your name"), AriaLabel: tr("input.yourName", "Your name")},
			readout(map[bool]string{true: tr("input.type", "Type something."), false: tr("input.hello", "Hello, {name}!", "name", name)}[name == ""]),
		),
	)
}

func fieldPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	email, setEmail := tela.UseState("mei@")
	bio, setBio := tela.UseState(tr("field.bioSample", "Designer, cook and amateur astronomer."))
	plan, setPlan := tela.UseState("pro")
	align, setAlign := tela.UseState("start")
	const bioLimit = 140
	valid := strings.Contains(email, "@") && strings.Contains(email[strings.Index(email, "@")+1:], ".")
	errText := ""
	if !valid {
		errText = tr("field.emailError", "Enter a valid email address.")
	}
	// The username "mei" is taken: the error offers a free one, which the
	// link fills in.
	handle, setHandle := tela.UseState("mei")
	var takenError tela.Component
	takenText := ""
	if handle == "mei" {
		suggestion := "mei.tanaka"
		takenText = tr("field.taken", "That username is taken.")
		takenError = Span(Text(takenText+" "),
			A(Href("#suggestion"), Class("text-destructive underline underline-offset-4"),
				OnClick(func() { setHandle.Set(suggestion) }),
				Text(tr("field.use", "Use {name}", "name", suggestion))))
	}
	return grid(
		demo("Label, description and error", "The label names the control and focuses it when clicked; the description or the error describes it. Edit the email to clear the error.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.email", "Email"), Description: tr("field.neverShare", "We never share it."), Error: errText,
				Children: kit.Input{Type: "email", Value: &email, OnValueChange: setEmail.Set, Props: Dir("ltr")}}),
		demo("A slot beside the label", "LabelEnd sits at the end of the label's line, outside the label: pressing it does not focus the field.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.password", "Password"),
				LabelEnd: kit.Button{Variant: kit.Link, Size: kit.Sm, Class: "h-auto p-0 text-xs", Children: Text(tr("field.forgot", "Forgot password?"))},
				Children: kit.Input{Type: "password", DefaultValue: "correct horse"}}),
		demo("A rich label", "LabelContent is drawn in place of the Label text: here the asterisk of a required field. Label still names the control for assistive technologies.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.fullName", "Full name"),
				LabelContent: Span(Text(tr("field.fullName", "Full name")), Span(Class("ms-0.5 text-destructive"), AriaHidden(true), Text("*"))),
				Description:  tr("field.required", "Required: it appears on your invoices."),
				Children:     kit.Input{Placeholder: "Mei Tanaka"}}),
		demo("A counter", "DescriptionEnd sits at the end of the description's line: a character counter beside the help text.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.bio", "Bio"), Description: tr("field.bioHelp", "A line about yourself."),
				DescriptionEnd: Span(Class("text-xs text-muted-foreground tabular-nums"), Dir("ltr"), // "29 / 140" in every direction
					Text(tr("field.count", "{n} / {max}", "n", f.Number(float64(utf8.RuneCountInString(bio))), "max", f.Number(bioLimit)))),
				Children: kit.Textarea{Value: &bio, OnValueChange: setBio.Set, MaxLength: bioLimit, Rows: 3}}),
		demo("A rich description", "DescriptionContent takes any inline content, such as a link.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.username", "Username"), Description: tr("field.usernameHelp", "Choose a username. Read the terms of service."),
				DescriptionContent: Span(Text(tr("field.usernameText", "Choose a username. Read the")+" "),
					A(Href("#terms"), Class("text-foreground underline underline-offset-4"), Text(tr("field.terms", "terms of service"))), Text(".")),
				Children: kit.Input{DefaultValue: "mei"}}),
		demo("A rich error", "ErrorContent is drawn in place of Error, for a message that is more than text: here a link. It makes the field invalid, shows as an alert under the control and describes it. Press the link, or edit the name, to clear it.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.handle", "Username"), Error: takenText, ErrorContent: takenError,
				Children: kit.Input{Value: &handle, OnValueChange: setHandle.Set}}),
		demo("A group in a Field", "A RadioGroup or a ToggleGroup in a Field is named by the Field's label: neither has an AriaLabel here.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.plan", "Plan"), Description: tr("field.planHelp", "Choose how you are billed."),
				Children: kit.RadioGroup[string]{Value: &plan, OnValueChange: setPlan.Set, Items: []kit.RadioItem[string]{
					{Value: "free", Label: tr("field.free", "Free")}, {Value: "pro", Label: tr("field.pro", "Pro")}, {Value: "team", Label: tr("field.team", "Team")}}}},
			kit.Field{Class: "max-w-sm", Label: tr("field.align", "Alignment"),
				Children: kit.ToggleGroup[string]{Variant: kit.Outline, DisallowEmptySelection: true, Value: &align, OnValueChange: setAlign.Set,
					Items: []kit.ToggleGroupItem[string]{
						{Value: "start", Icon: icons.TextAlignStart, AriaLabel: tr("field.alignStart", "Align left")},
						{Value: "center", Icon: icons.TextAlignCenter, AriaLabel: tr("field.alignCenter", "Align center")},
						{Value: "end", Icon: icons.TextAlignEnd, AriaLabel: tr("field.alignEnd", "Align right")}}}}),
		demo("Other controls", "Select, Textarea, DatePicker and the choice controls take the field's id the same way.", col,
			Div(Class("flex w-full max-w-sm flex-col gap-5"),
				kit.Field{Label: tr("field.role", "Role"), Description: tr("field.roleHelp", "What you do in the team."),
					Children: kit.Select[string]{Placeholder: tr("field.selectRole", "Select a role"), Items: []kit.SelectItem[string]{
						{Value: "design", Label: tr("field.design", "Design")}, {Value: "eng", Label: tr("field.eng", "Engineering")}, {Value: "ops", Label: tr("field.ops", "Operations")}}}},
				kit.Field{Label: tr("field.about", "About you"), Children: kit.Textarea{Placeholder: tr("field.tell", "Tell us a little about yourself"), Rows: 3}},
			)),
		demo("Horizontal", "Orientation Horizontal puts the label beside a checkbox; ControlAfter puts the control after the text.", col,
			Div(Class("flex w-full max-w-sm flex-col gap-5"),
				kit.Field{Orientation: kit.Horizontal, Label: tr("field.remember", "Remember me"), Description: tr("field.rememberHelp", "Stay signed in on this device."),
					Children: kit.Checkbox{DefaultChecked: true}},
				kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center", Label: tr("field.marketing", "Marketing emails"),
					Description: tr("field.marketingHelp", "News about the product, once a month."), Children: kit.Switch{}},
			)),
		demo("Invalid with no message", "Invalid marks the control and the label when the message is shown somewhere else.", col,
			kit.Field{Class: "max-w-sm", Label: tr("field.nick", "Nickname"), Invalid: true,
				Children: kit.Input{DefaultValue: "mei tanaka"}},
			kit.Field{Class: "max-w-sm", Label: tr("field.password", "Password"), Error: tr("field.short", "Use at least 8 characters."),
				Children: kit.Input{Type: "password", DefaultValue: "short"}}),
	)
}
