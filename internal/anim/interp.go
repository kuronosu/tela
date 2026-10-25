package anim

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Typed interpolation of animatable values (CSS Values 4, section 10, and
// the per-property rules of CSS Transforms, Filter Effects and Backgrounds).
//
// Every function returns exactly its first argument at t = 0 and exactly
// its second at t = 1, and rounds like the rest of the pixel pipeline
// (ADR-041): a product that feeds an addition is rounded on its own, so the
// results are the same on every architecture.

// Lerp interpolates linearly between two numbers.
//
// t is the eased progress and may leave [0, 1] (a cubic-bezier() whose y
// values do) to overshoot; only 0 and 1 give the end points exactly.
func Lerp(a, b float32, t float64) float32 {
	switch t {
	case 0:
		return a
	case 1:
		return b
	}
	return float32(lerp64(float64(a), float64(b), t))
}

func lerp64(a, b, t float64) float64 {
	return a + float64((b-a)*t)
}

// LerpLP interpolates a length-percentage: the pixel and percentage parts
// separately, as calc() would.
func LerpLP(a, b style.LP, t float64) style.LP {
	return style.LP{Px: Lerp(a.Px, b.Px, t), Pct: Lerp(a.Pct, b.Pct, t)}
}

// LerpDim interpolates two dimensions. Only lengths interpolate: auto and
// the intrinsic keywords do not (ok is false), and a pair that does not
// interpolate switches from a to b halfway, like CSS's discrete animation.
func LerpDim(a, b style.Dim, t float64) (d style.Dim, ok bool) {
	if a.Kind == style.DimLength && b.Kind == style.DimLength {
		// The value is (1-t)·a + t·b, as calc() would write it: it has a
		// percentage part (0% included, ADR-157) when a term that has one
		// is there.
		pct := a.HasPercent() && t != 1 || b.HasPercent() && t != 0
		return style.DimLP(LerpLP(a.LP, b.LP, t), pct), true
	}
	if t < 0.5 {
		return a, false
	}
	return b, false
}

// LerpColor interpolates two colors the way Chromium does for transitions
// and animations (CSS Color 4, section 12): premultiplied by alpha, in
// sRGB when both colors are legacy sRGB colors (hex, rgb(), named) and in
// Oklab otherwise (oklch(), oklab(), color-mix()). Components are extended
// sRGB and are not clamped.
//
// A computed color does not remember the notation it was written in, so a
// color counts as legacy when its channels are whole multiples of 1/255,
// which is exact for hex and rgb() colors and does not happen for the
// converted Oklch colors of Tailwind's palette.
func LerpColor(a, b gfx.Color, t float64) gfx.Color {
	switch t {
	case 0:
		return a
	case 1:
		return b
	}
	alpha := min(lerp64(float64(a.A), float64(b.A), t), 1)
	if alpha <= 0 {
		return gfx.Transparent
	}
	var c0, c1, c2 float64
	if isLegacy(a) && isLegacy(b) {
		c0 = lerp64(float64(a.R*a.A), float64(b.R*b.A), t)
		c1 = lerp64(float64(a.G*a.A), float64(b.G*b.A), t)
		c2 = lerp64(float64(a.B*a.A), float64(b.B*b.A), t)
		return gfx.Color{R: float32(c0 / alpha), G: float32(c1 / alpha), B: float32(c2 / alpha), A: float32(alpha)}
	}
	al, aa, ab := a.ToOklab()
	bl, ba, bb := b.ToOklab()
	c0 = lerp64(float64(al*a.A), float64(bl*b.A), t)
	c1 = lerp64(float64(aa*a.A), float64(ba*b.A), t)
	c2 = lerp64(float64(ab*a.A), float64(bb*b.A), t)
	return gfx.Oklab(float32(c0/alpha), float32(c1/alpha), float32(c2/alpha), float32(alpha))
}

// isLegacy reports whether c is a legacy sRGB color (see LerpColor).
func isLegacy(c gfx.Color) bool {
	return whole255(c.R) && whole255(c.G) && whole255(c.B)
}

func whole255(v float32) bool {
	if v < 0 || v > 1 {
		return false
	}
	x := float64(v) * 255
	return math.Abs(x-math.Round(x)) < 1e-3
}

// LerpTransform interpolates two transforms function by function: the
// translation, rotation, scale and skew components separately, which is
// what CSS does for the translate, rotate and scale properties of Tailwind
// and for transform lists whose functions match. The transform origin
// switches to b's as soon as t > 0.
func LerpTransform(a, b style.Transform, t float64) style.Transform {
	o := a
	if t > 0 {
		o.OriginX, o.OriginY = b.OriginX, b.OriginY
	}
	o.TranslateX = LerpLP(a.TranslateX, b.TranslateX, t)
	o.TranslateY = LerpLP(a.TranslateY, b.TranslateY, t)
	o.Rotate = Lerp(a.Rotate, b.Rotate, t)
	o.ScaleX = Lerp(a.ScaleX, b.ScaleX, t)
	o.ScaleY = Lerp(a.ScaleY, b.ScaleY, t)
	o.SkewX = Lerp(a.SkewX, b.SkewX, t)
	o.SkewY = Lerp(a.SkewY, b.SkewY, t)
	return o
}

// ComposeTransform returns the transform of a box that has base and an
// animated transform on top of it. CSS applies translate, rotate and scale
// before the transform property, which is the order of style.Transform, so
// for the transforms animations use (a rotation, a scale or a translation
// in keyframes) adding the components is exact: translations and angles
// add, scales multiply. A rotated or scaled base under an animated
// translation differs from CSS's order of operations.
func ComposeTransform(base, over style.Transform) style.Transform {
	o := base
	o.TranslateX = style.LP{Px: base.TranslateX.Px + over.TranslateX.Px, Pct: base.TranslateX.Pct + over.TranslateX.Pct}
	o.TranslateY = style.LP{Px: base.TranslateY.Px + over.TranslateY.Px, Pct: base.TranslateY.Pct + over.TranslateY.Pct}
	o.Rotate = base.Rotate + over.Rotate
	o.ScaleX = float32(float64(base.ScaleX) * float64(over.ScaleX))
	o.ScaleY = float32(float64(base.ScaleY) * float64(over.ScaleY))
	o.SkewX = base.SkewX + over.SkewX
	o.SkewY = base.SkewY + over.SkewY
	return o
}

// IdentityTransform is the transform that does nothing (the origin is the
// CSS default, the center of the box).
func IdentityTransform() style.Transform {
	return style.Transform{ScaleX: 1, ScaleY: 1, OriginX: style.PctLP(50), OriginY: style.PctLP(50)}
}

// LerpShadows interpolates two box-shadow (or drop-shadow, text-shadow)
// lists. Corresponding shadows interpolate their offsets, blur, spread and
// color; the shorter list is padded with transparent shadows of no size,
// inset like the shadow they meet. ok is false when an inset shadow meets
// an outer one in the same position, which CSS does not interpolate; the
// list then switches from a to b halfway.
//
// Tailwind composes a box-shadow from fixed slots (inset shadow, inset
// ring, ring offset, ring, shadow) that Chromium keeps in place when a slot
// is empty. Tela's list only holds the layers that exist, so the layers
// are paired slot by slot (see shadowSlot) before padding, and a ring that
// appears grows from nothing instead of morphing into the shadow below.
func LerpShadows(a, b []style.Shadow, t float64) (out []style.Shadow, ok bool) {
	if len(a) == 0 && len(b) == 0 {
		return nil, true
	}
	pa, pb, ok := pairShadows(a, b)
	if !ok {
		if t < 0.5 {
			return a, false
		}
		return b, false
	}
	switch t {
	case 0:
		return a, true
	case 1:
		return b, true
	}
	out = make([]style.Shadow, len(pa))
	for i := range pa {
		out[i] = lerpShadow(pa[i], pb[i], t)
	}
	return out, true
}

func lerpShadow(a, b style.Shadow, t float64) style.Shadow {
	return style.Shadow{
		OffsetX: Lerp(a.OffsetX, b.OffsetX, t),
		OffsetY: Lerp(a.OffsetY, b.OffsetY, t),
		Blur:    max(Lerp(a.Blur, b.Blur, t), 0),
		Spread:  Lerp(a.Spread, b.Spread, t),
		Color:   LerpColor(a.Color, b.Color, t),
		Inset:   a.Inset,
	}
}

// Shadow slots of a Tailwind box-shadow, in painting order.
const (
	slotInsetShadow = iota
	slotInsetRing
	slotRing // ring offset and ring
	slotShadow
	numShadowSlots
)

// shadowSlot classifies a layer of a composed box-shadow by the utility
// that produced it: a ring is a spread-only shadow without offset or blur.
func shadowSlot(s style.Shadow) int {
	ringLike := s.OffsetX == 0 && s.OffsetY == 0 && s.Blur == 0 && s.Spread != 0
	switch {
	case s.Inset && ringLike:
		return slotInsetRing
	case s.Inset:
		return slotInsetShadow
	case ringLike:
		return slotRing
	}
	return slotShadow
}

// pairShadows aligns two shadow lists slot by slot and pads each slot with
// the transparent no-op shadow of the other list's layer.
func pairShadows(a, b []style.Shadow) (pa, pb []style.Shadow, ok bool) {
	var ga, gb [numShadowSlots][]style.Shadow
	for _, s := range a {
		k := shadowSlot(s)
		ga[k] = append(ga[k], s)
	}
	for _, s := range b {
		k := shadowSlot(s)
		gb[k] = append(gb[k], s)
	}
	for k := range numShadowSlots {
		n := max(len(ga[k]), len(gb[k]))
		for i := range n {
			var x, y style.Shadow
			switch {
			case i < len(ga[k]) && i < len(gb[k]):
				x, y = ga[k][i], gb[k][i]
			case i < len(ga[k]):
				x = ga[k][i]
				y = style.Shadow{Inset: x.Inset, Color: gfx.Transparent}
			default:
				y = gb[k][i]
				x = style.Shadow{Inset: y.Inset, Color: gfx.Transparent}
			}
			pa, pb = append(pa, x), append(pb, y)
		}
	}
	for i := range pa {
		if pa[i].Inset != pb[i].Inset {
			return nil, nil, false
		}
	}
	return pa, pb, true
}

// LerpFilters interpolates two filter lists (filter or backdrop-filter).
// Tela keeps filters in Tailwind's fixed order, so the lists always match
// function by function; a function only one side has interpolates from its
// identity value. Drop shadows interpolate like LerpShadows.
func LerpFilters(a, b style.FilterSet, t float64) style.FilterSet {
	switch t {
	case 0:
		return a
	case 1:
		return b
	}
	o := style.FilterSet{Set: a.Set | b.Set}
	pick := func(m style.FilterMask, av, bv, id float32) float32 {
		if o.Set&m == 0 {
			return 0
		}
		if a.Set&m == 0 {
			av = id
		}
		if b.Set&m == 0 {
			bv = id
		}
		return Lerp(av, bv, t)
	}
	o.Blur = max(pick(style.FilterBlur, a.Blur, b.Blur, 0), 0)
	o.Brightness = pick(style.FilterBrightness, a.Brightness, b.Brightness, 1)
	o.Contrast = pick(style.FilterContrast, a.Contrast, b.Contrast, 1)
	o.Grayscale = pick(style.FilterGrayscale, a.Grayscale, b.Grayscale, 0)
	o.HueRotate = pick(style.FilterHueRotate, a.HueRotate, b.HueRotate, 0)
	o.Invert = pick(style.FilterInvert, a.Invert, b.Invert, 0)
	o.Saturate = pick(style.FilterSaturate, a.Saturate, b.Saturate, 1)
	o.Sepia = pick(style.FilterSepia, a.Sepia, b.Sepia, 0)
	if len(a.DropShadow) > 0 || len(b.DropShadow) > 0 {
		pa, pb, ok := pairShadows(a.DropShadow, b.DropShadow)
		if ok {
			o.DropShadow = make([]style.Shadow, len(pa))
			for i := range pa {
				o.DropShadow[i] = lerpShadow(pa[i], pb[i], t)
			}
		} else if t < 0.5 {
			o.DropShadow = a.DropShadow
		} else {
			o.DropShadow = b.DropShadow
		}
	}
	return o
}
