// Portions of this file follow React Aria's useColorField and
// react-stately's useColorFieldState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/color
// and packages/@react-stately/color), Copyright 2020 Adobe, licensed
// under the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// ColorFieldOptions configures [UseColorField]: a text field for the hex
// code of the shared color.
type ColorFieldOptions struct {
	// State is the color the field edits, from [UseColorState]. It is
	// required, and must come from the same component.
	State *ColorState
	// Alpha shows the opacity in the text ("#rrggbbaa"). Without it the
	// text is "#rrggbb" and a code of 3 or 6 digits keeps the opacity;
	// with it a code of 4 or 8 digits sets it too.
	Alpha bool
	// Disabled and ReadOnly stop the field from changing the color.
	Disabled, ReadOnly bool
	// AriaLabel names the field; else it is named by the element with
	// LabelProps, or by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the field, when it is not
	// the one with LabelProps.
	AriaLabelledBy tela.ElementRef
	// Ref is the ref to attach to the input, to focus it from outside;
	// the hook makes one when it is zero.
	Ref tela.ElementRef
}

// ColorFieldResult is what [UseColorField] returns.
type ColorFieldResult struct {
	// InputProps go to the Input: the text, the handlers and the name.
	InputProps tela.Prop
	// LabelProps go to the element that labels the field; a press on it
	// focuses the field.
	LabelProps tela.Prop
	// InputValue is the text of the input as of this build: what the user
	// is typing, or the color's code.
	InputValue string
	// Editing is whether the text is a draft that is not committed yet.
	Editing bool
	// Ref is the ref that InputProps attaches to the input.
	Ref tela.ElementRef
}

// UseColorField holds a hex text field for the shared color (React Aria's
// useColorField). The user types a code ("#6366f1", "6366f1" or "#fff")
// and it is committed when the field loses the focus or on Enter: a code
// that parses sets the color, anything else (including an empty text) is
// reverted to the color's code. While the field is not being edited it
// shows the color, so a drag elsewhere in the picker updates it. ArrowUp
// and ArrowDown add or subtract one from the color as a 24-bit number,
// PageUp and PageDown sixteen.
//
//	st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: tela.Hex("#6366f1")})
//	hex := aria.UseColorField(aria.ColorFieldOptions{State: st, AriaLabel: "Hex"})
//	return Input(hex.InputProps, Class("w-28 rounded-md border px-2 py-1 font-mono"))
func UseColorField(o ColorFieldOptions) ColorFieldResult {
	if o.State == nil {
		panic("aria: UseColorField: ColorFieldOptions.State is required: make it in the same component with aria.UseColorState(aria.ColorStateOptions{...})")
	}
	_, bump := tela.UseState(0)
	own := tela.UseElementRef()
	ref := own
	if o.Ref != (tela.ElementRef{}) {
		ref = o.Ref
	}
	label := tela.UseElementRef()
	f := tela.UseOnce(func() *colorFieldState { return &colorFieldState{} })
	f.bump, f.st, f.o = bump, o.State, o

	text := FormatHexColor(f.st.Color(), o.Alpha)
	if f.editing {
		text = f.draft
	}
	name := h.AriaLabelledBy(label)
	switch {
	case o.AriaLabel != "":
		name = h.AriaLabel(o.AriaLabel)
	case o.AriaLabelledBy != (tela.ElementRef{}):
		name = h.AriaLabelledBy(o.AriaLabelledBy)
	}
	ps := []tela.Prop{
		h.Ref(ref), h.Value(text), name,
		h.OnBeforeInput(f.onBeforeInput), h.OnChange(f.onChange), h.OnKeyDown(f.onKeyDown), h.OnBlur(f.commit),
	}
	if o.Disabled {
		ps = append(ps, h.Disabled(true))
	}
	if o.ReadOnly {
		ps = append(ps, h.ReadOnly(true))
	}
	return ColorFieldResult{
		InputProps: core.MergeProps(ps...),
		LabelProps: core.MergeProps(h.Ref(label), h.OnClick(func() {
			if !o.Disabled {
				ref.Focus()
			}
		})),
		InputValue: text,
		Editing:    f.editing,
		Ref:        ref,
	}
}

// colorFieldState is the draft of one field hook, on the UI thread.
type colorFieldState struct {
	bump    tela.Setter[int]
	st      *ColorState
	o       ColorFieldOptions
	draft   string
	editing bool
}

func (f *colorFieldState) invalidate() {
	f.bump.Update(func(n int) int { return n + 1 })
}

// acceptable reports whether text can become a hex code by typing more: an
// optional "#" and up to eight hex digits.
func acceptable(text string) bool {
	text = strings.TrimPrefix(text, "#")
	if len(text) > 8 {
		return false
	}
	for i := 0; i < len(text); i++ {
		if _, ok := hexDigit(text[i]); !ok {
			return false
		}
	}
	return true
}

// onBeforeInput keeps what cannot be part of a hex code out of the field:
// the edit is cancelled before it happens, so the caret and the selection
// stay where they are (React Aria's beforeinput check).
func (f *colorFieldState) onBeforeInput(e tela.InputEvent) {
	if e.IsComposing || !strings.HasPrefix(e.InputType, "insert") {
		return
	}
	for _, r := range e.Data {
		if _, ok := hexDigit(byte(r)); r > 0x7f || (!ok && r != '#') {
			e.PreventDefault()
			return
		}
	}
}

func (f *colorFieldState) onChange(e tela.ChangeEvent) {
	if f.o.Disabled || f.o.ReadOnly {
		return
	}
	if !e.IsComposing && !acceptable(strings.TrimSpace(e.Value)) {
		f.invalidate() // the field shows the draft again
		return
	}
	f.draft, f.editing = e.Value, true
	f.invalidate()
}

// commit parses the draft: a code sets the color, anything else is
// reverted.
func (f *colorFieldState) commit() {
	if !f.editing {
		return
	}
	f.editing = false
	defer f.invalidate()
	c, err := ParseHexColor(f.draft)
	if err != nil {
		return
	}
	f.setRGB(c, len(strings.TrimPrefix(strings.TrimSpace(f.draft), "#")))
}

// setRGB sets the color from a code of n digits: the code's opacity is
// taken only when the field has the alpha and the code has it.
func (f *colorFieldState) setRGB(c tela.Color, digits int) {
	if !(f.o.Alpha && (digits == 4 || digits == 8)) {
		c.A = f.st.latest.Color().A
	}
	f.st.Set(c)
}

func (f *colorFieldState) onKeyDown(e tela.KeyboardEvent) {
	if f.o.Disabled || f.o.ReadOnly {
		return
	}
	var by int
	switch e.Key {
	case tela.KeyEnter:
		f.commit()
		return
	case tela.KeyArrowUp:
		by = 1
	case tela.KeyArrowDown:
		by = -1
	case tela.KeyPageUp:
		by = 16
	case tela.KeyPageDown:
		by = -16
	default:
		return
	}
	e.PreventDefault()
	f.editing = false
	c := f.st.latest.Color()
	n := int(c.NRGBA8().R)<<16 | int(c.NRGBA8().G)<<8 | int(c.NRGBA8().B)
	n = min(max(n+by, 0), 0xFFFFFF)
	next := tela.Color{R: float32(n>>16&0xFF) / 255, G: float32(n>>8&0xFF) / 255, B: float32(n&0xFF) / 255, A: c.A}
	f.st.Set(next)
	f.invalidate()
}
