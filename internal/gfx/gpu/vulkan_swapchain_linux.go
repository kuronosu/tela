//go:build telagpu && linux

package gpu

// Swapchains on windows are Windows' (swapchain.go): on Linux a window is
// presented by reading back or through dmabufs (export.go), and the device
// is not a WindowPresenter.

// surfaceSupported: this build makes no surfaces of windows.
const surfaceSupported = false

// vkSwapchain is never created here.
type vkSwapchain struct{}

func (s *vkSwapchain) destroy() {}
