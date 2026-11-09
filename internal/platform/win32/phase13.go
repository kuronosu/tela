//go:build windows

package win32

import (
	"maps"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Phase-13 requests and services (ADR-209, ADR-210): OLE drag and drop
// (dnd_windows.go, comserver_windows.go), the clipboard formats
// (dataobject_windows.go, dataformat.go), menu bars and popup menus
// (menu_windows.go), and the app-level services: global shortcuts, secrets
// and single instance (appservices_windows.go). Registration of schemes and
// file types needs no window: package assoc.

// ActivateWithToken activates the window. Windows has no tokens: the
// second process allowed this one to take the foreground
// (AllowSetForegroundWindow).
func (w *Window) ActivateWithToken(string) { w.Activate() }

// SetAppID checks id and makes it the app's AppUserModelID (ADR-217,
// ADR-235): the process's explicit AUMID, and the one the toasts are shown
// and registered under (toast_windows.go). RegisterHotKey, the
// notification area and the single-instance mutex (which takes the ID as
// an argument) need no app ID.
func (d desktop) SetAppID(id string) {
	platform.CheckAppID("SetAppID", id)
	d.b.setToastAppID(id)
}

// SetStrings keeps the app's texts (ADR-251): the next message boxes get
// the buttons' labels the app translated (messageBoxLabels). The other
// buttons keep the labels of the user's Windows.
func (d desktop) SetStrings(strs platform.Strings) {
	d.b.desk.strings = maps.Clone(strs)
}

// Associations returns the current user's registration (package assoc).
func (d desktop) Associations() platform.Associations { return assoc.Native() }

// SetAppMenu checks the menus and returns ErrUnsupported: Windows has no
// app menu bar. The layers above give it to the framed windows instead
// (Window.SetMenu; ADR-224), or draw it.
func (d desktop) SetAppMenu(menu, appItems []platform.MenuItem) error {
	return platform.UnsupportedDesktop{}.SetAppMenu(menu, appItems)
}
