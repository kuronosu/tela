package telatest_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The title bar colors and the backdrop of the App (MountApp) and of a Window
// reach the fake window manager, and a changed Window field updates it.
func TestWindowStyle(t *testing.T) {
	navy := tela.Hex("#1e1b4b")
	root := func() tela.Node {
		acrylic, setAcrylic := tela.UseState(false)
		backdrop := tela.BackdropMica
		if acrylic {
			backdrop = tela.BackdropAcrylic
		}
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setAcrylic.Set(true) }), Text("Acrylic")),
			tela.Component(tela.Window{
				Title:    "Child",
				TitleBar: tela.TitleBarColors{Text: tela.White},
				Backdrop: backdrop,
				OnClose:  func() {},
				Root:     tela.C(func() tela.Node { return Div(Text("child")) }),
			}),
		)
	}
	app := telatest.MountApp(t, tela.App{
		Root:     tela.C(root),
		TitleBar: tela.TitleBarColors{Background: navy},
		Backdrop: tela.BackdropMicaAlt,
	})
	if got := app.TitleBar(); got.Background != navy || got.Text != (tela.Color{}) || got.Border != (tela.Color{}) {
		t.Errorf("main window title bar = %+v, want only Background %v", got, navy)
	}
	if got := app.Backdrop(); got != tela.BackdropMicaAlt {
		t.Errorf("main window backdrop = %v, want MicaAlt", got)
	}
	child := app.Window("Child")
	if got := child.TitleBar().Text; got != tela.White {
		t.Errorf("child title bar text = %v, want white", got)
	}
	if got := child.Backdrop(); got != tela.BackdropMica {
		t.Errorf("child backdrop = %v, want Mica", got)
	}
	app.Click(telatest.ByRole("button", "Acrylic"))
	if got := child.Backdrop(); got != tela.BackdropAcrylic {
		t.Errorf("child backdrop after the change = %v, want Acrylic", got)
	}
}

// Options set them by hand for Mount.
func TestWindowStyleOptions(t *testing.T) {
	app := telatest.Mount(t, Div(Text("x")),
		telatest.TitleBar(tela.TitleBarColors{Border: tela.Hex("#ff0000")}), telatest.Backdrop(tela.BackdropAcrylic))
	if app.TitleBar().Border != tela.Hex("#ff0000") || app.Backdrop() != tela.BackdropAcrylic {
		t.Errorf("title bar %+v, backdrop %v", app.TitleBar(), app.Backdrop())
	}
}
