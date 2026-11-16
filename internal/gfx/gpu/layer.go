package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Layers: PushLayer and PopLayer, with the meaning and the geometry of the
// rasterizer (raster/clip.go, filter.go, blend.go).
//
// A layer is an offscreen target of the pool, as large as the pixels it
// composites (the clip and the region inside its device bounds; grown by
// the filters' reach when it has filters). The pass being encoded ends at
// PushLayer and the layer's content is a pass of its own, encoded without
// the clip around the layer; at PopLayer the filters run (filter.go), the
// pass of the surface below goes on, over the pixels it left, and one
// instance composites the layer through the clip that was active at
// PushLayer:
//
//   - with gfx.SrcOver, a quad that paints the layer's pixels at its
//     opacity, times the coverage of its mask, blended by the device like
//     any other;
//   - with any other blend mode, a quad of PipelineBlend, which the device
//     does not blend: the pixels under the layer are first copied to a
//     target (a fragment cannot read the target it writes), and the shader
//     computes the W3C formula from the layer and the copy, on 8-bit
//     values with the rasterizer's integer arithmetic where it has one,
//     then interpolates between the copy and the result by the clip's
//     coverage.
//
// A mask (gfx.Mask) is its display list encoded into a target of its own
// before the content, under the transform at PushLayer and no clip, whole
// (not culled): the composite reads it and takes its alpha or luminance.
// It is drawn on the GPU, not rasterized on the CPU into the atlas, because
// a mask is a list recorded again every frame, without an identity to find
// its picture by: on the CPU it would be rasterized and uploaded each
// frame, where the GPU draws it with the glyphs and gradients it already
// holds and a steady frame uploads nothing.
//
// Passes are executed in the order they are encoded, so everything a
// layer needs is drawn when its composite is, and its targets go back to
// the pool as soon as the composite is encoded.
//
// perf: each layer ends the pass of the surface below it and starts it
// again: two passes per layer. The content of the layers that read nothing
// below them could be drawn before the pass of their surface, which would
// then run without a break, at the cost of keeping their targets until it
// ends. A layer with only an opacity around one primitive could be folded
// into the instance.

// layerRec is an open layer.
type layerRec struct {
	// outer is the state at PushLayer, inside the layer's bounds: the
	// composite goes through its clips, into the surface it draws into.
	outer state
	// The pass of the surface below, which goes on at PopLayer, and what
	// its quads are cut to.
	pass passHead
	lim  image.Rectangle
	// comp holds the pixels composited, and surf those of the layer's
	// target, which is larger for a layer with filters.
	comp, surf image.Rectangle
	target     *poolTarget
	opacity    uint8
	blend      gfx.BlendMode
	// mask is the target of the layer's mask, whose first pixel is the
	// device pixel maskOrg, or nil.
	mask     *poolTarget
	maskOrg  image.Point
	maskMode gfx.MaskMode
	// nf is the number of filters the layer pushed onto Encoder.dfs.
	nf int
}

// pixelBounds returns the device pixels a device-space box can touch, with
// a margin of one pixel, clamped to ±coordLimit (raster's pixelBounds).
func pixelBounds(x0, y0, x1, y1 float64) image.Rectangle {
	if !(x0 <= x1 && y0 <= y1) { // also NaN
		return image.Rectangle{}
	}
	return image.Rect(
		int(math.Floor(clampLim(x0)))-1, int(math.Floor(clampLim(y0)))-1,
		int(math.Ceil(clampLim(x1)))+1, int(math.Ceil(clampLim(y1)))+1,
	)
}

// devBounds returns the device pixels a rectangle of the root space of the
// list being encoded can touch (raster's devBounds).
func (e *Encoder) devBounds(bb gfx.Rect) image.Rectangle {
	return boundsUnder(&e.root, bb)
}

// boundsUnder returns the device pixels a rectangle can touch under m,
// with pixelBounds' margin.
func boundsUnder(m *gfx.Affine, bb gfx.Rect) image.Rectangle {
	if m.IsScaleTranslate() {
		x0, y0 := apply(m, bb.X0, bb.Y0)
		x1, y1 := apply(m, bb.X1, bb.Y1)
		return pixelBounds(min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1))
	}
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4][2]float32{{bb.X0, bb.Y0}, {bb.X1, bb.Y0}, {bb.X1, bb.Y1}, {bb.X0, bb.Y1}} {
		x, y := apply(m, p[0], p[1])
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	return pixelBounds(x0, y0, x1, y1)
}

// contentBounds returns the union of the root-space bounds of the commands
// directly inside the block that starts at index i of the list being
// encoded.
func (e *Encoder) contentBounds(i int) gfx.Rect {
	var u gfx.Rect
	end := e.dl.Match(i)
	for j := i + 1; j < end; j++ {
		u = u.Union(e.dl.BoundsAt(j))
		if m := e.dl.Match(j); m > j {
			j = m // its push has the union of the nested block
		}
	}
	return u
}

// growXY returns rect grown by dx on the left and right and by dy on the
// top and bottom, clamped to ±coordLimit. An empty rect stays empty.
func growXY(rect image.Rectangle, dx, dy int) image.Rectangle {
	if rect.Empty() {
		return image.Rectangle{}
	}
	lim := image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit)
	return image.Rect(rect.Min.X-dx, rect.Min.Y-dy, rect.Max.X+dx, rect.Max.Y+dy).Intersect(lim)
}

// pushLayer starts the layer of the PushLayer at index i of the list being
// encoded. Its state has been pushed.
func (e *Encoder) pushLayer(i int, c gfx.PushLayer) {
	st := e.top()
	op := raster.Unit8(float64(c.Opacity))
	if op == 0 {
		st.skip = true
		return
	}
	// The layer's bounds (its content, clipped by the clip at PushLayer; for
	// a layer with filters, grown by their reach first) in device pixels.
	bb := e.devBounds(e.dl.BoundsAt(i))
	// outer is the state the composite is drawn in: the clips around the
	// layer, inside its bounds.
	if !e.narrow(st, bb) {
		return
	}
	if e.fades(i, &c, st, op) {
		// The layer's one rectangle is drawn in its place, faded: no
		// target, no pass.
		st.fade = op
		return
	}
	outer := *st
	comp := st.scissor // the pixels the layer composites onto
	surf, in, lim := comp, comp, st.bound
	nf := 0
	if len(c.Filters) > 0 {
		// The content of a layer with filters is kept up to their reach
		// beyond the pixels composited, and beyond the clip (which cuts
		// the filtered result): the passes of a blur and of the filters
		// after it read it there.
		first := len(e.dfs)
		e.prepareFilters(c.Filters, &st.ctm)
		nf = len(e.dfs) - first
		rx, ry := filtersReach(e.dfs[first:])
		lim = growXY(st.bound, rx, ry).Intersect(e.devBounds(e.contentBounds(i)))
		surf = growXY(comp, rx, ry)
		in = surf.Intersect(lim)
	}
	fail := func(what string) {
		e.unsupported(what)
		clear(e.dfs[len(e.dfs)-nf:])
		e.dfs = e.dfs[:len(e.dfs)-nf]
		e.top().skip = true
	}
	if tl := e.res.limits.MaxTextureSize; surf.Dx() > tl || surf.Dy() > tl {
		fail("PushLayer (a layer larger than the device's textures)")
		return
	}
	rec := layerRec{outer: outer, pass: e.pass, lim: e.lim, comp: comp, surf: surf, opacity: op, blend: c.Blend, nf: nf, maskMode: c.Mask.Mode}
	rec.pass.hasClear = false // done by the pass that ends here
	if c.Mask.List != nil {
		var ok bool
		if rec.mask, rec.maskOrg, ok = e.renderMask(c.Mask, comp, outer.bound); !ok {
			fail("PushLayer (a mask larger than the device's textures)")
			return
		}
		st = e.top() // the mask's list may have grown the stack
	}
	rec.target, _ = e.acquireTarget(surf.Size(), FormatRGBA8) // it fits: checked above
	// The content: a pass of its own, into a transparent surface, without
	// the clips around the layer.
	e.setTarget(rec.target.id, surf.Min, &Clear{Rect: surf})
	e.layers = append(e.layers, rec)
	st.layer = len(e.layers) - 1
	st.scissor, st.bound, st.clip, st.mask, st.surf = in, lim, -1, -1, surf
	e.lim = lim
	if in.Empty() {
		// No content can show, but the filters still run at PopLayer (a
		// color matrix can make transparent pixels visible).
		st.skip = true
	}
}

// fades reports whether the layer of the PushLayer c at index i, with the
// opacity op, is drawn without a target: it only fades (source-over, no
// filter, no mask) and holds one command, a rectangle filled with a color
// whose device edges fall on pixel boundaries. In its own target the
// rectangle's pixels would hold that color, exactly, and the composite
// would paint them at the layer's opacity times the clip's coverage: what
// the fill's instance computes when the opacity multiplies its coverage
// (FlagOpacity). A scrim that fades in is such a layer, over the whole
// window: three passes of its size less.
func (e *Encoder) fades(i int, c *gfx.PushLayer, st *state, op uint8) bool {
	if op == 255 || c.Blend != gfx.SrcOver || len(c.Filters) != 0 || c.Mask.List != nil || e.dl.Match(i) != i+2 {
		return false
	}
	fr, ok := e.dl.At(i + 1).(gfx.FillRect)
	if !ok || !st.ctm.IsAxisAligned() {
		return false
	}
	if _, ok := fr.Paint.(gfx.Color); !ok {
		return false
	}
	sh, _, ok := deviceShape(&st.ctm, gfx.RRect{Rect: fr.Rect})
	if !ok {
		return false
	}
	for _, v := range sh.rect {
		if v != math.Floor(v) {
			return false
		}
	}
	return true
}

// popLayer ends the layer the state on top of the stack opened: its
// filters run, and it is composited into the surface below.
func (e *Encoder) popLayer() {
	k := e.top().layer
	rec := e.layers[k]
	e.layers[k] = layerRec{}
	e.layers = e.layers[:k]

	// What is composited: the layer's target, or the result of its filters.
	src := targetPlane(rec.target, rec.surf)
	if rec.nf > 0 {
		fs := e.dfs[len(e.dfs)-rec.nf:]
		out := e.runFilters(src, fs, rec.comp)
		if out.from != src.from {
			e.releaseTarget(src.t)
			src = out
		}
		clear(fs)
		e.dfs = e.dfs[:len(e.dfs)-rec.nf]
	}
	if src.from == 0 {
		// A step did not fit the device's textures (reported): the layer
		// is left out.
		e.releaseTarget(rec.mask)
		e.setTarget(rec.pass.target, rec.pass.origin, nil)
		e.lim = rec.lim
		return
	}
	st := rec.outer
	st.scissor = st.scissor.Intersect(rec.comp)
	below := Source(rec.pass.target)
	if rec.pass.target == 0 {
		below = SourceDest
	}
	// A blend mode reads the pixels under the layer from a copy.
	var back *poolTarget
	if rec.blend != gfx.SrcOver {
		back, _ = e.acquireTarget(st.scissor.Size(), FormatRGBA8) // no larger than the layer
		e.setTarget(back.id, st.scissor.Min, nil)
		e.copyPixels(st.scissor, below, rec.outer.surf, rec.pass.origin)
	}
	// The pass of the surface below goes on.
	e.setTarget(rec.pass.target, rec.pass.origin, nil)
	e.lim = rec.lim

	// One quad over the layer's bounds (which do not depend on the region;
	// the scissor keeps the pixels composited), through the clips at
	// PushLayer.
	var in instance
	b := st.bound
	quad := e.begin(&in, b)
	ox, oy := float64(quad.Min.X), float64(quad.Min.Y)
	sh := shape{rect: [4]float64{float64(b.Min.X), float64(b.Min.Y), float64(b.Max.X), float64(b.Max.Y)}}
	sh.put(&in, SlotShape, ox, oy)
	lp := layerPaint{src: src, opacity: rec.opacity, blend: rec.blend, inside: b}
	if rec.mask != nil {
		lp.mask, lp.maskOrg, lp.maskKind = rec.mask, rec.maskOrg, LayerMaskAlpha
		if rec.maskMode == gfx.MaskLuminance {
			lp.maskKind = LayerMaskLuminance
		}
	}
	if back != nil {
		lp.back, lp.backOrg = back, st.scissor.Min
	}
	e.emitLayer(&in, 0, &st, quad, &lp)
	// Every pass that draws into these targets again comes after the one
	// being encoded.
	e.releaseTarget(src.t)
	e.releaseTarget(rec.mask)
	e.releaseTarget(back)
}

// renderMask encodes the mask m of a layer that composites the device
// pixels comp into a target of its own and returns it, with the device
// pixel its first pixel is. lim holds the pixels the layer can composite
// in any region. ok is false when the mask does not fit a texture.
func (e *Encoder) renderMask(m gfx.Mask, comp, lim image.Rectangle) (t *poolTarget, org image.Point, ok bool) {
	ctm := e.top().ctm
	rect := comp
	if m.List.BackdropReach() > 0 {
		// Its backdrop filters read their whole shapes on the mask's
		// surface: all of it is drawn, so that they never see the region.
		box := image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit)
		all := boundsUnder(&ctm, m.List.Bounds()).Intersect(box)
		rect, lim = all.Union(comp), all.Union(lim)
	}
	if t, ok = e.acquireTarget(rect.Size(), FormatRGBA8); !ok {
		return nil, org, false
	}
	e.setTarget(t.id, rect.Min, &Clear{Rect: rect})
	dl, root, saved := e.dl, e.root, e.lim
	depth := len(e.stack)
	e.stack = append(e.stack, state{ctm: ctm, scissor: rect, bound: lim, clip: -1, mask: -1, surf: rect, layer: -1})
	// The root space of the mask's list is the space the layer is pushed
	// in.
	e.dl, e.root, e.lim = m.List, ctm, lim
	for i := range m.List.Len() {
		e.visit(i, m.List.At(i))
	}
	e.dl, e.root, e.lim = dl, root, saved
	e.stack = e.stack[:depth]
	return t, rect.Min, true
}

// layerPaint is the paint of an instance that paints the pixels of a
// target (FlagLayer): a layer, or a filtered backdrop.
type layerPaint struct {
	src     plane
	opacity uint8
	blend   gfx.BlendMode
	// fade: the opacity scales the colors, as for a backdrop filter; else
	// it is coverage.
	fade bool
	// The mask of a layer (a LayerMask constant, and the device pixel of
	// its target's first pixel), and the copy of the backdrop a blend
	// mode reads.
	mask     *poolTarget
	maskOrg  image.Point
	maskKind uint32
	back     *poolTarget
	backOrg  image.Point
	// inside holds pixels the instance's shape covers entirely (emitShape).
	inside image.Rectangle
}

// emitLayer finishes an instance that paints the pixels of a target and
// emits it: in holds its shape, flags the shape's.
func (e *Encoder) emitLayer(in *instance, flags uint32, st *state, quad image.Rectangle, lp *layerPaint) {
	head := uint32(lp.opacity) | uint32(lp.blend)<<8 | lp.maskKind<<16
	if lp.fade {
		head |= 1 << 24
	}
	in[SlotHead][1] = head
	in[SlotTex][0], in[SlotTex][1] = uint32(int32(-lp.src.org.X)), uint32(int32(-lp.src.org.Y))
	// Whole numbers, which float32 holds exactly.
	in.floats(SlotPaint+1, float64(-lp.maskOrg.X), float64(-lp.maskOrg.Y), float64(-lp.backOrg.X), float64(-lp.backOrg.Y))
	e.sources = [3]Source{lp.src.from}
	if lp.mask != nil {
		e.sources[1] = Source(lp.mask.id)
	}
	if lp.back != nil {
		e.sources[2] = Source(lp.back.id)
		e.pipeline = PipelineBlend
	}
	e.emitShape(in, flags|FlagLayer, st, quad, lp.inside, false)
	e.sources, e.pipeline = [3]Source{}, PipelineQuad
}
