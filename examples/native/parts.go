package main

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/native/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// ---------------------------------------------------------------------------
// The sidebar.

// sidebar lists the places: one ToggleGroup per section, so the arrows move
// between the places of a section and the selected one is the group's
// choice. The look puts a heading above a section or a line between two.
func (w *win) sidebar() tela.Node {
	l, f := w.l, w.l.Frame
	kids := []tela.Child{Role("navigation"), AriaLabel("Places"), Class(f.Sidebar)}
	for i, g := range l.SideGroups {
		if i > 0 && f.SideSep != "" {
			kids = append(kids, kit.Separator{Class: f.SideSep, Props: Key(fmt.Sprint("sep", i))})
		}
		if g.Title != "" {
			kids = append(kids, Div(Class(f.SideHead), Key("head"+g.Title), Text(g.Title)))
		}
		// The group's choice is the place whose folder is open; a section
		// that does not hold it has none (the empty path is Home, so the
		// "none" key is one no folder has).
		current := "\x00none"
		items := make([]kit.ToggleGroupItem[Path], len(g.Items))
		for j, pl := range g.Items {
			on := w.st.Path == pl.Path
			if on {
				current = pl.Path
			}
			items[j] = kit.ToggleGroupItem[Path]{
				Value: pl.Path, Icon: pl.Icon, AriaLabel: pl.Name,
				Children: Fragment(If(on && f.SideMark != "", Div(Class(f.SideMark))), Span(Class("truncate"), Text(pl.Name))),
			}
		}
		title := g.Title
		if title == "" {
			title = g.Label
		}
		kids = append(kids, kit.ToggleGroup[Path]{
			Items: items, Variant: Sidebar, Size: SideSize, Orientation: kit.Vertical,
			Value: &current, DisallowEmptySelection: true, AriaLabel: title,
			OnValueChange: func(p Path) { w.d.Dispatch(Action{Kind: "open", Path: p}) },
			Class:         "w-full flex-col items-stretch " + f.SideGroup,
			Props:         Key("group" + g.Title + fmt.Sprint(i)),
		})
	}
	return Div(kids...)
}

// ---------------------------------------------------------------------------
// The file list.

// content is the file list: a table, or tiles in the icon view, inside the
// context menu of the files. The element that takes the keyboard is the one
// between the menu's area and the list, so the menu's keys (the context menu
// key, Shift+F10) reach the area from it.
func (w *win) content() tela.Node {
	var body tela.Component
	if w.view == "icons" {
		body = w.tiles()
	} else {
		body = w.table()
	}
	return Div(Class(w.l.Frame.Content),
		kit.ContextMenu[string]{
			AriaLabel: "File actions", Class: "flex min-h-0 flex-1 flex-col",
			Items: w.contextItems(), OnSelect: w.onMenu,
			Children: Div(Class("flex min-h-0 flex-1 flex-col outline-none"), Ref(w.list), TabIndex(0), AutoFocus(true),
				OnKeyDown(w.listKeys), body),
		})
}

// listKeys is the keyboard of the list: the arrows, Page keys, Home and End
// move the selection, Enter opens, Backspace goes back, F2 renames, Delete
// deletes and Ctrl+X, C and V are the clipboard. Keys that bubble up from a
// button in the header are left to it.
func (w *win) listKeys(e tela.KeyboardEvent) {
	if !w.list.Focused() {
		return
	}
	d, st := w.d, w.st
	if e.Ctrl && !e.Alt && !e.Meta {
		switch strings.ToLower(string(e.Key)) {
		case "x":
			d.Dispatch(Action{Kind: "cut", Name: st.Sel})
		case "c":
			d.Dispatch(Action{Kind: "copy", Name: st.Sel})
		case "v":
			d.Dispatch(Action{Kind: "paste"})
		default:
			return
		}
		e.PreventDefault()
		return
	}
	if e.Alt || e.Ctrl || e.Meta {
		return
	}
	switch e.Key {
	case tela.KeyArrowDown:
		d.Dispatch(Action{Kind: "move", Delta: 1})
	case tela.KeyArrowUp:
		d.Dispatch(Action{Kind: "move", Delta: -1})
	case tela.KeyArrowRight:
		if w.view != "icons" {
			return
		}
		d.Dispatch(Action{Kind: "move", Delta: 1})
	case tela.KeyArrowLeft:
		if w.view != "icons" {
			return
		}
		d.Dispatch(Action{Kind: "move", Delta: -1})
	case tela.KeyPageDown:
		d.Dispatch(Action{Kind: "move", Delta: 10})
	case tela.KeyPageUp:
		d.Dispatch(Action{Kind: "move", Delta: -10})
	case tela.KeyHome:
		d.Dispatch(Action{Kind: "first"})
	case tela.KeyEnd:
		d.Dispatch(Action{Kind: "last"})
	case tela.KeyEnter:
		d.Dispatch(Action{Kind: "activate"})
	case tela.KeyBackspace:
		d.Dispatch(Action{Kind: "back"})
	case tela.KeyF2:
		if w.sel == nil {
			return
		}
		w.setRename.Set(true)
	case tela.KeyDelete:
		if w.sel == nil {
			return
		}
		w.setDelete.Set(true)
	default:
		return
	}
	e.PreventDefault()
}

// iconPx is the size of the icons of the list, which the zoom slider of
// KDE changes.
func (w *win) iconPx() float32 {
	if !w.l.Zoom {
		return w.l.IconPx
	}
	return 16 + float32(w.zoom)*0.15
}

// rowProps are the handlers of what a row of the list is made of: a click
// selects, a double click opens and a right click selects what the context
// menu is about. A table has no props for a row, so each cell's content
// carries them.
func (w *win) rowProps(e *Entry) []tela.Child {
	d := w.d
	return []tela.Child{
		OnClick(func() { d.Dispatch(Action{Kind: "select", Name: e.Name}) }),
		OnContextMenu(func() { d.Dispatch(Action{Kind: "select", Name: e.Name}) }),
		OnDoubleClick(func() {
			d.Dispatch(Action{Kind: "select", Name: e.Name})
			d.Dispatch(Action{Kind: "activate"})
		}),
	}
}

// table is the file list: a kit.Table whose headers sort.
func (w *win) table() tela.Component {
	l, f, st, d := w.l, w.l.Frame, w.st, w.d
	cols := make([]kit.Column[*Entry], len(l.Columns))
	for i, c := range l.Columns {
		col := kit.Column[*Entry]{
			ID: colIDs[c], Header: l.Label(c), Sortable: true, Width: l.Widths[c],
			Cell: func(e *Entry) tela.Node { return w.cell(c, e) },
		}
		if l.Right[c] {
			col.Align = kit.AlignRight
		}
		if c != ColName {
			col.Class = "text-muted-foreground " + f.CellDimOn
		}
		cols[i] = col
	}
	dir := kit.Ascending
	if st.Desc {
		dir = kit.Descending
	}
	sort := kit.SortState{Column: colIDs[st.Sort], Direction: dir}
	empty := "This folder is empty"
	if st.Query != "" {
		empty = "No results for “" + st.Query + "”"
	}
	return kit.Table[*Entry]{
		Rows: w.entries, RowKey: func(e *Entry) any { return e.Name }, Columns: cols,
		Sort: &sort, AriaLabel: "Files", Empty: Text(empty),
		SortIcon: noSort, SortAscIcon: icons.ChevronUp, SortDescIcon: icons.ChevronDown,
		// The third press of a header would leave the table unsorted; the
		// explorer always has an order, so it goes back to the name.
		OnSortChange: func(s kit.SortState) {
			if s.Direction == "" {
				d.Dispatch(Action{Kind: "sort", Col: ColName})
				return
			}
			d.Dispatch(Action{Kind: "sort", Col: colOf(s.Column), Desc: s.Direction == kit.Descending})
		},
		Selected: func(e *Entry) bool { return e.Name == st.Sel },
	}
}

// cell is the content of one cell of a row.
func (w *win) cell(c Column, e *Entry) tela.Node {
	l := w.l
	align := ""
	if l.Right[c] {
		align = "justify-end text-right"
	}
	kids := append([]tela.Child{Class("flex h-full w-full min-w-0 items-center gap-2.5 px-2", align)}, w.rowProps(e)...)
	var text string
	switch c {
	case ColName:
		kids = append(kids, EntryIcon(e, w.iconPx(), w.l.Frame.IconOn))
		text = e.Name
	case ColModified:
		text = l.Date(e.Modified)
	case ColType:
		text = l.TypeName(e)
	case ColSize:
		text = l.SizeCell(e)
	}
	kids = append(kids, Span(Class("truncate"), Text(text)))
	return Div(kids...)
}

// tiles is the icon view: one toggle button per entry.
func (w *win) tiles() tela.Component {
	if len(w.entries) == 0 {
		return Fragment(Div(Class("flex flex-1 items-center justify-center text-muted-foreground"), Text(w.emptyText())))
	}
	return Fragment(Div(Class("flex min-h-0 flex-1 flex-wrap content-start gap-1 overflow-y-auto p-3"), AriaLabel("Files"),
		MapKeyed(w.entries, func(e *Entry) string { return e.Name }, func(e *Entry) tela.Component {
			return tile{E: e, On: e.Name == w.st.Sel, Px: w.iconPx() * 2, IconOn: w.l.Frame.TileIconOn, D: w.d}
		})))
}

func (w *win) emptyText() string {
	if w.st.Query != "" {
		return "No results for “" + w.st.Query + "”"
	}
	return "This folder is empty"
}

// tile is one entry of the icon view. It is a kit.Toggle: pressed while the
// entry is selected, so the toggle's own aria-pressed and its hover are what
// show the selection.
type tile struct {
	E      *Entry
	On     bool
	Px     float32
	IconOn string
	D      tela.Dispatcher[Action]
}

func (p tile) Build() tela.Node {
	on, e, d := p.On, p.E, p.D
	iconOn := ""
	if on {
		iconOn = p.IconOn
	}
	return Fragment(kit.Toggle{
		Variant: Tile, Size: TileSize, Pressed: &on,
		OnPressedChange: func(bool) { d.Dispatch(Action{Kind: "select", Name: e.Name}) },
		Props: OnDoubleClick(func() {
			d.Dispatch(Action{Kind: "select", Name: e.Name})
			d.Dispatch(Action{Kind: "activate"})
		}),
		Children: Fragment(EntryIcon(e, p.Px, iconOn), Span(Class("w-full break-words"), Text(e.Name))),
	})
}

// contextItems are the entries of the menu a right click on the files opens.
func (w *win) contextItems() []kit.MenuItem[string] {
	type item = kit.MenuItem[string]
	paste := item{Key: "paste", Label: "Paste", Icon: icons.ClipboardPaste, Shortcut: "Ctrl+V", Disabled: w.st.Clip == nil}
	newFolder := item{Key: "new-folder", Label: "New folder", Icon: icons.FolderPlus}
	sep := item{Kind: kit.MenuSeparator}
	if w.sel == nil {
		return []item{paste, sep, newFolder}
	}
	return []item{
		{Key: "open", Label: "Open", Icon: icons.FolderOpen, Shortcut: "Enter"},
		sep,
		{Key: "cut", Label: "Cut", Icon: icons.Scissors, Shortcut: "Ctrl+X"},
		{Key: "copy", Label: "Copy", Icon: icons.Copy, Shortcut: "Ctrl+C"},
		paste,
		sep,
		{Key: "rename", Label: "Rename", Icon: icons.Pencil, Shortcut: "F2"},
		{Key: "delete", Label: "Delete", Icon: icons.Trash2, Shortcut: "Del", Variant: kit.Destructive},
		sep,
		newFolder,
	}
}

// ---------------------------------------------------------------------------
// The details pane and the status bar.

// detailsPane shows the selected entry, or the folder.
func (w *win) detailsPane() tela.Node {
	l, f := w.l, w.l.Frame
	e := w.sel
	if e == nil {
		e = w.st.Dir()
	}
	name := e.Name
	if name == "" {
		name = l.Root
	}
	prop := func(k, v string) tela.Node {
		return Div(Class("flex flex-col gap-0.5"), Key(k),
			Span(Class(f.DetailsKey), Text(k)),
			Span(Class(f.DetailsVal), Text(v)))
	}
	props := []tela.Child{Class("flex flex-col gap-3")}
	if e.IsFolder() {
		items := len(e.Visible())
		if w.sel == nil {
			items = len(w.entries)
		}
		props = append(props, prop("Type", l.TypeName(e)), prop("Items", fmt.Sprint(items)))
	} else {
		props = append(props, prop("Type", l.TypeName(e)), prop("Size", l.Size(e.Size)))
	}
	props = append(props, prop(l.Label(ColModified), l.Date(e.Modified)))
	loc := w.st.Path
	if w.sel == nil {
		loc = Parent(loc)
	}
	props = append(props, prop("Location", strings.Join(append([]string{l.Root}, Segments(loc)...), " › ")))
	return Div(Class(f.Details), Role("complementary"), AriaLabel("Details"),
		Div(Class(f.DetailsPreview), EntryIcon(e, 72)),
		Div(Class(f.DetailsTitle), Text(name)),
		Div(props...),
	)
}

// statusBar counts the entries and the selection; KDE adds the zoom slider
// and Windows the view switcher.
func (w *win) statusBar() tela.Node {
	l, f := w.l, w.l.Frame
	files, folders, size := Total(w.entries)
	text := l.Status(files, folders, size, w.sel)
	if w.st.Msg != "" {
		text = w.st.Msg
	}
	kids := []tela.Child{Class(f.Status), Role("status"), Span(Class("truncate"), Text(text))}
	if l.Zoom {
		zoom := w.zoom
		kids = append(kids, kit.Slider{Value: &zoom, OnValueChange: w.setZoom.Set, Max: 100, AriaLabel: "Zoom", Class: "ml-auto w-[130px]"})
	}
	if l.StatusViews {
		kids = append(kids, w.viewSwitcher())
	}
	return Div(kids...)
}

// ---------------------------------------------------------------------------
// The rename dialog.

// renameForm is the content of the rename dialog. It exists only while the
// dialog is open, so the text starts as the name each time.
type renameForm struct {
	Name     string
	OnRename func(to string)
	OnCancel func()
}

func (p renameForm) Build() tela.Node {
	text, setText := tela.UseState(p.Name)
	return Form(Class("grid gap-4"), OnSubmit(func() { p.OnRename(text) }),
		kit.Input{Placeholder: "Name", Value: &text, OnValueChange: setText.Set, AutoFocus: true, Class: "pl-3"},
		Div(Class("flex justify-end gap-2"),
			kit.Button{Variant: kit.Outline, Type: "button", OnClick: p.OnCancel, Children: Text("Cancel")},
			kit.Button{Type: "submit", Children: Text("Rename")},
		),
	)
}
