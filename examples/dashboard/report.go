package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// ReportDialog is the report dialog, a kit.Dialog opened by state: by the
// palette's New report ("new") and by the weekly report's Edit button
// ("weekly"). Mode is "" while it is closed.
type ReportDialog struct {
	Mode      string
	SetMode   tela.Setter[string]
	Weekly    bool
	SetWeekly tela.Setter[bool]
}

func (p ReportDialog) Build() tela.Node {
	open := p.Mode != ""
	// The dialog keeps its last mode while it leaves.
	last := tela.UseRef("new")
	if open {
		last.Current = p.Mode
	}
	mode := last.Current
	title, desc := "New report", "Pick what to include and how to deliver it."
	if mode == "weekly" {
		title, desc = "Weekly report", "What goes out to the team every Monday at 9:00."
	}
	return kit.Dialog{
		Open: &open, OnOpenChange: func(o bool) {
			if !o {
				p.SetMode.Set("")
			}
		},
		Title: Text(title), Description: Text(desc),
		// The dialog mounts its content when it opens, so the form starts
		// from the defaults each time.
		Children: ReportForm{Mode: mode, Close: p.SetMode, Weekly: p.Weekly, SetWeekly: p.SetWeekly},
	}
}

var formats = []string{"PDF", "CSV", "XLSX"}

// ReportForm is the content of the dialog: a kit.Field with a kit.Input, a
// segmented control of formats (a kit.ToggleGroup with a sliding pill), a
// kit.Switch and the buttons.
type ReportForm struct {
	Mode      string
	Close     tela.Setter[string]
	Weekly    bool
	SetWeekly tela.Setter[bool]
}

func (p ReportForm) Build() tela.Node {
	toast := kit.UseToast()
	s := kit.UseStyle()
	name, setName := tela.UseState("Q3 revenue summary")
	format, setFormat := tela.UseState("PDF")
	weekly, setWeekly := tela.UseState(p.Weekly || p.Mode == "new")
	weeklyID := tela.UseID()
	submit := func() {
		p.Close.Set("")
		p.SetWeekly.Set(weekly)
		if p.Mode == "weekly" {
			toast.Success("Weekly report saved", kit.ToastDescription(name+", "+format))
		} else {
			toast.Success("Report queued: " + name)
		}
	}
	action := "Create report"
	if p.Mode == "weekly" {
		action = "Save"
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
		Div(Class(s.Dialog.Footer),
			kit.Button{Type: "button", Variant: kit.Outline, OnClick: func() { p.Close.Set("") }, Children: Text("Cancel")},
			kit.Button{Type: "submit", Children: Text(action)},
		),
	)
}

// ShowToast shows a toast when it mounts: the Toast preview starts the
// page with one. A pinned toast stays until it is dismissed (a snapshot
// has no clock to count down).
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
