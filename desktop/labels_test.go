package desktop_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// useLabels installs a label source with the given texts for the test; a
// test swaps it to change the language at run time.
func useLabels(t *testing.T, texts map[string]string) {
	t.Helper()
	tela.SetLabelSource(func(id string, _ ...any) (string, bool) { s, ok := texts[id]; return s, ok })
	t.Cleanup(func() { tela.SetLabelSource(nil) })
}

var spanish = map[string]string{
	"tela.messagebox.ok": "Aceptar", "tela.messagebox.cancel": "Cancelar", "tela.messagebox.yes": "Sí", "tela.messagebox.no": "No",
	"tela.messagebox.question": "Pregunta", "tela.messagebox.warning": "Aviso",
	"tela.menu.edit": "Edición", "tela.menu.copy": "Copiar", "tela.menu.minimize": "Minimizar",
}

func messageApp(t *testing.T, m desktop.Message) *telatest.App {
	return telatest.MountFunc(t, func() tela.Node {
		answer, setAnswer := tela.UseState("none")
		return Div(Text("answer: "+answer), Button(OnClick(func() {
			if err := desktop.ShowMessage(m, func(a desktop.Answer) { setAnswer.Set(a.String()) }); err != nil {
				setAnswer.Set("error " + err.Error())
			}
		}), Text("Ask")))
	})
}

// The drawn message box takes its buttons and its default title from the
// built-in labels, and the answers still reach the app.
func TestDrawnMessageBoxLabels(t *testing.T) {
	useLabels(t, spanish)
	app := messageApp(t, desktop.Message{Kind: desktop.MessageQuestion, Text: "¿Guardar?", Buttons: desktop.ButtonsYesNoCancel})
	app.Unsupported(telatest.MessageBoxes)
	app.Click(telatest.ByRole("button", "Ask"))
	boxes := app.MessageBoxes()
	if len(boxes) != 1 || boxes[0].Title != "Pregunta" || !boxes[0].Drawn || boxes[0].Buttons != desktop.ButtonsYesNoCancel || boxes[0].Kind != desktop.MessageQuestion {
		t.Fatalf("message boxes %+v, want one drawn question titled Pregunta with three buttons", boxes)
	}
	app.AnswerMessage(desktop.AnswerYes) // clicks the button named "Sí"
	app.ExpectText("answer: yes")
}

// A language change while the drawn box is open renames its buttons.
func TestDrawnMessageBoxFollowsTheLanguage(t *testing.T) {
	useLabels(t, spanish)
	app := messageApp(t, desktop.Message{Text: "Hecho", Buttons: desktop.ButtonsOKCancel})
	app.Unsupported(telatest.MessageBoxes)
	app.Click(telatest.ByRole("button", "Ask"))
	if boxes := app.MessageBoxes(); len(boxes) != 1 || boxes[0].Buttons != desktop.ButtonsOKCancel {
		t.Fatalf("message boxes %+v", boxes)
	}
	useLabels(t, map[string]string{"tela.messagebox.ok": "D'accord", "tela.messagebox.cancel": "Annuler"})
	tela.LabelsChanged()
	boxes := app.MessageBoxes()
	if len(boxes) != 1 || boxes[0].Buttons != desktop.ButtonsOKCancel {
		t.Fatalf("message boxes after the change %+v, want the same set under the new names", boxes)
	}
	app.AnswerMessage(desktop.AnswerOK)
	app.ExpectText("answer: ok")
}

// A native box gets the translated default title, and the backend is told
// the button texts (the headless backend records them).
func TestNativeMessageBoxLabels(t *testing.T) {
	useLabels(t, spanish)
	app := messageApp(t, desktop.Message{Kind: desktop.MessageWarning, Text: "Cuidado", Buttons: desktop.ButtonsOKCancel})
	app.Click(telatest.ByRole("button", "Ask"))
	boxes := app.MessageBoxes()
	if len(boxes) != 1 || boxes[0].Title != "Aviso" || boxes[0].Drawn {
		t.Fatalf("message boxes %+v, want one native box titled Aviso", boxes)
	}
	if got := app.BackendStrings()["tela.messagebox.cancel"]; got != "Cancelar" {
		t.Errorf("the backend's strings %v, want the Spanish button texts", app.BackendStrings())
	}
}

// Role items without a label take the role's built-in label, and the
// menu bar is given again when the language changes.
func TestRoleLabelsFollowTheLanguage(t *testing.T) {
	useLabels(t, spanish)
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(tela.Component(desktop.AppMenuBar{Menus: []desktop.MenuItem{
			{Role: desktop.RoleEditMenu, Submenu: []desktop.MenuItem{{Role: desktop.RoleCopy}, {Role: desktop.RoleMinimize}}},
		}}))
	})
	bar := app.AppMenuBar()
	if len(bar) != 1 || bar[0].Label != "Edición" || bar[0].Submenu[0].Label != "Copiar" || bar[0].Submenu[1].Label != "Minimizar" {
		t.Fatalf("menu bar %+v, want the Spanish role labels", bar)
	}
	useLabels(t, map[string]string{"tela.menu.edit": "Édition", "tela.menu.copy": "Copier"})
	tela.LabelsChanged()
	bar = app.AppMenuBar()
	if len(bar) != 1 || bar[0].Label != "Édition" || bar[0].Submenu[0].Label != "Copier" || bar[0].Submenu[1].Label != "Minimize" {
		t.Fatalf("menu bar after the change %+v", bar)
	}
	if got := desktop.RoleSelectAll.Label(); got != "Select All" {
		t.Errorf("a role without a translation: %q, want the English label", got)
	}
}
