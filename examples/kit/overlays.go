package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the overlays: Dialog, AlertDialog, Popover, Tooltip and the
// Toaster. Each overlay has a Trigger slot, the element that opens it; here
// it is a kit.Button, and the overlay puts its props on that button, so the
// button you write is the one a screen reader finds.

func dialogPage() tela.Node {
	open, setOpen := tela.UseState(false)
	external, setExternal := tela.UseState(false)
	saved, setSaved := tela.UseState("")
	toast := kit.UseToast()
	return grid(
		demo("Edit profile", "A modal dialog: the page is inert, Tab stays inside, Escape and a press on the scrim close it, and the focus returns to the trigger.", colStart,
			kit.Dialog{
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text("Edit profile")},
				Title:       Text("Edit profile"),
				Description: Text("Make changes to your profile here. Click save when you're done."),
				Open:        &open, OnOpenChange: setOpen.Set,
				Class: "sm:max-w-[425px]",
				Children: Div(Class("flex flex-col gap-4"),
					kit.Field{Orientation: kit.Horizontal, Label: "Name", Class: "flex-row-reverse items-center gap-4", LabelClass: "w-20 justify-end",
						Children: kit.Input{DefaultValue: "Mei Tanaka"}},
					kit.Field{Orientation: kit.Horizontal, Label: "Username", Class: "flex-row-reverse items-center gap-4", LabelClass: "w-20 justify-end",
						Children: kit.Input{DefaultValue: "@mei"}},
				),
				Footer: kit.Button{OnClick: func() {
					setOpen.Set(false)
					setSaved.Set("Profile saved")
					toast.Success("Profile saved", kit.ToastDescription("Your changes are live."))
				}, Children: Text("Save changes")},
			},
			readout(map[bool]string{true: "No changes yet.", false: saved}[saved == ""])),
		demo("Share link", "A dialog with a read-only field and a button, and the close button, which shows by default.", "",
			kit.Dialog{
				Trigger:     kit.Button{Variant: kit.Outline, Icon: icons.Share2, Children: Text("Share")},
				Title:       Text("Share link"),
				Description: Text("Anyone who has this link will be able to view this."),
				Class:       "sm:max-w-md",
				Children: Div(Class("flex items-center gap-2"),
					kit.Input{ReadOnly: true, DefaultValue: "https://tela.dev/s/kit-gallery", AriaLabel: "Link"},
					kit.Button{Size: kit.Sm, Class: "px-3", Children: Text("Copy")}),
			}),
		demo("Scrolling content", "The content slot scrolls inside the panel with a max-h class.", "",
			kit.Dialog{
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text("Terms of service")},
				Title:       Text("Terms of service"),
				Description: Text("Please read them before you continue."),
				Children: Div(Class("flex max-h-48 flex-col gap-3 overflow-y-auto pr-2 text-sm text-muted-foreground"),
					P(Text("1. Using the service. You agree to use it in line with the law and these terms, and to keep your credentials safe.")),
					P(Text("2. Your content. You own what you create. You give us the licence we need to run the service for you.")),
					P(Text("3. Billing. Paid plans renew each period until cancelled. You can cancel at any time from the billing page.")),
					P(Text("4. Termination. We may suspend an account that breaks these terms, and we will tell you why.")),
					P(Text("5. Changes. We may update these terms. We will tell you before the changes take effect."))),
				Footer: kit.Button{Children: Text("I agree")},
			}),
		demo("Opened by your state", "With no Trigger the dialog is opened by Open, which is your state; the button here is not its trigger.", "",
			kit.Button{Variant: kit.Secondary, OnClick: func() { setExternal.Set(true) }, Children: Text("Show the welcome dialog")},
			kit.Dialog{Open: &external, OnOpenChange: setExternal.Set,
				Title: Text("Welcome to Tela"), Description: Text("This dialog has no trigger: its state is yours."),
				Footer: kit.Button{OnClick: func() { setExternal.Set(false) }, Children: Text("Get started")}}),
	)
}

func alertDialogPage() tela.Node {
	result, setResult := tela.UseState("")
	return grid(
		demo("Destructive confirmation", "Focus starts on Cancel, so Enter never confirms by accident. A press on the scrim does not close it; Escape does.", colStart,
			kit.AlertDialog{
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text("Delete project")},
				Title:       Text("Are you absolutely sure?"),
				Description: Text("This action cannot be undone. The project and its deployments are deleted from our servers."),
				Action:      Text("Delete"), Destructive: true,
				OnAction: func() { setResult.Set("Deleted the project.") },
				OnCancel: func() { setResult.Set("Cancelled.") },
			},
			readout(map[bool]string{true: "No decision yet.", false: result}[result == ""])),
		demo("Small", "Size Sm: a narrow dialog with the two buttons in equal columns.", "",
			kit.AlertDialog{
				Size:        kit.Sm,
				Trigger:     kit.Button{Variant: kit.Outline, Children: Text("Leave the team")},
				Title:       Text("Leave the team?"),
				Description: Text("You will lose access to its projects."),
				Cancel:      Text("Stay"), Action: Text("Leave"),
			}),
		demo("Default labels", "Cancel and Continue are the labels when the slots are nil.", "",
			kit.AlertDialog{
				Trigger:     kit.Button{Children: Text("Publish")},
				Title:       Text("Publish this page?"),
				Description: Text("It will be visible to everyone with the link."),
			}),
	)
}

func popoverPage() tela.Node {
	return grid(
		demo("Dimensions", "Not modal: the page stays usable, a press outside or Escape closes it, and the focus goes in and comes back.", "",
			kit.Popover{
				Trigger: kit.Button{Variant: kit.Outline, Children: Text("Open popover")},
				Align:   "start", AriaLabel: "Dimensions", Class: "w-80",
				Children: Div(Class("flex flex-col gap-4"),
					Div(Class("flex flex-col gap-1"), H4(Class("leading-none font-medium"), Text("Dimensions")), muted("Set the dimensions for the layer.")),
					Div(Class("flex flex-col gap-3"),
						dimension("Width", "100%"), dimension("Max. width", "300px"), dimension("Height", "25px")))}),
		demo("Placements", "Placement is the side, Align the position along it; the panel flips and shifts to stay in the window.", "",
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Top")}, Placement: "top", AriaLabel: "Top", Children: Text("Opens above.")},
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Right")}, Placement: "right", AriaLabel: "Right", Children: Text("Opens to the right.")},
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Bottom")}, Placement: "bottom", AriaLabel: "Bottom", Children: Text("Opens below.")},
			kit.Popover{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Left")}, Placement: "left", AriaLabel: "Left", Children: Text("Opens to the left.")}),
		demo("Any element as trigger", "TriggerFunc hands you the props to put on your own element, as Radix's asChild does.", "",
			kit.Popover{
				Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
					return Button(p, Class("rounded-full outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"), AriaLabel("Account"),
						kit.Avatar{Fallback: "MT"})
				}),
				Align: "start", AriaLabel: "Account", Class: "w-64",
				Children: Div(Class("flex items-center gap-3"),
					kit.Avatar{Size: kit.Lg, Fallback: "MT"},
					Div(Class("flex flex-col"), Span(Class("text-sm font-medium"), Text("Mei Tanaka")), muted("mei@example.com")))}),
	)
}

func dimension(label, value string) tela.Node {
	return Fragment(kit.Field{Props: Key(label), Orientation: kit.Horizontal, Label: label, Class: "flex-row-reverse items-center gap-4", LabelClass: "w-24 shrink-0",
		Children: kit.Input{DefaultValue: value, Class: "h-8"}})
}

func tooltipPage() tela.Node {
	tip := func(label string, icon tela.Icon, content string, opts kit.Tooltip) tela.Node {
		opts.Trigger = kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: icon, AriaLabel: label}
		opts.Content = Text(content)
		return Fragment(opts)
	}
	tipText := func(label, content string, opts kit.Tooltip) tela.Node {
		opts.Trigger = kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(label)}
		opts.Content = Text(content)
		return Fragment(opts)
	}
	return grid(
		demo("Icon buttons", "A tooltip names what an icon button does: on hover, on keyboard focus, and it describes its trigger.", "",
			tip("Add to library", icons.Plus, "Add to library", kit.Tooltip{}),
			tip("Download", icons.Download, "Download", kit.Tooltip{}),
			tip("Share", icons.Share2, "Share a link", kit.Tooltip{}),
			tip("Delete", icons.Trash2, "Delete forever", kit.Tooltip{})),
		demo("Placements", "Placement and Align, with a flip when there is no room.", "",
			tipText("Top", "On the top", kit.Tooltip{Placement: "top"}),
			tipText("Right", "On the right", kit.Tooltip{Placement: "right"}),
			tipText("Bottom", "On the bottom", kit.Tooltip{Placement: "bottom"}),
			tipText("Left", "On the left", kit.Tooltip{Placement: "left"})),
		demo("Delay and rich content", "Delay makes the pointer rest first. Once one tooltip has opened the next opens at once. Content is any short component.", "",
			kit.Tooltip{Delay: 600 * time.Millisecond,
				Trigger: kit.Button{Variant: kit.Outline, Children: Text("Hover and wait")},
				Content: Text("Opens after 600 ms")},
			kit.Tooltip{
				Trigger: kit.Button{Variant: kit.Outline, Icon: icons.Settings, Children: Text("Settings")},
				Content: Div(Class("flex items-center gap-2"), Text("Open settings"),
					Span(Class("rounded bg-background/20 px-1 font-mono text-[10px]"), Text("Ctrl ,")))}),
		demo("Disabled", "A disabled tooltip never opens.", "",
			kit.Tooltip{Disabled: true, Trigger: kit.Button{Variant: kit.Outline, Children: Text("No tooltip")}, Content: Text("You will not see this")}),
	)
}

func toastPage() tela.Node {
	toast := kit.UseToast()
	upload := func() {
		id := toast.Loading("Uploading photos...")
		time.AfterFunc(1800*time.Millisecond, func() {
			toast.Success("3 photos uploaded", kit.ToastReplace(id), kit.ToastDescription("They are in the Summer album."))
		})
	}
	return grid(
		demo("Kinds", "Toasts stack at a corner of the window; the newest is in front. The pointer over the stack fans it out and stops the timers.", "",
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Show("Event created", kit.ToastDescription("Sunday, October 25 at 9:00 AM")) }, Children: Text("Default")},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Success("Changes saved") }, Children: Text("Success")},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Info("A new version is available") }, Children: Text("Info")},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Warning("Your trial ends in 3 days") }, Children: Text("Warning")},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Error("The upload failed", kit.ToastDescription("Check your connection and try again.")) }, Children: Text("Error")}),
		demo("Action", "ToastAction adds a button; clicking it runs your function and dismisses the toast.", "",
			kit.Button{Variant: kit.Outline, Children: Text("Delete the draft"), OnClick: func() {
				toast.Show("Draft deleted", kit.ToastAction("Undo", func() { toast.Success("Draft restored") }))
			}}),
		demo("Loading, then done", "A Loading toast is replaced with ToastReplace; every method of the handle is safe from any goroutine.", "",
			kit.Button{Variant: kit.Outline, Icon: icons.Download, OnClick: upload, Children: Text("Upload photos")}),
		demo("Forever, and dismissing", "ToastDuration(kit.ToastForever) keeps the toast until it is dismissed.", "",
			kit.Button{Variant: kit.Outline, Children: Text("Sticky toast"), OnClick: func() {
				toast.Show("Pinned until you close it", kit.ToastDuration(kit.ToastForever))
			}},
			kit.Button{Variant: kit.Ghost, Children: Text("Dismiss all"), OnClick: toast.DismissAll}),
	)
}
