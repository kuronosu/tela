package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// NewDeployment is the button that opens the "New deployment" dialog, and
// the dialog.
//
//   - UseOverlayTrigger holds the open state and the trigger's props
//     (aria-haspopup="dialog", aria-expanded, aria-controls).
//   - UseOverlay puts the dialog on the overlay stack: Escape and a press on
//     the scrim close it, and Modal makes the rest of the window inert.
//   - UseDialog gives it the dialog role, its name (the heading with
//     TitleProps), moves the focus in when it opens, keeps Tab inside and
//     gives the focus back to the button when it closes.
//
// The scrim and the card are ordinary elements; Presence plays their exit.
type NewDeployment struct {
	// Project is the project the deployment is for.
	Project string
	// OnDeploy is told of the deployment the form made.
	OnDeploy func(Deployment)

	// The fields below start the dialog open and filled: a preview's seed.
	Open    bool
	Version string
	// EnvOpen opens the environment list too.
	EnvOpen bool
}

func (n NewDeployment) Build() tela.Node {
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{DefaultOpen: n.Open})
	overlay := aria.UseOverlay(aria.OverlayOptions{
		Open: trig.Open, OnClose: trig.Close, Dismissable: true, Modal: true,
	})
	dialog := aria.UseDialog(aria.DialogOptions{Modal: true, Ref: trig.OverlayRef})
	closeBtn := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { trig.Close() }})

	return Fragment(
		Button(trig.TriggerProps, Class(button, primary, focusRing),
			ic(icons.Rocket, "size-4"), Text("New deployment")),
		Portal(Presence(If(trig.Open,
			Div(Class("fixed inset-0 z-50 flex items-center justify-center p-6"),
				Div(Class("absolute inset-0 bg-black/50 transition-opacity duration-200 starting:opacity-0"), Exit("opacity-0")),
				Div(overlay.Props, dialog.Props, Exit("scale-95 opacity-0 duration-100"),
					Class("relative flex w-full max-w-md flex-col gap-5 rounded-xl border border-border bg-background p-6 text-foreground shadow-lg",
						"transition duration-200 ease-out starting:scale-95 starting:opacity-0"),
					Div(Class("flex flex-col gap-1.5"),
						H2(dialog.TitleProps, Class("text-lg leading-none font-semibold"), Text("New deployment")),
						P(Class("text-sm text-muted-foreground"),
							Textf("Ship a version of %s to an environment.", n.Project))),
					DeployForm{Close: trig.Close, OnDeploy: n.OnDeploy, Project: n.Project,
						Version: n.Version, EnvOpen: n.EnvOpen},
					// The close button is last in the tree, so the first field
					// takes the focus when the dialog opens, and it sits in the
					// corner.
					Button(closeBtn.Props, AriaLabel("Close"),
						Class("absolute top-4 right-4 flex size-7 items-center justify-center rounded-sm text-muted-foreground",
							"transition-colors hover:bg-accent hover:text-accent-foreground", focusRing),
						ic(icons.X, "size-4")),
				),
			),
		))),
	)
}

// DeployForm is the form of the dialog: a text field, a select, a switch and
// two buttons, all behavior from aria hooks. It exists only while the dialog
// is open, so it starts empty every time.
type DeployForm struct {
	Close    func()
	OnDeploy func(Deployment)
	Project  string
	Version  string
	EnvOpen  bool
}

func (f DeployForm) Build() tela.Node {
	version, setVersion := tela.UseState(f.Version)
	versionID := tela.UseID()

	env := aria.UseSelect(aria.SelectOptions[string]{
		Keys: Environments, TextValue: EnvLabel,
		DefaultSelected: aria.SelectionOf("staging"), DefaultOpen: f.EnvOpen,
		Placeholder: "Choose an environment", MatchWidth: true, Offset: 4,
	})
	notify := aria.UseSwitch(aria.ToggleOptions{DefaultSelected: true})

	valid := strings.TrimSpace(version) != ""
	cancel := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { f.Close() }})
	submit := aria.UseButton(aria.ButtonOptions{Type: "submit", Disabled: !valid})

	return Form(Class("flex flex-col gap-5"),
		OnSubmit(func() {
			if !valid {
				return
			}
			target, _ := env.Selected.First()
			f.OnDeploy(Deployment{
				Project: f.Project, Version: strings.TrimSpace(version), Env: target,
				Branch: "main", Author: "You", Status: Queued, Duration: "—", Age: "just now",
			})
			f.Close()
		}),

		Div(Class("flex flex-col gap-2"),
			Label(For(versionID), Class("text-sm leading-none font-medium"), Text("Version")),
			Input(ID(versionID), Bind(version, setVersion), Placeholder("v2.14.1"), AutoComplete("off"),
				Class(field, focusRing)),
		),

		Div(Class("flex flex-col gap-2"),
			Span(env.LabelProps, Class("text-sm leading-none font-medium"), Text("Environment")),
			Button(env.TriggerProps, Class(field, focusRing, "flex items-center justify-between text-left aria-expanded:border-ring"),
				Span(env.ValueProps, Text(env.Value)),
				ic(icons.ChevronsUpDown, "size-4 text-muted-foreground")),
		),

		Div(Class("flex items-center justify-between gap-4 rounded-lg border border-border p-3"),
			Div(Class("flex flex-col gap-1"),
				Label(For(notify.ID), Class("text-sm leading-none font-medium"), Text("Notify the team")),
				P(Class("text-xs text-muted-foreground"), Text("Post the release notes when it finishes."))),
			Button(notify.Props, Class("inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-transparent bg-input p-0.5 shadow-xs transition-colors checked:bg-primary", focusRing),
				Span(Class("block size-4 rounded-full bg-background shadow-sm transition-transform dark:bg-foreground"),
					If(notify.Selected, Class("translate-x-4 dark:bg-primary-foreground")))),
		),

		Div(Class("flex justify-end gap-2"),
			Button(cancel.Props, Class(button, outline, focusRing), Text("Cancel")),
			Button(submit.Props, Class(button, primary, focusRing), Text("Deploy")),
		),

		Portal(Presence(If(env.Open,
			Div(env.PopoverProps, Exit(exitPop), Class(surface, pop, "origin-top"),
				Div(env.ListBox.Props, Class("outline-none"),
					Range(len(Environments), func(i int) tela.Node {
						k := Environments[i]
						return Div(env.ListBox.ItemProps(k), Key(k),
							Class("flex cursor-default select-none items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm outline-none",
								"focus:bg-accent focus:text-accent-foreground"),
							Text(EnvLabel(k)),
							If(env.ListBox.IsSelected(k), ic(icons.Check, "size-4")))
					})),
			)))),
	)
}
