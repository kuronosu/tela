//go:build telagpu

package core

import (
	"fmt"
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/platform"
)

// A window's frames presented without a copy (ADR-281, ADR-283;
// gpuhost.go has the design).
//
// A GPUSwapchain is the buffers of one window: images the device exports
// as dmabufs (gpu.Export), each registered once with the window
// (platform.DmabufPresenter.CreateDmabuf). The surface's target stays the
// frame, drawn incrementally as before; a present takes a buffer the OS
// does not read, copies into it, on the device, what the frame has that
// the buffer lacks, and gives it to the window with the damage since the
// window's last present.
//
// Buffers lag the frame by different numbers of frames, so each keeps its
// own account (chainBuffer.stale): every frame's damage is added to every
// buffer's, and a buffer's is emptied when the frame is copied into it. A
// buffer is only ever written from the target, never read: what the OS
// did with its pixels meanwhile does not matter outside the rectangles
// copied, and a window that falls back to Window.Present (the X server
// may draw that into a buffer it flipped to) drops its buffers.
//
// There are as many buffers as the OS keeps busy plus one: two where the
// old frame is released when the new one is committed (Wayland), three
// where the server flips (X11). A present that finds every buffer busy
// registers another, up to chainMax; beyond that the frame is not
// presented, its damage is kept, and the caller tries again at the next
// frame. The buffers beyond chainKeep that no present used for chainIdle
// presents are destroyed, and all of those when the window rests (Trim).

// chainKeep is the number of buffers a swapchain keeps whatever their
// use: the one on screen and the one the next frame goes to.
const chainKeep = 2

// chainMax bounds the buffers of a swapchain. Three are what a server
// that flips needs; the fourth covers a release that is late by a frame.
const chainMax = 4

// chainIdle is the number of presents after which a buffer beyond
// chainKeep that none of them used is destroyed: two seconds of frames.
const chainIdle = 120

// chainMaxRects bounds the rectangles of a buffer's account: beyond it
// they are replaced by their bounding box.
const chainMaxRects = 16

// GPUPresent is what GPUSwapchain.Present did.
type GPUPresent int

const (
	// GPUPresentShown: a buffer holds the frame and the window was given
	// it.
	GPUPresentShown GPUPresent = iota
	// GPUPresentBusy: the OS still reads every buffer and there are as
	// many as a swapchain may have. Nothing was presented; the damage is
	// kept, and the caller presents again at the next frame.
	GPUPresentBusy
	// GPUPresentDropped: the window dropped the present (it is hidden).
	// The caller presents again when a frame comes, which the backend
	// asks for when the window is shown; that present is full.
	GPUPresentDropped
)

// GPUPresentStats describes the last GPUSwapchain.Present, and counts
// what the swapchain did since it was made.
type GPUPresentStats struct {
	// Copy is the time of the copy into the buffer, with its wait for the
	// GPU, Pixels the device pixels copied, and Present the time of the
	// window's PresentDmabuf; Register the time it took to create and
	// register a buffer, when the present needed one.
	Copy, Present, Register time.Duration
	Pixels                  int
	// Buffers is the number of buffers registered now.
	Buffers int
	// Shown, Busy and Dropped count the presents by what they did;
	// Created and Destroyed the buffers.
	Shown, Busy, Dropped, Created, Destroyed int
}

// chainDamage is what a frame has that something else lacks: everything,
// or rectangles.
type chainDamage struct {
	all   bool
	rects []image.Rectangle
}

// add adds the damage of a frame.
func (c *chainDamage) add(d *Damage, bounds image.Rectangle) {
	switch {
	case c.all:
	case d.Full:
		c.all, c.rects = true, nil
	default:
		for _, s := range d.Shifts {
			c.addRect(s.Rect.Intersect(bounds), bounds)
		}
		for _, r := range d.Rects {
			c.addRect(r.Intersect(bounds), bounds)
		}
	}
}

func (c *chainDamage) addRect(r, bounds image.Rectangle) {
	if c.all || r.Empty() {
		return
	}
	for _, have := range c.rects {
		if r.In(have) {
			return
		}
	}
	c.rects = slices.DeleteFunc(c.rects, func(have image.Rectangle) bool { return have.In(r) })
	c.rects = append(c.rects, r)
	if len(c.rects) > chainMaxRects {
		u := c.rects[0]
		for _, o := range c.rects[1:] {
			u = u.Union(o)
		}
		c.rects = append(c.rects[:0], u)
	}
	if len(c.rects) == 1 && c.rects[0] == bounds {
		c.all, c.rects = true, nil
	}
}

func (c *chainDamage) empty() bool { return !c.all && len(c.rects) == 0 }

// area returns the pixels to copy.
func (c *chainDamage) area(bounds image.Rectangle) int {
	if c.all {
		return bounds.Dx() * bounds.Dy()
	}
	n := 0
	for _, r := range c.rects {
		n += r.Dx() * r.Dy()
	}
	return n
}

// chainBuffer is one buffer of a swapchain.
type chainBuffer struct {
	export gpu.Export
	buf    platform.DmabufBuffer
	// stale is what the frame has that the buffer lacks.
	stale chainDamage
	// used is the swapchain's count of presents when the buffer was
	// presented last (or created).
	used int
}

// GPUSwapchain presents the frames of a GPUSurface in a window that shows
// dmabufs, without a copy through main memory. It belongs to the UI
// thread, like the window.
type GPUSwapchain struct {
	g   *GPU
	win platform.DmabufPresenter
	// serial is the Serial of the feedback accepted was made from (0:
	// none yet), and ready, why and expected the answer Ready gave for
	// it.
	serial          uint64
	accepted        []gpu.DmabufFormat
	fourcc          uint32
	modifiers       []uint64
	same            bool   // the feedback names the device, or none
	node            string // the feedback's device, for messages
	exportable, any bool   // a common format; a feedback and an exporter
	size            image.Point
	bufs            []*chainBuffer
	// owed is the damage of the window since its last present.
	owed chainDamage
	// Stats describes the last Present and counts all of them.
	Stats GPUPresentStats
}

// NewGPUSwapchain returns the swapchain of a window drawn by g. It
// registers nothing until its first Present.
func NewGPUSwapchain(g *GPU, win platform.DmabufPresenter) *GPUSwapchain {
	return &GPUSwapchain{g: g, win: win, owed: chainDamage{all: true}}
}

// Ready reports whether the window can be presented without a copy right
// now: it reports a dmabuf feedback, the device exports images in a format
// and a modifier the feedback lists, and the device is the one the
// feedback names, when it names one (presenting from another device is
// not attempted unless force is set: the X server refuses its buffers, and
// nothing verified it on Wayland). why says what is missing; expected is
// false when the window or the device simply has no such path (no
// feedback: a transparent window, a server without the protocol; a device
// that exports nothing), which is not worth a line of the log. It asks
// the window for its feedback, as a caller does once a frame, and drops
// the buffers when a new feedback changes the format they are made with.
func (c *GPUSwapchain) Ready(force bool) (ok bool, why string, expected bool) {
	fb, has := c.win.DmabufFeedback()
	if !has {
		c.serial = 0
		return false, "the window reports no dmabuf feedback (the display server lacks the protocol, or the window is transparent)", false
	}
	exp := c.g.Exporter()
	if exp == nil {
		return false, "the GPU device exports no dmabufs", false
	}
	if fb.Serial != c.serial {
		c.serial = fb.Serial
		c.accepted = c.accepted[:0]
		for _, f := range fb.Formats {
			c.accepted = append(c.accepted, gpu.DmabufFormat{Fourcc: f.Fourcc, Modifier: f.Modifier})
		}
		exportable := exp.ExportFormats()
		fourcc, modifiers, common := gpu.ChooseExport(c.accepted, exportable)
		if len(c.bufs) > 0 && (fourcc != c.fourcc || !slices.Equal(modifiers, c.modifiers)) {
			// The buffers are made differently from now on. The old ones
			// stay valid, but what the OS prefers now (a format it scans
			// out) is worth new ones.
			c.Release()
		}
		c.fourcc, c.modifiers, c.exportable, c.any = fourcc, modifiers, common, len(exportable) > 0
		c.same, c.node = true, ""
		if fb.HasDevice {
			c.same = slices.Contains(exp.DRMNodes(), gpu.DRMNode{Major: fb.DeviceMajor, Minor: fb.DeviceMinor})
			c.node = fmt.Sprintf("%d:%d", fb.DeviceMajor, fb.DeviceMinor)
		}
	}
	switch {
	case !c.any:
		return false, "the GPU device exports no dmabufs", false
	case !c.exportable:
		return false, fmt.Sprintf("%s exports no dmabuf in a format and a modifier the display server accepts", c.g.Name()), true
	case !c.same && !force:
		return false, fmt.Sprintf("the display server composites with another device (DRM node %s) than %s", c.node, c.g.Name()), true
	}
	return true, "", true
}

// Buffers returns the number of buffers registered.
func (c *GPUSwapchain) Buffers() int { return len(c.bufs) }

// Present brings a buffer the OS does not read to the frame s holds and
// gives it to the window. d is the damage of the frames drawn into s
// since the last Present (the Damage of RasterizeFrameGPU; an empty one
// when nothing was drawn), and full reports that the window must be
// given the whole frame (its first present after a ConfigureEvent, or
// after one that was dropped). Call Ready first, every frame. It returns
// what happened to the frame; after an error the caller releases the
// swapchain and presents the frame with Window.Present, read back.
func (c *GPUSwapchain) Present(s *GPUSurface, d Damage, full bool) (GPUPresent, error) {
	st := &c.Stats
	st.Copy, st.Present, st.Register, st.Pixels = 0, 0, 0, 0
	if s.target == nil {
		return 0, fmt.Errorf("gpu: a swapchain presents a surface that holds no frame")
	}
	size := s.target.Size()
	bounds := image.Rectangle{Max: size}
	if size != c.size {
		// Buffers have the size they were registered with.
		c.Release()
		c.size = size
	}
	for _, b := range c.bufs {
		b.stale.add(&d, bounds)
	}
	c.owed.add(&d, bounds)
	if full {
		c.owed = chainDamage{all: true}
	}
	if c.owed.empty() {
		return GPUPresentShown, nil // nothing changed since the window's last present
	}

	// The free buffer with the least to copy: the one presented most
	// recently, so that the ones a slower OS needed go unused and away.
	var best *chainBuffer
	for _, b := range c.bufs {
		if !b.buf.Busy() && (best == nil || b.stale.area(bounds) < best.stale.area(bounds)) {
			best = b
		}
	}
	if best == nil {
		if len(c.bufs) >= chainMax {
			st.Busy++
			return GPUPresentBusy, nil
		}
		t0 := time.Now()
		b, err := c.newBuffer(size)
		st.Register = time.Since(t0)
		if err != nil {
			return 0, err
		}
		best = b
	}

	rects := best.stale.rects
	if best.stale.all {
		rects = []image.Rectangle{bounds}
	}
	if len(rects) > 0 {
		t0 := time.Now()
		err := best.export.CopyFrom(s.target, rects...)
		st.Copy = time.Since(t0)
		if err != nil {
			return 0, c.g.note(err)
		}
		st.Pixels = best.stale.area(bounds)
		best.stale = chainDamage{}
	}

	var damage []image.Rectangle // nil: everything
	if !c.owed.all {
		damage = c.owed.rects
	}
	t0 := time.Now()
	err := c.win.PresentDmabuf(best.buf, damage)
	st.Present = time.Since(t0)
	if err != nil {
		return 0, err
	}
	if !best.buf.Busy() {
		// A present leaves its buffer free only when the window dropped
		// it: the frame is still owed, in full.
		c.owed = chainDamage{all: true}
		st.Dropped++
		return GPUPresentDropped, nil
	}
	c.owed = chainDamage{}
	st.Shown++
	best.used = st.Shown
	c.age()
	return GPUPresentShown, nil
}

// newBuffer creates an image of the size, exported, and registers it with
// the window. It holds no pixels.
func (c *GPUSwapchain) newBuffer(size image.Point) (*chainBuffer, error) {
	exp := c.g.Exporter()
	if exp == nil {
		return nil, gpu.ErrNoExport
	}
	e, err := exp.NewExport(size.X, size.Y, c.accepted)
	if err != nil {
		return nil, c.g.note(err)
	}
	f := e.Format()
	desc := platform.DmabufDescriptor{Size: size, Fourcc: f.Fourcc, Modifier: f.Modifier}
	for _, p := range e.Planes() {
		desc.Planes = append(desc.Planes, platform.DmabufPlane{FD: p.FD, Offset: p.Offset, Stride: p.Stride})
	}
	buf, err := c.win.CreateDmabuf(desc)
	if err != nil {
		e.Close()
		return nil, err
	}
	b := &chainBuffer{export: e, buf: buf, stale: chainDamage{all: true}, used: c.Stats.Shown}
	c.bufs = append(c.bufs, b)
	c.Stats.Created++
	c.Stats.Buffers = len(c.bufs)
	return b, nil
}

// destroy forgets buffer i: the window's side first (the OS keeps the
// memory of a frame it still shows), then the image.
func (c *GPUSwapchain) destroy(i int) {
	b := c.bufs[i]
	b.buf.Destroy()
	b.export.Close()
	c.bufs = slices.Delete(c.bufs, i, i+1)
	c.Stats.Destroyed++
	c.Stats.Buffers = len(c.bufs)
}

// age destroys one buffer beyond chainKeep that no present used for
// chainIdle presents and the OS does not read.
func (c *GPUSwapchain) age() {
	if len(c.bufs) <= chainKeep {
		return
	}
	for i, b := range c.bufs {
		if c.Stats.Shown-b.used > chainIdle && !b.buf.Busy() {
			c.destroy(i)
			return
		}
	}
}

// Trim destroys the buffers beyond the chainKeep presented most recently:
// for a window that rests. The frame on screen stays (the OS holds its
// memory), and the next presents go on with the buffers kept.
func (c *GPUSwapchain) Trim() {
	for len(c.bufs) > chainKeep {
		old := 0
		for i, b := range c.bufs {
			if b.used < c.bufs[old].used {
				old = i
			}
		}
		c.destroy(old)
	}
}

// Release destroys every buffer, busy or not: the window is hidden,
// minimized, resized or closed, or its frames are read back from now on.
// The next Present registers new ones and is full.
func (c *GPUSwapchain) Release() {
	for len(c.bufs) > 0 {
		c.destroy(len(c.bufs) - 1)
	}
	c.size = image.Point{}
	c.owed = chainDamage{all: true}
}
