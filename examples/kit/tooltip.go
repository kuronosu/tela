package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the Tooltip and the Toaster. Their stages are written with
// tr: English here, Spanish and Arabic in the catalogs.

func tooltipPage() tela.Node {
	tr := useTr()
	tip := func(label string, icon tela.Icon, content string, opts kit.Tooltip) tela.Node {
		opts.Trigger = kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: icon, AriaLabel: label}
		opts.Content = Text(content)
		return opts
	}
	tipText := func(label, content string, opts kit.Tooltip) tela.Node {
		opts.Trigger = kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(label)}
		opts.Content = Text(content)
		return opts
	}
	items := []kit.MenuItem[string]{
		{Key: "rename", Label: tr("tooltip.rename", "Rename")}, {Key: "duplicate", Label: tr("tooltip.duplicate", "Duplicate")},
		{Kind: kit.MenuSeparator},
		{Key: "delete", Label: tr("tooltip.delete", "Delete"), Variant: kit.Destructive}}
	return grid(
		demo("Icon buttons", "A tooltip names what an icon button does: on hover, on keyboard focus, and it describes its trigger.", "",
			tip(tr("tooltip.addLabel", "Add to library"), icons.Plus, tr("tooltip.addLabel", "Add to library"), kit.Tooltip{}),
			tip(tr("tooltip.download", "Download"), icons.Download, tr("tooltip.download", "Download"), kit.Tooltip{}),
			tip(tr("tooltip.share", "Share"), icons.Share2, tr("tooltip.shareLink", "Share a link"), kit.Tooltip{}),
			tip(tr("tooltip.deleteLabel", "Delete"), icons.Trash2, tr("tooltip.forever", "Delete forever"), kit.Tooltip{})),
		demo("Other triggers", "Anything that is a Trigger takes a tooltip: a DropdownMenu's button (its menu still opens), a Toggle, a Checkbox, a Switch and a Slider.", "flex flex-wrap items-center gap-x-6 gap-y-5",
			kit.Tooltip{
				Trigger: kit.DropdownMenu[string]{Trigger: kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: icons.Ellipsis, AriaLabel: tr("tooltip.moreLabel", "More options")}, Items: items},
				Content: Text(tr("tooltip.more", "More options"))},
			kit.Tooltip{
				Trigger: kit.Toggle{Variant: kit.Outline, Icon: icons.Bold, AriaLabel: tr("tooltip.boldLabel", "Bold")},
				Content: Text(tr("tooltip.bold", "Bold, Ctrl+B"))},
			kit.Tooltip{
				Trigger: kit.Checkbox{AriaLabel: tr("tooltip.agreeLabel", "Accept the terms")},
				Content: Text(tr("tooltip.agree", "You accept the terms of service"))},
			kit.Tooltip{
				Trigger: kit.Switch{AriaLabel: tr("tooltip.airplaneLabel", "Airplane mode")},
				Content: Text(tr("tooltip.airplane", "Turns off every connection"))},
			kit.Tooltip{
				Trigger: kit.Slider{Class: "w-40", DefaultValue: new(40.0), AriaLabel: tr("tooltip.volumeLabel", "Volume")},
				Content: Text(tr("tooltip.volume", "Volume"))}),
		demo("Placements", "Placement and Align, with a flip when there is no room.", "",
			tipText(tr("tooltip.top", "Top"), tr("tooltip.onTop", "On the top"), kit.Tooltip{Placement: "top"}),
			tipText(tr("tooltip.right", "Right"), tr("tooltip.onRight", "On the right"), kit.Tooltip{Placement: "right"}),
			tipText(tr("tooltip.bottom", "Bottom"), tr("tooltip.onBottom", "On the bottom"), kit.Tooltip{Placement: "bottom"}),
			tipText(tr("tooltip.left", "Left"), tr("tooltip.onLeft", "On the left"), kit.Tooltip{Placement: "left"})),
		demo("Delay and rich content", "Delay makes the pointer rest first. Once one tooltip has opened the next opens at once. Content is any short component.", "",
			kit.Tooltip{Delay: 600 * time.Millisecond,
				Trigger: kit.Button{Variant: kit.Outline, Children: Text(tr("tooltip.wait", "Hover and wait"))},
				Content: Text(tr("tooltip.after", "Opens after 600 ms"))},
			kit.Tooltip{
				Trigger: kit.Button{Variant: kit.Outline, Icon: icons.Settings, Children: Text(tr("tooltip.settings", "Settings"))},
				Content: Div(Class("flex items-center gap-2"), Text(tr("tooltip.openSettings", "Open settings")),
					Span(Dir("ltr"), Class("rounded bg-background/20 px-1 font-mono text-[10px]"), Text("Ctrl ,")))}),
		demo("Disabled", "Disabled on the tooltip keeps it from opening; so does a disabled trigger (kit.Button{Disabled: true}): the trigger's own Disabled wins.", "",
			kit.Tooltip{Disabled: true, Trigger: kit.Button{Variant: kit.Outline, Children: Text(tr("tooltip.none", "No tooltip"))}, Content: Text(tr("tooltip.noSee", "You will not see this"))},
			kit.Tooltip{Trigger: kit.Button{Variant: kit.Outline, Disabled: true, Children: Text(tr("tooltip.disabledButton", "Disabled button"))}, Content: Text(tr("tooltip.noSee", "You will not see this"))}),
	)
}

func toastPage() tela.Node {
	tr := useTr()
	toast := kit.UseToast()
	upload := func() {
		id := toast.Loading(tr("toast.uploading", "Uploading photos..."))
		time.AfterFunc(1800*time.Millisecond, func() {
			toast.Success(tr("toast.uploaded", "3 photos uploaded"), kit.ToastReplace(id), kit.ToastDescription(tr("toast.album", "They are in the Summer album.")))
		})
	}
	return grid(
		demo("Kinds", "Toasts stack at a corner of the window; the newest is in front. The pointer over the stack fans it out and stops the timers. The viewport (\"Notifications\") and the close button (\"Close toast\") are named in the window's language: the tela.toast.region and tela.toast.close messages of the catalogs.", "",
			kit.Button{Variant: kit.Outline, OnClick: func() {
				toast.Show(tr("toast.event", "Event created"), kit.ToastDescription(tr("toast.eventAt", "Sunday, October 25 at 9:00 AM")))
			}, Children: Text(tr("toast.default", "Default"))},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Success(tr("toast.saved", "Changes saved")) }, Children: Text(tr("toast.success", "Success"))},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Info(tr("toast.version", "A new version is available")) }, Children: Text(tr("toast.info", "Info"))},
			kit.Button{Variant: kit.Outline, OnClick: func() { toast.Warning(tr("toast.trial", "Your trial ends in 3 days")) }, Children: Text(tr("toast.warning", "Warning"))},
			kit.Button{Variant: kit.Outline, OnClick: func() {
				toast.Error(tr("toast.failed", "The upload failed"), kit.ToastDescription(tr("toast.retry", "Check your connection and try again.")))
			}, Children: Text(tr("toast.error", "Error"))}),
		demo("Action", "ToastAction adds a button; clicking it runs your function and dismisses the toast.", "",
			kit.Button{Variant: kit.Outline, Children: Text(tr("toast.deleteDraft", "Delete the draft")), OnClick: func() {
				toast.Show(tr("toast.draftDeleted", "Draft deleted"), kit.ToastAction(tr("toast.undo", "Undo"), func() { toast.Success(tr("toast.restored", "Draft restored")) }))
			}}),
		demo("Loading, then done", "A Loading toast is replaced with ToastReplace; every method of the handle is safe from any goroutine.", "",
			kit.Button{Variant: kit.Outline, Icon: icons.Download, OnClick: upload, Children: Text(tr("toast.upload", "Upload photos"))}),
		demo("Forever, and dismissing", "ToastDuration(kit.ToastForever) keeps the toast until it is dismissed.", "",
			kit.Button{Variant: kit.Outline, Children: Text(tr("toast.sticky", "Sticky toast")), OnClick: func() {
				toast.Show(tr("toast.pinned", "Pinned until you close it"), kit.ToastDuration(kit.ToastForever))
			}},
			kit.Button{Variant: kit.Ghost, Children: Text(tr("toast.dismissAll", "Dismiss all")), OnClick: toast.DismissAll}),
	)
}
