// Command aria is a "Command center" screen built only from the behavior
// hooks of package aria, the elements of package h and Tailwind classes with
// the theme tokens of shadcn/ui: no kit components and no hand-written key
// handling.
//
//   - A project combobox (UseCombobox) that filters thirty projects with
//     aria.Contains, with an icon per kind.
//   - A tooltip on an icon button (UseTooltipTrigger, UseFloating with an
//     arrow).
//   - Two menus (UseMenuTrigger, UsePopover, UseMenu): Actions has icons,
//     sections, a separator, a disabled item and a destructive one; View has
//     checkbox items that show and hide the columns of the list.
//   - A modal dialog (UseOverlayTrigger, UseOverlay, UseDialog) with a form:
//     an Input, a select (UseSelect), a switch (UseSwitch) and buttons
//     (UseButton, one of type "submit").
//   - Tabs (UseTabs) and a list box with a multiple selection (UseListBox).
//   - Entrance and exit animations with Presence, Exit and starting:, and
//     focus-visible: rings on everything the keyboard reaches.
//
// `tela snapshot ./examples/aria` renders it without a window; pick a state
// with --preview ("1 Overview", "2 Actions menu", "3 View menu", "4 Combobox",
// "5 Tooltip", "6 Dialog", "7 Dialog select") and add --dark for the dark
// scheme.
package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// Page is the whole screen, with its state. Its fields seed that state, so a
// preview can start it with an overlay open; the zero Page is the real app.
type Page struct {
	// Tab is the tab of the workspace ("overview", "activity", "alerts").
	Tab string
	// Menu opens a menu at the start: "actions" or "view". MenuFocus is the
	// item it focuses ("first" or "last").
	Menu      string
	MenuFocus string
	// Search is the text of the combobox and SearchOpen opens its list.
	Search     string
	SearchOpen bool
	// Tooltip opens the tooltip of the bell.
	Tooltip bool
	// Dialog opens the dialog, with Version typed in; EnvOpen opens the
	// select of its form.
	Dialog  bool
	Version string
	EnvOpen bool
}

// actions are the items of the Actions menu.
var actions = []MenuGroup{
	{Name: "Project", Items: []MenuItem{
		{Key: "rename", Label: "Rename project", Shortcut: "F2", Icon: icons.Pencil},
		{Key: "duplicate", Label: "Duplicate", Shortcut: "Ctrl+D", Icon: icons.Copy},
		{Key: "redeploy", Label: "Redeploy", Icon: icons.RefreshCw},
		{Key: "share", Label: "Share link", Icon: icons.Share2},
	}},
	{Name: "Manage", Items: []MenuItem{
		{Key: "export", Label: "Export report", Icon: icons.Download, Disabled: true},
		{Key: "archive", Label: "Archive", Icon: icons.Archive},
		{Key: "delete", Label: "Delete project", Shortcut: "Del", Icon: icons.Trash2, Destructive: true},
	}},
}

// columns are the items of the View menu.
var columns = []MenuGroup{
	{Name: "Columns", Items: []MenuItem{
		{Key: "branch", Label: "Branch"},
		{Key: "duration", Label: "Duration"},
		{Key: "author", Label: "Author"},
	}},
}

func (p Page) Build() tela.Node {
	project, setProject := tela.UseState("acme-api")
	rows, setRows := tela.UseState(Deployments)
	cols, setCols := tela.UseState(aria.SelectionOf("branch", "duration"))
	notice, setNotice := tela.UseState("")

	act := func(key string) {
		for _, g := range actions {
			for _, it := range g.Items {
				if it.Key == key {
					setNotice.Set(fmt.Sprintf("%s: %s", it.Label, project))
				}
			}
		}
	}
	deployed := func(d Deployment) {
		setRows.Update(func(rows []Deployment) []Deployment {
			return append([]Deployment{d}, rows...)
		})
		setNotice.Set(fmt.Sprintf("Queued %s %s for %s", d.Project, d.Version, EnvLabel(d.Env)))
	}

	return Div(Class("flex min-h-full flex-col bg-background text-foreground"),
		// The bar: the project combobox, then the buttons.
		Div(Role("banner"), Class("flex h-14 shrink-0 items-center gap-3 border-b border-border px-4"),
			Div(Class("flex items-center gap-2.5 pr-3"),
				Div(Class("flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"),
					ic(icons.LayoutDashboard, "size-4")),
				Span(Class("text-sm font-semibold tracking-tight"), Text("Command center"))),
			ProjectSearch{Project: project, OnChoose: setProject.Set, Text: p.Search, Open: p.SearchOpen},
			Div(Class("flex-1")),
			Bell{Unread: 3, Open: p.Tooltip},
			MenuButton{
				Label: "View", Icon: icons.Eye, Groups: columns, Mode: "multiple", Placement: "bottom-end",
				Checked: cols, OnChecked: setCols.Set,
				Open: p.Menu == "view", Focus: p.MenuFocus,
			},
			MenuButton{
				Label: "Actions", Groups: actions, Placement: "bottom-end", OnAction: act,
				Open: p.Menu == "actions", Focus: p.MenuFocus,
			},
			NewDeployment{
				Project: project, OnDeploy: deployed,
				Open: p.Dialog, Version: p.Version, EnvOpen: p.EnvOpen,
			},
		),
		Div(Role("main"), Class("mx-auto flex w-full max-w-6xl flex-1 flex-col gap-5 px-6 py-6"),
			Div(Class("flex items-end justify-between gap-4"),
				Div(Class("flex flex-col gap-1.5"),
					H1(Class("text-2xl leading-none font-semibold tracking-tight"), Text("Deployments")),
					Div(Class("flex items-center gap-1.5 text-sm text-muted-foreground"),
						Text("Releases of"),
						Span(Class("inline-flex items-center gap-1.5 font-medium text-foreground"),
							ic(projectKind(project).Icon, "size-4 text-muted-foreground"), Text(project)))),
				If(notice != "", Div(Role("status"),
					Class("flex items-center gap-2 rounded-md border border-border bg-card px-3 py-1.5 text-sm shadow-xs"),
					ic(icons.Check, "size-4 text-emerald-600 dark:text-emerald-500"), Text(notice))),
			),
			Div(Class("flex items-start gap-5"),
				Workspace{Rows: rows, Columns: cols, Project: project, Tab: p.Tab},
				Notify{Initial: []string{"Ana García", "Mei Tanaka"}},
			),
		),
	)
}

// scheme is the theme of the app: shadcn/ui's neutral colors with its 10 px
// radius, so rounded-md, rounded-lg and rounded-xl have shadcn/ui's sizes.
func theme() *tela.Theme {
	t := tela.DefaultTheme()
	t.Radius = 10
	return &t
}

// App is the window: main runs it, the tests mount it (telatest.MountApp)
// and `tela snapshot` renders it at the same size, with the same theme.
var App = tela.App{
	Title:     "Tela - command center",
	AppID:     "dev.telaui.aria",
	Width:     1120,
	Height:    760,
	Root:      Page{},
	RootClass: "bg-background text-foreground",
	Theme:     theme(),
}

// Previews are the states the README and the goldens show, each in App's
// window: the main screen and every overlay open.
var Previews = tela.Previews{
	"1 Overview":      {Root: Page{}},
	"2 Actions menu":  {Root: Page{Menu: "actions", MenuFocus: "first"}},
	"3 View menu":     {Root: Page{Menu: "view", MenuFocus: "first"}},
	"4 Combobox":      {Root: Page{Search: "se", SearchOpen: true}},
	"5 Tooltip":       {Root: Page{Tooltip: true}},
	"6 Dialog":        {Root: Page{Dialog: true, Version: "v2.14.1"}},
	"7 Dialog select": {Root: Page{Dialog: true, Version: "v2.14.1", EnvOpen: true}},
	"8 Activity tab":  {Root: Page{Tab: "activity"}},
}

func main() { tela.Main(App) }
