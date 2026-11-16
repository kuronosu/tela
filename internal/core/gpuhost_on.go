//go:build telagpu

package core

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// The Host's GPU hooks in a build with the tag telagpu (gpuhost.go has the
// design).

// EnvGPUTrace is the environment variable that makes a Host with a GPU
// write, to its error log, the device it opened and one line per frame
// drawn by the GPU: what the frame did and the time of each part
// (GPUFrameStats). A development aid of builds with the tag telagpu.
const EnvGPUTrace = "TELA_GPU_TRACE"

// hostGPU is the GPU renderer of a Host's windows.
type hostGPU struct {
	// g is the GPU, nil when the windows are drawn by the CPU rasterizer:
	// none was asked for, none initialized, or the device was lost.
	g *GPU
	// trimAt is the backend time at which the Renderer's memory is trimmed
	// (GPU.Trim): RasterSettle after a frame that left every window drawn
	// by the GPU at rest, moved or cancelled by the next frame; 0 for none.
	trimAt time.Duration
	trace  bool
}

// viewGPU is a window's part of the GPU renderer.
type viewGPU struct {
	// surf is the surface of the window's frames, nil until its first
	// frame on the GPU.
	surf *GPUSurface
	// off: the GPU failed for this window, which the CPU rasterizer draws
	// from then on.
	off bool
}

// openGPU opens the GPU for the Host's windows, unless TELA_RENDERER asks
// for the CPU rasterizer or the backend is the headless one. When no
// device initializes, one line of the error log says why and the CPU
// rasterizer draws the app.
func (h *Host) openGPU() {
	if h.gpu.g != nil || h.backend.Name() == "headless" {
		return
	}
	switch v := os.Getenv(EnvRenderer); v {
	case "cpu":
		return
	case "", "gpu":
	default:
		fmt.Fprintf(h.log, "tela: %s=%q is neither cpu nor gpu; using the GPU if there is one\n", EnvRenderer, v)
	}
	g, err := OpenGPU(func(msg string) { fmt.Fprintln(h.log, "tela: gpu:", msg) })
	if err != nil {
		fmt.Fprintf(h.log, "tela: no GPU renderer, using the CPU rasterizer: %s\n", oneLine(err))
		return
	}
	h.UseGPU(g)
	if h.gpu.trace = os.Getenv(EnvGPUTrace) != ""; h.gpu.trace {
		fmt.Fprintf(h.log, "tela: gpu: rendering with %s\n", g.Name())
	}
}

// UseGPU makes g the renderer of the frames of the Host's windows, the
// ones open and the ones opened later. The Host closes it when it ends, or
// when its device is lost. Run calls it with the GPU it opens; a test
// gives a Host on the headless backend the GPU of the device it tests.
func (h *Host) UseGPU(g *GPU) {
	h.closeGPU()
	h.gpu.g = g
	for _, v := range h.views {
		v.gpu.off = false
	}
}

// GPU returns the Host's GPU renderer, nil when its windows are drawn by
// the CPU rasterizer.
func (h *Host) GPU() *GPU { return h.gpu.g }

// closeGPU releases the windows' targets and closes the GPU.
func (h *Host) closeGPU() {
	g := h.gpu.g
	if g == nil {
		return
	}
	for _, v := range h.views {
		v.dropGPU()
		v.gpu.surf = nil // it is g's
	}
	h.gpu.g, h.gpu.trimAt = nil, 0
	g.Close()
}

// dropGPU releases the window's target on the GPU: the window is hidden,
// minimized or closed. Its next frame is drawn in full into a new one.
func (v *winView) dropGPU() {
	if v.gpu.surf != nil {
		v.gpu.surf.Release()
	}
}

// rasterize brings window v's framebuffer to its tree's frame: on the GPU,
// read back (Tree.RasterizeFrameGPU), or with the CPU rasterizer when the
// Host has no GPU or the GPU failed for the window.
func (h *Host) rasterize(v *winView, full bool) Damage {
	g := h.gpu.g
	if g == nil || v.gpu.off {
		return v.tree.RasterizeFrame(v.fb, &v.renderer, full)
	}
	if v.gpu.surf == nil {
		v.gpu.surf = g.NewSurface()
	}
	d, err := v.tree.RasterizeFrameGPU(v.fb, v.gpu.surf, &v.renderer, full)
	if h.gpu.trace {
		st := v.gpu.surf.Stats
		fmt.Fprintf(h.log, "tela: gpu: frame %dx%d full=%v renders=%d shifts=%d read=%dpx encode=%v draw=%v shift=%v readback=%v fallback=%v\n",
			v.fb.Rect.Dx(), v.fb.Rect.Dy(), d.Full, st.Renders, st.Shifts, st.Pixels, st.Encode, st.Draw, st.Shift, st.Read, st.Fallback)
	}
	if err != nil {
		h.gpuFailed(v, err)
	}
	return d
}

// gpuFailed switches window v to the CPU rasterizer after an error of the
// GPU (the frame was already drawn by it, in full), or every window when
// the device is lost, and says so once in the error log.
func (h *Host) gpuFailed(v *winView, err error) {
	g := h.gpu.g
	v.dropGPU()
	v.gpu.off = true
	if !g.Lost() {
		fmt.Fprintf(h.log, "tela: the GPU renderer failed, this window is drawn by the CPU rasterizer from now on: %s\n", oneLine(err))
		return
	}
	fmt.Fprintf(h.log, "tela: the GPU device is lost, the app is drawn by the CPU rasterizer from now on: %s\n", oneLine(err))
	// The framebuffers of the other windows hold their frames: their next
	// frame is the CPU rasterizer's, in full (layerState.rasGPU).
	h.closeGPU()
}

// oneLine returns the message of err on one line (the errors of the
// device selection list the devices, one per line).
func oneLine(err error) string { return strings.Join(strings.Fields(err.Error()), " ") }

// gpuTrimAt returns when the GPU renderer's memory is trimmed, 0 for
// never.
func (h *Host) gpuTrimAt() time.Duration { return h.gpu.trimAt }

// gpuScheduleTrim sets when the GPU renderer's memory is trimmed, after a
// frame of any window: RasterSettle after now when the Renderer may keep
// memory and every window it draws that gets frames is at rest (no
// animation, no update, no requested frame), else never. It reports whether the Host's timer must
// be armed again (a deadline appeared or went away; a later one needs no
// new timer, since one that fires early arms again).
func (h *Host) gpuScheduleTrim(now time.Duration) bool {
	was := h.gpu.trimAt
	at := time.Duration(0)
	if g := h.gpu.g; g != nil && g.TrimDue() {
		at = now + RasterSettle
		for _, v := range h.views {
			if t := v.tree; v.gpu.surf != nil && !v.gpu.off && !v.paused() && (t.asked || t.Animating() || t.Dirty()) {
				at = 0
				break
			}
		}
	}
	h.gpu.trimAt = at
	return (was == 0) != (at == 0)
}

// gpuTick trims the GPU renderer's memory when its deadline has come. It
// draws nothing and asks for no frame.
func (h *Host) gpuTick(now time.Duration) {
	if at := h.gpu.trimAt; at != 0 && at <= now {
		h.gpu.trimAt = 0
		if g := h.gpu.g; g != nil {
			g.Trim()
		}
	}
}
