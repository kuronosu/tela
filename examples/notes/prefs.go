package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The keys of the settings (desktop.UseSetting): the store settings.json
// in desktop.ConfigDir holds them.
const (
	keyFontSize     = "fontSize"
	keyWordWrap     = "wordWrap"
	keyKeepInTray   = "keepInTray"
	keyNotifyOnSave = "notifyOnSave"
)

// Prefs are the settings, as the main window reads them.
type Prefs struct {
	// FontSize is "small", "medium" or "large".
	FontSize string
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
	size, _ := desktop.UseSetting(keyFontSize, "medium")
	wrap, _ := desktop.UseSetting(keyWordWrap, true)
	tray, _ := desktop.UseSetting(keyKeepInTray, false)
	notify, _ := desktop.UseSetting(keyNotifyOnSave, true)
	return Prefs{FontSize: size, WordWrap: wrap, KeepInTray: tray, NotifyOnSave: notify}
}

// fontClass is the Tailwind class of a font size. The kit's Textarea has
// text-base and md:text-sm, so the class repeats at the md breakpoint.
func fontClass(size string) string {
	switch size {
	case "small":
		return "text-xs md:text-xs"
	case "large":
		return "text-lg md:text-lg"
	}
	return "text-sm md:text-sm"
}

// Preferences is the content of the Preferences window: the settings, each
// one a desktop.UseSetting. Changing one stores it (settings.json) and the
// main window, which reads the same key, follows.
type Preferences struct{}

func (Preferences) Build() tela.Node {
	size, setSize := desktop.UseSetting(keyFontSize, "medium")
	wrap, setWrap := desktop.UseSetting(keyWordWrap, true)
	tray, setTray := desktop.UseSetting(keyKeepInTray, false)
	notify, setNotify := desktop.UseSetting(keyNotifyOnSave, true)

	return Div(Class("flex h-screen flex-col gap-6 overflow-y-auto p-6"),
		Div(Class("flex flex-col gap-1"),
			H1(Class("text-xl font-semibold tracking-tight"), Text("Preferences")),
			P(Class("text-sm text-muted-foreground"), Text("Saved on this computer and shared by every window.")),
		),
		kit.FieldSet{
			Legend: "Font size", LegendVariant: kit.LegendVariantLabel,
			Children: kit.RadioGroup[string]{
				Orientation: kit.Horizontal,
				Items: []kit.RadioItem[string]{
					{Value: "small", Label: "Small"},
					{Value: "medium", Label: "Medium"},
					{Value: "large", Label: "Large"},
				},
				Value: &size, OnValueChange: setSize.Set,
			},
		},
		Div(Class("flex flex-col"),
			row("Word wrap", "Break long lines at the edge of the editor.", &wrap, setWrap.Set),
			row("Keep running in the tray when the window is closed", "Closing the window hides it. Quit from the tray menu.", &tray, setTray.Set),
			row("Notify when a file is saved", "Show a desktop notification after Save.", &notify, setNotify.Set),
		),
	)
}

// row is a label and description with a switch after them.
func row(label, hint string, on *bool, set func(bool)) tela.Node {
	return Fragment(kit.Field{
		Orientation: kit.Horizontal, ControlAfter: true,
		Label: label, Description: hint,
		Class:    "items-center gap-6 border-t border-border py-4 first:border-t-0 first:pt-0",
		Children: kit.Switch{Checked: on, OnCheckedChange: set},
	})
}
