package main

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Explorer is the whole window. One tree serves every theme: the Theme only
// supplies class strings, texts and number formats. Its fields select the
// theme and the screen the explorer starts on (they seed its state); they
// are comparable, so it is not built again when they do not change.
type Explorer struct {
	Theme string // a key of Themes
	Mode  Mode
	Path  Path
	Sel   string
	Query string
}

func (p Explorer) Build() tela.Node {
	th := Themes[p.Theme]
	if th == nil {
		panic(fmt.Sprintf("native: unknown theme %q; use one of %s", p.Theme, strings.Join(ThemeIDs, ", ")))
	}
	l := th.Resolve(p.Mode)

	st, d := tela.UseReducer(reduce, State{Path: p.Path, Sel: p.Sel, Query: p.Query})
	search := tela.UseElementRef()
	list := tela.UseElementRef()

	dir := Lookup(st.Path)
	entries := Sorted(dir, st.Sort, st.Desc, st.Query)
	var sel *Entry
	for _, e := range entries {
		if e.Name == st.Sel {
			sel = e
		}
	}

	// nav dispatches and hands the keyboard back to the list, so the arrow
	// keys keep working after a click on a button.
	nav := func(a Action) {
		d.Dispatch(a)
		list.Focus()
	}
	shortcuts := func(e tela.KeyboardEvent) {
		switch {
		case e.Alt && e.Key == tela.KeyArrowLeft:
			nav(Action{Kind: "back"})
		case e.Alt && e.Key == tela.KeyArrowRight:
			nav(Action{Kind: "forward"})
		case e.Alt && e.Key == tela.KeyArrowUp:
			nav(Action{Kind: "up"})
		case e.Ctrl && (e.Key == "f" || e.Key == "F"):
			e.PreventDefault()
			search.Focus()
		default:
			return
		}
		e.PreventDefault()
	}

	return Div(Class("flex h-screen w-full flex-col overflow-hidden select-none", l.Window),
		OnKeyDown(shortcuts),
		Toolbar{Th: th, L: l, Path: st.Path, Query: st.Query, CanBack: len(st.Back) > 0,
			CanForward: len(st.Forward) > 0, Search: search, D: d, Dir: dir.Name},
		Div(Class("flex min-h-0 flex-1"),
			Sidebar{Th: th, L: l, Path: st.Path, D: d},
			FileList{Th: th, L: l, State: st, Entries: entries, Ref: list, D: d},
			Details(th, l, dir, entries, sel, st.Path),
		),
		StatusBar(th, l, entries, sel, st.Msg),
	)
}

// Toolbar is the navigation buttons, the path bar and the search field,
// plus whatever extras the theme shows (a command bar, a header bar's
// buttons).
type Toolbar struct {
	Th                  *Theme
	L                   *Look
	Path                Path
	Query               string
	CanBack, CanForward bool
	Search              tela.ElementRef
	D                   tela.Dispatcher[Action]
	Dir                 string
}

func (p Toolbar) Build() tela.Node {
	th, l, d := p.Th, p.L, p.D
	px := th.SideIconPx
	navBtn := func(label, glyph, kind string, enabled bool) tela.Node {
		return Button(Class(l.NavBtn), AriaLabel(label), Disabled(!enabled),
			OnClick(func() { d.Dispatch(Action{Kind: kind}) }),
			Glyph(glyph, px))
	}
	dirName := p.Dir
	if dirName == "" {
		dirName = th.Root
	}
	return Div(Class(l.Toolbar),
		Div(Class(l.ToolbarRow),
			Div(Class(l.ToolbarLead),
				Button(Class(l.NavBtn), AriaLabel("Toggle sidebar"), Glyph("sidebar", px)),
				Button(Class(l.NavBtn), AriaLabel("Main menu"), Glyph("menu", px)),
			),
			Div(Class(l.NavGroup),
				navBtn("Back", "back", "back", p.CanBack),
				navBtn("Forward", "forward", "forward", p.CanForward),
				navBtn("Up", "up", "up", p.Path != ""),
			),
			PathBar(th, l, p.Path, d),
			Div(Class(l.Search),
				Span(Class(l.SearchIcon), Glyph("search", 14)),
				Input(Class(l.SearchInput), Type("search"), Placeholder(th.SearchHint(dirName)),
					AriaLabel("Search"), Ref(p.Search), Value(p.Query),
					OnChange(func(e tela.ChangeEvent) { d.Dispatch(Action{Kind: "query", Query: e.Value}) }),
					OnKeyDown(func(e tela.KeyboardEvent) {
						if e.Key == tela.KeyEscape {
							d.Dispatch(Action{Kind: "query"})
							p.Search.Blur()
						}
					})),
			),
			Div(Class(l.ToolbarEnd),
				Button(Class(l.NavBtn), AriaLabel("View as list"), Glyph("view-list", px)),
				Button(Class(l.NavBtn), AriaLabel("More options"), Glyph("more", px)),
			),
		),
		Div(Class(l.CmdBar),
			Button(Class(l.CmdBtn), AriaLabel("New"), Glyph("plus", 16), Span(Text("New")), Glyph("chevron-down", 12)),
			Div(Class(l.CmdSep)),
			Button(Class(l.CmdBtn), AriaLabel("Cut"), Glyph("cut", 16)),
			Button(Class(l.CmdBtn), AriaLabel("Copy"), Glyph("copy", 16)),
			Button(Class(l.CmdBtn), AriaLabel("Paste"), Glyph("paste", 16)),
			Button(Class(l.CmdBtn), AriaLabel("Rename"), Glyph("rename", 16)),
			Button(Class(l.CmdBtn), AriaLabel("Share"), Glyph("share", 16)),
			Button(Class(l.CmdBtn), AriaLabel("Delete"), Glyph("trash", 16)),
			Div(Class(l.CmdSep)),
			Button(Class(l.CmdBtn), AriaLabel("Sort"), Glyph("sort", 16), Span(Text("Sort")), Glyph("chevron-down", 12)),
			Button(Class(l.CmdBtn), AriaLabel("View"), Glyph("view-grid", 16), Span(Text("View")), Glyph("chevron-down", 12)),
			Button(Class(l.CmdBtn), AriaLabel("See more"), Glyph("more", 16)),
		),
	)
}

// PathBar is the breadcrumb bar: one button per folder of the path.
func PathBar(th *Theme, l *Look, path Path, d tela.Dispatcher[Action]) tela.Node {
	segs := Segments(path)
	crumb := func(label, to string, last bool, home bool) tela.Node {
		return Button(Class(l.PathSeg), If(last, Class(l.PathSegCur)), Key(to+"|"+label), AriaLabel("Go to "+label),
			OnClick(func() { d.Dispatch(Action{Kind: "open", Path: to}) }),
			If(home, Glyph("home", 14)),
			Span(Text(label)))
	}
	sep := func(i int) tela.Node {
		return Div(Class(l.PathSep), Key(fmt.Sprint("sep", i)),
			If(th.Chevrons, Glyph("chevron-right", 12)))
	}
	nodes := []tela.Child{crumb(th.Root, "", len(segs) == 0, true)}
	to := Path("")
	for i, s := range segs {
		to = Join(to, s)
		nodes = append(nodes, sep(i), crumb(s, to, i == len(segs)-1, false))
	}
	return Div(append([]tela.Child{Class(l.PathBar), Role("navigation"), AriaLabel("Path")}, nodes...)...)
}

// Sidebar lists the places.
type Sidebar struct {
	Th   *Theme
	L    *Look
	Path Path
	D    tela.Dispatcher[Action]
}

func (p Sidebar) Build() tela.Node {
	th, l, d := p.Th, p.L, p.D
	var kids []tela.Child
	kids = append(kids, Role("navigation"), AriaLabel("Places"), Class(l.Sidebar))
	for i, g := range th.SideGroups {
		if i > 0 {
			kids = append(kids, Div(Class(l.SideSep), Key(fmt.Sprint("sep", i))))
		}
		if g.Title != "" {
			kids = append(kids, Div(Class(l.SideHead), Key("head"+g.Title), Text(g.Title)))
		}
		for _, pl := range g.Items {
			on := p.Path == pl.Path
			kids = append(kids, Button(Class(l.SideItem), If(on, Class(l.SideItemOn)), Key("place"+pl.Name), AriaLabel(pl.Name),
				AriaPressed(on),
				OnClick(func() { d.Dispatch(Action{Kind: "open", Path: pl.Path}) }),
				If(on, Div(Class(l.SideMark))),
				Glyph(pl.Icon, th.SideIconPx),
				Span(Class("truncate"), Text(pl.Name)),
			))
		}
	}
	return Div(kids...)
}

func (l *Look) width(c Column) string {
	switch c {
	case ColModified:
		return l.WModified
	case ColType:
		return l.WType
	case ColSize:
		return l.WSize
	}
	return l.WName
}

// FileList is the column header and the rows.
type FileList struct {
	Th      *Theme
	L       *Look
	State   State
	Entries []*Entry
	Ref     tela.ElementRef
	D       tela.Dispatcher[Action]
}

func (p FileList) Build() tela.Node {
	th, l, st, d := p.Th, p.L, p.State, p.D
	header := func(c Column) tela.Node {
		sorted := st.Sort == c
		caret := "chevron-up"
		if st.Desc {
			caret = "chevron-down"
		}
		return Button(Class(l.ColCell, l.width(c)), Key(int(c)), AriaLabel("Sort by "+th.Label(c)),
			OnClick(func() { d.Dispatch(Action{Kind: "sort", Col: c}) }),
			Span(Class("truncate"), Text(th.Label(c))),
			If(sorted, Glyph(caret, 12)),
			Div(Class(l.ColSep)),
		)
	}
	var cols []tela.Child
	cols = append(cols, Class(l.ColHeader))
	for _, c := range th.Columns {
		cols = append(cols, header(c))
	}

	empty := "This folder is empty"
	if st.Query != "" {
		empty = "No results for “" + st.Query + "”"
	}
	body := tela.Node(Div(Class(l.Empty), Text(empty)))
	if len(p.Entries) > 0 {
		body = MapKeyed(p.Entries, func(e *Entry) string { return e.Name }, func(e *Entry) tela.Component {
			return Row{Th: th, L: l, E: e, On: e.Name == st.Sel, Path: st.Path, D: d}
		})
	}

	return Div(Class(l.Content),
		Div(cols...),
		Div(Class(l.List), Ref(p.Ref), Role("listbox"), AriaLabel("Files"), TabIndex(0), AutoFocus(true),
			OnKeyDown(func(e tela.KeyboardEvent) {
				if e.Alt || e.Ctrl || e.Meta {
					return
				}
				switch e.Key {
				case tela.KeyArrowDown:
					d.Dispatch(Action{Kind: "move", Delta: 1})
				case tela.KeyArrowUp:
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
				default:
					return
				}
				e.PreventDefault()
			}),
			body,
		),
	)
}

// Icon returns the icon of an entry at px pixels.
func Icon(l *Look, e *Entry, px float32) tela.Node {
	if e.IsFolder() {
		return FolderIcon(l, px)
	}
	return FileIcon(l, e.Kind, px)
}

// Row is one entry of the list.
type Row struct {
	Th   *Theme
	L    *Look
	E    *Entry
	On   bool
	Path Path
	D    tela.Dispatcher[Action]
}

func (p Row) Build() tela.Node {
	th, l, e, d := p.Th, p.L, p.E, p.D
	ref := tela.UseElementRef()
	tela.UseEffectDo(func() {
		if p.On {
			ref.ScrollIntoView()
		}
	}, p.On)

	cls, dim := l.RowIdle, l.CellDim
	if p.On {
		cls, dim = l.RowOn, l.CellDimOn
	}
	cell := func(c Column) tela.Node {
		switch c {
		case ColModified:
			return Div(Class("truncate", l.WModified, dim), Key(int(c)), Text(th.Date(e.Modified)))
		case ColType:
			return Div(Class("truncate", l.WType, dim), Key(int(c)), Text(th.TypeName(e)))
		case ColSize:
			size := ""
			if e.IsFolder() {
				size = th.FolderSize(len(e.Visible()))
			} else {
				size = th.Size(e.Size)
			}
			return Div(Class("truncate", l.WSize, dim), Key(int(c)), Text(size))
		}
		return Div(Class("flex items-center gap-2.5", l.WName), Key(int(c)),
			Icon(l, e, th.IconPx),
			Span(Class("truncate"), Text(e.Name)))
	}
	kids := []tela.Child{Class(l.Row, cls), Ref(ref), Role("option"), AriaLabel(e.Name), AriaSelected(p.On),
		OnClick(func() { d.Dispatch(Action{Kind: "select", Name: e.Name}) }),
		OnDoubleClick(func() {
			d.Dispatch(Action{Kind: "select", Name: e.Name})
			d.Dispatch(Action{Kind: "activate"})
		}),
	}
	for _, c := range th.Columns {
		kids = append(kids, cell(c))
	}
	return Div(kids...)
}

// Details is the preview pane: the selected entry, or the folder.
func Details(th *Theme, l *Look, dir *Entry, entries []*Entry, sel *Entry, path Path) tela.Node {
	e := sel
	if e == nil {
		e = dir
	}
	name := e.Name
	if name == "" {
		name = th.Root
	}
	prop := func(k, v string) tela.Node {
		return Div(Class("flex flex-col gap-0.5"), Key(k),
			Span(Class(l.DetailsKey), Text(k)),
			Span(Class(l.DetailsVal), Text(v)))
	}
	var props []tela.Child
	props = append(props, Class("flex flex-col gap-3"))
	if e.IsFolder() {
		items := len(e.Visible())
		if e == dir {
			items = len(entries)
		}
		props = append(props, prop("Type", th.TypeName(e)), prop("Items", fmt.Sprint(items)))
	} else {
		props = append(props, prop("Type", th.TypeName(e)), prop("Size", th.Size(e.Size)))
	}
	props = append(props, prop(th.Label(ColModified), th.Date(e.Modified)))
	loc := path
	if sel == nil {
		loc = Parent(path)
	}
	props = append(props, prop("Location", strings.Join(append([]string{th.Root}, Segments(loc)...), " › ")))
	return Div(Class(l.Details), Role("complementary"), AriaLabel("Details"),
		Div(Class(l.DetailsPreview), Icon(l, e, 72)),
		Div(Class(l.DetailsTitle), Text(name)),
		Div(props...),
	)
}

// StatusBar counts the entries and the selection.
func StatusBar(th *Theme, l *Look, entries []*Entry, sel *Entry, msg string) tela.Node {
	files, folders, size := Total(entries)
	text := th.Status(files, folders, size, sel)
	if msg != "" {
		text = msg
	}
	return Div(Class(l.Status), Role("status"),
		Span(Class("truncate"), Text(text)),
		Div(Class(l.Zoom),
			Div(Class(l.ZoomTrack), Div(Class(l.ZoomKnob))),
		),
		Div(Class("flex items-center gap-1"),
			Button(Class(l.StatusToggle, l.StatusToggleOn), AriaLabel("Details view"), AriaPressed(true), Glyph("view-list", 16)),
			Button(Class(l.StatusToggle), AriaLabel("Large icons view"), AriaPressed(false), Glyph("view-grid", 16)),
		),
	)
}
