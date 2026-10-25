package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// SwitchStyle is the recipe of [Switch] (shadcn/ui's Switch).
type SwitchStyle struct {
	// Root is the track: shape, the off and on colors, focus ring and
	// disabled. It has the classes "peer" and "group", for the thumb and
	// for siblings that use the peer-* variants.
	Root string
	// Sizes are the track's dimensions: the default and Sm.
	Sizes map[Size]string
	// Thumb is the knob: its colors and its move from the off position to
	// the on one (shadcn's data-[state=checked]:translate-x-[calc(100%-2px)]).
	Thumb string
	// ThumbSizes are the knob's size per switch size.
	ThumbSizes map[Size]string
}

func init() {
	defaultStyle.Switch = SwitchStyle{
		Root: "peer group inline-flex shrink-0 items-center rounded-full border border-transparent bg-input shadow-xs transition-all outline-none " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 " +
			"dark:bg-input/80 checked:bg-primary",
		Sizes: map[Size]string{
			"": "h-[1.15rem] w-8",
			Sm: "h-3.5 w-6",
		},
		Thumb: "pointer-events-none block rounded-full bg-background ring-0 transition-transform translate-x-0 " +
			"group-checked:translate-x-[calc(100%-2px)] dark:bg-foreground group-checked:dark:bg-primary-foreground",
		ThumbSizes: map[Size]string{
			"": "size-4",
			Sm: "size-3",
		},
	}
}

// Switch is an on/off control that slides, shadcn/ui's Switch, on
// aria.UseSwitch: a click or Space toggles it and it has the switch role
// and aria-checked. Its state follows the kit's rule: Checked and
// OnCheckedChange to keep it yourself, DefaultChecked to let the switch
// keep it.
//
//	wifi, setWifi := tela.UseState(true)
//	kit.Switch{Checked: &wifi, OnCheckedChange: setWifi.Set, AriaLabel: "Wi-Fi"}
//
//	kit.Field{Orientation: kit.Horizontal, Label: "Airplane mode", Children: kit.Switch{Size: kit.Sm}}
//
// The knob moves with a transition. A Size the active [Style] does not
// define panics in dev mode with the closest one.
type Switch struct {
	// Size is Default or Sm.
	Size Size
	// Checked points to the state of a controlled switch; nil leaves it to
	// the switch, which starts at DefaultChecked.
	Checked *bool
	// DefaultChecked is the initial state of an uncontrolled switch.
	DefaultChecked bool
	// OnCheckedChange is called with the next state when the user toggles
	// the switch. For a controlled switch it is how the change is asked
	// for.
	OnCheckedChange func(bool)
	// Disabled makes the switch inert and dims it.
	Disabled bool
	// Required marks the switch as one the user must turn on
	// (aria-required).
	Required bool
	// Invalid marks the value as wrong (aria-invalid). A [Field] with an
	// Error sets it on its control.
	Invalid bool
	// ID is the switch's id, for a [Label]'s For. Inside a [Field] it
	// defaults to the field's.
	ID string
	// AriaLabel names a switch that has no label of its own.
	AriaLabel string
	// Class is added after the recipe's classes; ThumbClass does the same
	// for the knob.
	Class      string
	ThumbClass string
	// Props are more props for the switch element.
	Props tela.Child
}

// Build returns the switch element.
func (s Switch) Build() tela.Node {
	r := &UseStyle().Switch
	id, _, field := useChoiceField(s.ID, s.Invalid)
	sw := aria.UseSwitch(aria.ToggleOptions{Selected: s.Checked, DefaultSelected: s.DefaultChecked,
		OnChange: s.OnCheckedChange, Disabled: s.Disabled, ID: id})
	kids := []tela.Child{h.Class(r.Root, pick("Switch", "Size", r.Sizes, s.Size), s.Class), sw.Props, s.Props}
	kids = append(kids, field...)
	if s.Required {
		kids = append(kids, h.AriaRequired(true))
	}
	if s.AriaLabel != "" {
		kids = append(kids, h.AriaLabel(s.AriaLabel))
	}
	kids = append(kids, h.Span(h.Class(r.Thumb, pick("Switch", "Size", r.ThumbSizes, s.Size), s.ThumbClass)))
	return h.Button(kids...)
}
