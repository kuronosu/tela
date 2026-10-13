package gfx

import (
	"fmt"
	"strings"
)

// Builder records commands into a DisplayList. Create it with NewBuilder, call
// one method per command, then Finish. A Builder is not safe for concurrent
// use; after Finish it is empty and can be reused.
//
// # Nesting
//
// PushClip*, PushTransform and PushLayer open blocks that PopClip,
// PopTransform and PopLayer close. Blocks nest strictly as ONE stack across
// all kinds: the innermost open block must be the one that is popped. Popping
// the wrong kind, popping with nothing open, or calling Finish with blocks
// open panics with a message naming the offending call and the open blocks.
//
// # Bounds
//
// While recording, the Builder tracks the current transform and clip and
// stores a conservative root-space bounding rectangle for every command (see
// DisplayList). Outsets are computed in the local coordinate space of the
// command and then transformed to root space with the bounding box of the
// transformed rectangle, so they are exact for axis-aligned transforms and
// conservative otherwise:
//
//   - fills: the shape's bounds (paths: the control point bounds);
//   - strokes: path control bounds grown by Width/2, multiplied by
//     MiterLimit for miter joins and by sqrt(2) for square caps;
//   - shadows: outer: the box translated by Offset and grown by
//     max(Spread, 0) + 3 sigma, with sigma = Blur/2; inset: the box;
//   - glyph runs: the union of the glyphs' Font.GlyphBounds, scaled by Size
//     and placed at their pen positions (rasterizers that snap pen
//     positions move glyphs by less than one device pixel, which the
//     one-pixel margin of their culling covers);
//   - layers: the union of the content grown by FiltersOutset (3 sigma per
//     Gaussian, plus the offset of a drop shadow), converted to root space with
//     Affine.MaxScale, and limited to the clip active at PushLayer. Inside a
//     layer with filters, the content is limited by that clip grown by the
//     same reach, since the filters pull content from outside the clip;
//   - everything is intersected with the active clip.
//
// # Invalid input and no-op commands
//
// Programmer errors (unbalanced pushes, a nil Paint, an unknown BlendMode)
// panic. Bad VALUES never reach the rasterizer:
//
//   - A command with a NaN or infinite coordinate, color, parameter or
//     transform is dropped. A push command with such a value drops its whole
//     block, including the matching pop (nesting is still validated).
//   - Commands that cannot change any pixel are dropped as well: empty
//     geometry, a fully transparent solid color, a stroke of zero width,
//     content whose bounds do not meet the active clip, a clip or layer that
//     can show nothing (empty clip, singular transform, opacity 0), and
//     blocks left empty after their commands were dropped. The indices
//     returned by DisplayList therefore do not match the number of Builder
//     calls.
//   - Out-of-range values are clamped: colors to [0, 1], Opacity to [0, 1],
//     negative blur to 0. Rectangles and RRects are made upright, and RRect
//     radii are normalized.
type Builder struct {
	cmds   []Command
	bounds []Rect
	match  []int32
	reach  []float32
	tags   []uint32
	srcs   []int32
	bds    []backdropRef

	tag uint32 // SetTag
	src int32  // source index of the command being appended, or -1

	inited   bool
	ctm      Affine // current transform: local -> root
	clip     Rect   // conservative root-space clip bounds
	stack    []frame
	rootAcc  Rect // union of top-level command bounds
	calls    int  // number of Builder calls so far, for messages
	backdrop float32
}

// frame is an open push block.
type frame struct {
	kind     stackKind
	name     string // push command name, for messages
	call     int    // Builder call number of the push, for messages
	idx      int    // command index of the push; -1 if skipped
	skipped  bool   // nothing is recorded in this block
	acc      Rect   // union of the root bounds of the recorded content
	prevCTM  Affine
	prevClip Rect
	reach    float32 // PushLayer: root-space outset of filters
}

// NewBuilder returns an empty Builder.
func NewBuilder() *Builder {
	b := &Builder{}
	b.reset()
	return b
}

func (b *Builder) reset() {
	b.cmds, b.bounds, b.match, b.reach = nil, nil, nil, nil
	b.tags, b.srcs, b.bds = nil, nil, nil
	b.tag, b.src = 0, -1
	b.stack = b.stack[:0]
	b.ctm = Identity
	b.clip = infiniteRect
	b.rootAcc = Rect{}
	b.backdrop = 0
	b.inited = true
}

func (b *Builder) begin() {
	if !b.inited {
		b.reset()
	}
	b.calls++
}

// suppressed reports whether the innermost open block is being dropped.
func (b *Builder) suppressed() bool {
	return len(b.stack) > 0 && b.stack[len(b.stack)-1].skipped
}

// record appends a command with its root-space bounds, which the caller has
// already intersected with the clip, and returns its index.
func (b *Builder) record(c Command, bounds Rect) int {
	// perf: storing a struct in the Command interface allocates; if profiles
	// show it, switch to typed per-command slices with a kind byte.
	idx := len(b.cmds)
	b.cmds = append(b.cmds, c)
	b.bounds = append(b.bounds, bounds)
	b.match = append(b.match, -1)
	b.reach = append(b.reach, 0)
	b.tags = append(b.tags, b.tag)
	b.srcs = append(b.srcs, b.src)
	b.addToParent(bounds)
	return idx
}

func (b *Builder) addToParent(r Rect) {
	if n := len(b.stack); n > 0 {
		b.stack[n-1].acc = b.stack[n-1].acc.Union(r)
	} else {
		b.rootAcc = b.rootAcc.Union(r)
	}
}

// drawBounds converts local bounds to root space and clips them. ok is false
// when the command cannot be visible.
func (b *Builder) drawBounds(local Rect) (Rect, bool) {
	if !local.IsFinite() {
		return Rect{}, false
	}
	r := b.ctm.TransformRect(local).Intersect(b.clip)
	return r, !r.Empty()
}

// ---------------------------------------------------------------------------
// Draw commands
// ---------------------------------------------------------------------------

// FillRect fills r with paint.
func (b *Builder) FillRect(r Rect, paint Paint) {
	b.begin()
	checkPaint("FillRect", paint)
	if b.suppressed() {
		return
	}
	p, ok := sanitizePaint(paint)
	r = r.Abs()
	if !ok || r.Empty() {
		return
	}
	if bb, ok := b.drawBounds(r); ok {
		b.record(FillRect{r, p}, bb)
	}
}

// FillRRect fills the rounded rectangle rr with paint. rr is normalized.
func (b *Builder) FillRRect(rr RRect, paint Paint) {
	b.begin()
	checkPaint("FillRRect", paint)
	if b.suppressed() {
		return
	}
	p, ok := sanitizePaint(paint)
	rr = rr.Normalize()
	if !ok || rr.Rect.Empty() || !rr.IsFinite() {
		return
	}
	if bb, ok := b.drawBounds(rr.Rect); ok {
		b.record(FillRRect{rr, p}, bb)
	}
}

// FillPath fills path with paint under the fill rule. The path is copied.
func (b *Builder) FillPath(path Path, rule FillRule, paint Paint) {
	b.begin()
	checkPaint("FillPath", paint)
	if b.suppressed() {
		return
	}
	p, ok := sanitizePaint(paint)
	if !ok || path.IsEmpty() || !path.IsFinite() {
		return
	}
	local := path.ControlBounds()
	if local.Empty() { // zero area: nothing to fill
		return
	}
	if bb, ok := b.drawBounds(local); ok {
		// perf: the copy could be avoided when callers hand over ownership.
		b.record(FillPath{path.Clone(), rule, p}, bb)
	}
}

// StrokePath strokes path with style and paint. The path and the dashes are
// copied.
func (b *Builder) StrokePath(path Path, style StrokeStyle, paint Paint) {
	b.begin()
	checkPaint("StrokePath", paint)
	if b.suppressed() {
		return
	}
	p, ok := sanitizePaint(paint)
	style, styleOK := sanitizeStroke(style)
	if !ok || !styleOK || path.IsEmpty() || !path.IsFinite() {
		return
	}
	half := style.Width / 2
	k := float32(1)
	if style.Cap == CapSquare {
		k = 1.41421357 // sqrt(2), rounded up
	}
	if style.Join == JoinMiter {
		k = max(k, style.MiterLimit)
	}
	local := path.ControlBounds().Outset(half * k)
	if bb, ok := b.drawBounds(local); ok {
		b.record(StrokePath{path.Clone(), style, p}, bb)
	}
}

// DrawImage draws the src region of img into dst. A zero (empty) src selects
// the whole image. A nil img panics; a zero-size image draws nothing.
func (b *Builder) DrawImage(img *Image, src, dst Rect, sampling Sampling) {
	b.begin()
	if img == nil {
		panic("gfx.Builder.DrawImage: image is nil; create it with gfx.NewImage")
	}
	if b.suppressed() {
		return
	}
	dst = dst.Abs()
	if img.w == 0 || img.h == 0 || dst.Empty() || !dst.IsFinite() || !src.IsFinite() {
		return
	}
	if src.Empty() {
		src = img.Bounds()
	} else {
		src = src.Abs()
		if !src.Intersects(img.Bounds()) {
			return
		}
	}
	if bb, ok := b.drawBounds(dst); ok {
		b.record(DrawImage{img, src, dst, sampling}, bb)
	}
}

// DrawShadow paints a CSS box-shadow; see the DrawShadow command for the
// exact semantics.
func (b *Builder) DrawShadow(s DrawShadow) {
	b.begin()
	if b.suppressed() {
		return
	}
	s.RRect = s.RRect.Normalize()
	if !s.RRect.IsFinite() || s.RRect.Rect.Empty() || !s.Offset.IsFinite() ||
		!finite(s.Blur) || !finite(s.Spread) || !s.Color.IsFinite() {
		return
	}
	s.Color = s.Color.Clamp()
	s.Blur = max(s.Blur, 0)
	if s.Color.A == 0 {
		return
	}
	local := s.RRect.Rect
	if !s.Inset {
		local = local.Translate(s.Offset.X, s.Offset.Y).Outset(max(s.Spread, 0) + gaussianExtent*s.Blur/2)
	}
	if bb, ok := b.drawBounds(local); ok {
		b.record(s, bb)
	}
}

// BackdropFilter applies filters to what is already painted inside shape;
// see the BackdropFilter command. A call with no effective filter is dropped.
func (b *Builder) BackdropFilter(shape RRect, filters ...Filter) {
	b.begin()
	if b.suppressed() {
		return
	}
	shape = shape.Normalize()
	fs, ok := sanitizeFilters(filters)
	if !ok || len(fs) == 0 || shape.Rect.Empty() || !shape.IsFinite() {
		return
	}
	if bb, ok := b.drawBounds(shape.Rect); ok {
		b.recordBackdrop(BackdropFilter{shape, fs}, bb, b.ctm.TransformRect(shape.Rect))
	}
}

// recordBackdrop records a BackdropFilter whose clipped root bounds are bb
// and whose shape covers read in root space.
func (b *Builder) recordBackdrop(c BackdropFilter, bb, read Rect) {
	idx := b.record(c, bb)
	reach := FiltersOutset(c.Filters) * b.ctm.MaxScale()
	b.backdrop = max(b.backdrop, reach)
	inFilter := false
	for i := range b.stack {
		if b.stack[i].kind == kindLayer && b.stack[i].reach > 0 {
			inFilter = true
			break
		}
	}
	b.bds = append(b.bds, backdropRef{idx: int32(idx), read: read, reach: reach, inFilterLay: inFilter})
}

// ---------------------------------------------------------------------------
// Push / Pop
// ---------------------------------------------------------------------------

// open starts a block. skip says the block is to be dropped; newClip, newCTM
// are the state inside it.
func (b *Builder) open(kind stackKind, name string, skip bool) *frame {
	f := frame{
		kind: kind, name: name, call: b.calls, idx: -1,
		skipped:  skip || b.suppressed(),
		prevCTM:  b.ctm,
		prevClip: b.clip,
	}
	b.stack = append(b.stack, f)
	return &b.stack[len(b.stack)-1]
}

func (b *Builder) pushClip(name string, c Command, local Rect, valid bool) {
	b.begin()
	if b.suppressed() {
		b.open(kindClip, name, true)
		return
	}
	if !valid || !local.IsFinite() || local.Empty() {
		b.open(kindClip, name, true)
		return
	}
	newClip := b.clip.Intersect(b.ctm.TransformRect(local))
	if newClip.Empty() { // clips everything away
		b.open(kindClip, name, true)
		return
	}
	f := b.open(kindClip, name, false)
	f.idx = b.recordPush(c)
	b.clip = newClip
}

// recordPush appends a push command without adding its bounds to the parent;
// the bounds are filled in by the matching pop.
func (b *Builder) recordPush(c Command) int {
	idx := len(b.cmds)
	b.cmds = append(b.cmds, c)
	b.bounds = append(b.bounds, Rect{})
	b.match = append(b.match, -1)
	b.reach = append(b.reach, 0)
	b.tags = append(b.tags, b.tag)
	b.srcs = append(b.srcs, b.src)
	return idx
}

// PushClipRect intersects the clip with r until the matching PopClip.
func (b *Builder) PushClipRect(r Rect) {
	r = r.Abs()
	b.pushClip("PushClipRect", PushClipRect{r}, r, true)
}

// PushClipRRect intersects the clip with rr (normalized) until the matching
// PopClip.
func (b *Builder) PushClipRRect(rr RRect) {
	rr = rr.Normalize()
	b.pushClip("PushClipRRect", PushClipRRect{rr}, rr.Rect, rr.IsFinite())
}

// PushClipPath intersects the clip with the interior of path under rule
// until the matching PopClip. The path is copied.
func (b *Builder) PushClipPath(path Path, rule FillRule) {
	valid := !path.IsEmpty() && path.IsFinite()
	var local Rect
	if valid {
		local = path.ControlBounds()
		path = path.Clone()
	}
	b.pushClip("PushClipPath", PushClipPath{path, rule}, local, valid)
}

// PopClip closes the innermost block, which must be a PushClip*.
func (b *Builder) PopClip() { b.pop(kindClip, "PopClip", PopClip{}) }

// PushTransform applies t to the coordinates of everything up to the matching
// PopTransform; see the PushTransform command.
func (b *Builder) PushTransform(t Affine) {
	b.begin()
	// A transform that is not finite, is singular (collapses everything to a
	// line or point), or overflows the accumulated transform shows nothing.
	var next Affine
	skip := b.suppressed() || !t.IsFinite() || t.Det() == 0
	if !skip {
		next = b.ctm.Mul(t)
		skip = !next.IsFinite() || next.Det() == 0
	}
	f := b.open(kindTransform, "PushTransform", skip)
	if skip {
		return
	}
	f.idx = b.recordPush(PushTransform{t})
	b.ctm = next
}

// PopTransform closes the innermost block, which must be a PushTransform.
func (b *Builder) PopTransform() { b.pop(kindTransform, "PopTransform", PopTransform{}) }

// PushLayer starts a group composited with opacity, blend and filters at the
// matching PopLayer; see the PushLayer command. The filters are copied.
// Opacity is clamped to [0, 1]; a layer with opacity 0 is dropped with its
// content.
func (b *Builder) PushLayer(opacity float32, blend BlendMode, filters ...Filter) {
	b.begin()
	if blend > PlusLighter {
		panic(fmt.Sprintf("gfx.Builder.PushLayer: unknown BlendMode(%d); use the BlendMode constants", blend))
	}
	fs, ok := sanitizeFilters(filters)
	skip := b.suppressed() || !ok || !finite(opacity) || opacity <= 0
	f := b.open(kindLayer, "PushLayer", skip)
	if skip {
		return
	}
	f.idx = b.recordPush(PushLayer{clamp01(opacity), blend, fs})
	f.reach = FiltersOutset(fs) * b.ctm.MaxScale()
	b.reach[f.idx] = f.reach
	// The filters read the content up to reach beyond the clip active at
	// PushLayer (the clip applies to the filtered result), so content that
	// lies outside that clip but within reach of it is kept.
	b.clip = b.clip.Outset(f.reach)
}

// PopLayer closes the innermost block, which must be a PushLayer.
func (b *Builder) PopLayer() { b.pop(kindLayer, "PopLayer", PopLayer{}) }

func (b *Builder) pop(kind stackKind, name string, c Command) {
	b.begin()
	n := len(b.stack)
	if n == 0 {
		panic(fmt.Sprintf("gfx.Builder.%s called with no open block; every %s must match an earlier %s",
			name, name, pushNames(kind)))
	}
	f := b.stack[n-1]
	if f.kind != kind {
		panic(fmt.Sprintf("gfx.Builder.%s called, but the innermost open block is %s (Builder call #%d); close blocks in the reverse order they were opened, with %s",
			name, f.name, f.call, popNameOf(f.kind)))
	}
	b.stack = b.stack[:n-1]
	b.ctm, b.clip = f.prevCTM, f.prevClip
	if f.skipped {
		return
	}
	acc := f.acc
	if kind == kindLayer && !acc.Empty() {
		// A layer can only paint where its content is, grown by the reach of
		// its filters, and never outside the clip active at PushLayer.
		acc = acc.Outset(f.reach).Intersect(f.prevClip)
	}
	if acc.Empty() {
		// Everything inside was dropped or invisible: remove the block.
		b.truncate(f.idx)
		return
	}
	b.bounds[f.idx] = acc
	end := len(b.cmds)
	b.cmds = append(b.cmds, c)
	b.bounds = append(b.bounds, acc)
	b.match = append(b.match, int32(f.idx))
	b.reach = append(b.reach, 0)
	b.tags = append(b.tags, b.tags[f.idx])
	b.srcs = append(b.srcs, b.src)
	b.match[f.idx] = int32(end)
	b.addToParent(acc)
}

func (b *Builder) truncate(n int) {
	for i := n; i < len(b.cmds); i++ {
		b.cmds[i] = nil // release references
	}
	b.cmds = b.cmds[:n]
	b.bounds = b.bounds[:n]
	b.match = b.match[:n]
	b.reach = b.reach[:n]
	b.tags = b.tags[:n]
	b.srcs = b.srcs[:n]
	for len(b.bds) > 0 && int(b.bds[len(b.bds)-1].idx) >= n {
		b.bds = b.bds[:len(b.bds)-1]
	}
}

// Depth returns the number of blocks (pushes) currently open, including
// dropped ones.
func (b *Builder) Depth() int { return len(b.stack) }

// Finish ends recording and returns the immutable DisplayList. It panics if
// blocks are still open. The Builder is empty afterwards and may be reused.
func (b *Builder) Finish() *DisplayList {
	b.begin()
	if len(b.stack) > 0 {
		var parts []string
		var pops []string
		for i := len(b.stack) - 1; i >= 0; i-- {
			f := b.stack[i]
			parts = append(parts, fmt.Sprintf("%s (Builder call #%d)", f.name, f.call))
			pops = append(pops, popNameOf(f.kind))
		}
		panic(fmt.Sprintf("gfx.Builder.Finish called with %d open block(s), innermost first: %s; add %s",
			len(b.stack), strings.Join(parts, ", "), strings.Join(pops, ", ")))
	}
	d := &DisplayList{
		cmds:          b.cmds,
		bounds:        b.bounds,
		match:         b.match,
		reach:         b.reach,
		total:         b.rootAcc,
		backdropReach: b.backdrop,
		tags:          b.tags,
		srcs:          b.srcs,
		backdrops:     b.bds,
	}
	b.reset()
	b.calls = 0
	return d
}

func popNameOf(k stackKind) string {
	switch k {
	case kindClip:
		return "PopClip"
	case kindTransform:
		return "PopTransform"
	case kindLayer:
		return "PopLayer"
	}
	return "Pop?"
}

func pushNames(k stackKind) string {
	switch k {
	case kindClip:
		return "PushClipRect/PushClipRRect/PushClipPath"
	case kindTransform:
		return "PushTransform"
	case kindLayer:
		return "PushLayer"
	}
	return "Push?"
}

// checkPaint panics on a nil paint, which is a programmer error.
func checkPaint(op string, p Paint) {
	if p == nil {
		panic("gfx.Builder." + op + ": paint is nil; pass a gfx.Color or a gradient")
	}
}

// sanitizeStroke normalizes a StrokeStyle for storage. ok is false when the
// stroke would draw nothing or has non-finite values.
func sanitizeStroke(s StrokeStyle) (StrokeStyle, bool) {
	if !finite(s.Width) || s.Width <= 0 || !finite(s.MiterLimit) || !finite(s.DashOffset) {
		return s, false
	}
	switch {
	case s.MiterLimit == 0:
		s.MiterLimit = DefaultMiterLimit
	case s.MiterLimit < 1:
		s.MiterLimit = 1
	}
	s.Dashes = normalizeDashes(s.Dashes)
	if s.Dashes == nil {
		s.DashOffset = 0
	}
	return s, true
}

// normalizeDashes returns a private, even-length copy of dashes, or nil for a
// solid line (empty, invalid or all-zero input).
func normalizeDashes(d []float32) []float32 {
	if len(d) == 0 {
		return nil
	}
	var sum float32
	for _, v := range d {
		if !finite(v) || v < 0 {
			return nil
		}
		sum += v
	}
	if !(sum > 0) {
		return nil
	}
	if len(d)%2 == 1 {
		out := make([]float32, 0, 2*len(d))
		out = append(out, d...)
		return append(out, d...)
	}
	return append([]float32(nil), d...)
}
