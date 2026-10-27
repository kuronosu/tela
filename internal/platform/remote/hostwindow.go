package remote

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/kuronosu/tela/internal/platform"
)

// realWin is a window of the host's backend and the child windows mapped
// onto it (one per child, all with the same window ID). UI thread only.
type realWin struct {
	id      uint32 // the window ID in the child that opened it
	key     string // what a child window must have to take it over
	w       platform.Window
	opts    platform.WindowOptions // it was created with (after HostOptions.WindowOptions)
	parent  *realWin
	closed  bool
	metrics platform.Metrics     // of the latest delivered ConfigureEvent
	state   platform.WindowState // of the latest delivered WindowStateEvent
	token   string               // the latest RestoreToken sent to children
	// wasActive: an active child had it (it shows that child's last frame
	// and is kept for the error overlay when no child uses it).
	wasActive bool

	// What the host last asked of w on behalf of the active child (window
	// control requests that are state; see applyWindow).
	visible  bool
	onTop    bool
	min, max platform.Size
	icon     []image.Image
	regions  []platform.HitRegion

	active  *childWin   // shown, receives input
	pending []*childWin // render offscreen
	// src is the child window whose frame is shown: the active one, or
	// the last active one after it was detached (its last frame stays).
	src *childWin

	full      bool // the next present is full
	direct    bool // the last present came straight from src's frame
	focused   bool
	textInput *platform.TextInputState // the state set on w
	pointer   *platform.PointerEvent   // the latest pointer event while inside
	covered   bool                     // the pointer is over ground HostOptions.Input consumes
	out       *image.RGBA              // composition buffer (resampling, overlay)
	smp       *sampler                 // cached for out
}

// attached returns the child windows that get window-state events.
func (rw *realWin) attached() []*childWin {
	if rw.active == nil {
		return rw.pending
	}
	return append([]*childWin{rw.active}, rw.pending...)
}

// flush presents rw's frame if it has something new: damage of its
// source, or a full present owed (configure, promotion, overlay). The
// frame is presented as is when it has the window's physical size and no
// overlay is set; otherwise it is composed into rw.out (resampled with
// nearest-neighbor sampling, then the overlay). A live child's frame whose
// logical size differs from the window's is not shown: the child is about
// to present one of the new size (configure is in flight); a detached
// child's is, top-left aligned at its scale, so the window still shows
// the last good frame after a resize.
func (h *Host) flush(rw *realWin) {
	if rw.closed || rw.metrics.Scale == 0 {
		return
	}
	dst := rw.metrics.Physical()
	if dst.X <= 0 || dst.Y <= 0 {
		return
	}
	src := rw.src
	if src == nil {
		if h.overlay != nil && rw.full {
			h.report(h.compose(rw, nil, platform.Metrics{}, false, nil, true))
			rw.full, rw.direct = false, false
		}
		return
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	frame := src.frame
	if frame == nil {
		if h.overlay != nil && rw.full {
			h.report(h.compose(rw, nil, platform.Metrics{}, false, nil, true))
			rw.full, rw.direct = false, false
		}
		return
	}
	live := !src.closed && src.c.state != stateDetached
	match := src.frameMetrics.Size == rw.metrics.Size
	if !match && live && h.overlay == nil {
		return
	}
	full := rw.full || src.full
	if !full && len(src.damage) == 0 {
		return
	}
	direct := h.overlay == nil && frame.Rect.Max == dst
	if direct != rw.direct {
		full = true
	}
	var err error
	if direct {
		var dmg []image.Rectangle
		if !full {
			dmg = src.damage
		}
		err = rw.w.Present(frame, dmg)
	} else {
		err = h.compose(rw, frame, src.frameMetrics, match, src.damage, full)
	}
	h.report(err)
	rw.direct = direct
	src.damage = src.damage[:0]
	src.full = false
	rw.full = false
}

func (h *Host) report(err error) {
	if err != nil {
		h.reportError(fmt.Errorf("remote: present: %w", err))
	}
}

// compose draws frame (nil: black) into rw.out, then the overlay, and
// presents rw.out. fm is the frame's metrics; match says whether its
// logical size is the window's.
func (h *Host) compose(rw *realWin, frame *image.RGBA, fm platform.Metrics, match bool, damage []image.Rectangle, full bool) error {
	dst := rw.metrics.Physical()
	if h.overlay != nil {
		full = true
	}
	if rw.out == nil || rw.out.Rect.Max != dst {
		rw.out = image.NewRGBA(image.Rectangle{Max: dst})
		full = true
	}
	var rects []image.Rectangle
	if frame == nil {
		fillBlack(rw.out, rw.out.Rect)
	} else {
		key := samplerKey{src: frame.Rect.Max, dst: dst, stretch: match, srcScale: fm.Scale, dstScale: rw.metrics.Scale}
		if rw.smp == nil || rw.smp.key != key {
			rw.smp = newSampler(key)
		}
		if full {
			rects = []image.Rectangle{rw.out.Rect}
		} else {
			rects = rw.smp.mapRects(damage)
			if len(rects) == 0 {
				return nil
			}
		}
		rw.smp.resample(rw.out, frame, rects)
	}
	if h.overlay != nil {
		h.overlay(rw.w, rw.out)
	}
	if full {
		rects = nil
	}
	return rw.w.Present(rw.out, rects)
}

// samplerKey identifies a resampling: frame size, output size, and how
// they relate.
type samplerKey struct {
	src, dst image.Point
	// stretch: the frame covers the window (same logical size), so each
	// axis maps by the ratio of the physical sizes. Otherwise it maps by
	// the ratio of the scales, from the top-left, and pixels outside the
	// frame are black.
	stretch            bool
	srcScale, dstScale float64
}

// sampler is a nearest-neighbor mapping from output pixels to frame
// pixels: output column x samples column xs[x] (-1: outside, black), and
// likewise for rows. Both maps are non-decreasing, with any -1 at the end.
type sampler struct {
	key      samplerKey
	xs, ys   []int32
	identity bool
}

func newSampler(k samplerKey) *sampler {
	s := &sampler{
		key: k,
		xs:  axisMap(k.src.X, k.dst.X, k),
		ys:  axisMap(k.src.Y, k.dst.Y, k),
	}
	s.identity = k.src == k.dst
	return s
}

// axisMap samples each output pixel's center: output pixel i covers
// [i, i+1) and takes the frame pixel under i + 0.5.
func axisMap(srcN, dstN int, k samplerKey) []int32 {
	m := make([]int32, dstN)
	ratio := k.srcScale / k.dstScale
	if k.stretch {
		ratio = float64(srcN) / float64(dstN)
	}
	for i := range m {
		s := int(math.Floor((float64(i) + 0.5) * ratio))
		if s >= srcN {
			s = -1
		}
		m[i] = int32(s)
	}
	return m
}

// first returns the first output index whose sample is >= v (outside
// samples count as +inf).
func first(m []int32, v int) int {
	return sort.Search(len(m), func(i int) bool { return m[i] < 0 || int(m[i]) >= v })
}

// mapRects returns the output pixels that sample the frame rectangles: the
// exact damage of the resampled output.
func (s *sampler) mapRects(rects []image.Rectangle) []image.Rectangle {
	if s.identity {
		return rects
	}
	out := make([]image.Rectangle, 0, len(rects))
	for _, r := range rects {
		o := image.Rect(first(s.xs, r.Min.X), first(s.ys, r.Min.Y), first(s.xs, r.Max.X), first(s.ys, r.Max.Y))
		if !o.Empty() {
			out = append(out, o)
		}
	}
	return out
}

// resample writes the output pixels of rects (output coordinates) from
// frame.
func (s *sampler) resample(dst, frame *image.RGBA, rects []image.Rectangle) {
	if s.identity {
		for _, r := range rects {
			platform.CopyRGBA(dst, frame, r)
		}
		return
	}
	for _, r := range rects {
		x0, x1 := 4*r.Min.X, 4*r.Max.X
		for y := r.Min.Y; y < r.Max.Y; y++ {
			drow := dst.Pix[y*dst.Stride : y*dst.Stride+4*dst.Rect.Dx()]
			sy := s.ys[y]
			if y > r.Min.Y && sy == s.ys[y-1] {
				// Same source row as the previous output row (upscaling).
				copy(drow[x0:x1], dst.Pix[(y-1)*dst.Stride+x0:(y-1)*dst.Stride+x1])
				continue
			}
			if sy < 0 {
				fillBlack(dst, image.Rect(r.Min.X, y, r.Max.X, y+1))
				continue
			}
			srow := frame.Pix[int(sy)*frame.Stride:]
			for x := r.Min.X; x < r.Max.X; x++ {
				v := uint32(opaqueBlack)
				if sx := s.xs[x]; sx >= 0 {
					v = binary.LittleEndian.Uint32(srow[4*sx:])
				}
				binary.LittleEndian.PutUint32(drow[4*x:], v)
			}
		}
	}
}

// opaqueBlack is RGBA (0, 0, 0, 255) read as a little-endian uint32.
const opaqueBlack = 0xff000000

func fillBlack(img *image.RGBA, r image.Rectangle) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
		for i := 0; i < len(row); i += 4 {
			binary.LittleEndian.PutUint32(row[i:], opaqueBlack)
		}
	}
}
