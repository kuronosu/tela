// Portions of this file follow React Aria's useDialog
// (https://github.com/adobe/react-spectrum, packages/@react-aria/dialog),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// DialogOptions configures [UseDialog].
type DialogOptions struct {
	// Role is "dialog" (the default) or "alertdialog", for a dialog that
	// interrupts the user with a message that needs a response. Another
	// value panics in dev mode.
	Role string
	// Modal marks the dialog aria-modal and keeps Tab inside it. Make the
	// dialog's [UseOverlay] modal too, so that the rest of the window is
	// inert.
	Modal bool
	// AriaLabel names the dialog when it has no visible title. Without it
	// the dialog is named by the element that has TitleProps.
	AriaLabel string
	// Ref is the dialog element's ref, for a hook used together with others
	// on the same element (the overlay trigger's OverlayRef, UsePopover);
	// the zero value makes the hook create its own.
	Ref tela.ElementRef
}

// DialogResult is what [UseDialog] returns.
type DialogResult struct {
	// Props go to the dialog's element: its role, aria-modal, its name, a
	// focus scope that moves the focus into the dialog when it opens, keeps
	// it there (with Modal) and gives it back to the element that had it
	// when the dialog closes, and the ref.
	Props tela.Prop
	// TitleProps go to the element that titles the dialog (a heading): its
	// text names the dialog.
	TitleProps tela.Prop
	// Manager moves the focus inside the dialog.
	Manager FocusManager
	// Ref is the ref attached to the dialog's element by Props.
	Ref tela.ElementRef
}

// UseDialog returns the props of a dialog (React Aria's useDialog): its
// role, its name from the title, and the focus: it goes to the first
// tabbable element inside when the dialog opens (the dialog itself when
// there is none), and returns to the element that had it when the dialog
// closes. Combine it with [UseOverlay] for the dismissal and the modality,
// on the same element.
//
//	dlg := aria.UseDialog(aria.DialogOptions{Modal: true})
//	ov := aria.UseOverlay(aria.OverlayOptions{Open: open, OnClose: close, Dismissable: true, Modal: true})
//	return Portal(If(open,
//	    Div(ov.Props, dlg.Props, Class("w-96 rounded-xl bg-background p-6 shadow-xl"),
//	        H2(dlg.TitleProps, Class("text-lg font-semibold"), Text("Delete project?")),
//	        Button(OnClick(close), Text("Cancel")))))
//
// An "alertdialog" is not dismissed by a press outside it: leave
// Dismissable off. The dialog element takes the focus when it has nothing
// tabbable (it is focusable with a script, not in the Tab order).
func UseDialog(o DialogOptions) DialogResult {
	role := o.Role
	switch role {
	case "":
		role = "dialog"
	case "dialog", "alertdialog":
	default:
		if core.DevNow() {
			panic(fmt.Sprintf("aria: DialogOptions.Role %q: a dialog is \"dialog\" or \"alertdialog\"", role))
		}
		role = "dialog"
	}
	title := tela.UseElementRef()
	scope := UseFocusScope(FocusScopeOptions{Contain: o.Modal, Restore: true, AutoFocus: true, Ref: o.Ref})
	name := h.AriaLabelledBy(title)
	if o.AriaLabel != "" {
		name = h.AriaLabel(o.AriaLabel)
	}
	return DialogResult{
		Props:      core.MergeProps(scope.Props, h.Role(role), h.TabIndex(-1), h.AriaModal(o.Modal), name),
		TitleProps: h.Ref(title),
		Manager:    scope.Manager,
		Ref:        scope.Ref,
	}
}
