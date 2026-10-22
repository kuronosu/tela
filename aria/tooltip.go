// Portions of this file follow React Aria's useTooltipTrigger and
// useTooltip (https://github.com/adobe/react-spectrum,
// packages/@react-aria/tooltip) and react-stately's
// useTooltipTriggerState, Copyright 2020 Adobe, licensed under the Apache
// License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// The tooltip delays of React Aria: a tooltip opens 1.5 s after the
// pointer enters its trigger, and closes 0.5 s after it leaves.
const (
	// DefaultTooltipDelay is TooltipTriggerOptions.Delay when it is zero.
	DefaultTooltipDelay = 1500 * time.Millisecond
	// DefaultTooltipCloseDelay is TooltipTriggerOptions.CloseDelay when it
	// is zero; it is also how long after the last tooltip closes the next
	// one still opens at once (the warm-up).
	DefaultTooltipCloseDelay = 500 * time.Millisecond
	// NoDelay as a delay of [TooltipTriggerOptions] opens or closes the
	// tooltip at once (the zero value stands for the default delay).
	NoDelay time.Duration = -1
)

// TooltipTriggerOptions configures [UseTooltipTrigger].
type TooltipTriggerOptions struct {
	// Delay is how long the pointer must stay on the trigger before the
	// tooltip opens: [DefaultTooltipDelay] when zero, none with [NoDelay].
	// A tooltip opened by the keyboard focus has no delay, and neither
	// does one that opens while another tooltip is open or closed less than
	// CloseDelay ago (React Aria's warm-up and cool-down).
	Delay time.Duration
	// CloseDelay is how long the tooltip stays after the pointer leaves:
	// [DefaultTooltipCloseDelay] when zero, none with [NoDelay]. Moving to
	// the tooltip itself keeps it open.
	CloseDelay time.Duration
	// Disabled turns the tooltip off: it does not open.
	Disabled bool
	// Trigger is what opens the tooltip: "" for both the pointer hovering
	// and the keyboard focus (focus that a pointer press did not give), or
	// "focus" for the keyboard focus alone. Another value panics in dev
	// mode.
	Trigger string
	// Open points to the open state of a controlled tooltip; nil leaves the
	// state to the hook, starting at DefaultOpen. OnOpenChange is called
	// with the next state: for a controlled tooltip it is how the change is
	// asked for, for an uncontrolled one a notification.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// Ref is the trigger's ref, for a hook used together with others on
	// the same element; the zero value makes the hook create its own.
	Ref tela.ElementRef
	// TooltipRef is the tooltip element's ref, which the trigger's
	// aria-describedby points to; the zero value makes the hook create its
	// own.
	TooltipRef tela.ElementRef
}

// TooltipTriggerResult is what [UseTooltipTrigger] returns.
type TooltipTriggerResult struct {
	// TriggerProps go to the trigger: the hover, focus, key and pointer
	// handlers that open and close the tooltip, the ref, and, while the
	// tooltip is open, aria-describedby. The trigger keeps its own role and
	// tab stop: tooltips are for elements that are already focusable.
	TriggerProps tela.Prop
	// TooltipProps go to the tooltip's element, rendered while Open (in a
	// Portal, placed by [UseFloating] or [UsePopover]-like options): its
	// "tooltip" role, the ref, hover that keeps it open, and Escape, which
	// closes it before anything below it.
	TooltipProps tela.Prop
	// Open is whether the tooltip is open.
	Open bool
	// Ref is the trigger's ref and TooltipRef the tooltip's.
	Ref, TooltipRef tela.ElementRef
}

// tooltipGroup is the state that the tooltips of one window share (React
// Aria's module-level tooltips map and warm-up flags): at most one
// tooltip is open, and the next one opens at once while the group is warm.
type tooltipGroup struct {
	nextID int
	hiders map[int]func(immediate bool)
	// warmed is set while a tooltip is open and until the cool-down that
	// follows its closing ends, at coolAt (0: no cool-down running).
	warmed bool
	coolAt time.Duration
}

var tooltipGroupKey byte

func (g *tooltipGroup) isWarm(now time.Duration) bool {
	return g.warmed && (g.coolAt == 0 || now < g.coolAt)
}

// closeOthers closes the tooltips other than id at once.
func (g *tooltipGroup) closeOthers(id int) {
	for other, hide := range g.hiders {
		if other != id {
			delete(g.hiders, other)
			hide(true)
		}
	}
}

// tooltipInst is the state of one UseTooltipTrigger call, kept across
// builds: React Aria's useTooltipTriggerState.
type tooltipInst struct {
	id      int
	g       *tooltipGroup
	st      *boolState
	ref     tela.ElementRef
	openT   core.Timeout
	closeT  core.Timeout
	delay   time.Duration
	closeD  time.Duration
	hovered bool
	focused bool
}

func (t *tooltipInst) now() time.Duration { return core.Clock(t.ref) }

func (t *tooltipInst) show() {
	t.closeT.Stop()
	t.g.closeOthers(t.id)
	t.g.hiders[t.id] = t.hide
	t.g.warmed, t.g.coolAt = true, 0
	t.openT.Stop()
	t.st.request(true)
}

// hide closes the tooltip at once, or after the close delay.
func (t *tooltipInst) hide(immediate bool) {
	if immediate || t.closeD <= 0 {
		t.closeT.Stop()
		t.st.request(false)
	} else if !t.closeT.Pending() {
		t.closeT.Start(t.closeD, func() { t.st.request(false) })
	}
	t.openT.Stop()
	if now := t.now(); t.g.isWarm(now) {
		t.g.coolAt = now + max(DefaultTooltipCloseDelay, t.closeD)
	}
}

// warmup opens the tooltip after the delay, or at once while the group is
// warm.
func (t *tooltipInst) warmup() {
	t.g.closeOthers(t.id)
	t.g.hiders[t.id] = t.hide
	switch {
	case t.st.cur:
	case !t.openT.Pending() && !t.g.isWarm(t.now()):
		t.openT.Start(t.delay, t.show)
	default:
		t.show()
	}
}

// open opens the tooltip: after the delay unless immediate, the delay is
// none, or the tooltip is closing.
func (t *tooltipInst) open(immediate bool) {
	if !immediate && t.delay > 0 && !t.closeT.Pending() {
		t.warmup()
	} else {
		t.show()
	}
}

// delayOf maps an option to a duration: zero is the default, negative none.
func delayOf(d, def time.Duration) time.Duration {
	switch {
	case d == 0:
		return def
	case d < 0:
		return 0
	}
	return d
}

// UseTooltipTrigger returns the props of a tooltip and of its trigger
// (React Aria's useTooltipTrigger and useTooltip, with
// useTooltipTriggerState): it opens when the pointer rests on the trigger
// or the keyboard focuses it, closes when either leaves, when the trigger
// is pressed (a pointer, or Enter or Space) and with Escape; other keys leave it open.
//
//	tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{})
//	fl := aria.UseFloating(tip.Ref, aria.Placement("top"), aria.Offset(6), aria.Flip(), aria.Shift(8))
//	return Fragment(
//	    Button(tip.TriggerProps, Class("rounded-md border p-2"), Text("Save")),
//	    Portal(Presence(If(tip.Open,
//	        Div(tip.TooltipProps, fl.Props, Exit("opacity-0"),
//	            Class("rounded bg-foreground px-2 py-1 text-xs text-background transition starting:opacity-0"),
//	            Text("Saves the file (Ctrl+S)"))))))
//
// The tooltips of a window behave as a group: opening one closes the
// others, and while one is open, or one closed less than
// [DefaultTooltipCloseDelay] ago, the next one opens at once. The delays
// run on the frame clock, with no goroutine, so an idle window with a
// tooltip pending or open asks for no frames.
func UseTooltipTrigger(o TooltipTriggerOptions) TooltipTriggerResult {
	switch o.Trigger {
	case "", "focus":
	default:
		if core.DevNow() {
			panic("aria: TooltipTriggerOptions.Trigger " + quote(o.Trigger) + ": use \"\" (hover and keyboard focus) or \"focus\" (keyboard focus only)")
		}
	}
	open, st := useBoolState(o.Open, o.DefaultOpen, o.OnOpenChange)
	ownTrigger, ownTooltip := tela.UseElementRef(), tela.UseElementRef()
	ref, tipRef := o.Ref, o.TooltipRef
	if ref == (tela.ElementRef{}) {
		ref = ownTrigger
	}
	if tipRef == (tela.ElementRef{}) {
		tipRef = ownTooltip
	}
	g := core.TreeLocal(&tooltipGroupKey, func() *tooltipGroup { return &tooltipGroup{hiders: map[int]func(bool){}} })
	inst := tela.UseRef((*tooltipInst)(nil))
	if inst.Current == nil {
		g.nextID++
		inst.Current = &tooltipInst{id: g.nextID, g: g}
	}
	t := inst.Current
	t.st, t.ref = st, ref
	t.openT, t.closeT = core.UseTimeout(), core.UseTimeout()
	t.delay, t.closeD = delayOf(o.Delay, DefaultTooltipDelay), delayOf(o.CloseDelay, DefaultTooltipCloseDelay)
	tela.UseEffect(func() func() {
		return func() { delete(g.hiders, t.id) }
	}, t.id)
	// A tooltip that is turned off closes.
	tela.UseEffect(func() func() {
		if o.Disabled && t.st.cur {
			t.hovered, t.focused = false, false
			t.hide(true)
		}
		return nil
	}, o.Disabled)

	focusOnly := o.Trigger == "focus"
	handleShow := func() {
		if t.hovered || t.focused {
			t.open(t.focused)
		}
	}
	hover := UseHover(HoverOptions{
		Disabled: o.Disabled,
		OnHoverStart: func(HoverEvent) {
			if focusOnly {
				return
			}
			t.hovered = true
			handleShow()
		},
		OnHoverEnd: func(HoverEvent) {
			if focusOnly {
				return
			}
			// However the trigger is left, the tooltip closes.
			t.focused, t.hovered = false, false
			t.hide(false)
		},
	})
	tipHover := UseHover(HoverOptions{
		OnHoverStart: func(HoverEvent) { t.open(true) },
		OnHoverEnd:   func(HoverEvent) { t.hide(false) },
	})
	pressStart := func() {
		// However the trigger is pressed, the tooltip closes.
		t.focused, t.hovered = false, false
		t.hide(true)
	}
	trigger := []tela.Prop{
		h.Ref(ref), hover.Props,
		h.OnFocus(func() {
			if !o.Disabled && core.FocusVisible(ref) {
				t.focused = true
				handleShow()
			}
		}),
		h.OnBlur(func() { t.focused, t.hovered = false, false; t.hide(true) }),
		h.OnPointerDown(pressStart),
		// A key press closes it as a press does in React Aria (usePress
		// starts on Enter and Space): Shift, arrows and shortcuts leave it.
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyEnter || e.Key == " " {
				pressStart()
			}
		}),
	}
	if open {
		trigger = append(trigger, h.AriaDescribedBy(tipRef))
	}
	return TooltipTriggerResult{
		TriggerProps: core.MergeProps(trigger...),
		TooltipProps: core.MergeProps(
			h.Ref(tipRef), h.Role("tooltip"), tipHover.Props,
			core.Overlay(core.OverlayOptions{EscapeOnly: true, OnEscape: func(core.KeyboardEvent) { t.hide(true) }}),
		),
		Open: open, Ref: ref, TooltipRef: tipRef,
	}
}

func quote(s string) string { return "\"" + s + "\"" }
