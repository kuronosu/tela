package remote

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
)

// Window control (phase 12, ADR-178): stubs until the remote backend
// forwards it to the host's real window over the protocol. Arguments are
// checked like on every backend; requests do nothing yet and State stays
// the zero WindowState (promoted from loop.WindowState).

// Minimize does nothing yet.
func (w *Window) Minimize() {}

// SetMaximized does nothing yet.
func (w *Window) SetMaximized(bool) {}

// SetFullscreen does nothing yet.
func (w *Window) SetFullscreen(bool) {}

// SetVisible does nothing yet.
func (w *Window) SetVisible(bool) {}

// SetAlwaysOnTop does nothing yet.
func (w *Window) SetAlwaysOnTop(bool) {}

// SetMinSize checks size and does nothing yet.
func (w *Window) SetMinSize(size platform.Size) { platform.CheckSize("SetMinSize size", size) }

// SetMaxSize checks size and does nothing yet.
func (w *Window) SetMaxSize(size platform.Size) { platform.CheckSize("SetMaxSize size", size) }

// SetPosition does nothing yet.
func (w *Window) SetPosition(image.Point) {}

// Activate does nothing yet.
func (w *Window) Activate() {}

// SetIcon checks the images and does nothing yet.
func (w *Window) SetIcon(images []image.Image) { platform.CheckIcon("SetIcon", images) }

// SetHitRegions checks the regions and does nothing yet.
func (w *Window) SetHitRegions(regions []platform.HitRegion) { platform.CheckHitRegions(regions) }

// RestoreToken returns "": no token yet.
func (w *Window) RestoreToken() string { return "" }
