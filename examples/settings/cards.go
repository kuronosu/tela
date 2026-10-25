package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/settings/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// NotificationsCard lists one switch per kind of notification. Its fields
// are comparable, so it is built again only when the form changes.
type NotificationsCard struct {
	Prefs Prefs
	Set   tela.Setter[Prefs]
}

func (p NotificationsCard) Build() tela.Node {
	return Fragment(kit.Card{
		Class:        "min-w-[340px] flex-1 basis-[420px]",
		Title:        H2(Text("Notifications")),
		Description:  Text("Choose what you want to be told about."),
		ContentClass: "flex flex-1 flex-col",
		Children: Fragment(
			MapKeyed(notifications, func(n Notification) Field { return n.Field }, func(n Notification) tela.Component {
				return SettingRow{Field: n.Field, Title: n.Title, Hint: n.Hint, On: p.Prefs.Bool(n.Field), Set: p.Set}
			}),
			SettingRow{
				Field: FieldSecurity, Title: "Security alerts", Hint: "Always on: sign-ins from new devices.",
				On: true, Locked: true, Set: p.Set,
			},
		),
	})
}

// SettingRow is a label with a description and its switch: a horizontal
// Field whose control comes after the text (flex-row-reverse). Rows after
// the first have a top border (first:border-t-0).
type SettingRow struct {
	Field       Field
	Title, Hint string
	On, Locked  bool
	Set         tela.Setter[Prefs]
}

func (p SettingRow) Build() tela.Node {
	on := p.On
	return Fragment(kit.Field{
		Orientation: kit.Horizontal,
		Label:       p.Title,
		Description: p.Hint,
		Class:       "flex-1 flex-row-reverse items-center justify-between gap-6 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0",
		Children: kit.Switch{
			Checked: &on, Disabled: p.Locked,
			OnCheckedChange: func(bool) { p.Set.Update(func(x Prefs) Prefs { return x.Toggle(p.Field) }) },
		},
	})
}

// The caption of a group of controls (shadcn's FieldLegend, which kit does
// not have) and the column that holds it and the group.
const (
	caption = "text-sm leading-none font-medium"
	group   = "flex flex-col gap-3"
)

// AppearanceCard has a theme RadioGroup, whose options are cards with a
// miniature of each theme, and a density ToggleGroup (a segmented control).
type AppearanceCard struct {
	Prefs Prefs
	Set   tela.Setter[Prefs]
}

func (p AppearanceCard) Build() tela.Node {
	theme, density := p.Prefs.Theme, p.Prefs.Density
	items := make([]kit.RadioItem[string], len(themes))
	for i, t := range themes {
		items[i] = kit.RadioItem[string]{Value: t.Value, Label: t.Title}
	}
	segments := make([]kit.ToggleGroupItem[string], len(densities))
	var hint string
	for i, d := range densities {
		segments[i] = kit.ToggleGroupItem[string]{Value: d.Value, Label: d.Title}
		if d.Value == density {
			hint = d.Hint
		}
	}
	return Fragment(kit.Card{
		Class:        "min-w-[340px] flex-1 basis-[420px]",
		Title:        H2(Text("Appearance")),
		Description:  Text("Pick a theme and how much room the interface takes."),
		ContentClass: "flex flex-col gap-6",
		Children: Fragment(
			Div(Class(group), Span(Class(caption), Text("Theme")),
				kit.RadioGroup[string]{
					AriaLabel: "Theme", Items: items, Render: themeCard, Class: "grid-cols-3",
					Value: &theme, OnValueChange: func(v string) {
						p.Set.Update(func(x Prefs) Prefs { return x.Choose(FieldTheme, v) })
					},
				}),
			Div(Class(group), Span(Class(caption), Text("Density")),
				kit.ToggleGroup[string]{
					AriaLabel: "Density", Items: segments, Variant: kit.Outline, DisallowEmptySelection: true,
					Class: "w-full", ItemClass: "flex-1",
					Value: &density, OnValueChange: func(v string) {
						p.Set.Update(func(x Prefs) Prefs { return x.Choose(FieldDensity, v) })
					},
				},
				P(Class("text-sm text-muted-foreground"), Text(hint)),
			),
		),
	})
}

// Footer shows the save state and the Reset and Save changes buttons,
// which are disabled while there is nothing to save. Reset asks first, with
// an AlertDialog. Save and Reset are Callback handles (UseCallback):
// comparable, so the footer rebuilds only when Dirty changes.
type Footer struct {
	Dirty       bool
	Save, Reset tela.Callback[func()]
}

func (p Footer) Build() tela.Node {
	status := Div(Class("flex items-center gap-2 text-sm text-muted-foreground"),
		Icon(icons.CircleCheck, Class("size-4 text-emerald-600 dark:text-emerald-400")),
		Text("All changes saved"))
	if p.Dirty {
		status = Div(Class("flex items-center gap-2 text-sm font-medium"),
			Icon(icons.CircleAlert, Class("size-4 text-amber-600 dark:text-amber-400")),
			Text("You have unsaved changes"))
	}
	return Fragment(kit.Card{
		Class: "py-4",
		Children: Div(Class("flex flex-wrap items-center justify-between gap-4"),
			status,
			Div(Class("flex gap-3"),
				kit.AlertDialog{
					Trigger:     kit.Button{Variant: kit.Outline, Children: Text("Reset")},
					Disabled:    !p.Dirty,
					Title:       Text("Discard your changes?"),
					Description: Text("The form goes back to the last saved settings. This cannot be undone."),
					Action:      Text("Discard changes"),
					Destructive: true,
					OnAction:    p.Reset.Fn(),
				},
				kit.Button{Disabled: !p.Dirty, OnClick: p.Save.Fn(), Children: Text("Save changes")},
			),
		),
	})
}
