package telatest_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// settingsProbe shows the system settings through the hooks, and a box
// whose size and colors follow them through classes.
func settingsProbe() tela.Node {
	return tela.C(func() tela.Node {
		accent, hasAccent := tela.UseAccentColor()
		return Div(Class("p-2"),
			Textf("scheme=%v motion=%v contrast=%v scale=%g accent=%v/%v",
				tela.UseColorScheme(), tela.UseReducedMotion(), tela.UseHighContrast(), tela.UseTextScale(), hasAccent, accent),
			Div(TestID("box"), Class("w-16 h-4 bg-white dark:bg-black")),
		)
	})
}

func TestSettersChangeTheSystemWhileMounted(t *testing.T) {
	app := telatest.Mount(t, settingsProbe(), telatest.Size(480, 120))
	app.ExpectText("scheme=Light motion=false contrast=false scale=1 accent=false/")

	app.SetDarkMode(true)
	app.ExpectText("scheme=Dark motion=false")
	r := app.Bounds(telatest.ByTestID("box"))
	if c := app.PixelAt(r.X+2, r.Y+2); c.R != 0 || c.G != 0 || c.B != 0 {
		t.Errorf("the box is %v in dark mode, want black from dark:bg-black", c)
	}
	app.SetDarkMode(false)
	app.ExpectText("scheme=Light")
	if c := app.PixelAt(r.X+2, r.Y+2); c.R != 255 || c.G != 255 || c.B != 255 {
		t.Errorf("the box is %v in light mode, want white", c)
	}

	app.SetReducedMotion(true)
	app.ExpectText("motion=true")
	app.SetReducedMotion(false)
	app.ExpectText("motion=false")

	app.SetHighContrast(true)
	app.ExpectText("contrast=true")
	app.SetHighContrast(false)
	app.ExpectText("contrast=false")

	// 4rem box: 64 px at the default scale, then scaled.
	if w := app.Bounds(telatest.ByTestID("box")).Width; w != 64 {
		t.Fatalf("the box is %g wide at scale 1, want 64", w)
	}
	app.SetTextScale(1.5)
	app.ExpectText("scale=1.5")
	if w := app.Bounds(telatest.ByTestID("box")).Width; w != 96 {
		t.Errorf("the box is %g wide at scale 1.5, want 96", w)
	}
	app.SetTextScale(1)
	if w := app.Bounds(telatest.ByTestID("box")).Width; w != 64 {
		t.Errorf("the box is %g wide at scale 1 again, want 64", w)
	}

	app.SetAccentColor(tela.RGB(0x12, 0x34, 0x56))
	app.ExpectText("accent=true/")
	app.ClearAccentColor()
	app.ExpectText("accent=false/")
}

// The setters change one setting and keep the others, including the ones
// the mount options gave.
func TestSettersKeepTheOtherSettings(t *testing.T) {
	app := telatest.Mount(t, settingsProbe(), telatest.Size(480, 120), telatest.Dark(), telatest.TextScale(1.25))
	app.ExpectText("scheme=Dark motion=false contrast=false scale=1.25")
	app.SetHighContrast(true)
	app.ExpectText("scheme=Dark motion=false contrast=true scale=1.25")
	app.SetTextScale(2)
	app.ExpectText(fmt.Sprintf("scheme=Dark motion=false contrast=true scale=%g", 2.0))
}

func TestSetTextScaleChecked(t *testing.T) {
	for _, scale := range []float64{0, -1, 5} {
		mustFail(t, func(tb testing.TB) {
			app := telatest.Mount(tb, settingsProbe(), telatest.Size(200, 100))
			app.SetTextScale(scale)
		}, "SetTextScale", "above 0 and at most 4")
	}
}
