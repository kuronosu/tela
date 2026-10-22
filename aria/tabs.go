// Portions of this file follow React Aria's useTabList, useTab, useTabPanel
// and react-stately's useTabListState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/tabs and
// packages/@react-stately/tabs), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// TabsOptions configures [UseTabs]. The tabs are keyed like the items of a
// list box: see [ListBoxOptions] for Keys, DisabledKeys, Selected,
// DefaultSelected, OnSelectionChange, Direction, AriaLabel and
// AriaLabelledBy.
type TabsOptions[K comparable] struct {
	Keys []K
	// TextValue is the text of a tab, for typeahead; the key as fmt.Sprint
	// prints it when nil. See [ListBoxOptions].
	TextValue    func(K) string
	DisabledKeys []K
	// Orientation is "horizontal" (the default; ArrowLeft and ArrowRight
	// move between tabs) or "vertical" (ArrowUp and ArrowDown).
	Orientation string
	// KeyboardActivation is "automatic" (the default: the tab that has the
	// focus is selected, so the arrows change the panel) or "manual" (the
	// arrows move the focus and Enter or Space selects).
	KeyboardActivation string
	// Selected is the selected tab of a controlled list, as a [Selection]
	// of one key; the zero Selection leaves it to the hook, which starts
	// at DefaultSelected. A tab list always has one selected tab: the
	// first enabled one when none is given or the given one is missing or
	// disabled.
	Selected          Selection[K]
	DefaultSelected   Selection[K]
	OnSelectionChange func(Selection[K])
	Direction         string
	AriaLabel         string
	AriaLabelledBy    tela.ElementRef
	// Ref is the tab list's ref, for a hook that is used together with
	// others on one element; the zero value makes the hook create its own.
	Ref tela.ElementRef
}

// TabsResult is what [UseTabs] returns.
type TabsResult[K comparable] struct {
	Collection[K]
	// Props go to the tab list's element: the "tablist" role,
	// aria-orientation, the ref and the key handler.
	Props tela.Prop
	// Selected is the selected tab (its [Selection.First] is the key).
	Selected Selection[K]
	// Ref is the ref attached to the tab list by Props.
	Ref tela.ElementRef
}

// TabProps returns the props of the tab with key k: the "tab" role,
// aria-selected, aria-controls (the panel, while selected), the ref, a tab
// stop on the selected tab (the arrows move between the others) and the
// handlers. It is [Collection.ItemProps].
func (r TabsResult[K]) TabProps(k K) tela.Prop { return r.ItemProps(k) }

// PanelProps returns the props of the panel of the tab with key k: the
// "tabpanel" role, named by the tab, the ref that the tab's aria-controls
// points to, and a tab stop (give the panel TabIndex(-1) after Props when
// it holds focusable content of its own). Render the panel of the selected
// tab only.
func (r TabsResult[K]) PanelProps(k K) tela.Prop {
	return core.MergeProps(h.Ref(r.c.panelRef(k)), h.Role("tabpanel"),
		h.AriaLabelledBy(r.c.refFor(k)), h.TabIndex(0))
}

// UseTabs holds the state of a tab list and returns the props of the list,
// its tabs and its panels (React Aria's useTabList, useTab and useTabPanel
// with useTabListState).
//
//	tabs := aria.UseTabs(aria.TabsOptions[string]{Keys: names, AriaLabel: "Settings"})
//	sel, _ := tabs.Selected.First()
//	return Div(
//	    Div(tabs.Props, Class("flex gap-1 border-b"),
//	        h.Range(len(names), func(i int) tela.Node {
//	            k := names[i]
//	            return Div(tabs.TabProps(k), Key(k),
//	                Class("px-3 py-2 aria-selected:border-b-2 aria-selected:border-primary focus-visible:ring-2"),
//	                Text(k))
//	        })),
//	    Div(tabs.PanelProps(sel), Class("p-4"), panel(sel)))
//
// The arrow keys move the focus between tabs and go around the ends; Home
// and End jump; with the default automatic activation the focused tab is
// selected, with KeyboardActivation "manual" Enter or Space selects it.
// Disabled tabs are skipped. Only the selected tab is a tab stop: Tab goes
// from it to its panel.
func UseTabs[K comparable](o TabsOptions[K]) TabsResult[K] {
	const hook = "UseTabs: TabsOptions."
	orient := checkChoice(hook, "Orientation", o.Orientation, "horizontal", "horizontal", "vertical")
	act := checkChoice(hook, "KeyboardActivation", o.KeyboardActivation, "automatic", "automatic", "manual")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	c := useCollection(collConfig[K]{
		kind: kindTabs, role: "tablist",
		keys: o.Keys, textValue: o.TextValue, disabled: o.DisabledKeys,
		mode: "single", behavior: "replace", disallowEmpty: true,
		selected: o.Selected, defaultSelected: o.DefaultSelected, onSelectionChange: o.OnSelectionChange,
		wrap: true, orientation: orient, rtl: dir == "rtl", manual: act == "manual",
		label: o.AriaLabel, labelledBy: o.AriaLabelledBy, ref: o.Ref,
	})
	return TabsResult[K]{Collection: Collection[K]{c}, Props: c.containerProps(), Selected: c.sel, Ref: c.ref}
}
