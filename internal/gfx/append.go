package gfx

// Composition of display lists: markers, tags and AppendList. A layer
// tree (package core) records each layer into a list of its own, with a
// Marker where a child layer goes, and composes a frame by appending the
// lists into one Builder.

// Marker records a placeholder with the given ID: it paints nothing, and
// AppendList hands it to its callback, which usually appends the list it
// stands for. Its bounds are the clip active here (the whole plane outside
// any clip), so the blocks around it are kept.
func (b *Builder) Marker(id uint32) {
	b.begin()
	if b.suppressed() {
		return
	}
	b.record(Marker{id}, b.clip)
}

// SetTag sets the tag of the commands recorded from now on (see
// DisplayList.Tag; push and pop pairs carry the tag of their push) and
// returns the previous one. Finish resets it to 0.
func (b *Builder) SetTag(tag uint32) uint32 {
	old := b.tag
	b.tag = tag
	return old
}

// AppendList records the commands of dl here, under the current transform
// and clip, as if they had been recorded by the methods that recorded dl:
// the root space of dl becomes the current local space. The commands keep
// their values and share their slices (lists are immutable); each one's
// bounds are its bounds in dl mapped by the current transform and cut by
// the current clip, so a command or block that the clip hides is dropped,
// like the Builder methods do. Every recorded command gets the current tag
// and, as its source (DisplayList.Source), its index in dl.
//
// For each Marker of dl that is not hidden, marker is called with the
// marker, at its place: what it records (usually another AppendList,
// possibly inside pushes it opens and closes) stands for the marker. While
// it runs, commands recorded directly have source -1. A nil marker drops
// the markers.
func (b *Builder) AppendList(dl *DisplayList, marker func(Marker)) {
	if dl == nil {
		return
	}
	saved := b.src
	defer func() { b.src = saved }()
	// The bounds of dl are in its root space: the local space here, mapped
	// by the transform current now, not by the transforms of dl that are
	// replayed on the way.
	base := b.ctm
	for i, c := range dl.cmds {
		b.src = int32(i)
		switch c := c.(type) {
		case FillRect, FillRRect, FillPath, StrokePath, DrawImage, DrawShadow, DrawGlyphRun:
			b.begin()
			if b.suppressed() {
				continue
			}
			if bb, ok := b.appendBounds(base, dl.bounds[i]); ok {
				b.record(c, bb)
			}
		case BackdropFilter:
			b.begin()
			if b.suppressed() {
				continue
			}
			if bb, ok := b.appendBounds(base, dl.bounds[i]); ok {
				b.recordBackdrop(c, bb, base.TransformRect(dl.backdropRead(i)))
			}
		case PushClipRect:
			b.pushClip("PushClipRect", c, c.Rect, true)
		case PushClipRRect:
			b.pushClip("PushClipRRect", c, c.RRect.Rect, true)
		case PushClipPath:
			b.pushClip("PushClipPath", c, c.Path.ControlBounds(), true)
		case PushTransform:
			b.PushTransform(c.Affine)
		case PushLayer:
			b.appendLayer(c)
		case PopClip:
			b.PopClip()
		case PopTransform:
			b.PopTransform()
		case PopLayer:
			b.PopLayer()
		case Marker:
			b.begin()
			if b.suppressed() || marker == nil {
				continue
			}
			b.src = -1
			marker(c)
		}
	}
}

// appendLayer opens a PushLayer taken from a list (already sanitized).
func (b *Builder) appendLayer(c PushLayer) {
	b.begin()
	b.openLayer("PushLayer", c)
}

// backdropRead returns the read bounds of the BackdropFilter at index i.
func (d *DisplayList) backdropRead(i int) Rect {
	lo, hi := 0, len(d.backdrops)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		if int(d.backdrops[m].idx) < i {
			lo = m + 1
		} else {
			hi = m
		}
	}
	if lo < len(d.backdrops) && int(d.backdrops[lo].idx) == i {
		return d.backdrops[lo].read
	}
	return d.bounds[i]
}

// appendBounds maps root-space bounds r of an appended list by m, the
// transform at AppendList, and clips them.
func (b *Builder) appendBounds(m Affine, r Rect) (Rect, bool) {
	r = m.TransformRect(r).Intersect(b.clip)
	return r, !r.Empty()
}
