//go:build telagpu && windows

package gpu

// Images exported as dmabufs are Linux's (export.go): on Windows a window
// is presented through a swapchain on its HWND (ADR-281), and the device
// is not an Exporter.

// exportSupported: this build exports no images.
const exportSupported = false

// vkExport is never created here, and the device asks the driver nothing
// of exports.
type (
	vkExport    struct{}
	exportCache struct{}
)

func (c *exportCache) forget() {}

func (e *vkExport) destroy() {}
