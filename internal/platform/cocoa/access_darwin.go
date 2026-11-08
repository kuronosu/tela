//go:build darwin

package cocoa

import "github.com/kuronosu/tela/internal/platform"

// SetAccessibility implements platform.Window. The cocoa backend has no
// accessibility bridge yet (a later task of phase 19, ADR-246): it never
// reports an active one, so it ignores updates after checking them, as
// every backend does.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
}
