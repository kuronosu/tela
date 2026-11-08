//go:build windows

package win32

import "github.com/kuronosu/tela/internal/platform"

// SetAccessibility implements platform.Window: the update goes to the
// window's UI Automation bridge (uia_windows.go), which exists once a UIA
// client has asked for the window's root; before that the update is
// checked and dropped, as every backend without an active bridge does.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
	if w.uia == nil || w.Closed() {
		return
	}
	w.uia.setGeometry(w.scale(), w.top)
	w.uia.apply(&update)
}
