package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the components that display: Button, Badge, Avatar, Card,
// Separator, Skeleton and Label.

func buttonPage() tela.Node {
	clicks, setClicks := tela.UseState(0)
	saving, setSaving := tela.UseState(false)
	save := func() {
		setSaving.Set(true)
		time.AfterFunc(1500*time.Millisecond, func() { setSaving.Set(false) })
	}
	return grid(
		demo("Variants", "Default, Secondary, Outline, Ghost, Destructive and Link.", "",
			kit.Button{Children: Text("Button")},
			kit.Button{Variant: kit.Secondary, Children: Text("Secondary")},
			kit.Button{Variant: kit.Outline, Children: Text("Outline")},
			kit.Button{Variant: kit.Ghost, Children: Text("Ghost")},
			kit.Button{Variant: kit.Destructive, Children: Text("Destructive")},
			kit.Button{Variant: kit.Link, Children: Text("Link")},
		),
		demo("Sizes", "Xs, Sm, the default and Lg.", "",
			kit.Button{Variant: kit.Outline, Size: kit.Xs, Children: Text("Extra small")},
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Small")},
			kit.Button{Variant: kit.Outline, Children: Text("Default")},
			kit.Button{Variant: kit.Outline, Size: kit.Lg, Children: Text("Large")},
		),
		demo("Icons", "An icon before or after the label, and icon-only buttons in four sizes (AriaLabel names them).", "",
			kit.Button{Icon: icons.Mail, Children: Text("Login with email")},
			kit.Button{Variant: kit.Secondary, IconEnd: icons.ChevronRight, Children: Text("Continue")},
			Div(Class("flex items-center gap-2"),
				kit.Button{Variant: kit.Outline, Size: kit.IconXs, Icon: icons.Plus, AriaLabel: "Add, extra small"},
				kit.Button{Variant: kit.Outline, Size: kit.IconSm, Icon: icons.Plus, AriaLabel: "Add, small"},
				kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: icons.Plus, AriaLabel: "Add"},
				kit.Button{Variant: kit.Outline, Size: kit.IconLg, Icon: icons.Plus, AriaLabel: "Add, large"},
			),
		),
		demo("States", "Disabled, loading (the label is any content) and a button that counts.", "",
			kit.Button{Disabled: true, Children: Text("Disabled")},
			kit.Button{Variant: kit.Outline, Disabled: true, Children: Text("Disabled")},
			kit.Button{Variant: kit.Outline, Disabled: saving, OnClick: save, Children: Fragment(
				If(saving, spinner()), Text(map[bool]string{true: "Saving...", false: "Save changes"}[saving]))},
			kit.Button{Variant: kit.Secondary, OnClick: func() { setClicks.Update(func(n int) int { return n + 1 }) },
				Children: Textf("Clicked %d times", clicks)},
		),
	)
}

func badgePage() tela.Node {
	return grid(
		demo("Variants", "Default, Secondary, Destructive, Outline and Ghost.", "",
			kit.Badge{Children: Text("Badge")},
			kit.Badge{Variant: kit.Secondary, Children: Text("Secondary")},
			kit.Badge{Variant: kit.Destructive, Children: Text("Destructive")},
			kit.Badge{Variant: kit.Outline, Children: Text("Outline")},
			kit.Badge{Variant: kit.Ghost, Children: Text("Ghost")},
		),
		demo("With icons and colors", "The Icon slot; a color is a Class.", "",
			kit.Badge{Variant: kit.Secondary, Icon: icons.Check, Class: "bg-blue-500 text-white dark:bg-blue-600", Children: Text("Verified")},
			kit.Badge{Variant: kit.Destructive, Icon: icons.CircleAlert, Children: Text("Failed")},
			kit.Badge{Variant: kit.Outline, Icon: icons.Star, Children: Text("Starred")},
		),
		demo("Counts", "A round badge with tabular digits.", "",
			kit.Badge{Class: "h-5 min-w-5 rounded-full px-1 font-mono tabular-nums", Children: Text("8")},
			kit.Badge{Variant: kit.Destructive, Class: "h-5 min-w-5 rounded-full px-1 font-mono tabular-nums", Children: Text("99")},
			kit.Badge{Variant: kit.Outline, Class: "h-5 min-w-5 rounded-full px-1 font-mono tabular-nums", Children: Text("20+")},
		),
		demo("In context", "Badges next to the text they describe.", col,
			Div(Class("flex items-center justify-between gap-6 text-sm"),
				Div(Class("flex flex-col"), Span(Class("font-medium"), Text("Invoice INV-4021")), muted("Due 12 November")),
				kit.Badge{Variant: kit.Secondary, Class: "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/50 dark:text-emerald-300", Children: Text("Paid")}),
			kit.Separator{},
			Div(Class("flex items-center justify-between gap-6 text-sm"),
				Div(Class("flex flex-col"), Span(Class("font-medium"), Text("Invoice INV-4022")), muted("Due 3 November")),
				kit.Badge{Variant: kit.Destructive, Icon: icons.CircleAlert, Children: Text("Overdue")}),
		),
	)
}

func avatarPage() tela.Node {
	people := []struct{ Name, Initials string }{
		{"Mei Tanaka", "MT"}, {"Ana García", "AG"}, {"Omar Haddad", "OH"}, {"Lena Fischer", "LF"},
	}
	var stacked, list []tela.Child
	for _, p := range people {
		stacked = append(stacked, kit.Avatar{Size: kit.Lg, Alt: p.Name, Fallback: p.Initials, Class: "ring-2 ring-background", Props: Key(p.Name)})
		list = append(list, Div(Key(p.Name), Class("flex items-center gap-3"),
			kit.Avatar{Alt: p.Name, Fallback: p.Initials},
			Div(Class("flex flex-col text-sm"), Span(Class("font-medium"), Text(p.Name)),
				muted(fmt.Sprintf("%s@example.com", strings.ToLower(p.Initials))))))
	}
	return grid(
		demo("Sizes", "Sm (24 px), the default (32 px) and Lg (40 px). Without a Src the fallback is shown; with one, the picture is painted over it.", "",
			kit.Avatar{Size: kit.Sm, Fallback: "SM"},
			kit.Avatar{Fallback: "MD"},
			kit.Avatar{Size: kit.Lg, Fallback: "LG"},
		),
		demo("Group", "Overlapping avatars with a ring in the background color.", "",
			Div(Class("flex -space-x-3"), Fragment(stacked...)),
			muted("+4 more"),
		),
		demo("In a list", "A person, as a row.", col, Fragment(list...)),
	)
}

func cardPage() tela.Node {
	return grid(
		demo("Login", "A header with an action, content and a footer.", "",
			kit.Card{
				Class:       "w-full max-w-sm",
				Title:       H2(Text("Login to your account")),
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
				Title:       H2(Text("Notifications")),
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
			kit.Card{Class: "w-64", Title: H2(Text("Storage")), Description: Text("62% of 10 GB used"),
				Children: kit.Progress{Value: 62, AriaLabel: "Storage used"}},
		),
	)
}

func notification(text, when string) tela.Node {
	return Div(Key(text), Class("flex items-start gap-3 text-sm"),
		Span(Class("mt-1.5 size-2 shrink-0 rounded-full bg-primary")),
		Div(Class("flex flex-col"), Span(Class("font-medium"), Text(text)), muted(when)))
}

func separatorPage() tela.Node {
	return grid(
		demo("Horizontal", "Between the parts of a block of text.", col,
			Div(Class("flex flex-col gap-1"),
				H3(Class("text-sm leading-none font-medium"), Text("Tela kit")),
				muted("Styled components for Tela apps, in the look of shadcn/ui.")),
			kit.Separator{},
			Div(Class("flex h-5 items-center gap-4 text-sm"),
				Span(Text("Blog")), kit.Separator{Orientation: kit.Vertical}, Span(Text("Docs")),
				kit.Separator{Orientation: kit.Vertical}, Span(Text("Source"))),
		),
		demo("In a menu", "A separator groups the rows of a list.", col,
			Div(Class("flex w-64 flex-col gap-2 text-sm"),
				Span(Text("Profile")), Span(Text("Billing")),
				kit.Separator{Class: "my-1"},
				Span(Text("Team")), Span(Text("Subscription")),
				kit.Separator{Class: "my-1"},
				Span(Class("text-destructive"), Text("Delete account"))),
		),
	)
}

func skeletonPage() tela.Node {
	loading, setLoading := tela.UseState(true)
	return grid(
		demo("Avatar and text", "Size a Skeleton with Class to the shape of what it stands for.", "",
			Div(Class("flex items-center gap-4"), AriaBusy(true),
				kit.Skeleton{Class: "size-12 rounded-full"},
				Div(Class("flex flex-col gap-2"),
					kit.Skeleton{Class: "h-4 w-[250px]"},
					kit.Skeleton{Class: "h-4 w-[200px]"}))),
		demo("Card", "The placeholder of a card with a picture.", "",
			Div(Class("flex w-72 flex-col gap-3"), AriaBusy(true),
				kit.Skeleton{Class: "h-32 w-full rounded-xl"},
				kit.Skeleton{Class: "h-4 w-5/6"},
				kit.Skeleton{Class: "h-4 w-2/3"})),
		demo("Loading, then loaded", "The same slot shows the skeleton while loading is true.", colStart,
			kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: func() { setLoading.Update(func(b bool) bool { return !b }) },
				Children: Text(map[bool]string{true: "Show the content", false: "Show the skeleton"}[loading])},
			IfElse(loading,
				Div(Class("flex items-center gap-4"), AriaBusy(true),
					kit.Skeleton{Class: "size-10 rounded-full"},
					Div(Class("flex flex-col gap-2"), kit.Skeleton{Class: "h-4 w-40"}, kit.Skeleton{Class: "h-3 w-28"})),
				Div(Class("flex items-center gap-4"),
					kit.Avatar{Size: kit.Lg, Fallback: "MT"},
					Div(Class("flex flex-col"), Span(Class("text-sm font-medium"), Text("Mei Tanaka")), muted("Product designer")))),
		),
	)
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
