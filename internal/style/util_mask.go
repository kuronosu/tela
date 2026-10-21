package style

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// Mask utilities (Tailwind v4.3.3, ADR-098).
//
// Image masks: mask-[url(...)] (and mask-[<gradient>], lists of both)
// set mask-image; mask-none removes it. mask-contain, mask-cover,
// mask-auto and mask-size-[...] are mask-size; mask-center, mask-top-left
// ... and mask-position-[...] mask-position; mask-repeat... mask-repeat;
// mask-origin-* and mask-clip-* (mask-no-clip) the boxes; mask-alpha,
// mask-luminance and mask-match mask-mode; mask-add, mask-subtract,
// mask-intersect and mask-exclude mask-composite. mask-type-* applies to
// SVG <mask> elements only and has no effect on elements, as in browsers.
//
// Gradient masks are Tailwind's composition of CSS variables, emulated
// here: any of them sets
//
//	mask-image: var(--tw-mask-linear), var(--tw-mask-radial), var(--tw-mask-conic);
//	mask-composite: intersect;
//
// where each variable defaults to an opaque image (linear-gradient(#fff,
// #fff)). --tw-mask-linear is the four edge gradients
// (mask-t/r/b/l/x/y-from/to-*: linear-gradient(to top, <from-color>
// <from-position>, <to-color> <to-position>) for the top edge), or the
// gradient of mask-linear-<angle> and mask-linear-from/to-*; the radial
// one takes mask-radial-from/to-*, its shape (mask-circle,
// mask-ellipse), size (mask-radial-closest-side..., mask-radial-[...])
// and position (mask-radial-at-*); the conic one mask-conic-<angle> and
// mask-conic-from/to-*. A stop's color defaults to black (from) and
// transparent (to), its position to 0% and 100%.
//
// Where two classes set the same variable, the one that comes last in
// the element's class order wins, as with every other Tela class
// (ARCHITECTURE section 10, ADR-103), not the one Tailwind emits later
// in its stylesheet. Each family variable (--tw-mask-linear and the
// others) takes the form of its last setter: the four edges, an angle
// (linear-gradient(var(--tw-mask-linear-stops, <angle>))) or the stops
// (linear-gradient(var(--tw-mask-linear-stops))); the stops variable is
// the same whichever from/to class sets it, so an angle form is valid
// when any from/to class of the family is present, anywhere in the
// class string. As in a browser, a family whose variable is invalid (an
// angle or size without stops, a circle with two radii, a length on a
// conic stop) makes the whole mask-image invalid at computed-value time:
// the element is not masked at all. An explicit mask-add, -subtract,
// -intersect or -exclude still wins over the intersect the gradient
// classes set, whatever the order (ADR-098).

// maskScratch is the mask state of one style computation.
type maskScratch struct {
	// image: 0 unset, 1 none, 2 explicit layers, 3 Tailwind's gradients.
	image  uint8
	layers []bgImage

	size     BackgroundSize
	posX     LP
	posY     LP
	repeat   BackgroundRepeat
	origin   BackgroundBox
	clip     BackgroundBox
	noClip   bool
	mode     MaskMode
	compSet  bool
	compOp   MaskComposite
	edgesSet [4]bool // top, right, bottom, left
	edges    [4]maskStops
	// linForm is the form of --tw-mask-linear its last setter gave it:
	// 0 unset, 1 the four edges, 2 an angle (valid with stops), 3 the
	// stops. linStops reports a mask-linear-from/to class (which sets
	// --tw-mask-linear-stops).
	linForm   uint8
	linStops  bool
	lin       maskStops
	linAngle  float32
	linCorner Corner // a corner direction from mask-linear-[to_...]
	// radForm: 0 unset, 1 a size (valid with stops), 2 the stops;
	// radStops reports a mask-radial-from/to class.
	radForm   uint8
	radStops  bool
	rad       maskStops
	radShape  RadialShape
	radSize   RadialSize
	radRadius [2]LP
	radNVals  int // explicit size values (1 or 2)
	radPctR   bool
	radX      LP
	radY      LP
	radPosSet bool
	// conForm: 0 unset, 1 an angle (valid with stops), 2 the stops;
	// conStops reports a mask-conic-from/to class.
	conForm  uint8
	conStops bool
	con      maskStops
	conAngle float32
}

// maskStops are the from and to stops of one gradient variable family.
type maskStops struct {
	from, to       colorSpec
	fromPos, toPos StopPos
	// lengthPos marks a stop position that is a length (invalid on a
	// conic gradient).
	lengthPos bool
}

func (m *maskScratch) gradients() { m.image = 3 }

// Edges in the order of maskScratch.edges, with their direction.
var maskEdges = [4]struct {
	name  string
	angle float32
}{{"t", 0}, {"r", 90}, {"b", 180}, {"l", 270}}

func registerMasks(r *registry) {
	keys := map[string]Op{
		"none":    op(func(e *env) { e.sc.mask.image, e.sc.mask.layers = 1, nil }),
		"contain": op(func(e *env) { e.sc.mask.size = BackgroundSize{Kind: SizeContain} }),
		"cover":   op(func(e *env) { e.sc.mask.size = BackgroundSize{Kind: SizeCover} }),
		"auto":    op(func(e *env) { e.sc.mask.size = BackgroundSize{} }),
		"repeat":  maskRepeatOp(RepeatBoth), "no-repeat": maskRepeatOp(RepeatNone),
		"repeat-x": maskRepeatOp(RepeatX), "repeat-y": maskRepeatOp(RepeatY),
		"repeat-round": maskRepeatOp(RepeatRound), "repeat-space": maskRepeatOp(RepeatSpace),
		"alpha":     op(func(e *env) { e.sc.mask.mode = MaskModeAlpha }),
		"luminance": op(func(e *env) { e.sc.mask.mode = MaskModeLuminance }),
		"match":     op(func(e *env) { e.sc.mask.mode = MaskMatchSource }),
		"add":       maskCompositeOp(MaskAdd), "subtract": maskCompositeOp(MaskSubtract),
		"intersect": maskCompositeOp(MaskIntersect), "exclude": maskCompositeOp(MaskExclude),
		"no-clip": op(func(e *env) { e.sc.mask.noClip = true }),
		"circle":  op(func(e *env) { e.sc.mask.radShape = ShapeCircle }),
		"ellipse": op(func(e *env) { e.sc.mask.radShape = ShapeEllipse }),
	}
	for _, k := range []string{"center", "top", "bottom", "left", "right", "top-left", "top-right", "bottom-left", "bottom-right"} {
		p := backgroundPositions[k]
		keys[k] = op(func(e *env) { e.sc.mask.posX, e.sc.mask.posY = PctLP(p[0]), PctLP(p[1]) })
	}
	r.enum("mask", false, keys)
	r.add("mask", false, maskArbitrary, nil)
	r.enum("mask-type", false, map[string]Op{"alpha": {}, "luminance": {}})

	boxes := map[string]BackgroundBox{"border": BackgroundBorderBox, "padding": BackgroundPaddingBox,
		"content": BackgroundContentBox, "fill": BackgroundContentBox, "stroke": BackgroundBorderBox, "view": BackgroundBorderBox}
	clip, origin := map[string]Op{}, map[string]Op{}
	for k, b := range boxes {
		clip[k] = op(func(e *env) { e.sc.mask.clip, e.sc.mask.noClip = b, false })
		origin[k] = op(func(e *env) { e.sc.mask.origin = b })
	}
	r.enum("mask-clip", false, clip)
	r.enum("mask-origin", false, origin)
	r.add("mask-size", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod {
			return Op{}, errNoMatch
		}
		return maskSizeOp(a.inner)
	}, nil)
	r.add("mask-position", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod {
			return Op{}, errNoMatch
		}
		return maskPositionOp(a.inner)
	}, nil)

	// Edge gradients.
	sides := map[string][]int{"t": {0}, "r": {1}, "b": {2}, "l": {3}, "x": {1, 3}, "y": {0, 2}}
	for _, name := range sortedMapKeys(sides) {
		idx := sides[name]
		for _, end := range []string{"from", "to"} {
			r.add("mask-"+name+"-"+end, false, maskStopUtility(end == "from", false, func(e *env, f func(*maskStops)) {
				m := &e.sc.mask
				m.gradients()
				for _, i := range idx {
					m.edgesSet[i] = true
					f(&m.edges[i])
				}
				m.linForm = 1
			}), maskStopValues)
		}
	}
	// Linear.
	r.add("mask-linear", true, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var angle float32
		corner := CornerNone
		switch {
		case a.arb:
			in := strings.TrimSpace(a.inner)
			if strings.HasPrefix(in, "to ") {
				var dir Gradient
				if err := parseLinearDirection(in, &dir); err != nil {
					return Op{}, err
				}
				angle, corner = dir.Angle, dir.Corner
			} else if v, ok := parseAngle(in); ok {
				angle = v
			} else {
				return Op{}, fmt.Errorf("%q is not a gradient direction; use an angle such as mask-linear-[45deg] or a side such as mask-linear-[to_right]", in)
			}
		default:
			n, ok := parseFloat(a.value)
			if !ok || n < 0 || strings.ContainsAny(a.value, "+-") {
				return Op{}, errNoMatch
			}
			angle = n
		}
		if a.neg {
			if a.arb {
				return Op{}, errNoMatch
			}
			angle = -angle
		}
		return op(func(e *env) {
			m := &e.sc.mask
			m.gradients()
			m.linAngle, m.linCorner = angle, corner
			m.linForm = 2
		}), nil
	}, func(*Theme) []string { return []string{"0", "45", "90", "135", "180", "225", "270", "315"} })
	for _, end := range []string{"from", "to"} {
		r.add("mask-linear-"+end, false, maskStopUtility(end == "from", false, func(e *env, f func(*maskStops)) {
			m := &e.sc.mask
			m.gradients()
			f(&m.lin)
			m.linForm, m.linStops = 3, true
		}), maskStopValues)
	}
	// Radial.
	r.add("mask-radial", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod || !a.arb {
			return Op{}, errNoMatch
		}
		var g Gradient
		f := strings.Fields(a.inner)
		if len(f) == 0 || len(f) > 2 {
			return Op{}, fmt.Errorf("%q is not a radial gradient size; use a keyword such as closest-side or one or two lengths", a.inner)
		}
		if err := parseRadialPrelude(a.inner, &g); err != nil {
			return Op{}, err
		}
		for _, w := range f {
			if w == "at" || w == "circle" || w == "ellipse" {
				return Op{}, fmt.Errorf("mask-radial-[...] takes a size only (closest-side, 40%%_60%%, 3rem); set the shape with mask-circle and the position with mask-radial-at-*")
			}
		}
		size, radius, n := g.Size, g.Radius, 0
		pct := false
		if size == SizeExplicit {
			n = len(f)
			pct = radius[0].Pct != 0 || radius[1].Pct != 0
		}
		return op(func(e *env) {
			m := &e.sc.mask
			m.gradients()
			m.radSize, m.radRadius, m.radNVals, m.radPctR = size, radius, n, pct
			m.radForm = 1
		}), nil
	}, nil)
	sizes := map[string]RadialSize{"closest-side": SizeClosestSide, "closest-corner": SizeClosestCorner,
		"farthest-side": SizeFarthestSide, "farthest-corner": SizeFarthestCorner}
	sizeOps := map[string]Op{}
	for k, s := range sizes {
		sizeOps[k] = op(func(e *env) {
			m := &e.sc.mask
			m.radSize, m.radNVals, m.radPctR = s, 0, false
		})
	}
	r.enum("mask-radial", false, sizeOps)
	atOps := map[string]Op{}
	for _, k := range []string{"center", "top", "bottom", "left", "right", "top-left", "top-right", "bottom-left", "bottom-right"} {
		p := backgroundPositions[k]
		atOps[k] = op(func(e *env) {
			m := &e.sc.mask
			m.radX, m.radY, m.radPosSet = PctLP(p[0]), PctLP(p[1]), true
		})
	}
	r.enum("mask-radial-at", false, atOps)
	r.add("mask-radial-at", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod {
			return Op{}, errNoMatch
		}
		x, y, err := parsePositionPair(strings.Fields(a.inner))
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) {
			m := &e.sc.mask
			m.radX, m.radY, m.radPosSet = x, y, true
		}), nil
	}, nil)
	for _, end := range []string{"from", "to"} {
		r.add("mask-radial-"+end, false, maskStopUtility(end == "from", false, func(e *env, f func(*maskStops)) {
			m := &e.sc.mask
			m.gradients()
			f(&m.rad)
			m.radForm, m.radStops = 2, true
		}), maskStopValues)
	}
	// Conic.
	r.add("mask-conic", true, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var angle float32
		if a.arb {
			v, ok := parseAngle(strings.TrimSpace(a.inner))
			if !ok || a.neg {
				return Op{}, fmt.Errorf("%q is not an angle; use e.g. mask-conic-[30deg]", a.inner)
			}
			angle = v
		} else {
			n, ok := parseFloat(a.value)
			if !ok || n < 0 || strings.ContainsAny(a.value, "+-") {
				return Op{}, errNoMatch
			}
			angle = n
			if a.neg {
				angle = -n
			}
		}
		return op(func(e *env) {
			m := &e.sc.mask
			m.gradients()
			m.conAngle = angle
			m.conForm = 1
		}), nil
	}, func(*Theme) []string { return []string{"0", "45", "90", "135", "180", "225", "270", "315"} })
	for _, end := range []string{"from", "to"} {
		r.add("mask-conic-"+end, false, maskStopUtility(end == "from", true, func(e *env, f func(*maskStops)) {
			m := &e.sc.mask
			m.gradients()
			f(&m.con)
			m.conForm, m.conStops = 2, true
		}), maskStopValues)
	}
}

func maskRepeatOp(rp BackgroundRepeat) Op { return op(func(e *env) { e.sc.mask.repeat = rp }) }

func maskCompositeOp(c MaskComposite) Op {
	return op(func(e *env) { e.sc.mask.compSet, e.sc.mask.compOp = true, c })
}

func maskStopValues(th *Theme) []string {
	return append(colorKeys(th), "0%", "10%", "25%", "50%", "75%", "90%", "100%")
}

// maskStopUtility handles the from and to classes of a gradient family:
// a position (a percentage, a spacing number, a length or, for conic
// gradients, an angle in brackets) or a color. apply runs the change on
// the family's stops.
func maskStopUtility(from, conic bool, apply func(e *env, f func(*maskStops))) handler {
	return func(c *pctx, a *arg) (Op, error) {
		if a.raw == "" {
			return Op{}, errNoMatch
		}
		// A position.
		if !a.hasMod {
			var x lenExpr
			isLen, ok := false, false
			switch {
			case a.arb && (a.hint == "" || a.hint == "length" || a.hint == "percentage") && !looksLikeColor(a.inner):
				in := strings.TrimSpace(a.inner)
				if v, isAngle := parseAngle(in); isAngle && conic && v != 0 {
					pct := float32(float64(v) / 360 * 100)
					return op(func(e *env) {
						apply(e, func(s *maskStops) { setStopPos(s, from, StopPos{Set: true, LP: PctLP(pct)}, false) })
					}), nil
				}
				e, err := parseLengthExpr(in, true)
				if err != nil {
					return Op{}, fmt.Errorf("%q is not a gradient stop position; use a percentage or a length", in)
				}
				x, ok, isLen = e, true, !strings.HasSuffix(in, "%")
			case !a.arb && strings.HasSuffix(a.value, "%"):
				e, err := parseLengthExpr(a.value, true)
				if err != nil {
					return Op{}, errNoMatch
				}
				x, ok = e, true
			case !a.arb:
				if e, sp := spacingExpr(c.th, a.value); sp {
					x, ok, isLen = e, true, true
				}
			}
			if ok {
				return op(func(e *env) {
					lp := e.lp(&x)
					apply(e, func(s *maskStops) { setStopPos(s, from, StopPos{Set: true, LP: lp}, isLen) })
				}), nil
			}
		}
		spec, err := colorArg(c, a)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) {
			sp := spec.bind(e.theme, e.dark)
			apply(e, func(s *maskStops) {
				if from {
					s.from = sp
				} else {
					s.to = sp
				}
			})
		}), nil
	}
}

func setStopPos(s *maskStops, from bool, p StopPos, isLen bool) {
	if from {
		s.fromPos = p
	} else {
		s.toPos = p
	}
	if isLen {
		s.lengthPos = true
	}
}

// maskArbitrary handles mask-[...]: an image list (url(), gradient
// functions), a size ([size:...]) or a position ([position:...]).
func maskArbitrary(c *pctx, a *arg) (Op, error) {
	if !a.arb || a.hasMod {
		return Op{}, errNoMatch
	}
	in := strings.TrimSpace(a.inner)
	if in == "" {
		return Op{}, fmt.Errorf("the value is empty; use e.g. mask-[url(icons/star.svg)]")
	}
	switch a.hint {
	case "size", "length", "bg-size":
		return maskSizeOp(in)
	case "position":
		return maskPositionOp(in)
	case "", "url", "image":
	default:
		return Op{}, errNoMatch
	}
	items := splitTop(in, ',')
	list := make([]bgImage, len(items))
	for i, it := range items {
		it = strings.TrimSpace(it)
		if !bgImageItem(it) {
			return Op{}, fmt.Errorf("%q is not a mask image; use url(path/to/mask.png) or a gradient such as linear-gradient(black,transparent)", it)
		}
		img, err := parseBgImage(c, it)
		if err != nil {
			return Op{}, err
		}
		if img.kind == 1 {
			img.gradient.Interp = gfx.InterpSRGB // CSS's default; Tailwind's bg-linear-* add "in oklab"
		}
		list[i] = img
	}
	return op(func(e *env) { e.sc.mask.image, e.sc.mask.layers = 2, list }), nil
}

func maskSizeOp(in string) (Op, error) {
	o, err := bgSizeOp(in)
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) {
		save := e.sc.bgSize
		o.apply(e)
		e.sc.mask.size, e.sc.bgSize = e.sc.bgSize, save
	}), nil
}

func maskPositionOp(in string) (Op, error) {
	x, y, err := parsePositionPair(strings.Fields(in))
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.sc.mask.posX, e.sc.mask.posY = x, y }), nil
}

// finalizeMask builds the mask layers from the scratch state.
func (e *env) finalizeMask(cur gfx.Color) {
	m := &e.sc.mask
	s := e.s
	var items []BackgroundLayer
	switch m.image {
	case 2:
		for _, it := range m.layers {
			l := BackgroundLayer{}
			if it.kind == 2 {
				l.URL = it.url
			} else {
				g := it.gradient
				l.Gradient = &g
			}
			items = append(items, l)
		}
	case 3:
		var ok bool
		if items, ok = m.gradientLayers(cur); !ok {
			return // invalid at computed-value time: no mask
		}
	default:
		return
	}
	base := BackgroundLayer{Size: m.size, PosX: m.posX, PosY: m.posY, Repeat: m.repeat, Origin: m.origin}
	for i := range items {
		g, u := items[i].Gradient, items[i].URL
		items[i] = base
		items[i].Gradient, items[i].URL = g, u
	}
	comp := MaskAdd
	if m.image == 3 {
		comp = MaskIntersect
	}
	if m.compSet {
		comp = m.compOp
	}
	s.Mask = Mask{Layers: items, Mode: m.mode, Composite: comp, Clip: m.clip, NoClip: m.noClip}
}

// opaqueMask is the initial value of Tailwind's mask variables:
// linear-gradient(#fff, #fff).
func opaqueMask() *Gradient {
	g := newGradient(GradientLinear)
	g.Interp = gfx.InterpSRGB
	g.Stops = []ColorStop{{Color: gfx.White}, {Color: gfx.White}}
	return &g
}

// stops returns the two stops of a family.
func (s *maskStops) stops(cur gfx.Color) []ColorStop {
	from, to := gfx.Black, gfx.Transparent
	if s.from.set {
		from = s.from.resolve(cur)
	}
	if s.to.set {
		to = s.to.resolve(cur)
	}
	fp, tp := StopPos{Set: true, LP: PctLP(0)}, StopPos{Set: true, LP: PctLP(100)}
	if s.fromPos.Set {
		fp = s.fromPos
	}
	if s.toPos.Set {
		tp = s.toPos
	}
	return []ColorStop{{from, fp}, {to, tp}}
}

// gradientLayers returns the layers of Tailwind's gradient masks: the
// linear variable (four edges or one gradient), the radial and the conic
// ones. ok is false when one of them is invalid.
func (m *maskScratch) gradientLayers(cur gfx.Color) (layers []BackgroundLayer, ok bool) {
	layer := func(g *Gradient) BackgroundLayer { return BackgroundLayer{Gradient: g} }
	switch m.linForm {
	case 0:
		layers = append(layers, layer(opaqueMask()))
	case 1:
		// left, right, bottom, top (the order of --tw-mask-linear).
		for _, i := range [4]int{3, 1, 2, 0} {
			if !m.edgesSet[i] {
				layers = append(layers, layer(opaqueMask()))
				continue
			}
			g := newGradient(GradientLinear)
			g.Angle, g.Interp = maskEdges[i].angle, gfx.InterpSRGB
			g.Stops = m.edges[i].stops(cur)
			layers = append(layers, layer(&g))
		}
	case 2, 3:
		if !m.linStops {
			return nil, false // linear-gradient(<angle>): no stops
		}
		g := newGradient(GradientLinear)
		g.Angle, g.Corner, g.Interp = m.linAngle, m.linCorner, gfx.InterpSRGB
		g.Stops = m.lin.stops(cur)
		layers = append(layers, layer(&g))
	}
	switch m.radForm {
	case 0:
		layers = append(layers, layer(opaqueMask()))
	case 1, 2:
		if !m.radStops {
			return nil, false // radial-gradient(<size>): no stops
		}
		g := newGradient(GradientRadial)
		g.Interp = gfx.InterpSRGB
		g.Shape, g.Size, g.Radius = m.radShape, m.radSize, m.radRadius
		if m.radSize == SizeExplicit {
			switch {
			case m.radShape == ShapeCircle && (m.radNVals != 1 || m.radPctR):
				return nil, false // a circle takes one length
			case m.radShape == ShapeEllipse && m.radNVals != 2:
				return nil, false // an ellipse takes two
			}
		}
		if m.radPosSet {
			g.CenterX, g.CenterY = m.radX, m.radY
		}
		g.Stops = m.rad.stops(cur)
		layers = append(layers, layer(&g))
	}
	switch m.conForm {
	case 0:
		layers = append(layers, layer(opaqueMask()))
	case 1, 2:
		if !m.conStops {
			return nil, false // conic-gradient(<angle>): no stops
		}
		if m.con.lengthPos {
			return nil, false // conic stops are angles or percentages
		}
		g := newGradient(GradientConic)
		g.Angle, g.Interp = m.conAngle, gfx.InterpSRGB
		g.Stops = m.con.stops(cur)
		layers = append(layers, layer(&g))
	}
	return layers, true
}
