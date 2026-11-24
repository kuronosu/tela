//go:build !telagpu

package core

import (
	"fmt"
	"image"
	"os"
	"time"
)

// The Host's GPU hooks in a build without the tag telagpu (gpuhost.go):
// there is no GPU renderer, and nothing of it is linked.

// hostGPU and viewGPU are what a Host and a window keep of the GPU
// renderer: nothing here.
type (
	hostGPU struct{}
	viewGPU struct{}
)

// openGPU says, once, that TELA_RENDERER=gpu asks for what this build does
// not have.
func (h *Host) openGPU() {
	if os.Getenv(EnvRenderer) == "gpu" {
		fmt.Fprintf(h.log, "tela: %s=gpu: this build has no GPU renderer (it needs the build tag telagpu); using the CPU rasterizer\n", EnvRenderer)
	}
}

func (h *Host) closeGPU()   { h.gpu = hostGPU{} }
func (h *Host) endLostGPU() {}

// quitGPU ends the swapchains on the windows before the loop ends: there
// are none.
func (h *Host) quitGPU() {}

// rasterize brings window v's framebuffer to its tree's frame.
func (h *Host) rasterize(v *winView, full bool) Damage {
	return v.tree.RasterizeFrame(v.fb, &v.renderer, full)
}

// present shows window v its framebuffer.
func (h *Host) present(v *winView, d Damage, damage []image.Rectangle) (bool, error) {
	return true, v.win.Present(v.fb, damage)
}

// presentOwed reports whether a frame must present although nothing
// changed: never, when every frame is presented from the framebuffer.
func (h *Host) presentOwed(*winView) bool { return false }

func (h *Host) gpuTrimAt() time.Duration               { return 0 }
func (h *Host) gpuScheduleTrim(now time.Duration) bool { return false }
func (h *Host) gpuTick(now time.Duration)              {}

// dropGPU releases the window's target on the GPU, and endGPU the
// swapchain on the window: it has neither.
func (v *winView) dropGPU() { v.gpu = viewGPU{} }
func (v *winView) endGPU()  {}

// shownGPU and syncPixels are for windows presented from the GPU's
// images: the framebuffer always holds the frame here.
func (v *winView) shownGPU()        {}
func (v *winView) syncPixels() bool { return true }
