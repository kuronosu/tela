//go:build windows

package win32

import (
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

// SetAppID checks id and keeps nothing (ADR-217): RegisterHotKey, the
// notification area and the single-instance mutex (which takes the ID as
// an argument) need no app ID, and Tela sets no AppUserModelID yet.
func (d desktop) SetAppID(id string) { platform.CheckAppID("SetAppID", id) }

// Associations returns the current user's registration (package assoc).
func (d desktop) Associations() platform.Associations { return assoc.Native() }
