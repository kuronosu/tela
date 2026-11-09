package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// iconTerminal and iconTriangleAlert are Lucide's (ISC, THIRD_PARTY.md).
var (
	iconTerminal      = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 19h8"/><path d="m4 17 6-6-6-6"/></svg>`}
	iconInfo          = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>`}
	iconTriangleAlert = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>`}
	iconCircleCheck   = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></svg>`}
)

func alertSheet() *sheet {
	cell1 := func(name string, a kit.Alert) cell {
		a.Props = h.TestID(name + "-alert")
		return cell{name: name, width: 380, probes: []string{name + "-alert"}, tela: a}
	}
	return &sheet{name: "alert", width: 880, cells: []cell{
		cell1("default", kit.Alert{Icon: iconTerminal, Title: h.Text("Heads up!"),
			Description: h.Text("You can add components to your app using the cli.")}),
		cell1("destructive", kit.Alert{Variant: kit.Destructive, Icon: iconCircleAlert, Title: h.Text("Unable to process your payment."),
			Description: h.Text("Please verify your billing information and try again.")}),
		cell1("no-icon", kit.Alert{Title: h.Text("Your session has expired"), Description: h.Text("Log in again to keep working.")}),
		cell1("title-only", kit.Alert{Icon: iconInfo, Title: h.Text("A new version is available")}),
		cell1("description-only", kit.Alert{Icon: iconInfo, Description: h.Text("This account was created with a different sign-in method.")}),
		cell1("wrapped", kit.Alert{Variant: kit.Destructive, Icon: iconCircleAlert, Title: h.Text("Unable to process your payment."),
			Description: h.Div(h.Class("flex flex-col gap-1"),
				h.P(h.Text("Please verify your billing information and try again.")),
				h.Div(h.Class("flex flex-col text-sm"), h.Div(h.Text("Check your card details")), h.Div(h.Text("Ensure sufficient funds"))))}),
		cell1("success", kit.Alert{Variant: kit.Success, Icon: iconCircleCheck, Title: h.Text("Payment received"),
			Description: h.Text("A receipt is on its way to your inbox.")}),
		cell1("warning", kit.Alert{Variant: kit.Warning, Icon: iconTriangleAlert, Title: h.Text("Your trial ends in 3 days"),
			Description: h.Text("Add a payment method to keep your projects.")}),
		cell1("info", kit.Alert{Variant: kit.Info, Icon: iconInfo, Title: h.Text("Scheduled maintenance"),
			Description: h.Text("The service is read-only on Sunday from 02:00 to 03:00 UTC.")}),
	}}
}

func TestAlertGolden(t *testing.T) { alertSheet().golden(t) }

func rtlAlertSheet() *sheet {
	s := alertSheet()
	s.name = "alert_rtl"
	s.rtl = true
	s.cells = s.cells[:2]
	return s
}

func TestAlertRTLGolden(t *testing.T) { rtlAlertSheet().golden(t) }

func TestAlertIsAnAlert(t *testing.T) {
	app := telatest.Mount(t, kit.Alert{Icon: iconTerminal, Title: h.Text("Heads up!"), Description: h.Text("Add components with the cli.")})
	app.ExpectAria(`- alert: Heads up! Add components with the cli.`)
}

func TestAlertRole(t *testing.T) {
	for _, role := range []string{"status", "note"} {
		app := telatest.Mount(t, kit.Alert{Role: role, Title: h.Text("Saved")})
		app.ExpectCount(telatest.ByRole(role, ""), 1)
		app.ExpectCount(telatest.ByRole("alert", ""), 0)
	}
}

func TestAlertRoleIsChecked(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("a role the alert does not support did not panic in dev mode")
		}
	}()
	telatest.Mount(t, kit.Alert{Role: "banner", Title: h.Text("x")})
}

// The icon column has no width without an icon, and the texts line up with
// the title either way.
func TestAlertTextColumn(t *testing.T) {
	with := telatest.Mount(t, h.Div(h.Class("w-96"), kit.Alert{Icon: iconTerminal, Title: h.Text("Heads up!"), Props: h.TestID("a")}))
	without := telatest.Mount(t, h.Div(h.Class("w-96"), kit.Alert{Title: h.Text("Heads up!"), Props: h.TestID("a")}))
	if w, wo := with.Bounds(telatest.ByText("Heads up!")), without.Bounds(telatest.ByText("Heads up!")); w.X-wo.X < 27.5 || w.X-wo.X > 28.5 {
		t.Errorf("the icon column pushes the title by %v px, want 28 (the 16 px icon column and the 12 px gap)", w.X-wo.X)
	}
}
