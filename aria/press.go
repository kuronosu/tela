// Portions of this file follow React Aria's usePress
// (https://github.com/adobe/react-spectrum, packages/@react-aria/interactions),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// PointerType is the device or the way a press, or a hover, came from.
type PointerType string

// The pointer types.
const (
	// PointerMouse is a mouse or a touchpad.
	PointerMouse PointerType = "mouse"
	// PointerTouch is a finger on a touchscreen.
	PointerTouch PointerType = "touch"
	// PointerPen is a stylus.
	PointerPen PointerType = "pen"
	// PointerKeyboard is a press made with Enter or Space on the focused
	// element.
	PointerKeyboard PointerType = "keyboard"
	// PointerVirtual is a click that no pointer or key made: an assistive
	// technology or an agent (the "click" action of the semantics tree),
	// or the click a Label sends to the control it labels.
	PointerVirtual PointerType = "virtual"
)

func pointerTypeOf(k tela.PointerKind) PointerType {
	switch k {
	case tela.PointerTouch:
		return PointerTouch
	case tela.PointerPen:
		return PointerPen
	}
	return PointerMouse
}

// PressEvent is passed to the callbacks of [PressOptions]. X and Y are the
// position of the event in the window, LocalX and LocalY relative to the
// border box of the element, in logical pixels; a press that no pointer
// made (keyboard, virtual) reports the center of the element.
//
// Pressing stops the propagation of the events that make it (the
// pointer's, the key's, the click), as React Aria does, so a pressable
// card inside a pressable row presses only the card. Call
// [PressEvent.ContinuePropagation] in a callback to let them bubble.
type PressEvent struct {
	// PointerType is where the press came from.
	PointerType PointerType
	// X and Y are window coordinates; LocalX and LocalY are relative to the
	// element's border box.
	X, Y, LocalX, LocalY float32
	// Modifier keys held during the event; Meta is the Windows, Command or
	// Super key.
	Shift, Ctrl, Alt, Meta bool

	cont *bool
}

// ContinuePropagation lets the event that made this press bubble to the
// ancestors' handlers, instead of stopping it. It applies to the whole
// step of the press: if any of its callbacks continues, the event does.
func (e PressEvent) ContinuePropagation() {
	if e.cont != nil {
		*e.cont = true
	}
}

// PressOptions configures [UsePress].
type PressOptions struct {
	// OnPressStart is called when a press begins: the pointer goes down on
	// the element, or Space goes down on it while it is focused. A press
	// that left the element (without [PressOptions.ShouldCancelOnPointerExit])
	// starts again if the pointer comes back while still held.
	OnPressStart func(PressEvent)
	// OnPressEnd is called when a press ends, whether it counts or not: the
	// release over the element, the pointer leaving it, the cancel of the
	// pointer or a lost focus.
	OnPressEnd func(PressEvent)
	// OnPressChange is called with true when a press starts and false when
	// it ends: the callback form of the Pressed field of the result.
	OnPressChange func(pressed bool)
	// OnPressUp is called when the pointer or the key is released over the
	// element, before OnPressEnd.
	OnPressUp func(PressEvent)
	// OnPress is called when the press completes: the release over the
	// element, Space released on it, Enter pressed on it, or a virtual
	// click. It does not run for a disabled element.
	OnPress func(PressEvent)
	// Disabled ignores every pointer, key and click on the element, and
	// ends a press in progress. It sets no prop: for a disabled look and
	// semantics use [UseButton] or h.Disabled.
	Disabled bool
	// ShouldCancelOnPointerExit ends the press for good when the pointer
	// leaves the element, instead of letting it start again when the
	// pointer comes back while pressed.
	ShouldCancelOnPointerExit bool
	// PreventFocusOnPress keeps the focus where it is when the pointer
	// goes down on the element (the default action of a press is to focus
	// the nearest focusable element).
	PreventFocusOnPress bool
	// Ref is the element's ref, for a hook that is used together with
	// others on one element (see the package documentation); the zero
	// value makes the hook create its own.
	Ref tela.ElementRef
}

// PressResult is what [UsePress] returns.
type PressResult struct {
	// Props go to the element: the pointer, key, click and blur handlers
	// and the ref, merged. Props given after them in the element's
	// arguments override what they set, but handlers of the same event all
	// run.
	Props tela.Prop
	// Pressed is true while the element is pressed (a pointer is down on
	// it, or Space is held): style it with a class, since `active:` follows
	// the pointer only.
	Pressed bool
	// Ref is the ref attached to the element by Props.
	Ref tela.ElementRef
}

// UsePress returns the props that make an element pressable, the same
// across mouse, touch, pen and keyboard (React Aria's usePress).
//
//	press := aria.UsePress(aria.PressOptions{
//	    OnPress: func(e aria.PressEvent) { save() },
//	    Disabled: busy,
//	})
//	return Div(press.Props, Role("button"), TabIndex(0),
//	    Class("rounded-md px-3 py-1.5"), If(press.Pressed, Class("scale-95")), Text("Save"))
//
// A press starts when the primary button, a touch contact or a pen tip
// goes down on the element. It ends when the pointer is released
// ([PressOptions.OnPress] runs if that was over the element), leaves the
// element (which also ends it for good with
// [PressOptions.ShouldCancelOnPointerExit]) or is cancelled: the system
// takes the touch, a touch becomes a scroll, or the element is
// unmounted. The element keeps receiving the pointer while it is pressed,
// wherever it goes.
//
// With the keyboard, Enter on the focused element presses it at once
// (OnPressStart, OnPressUp, OnPressEnd and OnPress on the key down, never
// on a repeat), and Space starts the press on the key down and completes
// it on the key up; a press by Space ends without OnPress when the focus
// leaves the element. The hook prevents the default action of those keys
// (the click of a Button, the scroll of Space). A click that no pointer
// made (an assistive technology, an agent, a Label) is a virtual press.
//
// The hook stops the propagation of the events that make a press
// (PressEvent.ContinuePropagation lets them bubble), prevents the default
// action of the click (a Button does not submit its Form), and does not run
// for a Disabled element.
//
// The element needs no role: UsePress adds none. For a button use
// [UseButton], which adds the role, the tab stop and the disabled state.
func UsePress(o PressOptions) PressResult {
	props, pressed, ref := usePressCore(pressConfig{PressOptions: o, enter: true})
	return PressResult{Props: props, Pressed: pressed, Ref: ref}
}

// pressConfig is PressOptions plus what the hooks built on usePressCore
// decide.
type pressConfig struct {
	PressOptions
	// enter makes Enter press the element (buttons; not checkboxes).
	enter bool
	// activate is the activation behavior that follows a completed press:
	// "submit" or "reset" the element's Form, "" nothing.
	activate string
}

// pressState is the press in progress, kept across builds.
type pressState struct {
	pressed    bool
	kind       PointerType
	pointerID  int
	overTarget bool
	// tracking: the pointer that went down on the element is still down,
	// whether or not the press is still going (it left the element and
	// cancelled). expectClick: the click that follows its release is a
	// press's own, not a virtual one.
	tracking    bool
	expectClick bool
	// enterUp: the key up of an Enter that pressed is swallowed too.
	enterUp bool
}

// presser runs the press of one build: the state of the hook slot, the
// options of that build and the setter of Pressed.
type presser struct {
	c          pressConfig
	st         *pressState
	ref        tela.ElementRef
	setPressed tela.Setter[bool]
}

// usePressCore is the press hook the others build on. It returns the
// merged props, Pressed and the ref it attaches.
func usePressCore(c pressConfig) (tela.Prop, bool, tela.ElementRef) {
	own := tela.UseElementRef()
	ref := c.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	pressed, setPressed := tela.UseState(false)
	st := &tela.UseRef(pressState{}).Current
	p := &presser{c: c, st: st, ref: ref, setPressed: setPressed}
	tela.UseEffect(func() func() {
		if c.Disabled {
			p.cancel(input{})
		}
		return nil
	}, c.Disabled)
	// A Button with a press hook is not the Form's default button unless the
	// hook is told to submit: Enter in a text field would press it.
	buttonType := "button"
	if c.activate != "" {
		buttonType = c.activate
	}
	props := core.MergeProps(h.Ref(ref), core.DefaultButtonType(buttonType), p.handlers())
	return props, pressed, ref
}

// handlers are the event handlers of the press. A hook that presses one
// element per key (the triggers of a menu bar) builds a presser for each
// instead of calling usePressCore.
func (p *presser) handlers() tela.Prop {
	return core.MergeProps(
		h.OnPointerDown(p.onPointerDown),
		h.OnPointerMove(p.onPointerMove),
		h.OnPointerUp(p.onPointerUp),
		h.OnPointerCancel(p.onPointerCancel),
		h.OnClick(p.onClick),
		h.OnKeyDown(p.onKeyDown),
		h.OnKeyUp(p.onKeyUp),
		h.OnBlur(p.onBlur),
	)
}

// input is what a step of the press knows about its cause.
type input struct {
	kind                   PointerType
	x, y                   float32
	shift, ctrl, alt, meta bool
	// at is false for a press that no pointer made: it reports the center
	// of the element.
	at bool
}

func (p *presser) event(in input, cont *bool) PressEvent {
	b := p.ref.Bounds()
	e := PressEvent{PointerType: in.kind, Shift: in.shift, Ctrl: in.ctrl, Alt: in.alt, Meta: in.meta, cont: cont}
	if in.at {
		e.X, e.Y = in.x, in.y
	} else {
		e.X, e.Y = b.X+b.Width/2, b.Y+b.Height/2
	}
	e.LocalX, e.LocalY = e.X-b.X, e.Y-b.Y
	return e
}

// centered returns the input of a press with no position; mods come from a
// key or click event when there is one.
func (p *presser) centered(kind PointerType, mods *[4]bool) input {
	in := input{kind: kind}
	if mods != nil {
		in.shift, in.ctrl, in.alt, in.meta = mods[0], mods[1], mods[2], mods[3]
	}
	return in
}

func pointerInput(e tela.PointerEvent) input {
	return input{kind: pointerTypeOf(e.Kind), x: e.X, y: e.Y, at: true,
		shift: e.Shift, ctrl: e.Ctrl, alt: e.Alt, meta: e.Meta}
}

func keyInput(e tela.KeyboardEvent) input {
	return input{kind: PointerKeyboard, shift: e.Shift, ctrl: e.Ctrl, alt: e.Alt, meta: e.Meta}
}

// fireStart runs the callbacks of a press that begins.
func (p *presser) fireStart(in input, cont *bool) {
	if f := p.c.OnPressStart; f != nil {
		f(p.event(in, cont))
	}
	if f := p.c.OnPressChange; f != nil {
		f(true)
	}
	p.setPressed.Set(true)
}

func (p *presser) fireUp(in input, cont *bool) {
	if f := p.c.OnPressUp; f != nil {
		f(p.event(in, cont))
	}
}

// fireEnd runs the callbacks of a press that ends; OnPress only when it
// completed (wasPressed).
func (p *presser) fireEnd(in input, wasPressed bool, cont *bool) {
	if f := p.c.OnPressEnd; f != nil {
		f(p.event(in, cont))
	}
	if f := p.c.OnPressChange; f != nil {
		f(false)
	}
	p.setPressed.Set(false)
	if wasPressed && !p.c.Disabled {
		if f := p.c.OnPress; f != nil {
			f(p.event(in, cont))
		}
		if p.c.activate != "" {
			core.ActivateButton(p.ref, p.c.activate)
		}
	}
}

// isOver reports whether a window point is over the element's bounds
// (React Aria's isOverTarget). A ref that is attached to nothing counts as
// over, so a misplaced ref degrades to a press that never leaves.
func (p *presser) isOver(x, y float32) bool {
	b := p.ref.Bounds()
	if b.Width == 0 && b.Height == 0 {
		return true
	}
	return b.Contains(x, y)
}

func (p *presser) onPointerDown(e tela.PointerEvent) {
	if e.Button != tela.MouseButtonLeft || p.c.Disabled {
		return
	}
	st := p.st
	if st.pressed {
		e.StopPropagation() // another pointer or a key is already pressing
		return
	}
	in := pointerInput(e)
	st.pressed, st.overTarget, st.tracking, st.expectClick = true, true, true, false
	st.kind, st.pointerID = in.kind, e.PointerID
	if p.c.PreventFocusOnPress {
		e.PreventDefault()
	}
	if e.Kind == tela.PointerMouse {
		// A mouse goes where it likes while pressed: keep its events.
		// Touch and pen are captured by the element they pressed.
		core.SetPointerCapture(p.ref, e.PointerID)
	}
	cont := false
	p.fireStart(in, &cont)
	if !cont {
		e.StopPropagation()
	}
}

func (p *presser) onPointerMove(e tela.PointerEvent) {
	st := p.st
	if !st.pressed || st.kind == PointerKeyboard || e.PointerID != st.pointerID {
		return
	}
	over := p.isOver(e.X, e.Y)
	in := pointerInput(e)
	switch {
	case over && !st.overTarget:
		st.overTarget = true
		cont := false
		p.fireStart(in, &cont)
		if !cont {
			e.StopPropagation()
		}
	case !over && st.overTarget:
		st.overTarget = false
		cont := false
		p.fireEnd(in, false, &cont)
		if p.c.ShouldCancelOnPointerExit {
			p.finish(e.PointerID)
		}
		if !cont {
			e.StopPropagation()
		}
	}
}

func (p *presser) onPointerUp(e tela.PointerEvent) {
	st := p.st
	if e.Button != tela.MouseButtonLeft || !st.tracking || e.PointerID != st.pointerID {
		return
	}
	// The release of the pointer that went down here: a click may follow,
	// even if the press ended before (the pointer left and cancelled).
	st.tracking, st.expectClick = false, true
	if !st.pressed || st.kind == PointerKeyboard {
		return
	}
	in := pointerInput(e)
	over := p.isOver(e.X, e.Y)
	wasOver := st.overTarget
	st.pressed, st.overTarget = false, false
	p.release(e.PointerID)
	cont := false
	switch {
	case over:
		p.fireUp(in, &cont)
		p.fireEnd(in, true, &cont)
	case wasOver:
		p.fireEnd(in, false, &cont)
	}
	if !cont {
		e.StopPropagation()
	}
}

func (p *presser) onPointerCancel(e tela.PointerEvent) {
	st := p.st
	if e.PointerID == st.pointerID {
		st.tracking = false // a cancelled pointer makes no click
	}
	if !st.pressed || st.kind == PointerKeyboard || e.PointerID != st.pointerID {
		return
	}
	p.cancel(pointerInput(e))
}

// release gives the pointer back (the capture of a mouse).
func (p *presser) release(pointerID int) {
	core.ReleasePointerCapture(p.ref, pointerID)
}

// finish ends the press without callbacks (their step already ran).
func (p *presser) finish(pointerID int) {
	p.st.pressed, p.st.overTarget = false, false
	p.release(pointerID)
}

// cancel ends a press in progress without completing it.
func (p *presser) cancel(in input) {
	st := p.st
	if !st.pressed {
		return
	}
	was := st.overTarget
	st.pressed, st.overTarget, st.tracking = false, false, false
	if st.kind != PointerKeyboard {
		p.release(st.pointerID)
	}
	if in.kind == "" {
		in.kind = st.kind
	}
	if was {
		cont := false
		p.fireEnd(in, false, &cont)
	} else {
		p.setPressed.Set(false)
	}
}

// onClick handles the click of the DOM: after a pointer press it is the
// click that press made (ignored), otherwise nothing pressed the element
// and it is a virtual press.
func (p *presser) onClick(e tela.MouseEvent) {
	if e.Button != tela.MouseButtonLeft {
		return
	}
	st := p.st
	// A press owns the activation: a Button does not submit its Form, a
	// Label does not forward the click.
	e.PreventDefault()
	if p.c.Disabled {
		e.StopPropagation()
		return
	}
	stop := true
	switch {
	case st.expectClick:
		st.expectClick = false
	case !st.pressed:
		in := p.centered(PointerVirtual, &[4]bool{e.Shift, e.Ctrl, e.Alt, e.Meta})
		cont := false
		p.fireStart(in, &cont)
		p.fireUp(in, &cont)
		p.fireEnd(in, true, &cont)
		stop = !cont
	}
	if stop {
		e.StopPropagation()
	}
}

func isSpace(e tela.KeyboardEvent) bool { return e.Key == " " }

func (p *presser) onKeyDown(e tela.KeyboardEvent) {
	// Only the element itself presses with the keyboard: a key typed in a
	// field inside it is the field's.
	if p.c.Disabled || !p.ref.Focused() {
		return
	}
	enter := p.c.enter && e.Key == tela.KeyEnter
	if !enter && e.Key == tela.KeyEnter {
		e.PreventDefault() // a checkbox does not activate with Enter, and neither does its Button
		return
	}
	if !enter && !isSpace(e) {
		return
	}
	e.PreventDefault()
	st := p.st
	if e.Repeat || st.pressed {
		e.StopPropagation()
		return
	}
	in := keyInput(e)
	cont := false
	st.pressed, st.overTarget, st.kind, st.expectClick = true, true, PointerKeyboard, false
	p.fireStart(in, &cont)
	if enter {
		// Enter activates on the key down, like a button.
		st.pressed, st.overTarget = false, false
		p.fireUp(in, &cont)
		p.fireEnd(in, true, &cont)
		st.enterUp = !cont
	}
	if !cont {
		e.StopPropagation()
	}
}

func (p *presser) onKeyUp(e tela.KeyboardEvent) {
	st := p.st
	if e.Key == tela.KeyEnter && st.enterUp {
		st.enterUp = false
		e.StopPropagation() // the release of the Enter that pressed
		return
	}
	if !st.pressed || st.kind != PointerKeyboard || !isSpace(e) {
		return
	}
	e.PreventDefault()
	in := keyInput(e)
	if p.c.Disabled {
		p.cancel(in)
		return
	}
	st.pressed, st.overTarget = false, false
	cont := false
	p.fireUp(in, &cont)
	p.fireEnd(in, true, &cont)
	if !cont {
		e.StopPropagation()
	}
}

// onBlur ends a press by Space when the focus leaves: the key up will not
// come to the element.
func (p *presser) onBlur(tela.FocusEvent) {
	if p.st.pressed && p.st.kind == PointerKeyboard {
		p.cancel(input{kind: PointerKeyboard})
	}
}
