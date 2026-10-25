package main

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/native/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Explorer is the app: it holds which look is drawn and gives the window the
// look's style and theme. Switching look (the Look group of the menu) swaps
// the kit.StyleProvider's style and calls tela.SetTheme with the look's
// tokens; the tree below never asks which platform it is.
//
// Its fields select the look, the color scheme and the screen the explorer
// starts on (they seed its state); they are comparable, so it is not built
// again when they do not change.
type Explorer struct {
	Look   string           // a key of Looks
	Scheme tela.ColorScheme // System follows the system's setting
	Path   Path
	Sel    string
	Query  string
	View   string // "details" (the default) or "icons"
}

func (p Explorer) Build() tela.Node {
	look, setLook := tela.UseState(p.Look)
	l := Looks[look]
	if l == nil {
		panic(fmt.Sprintf("native: unknown look %q; use one of %s", look, strings.Join(LookIDs, ", ")))
	}
	// The tokens of the look: App.Theme has them already when the app
	// starts with this look (the first build), and this is what changes
	// them when the look does.
	tela.UseUpdateEffect(func() func() {
		tela.SetTheme(l.Theme)
		return nil
	}, look)

	return Div(Class("h-screen w-full bg-background text-foreground"), Scheme(p.Scheme),
		kit.StyleProvider{Style: l.Style, Children: Window{Look: l, SetLook: setLook, Path: p.Path, Sel: p.Sel, Query: p.Query, View: p.View}})
}

// Window is the explorer's window: the toolbar, the sidebar, the file list,
// the details pane and the status bar of one look.
type Window struct {
	Look    *Look
	SetLook tela.Setter[string]
	Path    Path
	Sel     string
	Query   string
	View    string
}

// win is what the parts of the window share while it builds.
type win struct {
	l       *Look
	setLook tela.Setter[string]
	st      State
	d       tela.Dispatcher[Action]
	list    tela.ElementRef
	search  tela.ElementRef
	sel     *Entry
	entries []*Entry

	view       string
	setView    tela.Setter[string]
	details    bool
	setDetails tela.Setter[bool]
	zoom       float64
	setZoom    tela.Setter[float64]
	setRename  tela.Setter[bool]
	setDelete  tela.Setter[bool]
}

func (p Window) Build() tela.Node {
	st, d := tela.UseReducer(reduce, State{FS: root, Path: p.Path, Sel: p.Sel, Query: p.Query})
	view, setView := tela.UseState(p.View)
	details, setDetails := tela.UseState(true)
	sidebar, setSidebar := tela.UseState(true)
	zoom, setZoom := tela.UseState(40.0)
	renaming, setRenaming := tela.UseState(false)
	deleting, setDeleting := tela.UseState(false)
	w := &win{
		l: p.Look, setLook: p.SetLook, st: st, d: d,
		list: tela.UseElementRef(), search: tela.UseElementRef(),
		sel: st.Selected(), entries: st.Entries(),
		view: view, setView: setView, details: details, setDetails: setDetails,
		zoom: zoom, setZoom: setZoom, setRename: setRenaming, setDelete: setDeleting,
	}
	if w.view == "" {
		w.view = "details"
	}
	l, f := w.l, w.l.Frame

	shortcuts := func(e tela.KeyboardEvent) {
		switch {
		case e.Alt && e.Key == tela.KeyArrowLeft:
			w.nav(Action{Kind: "back"})
		case e.Alt && e.Key == tela.KeyArrowRight:
			w.nav(Action{Kind: "forward"})
		case e.Alt && e.Key == tela.KeyArrowUp:
			w.nav(Action{Kind: "up"})
		case e.Ctrl && (e.Key == "f" || e.Key == "F"):
			w.search.Focus()
		default:
			return
		}
		e.PreventDefault()
	}

	body := []tela.Child{Class("flex min-h-0 flex-1")}
	if sidebar || !l.HeaderBar {
		body = append(body, w.sidebar())
	}
	body = append(body, w.content())
	if details {
		body = append(body, w.detailsPane())
	}
	return Div(Class("flex h-full w-full flex-col overflow-hidden select-none", f.Window), OnKeyDown(shortcuts),
		w.toolbar(sidebar, setSidebar),
		Div(body...),
		w.statusBar(),
		kit.Dialog{
			Open: &renaming, OnOpenChange: w.dialogClosed(setRenaming),
			Title: Text("Rename"), Description: Text("Type a new name for this item."),
			Children: renameForm{Name: st.Sel, OnRename: func(to string) {
				d.Dispatch(Action{Kind: "rename", Name: st.Sel, To: to})
				setRenaming.Set(false)
				w.list.Focus()
			}, OnCancel: func() { setRenaming.Set(false); w.list.Focus() }},
		},
		kit.AlertDialog{
			Open: &deleting, OnOpenChange: w.dialogClosed(setDeleting),
			Title:       Text("Delete “" + st.Sel + "”?"),
			Description: Text("The item is deleted for good. This cannot be undone."),
			Action:      Text("Delete"), Destructive: true,
			OnAction: func() { d.Dispatch(Action{Kind: "delete", Name: st.Sel}); w.list.Focus() },
			OnCancel: func() { w.list.Focus() },
		},
	)
}

// dialogClosed hands the keyboard back to the list when a dialog closes.
func (w *win) dialogClosed(set tela.Setter[bool]) func(bool) {
	return func(open bool) {
		set.Set(open)
		if !open {
			w.list.Focus()
		}
	}
}

// nav dispatches and hands the keyboard back to the list, so the arrow keys
// keep working after a click on a button.
func (w *win) nav(a Action) {
	w.d.Dispatch(a)
	w.list.Focus()
}

// iconButton is a toolbar button that holds an icon and has a tooltip.
func (w *win) iconButton(label string, ic tela.Icon, enabled bool, class string, onClick func()) tela.Component {
	return kit.Tooltip{
		Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: label, Icon: ic, Disabled: !enabled, Class: class, OnClick: onClick},
		Content: Text(label),
	}
}

// ---------------------------------------------------------------------------
// The toolbar.

func (w *win) toolbar(sidebar bool, setSidebar tela.Setter[bool]) tela.Node {
	l, f, st := w.l, w.l.Frame, w.st
	dirName := st.Dir().Name
	if dirName == "" {
		dirName = l.Root
	}

	nav := Div(Class(f.Nav),
		w.iconButton("Back", icons.ArrowLeft, len(st.Back) > 0, "", func() { w.nav(Action{Kind: "back"}) }),
		w.iconButton("Forward", icons.ArrowRight, len(st.Forward) > 0, "", func() { w.nav(Action{Kind: "forward"}) }),
		w.iconButton("Up", icons.ArrowUp, st.Path != "", "", func() { w.nav(Action{Kind: "up"}) }),
	)
	search := Div(Class(f.Search),
		Icon(icons.Search, Class(f.SearchIcon)),
		kit.Input{
			Type: "search", Placeholder: l.SearchHint(dirName),
			Value: &st.Query, OnValueChange: func(q string) { w.d.Dispatch(Action{Kind: "query", Query: q}) },
			Props: Ref(w.search),
		},
		// Escape clears the search and hands the keyboard back to the list.
		OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyEscape {
				w.d.Dispatch(Action{Kind: "query"})
				w.search.Blur()
				w.list.Focus()
			}
		}),
	)

	row := []tela.Child{Class(f.ToolbarRow)}
	if l.HeaderBar {
		row = append(row, Div(Class(f.ToolbarLead),
			w.iconButton("Toggle sidebar", icons.PanelLeft, true, "", func() { setSidebar.Set(!sidebar) }),
			kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: "Main menu", Icon: icons.Menu},
				Align:   "start", AriaLabel: "Main menu",
				Items:    w.menu(menuLook, menuDetails),
				OnSelect: w.onMenu,
			},
		))
	}
	row = append(row, nav, w.pathBar(), search)
	if l.ToolbarViews || l.HeaderBar {
		end := []tela.Child{Class(f.ToolbarEnd)}
		if l.ToolbarViews {
			end = append(end, w.viewSwitcher())
		}
		end = append(end, kit.DropdownMenu[string]{
			Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: "More options", Icon: icons.Ellipsis},
			Align:   "end", AriaLabel: "More options",
			Items:    w.menu(menuNew, menuSort, menuDetails, menuLook),
			OnSelect: w.onMenu,
		})
		row = append(row, Div(end...))
	}

	out := []tela.Child{Class("flex shrink-0 flex-col"), Div(row...)}
	if l.CommandBar {
		out = append(out, w.commandBar())
	}
	return Div(out...)
}

// pathBar is the breadcrumb bar: one button per folder of the path.
func (w *win) pathBar() tela.Node {
	l, f := w.l, w.l.Frame
	segs := Segments(w.st.Path)
	crumb := func(label, to string, last, home bool) tela.Component {
		b := kit.Button{
			Variant: kit.Ghost, Size: kit.Sm, AriaLabel: "Go to " + label,
			Props:    Key(to + "|" + label),
			Children: Text(label),
			OnClick:  func() { w.d.Dispatch(Action{Kind: "open", Path: to}) },
		}
		if home {
			b.Icon = icons.House
		}
		if last {
			b.Class = f.PathCur
		}
		return b
	}
	sep := func(i int) tela.Child {
		if l.Chevrons {
			return Div(Class(f.PathSep), Key(fmt.Sprint("sep", i)), Icon(icons.ChevronRight, Class("size-3")))
		}
		return Div(Class(f.PathSep), Key(fmt.Sprint("sep", i)), kit.Separator{Orientation: kit.Vertical, Class: "h-4"})
	}
	nodes := []tela.Child{Class(f.PathBar), Role("navigation"), AriaLabel("Path"), crumb(l.Root, "", len(segs) == 0, true)}
	to := Path("")
	for i, s := range segs {
		to = Join(to, s)
		nodes = append(nodes, sep(i), crumb(s, to, i == len(segs)-1, false))
	}
	return Div(nodes...)
}

// commandBar is Windows' command bar: New, the clipboard, Rename, Share and
// Delete, then Sort, View and the rest as menus.
func (w *win) commandBar() tela.Node {
	f, st := w.l.Frame, w.st
	has := w.sel != nil
	cmd := func(label string, ic tela.Icon, enabled bool, onClick func()) tela.Component {
		return kit.Button{Variant: kit.Ghost, AriaLabel: label, Icon: ic, Disabled: !enabled, OnClick: onClick}
	}
	menuBtn := func(label string, ic tela.Icon) kit.Button {
		return kit.Button{Variant: kit.Ghost, AriaLabel: label, Icon: ic, IconEnd: icons.ChevronDown, Children: Text(label)}
	}
	sep := func(k string) tela.Child {
		return kit.Separator{Orientation: kit.Vertical, Class: f.CmdSep, Props: Key(k)}
	}
	return Div(Class(f.CmdBar),
		kit.DropdownMenu[string]{Trigger: menuBtn("New", icons.Plus), Align: "start", AriaLabel: "New",
			Items: w.menu(menuNew), OnSelect: w.onMenu},
		sep("a"),
		cmd("Cut", icons.Scissors, has, func() { w.nav(Action{Kind: "cut", Name: st.Sel}) }),
		cmd("Copy", icons.Copy, has, func() { w.nav(Action{Kind: "copy", Name: st.Sel}) }),
		cmd("Paste", icons.ClipboardPaste, st.Clip != nil, func() { w.nav(Action{Kind: "paste"}) }),
		cmd("Rename", icons.Pencil, has, func() { w.setRename.Set(true) }),
		cmd("Share", icons.Share2, has, func() { w.nav(Action{Kind: "msg", Name: "Sharing is not available in this demo"}) }),
		cmd("Delete", icons.Trash2, has, func() { w.setDelete.Set(true) }),
		sep("b"),
		kit.DropdownMenu[string]{Trigger: menuBtn("Sort", icons.ArrowUpDown), Align: "start", AriaLabel: "Sort",
			Items: w.menu(menuSort), OnSelect: w.onMenu},
		kit.DropdownMenu[string]{Trigger: menuBtn("View", icons.LayoutGrid), Align: "start", AriaLabel: "View",
			Items: w.menu(menuView, menuDetails), OnSelect: w.onMenu},
		kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, AriaLabel: "See more", Icon: icons.Ellipsis},
			Align: "start", AriaLabel: "See more", Items: w.menu(menuLook), OnSelect: w.onMenu},
	)
}

// viewSwitcher is the segmented control that picks how the folder shows:
// as a list with columns or as large icons.
func (w *win) viewSwitcher() tela.Component {
	view := w.view
	return kit.ToggleGroup[string]{
		Items: []kit.ToggleGroupItem[string]{
			{Value: "details", Icon: icons.List, AriaLabel: "Details view"},
			{Value: "icons", Icon: icons.LayoutGrid, AriaLabel: "Large icons view"},
		},
		Variant: View, Size: ViewSize, Spacing: 1, Class: w.l.Frame.Views,
		Value: &view, OnValueChange: w.setView.Set, DisallowEmptySelection: true, AriaLabel: "View",
	}
}

// ---------------------------------------------------------------------------
// The menus.

// The groups of items a menu is made of.
const (
	menuNew = iota
	menuSort
	menuView
	menuDetails
	menuLook
)

// menu returns the items of the groups, with a separator between them.
func (w *win) menu(groups ...int) []kit.MenuItem[string] {
	var items []kit.MenuItem[string]
	for i, g := range groups {
		if i > 0 {
			items = append(items, kit.MenuItem[string]{Kind: kit.MenuSeparator})
		}
		items = append(items, w.menuGroup(g)...)
	}
	return items
}

func (w *win) menuGroup(g int) []kit.MenuItem[string] {
	type item = kit.MenuItem[string]
	st := w.st
	switch g {
	case menuNew:
		return []item{{Key: "new-folder", Label: "New folder", Icon: icons.FolderPlus}}
	case menuSort:
		sortBy, order := "sort:"+colIDs[st.Sort], "order:asc"
		if st.Desc {
			order = "order:desc"
		}
		return []item{
			{Kind: kit.MenuLabel, Label: "Sort by"},
			{Kind: kit.MenuRadioGroup, Key: "sort", Value: &sortBy, OnValueChange: func(k string) {
				w.d.Dispatch(Action{Kind: "sort", Col: colOf(strings.TrimPrefix(k, "sort:")), Desc: st.Desc})
			}, Items: w.sortItems()},
			{Kind: kit.MenuSeparator},
			{Kind: kit.MenuRadioGroup, Key: "order", Value: &order, OnValueChange: func(k string) {
				w.d.Dispatch(Action{Kind: "sort", Col: st.Sort, Desc: k == "order:desc"})
			}, Items: []item{{Key: "order:asc", Label: "Ascending"}, {Key: "order:desc", Label: "Descending"}}},
		}
	case menuView:
		view := w.view
		return []item{
			{Kind: kit.MenuLabel, Label: "View"},
			{Kind: kit.MenuRadioGroup, Key: "view", Value: &view, OnValueChange: w.setView.Set,
				Items: []item{{Key: "details", Label: "Details"}, {Key: "icons", Label: "Large icons"}}},
		}
	case menuDetails:
		details := w.details
		return []item{{Kind: kit.MenuCheckbox, Key: "details-pane", Label: "Details pane", Checked: &details, OnCheckedChange: w.setDetails.Set}}
	default:
		look := "look:" + w.l.ID
		items := make([]item, len(LookIDs))
		for i, id := range LookIDs {
			items[i] = item{Key: "look:" + id, Label: Looks[id].Name}
		}
		return []item{
			{Kind: kit.MenuLabel, Label: "Look"},
			{Kind: kit.MenuRadioGroup, Key: "look", Value: &look, Items: items,
				OnValueChange: func(k string) { w.setLook.Set(strings.TrimPrefix(k, "look:")) }},
		}
	}
}

// sortItems are the columns of the look as radio items.
func (w *win) sortItems() []kit.MenuItem[string] {
	items := make([]kit.MenuItem[string], len(w.l.Columns))
	for i, c := range w.l.Columns {
		items[i] = kit.MenuItem[string]{Key: "sort:" + colIDs[c], Label: w.l.Label(c)}
	}
	return items
}

// onMenu runs the plain items of every menu.
func (w *win) onMenu(key string) {
	switch key {
	case "new-folder":
		w.nav(Action{Kind: "newfolder"})
	case "open":
		w.nav(Action{Kind: "activate"})
	case "cut", "copy":
		w.nav(Action{Kind: key, Name: w.st.Sel})
	case "paste":
		w.nav(Action{Kind: "paste"})
	case "rename":
		w.setRename.Set(true)
	case "delete":
		w.setDelete.Set(true)
	}
}

// colIDs name the columns for the table's sort state and the menus.
var colIDs = map[Column]string{ColName: "name", ColModified: "modified", ColType: "type", ColSize: "size"}

// colOf is the column with an id.
func colOf(id string) Column {
	for c, s := range colIDs {
		if s == id {
			return c
		}
	}
	return ColName
}
