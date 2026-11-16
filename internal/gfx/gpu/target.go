package gpu

import "image"

// Offscreen targets: the surfaces of layers and masks, the steps of
// filters, the copies of backdrops. An Encoder keeps a pool of them on the
// device, as it keeps the atlases: a frame lists the targets that exist
// (Frame.Targets), and its passes draw into them and read them by ID.
//
// A target's pixels live for the frame only: every use starts by clearing
// what will be read or by writing all of it. What lasts is the texture,
// so that a layer drawn frame after frame does not create one each time.
// Sizes are rounded up to a few steps, so that a layer that grows or
// shrinks a little finds its texture again; a target drawn into is used
// from its first pixel, whatever its size.
//
// The frames are executed in the order they are encoded and every pass
// that draws into a target comes after the passes encoded before it, so a
// target is free for another use as soon as the last instance that reads
// it is encoded: a frame of many layers one after the other needs the
// targets of one.

// poolTarget is an offscreen target of the pool.
type poolTarget struct {
	id     int
	w, h   int
	format TargetFormat
	busy   bool   // in use by the frame being encoded
	used   uint64 // the frame that used it last
	idle   uint16 // frames ended since its last use (EndFrame)
}

func (t *poolTarget) bytes() int64 { return int64(t.w) * int64(t.h) * int64(t.format.bytesPerPixel()) }

// minTargetDim is the smallest width and height of a pooled target.
const minTargetDim = 64

// bucketDim rounds a width or a height up to a size of the pool: steps of
// an eighth to a sixteenth, at least minTargetDim, at most limit (which n
// does not exceed).
func bucketDim(n, limit int) int {
	if n <= minTargetDim {
		return min(minTargetDim, max(limit, n))
	}
	q := 8
	for q*16 <= n {
		q *= 2
	}
	return min((n+q-1)/q*q, max(limit, n))
}

// acquireTarget returns a target of at least size pixels in the format,
// free for the frame being encoded to use: one of the pool, or a new one.
// The caller releases it with releaseTarget when the last instance that
// reads it is encoded. ok is false when the device's textures are smaller
// than size.
func (e *Encoder) acquireTarget(size image.Point, format TargetFormat) (*poolTarget, bool) {
	r := e.res
	lim := r.limits.MaxTextureSize
	if size.X < 1 || size.Y < 1 || size.X > lim || size.Y > lim {
		return nil, false
	}
	w, h := bucketDim(size.X, lim), bucketDim(size.Y, lim)
	for _, t := range r.targets {
		if !t.busy && t.w == w && t.h == h && t.format == format {
			t.busy, t.used = true, r.frame
			return t, true
		}
	}
	r.nextTarget++
	t := &poolTarget{id: r.nextTarget, w: w, h: h, format: format, busy: true, used: r.frame}
	r.targets = append(r.targets, t)
	r.targetBytes += t.bytes()
	// Over the budget, the targets this frame does not use go, the ones
	// unused for longest first. (A frame that needs more than the budget
	// keeps what it draws with.)
	for r.targetBytes > r.limits.TargetBytes {
		old := -1
		for i, o := range r.targets {
			if !o.busy && o.used != r.frame && (old < 0 || o.used < r.targets[old].used) {
				old = i
			}
		}
		if old < 0 {
			break
		}
		r.dropTarget(old)
	}
	return t, true
}

// releaseTarget gives a target back to the pool. nil is ignored.
func (e *Encoder) releaseTarget(t *poolTarget) {
	if t != nil {
		t.busy = false
	}
}

// dropTarget removes target i from the pool: the frames that follow do not
// list it, and the device destroys it.
func (r *resources) dropTarget(i int) {
	r.targetBytes -= r.targets[i].bytes()
	last := len(r.targets) - 1
	r.targets[i] = r.targets[last]
	r.targets[last] = nil
	r.targets = r.targets[:last]
}

// endFrameTargets ages the targets: those no frame used since mark, the
// frame of the previous EndFrame, are idle for one frame more, and those
// idle for more than idleFrames are given up.
func (r *resources) endFrameTargets(mark uint64) {
	for i := 0; i < len(r.targets); i++ {
		t := r.targets[i]
		if t.used > mark {
			t.idle = 0
		} else if t.idle++; t.idle > idleFrames {
			r.dropTarget(i)
			i--
		}
	}
}

// listTargets appends the targets of the pool to the frame's list.
func (r *resources) listTargets(list []TargetInfo) []TargetInfo {
	for _, t := range r.targets {
		list = append(list, TargetInfo{ID: t.id, Width: t.w, Height: t.h, Format: t.format})
	}
	return list
}
