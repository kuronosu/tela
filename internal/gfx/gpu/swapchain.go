package gpu

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"time"
)

// Presenting a target through a swapchain on a window (ADR-281, ADR-289):
// on Windows, a Vulkan swapchain on the window's HWND
// (VK_KHR_win32_surface), whose images the presentation engine and DWM
// show. This package does not know the platform layer: it is given the
// window's two handles, as plain values.
//
// A window's target stays the frame, drawn into, shifted and kept between
// frames as before. A present acquires an image of the swapchain without
// waiting, copies into it on the device what the frame has that the image
// lacks, and presents it with the rectangles that changed since the
// window's last present (VK_KHR_incremental_present, where the driver has
// it). Images lag the frame by different numbers of frames, so each keeps
// its own account of what it lacks, as the dmabuf buffers of a Linux
// window do (core's GPUSwapchain): every frame's damage is added to every
// image's, an image's is emptied when the frame is copied into it, and a
// new image (a swapchain made again) lacks everything. An image's pixels
// are only ever written from the target, by copies that keep the rest:
// the image goes back from the layout of presenting to the one of copying
// with its contents, which is what the account relies on.
//
// The Swapchain below is that logic, written against SwapchainOps (the
// calls into the device and the surface) and tested with a fake one; the
// Vulkan device implements them on Windows (vulkan_swapchain_windows.go).

// NativeWindow names an OS window to present to: on Windows the HINSTANCE
// of the module that registered its class and its HWND.
type NativeWindow struct {
	Instance, Window uintptr
}

// A WindowPresenter is a Device that presents its targets to OS windows
// through a swapchain on a surface of the window: the Vulkan device on
// Windows. A caller asks a Device for it with a type assertion.
type WindowPresenter interface {
	// NewSwapchain makes a surface of the window and returns its
	// Swapchain, which makes the swapchain itself at its first Present.
	// It fails when the device cannot present to the window (no
	// VK_KHR_swapchain, a queue that does not present to it, no format
	// the renderer writes), and the caller reads the frames back.
	NewSwapchain(w NativeWindow) (*Swapchain, error)
}

// SwapchainStatus is what a presentation engine answers an acquire or a
// present.
type SwapchainStatus int

const (
	// SwapchainOK: done.
	SwapchainOK SwapchainStatus = iota
	// SwapchainSuboptimal: done, and the swapchain should be made again
	// (VK_SUBOPTIMAL_KHR).
	SwapchainSuboptimal
	// SwapchainOutOfDate: not done, and the swapchain must be made again
	// (VK_ERROR_OUT_OF_DATE_KHR).
	SwapchainOutOfDate
	// SwapchainNotReady: no image is free right now (an acquire that does
	// not wait).
	SwapchainNotReady
)

// SurfaceInfo is what a window's surface allows now
// (VkSurfaceCapabilitiesKHR).
type SurfaceInfo struct {
	// Extent is the size a swapchain must have: the window's client area,
	// empty while it is minimized. AnyExtent reports that the surface takes
	// the swapchain's size instead (an extent of 0xFFFFFFFF in Vulkan; not
	// on Windows).
	Extent    image.Point
	AnyExtent bool
	// MinImages and MaxImages bound the images of a swapchain (MaxImages
	// 0: no bound).
	MinImages, MaxImages int
	// Incremental reports that a present can name the rectangles it
	// changed (VK_KHR_incremental_present).
	Incremental bool
}

// SwapchainOps are the calls a Swapchain makes into a device and a
// window's surface. The Vulkan device implements them on Windows; a test
// implements them with a fake.
type SwapchainOps interface {
	// Surface returns what the surface allows now.
	Surface() (SurfaceInfo, error)
	// Create makes the swapchain of the given extent with the given number
	// of images (the presentation engine may make more), replacing the
	// one there is, and returns the number of images it has. Their
	// pixels are unknown.
	Create(extent image.Point, images int) (int, error)
	// Acquire returns the index of an image the presentation engine does
	// not use, without waiting: SwapchainNotReady when there is none,
	// SwapchainOutOfDate without an image, SwapchainSuboptimal with one.
	Acquire() (int, SwapchainStatus, error)
	// Present copies the rectangles of t, a target of the swapchain's
	// extent, into the acquired image (the rest of the image is kept),
	// and presents it with damage, the rectangles that changed since the
	// window's last present (nil: everything; ignored without
	// SurfaceInfo.Incremental). An image acquired is presented, also
	// with no rectangle to copy.
	Present(t Target, image int, copy, damage []image.Rectangle) (SwapchainStatus, error)
	// DestroySwapchain destroys the swapchain, and keeps the surface.
	DestroySwapchain()
	// Close destroys the swapchain and the surface.
	Close()
}

// SwapchainResult is what Swapchain.Present did with a frame.
type SwapchainResult int

const (
	// SwapchainShown: the window was given the frame (or had it already).
	SwapchainShown SwapchainResult = iota
	// SwapchainRetry: the frame was not presented (no image was free, the
	// swapchain went out of date, the window has another size than the
	// frame); its damage is kept, and the caller presents again at the
	// next frame, which it asks for.
	SwapchainRetry
	// SwapchainHidden: the window is minimized and nothing is presented
	// to it; the damage is kept for when it comes back, which a
	// ConfigureEvent tells.
	SwapchainHidden
)

// SwapchainStats describes the last Swapchain.Present and counts what the
// swapchain did since it was made.
type SwapchainStats struct {
	// Images is the number of images of the swapchain now, 0 without one.
	Images int
	// Pixels are the device pixels the last Present copied, Rects the
	// rectangles it presented (0: the whole frame), and Time what it took,
	// the copy's wait for the GPU and the present included.
	Pixels, Rects int
	Time          time.Duration
	// Created counts the swapchains made, Shown the frames presented,
	// Retried and Hidden the Presents that returned SwapchainRetry and
	// SwapchainHidden, OutOfDate the answers that made a swapchain again.
	Created, Shown, Retried, Hidden, OutOfDate int
}

// maxMismatch is the number of Presents in a row that may find the window
// at one size other than the frame's before Present fails: a resize is
// over within a frame or two, and a window that stays at another size
// than its frames is a mistake that reading back does not have.
const maxMismatch = 30

// maxSwapchainRects bounds the rectangles of an account: beyond it they
// are replaced by their bounding box.
const maxSwapchainRects = 16

// ErrSwapchainClosed is returned by a Present after Close.
var ErrSwapchainClosed = errors.New("gpu: the swapchain is closed")

// Swapchain presents a window's target through a swapchain on the
// window's surface (see above). It belongs to the UI thread, like the
// window.
type Swapchain struct {
	ops SwapchainOps
	// extent is the swapchain's, zero without one; remake asks for a new
	// one at the next Present.
	extent image.Point
	remake bool
	// stale is what the frame has that each image lacks, and owed what
	// it has that the window was not given since its last present.
	stale []rects
	owed  rects
	// mismatched is the window's size the last Presents found unlike the
	// frame's, mismatches how many in a row.
	mismatched image.Point
	mismatches int
	closed     bool
	// last and presented are LastPresent's.
	last      []image.Rectangle
	presented bool
	// Stats describes the last Present and counts all of them.
	Stats SwapchainStats
}

// NewSwapchainOf returns the Swapchain of a window whose surface and
// device ops calls into. It makes nothing until its first Present. A
// WindowPresenter calls it; a test gives it a fake.
func NewSwapchainOf(ops SwapchainOps) *Swapchain {
	return &Swapchain{ops: ops, owed: rects{all: true}}
}

// Images returns the number of images of the swapchain, 0 without one.
func (s *Swapchain) Images() int { return len(s.stale) }

// LastPresent returns the rectangles the last Present presented (nil: the
// whole frame), and false when it presented nothing (the window had the
// frame, or the result was not SwapchainShown). The window is told of
// them.
func (s *Swapchain) LastPresent() (damage []image.Rectangle, ok bool) {
	return s.last, s.presented
}

// Present brings an image of the swapchain to the frame t holds and
// presents it. damage is what changed in t since the last Present (every
// frame drawn into t is given, presented or not; all reports that the
// whole frame changed), and whole that the window must be given the whole
// frame, changed or not (its first present, one after a repaint the OS
// asked for). It returns what happened to the frame; after an error the
// caller closes the swapchain and presents with the platform's Present,
// reading the frame back.
func (s *Swapchain) Present(t Target, damage []image.Rectangle, all, whole bool) (SwapchainResult, error) {
	st := &s.Stats
	st.Pixels, st.Rects, st.Time = 0, 0, 0
	s.last, s.presented = nil, false
	if s.closed {
		return 0, ErrSwapchainClosed
	}
	t0 := time.Now()
	defer func() { st.Time = time.Since(t0) }()
	size := t.Size()
	bounds := image.Rectangle{Max: size}
	for i := range s.stale {
		s.stale[i].add(damage, all, bounds)
	}
	s.owed.add(damage, all, bounds)
	if whole {
		s.owed = rects{all: true}
	}
	if s.owed.empty() {
		return SwapchainShown, nil // the window has the frame
	}
	for range 2 {
		info, err := s.ops.Surface()
		if err != nil {
			return 0, err
		}
		if info.AnyExtent {
			info.Extent = size
		}
		switch {
		case info.Extent.X <= 0 || info.Extent.Y <= 0:
			st.Hidden++
			return SwapchainHidden, nil
		case info.Extent != size:
			if info.Extent != s.mismatched {
				s.mismatched, s.mismatches = info.Extent, 0
			}
			if s.mismatches++; s.mismatches > maxMismatch {
				return 0, fmt.Errorf("gpu: the window's surface stays at %v and its frames at %v", info.Extent, size)
			}
			st.Retried++
			return SwapchainRetry, nil
		}
		s.mismatched, s.mismatches = image.Point{}, 0
		fresh := false
		if s.extent != size || s.remake || len(s.stale) == 0 {
			if err := s.create(info, size); err != nil {
				return 0, err
			}
			fresh = true
		}
		index, status, err := s.ops.Acquire()
		if err != nil {
			return 0, err
		}
		switch status {
		case SwapchainNotReady:
			st.Retried++
			return SwapchainRetry, nil
		case SwapchainOutOfDate:
			st.OutOfDate++
			s.remake = true
			continue // made again, at the size the surface has now
		case SwapchainSuboptimal:
			// Made again after this present, unless it was just made: a
			// surface that is never optimal would be made every frame.
			s.remake = !fresh
		}
		if index < 0 || index >= len(s.stale) {
			return 0, fmt.Errorf("gpu: the swapchain acquired the image %d of %d", index, len(s.stale))
		}
		img := &s.stale[index]
		copies := img.list
		if img.all {
			copies = []image.Rectangle{bounds}
		}
		var presented []image.Rectangle // nil: everything
		if info.Incremental && !s.owed.all {
			presented = s.owed.list
		}
		status, err = s.ops.Present(t, index, copies, presented)
		if err != nil {
			return 0, err
		}
		pixels := img.area(bounds)
		*img = rects{}
		switch status {
		case SwapchainOutOfDate:
			// Not shown: the frame is owed whole to the swapchain made
			// again, whose images lack everything.
			st.OutOfDate++
			s.remake = true
			s.owed = rects{all: true}
			st.Retried++
			return SwapchainRetry, nil
		case SwapchainSuboptimal:
			s.remake = !fresh
		}
		st.Pixels, st.Rects = pixels, len(presented)
		s.last, s.presented = slices.Clone(presented), true
		s.owed = rects{}
		st.Shown++
		return SwapchainShown, nil
	}
	st.Retried++
	return SwapchainRetry, nil
}

// create makes the swapchain at size, with the images the surface asks
// for and one more, so that one is free while one is shown and another
// waits for the vertical blank.
func (s *Swapchain) create(info SurfaceInfo, size image.Point) error {
	n := max(info.MinImages+1, 2)
	if info.MaxImages > 0 {
		n = min(n, info.MaxImages)
	}
	got, err := s.ops.Create(size, n)
	if err != nil {
		s.extent, s.stale = image.Point{}, nil
		return err
	}
	if got < 1 {
		return fmt.Errorf("gpu: a swapchain of %d images", got)
	}
	s.extent, s.remake = size, false
	s.stale = make([]rects, got)
	for i := range s.stale {
		s.stale[i] = rects{all: true}
	}
	s.owed = rects{all: true}
	s.Stats.Created++
	s.Stats.Images = got
	return nil
}

// Release destroys the swapchain and keeps the surface: the window is
// hidden, or its frames are read back from now on. The next Present makes
// a swapchain and is whole.
func (s *Swapchain) Release() {
	if s.closed {
		return
	}
	if len(s.stale) > 0 || s.extent != (image.Point{}) {
		s.ops.DestroySwapchain()
	}
	s.extent, s.stale, s.remake = image.Point{}, nil, false
	s.owed = rects{all: true}
	s.Stats.Images = 0
}

// Close destroys the swapchain and the surface: before the window closes,
// and when its device goes. It may be called more than once.
func (s *Swapchain) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.ops.Close()
	s.extent, s.stale = image.Point{}, nil
	s.Stats.Images = 0
}

// rects is what a frame has that something else lacks: everything, or
// rectangles.
type rects struct {
	all  bool
	list []image.Rectangle
}

// add adds the damage of a frame.
func (c *rects) add(damage []image.Rectangle, all bool, bounds image.Rectangle) {
	if c.all {
		return
	}
	if all {
		c.all, c.list = true, nil
		return
	}
	for _, r := range damage {
		c.addRect(r.Intersect(bounds), bounds)
	}
}

func (c *rects) addRect(r, bounds image.Rectangle) {
	if c.all || r.Empty() {
		return
	}
	for _, have := range c.list {
		if r.In(have) {
			return
		}
	}
	c.list = slices.DeleteFunc(c.list, func(have image.Rectangle) bool { return have.In(r) })
	c.list = append(c.list, r)
	if len(c.list) > maxSwapchainRects {
		u := c.list[0]
		for _, o := range c.list[1:] {
			u = u.Union(o)
		}
		c.list = append(c.list[:0], u)
	}
	if len(c.list) == 1 && c.list[0] == bounds {
		c.all, c.list = true, nil
	}
}

func (c *rects) empty() bool { return !c.all && len(c.list) == 0 }

// area returns the pixels to copy.
func (c *rects) area(bounds image.Rectangle) int {
	if c.all {
		return bounds.Dx() * bounds.Dy()
	}
	n := 0
	for _, r := range c.list {
		n += r.Dx() * r.Dy()
	}
	return n
}
