package aria_test

// The tests of the screen_*_test.go files are integration tests of the hooks
// together: one small "command center" screen built only from aria hooks and
// h elements (no kit), the way an application assembles them. A combobox, a
// tooltip with an arrow, two menus (one with sections, a disabled item and a
// destructive one; one of checkbox items that drives the list under it), a
// modal dialog with a form (a text field, a select, a switch, UseButton
// buttons, one of them of type "submit"), tabs with panels and a list box
// with a multiple selection.
//
// What they check is what the single-hook tests cannot: the focus order of a
// whole screen, overlays that open over overlays (a select inside a modal
// dialog), state that leaves a widget and comes back (a chosen project, a
// column, a deployment), and the frames that all of it paints and asks for.
// The look is only enough to lay the screen out and to make the overlays
// animate; the fixture is ported from the examples/aria screen, whose tests
// these replace.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// screenFocusRing is the ring of focus-visible:, which shows after the
// keyboard moves the focus and not after a click.
const screenFocusRing = "outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"

const (
	screenButton  = "inline-flex h-9 shrink-0 cursor-default items-center justify-center gap-2 whitespace-nowrap rounded-md border border-input bg-background px-4 text-sm font-medium transition-all hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
	screenPrimary = "inline-flex h-9 shrink-0 cursor-default items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:pointer-events-none disabled:opacity-50"
	screenField   = "h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
	// screenSurface is a floating surface; screenPop its entrance and
	// screenExitPop the Exit classes that undo it while Presence keeps it
	// mounted.
	screenSurface = "z-50 overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md"
	screenPop     = "transition duration-150 ease-out starting:scale-95 starting:opacity-0"
	screenExitPop = "scale-95 opacity-0 duration-100"
)

// screenProjects are the thirty projects the combobox filters.
var screenProjects = []string{
	"acme-api", "auth-gateway", "billing-service", "edge-router", "media-transcoder", "payments-worker",
	"analytics-warehouse", "events-stream", "ledger-db", "orders-db", "sessions-cache", "users-db",
	"admin-console", "careers", "customer-portal", "docs-portal", "marketing-site", "status-page",
	"cli-tools", "design-system", "sdk-go", "sdk-js", "telemetry-kit", "ui-icons",
	"api-reference", "changelog", "onboarding-guide", "runbooks", "security-policy", "style-guide",
}

// screenEnvs are the targets of a deployment.
var screenEnvs = []string{"production", "staging", "preview", "development"}

func screenEnvLabel(key string) string { return strings.ToUpper(key[:1]) + key[1:] }

// screenDeployment is a row of the recent deployments.
type screenDeployment struct{ Project, Version, Env, Branch, Author, Duration string }

var screenDeployments = []screenDeployment{
	{"acme-api", "v2.14.0", "staging", "main", "Ana García", "1m 48s"},
	{"customer-portal", "v5.2.3", "production", "release/5.2", "Mei Tanaka", "3m 12s"},
	{"payments-worker", "v1.9.1", "production", "fix/retry-backoff", "Omar Haddad", "2m 05s"},
	{"design-system", "v8.0.0", "staging", "main", "Sofia Rossi", "4m 31s"},
	{"auth-gateway", "v3.7.4", "production", "main", "Kwame Mensah", "2m 49s"},
}

type screenMember struct{ Name, Role string }

var screenMembers = []screenMember{
	{"Ana García", "Platform lead"}, {"Mei Tanaka", "Site reliability"}, {"Omar Haddad", "Backend engineer"},
	{"Lena Fischer", "Frontend engineer"}, {"Sofia Rossi", "Product designer"}, {"Kwame Mensah", "Security engineer"},
}

var screenMemberNames = func() []string {
	names := make([]string, len(screenMembers))
	for i, m := range screenMembers {
		names[i] = m.Name
	}
	return names
}()

var (
	screenTabKeys   = []string{"overview", "activity", "alerts"}
	screenTabLabels = map[string]string{"overview": "Overview", "activity": "Activity", "alerts": "Alerts"}
)

// screenMenuItem is one item of a menu; screenMenuGroup a section of it.
type screenMenuItem struct {
	Key, Label  string
	Disabled    bool
	Destructive bool
}

type screenMenuGroup struct {
	Name  string
	Items []screenMenuItem
}

var screenActions = []screenMenuGroup{
	{Name: "Project", Items: []screenMenuItem{
		{Key: "rename", Label: "Rename project"}, {Key: "duplicate", Label: "Duplicate"},
		{Key: "redeploy", Label: "Redeploy"}, {Key: "share", Label: "Share link"},
	}},
	{Name: "Manage", Items: []screenMenuItem{
		{Key: "export", Label: "Export report", Disabled: true}, {Key: "archive", Label: "Archive"},
		{Key: "delete", Label: "Delete project", Destructive: true},
	}},
}

var screenColumns = []screenMenuGroup{
	{Name: "Columns", Items: []screenMenuItem{
		{Key: "branch", Label: "Branch"}, {Key: "duration", Label: "Duration"}, {Key: "author", Label: "Author"},
	}},
}

// screenPage is the whole screen, with its state. Its fields seed that
// state, so a test can start it with an overlay open; the zero value is the
// screen as it opens.
type screenPage struct {
	// Tab is the tab of the workspace ("overview", "activity", "alerts").
	Tab string
	// Menu opens a menu at the start: "actions" or "view".
	Menu string
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

func (p screenPage) Build() tela.Node {
	project, setProject := tela.UseState("acme-api")
	rows, setRows := tela.UseState(screenDeployments)
	cols, setCols := tela.UseState(aria.SelectionOf("branch", "duration"))
	notice, setNotice := tela.UseState("")

	act := func(key string) {
		for _, g := range screenActions {
			for _, it := range g.Items {
				if it.Key == key {
					setNotice.Set(fmt.Sprintf("%s: %s", it.Label, project))
				}
			}
		}
	}
	deployed := func(d screenDeployment) {
		setRows.Update(func(rows []screenDeployment) []screenDeployment {
			return append([]screenDeployment{d}, rows...)
		})
		setNotice.Set(fmt.Sprintf("Queued %s %s for %s", d.Project, d.Version, screenEnvLabel(d.Env)))
	}

	return Div(Class("flex min-h-full flex-col bg-background text-foreground"),
		Div(Role("banner"), Class("flex h-14 shrink-0 items-center gap-3 border-b border-border px-4"),
			screenSearch{Project: project, OnChoose: setProject.Set, Text: p.Search, Open: p.SearchOpen},
			Div(Class("flex-1")),
			screenBell{Unread: 3, Open: p.Tooltip},
			screenMenuButton{
				Label: "View", Groups: screenColumns, Mode: "multiple", Placement: "bottom-end",
				Checked: cols, OnChecked: setCols.Set, Open: p.Menu == "view",
			},
			screenMenuButton{
				Label: "Actions", Groups: screenActions, Placement: "bottom-end", OnAction: act,
				Open: p.Menu == "actions",
			},
			screenNewDeployment{
				Project: project, OnDeploy: deployed, Open: p.Dialog, Version: p.Version, EnvOpen: p.EnvOpen,
			},
		),
		Div(Role("main"), Class("mx-auto flex w-full max-w-6xl flex-1 flex-col gap-5 px-6 py-6"),
			Div(Class("flex items-end justify-between gap-4"),
				Div(Class("flex flex-col gap-1.5"),
					H1(Class("text-2xl font-semibold"), Text("Deployments")),
					Div(Class("text-sm text-muted-foreground"), Text("Releases of "+project))),
				If(notice != "", Div(Role("status"), Class("rounded-md border border-border px-3 py-1.5 text-sm"), Text(notice)))),
			Div(Class("flex items-start gap-5"),
				screenWorkspace{Rows: rows, Columns: cols, Project: project, Tab: p.Tab},
				screenNotify{Initial: []string{"Ana García", "Mei Tanaka"}}),
		),
	)
}

// screenSearch is the combobox that picks the project: UseCombobox holds the
// text, filters the thirty projects with aria.Contains (case and accents
// ignored) and keeps the focus on the input while the arrow keys highlight
// an item. The popover is placed by UseFloating and exits with Presence.
type screenSearch struct {
	Project  string
	OnChoose func(project string)
	Text     string
	Open     bool
}

func (s screenSearch) Build() tela.Node {
	anchor := tela.UseElementRef()
	cb := aria.UseCombobox(aria.ComboboxOptions[string]{
		Items:             screenProjects,
		DefaultSelected:   aria.SelectionOf(s.Project),
		DefaultInputValue: s.Text,
		DefaultOpen:       s.Open,
		OnSelectionChange: func(sel aria.Selection[string]) {
			if k, ok := sel.First(); ok {
				s.OnChoose(k)
			}
		},
		AllowsEmptyCollection: true,
		AnchorRef:             anchor,
		MatchWidth:            true,
		Placement:             "bottom-start",
		Offset:                6,
		AriaLabel:             "Project",
		ButtonLabel:           "Show all projects",
	})
	return Fragment(
		Div(Ref(anchor), Class("relative w-72"),
			Input(cb.InputProps, Placeholder("Search projects"), Class(screenField, screenFocusRing, "pr-9")),
			Button(cb.ButtonProps, Class("absolute top-1/2 right-1 size-7 -translate-y-1/2 rounded-sm text-sm"))),
		Portal(Presence(If(cb.Open,
			Div(cb.PopoverProps, Exit(screenExitPop), Class(screenSurface, screenPop, "origin-top"),
				Div(cb.ListBox.Props, Class("max-h-72 overflow-y-auto outline-none"),
					Range(len(cb.Keys), func(i int) tela.Node {
						name := cb.Keys[i]
						return Div(cb.ListBox.ItemProps(name), Key(name),
							Class("flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none aria-disabled:opacity-50"),
							If(cb.ListBox.IsFocused(name), Class("bg-accent text-accent-foreground")),
							Text(name))
					})),
				If(len(cb.Keys) == 0,
					Div(Role("status"), Class("px-2 py-6 text-center text-sm"), Text("No project matches your search.")))),
		))),
	)
}

// screenBell is an icon button with a tooltip: UseTooltipTrigger opens it
// when the pointer rests on the button or the keyboard focuses it, closes it
// with Escape and describes the button with it. UseFloating places it with an
// arrow that points at the button.
type screenBell struct {
	Unread int
	Open   bool
}

func (b screenBell) Build() tela.Node {
	tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{DefaultOpen: b.Open})
	arrow := tela.UseElementRef()
	fl := aria.UseFloating(tip.Ref, aria.Placement("bottom"), aria.Offset(8), aria.Flip(), aria.Shift(8), aria.Arrow(arrow))
	return Fragment(
		Button(tip.TriggerProps, AriaLabel("Notifications"), Class(screenButton, screenFocusRing, "relative size-9 px-0")),
		Portal(Presence(If(tip.Open,
			Div(tip.TooltipProps, fl.Props, Exit("scale-95 opacity-0 duration-100"),
				Class("z-50 rounded-md bg-foreground px-3 py-1.5 text-xs text-background shadow-md",
					"transition duration-150 starting:scale-95 starting:opacity-0"),
				Textf("%d unread notifications", b.Unread),
				Div(Ref(arrow), fl.ArrowProps, Class("size-2.5 rotate-45 rounded-[2px] bg-foreground")),
			)))),
	)
}

// screenMenuButton is a button that opens a menu: UseMenuTrigger holds the
// open state and the keys that open it, UsePopover places and dismisses the
// menu, and UseMenu (in screenMenuList, which exists only while the menu is
// open) holds the focused item and the selection. Mode "multiple" makes
// checkbox items whose state is the caller's (Checked and OnChecked).
type screenMenuButton struct {
	Label     string
	Groups    []screenMenuGroup
	Mode      string
	Placement string
	OnAction  func(key string)
	Checked   aria.Selection[string]
	OnChecked func(aria.Selection[string])
	Open      bool
}

func (m screenMenuButton) Build() tela.Node {
	mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{DefaultOpen: m.Open})
	popover := aria.UsePopover(aria.PopoverOptions{
		Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close,
		Placement: m.Placement, Offset: 6, Modal: true,
	})
	return Fragment(
		Button(mt.TriggerProps, Class(screenButton, screenFocusRing, "aria-expanded:bg-accent"), Text(m.Label)),
		Portal(Presence(If(mt.Open, screenMenuList{Button: m, Trigger: mt, Popover: popover}))),
	)
}

type screenMenuList struct {
	Button  screenMenuButton
	Trigger aria.MenuTriggerResult
	Popover aria.PopoverResult
}

func (m screenMenuList) Build() tela.Node {
	b := m.Button
	var keys, disabled []string
	byKey := map[string]screenMenuItem{}
	for _, g := range b.Groups {
		for _, it := range g.Items {
			keys = append(keys, it.Key)
			byKey[it.Key] = it
			if it.Disabled {
				disabled = append(disabled, it.Key)
			}
		}
	}
	menu := aria.UseMenu(aria.MenuOptions[string]{
		Keys: keys, DisabledKeys: disabled,
		TextValue:     func(k string) string { return byKey[k].Label },
		SelectionMode: b.Mode, Selected: b.Checked, OnSelectionChange: b.OnChecked,
		OnAction: b.OnAction, OnClose: m.Trigger.Close,
		AutoFocus: true, FocusStrategy: m.Trigger.FocusStrategy, Ref: m.Trigger.MenuRef,
	})
	var groups []tela.Child
	for i, g := range b.Groups {
		sec := menu.Section(g.Name)
		if i > 0 {
			groups = append(groups, Div(Role("separator"), Class("-mx-1 my-1 h-px bg-border")))
		}
		var items []tela.Child
		for _, it := range g.Items {
			items = append(items, Div(menu.ItemProps(it.Key), Key(it.Key),
				Class("relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none",
					"focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50"),
				If(it.Destructive, Class("text-destructive focus:bg-destructive/10 focus:text-destructive")),
				Span(Class("flex-1"), Text(it.Label))))
		}
		groups = append(groups, Div(sec.Props,
			Div(sec.HeadingProps, Class("px-2 py-1.5 text-xs font-medium text-muted-foreground"), Text(g.Name)),
			Fragment(items...)))
	}
	return Div(m.Popover.Props, m.Trigger.MenuProps, menu.Props, Exit(screenExitPop),
		Class(screenSurface, screenPop, "origin-top-left min-w-52 outline-none"),
		Fragment(groups...))
}

// screenNewDeployment is the button that opens the "New deployment" dialog,
// and the dialog. UseOverlayTrigger holds the open state, UseOverlay puts the
// dialog on the overlay stack (Escape and a press on the scrim close it, and
// Modal makes the rest of the window inert) and UseDialog names it, moves the
// focus in, keeps Tab inside and gives the focus back to the button.
type screenNewDeployment struct {
	Project  string
	OnDeploy func(screenDeployment)
	Open     bool
	Version  string
	EnvOpen  bool
}

func (n screenNewDeployment) Build() tela.Node {
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{DefaultOpen: n.Open})
	overlay := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Dismissable: true, Modal: true})
	dialog := aria.UseDialog(aria.DialogOptions{Modal: true, Ref: trig.OverlayRef})
	closeBtn := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { trig.Close() }})

	return Fragment(
		Button(trig.TriggerProps, Class(screenPrimary, screenFocusRing), Text("New deployment")),
		Portal(Presence(If(trig.Open,
			Div(Class("fixed inset-0 z-50 flex items-center justify-center p-6"),
				Div(Class("absolute inset-0 bg-black/50 transition-opacity duration-200 starting:opacity-0"), Exit("opacity-0")),
				Div(overlay.Props, dialog.Props, Exit("scale-95 opacity-0 duration-100"),
					Class("relative flex w-full max-w-md flex-col gap-5 rounded-xl border border-border bg-background p-6 text-foreground shadow-lg",
						"transition duration-200 ease-out starting:scale-95 starting:opacity-0"),
					Div(Class("flex flex-col gap-1.5"),
						H2(dialog.TitleProps, Class("text-lg font-semibold"), Text("New deployment")),
						P(Class("text-sm text-muted-foreground"), Textf("Ship a version of %s to an environment.", n.Project))),
					screenDeployForm{Close: trig.Close, OnDeploy: n.OnDeploy, Project: n.Project, Version: n.Version, EnvOpen: n.EnvOpen},
					// The close button is last in the tree, so the first field
					// takes the focus when the dialog opens.
					Button(closeBtn.Props, AriaLabel("Close"),
						Class("absolute top-4 right-4 size-7 rounded-sm text-sm", screenFocusRing)),
				),
			),
		))),
	)
}

// screenDeployForm is the form of the dialog: a text field, a select, a
// switch and two buttons, all behavior from aria hooks. It exists only while
// the dialog is open, so it starts empty every time.
type screenDeployForm struct {
	Close    func()
	OnDeploy func(screenDeployment)
	Project  string
	Version  string
	EnvOpen  bool
}

func (f screenDeployForm) Build() tela.Node {
	version, setVersion := tela.UseState(f.Version)
	versionID := tela.UseID()
	env := aria.UseSelect(aria.SelectOptions[string]{
		Keys: screenEnvs, TextValue: screenEnvLabel,
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
			f.OnDeploy(screenDeployment{
				Project: f.Project, Version: strings.TrimSpace(version), Env: target,
				Branch: "main", Author: "You", Duration: "-",
			})
			f.Close()
		}),
		Div(Class("flex flex-col gap-2"),
			Label(For(versionID), Class("text-sm font-medium"), Text("Version")),
			Input(ID(versionID), Bind(version, setVersion), Placeholder("v2.14.1"), AutoComplete("off"),
				Class(screenField, screenFocusRing))),
		Div(Class("flex flex-col gap-2"),
			Span(env.LabelProps, Class("text-sm font-medium"), Text("Environment")),
			Button(env.TriggerProps, Class(screenField, screenFocusRing, "flex items-center justify-between text-left aria-expanded:border-ring"),
				Span(env.ValueProps, Text(env.Value)))),
		Div(Class("flex items-center justify-between gap-4 rounded-lg border border-border p-3"),
			Div(Class("flex flex-col gap-1"),
				Label(For(notify.ID), Class("text-sm font-medium"), Text("Notify the team")),
				P(Class("text-xs text-muted-foreground"), Text("Post the release notes when it finishes."))),
			Button(notify.Props, Class("inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-transparent bg-input p-0.5 checked:bg-primary", screenFocusRing),
				Span(Class("block size-4 rounded-full bg-background transition-transform"),
					If(notify.Selected, Class("translate-x-4"))))),
		Div(Class("flex justify-end gap-2"),
			Button(cancel.Props, Class(screenButton, screenFocusRing), Text("Cancel")),
			Button(submit.Props, Class(screenPrimary, screenFocusRing), Text("Deploy"))),
		Portal(Presence(If(env.Open,
			Div(env.PopoverProps, Exit(screenExitPop), Class(screenSurface, screenPop, "origin-top"),
				Div(env.ListBox.Props, Class("outline-none"),
					Range(len(screenEnvs), func(i int) tela.Node {
						k := screenEnvs[i]
						return Div(env.ListBox.ItemProps(k), Key(k),
							Class("flex cursor-default select-none items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm outline-none",
								"focus:bg-accent focus:text-accent-foreground"),
							Text(screenEnvLabel(k)))
					})),
			)))),
	)
}

// screenWorkspace is the card with the tabs. UseTabs holds the selected tab;
// the card renders the tab list and the panel of the selected tab, and the
// rows of the overview follow the columns of the View menu.
type screenWorkspace struct {
	Rows    []screenDeployment
	Columns aria.Selection[string]
	Project string
	Tab     string
}

func (w screenWorkspace) Build() tela.Node {
	first := w.Tab
	if first == "" {
		first = "overview"
	}
	tabs := aria.UseTabs(aria.TabsOptions[string]{
		Keys: screenTabKeys, DefaultSelected: aria.SelectionOf(first), AriaLabel: "Sections",
	})
	current, _ := tabs.Selected.First()

	var panel tela.Node
	switch current {
	case "activity":
		panel = Div(Role("list"), AriaLabel("Activity"),
			Div(Role("listitem"), Text("Mei Tanaka deployed customer-portal v5.2.3 to production")),
			Div(Role("listitem"), Text("Omar Haddad rolled back payments-worker to v1.9.0 on production")))
	case "alerts":
		panel = Div(Class("flex flex-col gap-3"),
			Span(Text("payments-worker failed its health check")),
			Span(Text("Build cache is 91% full")))
	default:
		panel = Div(Class("flex flex-col"),
			H3(Class("pb-2 text-sm font-medium"), Text("Recent deployments")),
			Div(Role("list"), AriaLabel("Recent deployments"), Class("overflow-hidden rounded-lg border border-border"),
				Range(len(w.Rows), func(i int) tela.Node {
					d := w.Rows[i]
					return Div(Role("listitem"), Key(d.Project+" "+d.Version),
						Class("flex items-center gap-3 border-t border-border px-4 py-3 text-sm first:border-t-0"),
						Span(Class("w-40 truncate font-medium"), Text(d.Project)),
						Span(Class("w-20 text-xs"), Text(d.Version)),
						If(w.Columns.Has("branch"), Span(Class("w-40 truncate text-xs"), Text(d.Branch))),
						If(w.Columns.Has("duration"), Span(Class("w-16 text-right text-xs"), Text(d.Duration))),
						If(w.Columns.Has("author"), Span(Class("w-28 truncate text-xs"), Text(d.Author))))
				})))
	}
	return Div(Class("flex min-w-0 flex-1 flex-col rounded-xl border border-border bg-card text-card-foreground"),
		Div(Class("flex items-center border-b border-border px-4 py-3"),
			Div(tabs.Props, Class("inline-flex h-9 items-center rounded-lg bg-muted p-[3px] text-muted-foreground"),
				Range(len(screenTabKeys), func(i int) tela.Node {
					k := screenTabKeys[i]
					return Div(tabs.TabProps(k), Key(k),
						Class("inline-flex h-7 cursor-default items-center justify-center rounded-md px-3 text-sm font-medium whitespace-nowrap",
							"aria-selected:bg-background aria-selected:text-foreground", screenFocusRing),
						Text(screenTabLabels[k]))
				}))),
		Div(tabs.PanelProps(current), Class("flex flex-col gap-4 rounded-b-xl p-4 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"), panel),
	)
}

// screenNotify is the card with the list box of people to notify: a
// multiple selection that the card controls to show its count.
type screenNotify struct{ Initial []string }

func (n screenNotify) Build() tela.Node {
	sel, setSel := tela.UseState(aria.SelectionOf(n.Initial...))
	lb := aria.UseListBox(aria.ListBoxOptions[string]{
		Keys: screenMemberNames, TextValue: func(name string) string { return name },
		SelectionMode: "multiple", AriaLabel: "People to notify",
		Selected: sel, OnSelectionChange: setSel.Set,
	})
	return Div(Class("flex w-80 shrink-0 flex-col rounded-xl border border-border bg-card"),
		Div(Class("flex flex-col gap-1 px-4 pt-4 pb-3"),
			H3(Class("text-sm font-semibold"), Text("Release notes"))),
		Div(lb.Props, Class("flex flex-col gap-1 px-2 pb-2 outline-none"),
			Range(len(screenMembers), func(i int) tela.Node {
				m := screenMembers[i]
				return Div(lb.ItemProps(m.Name), Key(m.Name),
					Class("flex cursor-default items-center gap-3 rounded-lg border border-transparent px-2 py-2 text-sm",
						"aria-selected:border-border aria-selected:bg-accent", screenFocusRing),
					Div(Class("flex min-w-0 flex-1 flex-col"),
						Span(Class("truncate font-medium"), Text(m.Name)),
						Span(Class("truncate text-xs text-muted-foreground"), Text(m.Role))))
			})),
		Div(Class("border-t border-border px-4 py-3 text-xs text-muted-foreground"),
			Textf("%d of %d selected", len(sel.In(screenMemberNames)), len(screenMembers))),
	)
}

// screenMount opens a root in a 1120 x 760 window. Dev mode is on, so any
// dev message (an unknown class, a hook misuse) fails the test.
func screenMount(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.Mount(t, root, append([]telatest.Option{telatest.Size(1120, 760)}, opts...)...)
}

// The elements the tests use, by role and accessible name, as a user of a
// screen reader finds them.
var (
	screenProjectBox    = telatest.ByRole("combobox", "Project")
	screenProjectToggle = telatest.ByRole("button", "Show all projects")
	screenBellButton    = telatest.ByRole("button", "Notifications")
	screenViewButton    = telatest.ByRole("button", "View")
	screenActionsButton = telatest.ByRole("button", "Actions")
	screenNewButton     = telatest.ByRole("button", "New deployment")
	screenOverviewTab   = telatest.ByRole("tab", "Overview")
	screenVersionField  = telatest.ByRole("textbox", "Version")
	screenDeployButton  = telatest.ByRole("button", "Deploy")
	screenCancelButton  = telatest.ByRole("button", "Cancel")
	screenCloseButton   = telatest.ByRole("button", "Close")
	screenNotifySwitch  = telatest.ByRole("switch", "Notify the team")
	screenDialog        = telatest.ByRole("dialog", "New deployment")
)

// screenBlankX and screenBlankY are a point of the window that no control
// covers: below the cards.
const screenBlankX, screenBlankY = 560, 735

// screenActive expects the combobox's virtual focus on the option named
// name: the input keeps the focus and points to the highlighted item with
// aria-activedescendant, which a screen reader announces.
func screenActive(name string) telatest.State {
	return telatest.ActiveDescendant(telatest.ByRole("option", name))
}

// screenPressTab presses Tab n times: the screen is reached with the
// keyboard only.
func screenPressTab(a *telatest.App, n int) {
	for i := 0; i < n; i++ {
		a.Press("Tab")
	}
}

// screenSection returns the part of an aria snapshot that starts at its first
// line whose text, indentation aside, begins with start (such as
// `- listbox "Project":`) and holds the lines indented deeper than it,
// without that indentation: the snapshot of one widget out of the whole
// window's.
func screenSection(snapshot, start string) string {
	lines := strings.Split(snapshot, "\n")
	for i, l := range lines {
		text := strings.TrimLeft(l, " ")
		if !strings.HasPrefix(text, start) {
			continue
		}
		indent := len(l) - len(text)
		end := i + 1
		for end < len(lines) && len(lines[end])-len(strings.TrimLeft(lines[end], " ")) > indent {
			end++
		}
		out := lines[i:end]
		for j, o := range out {
			out[j] = o[indent:]
		}
		return strings.Join(out, "\n")
	}
	return ""
}

// screenOptionsOf counts the options of the listbox with the given name in
// the window's aria snapshot.
func screenOptionsOf(a *telatest.App, listbox string) int {
	return strings.Count(screenSection(a.AriaSnapshot(), `- listbox "`+listbox+`"`), "- option")
}

// screenDedent trims the blank lines around a raw string and the indentation
// that all its other lines share, so that it can be written inside the code.
func screenDedent(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := -1
	for _, l := range lines {
		if n := len(l) - len(strings.TrimLeft(l, " \t")); indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(l[min(indent, len(l)):], " \t")
	}
	return strings.Join(lines, "\n")
}

// screenExpectSection checks one part of the window's aria snapshot against
// a raw string. The snapshot settles the app first.
func screenExpectSection(t *testing.T, a *telatest.App, start, want string) {
	t.Helper()
	if got := screenSection(a.AriaSnapshot(), start); got != screenDedent(want) {
		t.Errorf("the aria snapshot from %q is\n%s\nwant\n%s", start, got, screenDedent(want))
	}
}
