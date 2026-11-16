//go:build !telagpu

package core

import (
	"fmt"
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

func (h *Host) closeGPU() { h.gpu = hostGPU{} }

// rasterize brings window v's framebuffer to its tree's frame.
func (h *Host) rasterize(v *winView, full bool) Damage {
	return v.tree.RasterizeFrame(v.fb, &v.renderer, full)
}

func (h *Host) gpuTrimAt() time.Duration               { return 0 }
func (h *Host) gpuScheduleTrim(now time.Duration) bool { return false }
func (h *Host) gpuTick(now time.Duration)              {}

// dropGPU releases the window's target on the GPU: it has none.
func (v *winView) dropGPU() { v.gpu = viewGPU{} }
