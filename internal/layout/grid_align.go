// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/grid/alignment.rs); see
// THIRD_PARTY.md.
//
// Absolutely positioned children are laid out by layoutAbsolute (CSS 2
// 10.3.7 inside the grid area), not by Taffy's variant of
// align_and_position_item.

package layout

// reversedAlign returns the alignment keyword of a reversed axis (Taffy's
// AlignContentKeyword::reversed).
func reversedAlign(k Align) Align {
	switch k {
	case AlignStart, AlignStretch:
		return AlignEnd
	case AlignEnd:
		return AlignStart
	case AlignFlexStart:
		return AlignFlexEnd
	case AlignFlexEnd:
		return AlignFlexStart
	}
	return k
}

// alignTracks sets the offsets of the tracks of an axis from the
// container's border box, aligning them per align-content or
// justify-content (gutters are tracks, so no gap is applied here).
// reversed lays the axis out end to start (columns in RTL).
func alignTracks(contentSize, paddingStart, borderStart float32, tracks []gridTrack, align Align, reversed bool) {
	var used float32
	for i := range tracks {
		used += tracks[i].base
	}
	free := contentSize - used
	n := 0
	for i := 1; i < len(tracks); i += 2 {
		if !tracks[i].collapsed {
			n++
		}
	}
	k := alignmentFallback(free, n, align)
	if reversed {
		k = reversedAlign(k)
	}
	// Without tracks the lines are still aligned in the container.
	total := paddingStart + borderStart
	if n == 0 {
		total += alignmentOffset(free, n, 0, k, false, true)
	}
	seen := false
	place := func(i int, t *gridTrack) {
		isTrack := i%2 == 1 && !t.collapsed
		var off float32
		if isTrack {
			off = alignmentOffset(free, n, 0, k, false, !seen)
			seen = true
		}
		t.offset = total + off
		total += off + t.base
	}
	if reversed {
		for i := len(tracks) - 1; i >= 0; i-- {
			place(len(tracks)-1-i, &tracks[i])
		}
	} else {
		for i := range tracks {
			place(i, &tracks[i])
		}
	}
}

// selfAlignment resolves an item's justify-self (inline) or align-self:
// its own value, else the container's justify-items or align-items, else
// start for an item sized in that axis (or with an aspect ratio, in the
// block axis) and stretch otherwise. self-start and self-end resolve
// against the item's direction.
func selfAlignment(own, container Align, sized bool, itemDir, dir Direction, inline bool) Align {
	a := own
	if a == AlignNormal {
		a = container
	}
	if a == AlignNormal {
		if sized {
			return AlignStart
		}
		return AlignStretch
	}
	return resolveSelfAlign(a, itemDir, dir, inline)
}

// alignInArea aligns a box of size sz with margins m0 and m1 (none: auto)
// in [a0, a1] of one axis. shim is a baseline shim added to the start
// margin; rtl lays the axis out right to left. It returns the start and
// the used margins.
func alignInArea(a0, a1 float32, align Align, sz float32, m0, m1 opt, shim float32, rtl bool) (float32, float32, float32) {
	nonAuto0, nonAuto1 := m0.or0()+shim, m1.or0()
	areaSize := fmax(a1-a0, 0)
	free := fmax(areaSize-sz-nonAuto0-nonAuto1, 0)
	autos := 0
	if !m0.ok {
		autos++
	}
	if !m1.ok {
		autos++
	}
	var autoSize float32
	if autos > 0 {
		autoSize = free / float32(autos)
	}
	r0, r1 := m0.unwrapOr(autoSize)+shim, m1.unwrapOr(autoSize)
	k := align.keyword()
	if align.isSafe() && sz+nonAuto0+nonAuto1 > areaSize {
		k = AlignStart
	}
	var off float32
	switch k {
	case AlignEnd, AlignFlexEnd:
		if rtl {
			off = r0
		} else {
			off = areaSize - sz - r1
		}
	case AlignCenter:
		off = (areaSize - sz + r0 - r1) / 2
	default: // start, flex-start, baseline (as start), stretch
		if rtl {
			off = areaSize - sz - r1
		} else {
			off = r0
		}
	}
	return a0 + off, r0, r1
}

// gridItemSizing is what positioning an item needs from the container.
type gridItemSizing struct {
	justifyItems, alignItems Align
	dir                      Direction
	containerW               float32
	border                   rect
	scrolls                  bool
}

// positionGridItem sizes an in-flow item in its grid area (container
// border-box coordinates), aligns it, lays it out and records its layout.
// It returns its scrollable overflow contribution, its y and height and
// the output of its layout.
func (e *Engine) positionGridItem(n, c Node, st *Style, area rect, shim float32, gs *gridItemSizing) (rect, float32, float32, output) {
	areaSize := size{area.r - area.l, area.b - area.t}
	areaOpts := areaSize.opts()
	ratio := st.aspectRatio()
	aw := some(areaSize.w)

	padding := st.Padding.resolveOrZero(aw)
	border := st.Border.resolveOrZero(aw)
	pbSum := padding.add(border).sum()

	inherent := st.size().resolve(areaOpts).applyAspectRatio(ratio)
	minSize := st.minSize().resolve(areaOpts).or(pbSum.opts()).maybeMaxF(pbSum).applyAspectRatio(ratio)
	maxSize := st.maxSize().resolve(areaOpts).applyAspectRatio(ratio)

	hAlign := selfAlignment(st.JustifySelf, gs.justifyItems, inherent.w.ok || st.Width.isSizingKeyword(), st.Direction, gs.dir, true)
	vAlign := selfAlignment(st.AlignSelf, gs.alignItems, inherent.h.ok || st.Height.isSizingKeyword() || ratio.ok, st.Direction, gs.dir, false)

	// Both horizontal and vertical margins resolve against the width of
	// the grid area.
	margin := st.Margin.resolveMargins(aw)
	minus := size{
		w: fsubOpt(fsubOpt(areaSize.w, margin.l), margin.r),
		h: fsubOpt(fsubOpt(areaSize.h, margin.t), margin.b) - shim,
	}

	// Sizing keywords resolve exactly or by measuring the item.
	kw, kh := keywordNone, keywordNone
	var aws, ahs AvailableSpace
	var ew, eh float32
	if !inherent.w.ok {
		kw, aws, ew = resolveSizingKeyword(st.Width, some(minus.w), some(areaSize.w))
	}
	if !inherent.h.ok {
		kh, ahs, eh = resolveSizingKeyword(st.Height, some(minus.h), some(areaSize.h))
	}
	var measured optSize
	if kw == keywordMeasure && kh == keywordMeasure {
		measured = e.measureChildSizeBoth(n, c, optSize{}, areaOpts, availSize{aws, ahs}, inherentSize).opts()
	}

	w := inherent.w
	if !w.ok {
		switch {
		case kw == keywordExact:
			w = some(ew)
		case kw == keywordMeasure:
			w = measured.w
			if !w.ok {
				w = some(e.measureChildSize(n, c, optSize{}, areaOpts, availSize{aws, DefiniteSpace(minus.h)}, inherentSize, horizontal))
			}
		case margin.l.ok && margin.r.ok && hAlign.keyword() == AlignStretch:
			w = some(minus.w)
		}
	}
	k := optSize{w, inherent.h}.applyAspectRatio(ratio)
	if !k.h.ok {
		switch {
		case kh == keywordExact:
			k.h = some(eh)
		case kh == keywordMeasure:
			k.h = measured.h
			if !k.h.ok {
				avw := DefiniteSpace(minus.w)
				if k.w.ok {
					avw = DefiniteSpace(k.w.v)
				}
				k.h = some(e.measureChildSize(n, c, optSize{w: k.w}, areaOpts, availSize{avw, ahs}, inherentSize, vertical))
			}
		case margin.t.ok && margin.b.ok && vAlign.keyword() == AlignStretch:
			k.h = some(minus.h)
		}
	}
	k = k.applyAspectRatio(ratio).maybeClamp(minSize, maxSize)

	out := e.performChildLayout(n, c, k, areaOpts, availSize{DefiniteSpace(minus.w), DefiniteSpace(minus.h)}, inherentSize)
	final := k.unwrapOr(out.size).maybeClamp(minSize, maxSize)

	rtl := gs.dir == RTL
	x, ml, mr := alignInArea(area.l, area.r, hAlign, final.w, margin.l, margin.r, 0, rtl)
	y, mt, mb := alignInArea(area.t, area.b, vAlign, final.h, margin.t, margin.b, shim, false)
	if st.Position == PositionRelative {
		in := st.Inset.resolveInsets(areaOpts)
		var dx opt
		if rtl {
			dx = in.r
			if dx.ok {
				dx.v = -dx.v
			} else {
				dx = in.l
			}
		} else {
			dx = in.l
			if !dx.ok && in.r.ok {
				dx = some(-in.r.v)
			}
		}
		dy := in.t
		if !dy.ok && in.b.ok {
			dy = some(-in.b.v)
		}
		x += dx.or0()
		y += dy.or0()
	}

	var sb point
	if st.OverflowY == OverflowScroll {
		sb.x = st.ScrollbarWidth
	}
	if st.OverflowX == OverflowScroll {
		sb.y = st.ScrollbarWidth
	}
	c.LayoutData().setLayout(point{x, y}, final, border, padding, rect{ml, mr, mt, mb}, sb, out.overflow, out.firstBaseline)

	// The contribution to the container's scrollable overflow is measured
	// from its padding box (mirrored in RTL).
	loc := point{x - gs.border.l, y - gs.border.t}
	if rtl {
		loc.x = gs.containerW - (x + final.w) - gs.border.r
	}
	return overflowContribution(loc, final, out.overflow, st, gs.scrolls), y, final.h, out
}
