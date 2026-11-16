//go:build telagpu

package core

import (
	"errors"
	"image"

	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/platform"
)

// A window's frames presented through a swapchain on the window itself
// (ADR-281, ADR-289; gpuhost.go has the design): Win32 windows, whose
// HWND the device makes a Vulkan surface of. The platform side is
// platform.Win32SurfacePresenter, the device's gpu.WindowPresenter and
// gpu.Swapchain, which keeps the account of what each image lacks; this
// is the glue, the counterpart of the dmabuf swapchain of a Linux window
// (gpuchain.go).
//
// The surface's target stays the frame. A present gives the swapchain the
// damage of the frame (it keeps what each of its images lacks), and the
// window's Full (a repaint it asked for, a show) as a whole present; the
// window is told of each present, which shows it the first time. A frame
// that finds no image free, or a window at another size than the frame
// (a resize under way), is presented at the next frame, which is asked
// for; a minimized window gets nothing until it comes back. A minimized
// window keeps its swapchain (the restore animation shows its last image),
// a hidden one does not; closing the window, the GPU going and the app
// quitting destroy the swapchain and the surface first, and the window
// calls Close before it is destroyed by anything else (the backend's loop
// ending destroys its windows before Run returns).

// GPUWindowSurface presents the frames of a GPUSurface in a window through
// a swapchain on the window. It belongs to the UI thread, like the
// window.
type GPUWindowSurface struct {
	g   *GPU
	win platform.Win32SurfacePresenter
	// sc is the swapchain, nil until the first Present and after Close;
	// presented: it presented a frame since it was made or released.
	sc        *gpu.Swapchain
	presented bool
	// Stats are the swapchain's (gpu.SwapchainStats) after the last
	// Present.
	Stats gpu.SwapchainStats
}

// NewGPUWindowSurface returns the presenter of a window drawn by g. It
// makes nothing until its first Present.
func NewGPUWindowSurface(g *GPU, win platform.Win32SurfacePresenter) *GPUWindowSurface {
	return &GPUWindowSurface{g: g, win: win}
}

// Ready reports whether the window can be presented through a swapchain
// right now: it reports a surface (not a transparent window, one with a
// client-area backdrop or a title bar overlay), and the device presents
// to windows. why says what is missing; expected is false when that is
// by design, not worth a line of the log.
func (c *GPUWindowSurface) Ready() (ok bool, why string, expected bool) {
	if _, ok := c.win.Win32Surface(); !ok {
		return false, "the window cannot be presented through a swapchain (it is transparent, has a client-area backdrop or a title bar overlay)", false
	}
	if c.g.WindowPresenter() == nil {
		return false, "the GPU device presents to no window", false
	}
	return true, "", true
}

// Owed reports whether the window asks for a whole present although
// nothing changed (a repaint, a show), once it was presented through the
// swapchain.
func (c *GPUWindowSurface) Owed() bool {
	if !c.presented {
		return false
	}
	s, ok := c.win.Win32Surface()
	return ok && s.Full
}

// Present brings an image of the swapchain to the frame s holds and
// presents it. d is the damage of the frames drawn into s since the last
// Present (as GPUSwapchain.Present takes it), and full reports that the
// window must be given the whole frame. After an error the caller closes
// the presenter and presents the frame with Window.Present, read back.
func (c *GPUWindowSurface) Present(s *GPUSurface, d Damage, full bool) (GPUPresent, error) {
	if s.target == nil {
		return 0, errors.New("gpu: a window's swapchain presents a surface that holds no frame")
	}
	info, ok := c.win.Win32Surface()
	if !ok {
		return 0, errors.New("gpu: the window can no longer be presented through a swapchain")
	}
	if c.sc == nil {
		wp := c.g.WindowPresenter()
		if wp == nil {
			return 0, errors.New("gpu: the GPU device presents to no window")
		}
		sc, err := wp.NewSwapchain(gpu.NativeWindow{Instance: info.Instance, Window: info.Window})
		if err != nil {
			return 0, c.g.note(err)
		}
		c.sc = sc
		// The window may be destroyed before the Host closes this (a
		// backend's loop that ends destroys its windows first): it calls
		// Close then.
		c.win.SetSurfaceCloser(c.Close)
	}
	var damage []image.Rectangle
	if !d.Full {
		damage = d.Changed()
	}
	res, err := c.sc.Present(s.target, damage, d.Full, full || info.Full)
	c.Stats = c.sc.Stats
	if err != nil {
		return 0, c.g.note(err)
	}
	switch res {
	case gpu.SwapchainRetry:
		return GPUPresentBusy, nil
	case gpu.SwapchainHidden:
		return GPUPresentDropped, nil
	}
	if presented, ok := c.sc.LastPresent(); ok {
		c.win.SurfacePresented(presented)
		c.presented = true
	}
	return GPUPresentShown, nil
}

// Release destroys the swapchain and keeps the window's surface: the
// window is hidden, or its frames are read back from now on. The next
// Present makes a swapchain and is whole.
func (c *GPUWindowSurface) Release() {
	if c.sc != nil {
		c.sc.Release()
	}
	c.presented = false
}

// Close destroys the swapchain and the window's surface: before the
// window closes, after an error, when the GPU goes and when the app quits;
// the window calls it too, right before it is destroyed, when nothing did
// before (platform.Win32SurfacePresenter.SetSurfaceCloser).
func (c *GPUWindowSurface) Close() {
	if c.sc != nil {
		c.sc.Close()
		c.sc = nil
		c.win.SetSurfaceCloser(nil)
	}
	c.presented = false
}
