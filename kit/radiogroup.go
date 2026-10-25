// Portions of this file follow shadcn/ui, radio-group.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// RadioGroupStyle is the recipe of [RadioGroup] (shadcn/ui's RadioGroup and
// RadioGroupItem, with the label arrangement of its documentation and of
// [Field]).
type RadioGroupStyle struct {
	// Root is the group: the options in a grid.
	Root string
	// Orientations are the arrangement of the options: Vertical (a column)
	// and Horizontal (a row).
	Orientations map[Orientation]string
	// Option is the row of one option: the circle and its text.
	Option string
	// OptionWithDescription is added to an option that has a Description:
	// the circle sits at the top, beside the text.
	OptionWithDescription string
	// Item is the radio circle (shadcn's RadioGroupItem): size, border,
	// focus ring, disabled and invalid. It has the class "peer", so the
	// label dims with it, and its content (the dot) is centered, as a
	// browser centers a button's.
	Item string
	// ItemWithDescription is added to the circle of an option that has a
	// Description (shadcn's mt-px of a Field's radio).
	ItemWithDescription string
	// Indicator holds the dot and is only there while checked.
	Indicator string
	// Icon is the dot.
	Icon string
	// Label is added to the [Label] recipe's classes for an option's label.
	Label string
	// LabelWithDescription is added to Label when the option has a
	// Description (a [Field]'s label: snug leading).
	LabelWithDescription string
	// Content holds the label and the description of an option that has a
	// Description.
	Content string
	// Description is the help text under an option's label.
	Description string
}

func init() {
	defaultStyle.RadioGroup = RadioGroupStyle{
		Root: "grid gap-3",
		Orientations: map[Orientation]string{
			Vertical:   "",
			Horizontal: "grid-flow-col auto-cols-max gap-6",
		},
		Option:                "flex items-center gap-3",
		OptionWithDescription: "items-start",
		Item: "peer inline-flex aspect-square size-4 shrink-0 items-center justify-center rounded-full border border-input text-primary shadow-xs transition-[color,box-shadow] outline-none " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 " +
			"aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:bg-input/30 dark:aria-invalid:ring-destructive/40",
		ItemWithDescription:  "mt-px",
		Indicator:            "relative flex items-center justify-center",
		Icon:                 "absolute top-1/2 left-1/2 size-2 -translate-x-1/2 -translate-y-1/2 fill-primary",
		Label:                "",
		LabelWithDescription: "w-fit gap-2 leading-snug",
		Content:              "flex flex-1 flex-col gap-1.5 leading-snug peer-disabled:opacity-50",
		Description:          "text-sm leading-normal font-normal text-muted-foreground",
	}
}

// RadioItem is one option of a [RadioGroup].
type RadioItem[K comparable] struct {
	// Value is the option's key, what the group's value holds.
	Value K
	// Label is the option's text, which names the radio.
	Label string
	// Description is help text under the label.
	Description string
	// Disabled makes the option inert and dims it.
	Disabled bool
}

// RadioState is what a [RadioGroup]'s Render func is given about one
// option, to build a custom item that the group still drives.
type RadioState struct {
	// Checked is whether the option is the group's value.
	Checked bool
	// Disabled is whether the option, or the whole group, is disabled.
	Disabled bool
	// Invalid is whether the group is invalid.
	Invalid bool
	// ID is the id of the radio element, for a [Label]'s For.
	ID string
	// Props make an element the radio: its role, aria-checked, id, tab
	// stop and handlers (a click, Space and the arrow keys check it). Give
	// them to the one element the user presses, usually an h.Button, and
	// style it with the checked: and aria-checked: variants.
	Props tela.Prop
	// Indicator is the circle of the default look (with its dot while
	// Checked), not interactive: put it in your item to keep the familiar
	// mark, or leave it out.
	Indicator tela.Component
}

// RadioGroup is a set of options of which one is chosen, shadcn/ui's
// RadioGroup, on aria.UseRadioGroup: the arrow keys (all four) move the
// focus to the next option and check it, Space checks, and Tab enters the
// group at the checked option. The value follows the kit's rule for a
// choice among keys: Value (a pointer to the key), DefaultValue and
// OnValueChange; nothing is checked while there is no value.
//
//	plan, setPlan := tela.UseState("pro")
//	kit.RadioGroup[string]{
//	    AriaLabel: "Plan", Value: &plan, OnValueChange: setPlan.Set,
//	    Items: []kit.RadioItem[string]{
//	        {Value: "free", Label: "Free"},
//	        {Value: "pro", Label: "Pro", Description: "For small teams."},
//	    },
//	}
//
// To draw the options yourself, such as theme cards, give Render: it is
// called for each option with the state and the props of its radio, and the
// group keeps the behavior.
//
//	kit.RadioGroup[string]{AriaLabel: "Theme", DefaultValue: new("light"), Orientation: kit.Horizontal,
//	    Items: themes, Render: func(it kit.RadioItem[string], st kit.RadioState) tela.Node {
//	        return h.Button(st.Props, cls("rounded-lg border p-4 aria-checked:border-primary"),
//	            h.Text(it.Label))
//	    }}
//
// Inside a [Field] the group takes the field's description and invalid
// state; name it with AriaLabel.
type RadioGroup[K comparable] struct {
	// Items are the options, in order.
	Items []RadioItem[K]
	// Value points to the checked key of a controlled group; nil leaves it
	// to the group, which starts at DefaultValue (nil: nothing checked).
	Value        *K
	DefaultValue *K
	// OnValueChange is called with the next key when the user checks an
	// option. For a controlled group it is how the change is asked for.
	OnValueChange func(K)
	// Orientation is Vertical (the default) or Horizontal; it is the
	// group's aria-orientation and the arrangement of the options.
	Orientation Orientation
	// Direction is "ltr" or "rtl"; empty is the direction at the component's
	// position (tela.UseDirection: the nearest h.Dir above it, left to right
	// without one). Under "rtl" ArrowLeft goes to the next option and ArrowRight to the previous one; a Direction given here also
	// sets the direction of the layout.
	Direction string
	// Disabled disables every option.
	Disabled bool
	// ReadOnly keeps the options focusable but not changeable.
	ReadOnly bool
	// Required marks the group as one the user must answer.
	Required bool
	// Invalid marks the group wrong: the circles take the invalid look. A
	// [Field] with an Error sets it.
	Invalid bool
	// AriaLabel names the group.
	AriaLabel string
	// Render draws one option in place of the default (circle, label and
	// description); see [RadioState]. It runs while the group builds.
	Render func(item RadioItem[K], state RadioState) tela.Node
	// Class is added after the recipe's classes. OptionClass, ItemClass,
	// LabelClass and DescriptionClass do the same for their parts of the
	// default look.
	Class            string
	OptionClass      string
	ItemClass        string
	LabelClass       string
	DescriptionClass string
	// Props are more props for the group element.
	Props tela.Child
}

// radioIndicator is the circle of the default look, without behavior.
type radioIndicator struct {
	checked bool
	class   string
	icon    string
}

func (r radioIndicator) Build() tela.Node {
	s := &UseStyle().RadioGroup
	kids := []tela.Child{cls("inline-flex", s.Item, r.class), h.AriaHidden(true)}
	if r.checked {
		kids = append(kids, h.Span(cls(s.Indicator), h.Icon(iconCircle, cls(s.Icon, r.icon))))
	}
	return h.Span(kids...)
}

// Build returns the group element.
func (g RadioGroup[K]) Build() tela.Node {
	s := &UseStyle().RadioGroup
	o := orientationOr(g.Orientation, Vertical)
	id, invalid, field := useChoiceField("", g.Invalid)
	keys := make([]K, len(g.Items))
	var disabled []K
	for i, it := range g.Items {
		keys[i] = it.Value
		if it.Disabled {
			disabled = append(disabled, it.Value)
		}
	}
	dir, rootDir := useDirection("RadioGroup", g.Direction)
	grp := aria.UseRadioGroup(aria.RadioGroupOptions[K]{
		Keys: keys, DisabledKeys: disabled, Value: g.Value, DefaultValue: g.DefaultValue, OnValueChange: g.OnValueChange,
		Disabled: g.Disabled, ReadOnly: g.ReadOnly, Required: g.Required, Orientation: string(o), AriaLabel: g.AriaLabel, Direction: dir,
	})
	kids := []tela.Child{cls(s.Root, pick("RadioGroup", "Orientation", s.Orientations, o), g.Class), rootDir, grp.Props, g.Props}
	kids = append(kids, field...)
	if id != "" {
		kids = append(kids, h.ID(id))
	}
	for _, it := range g.Items {
		isDisabled := g.Disabled || it.Disabled
		if g.Render != nil {
			kids = append(kids, g.Render(it, RadioState{
				Checked: grp.IsSelected(it.Value), Disabled: isDisabled, Invalid: invalid,
				ID: grp.RadioID(it.Value), Props: grp.RadioProps(it.Value),
				Indicator: radioIndicator{checked: grp.IsSelected(it.Value), class: g.ItemClass},
			}))
			continue
		}
		hasDesc := it.Description != ""
		radio := []tela.Child{cls(s.Item, when(hasDesc, s.ItemWithDescription), g.ItemClass), grp.RadioProps(it.Value)}
		if invalid {
			radio = append(radio, h.AriaInvalid(true))
		}
		if grp.IsSelected(it.Value) {
			radio = append(radio, h.Span(cls(s.Indicator), h.Icon(iconCircle, cls(s.Icon))))
		}
		var text tela.Child
		label := Label{For: grp.RadioID(it.Value), Children: h.Text(it.Label), Class: joinClass(s.Label, when(hasDesc, s.LabelWithDescription), g.LabelClass)}
		if hasDesc {
			text = h.Div(cls(s.Content), label, h.P(cls(s.Description, g.DescriptionClass), h.Text(it.Description)))
		} else {
			text = label
		}
		kids = append(kids, h.Div(h.Key(it.Value), cls(s.Option, when(hasDesc, s.OptionWithDescription), g.OptionClass),
			h.Button(radio...), text))
	}
	return h.Div(kids...)
}
