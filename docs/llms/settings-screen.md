# Recipe: a complete settings screen

Questions: what does a whole, good-looking kit screen look like; an app shell with a sidebar, cards of switch rows, radio cards and a select; Save with a toast; how to structure a form as one comparable value. Copy its structure.

A compact, complete screen (one file, `package main`): an app shell with a sidebar whose current item has `AriaCurrent("page")`, a header with Save, a card of switch rows, a card with a `FieldSet` of radio cards and a horizontal `Field` with a `Select`; Save applies the theme with `tela.SetColorScheme` and shows a toast. It uses tokens only, so dark mode needs no `dark:` class. Copy its structure: `max-w-3xl` centered content, `gap-6` between cards, `text-2xl font-semibold tracking-tight` title with a `text-sm text-muted-foreground` subtitle, rows separated by `border-t first:border-t-0`. `examples/settings` is a larger screen built by an LLM from this guide and a prompt (in its README): five sections, validation, confirmations, an accent picked at run time.

```go
// Command settings is a settings screen on kit: an app shell with a sidebar
// and a page of switches, radio cards and a select.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"

	"github.com/kuronosu/tela/icons/lucide"
)

// Prefs is the whole form: comparable, so prefs != saved means "unsaved".
type Prefs struct {
	Email, Push, Digest bool
	Theme, Language     string
}

var defaults = Prefs{Email: true, Push: true, Theme: "system", Language: "en"}

type navItem struct {
	Label string
	Icon  tela.Icon
}

var nav = []navItem{
	{"Dashboard", lucide.LayoutDashboard}, {"Projects", lucide.FolderKanban},
	{"Team", lucide.Users}, {"Settings", lucide.Settings},
}

var themes = []kit.RadioItem[string]{
	{Value: "light", Label: "Light"}, {Value: "dark", Label: "Dark"}, {Value: "system", Label: "System"},
}

var themeIcons = map[string]tela.Icon{"light": lucide.Sun, "dark": lucide.Moon, "system": lucide.Monitor}

var schemes = map[string]tela.ColorScheme{"light": tela.Light, "dark": tela.Dark, "system": tela.System}

var languages = []kit.SelectItem[string]{
	{Value: "en", Label: "English"}, {Value: "es", Label: "Español"},
	{Value: "fr", Label: "Français"}, {Value: "de", Label: "Deutsch"},
}

// Settings is the screen; tela.C gives settings its own hook state.
func Settings() tela.Node { return tela.C(settings) }

func settings() tela.Node {
	page, setPage := tela.UseState("Settings")
	prefs, setPrefs := tela.UseState(defaults)
	saved, setSaved := tela.UseState(defaults)
	toast := kit.UseToast()

	edit := func(f func(p *Prefs)) { setPrefs.Update(func(p Prefs) Prefs { f(&p); return p }) }
	save := func() {
		setSaved.Set(prefs)
		tela.SetColorScheme(schemes[prefs.Theme])
		toast.Success("Settings saved", kit.ToastDescription("Your preferences were updated."))
	}
	row := func(label, hint string, on bool, set func(p *Prefs, on bool)) tela.Node {
		return kit.Field{
			Orientation: kit.Horizontal, ControlAfter: true, Label: label, Description: hint,
			Class:    "items-center gap-6 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0",
			Children: kit.Switch{Checked: &on, OnCheckedChange: func(v bool) { edit(func(p *Prefs) { set(p, v) }) }},
		}
	}

	sidebar := Div(Role("navigation"), AriaLabel("Main"),
		Class("flex w-60 shrink-0 flex-col gap-1 border-r border-border bg-muted/40 p-3"),
		Div(Class("flex items-center gap-2.5 px-2 pt-1 pb-5"),
			Div(Class("flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"),
				Icon(lucide.Layers, Class("size-4"))),
			Span(Class("font-semibold tracking-tight"), Text("Acme"))),
		Map(nav, func(n navItem) tela.Node {
			current := ""
			if n.Label == page {
				current = "page"
			}
			return Button(Key(n.Label), AriaCurrent(current), OnClick(func() { setPage.Set(n.Label) }),
				Class("flex h-9 items-center gap-3 rounded-md px-3 text-sm text-muted-foreground transition-colors",
					"hover:bg-accent hover:text-accent-foreground",
					"aria-[current=page]:bg-accent aria-[current=page]:font-medium aria-[current=page]:text-foreground"),
				Icon(n.Icon, Class("size-4")), Text(n.Label))
		}),
		Div(Class("mt-auto flex items-center gap-3 border-t border-border px-2 pt-4"),
			kit.Avatar{Fallback: "AL"},
			Div(Class("flex min-w-0 flex-col text-sm"),
				Span(Class("font-medium"), Text("Ada Lovelace")),
				Span(Class("truncate text-xs text-muted-foreground"), Text("ada@example.com")))),
	)

	content := Div(Class("mx-auto flex max-w-3xl flex-col gap-6 px-8 py-8"),
		Div(Class("flex items-end justify-between gap-4"),
			Div(Class("flex flex-col gap-1"),
				H1(Class("text-2xl font-semibold tracking-tight"), Text("Settings")),
				P(Class("text-sm text-muted-foreground"), Text("Manage your notifications and how Acme looks."))),
			kit.Button{Icon: lucide.Check, Disabled: prefs == saved, OnClick: save, Children: Text("Save changes")}),
		kit.Card{TitleLevel: 2, Title: Text("Notifications"), Description: Text("Choose what we tell you about."),
			Children: Div(Class("flex flex-col"),
				row("Email notifications", "A summary of activity in your workspace.", prefs.Email, func(p *Prefs, v bool) { p.Email = v }),
				row("Push notifications", "Mentions and replies, on your devices.", prefs.Push, func(p *Prefs, v bool) { p.Push = v }),
				row("Weekly digest", "A Monday recap of what your team shipped.", prefs.Digest, func(p *Prefs, v bool) { p.Digest = v }))},
		kit.Card{TitleLevel: 2, Title: Text("Appearance"), Description: Text("Applied when you save."),
			ContentClass: "flex flex-col gap-6",
			Children: Fragment(
				kit.FieldSet{Legend: "Theme", LegendVariant: kit.LegendVariantLabel,
					Children: kit.RadioGroup[string]{Items: themes, Render: themeCard, Class: "grid-cols-3",
						Value: &prefs.Theme, OnValueChange: func(v string) { edit(func(p *Prefs) { p.Theme = v }) }}},
				kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center gap-6",
					Label: "Language", Description: "Used for dates, numbers and messages.",
					Children: kit.Select[string]{Items: languages, Class: "w-44",
						Value: &prefs.Language, OnValueChange: func(v string) { edit(func(p *Prefs) { p.Language = v }) }}})},
	)

	return Div(Class("flex h-screen"), sidebar,
		Div(Class("min-w-0 flex-1 overflow-y-auto"),
			IfElse(page == "Settings", content, Div(Class("flex h-full items-center justify-center text-sm text-muted-foreground"),
				Textf("%s is empty in this example.", page)))))
}

// themeCard draws one theme option; st.Props makes the button the radio.
func themeCard(o kit.RadioItem[string], st kit.RadioState) tela.Node {
	return Button(Key(o.Value), st.Props,
		Class("flex items-center justify-center gap-2 rounded-lg border border-border bg-card px-3 py-2.5 text-sm font-medium shadow-xs transition-colors",
			"hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50",
			"aria-checked:border-primary aria-checked:bg-accent aria-checked:ring-1 aria-checked:ring-primary"),
		Icon(themeIcons[o.Value], Class("size-4")), Text(o.Label))
}

var theme = kit.Theme()

// App is the window; tela snapshot, tela mcp and the tests mount it too.
var App = tela.App{
	Title: "Settings", Width: 1040, Height: 800,
	Theme: &theme, RootClass: "bg-background text-foreground",
	Root: kit.Toaster{Children: Settings()},
}

func main() { tela.Main(App) }
```

Its aria snapshot (`tela inspect`, or the `snapshot` tool, which adds refs):

```yaml
- navigation "Main":
  - text: Acme
  - button "Dashboard"
  - button "Projects"
  - button "Team"
  - button "Settings"
  - text: AL Ada Lovelace ada@example.com
- heading "Settings" [level=1]
- paragraph: Manage your notifications and how Acme looks.
- button "Save changes" [disabled]
- heading "Notifications" [level=2]
- text: Choose what we tell you about.
- group:
  - text: Email notifications
  - paragraph: A summary of activity in your workspace.
  - switch "Email notifications" [checked]
...
- group "Theme":
  - text: Theme
  - radiogroup "Theme":
    - radio "Light"
    - radio "Dark"
    - radio "System" [checked]
- group:
  - text: Language
  - paragraph: Used for dates, numbers and messages.
  - button "English Language": English
- region "Notifications"
```

(`aria-current` is not printed in snapshots, as in Playwright; `region "Notifications"` is the Toaster's.)
