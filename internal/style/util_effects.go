package style

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
)

// shadowUtility registers a shadow family: theme keys, none, arbitrary
// shadow lists and a color.
func shadowUtility(r *registry, root string, ns Namespace, themeLayers func(*Theme) map[string][]Shadow,
	allowSpread bool, slot colorSlot, set func(*env, []Shadow)) {
	r.add(root, false, func(c *pctx, a *arg) (Op, error) {
		var layers []Shadow
		switch {
		case a.hasMod:
			return Op{}, errNoMatch
		case a.arb:
			if a.hint != "" && a.hint != "shadow" {
				return Op{}, errNoMatch
			}
			if a.hint == "" && looksLikeColor(a.inner) {
				return Op{}, errNoMatch
			}
			l, err := parseShadowList(c, a.inner, allowSpread)
			if err != nil {
				return Op{}, err
			}
			layers = l
		case a.raw == "":
			l, ok := themeLayers(c.th)["DEFAULT"]
			if !ok {
				return Op{}, errNoMatch
			}
			layers = l
		case a.value == "none":
		default:
			l, ok := themeLayers(c.th)[a.value]
			if !ok || a.value == "DEFAULT" {
				return Op{}, errNoMatch
			}
			layers = l
		}
		return op(func(e *env) { set(e, layers) }), nil
	}, func(th *Theme) []string { return append(th.Keys(ns), "none") })
	r.colorUtil(root, slot, nil)
}

func registerEffects(r *registry) {
	shadowUtility(r, "shadow", NamespaceShadow, func(t *Theme) map[string][]Shadow { return t.Shadows }, true, slotShadow,
		func(e *env, l []Shadow) {
			e.sc.shadowUsed = true
			// An inset layer of the theme ("inner") goes with the inset shadows.
			e.sc.shadow, e.sc.insetShadow = nil, nil
			for _, s := range l {
				if s.Inset {
					e.sc.insetShadow = append(e.sc.insetShadow, s)
				} else {
					e.sc.shadow = append(e.sc.shadow, s)
				}
			}
		})
	shadowUtility(r, "inset-shadow", NamespaceInsetShadow, func(t *Theme) map[string][]Shadow { return t.InsetShadows }, false, slotInsetShadow,
		func(e *env, l []Shadow) {
			e.sc.shadowUsed = true
			e.sc.insetShadow = nil
			for _, s := range l {
				s.Inset = true
				e.sc.insetShadow = append(e.sc.insetShadow, s)
			}
		})
	shadowUtility(r, "text-shadow", NamespaceTextShadow, func(t *Theme) map[string][]Shadow { return t.TextShadows }, false, slotTextShadow,
		func(e *env, l []Shadow) { e.s.Text.Shadow = l })

	// Opacity.
	r.add("opacity", false, func(c *pctx, a *arg) (Op, error) {
		var v float32
		if a.arb {
			n, ok := numberArg(a)
			if !ok {
				return Op{}, fmt.Errorf("%q is not a number; use opacity-[.67] or opacity-50", a.inner)
			}
			v = n
		} else {
			n, ok := parseFloat(a.value)
			if !ok || a.hasMod || n < 0 || n > 100 {
				return Op{}, errNoMatch
			}
			v = n / 100
		}
		v = clampf(v, 0, 1)
		return op(func(e *env) { e.s.Opacity = v }), nil
	}, func(*Theme) []string {
		return []string{"0", "5", "10", "20", "25", "30", "40", "50", "60", "70", "75", "80", "90", "95", "100"}
	})

	// Blend modes.
	blends := map[string]gfx.BlendMode{
		"normal": gfx.SrcOver, "multiply": gfx.Multiply, "screen": gfx.Screen, "overlay": gfx.Overlay,
		"darken": gfx.Darken, "lighten": gfx.Lighten, "color-dodge": gfx.ColorDodge, "color-burn": gfx.ColorBurn,
		"hard-light": gfx.HardLight, "soft-light": gfx.SoftLight, "difference": gfx.Difference,
		"exclusion": gfx.Exclusion, "hue": gfx.Hue, "saturation": gfx.Saturation, "color": gfx.ColorBlend,
		"luminosity": gfx.Luminosity, "plus-lighter": gfx.PlusLighter,
	}
	bl := map[string]Op{}
	for _, k := range sortedMapKeys(blends) {
		m := blends[k]
		bl[k] = op(func(e *env) { e.s.MixBlendMode = m })
	}
	r.enum("mix-blend", false, bl)
	r.unsupported["mix-blend-plus-darker"] = "plus-darker is not supported; use mix-blend-multiply or mix-blend-darken"
	// background-blend-mode takes the Level 1 modes (no plus-lighter).
	bb := map[string]Op{}
	for _, k := range sortedMapKeys(blends) {
		if m := blends[k]; m != gfx.PlusLighter {
			bb[k] = op(func(e *env) { e.s.BackgroundBlend = m })
		}
	}
	r.enum("bg-blend", false, bb)

	// Filters and backdrop filters.
	registerFilters(r, "", func(s *ComputedStyle) *FilterSet { return &s.Filter })
	registerFilters(r, "backdrop-", func(s *ComputedStyle) *FilterSet { return &s.BackdropFilter })
	r.static("filter", Op{})
	r.static("backdrop-filter", Op{})
	r.static("filter-none", op(func(e *env) { e.s.Filter = FilterSet{} }))
	r.static("backdrop-filter-none", op(func(e *env) { e.s.BackdropFilter = FilterSet{} }))
	r.unsupported["backdrop-opacity"] = "backdrop-opacity is not supported; use backdrop-brightness or a translucent background"

	// drop-shadow.
	shadowUtility(r, "drop-shadow", NamespaceDropShadow, func(t *Theme) map[string][]Shadow { return t.DropShadows }, false, slotDropShadow,
		func(e *env, l []Shadow) { e.s.Filter.DropShadow = l })
}

func registerFilters(r *registry, prefix string, target func(*ComputedStyle) *FilterSet) {
	// blur
	r.add(prefix+"blur", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var ex lenExpr
		switch {
		case a.arb:
			e, err := lenArg(c, a, false, false)
			if err != nil {
				return Op{}, err
			}
			ex = e
		case a.raw == "":
			l, ok := c.th.Blur["DEFAULT"]
			if !ok {
				return Op{}, errNoMatch
			}
			ex = exprFromLength(l)
		case a.value == "none":
		default:
			l, ok := c.th.Blur[a.value]
			if !ok || a.value == "DEFAULT" {
				return Op{}, errNoMatch
			}
			ex = exprFromLength(l)
		}
		return op(func(e *env) {
			f := target(e.s)
			f.Set |= FilterBlur
			f.Blur = e.lp(&ex).Px
		}), nil
	}, func(th *Theme) []string { return append(th.Keys(NamespaceBlur), "none") })

	// Amount filters: brightness-150, contrast-50, saturate-200 (default 1
	// is 100%), and grayscale, invert, sepia (default 0, bare = 100%).
	amount := func(name string, bit FilterMask, bareValue float32, field func(*FilterSet) *float32) {
		r.add(prefix+name, false, func(c *pctx, a *arg) (Op, error) {
			var v float32
			switch {
			case a.hasMod:
				return Op{}, errNoMatch
			case a.raw == "":
				if bareValue < 0 {
					return Op{}, errNoMatch
				}
				v = bareValue
			case a.arb:
				n, ok := numberArg(a)
				if !ok {
					return Op{}, fmt.Errorf("%q is not a number; use e.g. %s-[1.25] or %s-125", a.inner, name, name)
				}
				v = n
			default:
				n, ok := parseFloat(a.value)
				if !ok || n < 0 || a.value[0] == '+' {
					return Op{}, errNoMatch
				}
				v = n / 100
			}
			return op(func(e *env) {
				f := target(e.s)
				f.Set |= bit
				*field(f) = v
			}), nil
		}, func(*Theme) []string {
			vals := []string{"0", "50", "75", "100", "125", "150", "200"}
			if bareValue >= 0 {
				vals = append([]string{""}, vals...)
			}
			return vals
		})
	}
	amount("brightness", FilterBrightness, -1, func(f *FilterSet) *float32 { return &f.Brightness })
	amount("contrast", FilterContrast, -1, func(f *FilterSet) *float32 { return &f.Contrast })
	amount("saturate", FilterSaturate, -1, func(f *FilterSet) *float32 { return &f.Saturate })
	amount("grayscale", FilterGrayscale, 1, func(f *FilterSet) *float32 { return &f.Grayscale })
	amount("invert", FilterInvert, 1, func(f *FilterSet) *float32 { return &f.Invert })
	amount("sepia", FilterSepia, 1, func(f *FilterSet) *float32 { return &f.Sepia })

	r.add(prefix+"hue-rotate", true, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var deg float32
		if a.arb {
			d, ok := parseAngle(a.inner)
			if !ok {
				return Op{}, fmt.Errorf("%q is not an angle; use e.g. hue-rotate-[45deg]", a.inner)
			}
			deg = d
		} else {
			n, ok := parseFloat(a.value)
			if !ok || n < 0 || a.value[0] == '+' {
				return Op{}, errNoMatch
			}
			deg = n
		}
		if a.neg {
			deg = -deg
		}
		return op(func(e *env) {
			f := target(e.s)
			f.Set |= FilterHueRotate
			f.HueRotate = deg
		}), nil
	}, func(*Theme) []string { return []string{"0", "15", "30", "60", "90", "180"} })
}
