package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// stateApp mounts a component that holds a ColorState and shows its color.
func stateApp(t *testing.T, o aria.ColorStateOptions) (*telatest.App, **aria.ColorState) {
	t.Helper()
	var st *aria.ColorState
	app := telatest.MountFunc(t, func() tela.Node {
		st = aria.UseColorState(o)
		return Span(Text("color " + aria.FormatHexColor(st.Color(), true)))
	}, telatest.Size(200, 50))
	return app, &st
}

func TestColorStateSetKeepsTheHue(t *testing.T) {
	var changes events
	app, st := stateApp(t, aria.ColorStateOptions{
		DefaultValue: new(aria.HSB{H: 200, S: 100, B: 100, A: 1}.Color()),
		OnChange:     func(c tela.Color) { changes.add(aria.FormatHexColor(c, true)) },
	})
	app.ExpectText("color #00aaffff")
	(*st).Set(tela.Hex("#808080")) // a gray has no hue of its own
	app.ExpectText("color #808080ff")
	if h := (*st).HSB(); !closeTo(h.H, 200, 1e-3) || h.S != 0 {
		t.Errorf("after a gray: %+v, want the hue 200", h)
	}
	(*st).Set(tela.Black)
	app.ExpectText("color #000000ff")
	if h := (*st).HSB(); !closeTo(h.H, 200, 1e-3) || h.B != 0 {
		t.Errorf("after black: %+v", h)
	}
	// Setting the color that is held changes nothing.
	changes.take()
	(*st).Set(tela.Black)
	(*st).SetHSB((*st).HSB())
	if got := changes.take(); got != "" {
		t.Errorf("OnChange for the same color: %s", got)
	}
	// A color with its own hue replaces it.
	(*st).Set(tela.Hex("#ff0000"))
	app.ExpectText("color #ff0000ff")
	if h := (*st).HSB(); h.H != 0 || h.S != 100 {
		t.Errorf("after red: %+v", h)
	}
	if got := changes.take(); got != "#ff0000ff" {
		t.Errorf("OnChange: %q", got)
	}
}

func TestColorStateSetHSBClamps(t *testing.T) {
	app, st := stateApp(t, aria.ColorStateOptions{DefaultValue: new(tela.Black)})
	(*st).SetHSB(aria.HSB{H: 999, S: -5, B: 500, A: 7})
	app.ExpectText("color #ffffffff")
	if h := (*st).HSB(); h.H != 360 || h.S != 0 || h.B != 100 || h.A != 1 {
		t.Errorf("%+v", h)
	}
}

// A controlled caller that stores the color it was told keeps the hue; a
// color of its own is converted.
func TestColorStateControlledKeepsTheHue(t *testing.T) {
	var st *aria.ColorState
	var set tela.Setter[tela.Color]
	app := telatest.MountFunc(t, func() tela.Node {
		c, s := tela.UseState(aria.HSB{H: 200, S: 100, B: 100, A: 1}.Color())
		set = s
		st = aria.UseColorState(aria.ColorStateOptions{Value: &c, OnChange: s.Set})
		return Span(Text("color " + aria.FormatHexColor(st.Color(), true)))
	}, telatest.Size(200, 50))
	st.SetHSB(aria.HSB{H: 200, S: 0, B: 80, A: 1})
	app.ExpectText("color #ccccccff")
	if h := st.HSB(); !closeTo(h.H, 200, 1e-3) {
		t.Errorf("the hue of the stored gray is %g, want 200", h.H)
	}
	// The caller sets a gray of its own: the hue it was shown with stays.
	set.Set(tela.Hex("#666666"))
	app.ExpectText("color #666666ff")
	if h := st.HSB(); !closeTo(h.H, 200, 1e-3) || h.S != 0 {
		t.Errorf("after the caller's gray: %+v", h)
	}
	set.Set(tela.Hex("#00ff00"))
	app.ExpectText("color #00ff00ff")
	if h := st.HSB(); h.H != 120 {
		t.Errorf("after the caller's green: %+v", h)
	}
}

func TestColorStateStartsFromTheColor(t *testing.T) {
	_, st := stateApp(t, aria.ColorStateOptions{DefaultValue: new(tela.Hex("#336699").WithAlpha(0.25))})
	h := (*st).HSB()
	if !closeTo(h.H, 210, 0.01) || !closeTo(h.S, 66.667, 0.01) || !closeTo(h.B, 60, 0.01) || h.A != 0.25 {
		t.Errorf("%+v", h)
	}
	if got := aria.FormatHexColor((*st).Color(), true); got != "#33669940" {
		t.Errorf("Color() = %s", got)
	}
	if (*st).LiveColor() != (*st).Color() {
		t.Error("LiveColor and Color differ with no drag")
	}
}
