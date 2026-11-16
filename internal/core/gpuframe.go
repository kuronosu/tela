//go:build telagpu

package core

import (
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Frames drawn by the GPU (gpuhost.go has the design; ADR-281).
//
// A GPU is a device and the Renderer of its targets; a GPUSurface is the
// target of one window's frames. Tree.RasterizeFrameGPU brings a surface,
// and the framebuffer beside it, to the tree's last frame, as
// Tree.RasterizeFrame brings a framebuffer to it with the CPU rasterizer.
// A Host owns one GPU and a surface per window (gpuhost_on.go); a test rig
// that drives a Tree without a Host owns its own.

// GPU is the GPU renderer of a process: a device, and the one Renderer
// that draws into its targets. It belongs to the UI thread.
type GPU struct {
	dev gpu.Device
	// own: Close closes the device (OpenGPU); otherwise it is the
	// caller's (NewGPU).
	own bool
	r   gpu.Renderer
	// used: a frame was drawn since the last Trim, so the Renderer may
	// keep memory that Trim releases.
	used bool
	// lost: the device is gone (gpu.ErrDeviceLost); nothing more is drawn
	// with it.
	lost bool
}

// OpenGPU opens the device ADR-281 chooses (the environment variable
// TELA_GPU_DEVICE, else the first discrete device). With TELA_VK_VALIDATE=1
// the device runs under the API's validation layer, whose messages go to
// log. It returns an error when the build has no GPU backend
// (gpu.ErrUnavailable) or no device initializes: the caller then draws
// with the CPU rasterizer.
func OpenGPU(log func(msg string)) (*GPU, error) {
	var cfg gpu.Config
	if os.Getenv("TELA_VK_VALIDATE") == "1" {
		cfg.Validate, cfg.Log = true, log
	}
	dev, err := gpu.Open(cfg)
	if err != nil {
		return nil, err
	}
	return &GPU{dev: dev, own: true}, nil
}

// NewGPU returns the GPU renderer of a device the caller opened and
// closes, after the GPU.
func NewGPU(dev gpu.Device) *GPU { return &GPU{dev: dev} }

// Name describes the device.
func (g *GPU) Name() string { return g.dev.Name() }

// Lost reports that the device is gone: its surfaces draw nothing more,
// and every frame is the CPU rasterizer's.
func (g *GPU) Lost() bool { return g.lost }

// NewSurface returns the surface of a window's frames. Its target is
// created by the first frame.
func (g *GPU) NewSurface() *GPUSurface { return &GPUSurface{g: g} }

// TrimDue reports whether the Renderer may keep memory that Trim
// releases: a frame was drawn since the last one.
func (g *GPU) TrimDue() bool { return g.used && !g.lost }

// Trim releases what the Renderer keeps for its frames, on the device
// too (gpu.Renderer.Trim): for windows that rest. The surfaces keep their
// pixels, and the next frame uploads what it draws with.
func (g *GPU) Trim() {
	g.used = false
	if !g.lost {
		g.r.Trim()
	}
}

// Close releases the Renderer's memory and closes the device OpenGPU
// opened, with the targets of the surfaces that were not released.
func (g *GPU) Close() {
	if g.dev == nil {
		return
	}
	g.r.Trim()
	if g.own {
		g.dev.Close()
	}
	g.dev, g.lost = nil, true
}

// note records what an error of the device means for the GPU.
func (g *GPU) note(err error) error {
	if errors.Is(err, gpu.ErrDeviceLost) {
		g.lost = true
	}
	return err
}

// GPUSurface is what the frames of one window are drawn into on the GPU:
// a target of the size of the window's framebuffer, which keeps the last
// frame between frames as the framebuffer does.
type GPUSurface struct {
	g      *GPU
	target gpu.Target
	// ras is the frame the target holds (and the framebuffer with it), nil
	// when it holds none.
	ras *frame
	// Stats describes the last RasterizeFrameGPU.
	Stats GPUFrameStats
}

// GPUFrameStats describes one RasterizeFrameGPU: what it did, and the time
// of each part (the time of the frame not counted in them is the damage
// between the frames).
type GPUFrameStats struct {
	// Renders counts the renders (one for a full frame, one per damaged
	// rectangle), Shifts the shifts, and Pixels the device pixels read
	// back.
	Renders, Shifts, Pixels int
	// Encode is the time the encoder took on the CPU, Draw the time the
	// device took to draw, Shift to move the scrolled pixels and Read to
	// copy the damage back into the framebuffer, each with its wait for
	// the GPU.
	Encode, Draw, Shift, Read time.Duration
	// Fallback reports that the GPU failed and the CPU rasterizer drew the
	// frame.
	Fallback bool
}

// Release destroys the surface's target: for a window that is hidden,
// minimized or closed. The next frame creates one and is drawn in full.
func (s *GPUSurface) Release() {
	if s.target != nil {
		s.target.Close()
		s.target = nil
	}
	s.ras = nil
}

// Held reports whether the surface has a target.
func (s *GPUSurface) Held() bool { return s.target != nil }

// fit gives the surface a target of the given size: a new one, which
// holds no frame, unless it has one of that size.
func (s *GPUSurface) fit(size image.Point) error {
	if s.target != nil && s.target.Size() == size {
		return nil
	}
	s.Release()
	target, err := s.g.dev.NewTarget(size.X, size.Y)
	if err != nil {
		return err
	}
	s.target = target
	return nil
}

// DrawFull draws the last frame of t's Frame in full into the surface,
// which takes the size of dst, and copies its pixels into dst (at the
// origin). It is the reference of RasterizeFrameGPU, whose frames have
// these pixels bit for bit: the tree does not notice it (its next
// RasterizeFrameGPU into another surface goes on from its own frame), and
// the surface holds no frame of a session afterwards. For tests.
func (s *GPUSurface) DrawFull(t *Tree, dst *image.RGBA) error {
	f := t.lay.cur
	g := s.g
	switch {
	case f == nil:
		return errors.New("gpu: the tree has no frame yet")
	case g.lost || g.dev == nil:
		return gpu.ErrDeviceLost
	}
	if err := s.fit(dst.Rect.Size()); err != nil {
		return g.note(err)
	}
	s.ras = nil
	canvas := t.clearColor()
	res, err := g.r.Render(s.target, f.dl, gpu.Options{Scale: f.scale, Clear: &canvas})
	if err != nil {
		return g.note(err)
	}
	if len(res.Unsupported) > 0 {
		return fmt.Errorf("gpu: %s cannot draw %s", g.dev.Name(), strings.Join(res.Unsupported, ", "))
	}
	return g.note(s.target.ReadPixels(dst, dst.Rect))
}

// RasterizeFrameGPU is RasterizeFrame with the application's frame drawn
// by the GPU: it brings s and fb to the last frame of Frame. s and fb
// hold the frame this method drew last (the pixels outside the damage are
// kept), unless full is set, the scale or the size of fb changed, the
// surface was released, or the CPU rasterizer drew the tree's frame in
// between: the frame is then drawn in full. The damage is drawn on the
// GPU and read back into fb, which then holds the pixels of the target.
// They are those of a full frame of the GPU, bit for bit, and within the
// tolerance of ADR-282 of the CPU rasterizer's. It returns what it did to
// fb, like RasterizeFrame.
//
// r is the CPU Renderer of the window: it rasterizes the debug overlays
// over the frame read back when they are on (debug.go), and the frame
// itself when the GPU fails. In that case the frame is drawn in full by
// the CPU rasterizer, fb holds it, the surface's target is released, and
// the error is returned with the damage: the caller goes on with
// RasterizeFrame, or tries again.
func (t *Tree) RasterizeFrameGPU(fb *image.RGBA, s *GPUSurface, r *raster.Renderer, full bool) (Damage, error) {
	var err error
	s.Stats = GPUFrameStats{}
	draw := func(dst *image.RGBA, full bool) Damage {
		d, e := t.rasterizeAppGPU(dst, s, full)
		if e == nil {
			return d
		}
		// The target, and dst with it, may hold a part of the frame.
		err = s.g.note(e)
		s.Release()
		s.Stats.Fallback = true
		return t.rasterizeApp(dst, r, true)
	}
	var d Damage
	if t.dbg.flags == 0 {
		d = draw(fb, full)
	} else {
		d = t.rasterizeDebug(fb, r, full, draw)
	}
	if !s.g.lost {
		s.g.r.EndFrame()
	}
	r.EndFrame()
	t.lay.bdc.EndFrame()
	return d, err
}

// rasterizeAppGPU is rasterizeApp on the GPU (see RasterizeFrameGPU). On
// an error, the target and fb hold a mix of two frames.
func (t *Tree) rasterizeAppGPU(fb *image.RGBA, s *GPUSurface, full bool) (Damage, error) {
	L := &t.lay
	f := L.cur
	if f == nil {
		return Damage{}, nil
	}
	g := s.g
	if g.lost || g.dev == nil {
		return Damage{}, gpu.ErrDeviceLost
	}
	if fb.Rect.Min != (image.Point{}) {
		return Damage{}, fmt.Errorf("gpu: a framebuffer at %v, not at the origin", fb.Rect.Min)
	}
	if err := s.fit(fb.Rect.Size()); err != nil {
		return Damage{}, err
	}
	if !L.rasGPU {
		// The CPU rasterizer drew the tree's frames until now: what it
		// keeps between them describes pixels this path does not keep up
		// to date.
		L.bdc.Reset()
		L.dropSnaps()
	}
	canvas := t.clearColor()
	st := &s.Stats
	var d Damage
	switch {
	case full || !L.rasGPU || L.ras == nil || s.ras != L.ras || L.ras.scale != f.scale || L.rasRect != fb.Rect:
		d.Full = true
	case L.ras != f:
		L.dmg.bdc, L.dmg.snaps, L.dmg.canvas, L.dmg.finish = nil, nil, canvas, finishGPU
		d = frameDamage(L.ras, f, fb.Rect, &L.dmg)
		L.dmg.finish = nil
	}
	// Until the frame is whole, the target holds none.
	s.ras = nil
	render := func(region image.Rectangle) error {
		res, err := g.r.Render(s.target, f.dl, gpu.Options{Scale: f.scale, Region: region, Clear: &canvas})
		st.Renders++
		st.Encode += res.Encode
		st.Draw += res.Draw
		if err != nil {
			return err
		}
		if len(res.Unsupported) > 0 {
			return fmt.Errorf("gpu: %s cannot draw %s", g.dev.Name(), strings.Join(res.Unsupported, ", "))
		}
		return nil
	}
	read := func(rects ...image.Rectangle) error {
		t0 := time.Now()
		err := s.target.ReadPixels(fb, rects...)
		st.Read += time.Since(t0)
		for _, rc := range rects {
			st.Pixels += rc.Dx() * rc.Dy()
		}
		return err
	}
	switch {
	case d.Full:
		g.used = true
		if err := render(image.Rectangle{}); err != nil {
			return d, err
		}
		if err := read(fb.Rect); err != nil {
			return d, err
		}
	case !d.Empty():
		g.used = true
		// The scrolled pixels move on the device and, with the same
		// values, in the framebuffer: they are not read back.
		for _, sh := range d.Shifts {
			t0 := time.Now()
			err := s.target.Shift(sh.Rect, sh.DX, sh.DY)
			st.Shift += time.Since(t0)
			st.Shifts++
			if err != nil {
				return d, err
			}
			shiftPixels(fb, sh)
		}
		// A region is cleared to the canvas and drawn from the list's
		// start, with what its backdrop filters read beyond it (the
		// encoder grows it), so it does not depend on the pixels the
		// target holds: the rectangles may overlap, in any order.
		for _, rc := range d.Rects {
			if err := render(rc); err != nil {
				return d, err
			}
		}
		if err := read(d.Rects...); err != nil {
			return d, err
		}
	}
	s.ras = f
	L.ras, L.rasRect, L.damage, L.rasGPU = f, fb.Rect, d, true
	return d, nil
}

// gpuRectCost is what a damaged rectangle costs the GPU path whatever its
// size, in device pixels drawn or read back: a pass of the encoder over
// the frame's list and a submission that is waited for. Measured on the
// dashboard (examples/dashboard's TestSessionPerfGPU), that is about what
// 0.1 megapixels cost.
const gpuRectCost = 1 << 17

// gpuMaxRects bounds the rectangles of a GPU frame: beyond it, the pairs
// that cost the least to draw as one are merged.
const gpuMaxRects = 8

// finishGPU is damager.finish for the GPU path: the damaged rectangles
// merged into a few, without bases, and a full frame when they cost as
// much as one.
//
// The cost of a rectangle is the pixels its render draws, which are the
// rectangle grown by what the backdrop filters over it read
// (raster.BackdropRegion: the renderer has no cache of filtered
// backdrops, so a rectangle in a glass pane draws the pane's backdrop
// within the blur's reach again), the pixels read back, which are the
// rectangle, and gpuRectCost. Two rectangles are drawn as one when that
// costs no more: those of one pane share most of what they draw. The CPU
// path's fullFraction does not apply: a full frame is worth it only when
// the rectangles cost what the whole window does.
func finishGPU(d *damager, shifts []Shift) Damage {
	dl, scale := d.b.dl, d.b.scale
	cost := func(r image.Rectangle) int {
		g := raster.BackdropRegion(dl, scale, r, d.bounds)
		return g.Dx()*g.Dy() + r.Dx()*r.Dy() + gpuRectCost
	}
	rects := mergeGPU(mergeRects(d.rects), cost)
	total := 0
	for _, r := range rects {
		total += cost(r)
	}
	for _, s := range shifts {
		total += s.Rect.Dx() * s.Rect.Dy() / 8 // a shift costs two copies on the device and one in the framebuffer
	}
	if total >= 2*d.bounds.Dx()*d.bounds.Dy()+gpuRectCost {
		return Damage{Full: true}
	}
	return Damage{Shifts: shifts, Rects: rects}
}

// mergeGPU merges the pairs of rectangles whose union costs no more than
// the two apart, the pair that saves the most first, then the cheapest
// pairs until at most gpuMaxRects are left. It reuses rs.
func mergeGPU(rs []image.Rectangle, cost func(image.Rectangle) int) []image.Rectangle {
	costs := make([]int, len(rs))
	for i, r := range rs {
		costs[i] = cost(r)
	}
	for len(rs) > 1 {
		// extra is what drawing a pair as one adds to drawing it apart.
		bi, bj, best, bc := -1, -1, 0, 0
		for i := range rs {
			for j := i + 1; j < len(rs); j++ {
				c := cost(rs[i].Union(rs[j]))
				if extra := c - costs[i] - costs[j]; bi < 0 || extra < best {
					bi, bj, best, bc = i, j, extra, c
				}
			}
		}
		if best > 0 && len(rs) <= gpuMaxRects {
			break
		}
		rs[bi], costs[bi] = rs[bi].Union(rs[bj]), bc
		rs = append(rs[:bj], rs[bj+1:]...)
		costs = append(costs[:bj], costs[bj+1:]...)
	}
	return rs
}
