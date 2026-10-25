package kit

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// The icons Select draws, from Lucide (ISC, see THIRD_PARTY.md); each is
// replaceable with the Icon and CheckIcon fields.
var (
	selectChevronDown = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`}
	selectCheck       = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`}
)

// SelectStyle is the recipe of [Select] (shadcn/ui's SelectTrigger,
// SelectValue, SelectContent, SelectLabel, SelectItem and SelectSeparator).
// The root element of a Select is its trigger.
type SelectStyle struct {
	// Root is the trigger: the button that shows the selected item.
	Root string
	// Sizes are the trigger's heights: the default (36 px) and Sm (32 px).
	Sizes map[Size]string
	// Placeholder is added to the trigger while nothing is selected
	// (shadcn's data-[placeholder]:).
	Placeholder string
	// Value is the element inside the trigger that holds the selected
	// item's icon and label, or the placeholder.
	Value string
	// Icon is the chevron at the end of the trigger.
	Icon string
	// ValueIcon is the selected item's icon inside the trigger.
	ValueIcon string
	// Content is the popover that holds the list. It has no relative: the
	// popover's props make it fixed to the window and place it.
	Content string
	// ContentSides are the classes the content adds per side of the
	// trigger it opens on ("top", "bottom", "left", "right"): the origin of
	// its zoom.
	ContentSides map[string]string
	// ContentEnter and ContentExit are the classes of its entrance (the
	// starting: style) and of its exit. Their side-specific slide is
	// ContentEnterSides.
	ContentEnter, ContentExit string
	ContentEnterSides         map[string]string
	// Viewport is the list that scrolls inside the content.
	Viewport string
	// Label is a group's heading.
	Label string
	// Item is an option: Selected is added to the selected one.
	Item string
	// ItemIcon is an item's icon, ItemText the element that holds it and
	// the label.
	ItemIcon, ItemText string
	// ItemIndicator is the box at the end of the item that holds the check
	// and CheckIcon is the check.
	ItemIndicator, ItemCheck string
	// Separator is the line drawn between groups.
	Separator string
}

func init() {
	defaultStyle.Select = SelectStyle{
		Root: "flex w-fit items-center justify-between gap-2 rounded-md border border-input bg-transparent px-3 py-2 text-sm whitespace-nowrap shadow-xs transition-[color,box-shadow] outline-none " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 " +
			"aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:bg-input/30 dark:hover:bg-input/50 dark:aria-invalid:ring-destructive/40",
		Sizes: map[Size]string{
			"": "h-9",
			Sm: "h-8",
		},
		Placeholder: "text-muted-foreground",
		Value:       "line-clamp-1 flex items-center gap-2",
		Icon:        "pointer-events-none size-4 shrink-0 text-muted-foreground opacity-50",
		ValueIcon:   "pointer-events-none size-4 shrink-0 text-muted-foreground",
		Content:     "z-50 flex min-w-32 flex-col overflow-hidden rounded-md border border-border bg-popover text-popover-foreground shadow-md transition duration-150 ease-out",
		ContentSides: map[string]string{
			"top":    "origin-bottom",
			"bottom": "origin-top",
			"left":   "origin-right",
			"right":  "origin-left",
		},
		ContentEnter: "starting:scale-95 starting:opacity-0",
		ContentExit:  "scale-95 opacity-0",
		ContentEnterSides: map[string]string{
			"top":    "starting:translate-y-2",
			"bottom": "starting:-translate-y-2",
			"left":   "starting:translate-x-2",
			"right":  "starting:-translate-x-2",
		},
		Viewport:      "min-h-0 flex-auto overflow-x-hidden overflow-y-auto p-1 outline-none",
		Label:         "px-2 py-1.5 text-xs text-muted-foreground",
		Item:          "relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pe-8 ps-2 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50",
		ItemIcon:      "pointer-events-none size-4 shrink-0 text-muted-foreground",
		ItemText:      "flex items-center gap-2",
		ItemIndicator: "absolute end-2 flex size-3.5 items-center justify-center",
		ItemCheck:     "size-4",
		Separator:     "pointer-events-none -mx-1 my-1 h-px bg-border",
	}
}

// SelectItem is one option of a [Select].
type SelectItem[K comparable] struct {
	// Value is the option's key: what Select.Value holds and OnValueChange
	// reports.
	Value K
	// Label is the option's text, shown in the list and, once selected, in
	// the trigger. It is also what typing on the closed select matches.
	Label string
	// Icon is drawn before the label, in the list and in the trigger.
	Icon tela.Icon
	// Disabled makes the option impossible to choose.
	Disabled bool
	// Group names the group the option belongs to. Consecutive options with
	// the same Group are drawn together under that label, and a line
	// separates a group from the options before it; options with no Group
	// have no label.
	Group string
}

// Select is a dropdown that picks one of a list of options, shadcn/ui's
// Select: a trigger that shows the chosen option and a popover list that
// opens below it. It is built on aria.UseSelect: a press, Enter, Space and
// the arrow keys open the list, the arrows and Home and End move through
// it, typing jumps to an option, Enter or a click chooses it, and Escape
// closes it. On the closed trigger, typing and the left and right arrows
// choose without opening.
//
//	kit.Select[string]{
//	    Placeholder: "Select a fruit",
//	    Items: []kit.SelectItem[string]{
//	        {Value: "apple", Label: "Apple"},
//	        {Value: "banana", Label: "Banana"},
//	        {Value: "grape", Label: "Grape", Disabled: true},
//	    },
//	    OnValueChange: func(v string) { fmt.Println("chose", v) },
//	}
//
// The chosen option follows the kit's value rule: Value (a pointer to your
// state) with OnValueChange controls it, DefaultValue starts an uncontrolled
// one. Value and DefaultValue choose the option whose Value equals them;
// when none does, which is how the zero K shows the placeholder, nothing is
// chosen. A select whose keys include the zero value of K (the int 0, say)
// has that option chosen by an unset DefaultValue: give such keys a
// sentinel option, or use strings.
//
// Inside a [Field] the trigger takes the field's id, so the label points to
// it and names it, the field's description or error describes it, and an
// error marks it invalid.
//
// The list is a deviation from shadcn in one place: it always opens below
// (or above, when there is no room) the trigger, like shadcn's
// position="popper", where shadcn's default aligns the chosen option over
// the trigger; and it has no scroll buttons, since the list scrolls.
type Select[K comparable] struct {
	// Items are the options, in order.
	Items []SelectItem[K]
	// Value points to the chosen option of a controlled Select; nil leaves
	// the choice to the component, which starts at DefaultValue.
	Value *K
	// DefaultValue is the option an uncontrolled Select starts with.
	DefaultValue K
	// OnValueChange is called with the key of the option the user chose. For
	// a controlled Select it is how the change is asked for.
	OnValueChange func(K)
	// Open points to the open state of a controlled list; nil leaves it to
	// the component, which starts at DefaultOpen. OnOpenChange is called with
	// the next state.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// Placeholder is shown while nothing is chosen.
	Placeholder string
	// Size is Default (36 px) or Sm (32 px).
	Size Size
	// Disabled makes the select inert and dims it.
	Disabled bool
	// Invalid marks the choice as wrong: assistive technologies announce it
	// and the recipe's aria-invalid: classes (a red border and ring) apply.
	// A [Field] with an Error sets it.
	Invalid bool
	// ID is the trigger's id, for a [Label]'s For. Inside a [Field] it
	// defaults to the field's.
	ID string
	// AriaLabel names the select when no label is tied to it; inside a Field
	// with a Label that label is used.
	AriaLabel string
	// Icon replaces the chevron at the end of the trigger, CheckIcon the
	// check that marks the chosen option.
	Icon, CheckIcon tela.Icon

	// Class is added after the recipe's Root classes (the trigger's); the
	// other Class fields do the same for their part: the popover, an item,
	// and a group's label.
	Class        string
	ContentClass string
	ItemClass    string
	LabelClass   string
	// Props are more props for the trigger element.
	Props tela.Child
}

// selectRowKey keys the rows of the list that are not options (a group, a
// separator), apart from the options' own keys.
type selectRowKey struct{ kind, name string }

// Build returns the trigger and, in a portal, the list while it is open.
func (s Select[K]) Build() tela.Node {
	st := &UseStyle().Select
	fc := tela.UseContext(fieldCtx)

	keys := make([]K, len(s.Items))
	labels := make(map[K]string, len(s.Items))
	var disabled []K
	for i, it := range s.Items {
		keys[i] = it.Value
		labels[it.Value] = it.Label
		if it.Disabled {
			disabled = append(disabled, it.Value)
		}
	}
	has := func(k K) bool { _, ok := labels[k]; return ok }

	ariaLabel := s.AriaLabel
	if ariaLabel == "" {
		ariaLabel = fc.label
	}
	o := aria.SelectOptions[K]{
		Keys: keys, DisabledKeys: disabled, TextValue: func(k K) string { return labels[k] },
		Open: s.Open, DefaultOpen: s.DefaultOpen, OnOpenChange: s.OnOpenChange,
		Placeholder: s.Placeholder, Disabled: s.Disabled, AriaLabel: ariaLabel,
		Placement: "bottom", Offset: 4,
		Floating: []aria.FloatingOption{aria.Size(func(a aria.Available) { a.MaxHeight(a.Height) }, aria.SizeOptions{Padding: 10})},
	}
	switch {
	case s.Value != nil && has(*s.Value):
		o.Selected = aria.SelectionOf(*s.Value)
	case s.Value != nil:
		o.Selected = aria.SelectionOf[K]()
	case has(s.DefaultValue):
		o.DefaultSelected = aria.SelectionOf(s.DefaultValue)
	}
	if on := s.OnValueChange; on != nil {
		o.OnSelectionChange = func(sel aria.Selection[K]) {
			if k, ok := sel.First(); ok {
				on(k)
			}
		}
	}
	sel := aria.UseSelect(o)
	trigger := tela.UseSize(sel.TriggerRef)

	current, hasCurrent := sel.Selected.First()
	hasCurrent = hasCurrent && has(current)
	var currentItem SelectItem[K]
	if hasCurrent {
		for _, it := range s.Items {
			if it.Value == current {
				currentItem = it
				break
			}
		}
	}

	// The trigger.
	ic := s.Icon
	if ic == (tela.Icon{}) {
		ic = selectChevronDown
	}
	check := s.CheckIcon
	if check == (tela.Icon{}) {
		check = selectCheck
	}
	kids := []tela.Child{
		sel.TriggerProps, h.Type("button"),
		cls(st.Root, pick("Select", "Size", st.Sizes, s.Size), when(!hasCurrent, st.Placeholder), s.Class),
		s.Props,
		h.Span(sel.ValueProps, cls(st.Value), icon(currentItem.Icon, st.ValueIcon), h.Text(sel.Value)),
		icon(ic, st.Icon),
	}
	id := s.ID
	if id == "" {
		id = fc.id
	}
	if id != "" {
		kids = append(kids, h.ID(id))
	}
	if fc.describedBy != (tela.ElementRef{}) {
		kids = append(kids, h.AriaDescribedBy(fc.describedBy))
	}
	if s.Invalid || fc.invalid {
		kids = append(kids, h.AriaInvalid(true))
	}

	// The list, while open.
	var list tela.Child
	if sel.Open {
		side, _, _ := strings.Cut(sel.Placement, "-")
		if side == "" {
			side = "bottom"
		}
		minWidth := max(trigger.Get().Width, 128)
		var rows []tela.Child
		var group []tela.Child
		groupName, inGroup := "", false
		flush := func() {
			if inGroup {
				sec := sel.ListBox.Section(groupName)
				head := []tela.Child{sec.Props, h.Key(selectRowKey{"group", groupName})}
				head = append(head, h.Div(sec.HeadingProps, cls(st.Label, s.LabelClass), h.Text(groupName)))
				rows = append(rows, h.Div(append(head, group...)...))
			}
			group, inGroup = nil, false
		}
		for i, it := range s.Items {
			if i > 0 && it.Group != s.Items[i-1].Group {
				flush()
				rows = append(rows, h.Div(cls(st.Separator), h.Key(selectRowKey{"separator", it.Group})))
			}
			text := h.Span(cls(st.ItemText), icon(it.Icon, st.ItemIcon), h.Text(it.Label))
			indicator := h.Span(cls(st.ItemIndicator), h.If(sel.ListBox.IsSelected(it.Value), icon(check, st.ItemCheck)))
			row := h.Div(sel.ListBox.ItemProps(it.Value), h.Key(it.Value), cls(st.Item, s.ItemClass), indicator, text)
			if it.Group != "" {
				groupName, inGroup = it.Group, true
				group = append(group, row)
			} else {
				rows = append(rows, row)
			}
		}
		flush()
		content := h.Div(sel.PopoverProps,
			cls(st.Content, st.ContentSides[side], st.ContentEnter, st.ContentEnterSides[side], s.ContentClass),
			h.Exit(st.ContentExit), h.Style(tela.MinWidth(minWidth)),
			h.Div(append([]tela.Child{sel.ListBox.Props, cls(st.Viewport)}, rows...)...))
		list = content
	}
	return h.Fragment(h.Button(kids...), h.Portal(h.Presence(list)))
}
