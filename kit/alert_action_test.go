package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func alertActionSheet() *sheet {
	c := func(name string, a kit.Alert) cell {
		a.Props = h.TestID(name + "-alert")
		return cell{name: name, width: 380, probes: []string{name + "-alert"}, tela: a}
	}
	return &sheet{name: "alert_action", width: 880, cells: []cell{
		c("default", kit.Alert{Icon: iconInfo, Title: h.Text("Update available"), Description: h.Text("Restart to install version 2.4."),
			Action: kit.Button{Size: kit.Xs, Variant: kit.Outline, Children: h.Text("Restart")}}),
		c("destructive", kit.Alert{Variant: kit.Destructive, Icon: iconCircleAlert, Title: h.Text("Upload failed"),
			Description: h.Text("The file is larger than 10 MB."),
			Action:      kit.Button{Size: kit.Xs, Variant: kit.Outline, Children: h.Text("Retry")}}),
		c("dismiss", kit.Alert{Variant: kit.Info, Icon: iconInfo, Title: h.Text("Scheduled maintenance"),
			Description: h.Text("The service is read-only on Sunday from 02:00 to 03:00 UTC."),
			Action:      kit.Button{Size: kit.IconXs, Variant: kit.Ghost, Icon: iconX, AriaLabel: "Dismiss"}}),
		c("title-only", kit.Alert{Title: h.Text("A new version is available"),
			Action: kit.Button{Size: kit.Xs, Children: h.Text("Update")}}),
		// Tela's own (ADR-263): the room follows an action wider than
		// shadcn's pe-18; no Chromium cell.
		c("two-buttons", kit.Alert{Variant: kit.Warning, Icon: iconInfo, Title: h.Text("Unsaved changes"),
			Description: h.Text("You have edits that were not saved to the server yet."),
			Action: tela.C(func() tela.Node {
				return h.Div(h.Class("flex gap-2"),
					kit.Button{Size: kit.Xs, Variant: kit.Outline, Children: h.Text("Discard")},
					kit.Button{Size: kit.Xs, Children: h.Text("Save")})
			})}),
	}}
}

func TestAlertActionGolden(t *testing.T) { alertActionSheet().golden(t) }

func TestAlertActionRTLGolden(t *testing.T) {
	s := alertActionSheet()
	s.name, s.rtl = "alert_action_rtl", true
	s.cells = s.cells[:3]
	s.golden(t)
}

// The action sits in the top end corner, the texts leave room for it, and
// it takes presses.
func TestAlertAction(t *testing.T) {
	var pressed int
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-96"), kit.Alert{Title: h.Text("Update available"),
			Description: h.Text("A long description that would run under the action if the alert did not keep room for it."),
			Props:       h.TestID("alert"),
			Action:      kit.Button{Size: kit.Xs, Children: h.Text("Restart"), OnClick: func() { pressed++ }, Props: h.TestID("action")}})
	}, telatest.Size(400, 200))
	root, act := app.Bounds(telatest.ByTestID("alert")), app.Bounds(telatest.ByTestID("action"))
	if got := root.X + root.Width - (act.X + act.Width); got != 13 {
		t.Errorf("the action is %v px from the end, want 13 (end-3 inside the 1 px border)", got)
	}
	if got := act.Y - root.Y; got != 11 {
		t.Errorf("the action is %v px from the top, want 11 (top-2.5 inside the border)", got)
	}
	title := app.Bounds(telatest.ByText("Update available"))
	if title.X+title.Width > act.X {
		t.Errorf("the title ends at %v, under the action that starts at %v", title.X+title.Width, act.X)
	}
	desc := app.Bounds(telatest.ByText("A long description that would run under the action if the alert did not keep room for it."))
	if desc.X+desc.Width > root.X+root.Width-72 {
		t.Errorf("the description ends at %v, inside the 72 px kept for the action", desc.X+desc.Width)
	}
	app.Click(telatest.ByRole("button", "Restart"))
	if pressed != 1 {
		t.Errorf("the action was pressed %d times, want 1", pressed)
	}
	// Without an action the alert keeps its padding.
	app = telatest.Mount(t, h.Div(h.Class("w-96"), kit.Alert{Title: h.Text("x"), Props: h.TestID("plain")}))
	app.ExpectCount(telatest.ByClass("pe-18"), 0)
}

func TestAlertActionMirrors(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Dir("rtl"), h.Class("w-96"), kit.Alert{Title: h.Text("Update"), Props: h.TestID("alert"),
			Action: kit.Button{Size: kit.Xs, Children: h.Text("Restart"), Props: h.TestID("action")}})
	}, telatest.Size(400, 100))
	root, act := app.Bounds(telatest.ByTestID("alert")), app.Bounds(telatest.ByTestID("action"))
	if got := act.X - root.X; got != 13 {
		t.Errorf("under rtl the action is %v px from the left edge, want 13 (end-3 inside the 1 px border)", got)
	}
}
