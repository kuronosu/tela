// Portions of this file follow shadcn/ui, alert-dialog.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// AlertDialogStyle is the recipe of [AlertDialog] (shadcn/ui's
// AlertDialogOverlay, AlertDialogContent, AlertDialogHeader,
// AlertDialogTitle, AlertDialogDescription and AlertDialogFooter). Its
// Positioner, Overlay and Exit work as the ones of [DialogStyle].
type AlertDialogStyle struct {
	Positioner  string
	Overlay     string
	OverlayExit string
	// Root is the panel; Sizes adds the width of the size (the default and
	// Sm), and Exit is what the panel transitions to as it closes.
	Root  string
	Sizes map[Size]string
	Exit  string
	// Header holds the title and the description, HeaderSizes adds the
	// alignment of the size: shadcn centers the text, and aligns it to the
	// start of the default size from the sm breakpoint up.
	Header      string
	HeaderSizes map[Size]string
	Title       string
	Description string
	// Footer is the row of the two buttons, FooterSizes adds the size's
	// layout (two equal columns for Sm).
	Footer      string
	FooterSizes map[Size]string
}

func init() {
	defaultStyle.AlertDialog = AlertDialogStyle{
		Positioner:  "fixed inset-0 z-50 flex items-center justify-center p-4",
		Overlay:     "absolute inset-0 bg-black/50 transition-opacity duration-150 starting:opacity-0",
		OverlayExit: "opacity-0",
		Root: "relative grid w-full gap-4 rounded-lg border border-border bg-background p-6 shadow-lg " +
			"transition duration-200 starting:scale-95 starting:opacity-0",
		Sizes: map[Size]string{
			"": "max-w-lg",
			Sm: "max-w-xs",
		},
		Exit:   "scale-95 opacity-0",
		Header: "grid grid-rows-[auto_1fr] place-items-center gap-1.5 text-center",
		HeaderSizes: map[Size]string{
			"": "sm:place-items-start sm:text-start",
		},
		Title:       "text-lg font-semibold",
		Description: "text-sm text-muted-foreground",
		Footer:      "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
		FooterSizes: map[Size]string{
			Sm: "grid grid-cols-2",
		},
	}
}

// AlertDialog is a modal dialog that interrupts the user with a question
// that needs an answer, shadcn/ui's AlertDialog: a title, a description and
// two buttons, Cancel and Action. It has the alertdialog role, takes the
// focus on its first button (Cancel, so that Enter never confirms by
// accident), and, unlike a [Dialog], is not closed by a press on the scrim:
// only the buttons and Escape close it.
//
//	kit.AlertDialog{
//	    Trigger:     kit.Button{Variant: kit.Outline, Children: h.Text("Delete project")},
//	    Title:       h.Text("Are you absolutely sure?"),
//	    Description: h.Text("This action cannot be undone. The project and its deployments are deleted."),
//	    Action:      h.Text("Delete"),
//	    Destructive: true,
//	    OnAction:    deleteProject,
//	}
//
// Cancel and Action are the labels of the two buttons ("Cancel" and
// "Continue" when nil); a slot may be any component, such as a label with an
// icon. The buttons are [Button]s: Action is the default variant, or
// Destructive with Destructive set, and Cancel is the outline variant;
// ActionClass and CancelClass add classes to them. Both close the dialog;
// OnAction and OnCancel say which was chosen. Escape closes it as Cancel
// does, without calling OnCancel.
//
// The Trigger slot is a [Trigger]. Open, DefaultOpen and OnOpenChange are
// the usual controlled and uncontrolled pair.
type AlertDialog struct {
	// Trigger is the element that opens the dialog; nil for a dialog
	// opened only by its state.
	Trigger Trigger
	// Title and Description are the question and its explanation; both name
	// the dialog for assistive technologies.
	Title, Description tela.Component
	// Cancel and Action are the labels of the buttons.
	Cancel, Action tela.Component
	// Destructive gives the Action button the destructive variant.
	Destructive bool
	// OnAction and OnCancel are called when the Action or Cancel button is
	// pressed, before the dialog closes.
	OnAction, OnCancel func()
	// Size is the default or Sm, a narrow dialog with the buttons in two
	// equal columns and centered text.
	Size Size

	// Open points to the open state of a controlled dialog; nil leaves the
	// state to the dialog, which starts closed unless DefaultOpen is set.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// Disabled disables the trigger, which is then inert and dimmed by its
	// recipe; the dialog does not open. Set it here: the trigger props
	// override a Disabled given to the trigger component itself.
	Disabled bool

	// Class is added after the recipe's Root classes, to the panel. The
	// other Class fields do the same for their part.
	Class            string
	OverlayClass     string
	HeaderClass      string
	TitleClass       string
	DescriptionClass string
	FooterClass      string
	CancelClass      string
	ActionClass      string
	// Props are more props for the dialog element (the panel).
	Props tela.Child
}

// Build returns the trigger and the dialog's portal.
func (a AlertDialog) Build() tela.Node {
	s := &UseStyle().AlertDialog
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{
		Open: a.Open, DefaultOpen: a.DefaultOpen, OnOpenChange: a.OnOpenChange, Disabled: a.Disabled,
	})
	overlay := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Modal: true})
	dlg := aria.UseDialog(aria.DialogOptions{Role: "alertdialog", Modal: true, Ref: trig.OverlayRef})
	desc := tela.UseElementRef()
	size := pick("AlertDialog", "Size", s.Sizes, a.Size)

	root := alertDialogRoot(s, a, size, a.Size, overlay.Props, dlg.Props, dlg.TitleProps, desc, trig.Close)
	return h.Fragment(
		triggerOf(a.Trigger, trig.TriggerProps),
		h.Portal(h.Presence(h.If(trig.Open,
			h.Div(cls(s.Positioner),
				h.Div(cls(s.Overlay, a.OverlayClass), h.Exit(s.OverlayExit)),
				root,
			)))),
	)
}

// alertDialogRoot returns the panel element: the hooks' props, then the
// instance's, and the parts.
func alertDialogRoot(s *AlertDialogStyle, a AlertDialog, sizeClass string, size Size,
	overlay, dialog, titleProps tela.Prop, desc tela.ElementRef, closeIt func()) tela.Node {
	kids := []tela.Child{cls(s.Root, sizeClass, a.Class), h.Exit(s.Exit), overlay, dialog, a.Props}
	hasDesc := a.Description != nil && desc != (tela.ElementRef{})
	if hasDesc {
		kids = append(kids, h.AriaDescribedBy(desc))
	}
	if a.Title != nil || a.Description != nil {
		header := []tela.Child{cls(s.Header, s.HeaderSizes[size], a.HeaderClass)}
		if a.Title != nil {
			header = append(header, h.H2(titleProps, cls(s.Title, a.TitleClass), a.Title))
		}
		if a.Description != nil {
			dk := []tela.Child{cls(s.Description, a.DescriptionClass)}
			if hasDesc {
				dk = append(dk, h.Ref(desc))
			}
			header = append(header, h.P(append(dk, a.Description)...))
		}
		kids = append(kids, h.Div(header...))
	}

	cancel, action := a.Cancel, a.Action
	if cancel == nil {
		cancel = h.Text("Cancel")
	}
	if action == nil {
		action = h.Text("Continue")
	}
	actionVariant := Default
	if a.Destructive {
		actionVariant = Destructive
	}
	onCancel, onAction := a.OnCancel, a.OnAction
	kids = append(kids, h.Div(cls(s.Footer, s.FooterSizes[size], a.FooterClass),
		Button{Variant: Outline, Type: "button", Class: a.CancelClass, Children: cancel, OnClick: func() {
			if onCancel != nil {
				onCancel()
			}
			closeIt()
		}},
		Button{Variant: actionVariant, Type: "button", Class: a.ActionClass, Children: action, OnClick: func() {
			if onAction != nil {
				onAction()
			}
			closeIt()
		}},
	))
	return h.Div(kids...)
}
