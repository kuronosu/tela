package devtools

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/kit"
)

// ElementsPanel is the Elements tab: the element tree of the window with a
// pick tool and a search, and the details of the selection.
type ElementsPanel struct{ panelProps }

// Build returns the panel.
func (p ElementsPanel) Build() tela.Node {
	v, m := p.V, p.M
	search, setSearch := tela.UseState("")
	comps, setComps := tela.UseState(false)
	rows := tela.UseMemo(func() []Row {
		return TreeRows(v, TreeOptions{Components: comps, Search: search})
	}, v.ElsRev, v.CollapsedRev, comps, search)
	list := tela.UseElementRef()

	// A reveal (a pick, a selection that moved in a reload) shows the row:
	// the filters that hide it are dropped, and the list scrolls to it.
	seen := tela.UseRef(0)
	tela.UseLayoutEffect(func() func() {
		if seen.Current == v.RevealRev {
			return nil
		}
		i := RowIndex(rows, v.Selected)
		if i < 0 && v.Selected != 0 {
			if comps || search != "" {
				setComps.Set(false)
				setSearch.Set("")
			}
			return nil
		}
		seen.Current = v.RevealRev
		if i >= 0 {
			list.ScrollToIndex(i, tela.AlignCenter)
		}
		return nil
	})

	move := func(i int) {
		if i < 0 || i >= len(rows) {
			return
		}
		m.Select(rows[i].ID, false)
		list.ScrollToIndex(i, tela.AlignAuto)
	}
	onKey := func(e tela.KeyboardEvent) {
		cur := RowIndex(rows, v.Selected)
		switch e.Key {
		case "ArrowDown":
			move(min(cur+1, len(rows)-1))
		case "ArrowUp":
			move(max(cur-1, 0))
		case "Home":
			move(0)
		case "End":
			move(len(rows) - 1)
		case "ArrowRight":
			if cur < 0 {
				return
			}
			if r := rows[cur]; r.HasKids && !r.Open {
				m.ToggleCollapsed(v.Paths[r.I])
			} else {
				move(cur + 1)
			}
		case "ArrowLeft":
			if cur < 0 {
				return
			}
			if r := rows[cur]; r.HasKids && r.Open {
				m.ToggleCollapsed(v.Paths[r.I])
			} else if j := RowIndex(rows, v.Els[r.I].Parent); j >= 0 {
				move(j)
			}
		default:
			return
		}
		e.PreventDefault()
	}

	var body tela.Child
	switch {
	case len(v.Els) == 0:
		title, hint := "Waiting for the app", "The tree appears when the app shows a window."
		if v.Conn == Connected {
			title, hint = "No elements", "The window has no tree yet."
		}
		body = Div(Class("min-h-0 flex-1"), empty(iconLayers, title, hint))
	case len(rows) == 0:
		body = Div(Class("min-h-0 flex-1"), empty(iconSearch, "No element matches", "Nothing in the tree has \""+search+"\" in its kind, key, text or classes."))
	default:
		body = VirtualList(len(rows), func(int) float32 { return 24 }, func(i int) tela.Node {
			r := rows[i]
			el := &v.Els[r.I]
			return TreeRow{
				ID: r.ID, Path: v.Paths[r.I], Depth: r.Depth, HasKids: r.HasKids, Open: r.Open, Dim: r.Dim,
				Selected: r.ID == v.Selected, Kind: el.Kind, Key: el.Key, Text: el.Text, Classes: el.Classes,
				Component: el.Component, Site: el.Site.String(), M: m,
			}
		}, ItemHeight(24), ItemKey(func(i int) core.ElementID { return rows[i].ID }), Ref(list), Role("tree"),
			AriaLabel("Element tree"), TabIndex(0), OnKeyDown(onKey),
			OnMouseLeave(func() { m.Highlight(0) }),
			Class("min-h-0 flex-1 outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-ring/60"))
	}

	viewLabel := "all"
	if comps {
		viewLabel = "components"
	}
	return Div(Class("flex min-h-0 flex-1"),
		Div(Class("flex min-w-0 flex-1 flex-col"),
			Div(Class("flex h-10 shrink-0 items-center gap-2 border-b border-border bg-card px-2"),
				kit.Tooltip{Placement: "bottom", Content: Text("Pick an element in the app (Esc cancels)"), Trigger: kit.Button{
					Variant: pickVariant(v.Picking), Size: kit.IconSm, Icon: iconCrosshair, AriaLabel: "Pick an element",
					Class: "size-7 " + pickClass(v.Picking), Props: AriaPressed(v.Picking), OnClick: m.TogglePick,
				}},
				kit.Input{Icon: iconSearch, Placeholder: "Search by kind, text, key or class", AriaLabel: "Search elements",
					Value: &search, OnValueChange: setSearch.Set, Class: "w-72", InputClass: "h-7 text-xs"},
				kit.ToggleGroup[string]{AriaLabel: "Elements shown", Value: &viewLabel, DisallowEmptySelection: true, Size: kit.Sm,
					OnValueChange: func(s string) { setComps.Set(s == "components") }, Variant: kit.Outline, ItemClass: "h-7 px-2.5 text-xs",
					Items: []kit.ToggleGroupItem[string]{{Value: "all", Label: "All"}, {Value: "components", Label: "Components"}}},
				Div(Class("flex-1")),
				kit.Tooltip{Placement: "bottom", Content: Text("Fold all"), Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: iconFoldAll,
					AriaLabel: "Fold all", Class: "size-7", OnClick: func() { m.SetAllCollapsed(true) }}},
				kit.Tooltip{Placement: "bottom", Content: Text("Unfold all"), Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: iconUnfoldAll,
					AriaLabel: "Unfold all", Class: "size-7", OnClick: func() { m.SetAllCollapsed(false) }}},
			),
			body,
			Div(Class("flex h-6 shrink-0 items-center justify-between border-t border-border bg-card px-3 text-[11px] text-muted-foreground"),
				Span(Textf("%d elements", len(v.Els))),
				Span(Textf("%d shown", len(rows)))),
		),
		Div(Class("flex w-[440px] shrink-0 flex-col border-l border-border bg-card"),
			DetailsPane{V: v, M: m}),
	)
}

func pickVariant(on bool) kit.Variant {
	if on {
		return kit.Secondary
	}
	return kit.Ghost
}

func pickClass(on bool) string {
	if on {
		return "bg-primary/20 text-primary hover:bg-primary/25"
	}
	return ""
}

// TreeRow is a row of the element tree.
type TreeRow struct {
	ID                       core.ElementID
	Path                     string
	Depth                    int
	HasKids, Open, Dim       bool
	Selected                 bool
	Kind, Key, Text, Classes string
	Component                bool
	Site                     string
	M                        *Model
}

// Build returns the row.
func (r TreeRow) Build() tela.Node {
	m, id := r.M, r.ID
	var label tela.Child
	switch {
	case r.Kind == "text":
		label = Span(Class("truncate text-emerald-700 dark:text-emerald-300"), Textf("%q", r.Text))
	case r.Component:
		pkg, name := kindParts(r.Kind)
		label = Fragment(
			Span(Class("shrink-0 text-muted-foreground/70"), Text(pkg)),
			Span(Class("shrink-0 font-medium text-violet-700 dark:text-violet-300"), Text(name)),
		)
	default:
		label = Fragment(
			Span(Class("shrink-0 text-sky-700 dark:text-sky-300"), Text("<"+r.Kind+">")),
			If(r.Classes != "", Span(Class("truncate pl-2 text-muted-foreground/80"), Text(r.Classes))),
		)
	}
	var chevron tela.Child = Span(Class("size-4 shrink-0"))
	if r.HasKids {
		ic := iconChevronRight
		if r.Open {
			ic = iconChevronDown
		}
		chevron = Span(Class("flex size-4 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-foreground"),
			OnClick(tela.StopPropagation(func() { m.ToggleCollapsed(r.Path) })), Icon(ic, Class("size-3.5")))
	}
	return Div(
		Role("treeitem"), AriaSelected(r.Selected), AriaLabel(rowName(r)), AriaLevel(r.Depth+1),
		If(r.HasKids, AriaExpanded(r.Open)),
		Class("flex h-6 cursor-default items-center gap-1 pr-3 font-mono text-[12px] whitespace-nowrap",
			fmt.Sprintf("pl-[%dpx]", 6+r.Depth*14),
			ifs(r.Selected, "bg-primary/20 text-foreground", "hover:bg-accent/60"),
			ifs(r.Dim, "opacity-60", "")),
		OnClick(func() { m.Select(id, false) }),
		OnDoubleClick(func() {
			if r.HasKids {
				m.ToggleCollapsed(r.Path)
			}
		}),
		OnMouseEnter(func() { m.Highlight(id) }),
		chevron,
		label,
		If(r.Key != "", Span(Class("shrink-0 pl-2 text-amber-700 dark:text-amber-300"), Textf("key=%q", r.Key))),
		If(r.Component && r.Site != "", Span(Class("ml-auto shrink-0 pl-4 text-[11px] text-muted-foreground/70"), Text(r.Site))),
	)
}

// ifs returns a when cond holds and b otherwise.
func ifs(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func rowName(r TreeRow) string {
	if r.Kind == "text" {
		return fmt.Sprintf("text %q", r.Text)
	}
	if r.Key != "" {
		return fmt.Sprintf("%s key %s", r.Kind, r.Key)
	}
	return r.Kind
}
