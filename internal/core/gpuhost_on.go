//go:build telagpu

package core

import (
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/platform"
)

// The Host's GPU hooks in a build with the tag telagpu (gpuhost.go has the
// design).

// EnvGPUTrace is the environment variable that makes a Host with a GPU
// write, to its error log, the device it opened and one line per frame
// drawn by the GPU: what the frame did and the time of each part
// (GPUFrameStats). A development aid of builds with the tag telagpu.
const EnvGPUTrace = "TELA_GPU_TRACE"

// EnvPresent is the environment variable that forces how the frames the
// GPU draws reach a window, for tests and diagnosis: "copy" reads every
// frame back into the framebuffer for Window.Present, "dmabuf" presents
// the device's images without a copy wherever the window and the device
// can at all, also from another device than the display server's. Unset,
// a window is presented without a copy when it can be (gpuhost.go).
const EnvPresent = "TELA_PRESENT"

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
	// present is the value of EnvPresent: "", "copy" or "dmabuf".
	present string
}

// viewGPU is a window's part of the GPU renderer.
type viewGPU struct {
	// surf is the surface of the window's frames, nil until its first
	// frame on the GPU.
	surf *GPUSurface
	// off: the GPU failed for this window, which the CPU rasterizer draws
	// from then on.
	off bool
	// chain presents the window's frames without a copy; nil for a window
	// that does not present dmabufs, and until its first frame on the GPU.
	chain *GPUSwapchain
	// copied: presenting without a copy failed for this window, whose
	// frames are read back from then on.
	copied bool
	// owed: a frame the window was not given is still to be presented, in
	// full: every buffer was busy, the window dropped the present, or it
	// was shown again.
	owed bool
	// why is the reason last logged for reading the frames back.
	why string
	// lag: the GPU went away (it failed, or its device was lost) while
	// the framebuffer was behind frames presented without a copy, so it
	// holds no frame until the CPU rasterizer draws one, which was asked
	// for.
	lag bool
}

// openGPU opens the GPU for the Host's windows, unless TELA_RENDERER asks
// for the CPU rasterizer or the backend is the headless one: the device
// the main window's display server composites with, when it names one and
// TELA_GPU_DEVICE names none, else the first discrete one, else the first
// integrated one. A software device is used only when TELA_RENDERER=gpu
// asks for it (ADR-288). When no device initializes, one line of the error
// log says why and the CPU rasterizer draws the app.
func (h *Host) openGPU() {
	if h.gpu.g != nil || h.backend.Name() == "headless" {
		return
	}
	renderer := os.Getenv(EnvRenderer)
	switch renderer {
	case "cpu":
		return
	case "", "gpu":
	default:
		fmt.Fprintf(h.log, "tela: %s=%q is neither cpu nor gpu; using the GPU if there is one\n", EnvRenderer, renderer)
	}
	trace := os.Getenv(EnvGPUTrace) != ""
	cfg := gpu.Config{HardwareOnly: renderer != "gpu"}
	if p, ok := h.win.(platform.DmabufPresenter); ok {
		// Presenting from another device than the display server's costs
		// a copy at best.
		if fb, ok := p.DmabufFeedback(); ok && fb.HasDevice {
			cfg.DRM, cfg.HasDRM = gpu.DRMNode{Major: fb.DeviceMajor, Minor: fb.DeviceMinor}, true
		}
	}
	g, err := OpenGPUWith(cfg, func(msg string) { fmt.Fprintln(h.log, "tela: gpu:", msg) })
	switch {
	case errors.Is(err, gpu.ErrSoftwareDevice):
		// A machine whose only Vulkan device is a software one (a virtual
		// machine, a server): not worth a line, unless a device was asked
		// for by name.
		if trace || os.Getenv("TELA_GPU_DEVICE") != "" {
			fmt.Fprintf(h.log, "tela: using the CPU rasterizer, %s=gpu draws with the GPU: %s\n", EnvRenderer, oneLine(err))
		}
		return
	case err != nil:
		fmt.Fprintf(h.log, "tela: no GPU renderer, using the CPU rasterizer: %s\n", oneLine(err))
		return
	}
	h.UseGPU(g)
	if h.gpu.trace = trace; trace {
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
	switch v := os.Getenv(EnvPresent); v {
	case "", "copy", "dmabuf":
		h.gpu.present = v
	default:
		h.gpu.present = ""
		fmt.Fprintf(h.log, "tela: %s=%q is neither copy nor dmabuf; presenting without a copy where the window can\n", EnvPresent, v)
	}
	if h.gpu.present == "copy" {
		fmt.Fprintf(h.log, "tela: %s=copy: the frames the GPU draws are read back and presented from the framebuffer\n", EnvPresent)
	}
	for _, v := range h.views {
		v.gpu.off, v.gpu.copied, v.gpu.why = false, false, ""
	}
}

// OpenGPU opens the GPU for the Host's windows as Run does, after the main
// window exists (openGPU): for a caller that runs the backend's loop
// itself (a live test).
func (h *Host) OpenGPU() { h.openGPU() }

// GPUWindow describes how the GPU's frames of a window reach it.
type GPUWindow struct {
	// Direct reports that the window's last frame was presented without a
	// copy, from a buffer of its swapchain.
	Direct bool
	// Stats are the swapchain's (the zero value for a window that never
	// had one), and Busy the number of its buffers the OS still reads.
	Stats GPUPresentStats
	Busy  int
}

// GPUWindow returns how the GPU's frames of window w reach it; the zero
// value for a window the CPU rasterizer draws, and for one that is not the
// Host's.
func (h *Host) GPUWindow(w platform.Window) GPUWindow {
	v := h.viewOf(w)
	if v == nil || h.gpu.g == nil {
		return GPUWindow{}
	}
	var out GPUWindow
	if s := v.gpu.surf; s != nil {
		out.Direct = s.Held() && s.Stats.Direct
	}
	if c := v.gpu.chain; c != nil {
		out.Stats = c.Stats
		for _, b := range c.bufs {
			if b.buf.Busy() {
				out.Busy++
			}
		}
	}
	return out
}

// GPU returns the Host's GPU renderer, nil when its windows are drawn by
// the CPU rasterizer.
func (h *Host) GPU() *GPU { return h.gpu.g }

// closeGPU releases the windows' targets and buffers and closes the GPU.
func (h *Host) closeGPU() {
	g := h.gpu.g
	if g == nil {
		return
	}
	for _, v := range h.views {
		v.lagGPU()
		v.dropGPU()
		v.gpu.surf, v.gpu.chain = nil, nil // they are g's
	}
	h.gpu.g, h.gpu.trimAt = nil, 0
	g.Close()
}

// lagGPU runs before window v loses the GPU: when its framebuffer is
// behind frames presented without a copy, nothing can bring it to them
// any more, so the CPU rasterizer is asked for the frame, in full.
func (v *winView) lagGPU() {
	if s := v.gpu.surf; s == nil || !s.Behind() || v.host.ended {
		return
	}
	v.gpu.lag, v.stale = true, true
	v.win.RequestFrame()
}

// dropGPU releases the window's target on the GPU and the buffers it is
// presented from: the window is hidden, minimized or closed. Its next
// frame is drawn in full into a new one.
func (v *winView) dropGPU() {
	if v.gpu.chain != nil {
		v.gpu.chain.Release()
	}
	if v.gpu.surf != nil {
		v.gpu.surf.Release()
	}
}

// shownGPU notes that the window is shown again: a window whose frames
// are dmabufs is not mapped by the backend alone, which asks for a frame
// instead (platform, "Presenting dmabufs"), and that frame must present
// whether or not anything changed.
func (v *winView) shownGPU() {
	if v.gpu.chain != nil {
		v.gpu.owed = true
	}
}

// presentOwed reports whether window v must be presented at its next
// frame although nothing changed: a frame drawn for it did not reach the
// screen.
func (h *Host) presentOwed(v *winView) bool { return v.gpu.owed }

// direct reports whether window v's next frame stays on the device, to be
// presented without a copy: the window presents dmabufs and reports a
// feedback, the device exports a format the feedback accepts and is the
// display server's, nothing failed before, TELA_PRESENT does not say
// otherwise, and no debug overlay is on (the CPU rasterizer draws them
// over the frame read back). When a window that could is read back, one
// line of the error log says why, once per reason.
func (h *Host) direct(v *winView) bool {
	g := h.gpu.g
	p, ok := v.win.(platform.DmabufPresenter)
	if !ok || v.gpu.copied || h.gpu.present == "copy" {
		return false
	}
	if v.gpu.chain == nil {
		v.gpu.chain = NewGPUSwapchain(g, p)
	}
	force := h.gpu.present == "dmabuf"
	ready, why, expected := v.gpu.chain.Ready(force)
	if !ready {
		if (expected || force) && why != v.gpu.why {
			v.gpu.why = why
			fmt.Fprintf(h.log, "tela: the GPU's frames of this window are read back and presented from the framebuffer: %s\n", why)
		}
		return false
	}
	v.gpu.why = ""
	return v.tree.Debug() == 0
}

// rasterize brings window v to its tree's frame: on the GPU
// (Tree.RasterizeFrameGPU), read back into the framebuffer or, for a
// window presented without a copy, left in the surface's target; or with
// the CPU rasterizer when the Host has no GPU or the GPU failed for the
// window.
func (h *Host) rasterize(v *winView, full bool) Damage {
	g := h.gpu.g
	if g == nil || v.gpu.off {
		v.gpu.lag = false // a frame of the CPU rasterizer after the GPU's is whole
		return v.tree.RasterizeFrame(v.fb, &v.renderer, full)
	}
	if v.gpu.surf == nil {
		v.gpu.surf = g.NewSurface()
	}
	direct := h.direct(v)
	v.gpu.surf.SetDirect(direct)
	if !direct && v.gpu.chain != nil {
		// Read back, the frames are presented from the framebuffer, and
		// the X server may draw them into a buffer it flipped to: the
		// buffers are made again when the window is direct again.
		v.gpu.chain.Release()
	}
	d, err := v.tree.RasterizeFrameGPU(v.fb, v.gpu.surf, &v.renderer, full)
	if h.gpu.trace {
		st := v.gpu.surf.Stats
		fmt.Fprintf(h.log, "tela: gpu: frame %dx%d full=%v direct=%v renders=%d shifts=%d read=%dpx instances=%d draws=%d passes=%d uploads=%d (%d bytes) encode=%v draw=%v shift=%v readback=%v fallback=%v\n",
			v.fb.Rect.Dx(), v.fb.Rect.Dy(), d.Full, st.Direct, st.Renders, st.Shifts, st.Pixels, st.Instances, st.Draws, st.Passes, st.Uploads, st.UploadBytes, st.Encode, st.Draw, st.Shift, st.Read, st.Fallback)
	}
	if err != nil {
		h.gpuFailed(v, err)
	}
	return d
}

// present shows window v the frame rasterize drew, whose damage is d:
// from a buffer of the window's swapchain, without a copy, when the frame
// stayed on the device, else from the framebuffer (Window.Present with
// damage, nil for everything). It reports whether the window was given
// the frame; when it was not (every buffer busy, a hidden window), the
// frame is owed and presented at a later one (presentOwed).
func (h *Host) present(v *winView, d Damage, damage []image.Rectangle) (bool, error) {
	s, c := v.gpu.surf, v.gpu.chain
	if h.gpu.g == nil || v.gpu.off || s == nil || c == nil || !s.Direct() || !s.Held() || !s.Stats.Direct {
		v.gpu.owed = false
		return true, v.win.Present(v.fb, damage)
	}
	// A nil damage is everything (a ConfigureEvent, a full frame). An
	// empty one is a frame presented because one is owed: the swapchain
	// knows what the window lacks (everything after a dropped present,
	// the damage of the frames it could not present otherwise).
	res, err := c.Present(s, d, damage == nil)
	if h.gpu.trace {
		st := c.Stats
		fmt.Fprintf(h.log, "tela: gpu: present result=%d buffers=%d copied=%dpx register=%v copy=%v present=%v\n", res, st.Buffers, st.Pixels, st.Register, st.Copy, st.Present)
	}
	if err != nil {
		return true, h.directFailed(v, err)
	}
	switch res {
	case GPUPresentBusy:
		// Nothing wakes the loop when the OS releases a buffer.
		v.gpu.owed = true
		v.win.RequestFrame()
		return false, nil
	case GPUPresentDropped:
		v.gpu.owed = true
		return false, nil
	}
	v.gpu.owed = false
	return true, nil
}

// directFailed presents window v's frame from the framebuffer after
// presenting it without a copy failed (a buffer the display server does
// not import, an error of the device or of the window): the frame is
// read back, or drawn by the CPU rasterizer when the device is gone, and
// the window's frames are read back from then on. One line of the error
// log says why.
func (h *Host) directFailed(v *winView, err error) error {
	g := h.gpu.g
	v.gpu.copied, v.gpu.owed = true, false
	v.gpu.chain.Release()
	v.gpu.surf.SetDirect(false)
	if !g.Lost() {
		fmt.Fprintf(h.log, "tela: this window's frames cannot be presented without a copy, they are read back from now on: %s\n", oneLine(err))
		if err = v.gpu.surf.Sync(v.fb); err == nil && v.gpu.surf.Behind() {
			err = errors.New("gpu: the surface holds no frame to read back")
		}
	}
	if err != nil {
		h.gpuFailed(v, err)
		v.tree.RasterizeFrame(v.fb, &v.renderer, true)
		v.gpu.lag, v.stale = false, false // the framebuffer holds the frame
	}
	return v.win.Present(v.fb, nil)
}

// syncPixels brings window v's framebuffer to the frame the window shows,
// when its frames are presented without a copy and not read back: for
// what reads the framebuffer once in a while (a drag image, FramePixels).
// It reports whether the framebuffer holds the frame: not when the target
// was released (a minimized window) or the read failed.
func (v *winView) syncPixels() bool {
	if v.gpu.lag {
		return false
	}
	s := v.gpu.surf
	if s == nil || v.gpu.off || v.fb == nil || !s.Behind() {
		return true
	}
	if err := s.Sync(v.fb); err != nil {
		v.host.gpuFailed(v, err) // the next frame is the CPU rasterizer's
		return false
	}
	return !s.Behind()
}

// gpuFailed switches window v to the CPU rasterizer after an error of the
// GPU (the frame was already drawn by it, in full), or every window when
// the device is lost, and says so once in the error log.
func (h *Host) gpuFailed(v *winView, err error) {
	g := h.gpu.g
	v.lagGPU()
	v.dropGPU()
	v.gpu.off = true
	if !g.Lost() {
		fmt.Fprintf(h.log, "tela: the GPU renderer failed, this window is drawn by the CPU rasterizer from now on: %s\n", oneLine(err))
		return
	}
	fmt.Fprintf(h.log, "tela: the GPU device is lost, the app is drawn by the CPU rasterizer from now on: %s\n", oneLine(err))
	// The framebuffers of the other windows hold their frames, or the
	// CPU rasterizer was asked for them (lagGPU): their next frame is its,
	// in full (layerState.rasGPU).
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
			// What the windows show stays on the device.
			var lists []*gfx.DisplayList
			for _, v := range h.views {
				if v.gpu.surf != nil && !v.gpu.off {
					lists = append(lists, v.tree.LastFrame())
				}
			}
			g.Trim(lists...)
			for _, v := range h.views {
				if v.gpu.chain != nil {
					v.gpu.chain.Trim() // the buffers a slower OS needed
				}
			}
		}
	}
}
