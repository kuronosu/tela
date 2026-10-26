package core

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// The ARIA props of the widget patterns (ADR-096): popups, menus,
// listboxes, comboboxes and modal dialogs (React's aria-haspopup,
// aria-controls, aria-activedescendant, aria-modal, aria-multiselectable,
// aria-orientation and aria-autocomplete). They describe the widget in
// the semantics tree (semantics.Node.HasPopup and the fields after it),
// for the accessibility backends and package aria; Playwright 1.61's aria
// snapshot prints none of them, so the YAML does not change, except that
// aria-controls is a global ARIA attribute: Role("none") and
// Role("presentation") give way to the implicit role on an element with
// AriaControls, as in Playwright (ARIA's presentational role conflict
// resolution).

// ariaWidget holds the widget props of a primitive (ariaProps.w).
type ariaWidget struct {
	hasPopup     string // normalized (see the props); "" unset or false
	controls     *elemRef
	activeDesc   *elemRef
	modal        bool
	multi        bool
	orientation  string // "" unset or undefined
	autoComplete string // "" unset or none
}

func ariaWidgetProp(name string, f func(w *ariaWidget)) Prop {
	return ariaProp(name, func(a *ariaProps) {
		if a.w == nil {
			a.w = &ariaWidget{}
		}
		f(a.w)
	})
}

// ariaToken returns value lowercased when it is one of valid (ASCII
// case-insensitive, like HTML's enumerated attributes). In dev mode an
// unknown value panics with the closest valid one; otherwise ok is false
// and the prop is ignored.
func ariaToken(prop, value string, valid []string) (string, bool) {
	if v, ok := lookupToken(value, valid); ok {
		return v, true
	}
	if devNow() {
		panic(ariaTokenMessage(prop, value, valid))
	}
	return "", false
}

// lookupToken returns value lowercased and trimmed when it is one of valid.
func lookupToken(value string, valid []string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, ok := range valid {
		if v == ok {
			return v, true
		}
	}
	return "", false
}

// ariaTokenMessage is the dev-mode message of a value that is not one of
// valid (CheckAriaToken).
func ariaTokenMessage(prop, value string, valid []string) string {
	msg := fmt.Sprintf("tela: %s(%q): unknown value", prop, value)
	if best := closestName(strings.ToLower(strings.TrimSpace(value)), valid); best != "" {
		msg += fmt.Sprintf("; did you mean %q?", best)
	}
	return msg + "; it takes " + quoteList(valid)
}

func quoteList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(q, ", ")
}

var (
	hasPopupValues     = []string{"menu", "listbox", "tree", "grid", "dialog", "true", "false"}
	orientationValues  = []string{"horizontal", "vertical", "undefined"}
	autoCompleteValues = []string{"inline", "list", "both", "none"}
)

// AriaHasPopup sets the kind of popup an element opens (aria-haspopup):
// "menu", "listbox", "tree", "grid" or "dialog"; "true" is "menu" (ARIA)
// and "false" none. Unknown values panic in dev mode with the closest
// valid one and are ignored otherwise.
func AriaHasPopup(kind string) Prop {
	v, ok := ariaToken("AriaHasPopup", kind, hasPopupValues)
	switch v {
	case "true":
		v = "menu"
	case "false":
		v = ""
	}
	return ariaWidgetProp("AriaHasPopup", func(w *ariaWidget) {
		if ok {
			w.hasPopup = v
		}
	})
}

// AriaControls names the element whose content or presence this one
// controls (aria-controls): the element r is attached to.
func AriaControls(r ElementRef) Prop {
	if r.r == nil {
		panic("tela: AriaControls(zero ElementRef): pass an ElementRef from tela.UseElementRef attached with Ref to the controlled element")
	}
	return ariaWidgetProp("AriaControls", func(w *ariaWidget) { w.controls = r.r })
}

// AriaActiveDescendant names the active descendant of a composite widget
// that keeps the focus itself (aria-activedescendant): the option of a
// listbox or combobox that the arrow keys moved to, attached to r.
func AriaActiveDescendant(r ElementRef) Prop {
	if r.r == nil {
		panic("tela: AriaActiveDescendant(zero ElementRef): pass an ElementRef from tela.UseElementRef attached with Ref to the active option")
	}
	return ariaWidgetProp("AriaActiveDescendant", func(w *ariaWidget) { w.activeDesc = r.r })
}

// AriaModal marks a dialog as modal (aria-modal): the content outside it
// is not available while it is open. It only describes the dialog;
// package aria's modal overlays make the outside inert (overlay.go).
func AriaModal(modal bool) Prop {
	return ariaWidgetProp("AriaModal", func(w *ariaWidget) { w.modal = modal })
}

// AriaMultiSelectable marks a listbox, grid, tablist or tree that allows
// selecting several items (aria-multiselectable).
func AriaMultiSelectable(multi bool) Prop {
	return ariaWidgetProp("AriaMultiSelectable", func(w *ariaWidget) { w.multi = multi })
}

// AriaOrientation sets the orientation of a listbox, menu, tablist,
// toolbar, slider, scrollbar or separator (aria-orientation):
// "horizontal", "vertical" or "undefined". Unknown values panic in dev
// mode and are ignored otherwise.
func AriaOrientation(o string) Prop {
	v, ok := ariaToken("AriaOrientation", o, orientationValues)
	if v == "undefined" {
		v = ""
	}
	return ariaWidgetProp("AriaOrientation", func(w *ariaWidget) {
		if ok {
			w.orientation = v
		}
	})
}

// AriaAutoComplete describes the completion a combobox or textbox offers
// (aria-autocomplete): "inline", "list", "both" or "none". Unknown values
// panic in dev mode and are ignored otherwise.
func AriaAutoComplete(mode string) Prop {
	v, ok := ariaToken("AriaAutoComplete", mode, autoCompleteValues)
	if v == "none" {
		v = ""
	}
	return ariaWidgetProp("AriaAutoComplete", func(w *ariaWidget) {
		if ok {
			w.autoComplete = v
		}
	})
}

// semWidget copies the widget props of a onto node n.
func (t *Tree) semWidget(a *ariaProps, n *semantics.Node) {
	w := a.w
	if w == nil {
		return
	}
	n.HasPopup, n.Modal, n.MultiSelectable = w.hasPopup, w.modal, w.multi
	n.Orientation, n.AutoComplete = w.orientation, w.autoComplete
	n.Controls = t.semRef(w.controls)
	n.ActiveDescendant = t.semRef(w.activeDesc)
}

// semRef returns the node ID of the element r is attached to, 0 when it
// is not mounted.
func (t *Tree) semRef(r *elemRef) semantics.NodeID {
	if r == nil || r.el == nil || !r.el.mounted {
		return 0
	}
	return t.semID(r.el)
}
