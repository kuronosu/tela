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
// TELA_GPU_DEVICE, else the first discrete device, else the first
// integrated one). With TELA_VK_VALIDATE=1
// the device runs under the API's validation layer, whose messages go to
// log. It returns an error when the build has no GPU backend
// (gpu.ErrUnavailable) or no device initializes: the caller then draws
// with the CPU rasterizer.
func OpenGPU(log func(msg string)) (*GPU, error) { return OpenGPUWith(gpu.Config{}, log) }

// OpenGPUWith is OpenGPU with a choice of the device: the display server's
// (cfg.DRM), and only a hardware one (cfg.HardwareOnly, which fails with
// gpu.ErrSoftwareDevice).
func OpenGPUWith(cfg gpu.Config, log func(msg string)) (*GPU, error) {
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

// Exporter returns the device as one that exports images as dmabufs, for
// a window presented without a copy (gpuchain.go); nil when it is not one
// (no Linux, or a device without the extensions) or is lost.
func (g *GPU) Exporter() gpu.Exporter {
	if g.lost || g.dev == nil {
		return nil
	}
	exp, _ := g.dev.(gpu.Exporter)
	return exp
}

// WindowPresenter returns the device as one that presents to windows
// through a swapchain on their surfaces, for a Win32 window (gpuwin.go);
// nil when it is not one (not Windows) or is lost.
func (g *GPU) WindowPresenter() gpu.WindowPresenter {
	if g.lost || g.dev == nil {
		return nil
	}
	wp, _ := g.dev.(gpu.WindowPresenter)
	return wp
}

// NewSurface returns the surface of a window's frames. Its target is
// created by the first frame.
func (g *GPU) NewSurface() *GPUSurface { return &GPUSurface{g: g} }

// TrimDue reports whether the Renderer may keep memory that Trim
// releases: a frame was drawn since the last one.
func (g *GPU) TrimDue() bool { return g.used && !g.lost }

// Trim releases what the Renderer keeps only to draw the next frame
// faster, on the device too (gpu.Renderer.Rest), for windows that rest:
// the offscreen targets of layers and filters, the buffers of uploads and
// readbacks, and the textures of the images that none of lists, the frames
// the GPU's windows show, draws. The surfaces keep their pixels, and what
// is costly to make again stays, within its limits: the atlases of glyphs
// and path coverage, the images on screen and the filtered backdrops. The
// next frame uploads nothing for what the frames before the rest drew
// (ADR-241's rule: memory that only saves an allocation goes, memory that
// saves work stays).
func (g *GPU) Trim(lists ...*gfx.DisplayList) {
	g.used = false
	if !g.lost {
		g.r.Rest(lists...)
	}
}

// Memory returns what the Renderer holds on the device (gpu.Memory).
func (g *GPU) Memory() gpu.Memory { return g.r.Memory() }

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
	// direct: the frames are presented from the target (gpuchain.go), so
	// they are not read back into the framebuffer. behind: the target
	// holds pixels the framebuffer does not, drawn by direct frames.
	direct, behind bool
	// bdc caches the filtered backdrops of ras in the target
	// (gpu.BackdropCache), as layerState.bdc caches them for the CPU
	// rasterizer: the damage of two frames tells it which filters kept
	// their input.
	bdc gpu.BackdropCache
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
	// Instances, Draws and Passes count what the renders drew, Uploads the
	// pictures and images they added to the device's textures and
	// UploadBytes their bytes (gpu.Result): a frame that draws what earlier
	// frames drew uploads nothing.
	Instances, Draws, Passes, Uploads, UploadBytes int
	// Culled counts the commands the renders did not draw because an
	// opaque command after them hides them, and Backdrops what they did
	// with the backdrop filters of the surface's cache (gpu.Result).
	Culled    int
	Backdrops gpu.BackdropStats
	// Encode is the time the encoder took on the CPU, Draw the time the
	// device took to draw, Shift to move the scrolled pixels and Read to
	// copy the damage back into the framebuffer, each with its wait for
	// the GPU.
	Encode, Draw, Shift, Read time.Duration
	// Fallback reports that the GPU failed and the CPU rasterizer drew the
	// frame.
	Fallback bool
	// Direct reports that the frame was not read back: it is presented
	// from the target (GPUSwapchain).
	Direct bool
}

// Release destroys the surface's target: for a window that is hidden,
// minimized or closed. The next frame creates one and is drawn in full.
func (s *GPUSurface) Release() {
	if s.target != nil {
		s.target.Close()
		s.target = nil
	}
	s.ras = nil
	s.bdc.Reset()
}

// BackdropBytes returns the bytes of filtered backdrops the surface's
// cache holds on the device.
func (s *GPUSurface) BackdropBytes() int64 { return s.bdc.Bytes() }

// Held reports whether the surface has a target.
func (s *GPUSurface) Held() bool { return s.target != nil }

// SetDirect chooses what RasterizeFrameGPU does with the pixels it draws.
// Off, the default, it reads them back, so the framebuffer holds the
// frame, for Window.Present. On, the frame stays in the target, which a
// GPUSwapchain presents without a copy, and the framebuffer is left
// behind until a reader asks for it (Sync) or the surface is no longer
// direct: the first frame after that is drawn and read back in full. With
// the debug overlays on, which the CPU rasterizer draws over the frame
// read back, a frame is read back whatever SetDirect says.
func (s *GPUSurface) SetDirect(on bool) { s.direct = on }

// Direct reports the value of the last SetDirect.
func (s *GPUSurface) Direct() bool { return s.direct }

// Behind reports whether the framebuffer of the last RasterizeFrameGPU
// lacks pixels of the frame: direct frames were drawn since it was read
// back. It stays so when the target is released, until a frame is read
// back.
func (s *GPUSurface) Behind() bool { return s.behind }

// Sync brings fb, the framebuffer of the surface's frames, to the frame
// the target holds, when direct frames left it behind: the whole target is
// read back. For what reads a window's pixels once in a while (a drag
// image). It does nothing when fb is current, and cannot do anything once
// the target was released (Behind still reports true).
func (s *GPUSurface) Sync(fb *image.RGBA) error {
	if !s.behind || s.target == nil {
		return nil
	}
	if fb.Rect != (image.Rectangle{Max: s.target.Size()}) {
		return fmt.Errorf("gpu: a framebuffer of %v beside a target of %v", fb.Rect, s.target.Size())
	}
	if err := s.target.ReadPixels(fb, fb.Rect); err != nil {
		return s.g.note(err)
	}
	s.behind = false
	return nil
}

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
		s.behind = false // the CPU rasterizer draws the whole of dst
		return t.rasterizeApp(dst, r, true)
	}
	var d Damage
	if t.dbg.flags == 0 {
		d = draw(fb, full)
	} else {
		// The overlays are drawn over the frame read back.
		direct := s.direct
		s.direct = false
		d = t.rasterizeDebug(fb, r, full, draw)
		s.direct = direct
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
	st.Direct = s.direct
	var d Damage
	switch {
	case full || !L.rasGPU || L.ras == nil || s.ras != L.ras || L.ras.scale != f.scale || L.rasRect != fb.Rect,
		// Direct frames left fb behind, and this one is read back.
		!s.direct && s.behind:
		// A full frame without the damage of two frames: nothing is known
		// of what the cache of filtered backdrops holds.
		d.Full = true
		s.bdc.Reset()
	case L.ras != f:
		// The damage tells the cache of filtered backdrops which filters
		// kept their input, before the cost of its rectangles is counted:
		// those are drawn from the cache.
		L.dmg.bdc, L.dmg.snaps, L.dmg.canvas = nil, nil, canvas
		L.dmg.finish = func(dm *damager, shifts []Shift) Damage {
			s.bdc.Retain(f.dl, f.scale, L.dmg.carry, L.dmg.patch)
			return finishGPU(dm, shifts, &s.bdc)
		}
		d = frameDamage(L.ras, f, fb.Rect, &L.dmg)
		L.dmg.finish = nil
	}
	// Until the frame is whole, the target holds none.
	s.ras = nil
	render := func(region image.Rectangle) error {
		res, err := g.r.Render(s.target, f.dl, gpu.Options{Scale: f.scale, Region: region, Clear: &canvas, Backdrops: &s.bdc})
		st.Renders++
		st.Encode += res.Encode
		st.Draw += res.Draw
		st.Instances += res.Instances
		st.Draws += res.Draws
		st.Passes += res.Passes
		st.Uploads += res.Uploads
		st.UploadBytes += res.UploadBytes
		st.Culled += res.Culled
		st.Backdrops.Hits += res.Backdrops.Hits
		st.Backdrops.Patched += res.Backdrops.Patched
		st.Backdrops.Stored += res.Backdrops.Stored
		st.Backdrops.Computed += res.Backdrops.Computed
		if err != nil {
			return err
		}
		if len(res.Unsupported) > 0 {
			return fmt.Errorf("gpu: %s cannot draw %s", g.dev.Name(), strings.Join(res.Unsupported, ", "))
		}
		return nil
	}
	read := func(rects ...image.Rectangle) error {
		if s.direct {
			// The frame is presented from the target; fb is left behind.
			s.behind = true
			return nil
		}
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
		s.behind = s.direct
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
			if !s.direct {
				shiftPixels(fb, sh)
			}
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
// rectangle grown by what the backdrop filters over it that are computed
// read (gpu.BackdropCache.Region: those whose filtered backdrop is in bdc,
// the surface's cache, read nothing), the pixels read back, which are the
// rectangle, and gpuRectCost. Two rectangles are drawn as one when that
// costs no more: those of one pane share most of what they draw. The CPU
// path's fullFraction does not apply: a full frame is worth it only when
// the rectangles cost what the whole window does.
func finishGPU(d *damager, shifts []Shift, bdc *gpu.BackdropCache) Damage {
	dl, scale := d.b.dl, d.b.scale
	cost := func(r image.Rectangle) int {
		g := bdc.Region(dl, scale, r, d.bounds)
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
