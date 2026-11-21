package style

import (
	"sort"
	"strings"
)

// The second half of the describer's tables: the utilities that read like
// a CSS keyword (blend modes, boxes, animation controls), the sizing of
// containers and screens, and the mask utilities, whose gradients are
// Tailwind's composition of custom properties (see util_mask.go). A test
// walks the whole vocabulary and fails on any class that falls back to its
// utility name.

// boxWords maps the short names of background and mask boxes to CSS.
var boxWords = map[string]string{"border": "border-box", "padding": "padding-box", "content": "content-box"}

// positionWords are the keywords of background-position, object-position
// and transform-origin, in the order they are written.
var positionWords = []string{
	"center", "top", "bottom", "left", "right",
	"top-left", "top-right", "bottom-left", "bottom-right",
	"left-top", "left-bottom", "right-top", "right-bottom",
}

// positionOf turns a class position ("bottom-left") into CSS ("bottom left").
func positionOf(v string) (string, bool) {
	for _, w := range positionWords {
		if v == w {
			return strings.ReplaceAll(v, "-", " "), true
		}
	}
	return "", false
}

func descInitMore() {
	for k, v := range map[string][]Declaration{
		"ring-inset":           ds("--tw-ring-inset", "inset"),
		"rotate-none":          ds("rotate", "none"),
		"scale-none":           ds("scale", "none"),
		"translate-none":       ds("translate", "none"),
		"transform":            ds("transform", "var(--tw-rotate-x,) var(--tw-rotate-y,) var(--tw-rotate-z,) var(--tw-skew-x,) var(--tw-skew-y,)"),
		"transform-cpu":        ds("transform", "var(--tw-rotate-x,) var(--tw-rotate-y,) var(--tw-rotate-z,) var(--tw-skew-x,) var(--tw-skew-y,)"),
		"transform-gpu":        ds("transform", "translateZ(0) var(--tw-rotate-x,) var(--tw-rotate-y,) var(--tw-rotate-z,) var(--tw-skew-x,) var(--tw-skew-y,)"),
		"wrap-anywhere":        ds("overflow-wrap", "anywhere"),
		"wrap-break-word":      ds("overflow-wrap", "break-word"),
		"wrap-normal":          ds("overflow-wrap", "normal"),
		"diagonal-fractions":   ds("font-variant-numeric", "diagonal-fractions"),
		"stacked-fractions":    ds("font-variant-numeric", "stacked-fractions"),
		"filter":               ds("filter", "var(--tw-blur,) var(--tw-brightness,) var(--tw-contrast,) var(--tw-grayscale,) var(--tw-hue-rotate,) var(--tw-invert,) var(--tw-saturate,) var(--tw-sepia,) var(--tw-drop-shadow,)"),
		"filter-none":          ds("filter", "none"),
		"backdrop-filter":      ds("backdrop-filter", "var(--tw-backdrop-blur,) var(--tw-backdrop-brightness,) var(--tw-backdrop-contrast,) var(--tw-backdrop-grayscale,) var(--tw-backdrop-hue-rotate,) var(--tw-backdrop-invert,) var(--tw-backdrop-opacity,) var(--tw-backdrop-saturate,) var(--tw-backdrop-sepia,)"),
		"backdrop-filter-none": ds("backdrop-filter", "none"),
	} {
		descStatic[k] = v
	}
	for _, d := range [][2]string{{"x", "inline"}, {"y", "block"}} {
		descStatic["divide-"+d[0]+"-reverse"] = ds("--tw-divide-"+d[0]+"-reverse", "1 /* between the children */")
	}

	more := map[string]descFn{
		"container":          containerFn,
		"direction":          enumOf("animation-direction", keywords("normal", "reverse", "alternate", "alternate-reverse", "initial")),
		"fill-mode":          enumOf("animation-fill-mode", keywords("none", "forwards", "backwards", "both", "initial")),
		"field-sizing":       enumOf("field-sizing", keywords("content", "fixed")),
		"scroll":             enumOf("scroll-behavior", keywords("auto", "smooth", "instant")),
		"overscroll-x":       enumOf("overscroll-behavior-x", keywords("auto", "contain", "none")),
		"overscroll-y":       enumOf("overscroll-behavior-y", keywords("auto", "contain", "none")),
		"auto-cols":          enumOf("grid-auto-columns", map[string]string{"auto": "auto", "min": "min-content", "max": "max-content", "fr": "minmax(0, 1fr)"}),
		"auto-rows":          enumOf("grid-auto-rows", map[string]string{"auto": "auto", "min": "min-content", "max": "max-content", "fr": "minmax(0, 1fr)"}),
		"bg-blend":           enumOf("background-blend-mode", blendModes()),
		"bg-clip":            boxFn("background-clip"),
		"bg-origin":          boxFn("background-origin"),
		"bg-position":        positionFn("background-position"),
		"font-stretch":       fontStretchFn,
		"animation-duration": durationFn("animation-duration"),
		"animation-delay": func(c *descCtx, v string) []Declaration {
			if n, ok := number(v); ok && v != "" {
				return ds("animation-delay", numF(c.signed(n))+"ms")
			}
			return durationFn("animation-delay")(c, v)
		},
		"repeat":              repeatFn,
		"backdrop-hue-rotate": func(c *descCtx, v string) []Declaration { return rotateFilter("backdrop-filter", c, v) },
		"bg-conic":            conicFn("background-image", "var(--tw-gradient-stops)"),
	}
	for k, fn := range more {
		prev := descRoots[k]
		if prev == nil {
			descRoots[k] = fn
			continue
		}
		descRoots[k] = func(c *descCtx, v string) []Declaration {
			if ds := prev(c, v); ds != nil {
				return ds
			}
			return fn(c, v)
		}
	}
	descMasks()
}

// containerFn describes container: full width, capped at each breakpoint.
func containerFn(c *descCtx, v string) []Declaration {
	if v != "" {
		return nil
	}
	names := c.th.Keys(NamespaceBreakpoint)
	sort.SliceStable(names, func(i, j int) bool {
		return c.th.Breakpoints[names[i]].Value < c.th.Breakpoints[names[j]].Value
	})
	out := ds("width", "100%")
	for _, bp := range names {
		if l, ok := c.th.Breakpoints[bp]; ok {
			out = append(out, Declaration{"max-width", l.String() + " /* from " + bp + " up */"})
		}
	}
	return out
}

// blendModes are the blend modes the blend utilities take.
func blendModes() map[string]string {
	return keywords("normal", "multiply", "screen", "overlay", "darken", "lighten", "color-dodge", "color-burn",
		"hard-light", "soft-light", "difference", "exclusion", "hue", "saturation", "color", "luminosity",
		"plus-lighter", "plus-darker")
}

// boxFn describes a utility whose value is a box (border, padding,
// content), with the box suffix CSS gives it.
func boxFn(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if b, ok := boxWords[v]; ok {
			return ds(property, b)
		}
		return nil
	}
}

// positionFn describes a utility whose value is a position keyword.
func positionFn(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if p, ok := positionOf(v); ok {
			return ds(property, p)
		}
		if a, ok := arbitrary(v); ok {
			return ds(property, a)
		}
		return nil
	}
}

// fontStretchFn describes font-stretch-*: the keywords as the percentages
// Tailwind gives them, or a percentage or arbitrary value.
func fontStretchFn(c *descCtx, v string) []Declaration {
	stretch := map[string]string{
		"ultra-condensed": "50%", "extra-condensed": "62.5%", "condensed": "75%", "semi-condensed": "87.5%",
		"normal": "100%", "semi-expanded": "112.5%", "expanded": "125%", "extra-expanded": "150%", "ultra-expanded": "200%",
	}
	if s, ok := stretch[v]; ok {
		return ds("font-stretch", s)
	}
	if strings.HasSuffix(v, "%") && isNumber(strings.TrimSuffix(v, "%")) {
		return ds("font-stretch", v)
	}
	if a, ok := arbitrary(v); ok {
		return ds("font-stretch", a)
	}
	return nil
}

// repeatFn describes repeat-*, the iteration count of an animation.
func repeatFn(c *descCtx, v string) []Declaration {
	switch {
	case v == "infinite" || v == "initial":
		return ds("animation-iteration-count", v)
	case isNumber(v):
		return ds("animation-iteration-count", v)
	}
	if a, ok := arbitrary(v); ok {
		return ds("animation-iteration-count", a)
	}
	return nil
}

// conicFn describes a conic gradient from an angle: bg-conic-45.
func conicFn(property, stops string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if n, ok := number(v); ok && v != "" {
			return ds(property, "conic-gradient(from "+numF(c.signed(n))+"deg in oklab, "+stops+")")
		}
		if a, ok := arbitrary(v); ok && !c.neg {
			return ds(property, "conic-gradient("+a+", "+stops+")")
		}
		return nil
	}
}

// ---------------------------------------------------------------------
// Masks.

// maskGradient is what any gradient mask class adds to its own custom
// property: the composition of the three gradient families, intersected.
func maskGradient(pairs ...string) []Declaration {
	return ds(append(pairs,
		"mask-image", "var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic)",
		"mask-composite", "intersect")...)
}

// maskEdges are the sides of the edge gradients, with the custom property
// names of their sides.
var maskSideNames = map[string][]string{
	"t": {"top"}, "r": {"right"}, "b": {"bottom"}, "l": {"left"},
	"x": {"left", "right"}, "y": {"top", "bottom"},
}

// maskStopFn describes a stop of a gradient mask: family is the name used in
// the custom property ("top", "linear", "radial", "conic"), end is "from" or
// "to"; a color sets the stop's color and a percentage or a length its
// position.
func maskStopFn(families []string, end string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if v == "" {
			return nil
		}
		var pairs []string
		set := func(kind, value string) {
			for _, f := range families {
				pairs = append(pairs, "--tw-mask-"+f+"-"+end+"-"+kind, value)
			}
		}
		switch {
		case strings.HasSuffix(v, "%") && isNumber(strings.TrimSuffix(v, "%")):
			set("position", v)
		default:
			if col, ok := c.color(v); ok {
				set("color", col)
				break
			}
			if a, ok := arbitrary(v); ok {
				set("position", a)
				break
			}
			if s, ok := c.spacing(v); ok {
				set("position", s)
				break
			}
			return nil
		}
		return maskGradient(pairs...)
	}
}

// maskAngleFn describes mask-linear-<angle> and mask-conic-<angle>.
func maskAngleFn(family string) descFn {
	return func(c *descCtx, v string) []Declaration {
		var arg string
		switch n, ok := number(v); {
		case ok && v != "":
			arg = numF(c.signed(n)) + "deg"
		default:
			a, isArb := arbitrary(v)
			if !isArb || c.neg {
				return nil
			}
			arg = a
		}
		stops := "var(--tw-mask-" + family + "-stops)"
		if family == "conic" {
			return maskGradient("--tw-mask-conic", "conic-gradient(from "+arg+", "+stops+")")
		}
		return maskGradient("--tw-mask-linear", "linear-gradient("+arg+", "+stops+")")
	}
}

// maskFn describes mask-<keyword> and mask-[<image>].
func maskFn(c *descCtx, v string) []Declaration {
	switch v {
	case "none":
		return ds("mask-image", "none")
	case "auto", "cover", "contain":
		return ds("mask-size", v)
	case "add", "subtract", "intersect", "exclude":
		return ds("mask-composite", v)
	case "alpha", "luminance":
		return ds("mask-mode", v)
	case "match":
		return ds("mask-mode", "match-source")
	case "no-clip":
		return ds("mask-clip", "no-clip")
	case "repeat", "no-repeat", "repeat-x", "repeat-y", "repeat-round", "repeat-space":
		return ds("mask-repeat", v)
	case "circle", "ellipse":
		return maskGradient("--tw-mask-radial-shape", v)
	}
	if p, ok := positionOf(v); ok {
		return ds("mask-position", p)
	}
	if a, ok := arbitrary(v); ok {
		if strings.HasPrefix(a, "size:") {
			return ds("mask-size", strings.TrimPrefix(a, "size:"))
		}
		if strings.HasPrefix(a, "position:") {
			return ds("mask-position", strings.TrimPrefix(a, "position:"))
		}
		return ds("mask-image", a)
	}
	return nil
}

// maskBoxFn describes mask-clip-* and mask-origin-*: the boxes of CSS plus
// the SVG ones.
func maskBoxFn(property string) descFn {
	boxes := merge(boxWords, map[string]string{"fill": "fill-box", "stroke": "stroke-box", "view": "view-box"})
	return func(c *descCtx, v string) []Declaration {
		if b, ok := boxes[v]; ok {
			return ds(property, b)
		}
		return nil
	}
}

// maskRadialFn describes mask-radial-<size> and mask-radial-[<size>].
func maskRadialFn(c *descCtx, v string) []Declaration {
	switch v {
	case "closest-side", "closest-corner", "farthest-side", "farthest-corner":
		return maskGradient("--tw-mask-radial-size", v)
	}
	if a, ok := arbitrary(v); ok {
		return maskGradient("--tw-mask-radial", "radial-gradient("+a+", var(--tw-mask-radial-stops))")
	}
	return nil
}

// maskRadialAtFn describes mask-radial-at-<position>.
func maskRadialAtFn(c *descCtx, v string) []Declaration {
	if p, ok := positionOf(v); ok {
		return maskGradient("--tw-mask-radial-position", p)
	}
	if a, ok := arbitrary(v); ok {
		return maskGradient("--tw-mask-radial-position", a)
	}
	return nil
}

func descMasks() {
	descRoots["mask"] = maskFn
	descRoots["mask-clip"] = maskBoxFn("mask-clip")
	descRoots["mask-origin"] = maskBoxFn("mask-origin")
	descRoots["mask-type"] = enumOf("mask-type", keywords("alpha", "luminance"))
	descRoots["mask-size"] = func(c *descCtx, v string) []Declaration {
		if a, ok := arbitrary(v); ok {
			return ds("mask-size", a)
		}
		return nil
	}
	descRoots["mask-position"] = func(c *descCtx, v string) []Declaration {
		if a, ok := arbitrary(v); ok {
			return ds("mask-position", a)
		}
		return nil
	}
	descRoots["mask-linear"] = maskAngleFn("linear")
	descRoots["mask-conic"] = maskAngleFn("conic")
	descRoots["mask-radial"] = maskRadialFn
	descRoots["mask-radial-at"] = maskRadialAtFn
	for _, end := range []string{"from", "to"} {
		for side, names := range maskSideNames {
			descRoots["mask-"+side+"-"+end] = maskStopFn(names, end)
		}
		for _, family := range []string{"linear", "radial", "conic"} {
			descRoots["mask-"+family+"-"+end] = maskStopFn([]string{family}, end)
		}
	}
}
