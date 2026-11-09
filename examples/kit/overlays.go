package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of the overlays: Dialog, AlertDialog, Popover, Tooltip and the
// Toaster. Each overlay has a Trigger slot, the element that opens it; here
// it is a kit.Button, and the overlay puts its props on that button, so the
// button you write is the one a screen reader finds. The stages are written
// with tr: English here, Spanish and Arabic in the catalogs.

func dialogPage() tela.Node {
	open, setOpen := tela.UseState(false)
	external, setExternal := tela.UseState(false)
	saved, setSaved := tela.UseState("")
	toast := kit.UseToast()
	f := i18n.UseFormatter()
	tr, k := trFor(f), kitTextFor(f)
	return grid(
		demo("Edit profile", "A modal dialog: the page is inert, Tab stays inside, Escape and a press on the scrim close it, and the focus returns to the trigger.", colStart,
			kit.Dialog{
				CloseLabel:  k.Close,
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text(tr("dialog.editProfile", "Edit profile"))},
				Title:       Text(tr("dialog.editProfile", "Edit profile")),
				Description: Text(tr("dialog.editProfileText", "Make changes to your profile here. Click save when you're done.")),
				Open:        &open, OnOpenChange: setOpen.Set,
				Class: "sm:max-w-[425px]",
				Children: Div(Class("flex flex-col gap-4"),
					kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: tr("dialog.name", "Name"), Class: "items-center gap-4", LabelClass: "w-20 justify-end",
						Children: kit.Input{DefaultValue: "Mei Tanaka"}},
					kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: tr("dialog.username", "Username"), Class: "items-center gap-4", LabelClass: "w-20 justify-end",
						Children: kit.Input{DefaultValue: "@mei", Props: Dir("ltr")}},
				),
				Footer: kit.Button{OnClick: func() {
					setOpen.Set(false)
					setSaved.Set(tr("dialog.saved", "Profile saved"))
					toast.Success(tr("dialog.saved", "Profile saved"), kit.ToastDescription(tr("dialog.live", "Your changes are live.")))
				}, Children: Text(tr("dialog.saveChanges", "Save changes"))},
			},
			readout(map[bool]string{true: tr("dialog.noChanges", "No changes yet."), false: saved}[saved == ""])),
		demo("Share link", "A dialog with a read-only field and a button, and the close button, which shows by default.", "",
			kit.Dialog{
				CloseLabel:  k.Close,
				Trigger:     kit.Button{Variant: kit.Outline, Icon: icons.Share2, Children: Text(tr("dialog.share", "Share"))},
				Title:       Text(tr("dialog.shareLink", "Share link")),
				Description: Text(tr("dialog.shareText", "Anyone who has this link will be able to view this.")),
				Class:       "sm:max-w-md",
				Children: Div(Class("flex items-center gap-2"),
					kit.Input{ReadOnly: true, DefaultValue: "https://tela.dev/s/kit-gallery", AriaLabel: tr("dialog.link", "Link")},
					kit.Button{Size: kit.Sm, Class: "px-3", Children: Text(tr("dialog.copy", "Copy"))}),
			}),
		demo("Scrolling content", "The content slot scrolls inside the panel with a max-h class.", "",
			kit.Dialog{
				CloseLabel:  k.Close,
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text(tr("dialog.terms", "Terms of service"))},
				Title:       Text(tr("dialog.terms", "Terms of service")),
				Description: Text(tr("dialog.termsText", "Please read them before you continue.")),
				Children: Div(Class("flex max-h-48 flex-col gap-3 overflow-y-auto pr-2 text-sm text-muted-foreground"),
					P(Text(tr("dialog.term1", "1. Using the service. You agree to use it in line with the law and these terms, and to keep your credentials safe."))),
					P(Text(tr("dialog.term2", "2. Your content. You own what you create. You give us the licence we need to run the service for you."))),
					P(Text(tr("dialog.term3", "3. Billing. Paid plans renew each period until cancelled. You can cancel at any time from the billing page."))),
					P(Text(tr("dialog.term4", "4. Termination. We may suspend an account that breaks these terms, and we will tell you why."))),
					P(Text(tr("dialog.term5", "5. Changes. We may update these terms. We will tell you before the changes take effect.")))),
				Footer: kit.Button{Children: Text(tr("dialog.agree", "I agree"))},
			}),
		demo("Opened by your state", "With no Trigger the dialog is opened by Open, which is your state; the button here is not its trigger.", "",
			kit.Button{Variant: kit.Secondary, OnClick: func() { setExternal.Set(true) }, Children: Text(tr("dialog.showWelcome", "Show the welcome dialog"))},
			kit.Dialog{CloseLabel: k.Close, Open: &external, OnOpenChange: setExternal.Set,
				Title: Text(tr("dialog.welcome", "Welcome to Tela")), Description: Text(tr("dialog.welcomeText", "This dialog has no trigger: its state is yours.")),
				Footer: kit.Button{OnClick: func() { setExternal.Set(false) }, Children: Text(tr("dialog.getStarted", "Get started"))}}),
	)
}

func alertDialogPage() tela.Node {
	result, setResult := tela.UseState("")
	f := i18n.UseFormatter()
	tr, k := trFor(f), kitTextFor(f)
	return grid(
		demo("Destructive confirmation", "Focus starts on Cancel, so Enter never confirms by accident. A press on the scrim does not close it; Escape does.", colStart,
			kit.AlertDialog{
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text(tr("alertDialog.deleteProject", "Delete project"))},
				Title:       Text(tr("alertDialog.sure", "Are you absolutely sure?")),
				Description: Text(tr("alertDialog.sureText", "This action cannot be undone. The project and its deployments are deleted from our servers.")),
				Cancel:      Text(k.Cancel), Action: Text(tr("alertDialog.delete", "Delete")), Destructive: true,
				OnAction: func() { setResult.Set(tr("alertDialog.deleted", "Deleted the project.")) },
				OnCancel: func() { setResult.Set(tr("alertDialog.cancelled", "Cancelled.")) },
			},
			readout(map[bool]string{true: tr("alertDialog.noDecision", "No decision yet."), false: result}[result == ""])),
		demo("Small", "Size Sm: a narrow dialog with the two buttons in equal columns.", "",
			kit.AlertDialog{
				Size:        kit.Sm,
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text(tr("alertDialog.leaveTeam", "Leave the team"))},
				Title:       Text(tr("alertDialog.leaveQuestion", "Leave the team?")),
				Description: Text(tr("alertDialog.leaveText", "You will lose access to its projects.")),
				Cancel:      Text(tr("alertDialog.stay", "Stay")), Action: Text(tr("alertDialog.leave", "Leave")),
			}),
		demo("Default labels", "Cancel and Continue are the labels when the slots are nil; here they come from the language's catalog.", "",
			kit.AlertDialog{
				Cancel: Text(k.Cancel), Action: Text(k.Continue),
				Trigger:     kit.Button{Children: Text(tr("alertDialog.publish", "Publish"))},
				Title:       Text(tr("alertDialog.publishQuestion", "Publish this page?")),
				Description: Text(tr("alertDialog.publishText", "It will be visible to everyone with the link.")),
			}),
	)
}

func popoverPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	return grid(
		demo("Dimensions", "Not modal: the page stays usable, a press outside or Escape closes it, and the focus goes in and comes back.", "",
			kit.Popover{
				Trigger: kit.Button{Variant: kit.Outline, Children: Text(tr("popover.open", "Open popover"))},
				Align:   "start", AriaLabel: tr("popover.dimensions", "Dimensions"), Class: "w-80",
				Children: Div(Class("flex flex-col gap-4"),
					Div(Class("flex flex-col gap-1"), H4(Class("leading-none font-medium"), Text(tr("popover.dimensions", "Dimensions"))), muted(tr("popover.dimensionsText", "Set the dimensions for the layer."))),
					Div(Class("flex flex-col gap-3"),
						dimension(tr("popover.width", "Width"), "100%"), dimension(tr("popover.maxWidth", "Max. width"), "300px"), dimension(tr("popover.height", "Height"), "25px")))}),
		demo("Placements", "Placement is the side, Align the position along it; the panel flips and shifts to stay in the window.", "",
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("popover.top", "Top"))}, Placement: "top", AriaLabel: tr("popover.top", "Top"), Children: Text(tr("popover.above", "Opens above."))},
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("popover.right", "Right"))}, Placement: "right", AriaLabel: tr("popover.right", "Right"), Children: Text(tr("popover.toRight", "Opens to the right."))},
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("popover.bottom", "Bottom"))}, Placement: "bottom", AriaLabel: tr("popover.bottom", "Bottom"), Children: Text(tr("popover.below", "Opens below."))},
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("popover.left", "Left"))}, Placement: "left", AriaLabel: tr("popover.left", "Left"), Children: Text(tr("popover.toLeft", "Opens to the left."))}),
		demo("Any element as trigger", "TriggerFunc hands you the props to put on your own element, as Radix's asChild does.", "",
			kit.Popover{
				Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
					return Button(p, Class("rounded-full outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"), AriaLabel(tr("popover.account", "Account")),
						kit.Avatar{Fallback: "MT"})
				}),
				Align: "start", AriaLabel: tr("popover.account", "Account"), Class: "w-64",
				Children: Div(Class("flex items-center gap-3"),
					kit.Avatar{Size: kit.Lg, Fallback: "MT"},
					Div(Class("flex flex-col"), Span(Class("text-sm font-medium"), Text("Mei Tanaka")), muted("mei@example.com")))}),
	)
}

func dimension(label, value string) tela.Node {
	return kit.Field{Props: Key(label), Orientation: kit.Horizontal, ControlAfter: true, Label: label, Class: "items-center gap-4", LabelClass: "w-24 shrink-0",
		Children: kit.Input{DefaultValue: value, Class: "h-8"}}
}
