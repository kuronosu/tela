package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The dialog and the toasts are the kit's: kit.Dialog (opened by the New
// report button in Hero) and kit.Toaster (around the page, see Page), both
// in the glass style (glass.go). This file holds the dialog's form and the
// component that shows a toast when the page starts with one.

var formats = []string{"PDF", "CSV", "XLSX"}

// ReportForm is the content of the dialog: a form of a kit.Field with a
// kit.Input, a segmented control of formats (a kit.ToggleGroup with a
// sliding pill), a kit.Switch and the buttons of the dialog's footer.
// Close is the dialog's state, to close it.
type ReportForm struct {
	Close tela.Setter[bool]
}

func (p ReportForm) Build() tela.Node {
	toast := kit.UseToast()
	style := kit.UseStyle()
	name, setName := tela.UseState("Q3 revenue summary")
	format, setFormat := tela.UseState("PDF")
	weekly, setWeekly := tela.UseState(true)
	weeklyID := tela.UseID()
	submit := func() {
		p.Close.Set(false)
		toast.Success("Report queued: " + name)
	}
	items := make([]kit.ToggleGroupItem[string], len(formats))
	for i, f := range formats {
		items[i] = kit.ToggleGroupItem[string]{Value: f, Children: Segment{Label: f, Pill: "format-pill", On: f == format}}
	}
	return Form(Class("flex flex-col gap-5"), OnSubmit(submit),
		kit.Field{Label: "Name", Children: kit.Input{Value: &name, OnValueChange: setName.Set, AutoFocus: true, Placeholder: "Report name"}},
		Div(Class("flex flex-col gap-3"),
			Span(Class("text-sm leading-none font-medium"), Text("Format")),
			kit.ToggleGroup[string]{Items: items, Value: &format, OnValueChange: setFormat.Set, DisallowEmptySelection: true,
				AriaLabel: "Format", Class: "w-full", ItemClass: "flex-1"},
		),
		Div(Class("flex items-center justify-between"),
			kit.Label{For: weeklyID, Children: Text("Send every Monday")},
			kit.Switch{ID: weeklyID, Checked: &weekly, OnCheckedChange: setWeekly.Set},
		),
		Div(Class(style.Dialog.Footer),
			kit.Button{Type: "button", Variant: "glass", OnClick: func() { p.Close.Set(false) }, Children: Text("Cancel")},
			kit.Button{Type: "submit", Children: Text("Create report")},
		),
	)
}

// ShowToast shows a toast when it mounts: the page starts with one in the
// Toast preview. A Pinned toast stays until it is dismissed (a snapshot has
// no clock to count down).
type ShowToast struct {
	Message string
	Pinned  bool
}

func (p ShowToast) Build() tela.Node {
	toast := kit.UseToast()
	tela.UseMount(func() func() {
		var opts []kit.ToastOption
		if p.Pinned {
			opts = append(opts, kit.ToastDuration(kit.ToastForever))
		}
		toast.Success(p.Message, opts...)
		return nil
	})
	return Fragment()
}
