package main

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks/gallery/icons"
)

// Controls is the content area: kit's controls in two columns (Columns is
// "columns") or in one ("stack"). It holds the state of the controls that
// have any.
type Controls struct {
	Columns string
}

// asset is a row of the table.
type asset struct {
	Name, Kind string
	KB         int
}

var assets = []asset{
	{"Report", "Document", 2450},
	{"Photo", "Image", 860},
	{"Notes", "Text", 12},
	{"Budget", "Sheet", 340},
}

// noSort is the arrow of a sortable column header that is not the sorted
// one: an empty drawing, so that only the sorted column shows an arrow and
// the headers do not move when the order changes.
var noSort = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"></svg>`}

func size(kb int) string {
	if kb >= 1000 {
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(kb)/1000), "0"), ".") + " MB"
	}
	return fmt.Sprintf("%d KB", kb)
}

// section is a titled group of controls.
func section(title string, kids ...tela.Child) tela.Node {
	all := []tela.Child{Class("flex flex-col gap-2.5"), Key(title),
		Span(Class("text-[0.85em] font-semibold text-muted-foreground"), Text(title))}
	return Div(append(all, kids...)...)
}

func (c Controls) Build() tela.Node {
	volume, setVolume := tela.UseState(40.0)
	sort, setSort := tela.UseState(kit.SortState{Column: "name", Direction: kit.Ascending})
	selected, setSelected := tela.UseState("Photo")
	renaming, setRenaming := tela.UseState(false)
	layout, setLayout := tela.UseState("day")
	fruit, setFruit := tela.UseState("apple")

	buttons := section("Buttons",
		Div(Class("flex flex-wrap items-center gap-2"),
			kit.Button{Children: Text("Default")},
			kit.Button{Variant: kit.Secondary, Children: Text("Secondary")},
			kit.Button{Variant: kit.Outline, Children: Text("Outline")},
			kit.Button{Variant: kit.Ghost, Children: Text("Ghost")},
			kit.Button{Variant: kit.Destructive, Children: Text("Delete")},
			kit.Button{Variant: kit.Link, Children: Text("Link")},
			kit.Button{Variant: kit.Outline, Icon: icons.Plus, Children: Text("New")},
			kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: icons.Bell, AriaLabel: "Notifications"},
			kit.Button{Disabled: true, Children: Text("Disabled")},
		))

	fields := section("Fields",
		kit.Input{Placeholder: "Your name", AriaLabel: "Name", Class: "pl-3"},
		kit.Select[string]{
			Value: &fruit, OnValueChange: setFruit.Set, AriaLabel: "Fruit", Class: "w-full",
			Items: []kit.SelectItem[string]{
				{Value: "apple", Label: "Apple"}, {Value: "banana", Label: "Banana"},
				{Value: "cherry", Label: "Cherry"}, {Value: "grape", Label: "Grape", Disabled: true},
			},
		})

	choices := section("Choices",
		Div(Class("flex flex-col gap-2"),
			Div(Class("flex items-center gap-2"), kit.Checkbox{DefaultChecked: true, AriaLabel: "Notifications", ID: "c1"},
				kit.Label{For: "c1", Children: Text("Notifications")}),
			Div(Class("flex items-center gap-2"), kit.Checkbox{AriaLabel: "Sounds", ID: "c2"},
				kit.Label{For: "c2", Children: Text("Sounds")}),
			Div(Class("flex items-center gap-2"), kit.Switch{DefaultChecked: true, AriaLabel: "Autosave", ID: "s1"},
				kit.Label{For: "s1", Children: Text("Autosave")}),
		),
		kit.ToggleGroup[string]{
			Items: []kit.ToggleGroupItem[string]{
				{Value: "day", Label: "Day"}, {Value: "week", Label: "Week"}, {Value: "month", Label: "Month"},
			},
			Variant: kit.Outline, Spacing: 1, Class: "gap-1", Value: &layout, OnValueChange: setLayout.Set, DisallowEmptySelection: true, AriaLabel: "Range",
		})

	ranges := section("Range",
		kit.Slider{Value: &volume, OnValueChange: setVolume.Set, Max: 100, AriaLabel: "Volume"},
		kit.Progress{Value: volume, AriaLabel: "Volume level"})

	tabs := section("Tabs",
		kit.Tabs[string]{DefaultValue: new("overview"), AriaLabel: "Sections", Tabs: []kit.Tab[string]{
			{Value: "overview", Label: "Overview", Content: Div(Class("pt-2 text-muted-foreground"), Text("Everything at a glance."))},
			{Value: "activity", Label: "Activity", Content: Div(Class("pt-2 text-muted-foreground"), Text("What changed lately."))},
			{Value: "settings", Label: "Settings", Content: Div(Class("pt-2 text-muted-foreground"), Text("Names and access."))},
		}})

	table := section("Table",
		Div(Class("flex flex-col overflow-hidden"),
			kit.Table[asset]{
				Rows: assets, RowKey: func(a asset) any { return a.Name }, AriaLabel: "Files",
				Sort: &sort, OnSortChange: func(s kit.SortState) {
					if s.Direction == "" {
						s = kit.SortState{Column: "name", Direction: kit.Ascending}
					}
					setSort.Set(s)
				},
				SortIcon: noSort, SortAscIcon: icons.ChevronUp, SortDescIcon: icons.ChevronDown,
				Selected: func(a asset) bool { return a.Name == selected },
				RowProps: func(a asset) tela.Child {
					return tela.MergeProps(AriaLabel(a.Name), OnClick(func() { setSelected.Set(a.Name) }))
				},
				Class: "flex-none",
				Columns: []kit.Column[asset]{
					{ID: "name", Header: "Name", Width: "minmax(0,1fr)", Cell: func(a asset) tela.Node { return Span(Class("truncate"), Text(a.Name)) },
						Sort: func(a, b asset) int { return cmp.Compare(a.Name, b.Name) }},
					{ID: "kind", Header: "Kind", Width: "80px", Class: "text-muted-foreground group-aria-selected:text-selection-foreground",
						Cell: func(a asset) tela.Node { return Span(Class("truncate"), Text(a.Kind)) },
						Sort: func(a, b asset) int { return cmp.Compare(a.Kind, b.Kind) }},
					{ID: "size", Header: "Size", Width: "76px", Align: kit.AlignRight, Class: "text-muted-foreground group-aria-selected:text-selection-foreground",
						Cell: func(a asset) tela.Node { return Span(Class("truncate"), Text(size(a.KB))) },
						Sort: func(a, b asset) int { return cmp.Compare(a.KB, b.KB) }},
				},
			}))

	overlays := section("Menus and dialogs",
		Div(Class("flex flex-wrap items-center gap-2"),
			kit.DropdownMenu[string]{
				Trigger:   kit.Button{Variant: kit.Outline, IconEnd: icons.ChevronDown, Children: Text("Actions")},
				AriaLabel: "Actions", Align: "start",
				Items: []kit.MenuItem[string]{
					{Key: "cut", Label: "Cut", Icon: icons.Scissors, Shortcut: "Ctrl+X"},
					{Key: "copy", Label: "Copy", Icon: icons.Copy, Shortcut: "Ctrl+C"},
					{Key: "paste", Label: "Paste", Icon: icons.ClipboardPaste, Shortcut: "Ctrl+V", Disabled: true},
					{Kind: kit.MenuSeparator},
					{Key: "delete", Label: "Delete", Icon: icons.Trash2, Variant: kit.Destructive},
				},
				OnSelect: func(string) {},
			},
			kit.Dialog{
				Trigger: kit.Button{Variant: kit.Outline, Icon: icons.Pencil, Children: Text("Rename…")},
				Title:   Text("Rename"), Description: Text("Type a new name for this item."),
				Open: &renaming, OnOpenChange: setRenaming.Set, HideClose: true,
				Children: kit.Input{DefaultValue: selected, AriaLabel: "Name", AutoFocus: true, Class: "pl-3"},
				Footer: Div(Class("flex justify-end gap-2"),
					kit.Button{Variant: kit.Outline, OnClick: func() { setRenaming.Set(false) }, Children: Text("Cancel")},
					kit.Button{OnClick: func() { setRenaming.Set(false) }, Children: Text("Rename")}),
			},
		))

	card := kit.Card{
		TitleLevel: 2, Title: Text("Storage"), Description: Text("62% of 10 GB used"),
		Children: kit.Progress{Value: 62, AriaLabel: "Storage used"},
		Footer:   Div(Class("flex gap-2"), kit.Button{Size: kit.Sm, Children: Text("Upgrade")}, kit.Button{Variant: kit.Ghost, Size: kit.Sm, Children: Text("Manage")}),
	}

	left := []tela.Child{buttons, fields, choices, ranges}
	right := []tela.Child{tabs, table, overlays, card}

	dir := "flex-row"
	if c.Columns == "stack" {
		dir = "flex-col"
	}
	col := func(kids []tela.Child, k string) tela.Node {
		return Div(append([]tela.Child{Class("flex min-w-0 flex-1 flex-col gap-5"), Key(k)}, kids...)...)
	}
	return Div(Class("flex gap-5 p-4", dir), col(left, "left"), col(right, "right"))
}
