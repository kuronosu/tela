package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of the components that display: Button, Badge, Avatar, Alert,
// Separator and Skeleton. What a stage shows is written with tr, in English
// here and in Spanish and Arabic in the catalogs; the captions above the
// stages stay English.

// saveTime is how long the "save" of a loading demo takes.
const saveTime = 2 * time.Second

// useSaving is the state of a button that "saves": start sets it loading
// and the frame clock's timer, saveTime later, clears it. Nothing runs while
// it is not loading, so the page stays idle.
func useSaving() (loading bool, start func()) {
	loading, setLoading := tela.UseState(false)
	presses, setPresses := tela.UseState(0)
	d := time.Duration(0)
	if loading {
		d = saveTime
	}
	tela.UseTimeout(d, func() { setLoading.Set(false) }, presses)
	return loading, func() {
		setPresses.Update(func(n int) int { return n + 1 })
		setLoading.Set(true)
	}
}

// SaveButton is a kit.Button that shows Loading for two seconds when it is
// pressed.
type SaveButton struct {
	Variant       kit.Variant
	Size          kit.Size
	Label         string
	AriaLabel     string
	Icon, IconEnd tela.Icon
}

func (b SaveButton) Build() tela.Node {
	loading, start := useSaving()
	btn := kit.Button{Variant: b.Variant, Size: b.Size, Icon: b.Icon, IconEnd: b.IconEnd, AriaLabel: b.AriaLabel,
		Loading: loading, OnClick: start}
	if b.Label != "" {
		btn.Children = Text(b.Label)
	}
	return btn
}

// SubscribeForm is a form with an email field and a submit button that is
// Loading while the "request" is out.
type SubscribeForm struct {
	Email, Subscribe, Hint string
}

func (s SubscribeForm) Build() tela.Node {
	loading, start := useSaving()
	return Form(Class("flex w-full max-w-sm items-end gap-2"), OnSubmit(start),
		kit.Field{Label: s.Email, Class: "flex-1", Children: kit.Input{Type: "email", Placeholder: "m@example.com"}},
		kit.Button{Type: "submit", Loading: loading, Icon: icons.Mail, Children: Text(s.Subscribe)})
}

func buttonPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	clicks, setClicks := tela.UseState(0)
	return grid(
		demo("Variants", "Default, Secondary, Outline, Ghost, Destructive and Link.", "",
			kit.Button{Children: Text(tr("button.default", "Button"))},
			kit.Button{Variant: kit.Secondary, Children: Text(tr("button.secondary", "Secondary"))},
			kit.Button{Variant: kit.Outline, Children: Text(tr("button.outline", "Outline"))},
			kit.Button{Variant: kit.Ghost, Children: Text(tr("button.ghost", "Ghost"))},
			kit.Button{Variant: kit.Destructive, Children: Text(tr("button.destructive", "Destructive"))},
			kit.Button{Variant: kit.Link, Children: Text(tr("button.link", "Link"))},
		),
		demo("Sizes", "Xs, Sm, the default and Lg.", "",
			kit.Button{Variant: kit.Outline, Size: kit.Xs, Children: Text(tr("button.xs", "Extra small"))},
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("button.sm", "Small"))},
			kit.Button{Variant: kit.Outline, Children: Text(tr("button.md", "Default"))},
			kit.Button{Variant: kit.Outline, Size: kit.Lg, Children: Text(tr("button.lg", "Large"))},
		),
		demo("Icons", "An icon before or after the label, and icon-only buttons in four sizes (AriaLabel names them).", "",
			kit.Button{Icon: icons.Mail, Children: Text(tr("button.login", "Login with email"))},
			kit.Button{Variant: kit.Secondary, IconEnd: icons.ChevronRight, Children: Text(tr("button.continue", "Continue"))},
			Div(Class("flex items-center gap-2"),
				kit.Button{Variant: kit.Outline, Size: kit.IconXs, Icon: icons.Plus, AriaLabel: tr("button.addXs", "Add, extra small")},
				kit.Button{Variant: kit.Outline, Size: kit.IconSm, Icon: icons.Plus, AriaLabel: tr("button.addSm", "Add, small")},
				kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: icons.Plus, AriaLabel: tr("button.add", "Add")},
				kit.Button{Variant: kit.Outline, Size: kit.IconLg, Icon: icons.Plus, AriaLabel: tr("button.addLg", "Add, large")},
			),
		),
		demo("Loading", "Loading turns the Icon (or IconEnd) into a spinner, or draws one over a text-only label, and ignores presses; the button keeps its width and its place in the tab order. Press each one: it \"saves\" for two seconds.", "",
			SaveButton{Label: tr("button.save", "Save changes")},
			SaveButton{Variant: kit.Secondary, Icon: icons.Save, Label: tr("button.saveIcon", "Save")},
			SaveButton{Variant: kit.Outline, IconEnd: icons.ChevronRight, Label: tr("button.continue", "Continue")},
			SaveButton{Variant: kit.Outline, Size: kit.Icon, Icon: icons.Save, AriaLabel: tr("button.saveIcon", "Save")},
		),
		demo("In a form", "A submit button in a form is Loading while the request is out: Enter in the field submits, and the press is ignored until it ends.", col,
			SubscribeForm{Email: tr("button.email", "Email"), Subscribe: tr("button.subscribe", "Subscribe")}),
		demo("States", "Disabled, and a button that counts.", "",
			kit.Button{Disabled: true, Children: Text(tr("button.disabled", "Disabled"))},
			kit.Button{Variant: kit.Outline, Disabled: true, Children: Text(tr("button.disabled", "Disabled"))},
			kit.Button{Variant: kit.Secondary, OnClick: func() { setClicks.Update(func(n int) int { return n + 1 }) },
				Children: Text(tr("button.clicked", "Clicked {n} times", "n", f.Number(float64(clicks))))},
		),
	)
}

func badgePage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	name := func(id, en string) tela.Node { return Text(tr(id, en)) }
	count := func(v kit.Variant, n string) tela.Node {
		return kit.Badge{Variant: v, Class: "h-5 min-w-5 rounded-full px-1 font-mono tabular-nums", Children: Text(n)}
	}
	return grid(
		demo("Variants", "Default, Secondary, Destructive, Outline and Ghost.", "",
			kit.Badge{Children: name("badge.default", "Badge")},
			kit.Badge{Variant: kit.Secondary, Children: name("badge.secondary", "Secondary")},
			kit.Badge{Variant: kit.Destructive, Children: name("badge.destructive", "Destructive")},
			kit.Badge{Variant: kit.Outline, Children: name("badge.outline", "Outline")},
			kit.Badge{Variant: kit.Ghost, Children: name("badge.ghost", "Ghost")},
		),
		demo("Tones", "Success, Warning and Info: tones of a status, with and without an icon.", col,
			Div(Class(row),
				kit.Badge{Variant: kit.Success, Children: name("badge.success", "Success")},
				kit.Badge{Variant: kit.Warning, Children: name("badge.warning", "Warning")},
				kit.Badge{Variant: kit.Info, Children: name("badge.info", "Info")}),
			Div(Class(row),
				kit.Badge{Variant: kit.Success, Icon: icons.CircleCheck, Children: name("badge.success", "Success")},
				kit.Badge{Variant: kit.Warning, Icon: icons.TriangleAlert, Children: name("badge.warning", "Warning")},
				kit.Badge{Variant: kit.Info, Icon: icons.Info, Children: name("badge.info", "Info")}),
		),
		demo("Dot", "Dot draws a status dot in the color of the text, in every variant and tone; it is hidden from screen readers, so say the status in the label.", col,
			Div(Class(row),
				kit.Badge{Dot: true, Children: name("badge.default", "Badge")},
				kit.Badge{Variant: kit.Secondary, Dot: true, Children: name("badge.secondary", "Secondary")},
				kit.Badge{Variant: kit.Destructive, Dot: true, Children: name("badge.destructive", "Destructive")},
				kit.Badge{Variant: kit.Outline, Dot: true, Children: name("badge.outline", "Outline")},
				kit.Badge{Variant: kit.Ghost, Dot: true, Children: name("badge.ghost", "Ghost")}),
			Div(Class(row),
				kit.Badge{Variant: kit.Success, Dot: true, Children: name("badge.active", "Active")},
				kit.Badge{Variant: kit.Warning, Dot: true, Children: name("badge.pending", "Pending")},
				kit.Badge{Variant: kit.Info, Dot: true, Children: name("badge.syncing", "Syncing")}),
		),
		demo("Dot color", "DotClass colors the dot apart from the text: the Outline badge with a green, amber, red or gray dot.", "",
			kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-emerald-500", Children: name("badge.online", "Online")},
			kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-amber-500", Children: name("badge.away", "Away")},
			kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-red-500", Children: name("badge.busy", "Busy")},
			kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-muted-foreground", Children: name("badge.offline", "Offline")},
		),
		demo("With icons and colors", "The Icon slot; a color is a Class.", "",
			kit.Badge{Variant: kit.Secondary, Icon: icons.Check, Class: "bg-blue-500 text-white dark:bg-blue-600", Children: name("badge.verified", "Verified")},
			kit.Badge{Variant: kit.Destructive, Icon: icons.CircleAlert, Children: name("badge.failed", "Failed")},
			kit.Badge{Variant: kit.Outline, Icon: icons.Star, Children: name("badge.starred", "Starred")},
		),
		demo("Counts", "A round badge with tabular digits.", "",
			count(kit.Default, f.Number(8)),
			count(kit.Destructive, f.Number(99)),
			count(kit.Outline, f.Number(20)+"+"),
		),
		demo("In context", "Badges next to the text they describe.", col,
			Div(Class("flex items-center justify-between gap-6 text-sm"),
				Div(Class("flex flex-col"), Span(Class("font-medium"), Text(tr("badge.invoice1", "Invoice INV-4021"))), muted(tr("badge.due1", "Due 12 November"))),
				kit.Badge{Variant: kit.Success, Children: name("badge.paid", "Paid")}),
			kit.Separator{},
			Div(Class("flex items-center justify-between gap-6 text-sm"),
				Div(Class("flex flex-col"), Span(Class("font-medium"), Text(tr("badge.invoice2", "Invoice INV-4022"))), muted(tr("badge.due2", "Due 3 November"))),
				kit.Badge{Variant: kit.Destructive, Icon: icons.CircleAlert, Children: name("badge.overdue", "Overdue")}),
		),
	)
}

// avatarColors are the nine named colors of the recipe, each with a person.
var avatarColors = []struct{ Color, Name string }{
	{"red", "Rosa Díaz"}, {"orange", "Omar Haddad"}, {"amber", "Ana García"}, {"emerald", "Emma Weber"}, {"teal", "Tomás Vega"},
	{"sky", "Sofía Ríos"}, {"indigo", "Ines Silva"}, {"violet", "Victor Hugo"}, {"pink", "Paula Ortiz"},
}

func avatarPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	people := []struct{ Name, Initials string }{
		{"Mei Tanaka", "MT"}, {"Ana García", "AG"}, {"Omar Haddad", "OH"}, {"Lena Fischer", "LF"},
	}
	// team is a group of people, some with a picture and the others with
	// their initials, each with the color its name gives.
	team := []kit.Avatar{
		{Name: "Mei Tanaka", Src: SrcBytes(portrait(0))},
		{Name: "Ana García", Color: kit.AvatarAuto},
		{Name: "Omar Haddad", Src: SrcBytes(portrait(1))},
		{Name: "Lena Fischer", Color: kit.AvatarAuto},
		{Name: "Tomás Vega", Color: kit.AvatarAuto},
		{Name: "Sofía Ríos", Src: SrcBytes(portrait(2))},
	}
	var auto []kit.Avatar
	var list []tela.Child
	for _, p := range people {
		auto = append(auto, kit.Avatar{Name: p.Name, Color: kit.AvatarAuto})
		list = append(list, Div(Key(p.Name), Class("flex items-center gap-3"),
			kit.Avatar{Alt: p.Name, Fallback: p.Initials},
			Div(Class("flex flex-col text-sm"), Span(Class("font-medium"), Text(p.Name)),
				muted(fmt.Sprintf("%s@example.com", strings.ToLower(p.Initials))))))
	}
	var named []tela.Child
	for _, c := range avatarColors {
		named = append(named, Div(Key(c.Color), Class("flex flex-col items-center gap-1.5"),
			kit.Avatar{Name: c.Name, Color: kit.AvatarColor(c.Color), Size: kit.Lg},
			Span(Dir("ltr"), Class("font-mono text-xs text-muted-foreground"), Text(c.Color))))
	}
	return grid(
		demo("Sizes", "Sm (24 px), the default (32 px) and Lg (40 px). Without a Src the fallback is shown; with one, the picture is painted over it.", "",
			kit.Avatar{Size: kit.Sm, Fallback: "SM"},
			kit.Avatar{Fallback: "MD"},
			kit.Avatar{Size: kit.Lg, Fallback: "LG"},
			kit.Avatar{Size: kit.Lg, Src: SrcBytes(portrait(3)), Alt: tr("avatar.alt", "A portrait"), Fallback: "PT"},
		),
		demo("Named colors", "Color is a name from the recipe: the nine of Tailwind's palette that read well under white text.", "flex flex-wrap items-start gap-x-5 gap-y-4", Fragment(named...)),
		demo("Automatic color and initials", "Name gives the initials (the first letters of its first and last words) and, with Color AvatarAuto, a color that stays the same for the same name everywhere.", "",
			Fragment(avatarsOf(auto)...)),
		demo("Name, Alt and Fallback", "Name is the alt of a picture and the source of the initials; a Fallback of your own and an Alt do the same by hand.", "",
			kit.Avatar{Name: "Mei Tanaka", Src: SrcBytes(portrait(0)), Size: kit.Lg},
			kit.Avatar{Name: "Mei Tanaka", Color: kit.AvatarAuto, Size: kit.Lg},
			kit.Avatar{Alt: tr("avatar.alt", "A portrait"), Fallback: "?", Size: kit.Lg},
			kit.Avatar{Name: "Ada Lovelace", Fallback: "AL", FallbackClass: "bg-linear-to-br from-fuchsia-500 to-indigo-500 text-white", Size: kit.Lg},
		),
		demo("Group", "kit.AvatarGroup overlaps avatars with a ring in the background color and counts the rest: Max draws that many, Total says how many there are.", col,
			kit.AvatarGroup{Avatars: team[:4], Max: 3, Total: 8, Size: kit.Lg, AriaLabel: tr("avatar.team", "Team")},
			kit.AvatarGroup{Avatars: team, Max: 4, AriaLabel: tr("avatar.team", "Team")},
			kit.AvatarGroup{Avatars: team[:3], Size: kit.Sm, AriaLabel: tr("avatar.team", "Team")},
		),
		demo("Pictures and initials", "Avatars with a picture and avatars with initials mix in one group; the ring is as wide as the surface.", "",
			kit.AvatarGroup{Avatars: team[:3], Total: 12, Size: kit.Lg, AriaLabel: tr("avatar.team", "Team")},
		),
		demo("On a card", "On a different surface the ring takes its color: AvatarClass \"ring-card\".", "",
			kit.Card{Class: "w-full max-w-sm", TitleLevel: 2, Title: Text(tr("avatar.cardTitle", "Design team")),
				Description: Text(tr("avatar.cardText", "Eight people work on this project.")),
				Children:    kit.AvatarGroup{Avatars: team, Max: 5, Total: 8, AvatarClass: "ring-card", AriaLabel: tr("avatar.team", "Team")}}),
		demo("In a list", "A person, as a row.", col, Fragment(list...)),
	)
}

// avatarsOf is the avatars as children.
func avatarsOf(as []kit.Avatar) []tela.Child {
	out := make([]tela.Child, len(as))
	for i, a := range as {
		a.Size = kit.Lg
		out[i] = a
	}
	return out
}

func alertPage() tela.Node {
	tr := useTr()
	return grid(
		demo("Default", "An icon, a title and a description. Its role is alert, so a screen reader announces it when it appears.", col,
			kit.Alert{Icon: icons.Info, Title: Text(tr("alert.heads", "Heads up!")), Description: Text(tr("alert.headsText", "You can add components to your app with kit."))},
		),
		demo("Destructive", "For an error the user has to act on.", col,
			kit.Alert{Variant: kit.Destructive, Icon: icons.CircleAlert, Title: Text(tr("alert.payment", "Unable to process your payment.")), Description: Text(tr("alert.paymentText", "Please verify your billing information and try again."))},
		),
		demo("Title only, description only", "Either slot may be left out; the icon stays level with the first line.", col,
			kit.Alert{Icon: icons.Check, Title: Text(tr("alert.saved", "Your changes were saved."))},
			kit.Alert{Icon: icons.Mail, Description: Text(tr("alert.sent", "We sent a confirmation to your email address."))},
			kit.Alert{Title: Text(tr("alert.plain", "No icon, only a title"))},
		),
		demo("Several lines", "The description may be a few paragraphs, and it wraps under the title, not under the icon.", col,
			kit.Alert{Icon: icons.CircleCheck, Title: Text(tr("alert.renewed", "Your subscription was renewed")),
				Description: Div(Class("flex flex-col gap-1"),
					P(Text(tr("alert.renewedA", "The next payment is on 12 December, and the receipt is in your inbox."))),
					P(Text(tr("alert.renewedB", "You can change the plan or cancel at any time from the billing page, and what you already paid for stays yours until the end of the period."))))},
		),
		demo("Tones", "Success, Warning and Info, the tones of Badge, in an Alert.", col,
			kit.Alert{Variant: kit.Success, Icon: icons.CircleCheck, Title: Text(tr("alert.success", "Payment received")), Description: Text(tr("alert.successText", "Your card was charged $12.00."))},
			kit.Alert{Variant: kit.Warning, Icon: icons.TriangleAlert, Title: Text(tr("alert.warning", "Your trial ends in 3 days")), Description: Text(tr("alert.warningText", "Add a payment method to keep your projects."))},
			kit.Alert{Variant: kit.Info, Icon: icons.Info, Title: Text(tr("alert.info", "Scheduled maintenance on Sunday"))},
		),
		demo("Not an interruption", "Role \"status\" (a polite live region) or \"note\" (none) for a notice that is on the page from the start, such as the top of a form.", col,
			kit.Alert{Variant: kit.Info, Role: "status", Icon: icons.Info, Title: Text(tr("alert.status", "The service will be down on Sunday")), Description: Text(tr("alert.statusText", "From 02:00 to 03:00 UTC, for maintenance."))},
			kit.Alert{Role: "note", Icon: icons.Info, Title: Text(tr("alert.note", "Drafts are saved every minute."))},
		),
	)
}

func separatorPage() tela.Node {
	tr := useTr()
	panel := func(text string) tela.Node {
		return Div(Class("flex flex-1 items-center justify-center rounded-lg border border-border bg-card px-4 text-sm"), Text(text))
	}
	return grid(
		demo("Horizontal", "Between the parts of a block of text.", col,
			Div(Class("flex flex-col gap-1"),
				H3(Class("text-sm leading-none font-medium"), Text(tr("separator.title", "Tela kit"))),
				muted(tr("separator.text", "Styled components for Tela apps, in the look of shadcn/ui."))),
			kit.Separator{},
			Div(Class("flex h-5 items-center gap-4 text-sm"),
				Span(Text(tr("separator.blog", "Blog"))), kit.Separator{Orientation: kit.Vertical}, Span(Text(tr("separator.docs", "Docs"))),
				kit.Separator{Orientation: kit.Vertical}, Span(Text(tr("separator.source", "Source")))),
		),
		demo("With a caption", "Caption draws a label between two lines: the \"Or continue with\" of a login form.", col,
			Div(Class("flex w-full max-w-xs flex-col gap-4"),
				kit.Field{Label: tr("separator.email", "Email"), Children: kit.Input{Type: "email", Placeholder: "m@example.com"}},
				kit.Button{Children: Text(tr("separator.login", "Login"))},
				kit.Separator{Caption: Text(tr("separator.or", "Or continue with"))},
				Div(Class("grid grid-cols-2 gap-2"),
					kit.Button{Variant: kit.Outline, Children: Text("GitHub")},
					kit.Button{Variant: kit.Outline, Children: Text("Google")}))),
		demo("Vertical with a caption", "A vertical separator takes the height of its row and puts the caption in the middle.", "",
			Div(Class("flex h-40 w-full max-w-md items-stretch gap-3"),
				panel(tr("separator.left", "Sign in")),
				kit.Separator{Orientation: kit.Vertical, Caption: Text(tr("separator.orShort", "or"))},
				panel(tr("separator.right", "Create an account")))),
		demo("Styled caption", "CaptionClass restyles the caption: here a pill with an uppercase label.", col,
			kit.Separator{CaptionClass: "rounded-full border border-border bg-card px-3 py-1 text-xs font-semibold tracking-wider uppercase", Caption: Text(tr("separator.today", "Today"))},
			kit.Separator{CaptionClass: "text-primary", Caption: Div(Class("flex items-center gap-1.5"), ic(icons.Star, "size-3.5"), Text(tr("separator.featured", "Featured")))}),
		demo("In a menu", "A separator groups the rows of a list.", col,
			Div(Class("flex w-64 flex-col gap-2 text-sm"),
				Span(Text(tr("separator.profile", "Profile"))), Span(Text(tr("separator.billing", "Billing"))),
				kit.Separator{Class: "my-1"},
				Span(Text(tr("separator.team", "Team"))), Span(Text(tr("separator.subscription", "Subscription"))),
				kit.Separator{Class: "my-1"},
				Span(Class("text-destructive"), Text(tr("separator.delete", "Delete account")))),
		),
	)
}

func skeletonPage() tela.Node {
	tr := useTr()
	loading, setLoading := tela.UseState(true)
	dot := func(class string) tela.Node {
		return Span(Class("size-2.5 rounded-full bg-primary", class))
	}
	// Each row pulses a little after the one above it.
	delays := []string{"", "[animation-delay:150ms]", "[animation-delay:300ms]", "[animation-delay:450ms]"}
	widths := []string{"w-48", "w-40", "w-44", "w-32"}
	rowsOf := func() []tela.Child {
		var out []tela.Child
		for i := range delays {
			out = append(out, Div(Key(i), Class("flex items-center gap-3 animate-pulse", delays[i]),
				Span(Class("size-8 shrink-0 rounded-full bg-accent")),
				Div(Class("flex flex-1 flex-col gap-1.5"),
					Span(Class("h-3 rounded-md bg-accent", widths[i])),
					Span(Class("h-2.5 w-1/2 rounded-md bg-accent")))))
		}
		return out
	}
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
		demo("Staggered rows", "animate-pulse with an animation-delay on each row, [animation-delay:150ms], [animation-delay:300ms]...: the rows pulse one after the other.", col,
			Div(Class("flex w-full max-w-xs flex-col gap-4"), AriaBusy(true), Fragment(rowsOf()...))),
		demo("A three-dot loader", "animate-bounce with a delay on each dot: the arbitrary [animation-delay:-0.3s] (a negative delay starts the dot part-way through its cycle) and delay-150, which sets the animation's delay as well as the transition's.", col,
			Div(Class("flex items-center gap-3"),
				Div(Class("flex items-center gap-1.5"), Role("status"), AriaLabel(tr("skeleton.loading", "Loading")),
					dot("animate-bounce [animation-delay:-0.3s]"), dot("animate-bounce [animation-delay:-0.15s]"), dot("animate-bounce")),
				Span(Class("text-sm text-muted-foreground"), Text(tr("skeleton.loadingText", "Loading..."))),
			),
			Div(Class("flex items-center gap-3"),
				Div(Class("flex items-center gap-1.5"), Role("status"), AriaLabel(tr("skeleton.thinking", "Thinking")),
					dot("animate-bounce"), dot("animate-bounce delay-150"), dot("animate-bounce delay-300")),
				Span(Class("text-sm text-muted-foreground"), Text(tr("skeleton.thinkingText", "Thinking..."))),
			)),
		demo("Loading, then loaded", "The same slot shows the skeleton while loading is true.", colStart,
			kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: func() { setLoading.Update(func(b bool) bool { return !b }) },
				Children: Text(map[bool]string{true: tr("skeleton.showContent", "Show the content"), false: tr("skeleton.showSkeleton", "Show the skeleton")}[loading])},
			IfElse(loading,
				Div(Class("flex items-center gap-4"), AriaBusy(true),
					kit.Skeleton{Class: "size-10 rounded-full"},
					Div(Class("flex flex-col gap-2"), kit.Skeleton{Class: "h-4 w-40"}, kit.Skeleton{Class: "h-3 w-28"})),
				Div(Class("flex items-center gap-4"),
					kit.Avatar{Size: kit.Lg, Fallback: "MT"},
					Div(Class("flex flex-col"), Span(Class("text-sm font-medium"), Text("Mei Tanaka")), muted(tr("skeleton.role", "Product designer"))))),
		),
	)
}
