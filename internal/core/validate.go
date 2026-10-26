package core

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/semantics"
)

// The dev-mode checks of props whose argument is a literal, as pure
// functions. The props call them and panic with the message in dev mode;
// `tela lint` calls them on the constant arguments it finds in source
// (ADR-173), so both report the same text. Each returns "" when the value
// is valid.

// IsHookName reports whether a function name is a hook's: Use or use
// followed by an upper-case letter, a digit or nothing (UseTheme,
// useDrag; not User or useful). Dev mode and `tela lint` use it to tell
// custom hooks from other functions.
func IsHookName(name string) bool { return isHookName(name) }

// CheckRole returns the message of Role(role) for an unknown ARIA role
// (the first unknown token of a list), with the closest valid role.
func CheckRole(role string) string {
	_, bad := semantics.ParseRole(role)
	if bad == "" {
		return ""
	}
	msg := fmt.Sprintf("tela: Role(%q): unknown ARIA role %q", role, bad)
	if s := semantics.SuggestRole(bad); s != "" {
		return msg + fmt.Sprintf("; did you mean %q?", s)
	}
	return msg + "; see the WAI-ARIA 1.2 roles (button, checkbox, dialog, heading, link, switch, tab...)"
}

// AriaTokenProps are the props whose value is one of a fixed set of
// tokens, checked by CheckAriaToken.
func AriaTokenProps() []string {
	return []string{"AriaCurrent", "AriaSort", "AriaHasPopup", "AriaOrientation", "AriaAutoComplete"}
}

// ariaTokenValues returns the tokens of a prop of AriaTokenProps.
func ariaTokenValues(prop string) []string {
	switch prop {
	case "AriaCurrent":
		return currentValues
	case "AriaSort":
		return sortValues
	case "AriaHasPopup":
		return hasPopupValues
	case "AriaOrientation":
		return orientationValues
	case "AriaAutoComplete":
		return autoCompleteValues
	}
	return nil
}

// CheckAriaToken returns the message of the prop (one of AriaTokenProps)
// for a value that is not one of its tokens, with the closest token and
// the list of them. Like the props, it accepts any ASCII case and
// surrounding white space, and AriaCurrent accepts an empty value.
func CheckAriaToken(prop, value string) string {
	valid := ariaTokenValues(prop)
	if valid == nil || (prop == "AriaCurrent" && strings.TrimSpace(value) == "") {
		return ""
	}
	if _, ok := lookupToken(value, valid); ok {
		return ""
	}
	return ariaTokenMessage(prop, value, valid)
}

// CheckDir returns the message of Dir(dir) for a value that is not ltr,
// rtl, auto or empty.
func CheckDir(dir string) string {
	if _, ok := parseDir(dir); ok {
		return ""
	}
	return dirMessage(dir)
}

// FieldTypeNames returns the values of Type for an element: "input" (the
// types of an Input), "button" (submit, button, reset) or, for any other
// string, the union of both.
func FieldTypeNames(element string) []string {
	switch element {
	case "input":
		return fieldTypeNames[:]
	case "button":
		return buttonTypeNames[:]
	}
	return append(append([]string(nil), fieldTypeNames[:]...), buttonTypeNames[:]...)
}

// CheckFieldType returns the message of Type(typ) for a type the element
// does not take. element is "input", "button" or "" when it is not known
// (then any type of either is accepted).
func CheckFieldType(element, typ string) string {
	names := FieldTypeNames(element)
	for _, n := range names {
		if typ == n {
			return ""
		}
	}
	on := ""
	if element != "" {
		on = "<" + element + ">"
	}
	return fieldTypeMessage(typ, on, names)
}

// CheckEase returns the message of Ease(css) for a string that is not a
// CSS easing function, with what to write instead.
func CheckEase(css string) string {
	if _, err := anim.ParseEasing(css); err != nil {
		return "tela: " + easeError(css, err)
	}
	return ""
}
