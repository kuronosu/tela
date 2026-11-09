package main

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/gallery/icons"
)

// Gallery is the app: it holds which look is drawn and the color scheme, and
// gives the window the look's style and theme. Switching look swaps the
// kit.StyleProvider's style and calls tela.SetTheme with the look's tokens;
// the tree below never asks which platform it is.
//
// Its fields select the look (a key of the catalog; "" is the look of this
// OS, or LOOKS_LOOK) and the color scheme the gallery starts with. They are comparable, so
// it is not built again when they do not change.
type Gallery struct {
	Look   string
	Scheme tela.ColorScheme // System follows the system's setting
}

func (g Gallery) Build() tela.Node {
	id := g.Look
	if id == "" {
		id = defaultID
	}
	look, setLook := tela.UseState(id)
	scheme, setScheme := tela.UseState(g.Scheme)
	c := byID(look)
	if c == nil {
		panic(fmt.Sprintf("gallery: unknown look %q; use one of %s", look, strings.Join(ids(), ", ")))
	}
	l := tela.UseMemo(func() looks.Look { return c.Look() }, look)

	// The tokens of the look: App.Theme has them already when the app starts
	// with this look (the first build), and this is what changes them when
	// the look does.
	tela.UseUpdateEffect(func() func() {
		tela.SetTheme(l.Theme)
		return nil
	}, look)

	return Div(Class("h-screen w-full bg-background text-foreground"), Scheme(scheme),
		kit.StyleProvider{Style: l.Style, Children: Window{Look: l, ID: look, SetLook: setLook, Scheme: scheme, SetScheme: setScheme}})
}

// Window is the gallery's window: the toolbar, the sidebar, the content, the
// details pane and the status bar of one look.
type Window struct {
	Look      looks.Look
	ID        string
	SetLook   tela.Setter[string]
	Scheme    tela.ColorScheme
	SetScheme tela.Setter[tela.ColorScheme]
}

// schemeKeys name the color schemes in the sidebar and the menu.
var schemeKeys = []struct {
	Scheme tela.ColorScheme
	Key    string
	Title  string
	Icon   tela.Icon
}{
	{tela.Light, "light", "Light", icons.Sun},
	{tela.Dark, "dark", "Dark", icons.Moon},
	{tela.System, "system", "System", icons.Monitor},
}

func schemeKey(s tela.ColorScheme) string {
	for _, k := range schemeKeys {
		if k.Scheme == s {
			return k.Key
		}
	}
	return "system"
}

func schemeOf(key string) tela.ColorScheme {
	for _, k := range schemeKeys {
		if k.Key == key {
			return k.Scheme
		}
	}
	return tela.System
}

func (w Window) Build() tela.Node {
	r := w.Look.Regions
	details, setDetails := tela.UseState(true)
	sidebar, setSidebar := tela.UseState(true)
	columns, setColumns := tela.UseState("columns")

	body := []tela.Child{Class("flex min-h-0 flex-1")}
	if sidebar || r.ToolbarLead == "" {
		body = append(body, w.sidebar())
	}
	body = append(body, Div(Class(r.Content), Role("main"), AriaLabel("Controls"),
		Div(Class("flex min-h-0 flex-1 flex-col overflow-y-auto"), Controls{Columns: columns, Popup: byID(w.ID).Popup})))
	if details {
		body = append(body, w.detailsPane())
	}
	return Div(Class("flex h-full w-full flex-col overflow-hidden select-none", r.Window),
		w.toolbar(sidebar, setSidebar, columns, setColumns, details, setDetails),
		Div(body...),
		w.statusBar(),
	)
}

// iconButton is a toolbar button that holds an icon and has a tooltip.
func iconButton(label string, ic tela.Icon, onClick func()) tela.Component {
	return kit.Tooltip{
		Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: label, Icon: ic, OnClick: onClick},
		Content: Text(label),
	}
}

// step moves to the look before or after the current one.
func (w Window) step(delta int) {
	i := 0
	for j, c := range catalog {
		if c.ID == w.ID {
			i = j
		}
	}
	w.SetLook.Set(catalog[(i+delta+len(catalog))%len(catalog)].ID)
}

// ---------------------------------------------------------------------------
// The toolbar.

func (w Window) toolbar(sidebar bool, setSidebar tela.Setter[bool], columns string, setColumns tela.Setter[string], details bool, setDetails tela.Setter[bool]) tela.Node {
	r := w.Look.Regions

	nav := Div(Class(r.ToolbarGroup),
		iconButton("Previous look", icons.ArrowLeft, func() { w.step(-1) }),
		iconButton("Next look", icons.ArrowRight, func() { w.step(1) }),
	)
	search := Div(Class(r.Search),
		Icon(icons.Search, Class(r.SearchIcon)),
		kit.Input{Type: "search", Placeholder: "Search controls", AriaLabel: "Search controls", Class: r.SearchField},
	)

	row := []tela.Child{Class(r.Toolbar)}
	if r.ToolbarLead != "" {
		row = append(row, Div(Class(r.ToolbarLead),
			iconButton("Toggle sidebar", icons.PanelLeft, func() { setSidebar.Set(!sidebar) }),
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: "Main menu", Icon: icons.Menu},
				Align:   "start", AriaLabel: "Main menu",
				Items:    w.menu(details, setDetails),
				OnSelect: func(string) {},
			},
		))
	}
	row = append(row, nav, w.location(), search)

	view := columns
	end := []tela.Child{Class(r.ToolbarEnd),
		kit.ToggleGroup[string]{
			Items: []kit.ToggleGroupItem[string]{
				{Value: "columns", Icon: icons.LayoutGrid, AriaLabel: "Two columns"},
				{Value: "stack", Icon: icons.List, AriaLabel: "One column"},
			},
			Variant: looks.Toolbar, Size: looks.ToolbarSize, Spacing: 1, Class: r.ToolbarSegments,
			Value: &view, OnValueChange: setColumns.Set, DisallowEmptySelection: true, AriaLabel: "Layout",
		},
		kit.DropdownMenu[string]{
			Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: "More options", Icon: icons.Ellipsis},
			Align:   "end", AriaLabel: "More options",
			Items:    w.menu(details, setDetails),
			OnSelect: func(string) {},
		},
	}
	row = append(row, Div(end...))

	out := []tela.Child{Class("flex shrink-0 flex-col"), Div(row...)}
	if r.CommandBar != "" {
		out = append(out, w.commandBar())
	}
	return Div(out...)
}

// menu is the menu of the toolbar: the look and the color scheme as radio
// groups, and the details pane as a checkbox.
func (w Window) menu(details bool, setDetails tela.Setter[bool]) []kit.MenuItem[string] {
	type item = kit.MenuItem[string]
	look := "look:" + w.ID
	lookItems := make([]item, len(catalog))
	for i, c := range catalog {
		lookItems[i] = item{Key: "look:" + c.ID, Label: c.Title}
	}
	scheme := "scheme:" + schemeKey(w.Scheme)
	schemeItems := make([]item, len(schemeKeys))
	for i, k := range schemeKeys {
		schemeItems[i] = item{Key: "scheme:" + k.Key, Label: k.Title}
	}
	return []item{
		{Kind: kit.MenuLabel, Label: "Look"},
		{Kind: kit.MenuRadioGroup, Key: "look", Value: &look, Items: lookItems,
			OnValueChange: func(k string) { w.SetLook.Set(strings.TrimPrefix(k, "look:")) }},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuLabel, Label: "Color scheme"},
		{Kind: kit.MenuRadioGroup, Key: "scheme", Value: &scheme, Items: schemeItems,
			OnValueChange: func(k string) { w.SetScheme.Set(schemeOf(strings.TrimPrefix(k, "scheme:"))) }},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuCheckbox, Key: "details-pane", Label: "Details pane", Checked: &details, OnCheckedChange: setDetails.Set},
	}
}

// location is the breadcrumb of the toolbar: small buttons in the look's
// location bar.
func (w Window) location() tela.Node {
	r := w.Look.Regions
	crumb := func(label string, home, last bool) tela.Component {
		b := kit.Button{Variant: kit.Ghost, Size: kit.Sm, AriaLabel: "Go to " + label, Props: Key(label), Children: Text(label)}
		if home {
			b.Icon = icons.House
		}
		if last {
			b.Class = r.LocationCurrent
		}
		return b
	}
	sep := func(i int) tela.Child {
		return Div(Class(r.LocationSeparator), Key(fmt.Sprint("sep", i)), Icon(icons.ChevronRight, Class("size-3")))
	}
	return Div(Class(r.Location), Role("navigation"), AriaLabel("Location"),
		crumb("Looks", true, false), sep(0), crumb(byID(w.ID).Title, false, false), sep(1), crumb("Controls", false, true))
}

// commandBar is Windows' command bar: a second row of commands.
func (w Window) commandBar() tela.Node {
	r := w.Look.Regions
	cmd := func(label string, ic tela.Icon) tela.Component {
		return kit.Tooltip{Trigger: kit.Button{Variant: kit.Ghost, AriaLabel: label, Icon: ic}, Content: Text(label)}
	}
	sep := func(k string) tela.Child {
		return kit.Separator{Orientation: kit.Vertical, Class: r.CommandSeparator, Props: Key(k)}
	}
	return Div(Class(r.CommandBar), Role("toolbar"), AriaLabel("Commands"),
		kit.Button{Variant: kit.Ghost, Icon: icons.Plus, IconEnd: icons.ChevronDown, Children: Text("New")},
		sep("a"),
		cmd("Cut", icons.Scissors),
		cmd("Copy", icons.Copy),
		cmd("Paste", icons.ClipboardPaste),
		cmd("Rename", icons.Pencil),
		cmd("Share", icons.Share2),
		cmd("Delete", icons.Trash2),
		sep("b"),
		kit.Button{Variant: kit.Ghost, Icon: icons.ArrowUpDown, IconEnd: icons.ChevronDown, Children: Text("Sort")},
	)
}

// ---------------------------------------------------------------------------
// The sidebar.

// sidebar lists the looks and the color schemes: one ToggleGroup per section,
// so the arrows move between the entries of a section and the selected one is
// the group's choice. The look puts a heading above a section or a line
// between two.
func (w Window) sidebar() tela.Node {
	r := w.Look.Regions
	kids := []tela.Child{Role("navigation"), AriaLabel("Looks"), Class(r.Sidebar)}

	section := func(i int, title string, group tela.Component) {
		if i > 0 && r.SidebarSeparator != "" {
			kids = append(kids, kit.Separator{Class: r.SidebarSeparator, Props: Key(fmt.Sprint("sep", i))})
		}
		if r.SidebarHeading != "" {
			kids = append(kids, Div(Class(r.SidebarHeading), Key("head"+title), Text(title)))
		}
		kids = append(kids, group)
	}
	entry := func(on bool, name string) tela.Component {
		return Fragment(If(on && r.SidebarMark != "", Div(Class(r.SidebarMark))), Span(Class("truncate"), Text(name)))
	}

	lookItems := make([]kit.ToggleGroupItem[string], len(catalog))
	for i, c := range catalog {
		lookItems[i] = kit.ToggleGroupItem[string]{Value: c.ID, Icon: c.Icon, AriaLabel: c.Title, Children: entry(c.ID == w.ID, c.Title)}
	}
	look := w.ID
	section(0, "Look", kit.ToggleGroup[string]{
		Items: lookItems, Variant: looks.Sidebar, Size: looks.SidebarSize, Orientation: kit.Vertical,
		Value: &look, DisallowEmptySelection: true, AriaLabel: "Look",
		OnValueChange: w.SetLook.Set,
		Class:         "w-full flex-col items-stretch " + r.SidebarGroup,
		Props:         Key("looks"),
	})

	cur := schemeKey(w.Scheme)
	schemeItems := make([]kit.ToggleGroupItem[string], len(schemeKeys))
	for i, k := range schemeKeys {
		schemeItems[i] = kit.ToggleGroupItem[string]{Value: k.Key, Icon: k.Icon, AriaLabel: k.Title, Children: entry(k.Key == cur, k.Title)}
	}
	section(1, "Color scheme", kit.ToggleGroup[string]{
		Items: schemeItems, Variant: looks.Sidebar, Size: looks.SidebarSize, Orientation: kit.Vertical,
		Value: &cur, DisallowEmptySelection: true, AriaLabel: "Color scheme",
		OnValueChange: func(k string) { w.SetScheme.Set(schemeOf(k)) },
		Class:         "w-full flex-col items-stretch " + r.SidebarGroup,
		Props:         Key("schemes"),
	})
	return Div(kids...)
}

// ---------------------------------------------------------------------------
// The details pane and the status bar.

// detailsPane describes the look: its font, radius and the main tokens as
// swatches.
func (w Window) detailsPane() tela.Node {
	r := w.Look.Regions
	th := w.Look.Theme
	prop := func(k, v string) tela.Node {
		return Div(Class("flex flex-col gap-0.5"), Key(k),
			Span(Class(r.DetailsLabel), Text(k)),
			Span(Class(r.DetailsValue), Text(v)))
	}
	swatch := func(name string, bg string) tela.Node {
		return Div(Class("flex flex-col items-center gap-1"), Key(name),
			Div(Class("size-8 rounded-md border border-border", bg)),
			Span(Class(r.DetailsLabel), Text(name)))
	}
	return Div(Class(r.Details), Role("complementary"), AriaLabel("Details"),
		Div(Class(r.DetailsTitle), Text(byID(w.ID).Title)),
		Div(Class("flex flex-col gap-3"),
			prop("Font", th.FontSans),
			prop("Radius", fmt.Sprintf("%g px", th.Radius)),
			prop("Color scheme", byKey(schemeKey(w.Scheme)))),
		Div(Class("flex flex-col gap-2"),
			Span(Class(r.DetailsLabel), Text("Tokens")),
			Div(Class("grid grid-cols-3 gap-y-3"),
				swatch("primary", "bg-primary"),
				swatch("accent", "bg-accent"),
				swatch("muted", "bg-muted"),
				swatch("selection", "bg-selection"),
				swatch("destructive", "bg-destructive"),
			),
		),
	)
}

func index(id string) int {
	for i, c := range catalog {
		if c.ID == id {
			return i
		}
	}
	return 0
}

func byKey(key string) string {
	for _, k := range schemeKeys {
		if k.Key == key {
			return k.Title
		}
	}
	return ""
}

func (w Window) statusBar() tela.Node {
	r := w.Look.Regions
	return Div(Class(r.StatusBar), Role("status"),
		Span(Class("truncate"), Text(byID(w.ID).Title+" · "+byKey(schemeKey(w.Scheme)))),
		Span(Class("truncate text-muted-foreground"), Textf("Look %d of %d", index(w.ID)+1, len(catalog))),
	)
}
