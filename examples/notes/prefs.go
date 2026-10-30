package main

import (
	"runtime"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The keys of the settings (desktop.UseSetting): the store settings.json
// in desktop.ConfigDir holds them.
const (
	keyTheme        = "theme"
	keyAccent       = "accent"
	keyShowPreviews = "showPreviews"
	keyFontSize     = "fontSize"
	keyLineSpacing  = "lineSpacing"
	keyWordWrap     = "wordWrap"
	keyKeepInTray   = "keepInTray"
	keyNotifyOnSave = "notifyOnSave"
)

// Prefs are the settings, as the main window reads them.
type Prefs struct {
	// Theme is "light", "dark" or "system".
	Theme string
	// Accent is the name of one of accents.
	Accent string
	// ShowPreviews shows the first lines of a note in the list.
	ShowPreviews bool
	// FontSize is "small", "medium" or "large".
	FontSize string
	// LineSpacing is "tight", "relaxed" or "loose".
	LineSpacing string
	// WordWrap wraps long lines in the editor.
	WordWrap bool
	// KeepInTray makes closing the window hide it: Notes keeps running in
	// the tray until Quit.
	KeepInTray bool
	// NotifyOnSave shows a desktop notification after a file is saved.
	NotifyOnSave bool
}

// usePrefs reads the settings. Each desktop.UseSetting is a state that the
// store keeps in sync: when the Preferences window (another window, with
// its own UseSetting calls on the same keys) changes one, this component
// builds again with the new value.
func usePrefs() Prefs {
	theme, _ := desktop.UseSetting(keyTheme, "system")
	accent, _ := desktop.UseSetting(keyAccent, accents[0].Name)
	previews, _ := desktop.UseSetting(keyShowPreviews, true)
	size, _ := desktop.UseSetting(keyFontSize, "medium")
	spacing, _ := desktop.UseSetting(keyLineSpacing, "relaxed")
	wrap, _ := desktop.UseSetting(keyWordWrap, true)
	tray, _ := desktop.UseSetting(keyKeepInTray, false)
	notify, _ := desktop.UseSetting(keyNotifyOnSave, true)
	return Prefs{Theme: theme, Accent: accent, ShowPreviews: previews, FontSize: size, LineSpacing: spacing,
		WordWrap: wrap, KeepInTray: tray, NotifyOnSave: notify}
}

// fontClass is the Tailwind class of a font size of the editor.
func fontClass(size string) string {
	switch size {
	case "small":
		return "text-[13px]"
	case "large":
		return "text-[18px]"
	}
	return "text-[15px]"
}

// spacingClass is the Tailwind class of a line spacing of the editor.
func spacingClass(spacing string) string {
	switch spacing {
	case "tight":
		return "leading-normal"
	case "loose":
		return "leading-loose"
	}
	return "leading-[1.7]"
}

// glass is the class of a surface of the Preferences window: the opaque one,
// and on Windows the translucent one that lets the Mica backdrop of the
// window show through (everywhere else the backdrop is ignored, and a
// translucent surface would only be mixed with the window's clear color).
func glass(opaque, translucent string) string {
	if runtime.GOOS == "windows" {
		return translucent
	}
	return opaque
}

// The sections of Preferences.
type section struct {
	ID, Label string
	Icon      tela.Icon
}

var sections = []section{
	{"appearance", "Appearance", icons.Palette},
	{"editor", "Editor", icons.Type},
	{"general", "General", icons.SlidersHorizontal},
	{"shortcuts", "Shortcuts", icons.Keyboard},
}

// Preferences is the content of the Preferences window: a column of
// sections and the rows of the chosen one, each row a desktop.UseSetting.
// Changing one stores it (settings.json) and the main window, which reads
// the same key, follows. Tab is the section the window starts on ("" is
// Appearance).
type Preferences struct{ Tab string }

func (p Preferences) Build() tela.Node {
	tab, setTab := tela.UseState(p.Tab)
	if tab == "" {
		tab = "appearance"
	}
	// GNOME on Wayland draws no title bars: without one the window could
	// not be moved or closed, so draw the kit's there.
	csd := tela.UseWindow().State().ClientSideDecorations

	return Div(Class("flex h-screen flex-col"),
		If(csd, kit.TitleBar{Title: Text("Preferences"), HideMinimize: true, HideMaximize: true}),
		Div(Class("flex min-h-0 flex-1"),
			Div(Class("flex w-40 shrink-0 flex-col gap-0.5 p-2 pt-4", glass("bg-sidebar", "bg-sidebar/60")), Role("navigation"), AriaLabel("Sections"),
				Span(Class("px-2.5 pb-3 text-base font-semibold tracking-tight"), Text("Preferences")),
				Map(sections, func(s section) tela.Node {
					current := ""
					if s.ID == tab {
						current = "page"
					}
					return Button(Key(s.ID), AriaCurrent(current), OnClick(func() { setTab.Set(s.ID) }),
						Class("flex h-8 w-full items-center gap-2.5 rounded-md px-2.5 text-start text-[13px] text-foreground/70 outline-none transition-colors",
							"hover:bg-accent/60 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40",
							"aria-[current=page]:bg-accent aria-[current=page]:font-medium aria-[current=page]:text-foreground"),
						Icon(s.Icon, Class("size-4 text-muted-foreground")), Text(s.Label))
				}),
			),
			Div(Class("flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto border-s border-border px-6 py-5"),
				tela.WithKey(sectionPane{Tab: tab}, tab),
			),
		),
	)
}

// sectionPane is the content of the chosen section.
type sectionPane struct{ Tab string }

func (s sectionPane) Build() tela.Node {
	switch s.Tab {
	case "editor":
		return Fragment(editorSection{})
	case "general":
		return Fragment(generalSection{})
	case "shortcuts":
		return Fragment(shortcutsSection{})
	}
	return Fragment(appearanceSection{})
}

// appearanceSection is the theme as three small previews of the window, the
// accent as swatches, and the list previews.
type appearanceSection struct{}

func (appearanceSection) Build() tela.Node {
	theme, setTheme := desktop.UseSetting(keyTheme, "system")
	current, setAccent := desktop.UseSetting(keyAccent, accents[0].Name)
	previews, setPreviews := desktop.UseSetting(keyShowPreviews, true)
	return Fragment(
		Div(Class("flex flex-col gap-3"),
			heading("Theme", "Follow the system, or choose one."),
			Div(Class("grid grid-cols-3 gap-3"),
				themeTile("light", "Light", theme, setTheme.Set),
				themeTile("dark", "Dark", theme, setTheme.Set),
				themeTile("system", "System", theme, setTheme.Set),
			),
		),
		Div(Class("flex flex-col gap-3"),
			heading("Accent", ""),
			Div(Class("flex gap-2.5"), Role("group"), AriaLabel("Accent"),
				Map(accents, func(a accent) tela.Node { return swatch(a, current, setAccent.Set) }),
			),
		),
		Div(Class("flex flex-col"),
			row("Show previews in the list", "The first lines of each note under its title.",
				kit.Switch{AriaLabel: "Show previews in the list", Checked: &previews, OnCheckedChange: setPreviews.Set}),
		),
	)
}

// editorSection has the font size, the line spacing and word wrap.
type editorSection struct{}

func (editorSection) Build() tela.Node {
	size, setSize := desktop.UseSetting(keyFontSize, "medium")
	spacing, setSpacing := desktop.UseSetting(keyLineSpacing, "relaxed")
	wrap, setWrap := desktop.UseSetting(keyWordWrap, true)
	return Div(Class("flex flex-col"),
		row("Font size", "", segmented("Font size", &size, setSize.Set, "small", "Small", "medium", "Medium", "large", "Large")),
		row("Line spacing", "", segmented("Line spacing", &spacing, setSpacing.Set, "tight", "Tight", "relaxed", "Relaxed", "loose", "Loose")),
		row("Word wrap", "Break long lines at the edge of the editor.",
			kit.Switch{AriaLabel: "Word wrap", Checked: &wrap, OnCheckedChange: setWrap.Set}),
	)
}

// generalSection is what closing the window and saving do.
type generalSection struct{}

func (generalSection) Build() tela.Node {
	tray, setTray := desktop.UseSetting(keyKeepInTray, false)
	notify, setNotify := desktop.UseSetting(keyNotifyOnSave, true)
	return Div(Class("flex flex-col"),
		row("Keep running in the tray", "Closing the window hides it. Quit from the tray menu.",
			kit.Switch{AriaLabel: "Keep running in the tray", Checked: &tray, OnCheckedChange: setTray.Set}),
		row("Notify when a file is saved", "Show a desktop notification after Save.",
			kit.Switch{AriaLabel: "Notify when a file is saved", Checked: &notify, OnCheckedChange: setNotify.Set}),
	)
}

// shortcutsSection lists the keys the app answers to.
type shortcutsSection struct{}

func (shortcutsSection) Build() tela.Node {
	mod := "Ctrl"
	if runtime.GOOS == "darwin" {
		mod = "Cmd"
	}
	keys := [][2]string{
		{"New note", mod + " N"},
		{"Open a file", mod + " O"},
		{"Save", mod + " S"},
		{"Preferences", mod + " ,"},
		{"Delete the open note", mod + " Shift D"},
		{"Back to all notes, from the list", "Esc"},
	}
	return Div(Class("flex flex-col"), Role("list"), AriaLabel("Shortcuts"),
		Map(keys, func(k [2]string) tela.Node {
			return row(k[0], "", Span(Class("rounded-md border border-border px-2 py-0.5 font-mono text-xs text-muted-foreground"), Text(k[1])))
		}))
}

// heading is the title of a group of controls, with an optional hint.
func heading(title, hint string) tela.Node {
	return Div(Class("flex flex-col"),
		Span(Class("text-sm font-semibold"), Text(title)),
		If(hint != "", Span(Class("text-[13px] text-muted-foreground"), Text(hint))),
	)
}

// row is a label and an optional hint with a control after them.
func row(label, hint string, control tela.Component) tela.Node {
	return Div(Key(label), Class("flex min-h-12 items-center justify-between gap-4 border-t border-border py-2 first:border-t-0"),
		Div(Class("flex min-w-0 flex-col"),
			Span(Class("text-[13px] font-medium"), Text(label)),
			If(hint != "", Span(Class("text-xs text-muted-foreground"), Text(hint))),
		),
		control,
	)
}

// segmented is a group of toggle buttons for a choice among a few strings
// (value, label pairs).
func segmented(label string, value *string, set func(string), pairs ...string) tela.Component {
	var items []kit.ToggleGroupItem[string]
	for i := 0; i < len(pairs); i += 2 {
		items = append(items, kit.ToggleGroupItem[string]{Value: pairs[i], Label: pairs[i+1]})
	}
	return kit.ToggleGroup[string]{AriaLabel: label, Variant: kit.Outline, Size: kit.Sm, DisallowEmptySelection: true,
		Value: value, OnValueChange: set, Items: items}
}

// themeTile is a miniature of the window in a scheme; the chosen one has the
// accent border and a check.
func themeTile(id, label, current string, set func(string)) tela.Node {
	line := func(class string) tela.Node { return Div(Class("rounded-full", class)) }
	half := func(s tela.ColorScheme) tela.Node {
		return Div(Scheme(s), Class("w-1/2 bg-background p-2"),
			line("h-1.5 w-8 bg-foreground/80"), Div(Class("mt-1"), line("h-1 w-full bg-muted-foreground/40")))
	}
	var mini tela.Node
	if id == "system" {
		mini = Div(Class("flex h-16 overflow-hidden rounded-md border border-border"), half(tela.Light), half(tela.Dark))
	} else {
		s := tela.Light
		if id == "dark" {
			s = tela.Dark
		}
		mini = Div(Scheme(s), Class("flex h-16 overflow-hidden rounded-md border border-border bg-background"),
			Div(Class("w-5 shrink-0 bg-sidebar")),
			Div(Class("flex w-9 shrink-0 flex-col gap-1 border-e border-border p-1.5"),
				Div(Class("h-2 rounded-sm bg-muted")), Div(Class("h-2 rounded-sm bg-muted"))),
			Div(Class("flex flex-1 flex-col gap-1 p-2"),
				line("h-1.5 w-8 bg-foreground/80"), line("h-1 w-full bg-muted-foreground/40"), line("h-1 w-3/4 bg-muted-foreground/40")),
		)
	}
	return Button(Key(id), AriaPressed(id == current), AriaLabel(label), OnClick(func() { set(id) }),
		Class("flex flex-col gap-2 rounded-xl border border-border p-2 text-start outline-none transition-colors",
			"hover:border-input focus-visible:ring-[3px] focus-visible:ring-ring/40",
			"aria-pressed:border-gold aria-pressed:ring-1 aria-pressed:ring-gold/40"),
		mini,
		Span(Class("flex items-center justify-between px-1 text-[13px] font-medium"), Text(label),
			If(id == current, Icon(icons.Check, Class("size-3.5 text-gold")))),
	)
}

// swatch is a color of the accent.
func swatch(a accent, current string, set func(string)) tela.Node {
	return Button(Key(a.Name), AriaPressed(a.Name == current), AriaLabel(a.Label), OnClick(func() { set(a.Name) }),
		Class("flex size-7 items-center justify-center rounded-full border border-transparent outline-none",
			"aria-pressed:border-foreground/70 focus-visible:ring-[3px] focus-visible:ring-ring/40"),
		Span(Class("size-5 rounded-full bg-["+a.Fill+"]")))
}
