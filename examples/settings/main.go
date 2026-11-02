// Command settings is Darkroom's Preferences window, a larger screen than the
// one in docs/llms.txt: five sections, a form that is one comparable value,
// validation, confirmations and a theme applied on Save. An LLM built it
// from docs/llms.txt and the prompt in README.md.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"

	"github.com/kuronosu/tela/examples/settings/lucide"
)

type page struct {
	Key, Label, Eyebrow, Blurb string
	Icon                       tela.Icon
}

var pages = []page{
	{"General", "General", "The room", "Where your negatives live and how they come in.", lucide.House},
	{"Develop", "Develop", "The bath", "The look every new photo gets before you touch it.", lucide.SlidersHorizontal},
	{"Export", "Export", "The print", "How finished frames leave the darkroom.", lucide.Upload},
	{"Safelight", "Safelight", "The light", "Dim the room, pick your red, size the contact sheet.", lucide.Lamp},
	{"Account", "Account", "The ledger", "Who you are and what the archive costs.", lucide.User},
}

// Preferences is the window; Start (a section) and Scenario (a Prefs from
// scenario) only choose its first state, for the previews.
type Preferences struct{ Start, Scenario string }

func (w Preferences) Build() tela.Node {
	start := w.Start
	if start == "" {
		start = "General"
	}
	cur, setCur := tela.UseState(start)
	prefs, setPrefs := tela.UseState(scenario(w.Scenario))
	saved, setSaved := tela.UseState(defaults)
	toast := kit.UseToast()

	f := form{p: prefs, gap: densityGap[saved.Density], edit: func(fn func(p *Prefs)) { setPrefs.Update(func(p Prefs) Prefs { fn(&p); return p }) }, toast: toast}
	dirty := prefs != saved
	valid := prefs.valid()

	save := func() {
		if !dirty || !valid {
			return
		}
		setSaved.Set(prefs)
		tela.SetColorScheme(schemes[prefs.Theme])
		t := makeTheme(prefs.Accent)
		tela.SetTheme(t)
		toast.Success("Settings fixed.", kit.ToastDescription("Out of the bath and into the light."))
	}

	var pg page
	for _, p := range pages {
		if p.Key == cur {
			pg = p
		}
	}

	sidebar := Div(Role("navigation"), AriaLabel("Preferences sections"),
		Class("flex w-60 shrink-0 flex-col gap-1 border-r border-border bg-card p-3"),
		Div(Class("flex items-center gap-3 px-2 pt-2 pb-6"),
			Div(Class("relative flex size-9 items-center justify-center rounded-full bg-primary/20"),
				Div(Class("size-4 rounded-full bg-primary shadow-[0_0_14px_3px] shadow-primary/60"))),
			Div(Class("flex flex-col"),
				Span(Class("text-base leading-tight font-semibold tracking-tight"), Text("Darkroom")),
				Span(Class("text-xs text-muted-foreground"), Text("Preferences")))),
		Map(pages, func(n page) tela.Node {
			current := ""
			if n.Key == cur {
				current = "page"
			}
			unsaved := prefs.changed(n.Key, saved)
			note := ""
			if unsaved {
				note = "Unsaved changes"
			}
			return Button(Key(n.Key), AriaCurrent(current), AriaDescription(note), OnClick(func() { setCur.Set(n.Key) }),
				Class("group relative flex h-10 items-center gap-3 rounded-md px-3 text-sm text-muted-foreground transition-colors outline-none",
					"hover:bg-accent hover:text-accent-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50",
					"aria-[current=page]:bg-accent aria-[current=page]:font-medium aria-[current=page]:text-foreground"),
				Icon(n.Icon, Class("size-4 group-aria-[current=page]:text-primary")), Text(n.Label),
				If(unsaved, Span(Class("ml-auto size-1.5 rounded-full bg-primary"))))
		}),
		Div(Class("mt-auto flex items-center gap-3 border-t border-border px-2 pt-4"),
			kit.Avatar{Fallback: initials(prefs.Name)},
			Div(Class("flex min-w-0 flex-col text-sm"),
				Span(Class("truncate font-medium"), Text(prefs.Name)),
				Span(Class("text-xs text-muted-foreground"), Text("Fixer plan")))),
	)

	var body tela.Node
	switch cur {
	case "General":
		body = f.general()
	case "Develop":
		body = f.develop()
	case "Export":
		body = f.export()
	case "Safelight":
		body = f.safelight()
	default:
		body = f.account()
	}

	status := Div(Class("flex items-center gap-2 text-sm text-muted-foreground"), Role("status"),
		Span(Class("size-2 rounded-full bg-muted-foreground/40")), Text("All changes are fixed."))
	if dirty {
		status = Div(Class("flex items-center gap-2 text-sm font-medium text-foreground"), Role("status"),
			Span(Class("size-2 rounded-full bg-primary shadow-[0_0_8px_2px] shadow-primary/50")),
			Text("Unsaved changes: still in the bath."))
	}
	if !valid {
		status = Div(Class("flex items-center gap-2 text-sm font-medium text-destructive"), Role("status"),
			Icon(lucide.CircleAlert, Class("size-4")), Text("Fix the highlighted fields to save."))
	}

	main := Div(Class("flex min-w-0 flex-1 flex-col"),
		Div(Class("min-h-0 flex-1 overflow-y-auto"),
			Div(Class("mx-auto flex max-w-3xl flex-col px-10 py-8 "+f.gap),
				Div(Class("flex flex-col gap-1.5"),
					Span(Class("text-xs font-semibold tracking-[0.2em] text-primary uppercase"), Text(pg.Eyebrow)),
					H1(Class("text-3xl font-semibold tracking-tight"), Text(pg.Label)),
					P(Class("text-sm text-muted-foreground"), Text(pg.Blurb))),
				body)),
		Div(Class("flex shrink-0 items-center justify-between gap-4 border-t border-border bg-card/60 px-10 py-3.5"),
			status,
			Div(Class("flex items-center gap-2"),
				kit.AlertDialog{
					Disabled:    !dirty,
					Trigger:     kit.Button{Variant: kit.Outline, Icon: lucide.Undo2, Children: Text("Discard")},
					Title:       Text("Discard your changes?"),
					Description: Text("Everything you changed since the last save goes back to how it was. This can't be undone."),
					Cancel:      Text("Keep editing"), Action: Text("Discard"), Destructive: true,
					OnAction: func() { setPrefs.Set(saved) },
				},
				kit.Button{Icon: lucide.Check, Disabled: !dirty || !valid, OnClick: save, Children: Text("Save")})))

	return Div(Class("flex h-screen"),
		OnKeyDown(func(e tela.KeyboardEvent) {
			// Ctrl+S, or Cmd+S on macOS; a child that has the focus bubbles it up.
			if e.CmdOrCtrl() && e.Key == "s" {
				e.PreventDefault()
				save()
			}
		}),
		sidebar, main)
}

func initials(name string) string {
	out := ""
	prev := true
	for _, r := range name {
		if prev && r != ' ' {
			out += string(r)
		}
		prev = r == ' '
	}
	return out
}

var schemes = map[string]tela.ColorScheme{"light": tela.Light, "dark": tela.Dark, "system": tela.System}

var theme = makeTheme(safelight)

var App = tela.App{
	Title: "Darkroom Preferences", Width: 1100, Height: 760,
	Theme: &theme, RootClass: "bg-background text-foreground",
	Root: kit.Toaster{Children: Preferences{}},
}

func preview(start, sc string) tela.Preview {
	return tela.Preview{Root: kit.Toaster{Children: Preferences{Start: start, Scenario: sc}}}
}

var Previews = tela.Previews{
	"General":   preview("General", ""),
	"Develop":   preview("Develop", ""),
	"Export":    preview("Export", ""),
	"Safelight": preview("Safelight", ""),
	"Account":   preview("Account", ""),
	"Unsaved":   preview("Develop", "Unsaved"),
	"Invalid":   preview("Account", "Invalid"),
}

func main() { tela.Main(App) }
