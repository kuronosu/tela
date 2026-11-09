package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of Card and Label (the other components that display are in
// pieces.go). The stages are written with tr: English here, Spanish and
// Arabic in the catalogs.

func cardPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	return grid(
		demo("Login", "A header with an action, content and a footer.", "",
			kit.Card{
				Class:       "w-full max-w-sm",
				TitleLevel:  2,
				Title:       Text(tr("card.loginTitle", "Login to your account")),
				Description: Text(tr("card.loginText", "Enter your email below to login.")),
				Action:      kit.Button{Variant: kit.Link, Children: Text(tr("card.signUp", "Sign up"))},
				Children: Div(Class("flex flex-col gap-4"),
					kit.Field{Label: tr("card.email", "Email"), Children: kit.Input{Type: "email", Placeholder: "m@example.com"}},
					kit.Field{Label: tr("card.password", "Password"), Children: kit.Input{Type: "password"}},
				),
				Footer: Div(Class("flex w-full flex-col gap-2"),
					kit.Button{Class: "w-full", Children: Text(tr("card.login", "Login"))},
					kit.Button{Variant: kit.Outline, Class: "w-full", Children: Text(tr("card.loginGoogle", "Login with Google"))}),
				FooterClass: "",
			}),
		demo("Notifications", "A list in the content slot.", "",
			kit.Card{
				Class:       "w-full max-w-sm",
				TitleLevel:  2,
				Title:       Text(tr("card.notifications", "Notifications")),
				Description: Text(tr("card.unread", "You have 3 unread messages.")),
				Children: Div(Class("flex flex-col gap-4"),
					notification(tr("card.call", "Your call has been confirmed."), tr("card.hour1", "1 hour ago")),
					notification(tr("card.message", "You have a new message."), tr("card.hour1", "1 hour ago")),
					notification(tr("card.expiring", "Your subscription is expiring soon."), tr("card.hours2", "2 hours ago")),
				),
				Footer: kit.Button{Variant: kit.Outline, Class: "w-full", Icon: icons.Check, Children: Text(tr("card.markRead", "Mark all as read"))},
			}),
		demo("Plain surface", "Only content, or only a footer: slots that are nil draw nothing.", "",
			kit.Card{Class: "w-64", Children: Div(Class("flex flex-col gap-1"),
				Span(Class("text-sm text-muted-foreground"), Text(tr("card.revenue", "Total revenue"))),
				Span(Class("text-3xl font-semibold tabular-nums"), Text("$45,231.89")),
				Span(Class("text-xs text-muted-foreground"), Text(tr("card.growth", "+20.1% from last month"))))},
			kit.Card{Class: "w-64", TitleLevel: 2, Title: Text(tr("card.storage", "Storage")), Description: Text(tr("card.storageUsed", "62% of 10 GB used")),
				Children: kit.Progress{Value: 62, AriaLabel: tr("card.storageLabel", "Storage used")}},
		),
	)
}

func notification(text, when string) tela.Node {
	return Div(Key(text), Class("flex items-start gap-3 text-sm"),
		Span(Class("mt-1.5 size-2 shrink-0 rounded-full bg-primary")),
		Div(Class("flex flex-col"), Span(Class("font-medium"), Text(text)), muted(when)))
}

func labelPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	return grid(
		demo("With a checkbox", "A Label with For names the control and toggles it when clicked.", "",
			Div(Class("flex items-center gap-2"),
				kit.Checkbox{ID: "terms"},
				kit.Label{For: "terms", Children: Text(tr("label.terms", "Accept terms and conditions"))})),
		demo("With an input", "", col,
			Div(Class("flex w-72 flex-col gap-2"),
				kit.Label{For: "label-email", Children: Text(tr("label.email", "Email"))},
				kit.Input{ID: "label-email", Type: "email", Placeholder: "m@example.com"})),
		demo("Disabled", "Disabled dims the label and ignores the pointer.", "",
			Div(Class("flex items-center gap-2"),
				kit.Checkbox{ID: "terms-off", Disabled: true},
				kit.Label{For: "terms-off", Disabled: true, Children: Text(tr("label.newsletter", "Subscribe to the newsletter"))})),
		demo("Inside a Field", "A Field wires its label to its control with no ids to write.", col,
			kit.Field{Label: tr("label.displayName", "Display name"), Description: tr("label.displayHint", "Shown next to your comments."), Class: "w-72",
				Children: kit.Input{DefaultValue: "Mei Tanaka"}}),
	)
}
