package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of Card and Label (the other components that display are in
// pieces.go).

func cardPage() tela.Node {
	return grid(
		demo("Login", "A header with an action, content and a footer.", "",
			kit.Card{
				Class:       "w-full max-w-sm",
				TitleLevel:  2,
				Title:       Text("Login to your account"),
				Description: Text("Enter your email below to login."),
				Action:      kit.Button{Variant: kit.Link, Children: Text("Sign up")},
				Children: Div(Class("flex flex-col gap-4"),
					kit.Field{Label: "Email", Children: kit.Input{Type: "email", Placeholder: "m@example.com"}},
					kit.Field{Label: "Password", Children: kit.Input{Type: "password"}},
				),
				Footer: Div(Class("flex w-full flex-col gap-2"),
					kit.Button{Class: "w-full", Children: Text("Login")},
					kit.Button{Variant: kit.Outline, Class: "w-full", Children: Text("Login with Google")}),
				FooterClass: "",
			}),
		demo("Notifications", "A list in the content slot.", "",
			kit.Card{
				Class:       "w-full max-w-sm",
				TitleLevel:  2,
				Title:       Text("Notifications"),
				Description: Text("You have 3 unread messages."),
				Children: Div(Class("flex flex-col gap-4"),
					notification("Your call has been confirmed.", "1 hour ago"),
					notification("You have a new message.", "1 hour ago"),
					notification("Your subscription is expiring soon.", "2 hours ago"),
				),
				Footer: kit.Button{Variant: kit.Outline, Class: "w-full", Icon: icons.Check, Children: Text("Mark all as read")},
			}),
		demo("Plain surface", "Only content, or only a footer: slots that are nil draw nothing.", "",
			kit.Card{Class: "w-64", Children: Div(Class("flex flex-col gap-1"),
				Span(Class("text-sm text-muted-foreground"), Text("Total revenue")),
				Span(Class("text-3xl font-semibold tabular-nums"), Text("$45,231.89")),
				Span(Class("text-xs text-muted-foreground"), Text("+20.1% from last month")))},
			kit.Card{Class: "w-64", TitleLevel: 2, Title: Text("Storage"), Description: Text("62% of 10 GB used"),
				Children: kit.Progress{Value: 62, AriaLabel: "Storage used"}},
		),
	)
}

func notification(text, when string) tela.Node {
	return Div(Key(text), Class("flex items-start gap-3 text-sm"),
		Span(Class("mt-1.5 size-2 shrink-0 rounded-full bg-primary")),
		Div(Class("flex flex-col"), Span(Class("font-medium"), Text(text)), muted(when)))
}

func labelPage() tela.Node {
	return grid(
		demo("With a checkbox", "A Label with For names the control and toggles it when clicked.", "",
			Div(Class("flex items-center gap-2"),
				kit.Checkbox{ID: "terms"},
				kit.Label{For: "terms", Children: Text("Accept terms and conditions")})),
		demo("With an input", "", col,
			Div(Class("flex w-72 flex-col gap-2"),
				kit.Label{For: "label-email", Children: Text("Email")},
				kit.Input{ID: "label-email", Type: "email", Placeholder: "m@example.com"})),
		demo("Disabled", "Disabled dims the label and ignores the pointer.", "",
			Div(Class("flex items-center gap-2"),
				kit.Checkbox{ID: "terms-off", Disabled: true},
				kit.Label{For: "terms-off", Disabled: true, Children: Text("Subscribe to the newsletter")})),
		demo("Inside a Field", "A Field wires its label to its control with no ids to write.", col,
			kit.Field{Label: "Display name", Description: "Shown next to your comments.", Class: "w-72",
				Children: kit.Input{DefaultValue: "Mei Tanaka"}}),
	)
}
