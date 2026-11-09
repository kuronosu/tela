package win32

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Only the buttons the app translated and the box has are renamed; the
// others keep the labels of the user's Windows.
func TestMessageBoxLabels(t *testing.T) {
	spanish := platform.Strings{
		platform.StringMessageOK: "Aceptar", platform.StringMessageCancel: "Cancelar",
		platform.StringMessageYes: "Sí", platform.StringMenuQuit: "Salir de {name}",
	}
	for _, c := range []struct {
		b    platform.MessageButtons
		s    platform.Strings
		want []messageBoxLabel
	}{
		{platform.ButtonsOK, nil, nil},
		{platform.ButtonsOK, spanish, []messageBoxLabel{{idOK, "Aceptar"}}},
		{platform.ButtonsOKCancel, spanish, []messageBoxLabel{{idOK, "Aceptar"}, {idCancel, "Cancelar"}}},
		// No has no Spanish text here: it keeps Windows' own.
		{platform.ButtonsYesNo, spanish, []messageBoxLabel{{idYes, "Sí"}}},
		{platform.ButtonsYesNoCancel, spanish, []messageBoxLabel{{idCancel, "Cancelar"}, {idYes, "Sí"}}},
		{platform.ButtonsYesNo, platform.Strings{platform.StringMessageNo: ""}, nil},
	} {
		got := messageBoxLabels(c.b, c.s)
		if len(got) != len(c.want) {
			t.Errorf("messageBoxLabels(%d, ...) = %v, want %v", c.b, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("messageBoxLabels(%d, ...) = %v, want %v", c.b, got, c.want)
			}
		}
	}
}
