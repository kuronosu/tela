package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

const (
	card  = "rounded-xl border border-zinc-200 bg-white shadow-xs dark:border-zinc-800 dark:bg-zinc-900"
	muted = "text-zinc-500 dark:text-zinc-400"
)

// cardHeader is the title and description at the top of a card.
func cardHeader(title, description string) tela.Node {
	return Div(Class("flex flex-col gap-1.5 px-6 pt-6"),
		H2(Class("leading-none font-semibold tracking-tight"), Text(title)),
		P(Class("text-sm "+muted), Text(description)),
	)
}

// NotificationsProps are the props of NotificationsCard.
type NotificationsProps struct {
	Prefs Prefs
	Set   tela.Setter[Prefs]
}

// NotificationsCard lists one switch per kind of notification.
func NotificationsCard(p NotificationsProps) tela.Node {
	return Div(Class(card+" flex min-w-[340px] flex-1 basis-[420px] flex-col pb-2"),
		cardHeader("Notifications", "Choose what you want to be told about."),
		Div(Class("mt-4 flex flex-1 flex-col"),
			Map(notifications, func(n Notification) tela.Node {
				return tela.WithKey(tela.CP(SettingRow, RowProps{
					Field: n.Field, Title: n.Title, Hint: n.Hint, On: p.Prefs.Bool(n.Field), Set: p.Set,
				}), n.Field)
			}),
			tela.WithKey(tela.CP(SettingRow, RowProps{
				Field: FieldSecurity, Title: "Security alerts", Hint: "Always on: sign-ins from new devices.",
				On: true, Locked: true, Set: p.Set,
			}), FieldSecurity),
		),
	)
}

// RowProps are the props of SettingRow.
type RowProps struct {
	Field       Field
	Title, Hint string
	On, Locked  bool
	Set         tela.Setter[Prefs]
}

// SettingRow is a label with a description and its switch. Rows after the
// first have a top border (first:border-t-0).
func SettingRow(p RowProps) tela.Node {
	return Div(Class("flex flex-1 items-center justify-between gap-6 border-t border-zinc-100 px-6 py-4 first:border-t-0 dark:border-zinc-800"),
		Div(Class("flex min-w-0 flex-col gap-0.5"),
			Span(Class("text-sm leading-5 font-medium"), Text(p.Title)),
			Span(Class("text-sm leading-5 "+muted), Text(p.Hint)),
		),
		tela.CP(Switch, SwitchProps{Field: p.Field, Label: p.Title, On: p.On, Locked: p.Locked, Set: p.Set}),
	)
}

// AppearanceProps are the props of AppearanceCard.
type AppearanceProps struct {
	Prefs Prefs
	Set   tela.Setter[Prefs]
}

// AppearanceCard has two radio groups, named by their labels: a theme
// with previews and a density list.
func AppearanceCard(p AppearanceProps) tela.Node {
	themeLabel, densityLabel := tela.UseElementRef(), tela.UseElementRef()
	return Div(Class(card+" flex min-w-[340px] flex-1 basis-[420px] flex-col pb-6"),
		cardHeader("Appearance", "Pick a theme and how much room the interface takes."),
		Div(Class("mt-5 flex flex-col gap-2.5 px-6"),
			Span(Ref(themeLabel), Class("text-sm font-medium"), Text("Theme")),
			Div(Class("flex gap-3"), Role("radiogroup"), AriaLabelledBy(themeLabel),
				Map(themes, func(o Option) tela.Node {
					return tela.WithKey(tela.CP(ThemeOption, RadioProps{
						Field: FieldTheme, Value: o.Value, Selected: p.Prefs.Theme == o.Value, Set: p.Set,
					}), o.Value)
				}),
			),
		),
		Div(Class("mt-6 flex flex-col gap-2.5 px-6"),
			Span(Ref(densityLabel), Class("text-sm font-medium"), Text("Density")),
			Div(Class("flex flex-col gap-2"), Role("radiogroup"), AriaLabelledBy(densityLabel),
				Map(densities, func(o Option) tela.Node {
					return tela.WithKey(tela.CP(DensityOption, RadioProps{
						Field: FieldDensity, Value: o.Value, Selected: p.Prefs.Density == o.Value, Set: p.Set,
					}), o.Value)
				}),
			),
		),
	)
}

// FooterProps are the props of Footer. Save and Reset are Callback
// handles: comparable, so the footer rebuilds only when Dirty changes.
type FooterProps struct {
	Dirty       bool
	Save, Reset tela.Callback[func(tela.MouseEvent)]
}

// Footer shows the save state and the Reset and Save changes buttons,
// which are disabled while there is nothing to save.
func Footer(p FooterProps) tela.Node {
	status := Div(Class("flex items-center gap-2 text-sm "+muted),
		Div(Class("size-2 rounded-full bg-emerald-500")),
		Text("All changes saved"))
	if p.Dirty {
		status = Div(Class("flex items-center gap-2 text-sm font-medium text-amber-600 dark:text-amber-400"),
			Div(Class("size-2 rounded-full bg-amber-500")),
			Text("You have unsaved changes"))
	}
	return Div(Class(card+" flex flex-wrap items-center justify-between gap-4 px-6 py-4"),
		status,
		Div(Class("flex gap-3"),
			Button(Class("flex h-9 items-center rounded-md border border-zinc-200 bg-white px-4 text-sm font-medium shadow-xs "+
				"hover:bg-zinc-100 active:bg-zinc-200 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-white "+
				"dark:border-zinc-700 dark:bg-zinc-800 dark:hover:bg-zinc-700 dark:disabled:hover:bg-zinc-800"),
				Disabled(!p.Dirty), OnClick(p.Reset.Fn()), Text("Reset")),
			Button(Class("flex h-9 items-center rounded-md bg-zinc-900 px-4 text-sm font-medium text-zinc-50 shadow-sm "+
				"hover:bg-zinc-700 active:bg-zinc-950 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-zinc-900 "+
				"dark:bg-zinc-50 dark:text-zinc-900 dark:hover:bg-zinc-200 dark:disabled:hover:bg-zinc-50"),
				Disabled(!p.Dirty), OnClick(p.Save.Fn()), Text("Save changes")),
		),
	)
}
