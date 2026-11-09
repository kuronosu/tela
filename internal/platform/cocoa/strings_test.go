package cocoa

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// spanish is a set of the app's texts (Desktop.SetStrings, ADR-251).
var spanish = platform.Strings{
	platform.StringMessageOK: "Aceptar", platform.StringMessageCancel: "Cancelar",
	platform.StringMessageYes: "Sí", platform.StringMessageNo: "No guardar",
	platform.StringMenuAbout: "Acerca de {name}", platform.StringMenuServices: "Servicios",
	platform.StringMenuHide: "Ocultar {name}", platform.StringMenuHideOthers: "Ocultar otros",
	platform.StringMenuShowAll: "Mostrar todo", platform.StringMenuQuit: "Salir de {name}",
	platform.StringMenuEdit: "Edición", platform.StringMenuWindow: "Ventana",
	platform.StringMenuUndo: "Deshacer", platform.StringMenuCopy: "Copiar",
	platform.StringMenuMinimize: "Minimizar", platform.StringMenuClose: "Cerrar ventana",
	platform.StringMenuBringAllToFront: "Traer todo al frente",
}

func titles(entries []menuEntry) []string {
	var out []string
	for _, e := range entries {
		if e.Separator {
			out = append(out, "---")
			continue
		}
		out = append(out, e.Title)
		if len(e.Submenu) > 0 {
			out = append(out, "["+strings.Join(titles(e.Submenu), "|")+"]")
		}
	}
	return out
}

// The default bar, the app menu and the standard menus take the app's
// texts, and the names fill {name}; texts the app lacks stay English.
func TestDefaultMenuBarPlanStrings(t *testing.T) {
	got := strings.Join(titles(defaultMenuBarPlan("Demo", spanish)), " ")
	for _, want := range []string{
		"Acerca de Demo", "Servicios", "Ocultar Demo", "Ocultar otros", "Mostrar todo", "Salir de Demo",
		"Edición", "Deshacer", "Copiar", "Ventana", "Minimizar", "Traer todo al frente",
		"Redo", "Select All", "Zoom", // not translated: English
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the bar lacks %q:\n%s", want, got)
		}
	}
	for _, old := range []string{"About Demo", "Quit Demo", "Undo", "Bring All to Front"} {
		if strings.Contains(got, old) {
			t.Errorf("the bar still has %q:\n%s", old, got)
		}
	}
}

// Role items given without a label take the app's text, and those with
// one keep theirs.
func TestRolePlanStrings(t *testing.T) {
	menu := []platform.MenuItem{{Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{
		{ID: 1, Role: platform.RoleCopy}, {ID: 2, Role: platform.RoleCut, Label: "Cortar!"}, {ID: 3, Role: platform.RoleClose},
	}}}
	got := strings.Join(titles(appMenuBarPlan("Demo", menu, nil, spanish)[1:]), " ")
	want := "Edición [Copiar|Cortar!|Cerrar ventana]"
	if got != want {
		t.Errorf("plan titles %q, want %q", got, want)
	}
	// Without strings the labels are Electron's, "Close Window" included.
	got = strings.Join(titles(popupPlan([]platform.MenuItem{{ID: 1, Role: platform.RoleClose}}, nil)), " ")
	if got != "Close Window" {
		t.Errorf("popup role label %q, want the English default", got)
	}
}

func TestAlertButtonStrings(t *testing.T) {
	var titles []string
	for _, bt := range alertButtons(platform.ButtonsYesNoCancel, spanish) {
		s := bt.title
		if bt.escape {
			s += "(esc)"
		}
		titles = append(titles, s)
	}
	if got, want := strings.Join(titles, ", "), "Sí, Cancelar(esc), No guardar"; got != want {
		t.Errorf("buttons %q, want %q (Escape stays on Cancel whatever its title)", got, want)
	}
	if got := alertButtons(platform.ButtonsOK, platform.Strings{})[0].title; got != "OK" {
		t.Errorf("OK without strings is %q", got)
	}
}
