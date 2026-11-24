package gpu

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// shadow encodes a DrawShadow, with the cases of raster's drawShadow. The
// instance's shape is the box, which the shadow is painted outside of
// (outer) or inside of (inset); SlotBlur is the shape that is blurred.
//
// The blurred shape is evaluated by the shader in closed form, in a frame
// where the Gaussian is axis-aligned:
//
//   - under a scale and a translation, device space, with the standard
//     deviations of each axis, as the rasterizer does;
//   - under any other transform, the local space of the command, at each
//     pixel's center, with the pixel taken as a box of the size of its
//     local extent. For a rotation with a uniform scale this is the
//     rasterizer's method. For a skew or a rotation with different scales
//     the rasterizer blurs the rasterized shape with the two marginal
//     device standard deviations instead, an approximation of the same
//     picture, so the two differ there by a few levels.
func (e *Encoder) shadow(c gfx.DrawShadow) {
	st := e.top()
	m := &st.ctm
	blurShape, ok := raster.ShadowShape(c)
	if !ok && !c.Inset {
		return
	}
	sigma := float64(c.Blur) / 2
	if !(sigma > 0) {
		sigma = 0
	}
	// The pixels the shadow may paint, as the rasterizer has them: the
	// shape grown by 3 sigma (outer) or the box (inset).
	var zone image.Rectangle
	if c.Inset {
		zone = touchBounds(m, rect64(c.RRect.Rect, 0))
	} else {
		zone = touchBounds(m, rect64(blurShape.Rect, float64(3*sigma)))
	}
	if !zone.Overlaps(st.scissor) {
		return
	}
	pe, visible := solid(c.Color)
	if !visible {
		return
	}
	box, _, boxOK := shapeFor(m, c.RRect)
	if !boxOK {
		if c.Inset {
			return
		}
		box = shape{} // an empty box: nothing is cut out of the shadow
	}

	var in instance
	quad := e.begin(&in, zone)
	ox, oy := float64(quad.Min.X), float64(quad.Min.Y)
	flags := uint32(FlagOuterShadow)
	if c.Inset {
		flags = FlagInsetShadow
	}
	if box.local {
		flags |= FlagShapeLocal
	}
	box.put(&in, SlotShape, ox, oy)
	flags |= pe.put(&in, quad)

	mode := uint32(BlurEmpty)
	if ok {
		sx, sy := deviceSigmas(m, sigma)
		switch {
		case !(max(sx, sy) >= raster.MinSigma):
			// A hard shadow: the shape's coverage.
			hard, _, hok := shapeFor(m, blurShape)
			switch {
			case !hok:
			case hard.local:
				mode = BlurHardLocal
				hard.put(&in, SlotBlur, ox, oy)
			default:
				mode = BlurHardDevice
				hard.put(&in, SlotBlur, ox, oy)
			}
		case m.IsScaleTranslate():
			// The shape in device space (raster's rrBlur.setDevice), not
			// snapped. A standard deviation too small to blur on one axis
			// only is raised to the smallest that does.
			s, sok := deviceBlurShape(m, blurShape)
			if sok {
				mode = BlurAnalytic
				s.put(&in, SlotBlur, ox, oy)
				in.floats(SlotBlur+3, 1, 0, 0, 1)
				in.floats(SlotBlur+4, 0, 0, 0, 0)
				in.floats(SlotSigma, max(sx, raster.MinSigma), max(sy, raster.MinSigma), 1, 1)
			}
		default:
			inv, iok := invert(m)
			if iok && sigma > 0 {
				mode = BlurAnalytic
				s := shape{local: true, inv: inv, rect: rect64(blurShape.Rect, 0)}
				for i, p := range blurShape.Radii {
					if p.X > 0 && p.Y > 0 {
						s.radX[i], s.radY[i] = float64(p.X), float64(p.Y)
					}
				}
				s.put(&in, SlotBlur, ox, oy)
				// The local extent of a pixel along each axis.
				wx := math.Sqrt(float64(inv.a*inv.a) + float64(inv.c*inv.c))
				wy := math.Sqrt(float64(inv.b*inv.b) + float64(inv.d*inv.d))
				in.floats(SlotSigma, sigma, sigma, wx, wy)
			}
		}
	}
	if mode == BlurEmpty && !c.Inset {
		return // an outer shadow of nothing
	}
	flags |= mode << FlagBlurShift
	if !c.Inset && boxOK {
		// An outer shadow paints nothing where its box covers entirely:
		// under a pane, that is most of its pixels.
		e.emitShape(&in, flags, st, quad, box.inside(), true)
		return
	}
	e.emit(&in, flags, st, quad, nil)
}

// deviceBlurShape maps the shape of a shadow through the scale and
// translation m, with the radii on the device corners (flips swap them).
func deviceBlurShape(m *gfx.Affine, rr gfx.RRect) (s shape, ok bool) {
	x0, y0 := apply(m, rr.Rect.X0, rr.Rect.Y0)
	x1, y1 := apply(m, rr.Rect.X1, rr.Rect.Y1)
	ax, ay := math.Abs(float64(m[0])), math.Abs(float64(m[3]))
	for i, p := range rr.Radii {
		if !(p.X > 0 && p.Y > 0) {
			continue
		}
		right, bottom := (i == 1 || i == 2) != (x0 > x1), (i >= 2) != (y0 > y1)
		j := 0
		switch {
		case right && !bottom:
			j = 1
		case right && bottom:
			j = 2
		case bottom:
			j = 3
		}
		s.radX[j], s.radY[j] = float64(p.X)*ax, float64(p.Y)*ay // exact
	}
	s.rect = [4]float64{min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)}
	const lim = 1 << 40
	for _, v := range s.rect {
		if !(math.Abs(v) < lim) {
			return s, false
		}
	}
	return s, s.rect[0] < s.rect[2] && s.rect[1] < s.rect[3]
}

// rect64 returns r grown by e, in float64.
func rect64(r gfx.Rect, e float64) [4]float64 {
	return [4]float64{float64(r.X0) - e, float64(r.Y0) - e, float64(r.X1) + e, float64(r.Y1) + e}
}

// deviceSigmas returns the device standard deviations along x and y of a
// local Gaussian with standard deviation s under m (raster's
// deviceSigmas): s times the norms of the rows of the linear part.
func deviceSigmas(m *gfx.Affine, s float64) (float64, float64) {
	a, b, c, d := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	return float64(s * math.Sqrt(a*a+c*c)), float64(s * math.Sqrt(b*b+d*d))
}
