package kit_test

import (
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func badgeTonesSheet() *sheet {
	c := func(name string, b kit.Badge) cell {
		b.Props = h.TestID(name + "-badge")
		return cell{name: name, probes: []string{name + "-badge"}, tela: b}
	}
	return &sheet{name: "badge_tones", slot: 112, cells: []cell{
		c("success", kit.Badge{Variant: kit.Success, Children: h.Text("Paid")}),
		c("warning", kit.Badge{Variant: kit.Warning, Children: h.Text("Pending")}),
		c("info", kit.Badge{Variant: kit.Info, Children: h.Text("Draft")}),
		c("success-icon", kit.Badge{Variant: kit.Success, Icon: iconCheck, Children: h.Text("Verified")}),
		c("warning-icon", kit.Badge{Variant: kit.Warning, Icon: iconCircleAlert, Children: h.Text("Review")}),
		c("dot-success", kit.Badge{Variant: kit.Success, Dot: true, Children: h.Text("Active")}),
		c("dot-warning", kit.Badge{Variant: kit.Warning, Dot: true, Children: h.Text("Pending")}),
		c("dot-info", kit.Badge{Variant: kit.Info, Dot: true, Children: h.Text("Syncing")}),
		c("dot-destructive", kit.Badge{Variant: kit.Destructive, Dot: true, Children: h.Text("Failed")}),
		c("dot-default", kit.Badge{Dot: true, Children: h.Text("Default")}),
		c("dot-secondary", kit.Badge{Variant: kit.Secondary, Dot: true, Children: h.Text("Idle")}),
		c("dot-outline", kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-emerald-500", Children: h.Text("Online")}),
		c("dot-outline-amber", kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-amber-500", Children: h.Text("Away")}),
		c("dot-icon", kit.Badge{Variant: kit.Info, Icon: iconCheck, Dot: true, Children: h.Text("Both")}),
	}}
}

func TestBadgeTonesGolden(t *testing.T) { badgeTonesSheet().golden(t) }

func TestBadgeDotIsDecorative(t *testing.T) {
	app := telatest.Mount(t, kit.Badge{Variant: kit.Success, Dot: true, Children: h.Text("Active")})
	app.ExpectAria(`- text: Active`)
}

// The dot is a 6 px disc and a 4 px gap before the label.
func TestBadgeDotTakesTenPixels(t *testing.T) {
	plain := telatest.Mount(t, kit.Badge{Children: h.Text("Active"), Props: h.TestID("b")})
	dot := telatest.Mount(t, kit.Badge{Dot: true, DotClass: "bg-emerald-500", Children: h.Text("Active"), Props: h.TestID("b")})
	w0, w1 := plain.Bounds(telatest.ByTestID("b")).Width, dot.Bounds(telatest.ByTestID("b")).Width
	if w1-w0 < 9.99 || w1-w0 > 10.01 {
		t.Errorf("the dot adds %v px, want 10", w1-w0)
	}
}

func TestBadgeToneIsChecked(t *testing.T) {
	// The tones are variants of the recipe like the others: an app can
	// redefine them.
	s := kit.DefaultStyle()
	s.Badge.Variants[kit.Success] = "bg-lime-500 text-black"
	app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: kit.Badge{Variant: kit.Success, Children: h.Text("ok")}})
	app.ExpectCount(telatest.ByClass("bg-lime-500"), 1)
}
