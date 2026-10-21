package style

import (
	"errors"
	"fmt"
	"strings"
)

// SVG paints and replaced content (ADR-098): fill-*, stroke-*, stroke-<n>,
// object-fit and object-position.

func registerMedia(r *registry) {
	// fill-* and stroke-*: none, or a color (palette, token, current,
	// arbitrary, /NN). currentColor stays a keyword, resolved where the
	// document is drawn (SVGPaintCurrent); a token is bound for the
	// element's scheme when the op runs.
	for _, root := range []string{"fill", "stroke"} {
		set := func(e *env, p SVGPaint) {
			if root == "fill" {
				e.s.Fill = p
			} else {
				e.s.Stroke = p
			}
		}
		r.enum(root, false, map[string]Op{
			"none": op(func(e *env) { set(e, SVGPaint{Kind: SVGPaintNone}) }),
		})
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			if a.raw == "" || a.value == "none" {
				return Op{}, errNoMatch
			}
			spec, err := colorArg(c, a)
			if err != nil {
				return Op{}, err
			}
			return op(func(e *env) {
				sp := spec.bind(e.theme, e.dark)
				if sp.current {
					set(e, SVGPaint{Kind: SVGPaintCurrent, Color: sp.c})
				} else {
					set(e, SVGPaint{Kind: SVGPaintColor, Color: sp.c})
				}
			}), nil
		}, colorKeys)
	}
	// stroke-<n>: stroke-width in SVG user units (Tailwind: an integer
	// bare value, any number or length in brackets).
	r.add("stroke", false, strokeWidthUtility, func(*Theme) []string {
		return []string{"0", "1", "2", "3", "4"}
	})

	// object-fit and object-position.
	fits := map[string]Op{}
	for i, name := range objectFitNames {
		f := ObjectFit(i)
		fits[name] = op(func(e *env) { e.s.ObjectFit = f })
	}
	for _, k := range sortedMapKeys(backgroundPositions) {
		p := backgroundPositions[k]
		fits[k] = op(func(e *env) { e.s.ObjectPosX, e.s.ObjectPosY = PctLP(p[0]), PctLP(p[1]) })
	}
	r.enum("object", false, fits)
	r.add("object", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod || (a.hint != "" && a.hint != "position") {
			return Op{}, errNoMatch
		}
		x, y, err := parsePositionPair(strings.Fields(a.inner))
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { e.s.ObjectPosX, e.s.ObjectPosY = x, y }), nil
	}, nil)
}

// strokeWidthUtility handles stroke-2, stroke-[1.5], stroke-[3px] and
// stroke-[length:0.1em].
func strokeWidthUtility(c *pctx, a *arg) (Op, error) {
	if a.hasMod {
		return Op{}, errNoMatch
	}
	if !a.arb {
		if a.value == "" || strings.ContainsAny(a.value, ".+-eE") {
			return Op{}, errNoMatch
		}
		n, ok := parseFloat(a.value)
		if !ok || n < 0 {
			return Op{}, errNoMatch
		}
		return op(func(e *env) { e.s.StrokeWidth, e.s.StrokeWidthSet = n, true }), nil
	}
	switch a.hint {
	case "", "length", "number", "line-width":
	default:
		return Op{}, errNoMatch
	}
	in := strings.TrimSpace(a.inner)
	if n, ok := parseFloat(in); ok {
		if n < 0 {
			return Op{}, fmt.Errorf("a stroke width cannot be negative")
		}
		return op(func(e *env) { e.s.StrokeWidth, e.s.StrokeWidthSet = n, true }), nil
	}
	if a.hint == "" && looksLikeColor(in) {
		return Op{}, errNoMatch
	}
	if strings.HasSuffix(in, "%") {
		return Op{}, unsupportedErr("a stroke width in percent is not supported; use a number of user units (stroke-[1.5]) or a length (stroke-[2px])")
	}
	x, err := parseLengthExpr(in, false)
	if err != nil {
		if errors.Is(err, errNotLength) && a.hint == "" {
			return Op{}, errNoMatch
		}
		return Op{}, fmt.Errorf("%q is not a stroke width; use a number of user units such as stroke-[1.5] or a length such as stroke-[2px]", in)
	}
	return op(func(e *env) {
		e.s.StrokeWidth, e.s.StrokeWidthSet = max(e.lp(&x).Px, 0), true
	}), nil
}
