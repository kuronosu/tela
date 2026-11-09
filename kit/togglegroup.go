// Portions of this file follow shadcn/ui, toggle-group.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// ToggleGroupStyle is the recipe of [ToggleGroup] (shadcn/ui's ToggleGroup
// and ToggleGroupItem). The look of an item comes from [ToggleStyle]: its
// Root, the group's Variant and Size, and On.
type ToggleGroupStyle struct {
	// Root is the row that holds the items (shadcn's group/toggle-group class is
	// left out: Tela has no named groups, and nothing selects on it).
	Root string
	// Item is added to the toggle classes of every item.
	Item string
	// ItemWithoutSpacing is added to an item of a group with no space
	// between its items (shadcn's data-[spacing=0]:): square corners except
	// the ends, no shadow.
	ItemWithoutSpacing string
	// OutlineItemWithoutSpacing is added to an Outline item of a group with
	// no space between its items: one border between neighbors.
	OutlineItemWithoutSpacing string
	// ItemWithIndicator is added to every item of a group with an
	// Indicator: it places the indicator (relative) and keeps an indicator
	// with a negative z-index above the item's background and below its
	// content (isolate).
	ItemWithIndicator string
}

func init() {
	defaultStyle.ToggleGroup = ToggleGroupStyle{
		Root:                      "flex w-fit items-center rounded-md",
		Item:                      "w-auto min-w-0 shrink-0 px-3 focus:z-10 focus-visible:z-10",
		ItemWithoutSpacing:        "rounded-none shadow-none first:rounded-s-md last:rounded-e-md",
		OutlineItemWithoutSpacing: "border-s-0 first:border-s",
		ItemWithIndicator:         "relative isolate",
	}
}

// ToggleGroupItem is one item of a [ToggleGroup]: a toggle button with an
// icon, a label, or both.
type ToggleGroupItem[K comparable] struct {
	// Value is the item's key, what the group's selection holds.
	Value K
	// Label is the item's text.
	Label string
	// Icon is drawn before the label; give an AriaLabel to an item that has
	// only an icon.
	Icon tela.Icon
	// Children replaces the Label with any content.
	Children tela.Component
	// AriaLabel names an item whose content is not text.
	AriaLabel string
	// Disabled makes the item inert and dims it.
	Disabled bool
}

// ToggleGroup is a row of toggle buttons that share a choice, shadcn/ui's
// ToggleGroup, on aria.UseToggleGroup: the segmented control, and a
// formatting toolbar. With one choice (the default) the group is a radio
// group of toggle buttons; with Multiple any number of items are on and
// the group is a toolbar. The group is one tab stop: the arrows move the
// focus between the items, Home and End jump, and Enter or Space turns an
// item on or off.
//
// A single-choice group follows the kit's value rule with Value (a pointer
// to a key), DefaultValue and OnValueChange; pressing the item that is on
// turns it off unless DisallowEmptySelection, which a segmented control
// sets:
//
//	view, setView := tela.UseState("list")
//	kit.ToggleGroup[string]{
//	    Value: &view, OnValueChange: setView.Set, DisallowEmptySelection: true, AriaLabel: "View",
//	    Items: []kit.ToggleGroupItem[string]{
//	        {Value: "list", Label: "List"},
//	        {Value: "grid", Label: "Grid"},
//	    },
//	}
//
// A group with Multiple holds an [aria.Selection] instead, with Selected,
// DefaultSelected and OnSelectionChange:
//
//	kit.ToggleGroup[string]{Multiple: true, Variant: kit.Outline, DefaultSelected: aria.SelectionOf("bold"),
//	    Items: []kit.ToggleGroupItem[string]{{Value: "bold", Icon: lucide.Bold, AriaLabel: "Bold"}, ...}}
//
// Selected, DefaultSelected and OnSelectionChange also work on a
// single-choice group, for a controlled group that may have no item on.
//
// ValueFn and SelectedFn are the deferred forms of Value and Selected
// (ADR-250): read when the group builds, not when the component that holds
// the choice does, so a choice kept in a tela.Value rebuilds the group
// alone when it changes. With an Indicator, a pill that an h.LayoutID
// slides from item to item, the items need no state of the caller's to
// show the choice:
//
//	view := tela.UseValue("list") // read by the content, not by this component
//	kit.ToggleGroup[string]{
//	    ValueFn: view.Get, OnValueChange: view.Set, DisallowEmptySelection: true, AriaLabel: "View",
//	    Indicator: h.Div(h.Class("absolute inset-0 -z-10 rounded-md bg-background shadow-sm"), h.LayoutID("view-pill")),
//	    Items: []kit.ToggleGroupItem[string]{{Value: "list", Label: "List"}, {Value: "grid", Label: "Grid"}},
//	}
type ToggleGroup[K comparable] struct {
	// Items are the buttons, in order.
	Items []ToggleGroupItem[K]
	// Variant (Default or Outline) and Size (Default, Sm or Lg) are the
	// toggle looks of every item.
	Variant Variant
	Size    Size
	// Spacing is the space between items, in steps of 4 px. 0 joins the
	// items into one segmented bar (shadcn's default).
	Spacing int
	// Multiple lets any number of items be on.
	Multiple bool
	// Value points to the key that is on in a controlled single-choice
	// group; nil leaves it to the group, which starts at DefaultValue (nil:
	// none on). OnValueChange is called with the key that has just been
	// turned on.
	Value         *K
	DefaultValue  *K
	OnValueChange func(K)
	// ValueFn, the deferred form of Value, returns the key that is on in a
	// controlled single-choice group (a key that is not an item's turns
	// none on). The group calls it when it builds, so a tela.Value that it
	// reads makes a change rebuild the group and not the component that
	// gives it: pass the Value's Get, and its Set as OnValueChange.
	ValueFn func() K
	// Selected is the selection of a controlled group, DefaultSelected the
	// starting one of an uncontrolled group and OnSelectionChange is called
	// with the next selection. The zero aria.Selection leaves the selection
	// to the group.
	Selected          aria.Selection[K]
	DefaultSelected   aria.Selection[K]
	OnSelectionChange func(aria.Selection[K])
	// SelectedFn, the deferred form of Selected, returns the selection of a
	// controlled group when the group builds (see ValueFn); the zero
	// aria.Selection is the empty one here.
	SelectedFn func() aria.Selection[K]
	// Indicator is drawn inside every item that is on, as its first child:
	// a pill or an underline that marks the choice, such as
	// h.Div(h.Class("absolute inset-0 -z-10 rounded-md bg-background"),
	// h.LayoutID("pill")). The items get the recipe's ItemWithIndicator
	// (relative isolate), so an absolute indicator covers its item and a
	// negative z-index keeps it behind the label. With an h.LayoutID it
	// slides to the item turned on (one layout, then frames that only
	// composite); give a LayoutID only to a single-choice group, since with
	// Multiple every item that is on has one.
	Indicator tela.Component
	// DisallowEmptySelection keeps one item on: pressing the only item that
	// is on does nothing.
	DisallowEmptySelection bool
	// Orientation is Horizontal (the default) or Vertical, which the arrow
	// keys follow; the items are laid out in a row either way (use Class
	// "flex-col" for a column).
	Orientation Orientation
	// Direction is "ltr" or "rtl"; empty is the direction at the component's
	// position (tela.UseDirection: the nearest h.Dir above it, left to right
	// without one). Under "rtl" ArrowLeft goes to the next item and ArrowRight to the previous one; a Direction given here also
	// sets the direction of the layout.
	Direction string
	// Disabled disables every item.
	Disabled bool
	// AriaLabel names the group; without it a [FieldSet]'s legend does.
	AriaLabel string
	// Class is added after the recipe's classes; ItemClass is added to every
	// item.
	Class     string
	ItemClass string
	// Props are more props for the group element.
	Props tela.Child
}

// Build returns the group element.
func (g ToggleGroup[K]) Build() tela.Node {
	s := UseStyle()
	r, t := &s.ToggleGroup, &s.Toggle
	o := aria.ToggleGroupOptions[K]{
		Disabled: g.Disabled, DisallowEmptySelection: g.DisallowEmptySelection,
		Selected: g.Selected, DefaultSelected: g.DefaultSelected, OnSelectionChange: g.OnSelectionChange,
		Orientation: string(orientationOr(g.Orientation, Horizontal)), ShouldFocusWrap: true, AriaLabel: g.AriaLabel, AriaLabelledBy: useGroupLabel(g.AriaLabel),
		Keys: make([]K, len(g.Items)),
	}
	if g.Multiple {
		o.SelectionMode = "multiple"
	}
	for i, it := range g.Items {
		o.Keys[i] = it.Value
		if it.Disabled {
			o.DisabledKeys = append(o.DisabledKeys, it.Value)
		}
	}
	if g.Value != nil && g.Selected.IsSet() || g.DefaultValue != nil && g.DefaultSelected.IsSet() {
		panic("kit: ToggleGroup: give either Value and DefaultValue or Selected and DefaultSelected, not both")
	}
	if n := countTrue(g.Value != nil, g.ValueFn != nil, g.Selected.IsSet(), g.SelectedFn != nil); n > 1 {
		panic("kit: ToggleGroup: give one of Value, ValueFn, Selected and SelectedFn, the controlled choice")
	}
	switch {
	case g.ValueFn != nil:
		// Read here, in the group's build: a Value it reads rebuilds the
		// group (ADR-250).
		o.Selected = aria.SelectionOf(g.ValueFn())
	case g.SelectedFn != nil:
		if o.Selected = g.SelectedFn(); !o.Selected.IsSet() {
			o.Selected = aria.SelectionOf[K]()
		}
	case g.Value != nil:
		o.Selected = aria.SelectionOf(*g.Value)
	case g.DefaultValue != nil:
		o.DefaultSelected = aria.SelectionOf(*g.DefaultValue)
	}
	if g.OnValueChange != nil || g.OnSelectionChange != nil {
		onValue, onSel := g.OnValueChange, g.OnSelectionChange
		o.OnSelectionChange = func(sel aria.Selection[K]) {
			if onSel != nil {
				onSel(sel)
			}
			if onValue != nil {
				if k, ok := sel.First(); ok {
					onValue(k)
				}
			}
		}
	}
	dir, rootDir := useDirection("ToggleGroup", g.Direction)
	o.Direction = dir
	grp := aria.UseToggleGroup(o)

	variant, size := pick("Toggle", "Variant", t.Variants, g.Variant), pick("Toggle", "Size", t.Sizes, g.Size)
	joined := g.Spacing <= 0
	itemClass := joinClass(t.Root, variant, size, t.On, r.Item,
		when(joined, joinClass(r.ItemWithoutSpacing, when(g.Variant == Outline, r.OutlineItemWithoutSpacing))),
		when(g.Indicator != nil, r.ItemWithIndicator), g.ItemClass)
	kids := []tela.Child{cls(r.Root, when(!joined, fmt.Sprintf("gap-%d", g.Spacing)), g.Class), rootDir, grp.Props, g.Props}
	for _, it := range g.Items {
		ik := []tela.Child{h.Key(it.Value), cls(itemClass), grp.ItemProps(it.Value)}
		if it.AriaLabel != "" {
			ik = append(ik, h.AriaLabel(it.AriaLabel))
		}
		if g.Indicator != nil {
			// An empty child while the item is off keeps the positions
			// of its content, so turning it on remounts nothing.
			ik = append(ik, h.If(grp.IsSelected(it.Value), g.Indicator))
		}
		ik = append(ik, icon(it.Icon, t.Icon, t.IconVariants[g.Variant]))
		if it.Children != nil {
			ik = append(ik, it.Children)
		} else if it.Label != "" {
			ik = append(ik, h.Text(it.Label))
		}
		kids = append(kids, h.Button(ik...))
	}
	return h.Div(kids...)
}

// countTrue returns how many of bs are true.
func countTrue(bs ...bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}
