// Portions of this file follow shadcn/ui, dialog.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// Icons embedded in this file are copied unaltered from Lucide
// (https://lucide.dev, lucide-static 1.61.0), Copyright (c) 2026 Lucide Icons and
// Contributors, licensed under the ISC License (the icons derived from Feather are
// MIT, Copyright (c) 2013-present Cole Bemis).
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// DialogStyle is the recipe of [Dialog] (shadcn/ui's DialogOverlay,
// DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter
// and the close button of DialogContent).
//
// Root is the dialog element, the panel with the role. shadcn centers it
// with fixed top-[50%] left-[50%] and a translation; here the Positioner,
// a fixed layer over the window, centers it with flex, which gives the same
// box and lets the panel keep its own transform for the entrance.
type DialogStyle struct {
	// Positioner is the layer over the window that holds the overlay and
	// the dialog and centers the dialog, leaving shadcn's 1rem of room on
	// each side (its max-w-[calc(100%-2rem)]).
	Positioner string
	// Overlay is the scrim behind the dialog, and OverlayExit the classes
	// it transitions to while the dialog closes.
	Overlay     string
	OverlayExit string
	// Root is the dialog panel, and Exit the classes it transitions to
	// while it closes (the reverse of its starting: classes).
	Root string
	Exit string
	// Header holds the title and the description; it is drawn when the
	// dialog has either.
	Header      string
	Title       string
	Description string
	Footer      string
	// Close is the button in the corner (ShowClose) and CloseIcon its icon.
	Close     string
	CloseIcon string
}

func init() {
	defaultStyle.Dialog = DialogStyle{
		Positioner:  "fixed inset-0 z-50 flex items-center justify-center p-4",
		Overlay:     "absolute inset-0 bg-black/50 transition-opacity duration-150 starting:opacity-0",
		OverlayExit: "opacity-0",
		Root: "relative grid w-full max-w-lg gap-4 rounded-lg border border-border bg-background p-6 shadow-lg outline-none " +
			"transition duration-200 starting:scale-95 starting:opacity-0",
		Exit:        "scale-95 opacity-0",
		Header:      "flex flex-col gap-2 text-center sm:text-start",
		Title:       "text-lg leading-none font-semibold",
		Description: "text-sm text-muted-foreground",
		Footer:      "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
		Close: "absolute top-4 end-4 rounded-xs opacity-70 ring-offset-background transition-opacity hover:opacity-100 " +
			"focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none",
		CloseIcon: "pointer-events-none size-4 shrink-0",
	}
}

// iconX is Lucide's x (ISC, THIRD_PARTY.md), the default icon of the close
// button of [Dialog].
var iconX = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`}

// Dialog is a modal window over the page, shadcn/ui's Dialog: a trigger, and
// a panel with a title, a description, content and a footer that opens in
// the middle of the window over a scrim. While it is open the page is
// inert, Tab stays inside the dialog, Escape and a press on the scrim close
// it, and the focus goes back to the trigger. It fades and scales in and
// out.
//
//	kit.Dialog{
//	    Trigger:     kit.Button{Variant: kit.Outline, Children: h.Text("Edit profile")},
//	    Title:       h.Text("Edit profile"),
//	    Description: h.Text("Make changes to your profile here."),
//	    Children:    form,
//	    Footer:      kit.Button{Children: h.Text("Save changes")},
//	    ShowClose:   true,
//	}
//
// The Trigger slot is a [Trigger]: a [Button], or anything made one with
// [TriggerFunc]. Without a Trigger the dialog is opened from your state:
//
//	kit.Dialog{Open: &open, OnOpenChange: setOpen.Set, Title: h.Text("Welcome")}
//
// The panel exists only while the dialog is open, so the state of what is
// inside it starts afresh each time. Open, DefaultOpen and OnOpenChange are
// the usual controlled and uncontrolled pair. Close the dialog from inside
// with a button whose handler asks for it through OnOpenChange, or with
// the state you control.
//
// A dialog that asks for a decision and must not close by a press outside
// is an [AlertDialog].
type Dialog struct {
	// Trigger is the element that opens the dialog; nil for a dialog
	// opened only by its state.
	Trigger Trigger
	// Title and Description name and describe the dialog for assistive
	// technologies and sit in its header. Give a dialog that has no visible
	// title an AriaLabel.
	Title, Description tela.Component
	// Children is the content, between the header and the footer.
	Children tela.Component
	// Footer is the row of actions at the bottom, usually buttons.
	Footer tela.Component
	// ShowClose draws the close button in the top-right corner. shadcn
	// draws it unless told not to; here it is off until asked for.
	ShowClose bool
	// CloseIcon replaces the X of the close button; CloseLabel is its
	// accessible name ("Close").
	CloseIcon  tela.Icon
	CloseLabel string
	// AriaLabel names a dialog that has no Title.
	AriaLabel string

	// Open points to the open state of a controlled dialog; nil leaves the
	// state to the dialog, which starts closed unless DefaultOpen is set.
	// OnOpenChange is called with the next state when the trigger, Escape,
	// a press on the scrim or the close button asks for a change.
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
	CloseClass       string
	// Props are more props for the dialog element (the panel).
	Props tela.Child
}

// Build returns the trigger and the dialog's portal.
func (d Dialog) Build() tela.Node {
	s := &UseStyle().Dialog
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{
		Open: d.Open, DefaultOpen: d.DefaultOpen, OnOpenChange: d.OnOpenChange, Disabled: d.Disabled,
	})
	overlay := aria.UseOverlay(aria.OverlayOptions{
		Open: trig.Open, OnClose: trig.Close, Dismissable: true, Modal: true,
	})
	dlg := aria.UseDialog(aria.DialogOptions{Modal: true, AriaLabel: d.AriaLabel, Ref: trig.OverlayRef})
	desc := tela.UseElementRef()

	panel := dialogPanel{
		style: s, title: d.Title, description: d.Description, children: d.Children, footer: d.Footer,
		showClose: d.ShowClose, closeIcon: d.CloseIcon, closeLabel: d.CloseLabel, onClose: trig.Close,
		class: d.Class, headerClass: d.HeaderClass, titleClass: d.TitleClass, descriptionClass: d.DescriptionClass,
		footerClass: d.FooterClass, closeClass: d.CloseClass,
		titleProps: dlg.TitleProps, descriptionRef: desc,
	}
	root := panel.root(overlay.Props, dlg.Props, d.Props)
	return h.Fragment(
		triggerOf(d.Trigger, trig.TriggerProps),
		h.Portal(h.Presence(h.If(trig.Open,
			h.Div(cls(s.Positioner),
				h.Div(cls(s.Overlay, d.OverlayClass), h.Exit(s.OverlayExit)),
				root,
			)))),
	)
}

// dialogPanel is what the panel of a [Dialog] or an [AlertDialog] is made
// of; root builds its element.
type dialogPanel struct {
	style                                            *DialogStyle
	title, description, children, footer             tela.Component
	showClose                                        bool
	closeIcon                                        tela.Icon
	closeLabel                                       string
	onClose                                          func()
	class, headerClass, titleClass, descriptionClass string
	footerClass, closeClass                          string
	// titleProps go on the title, and descriptionRef is attached to the
	// description, which the panel is described by.
	titleProps     tela.Prop
	descriptionRef tela.ElementRef
}

// root returns the dialog element with props (from the hooks, then the
// instance's) applied.
func (p dialogPanel) root(props ...tela.Child) tela.Node {
	s := p.style
	kids := []tela.Child{cls(s.Root, p.class), h.Exit(s.Exit)}
	kids = append(kids, props...)
	if p.description != nil && p.descriptionRef != (tela.ElementRef{}) {
		kids = append(kids, h.AriaDescribedBy(p.descriptionRef))
	}
	if p.title != nil || p.description != nil {
		header := []tela.Child{cls(s.Header, p.headerClass)}
		if p.title != nil {
			header = append(header, h.H2(p.titleProps, cls(s.Title, p.titleClass), p.title))
		}
		if p.description != nil {
			dk := []tela.Child{cls(s.Description, p.descriptionClass)}
			if p.descriptionRef != (tela.ElementRef{}) {
				dk = append(dk, h.Ref(p.descriptionRef))
			}
			header = append(header, h.P(append(dk, p.description)...))
		}
		kids = append(kids, h.Div(header...))
	}
	kids = append(kids, p.children)
	if p.footer != nil {
		kids = append(kids, h.Div(cls(s.Footer, p.footerClass), p.footer))
	}
	if p.showClose {
		ic := p.closeIcon
		if ic == (tela.Icon{}) {
			ic = iconX
		}
		label := p.closeLabel
		if label == "" {
			label = "Close"
		}
		close := p.onClose
		kids = append(kids, h.Button(cls(s.Close, p.closeClass), h.Type("button"), h.AriaLabel(label),
			h.OnClick(func() {
				if close != nil {
					close()
				}
			}),
			icon(ic, s.CloseIcon)))
	}
	return h.Div(kids...)
}
