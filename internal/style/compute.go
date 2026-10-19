package style

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// Context is the environment shared by every style computation of a
// frame: the theme, the window and the user's preferences.
type Context struct {
	// Theme provides the root font size, the default fonts and the
	// transition defaults; it should be the theme of the Parser that
	// compiled the classes. Nil means DefaultTheme.
	Theme *Theme
	// Viewport is the window size in logical pixels: vw and vh units and
	// the sm: to 2xl: variants.
	Viewport gfx.Size
	// Dark selects the dark: variants.
	Dark bool
	// ReducedMotion selects motion-reduce: (and deselects motion-safe:).
	ReducedMotion bool
}

// Input describes one element whose style is computed.
type Input struct {
	// Kind selects the element defaults (ADR-038).
	Kind Kind
	// Parent is the computed style of the parent element, nil for the root.
	Parent *ComputedStyle
	// State is the interaction state of the element, and Group and Peer
	// the combined states of its group ancestors and of its preceding peer
	// siblings (for the group-* and peer-* variants).
	State, Group, Peer StateFlags
	// Classes are the compiled Class strings in order of appearance.
	Classes []*Classes
	// Props are the typed Style props, applied after the classes.
	Props []Op
	// Starting computes the starting style (the starting: variants hold):
	// the style the first transitions of an element that was not rendered
	// before start from (CSS @starting-style).
	Starting bool
}

// env is the state of one computation, passed to every op.
type env struct {
	s        *ComputedStyle
	sc       *scratch
	parent   *ComputedStyle
	theme    *Theme
	viewport gfx.Size
	// fontSize is the element's own font size; it is final once the
	// first pass has run, and the parent's before that.
	fontSize       float32
	parentFontSize float32
}

func (e *env) rootFont() float32 {
	if e.theme != nil {
		return e.theme.RootFontSize
	}
	return 16
}

// lp evaluates a length expression to a length-percentage.
func (e *env) lp(x *lenExpr) LP { return x.eval(e) }

// colorSlot identifies one color property of an element. Slots are
// resolved at the end of the computation, when currentColor is known.
type colorSlot uint8

const (
	slotText colorSlot = iota
	slotBg
	slotBorderTop
	slotBorderRight
	slotBorderBottom
	slotBorderLeft
	slotOutline
	slotDecoration
	slotShadow
	slotInsetShadow
	slotRing
	slotRingOffset
	slotInsetRing
	slotFrom
	slotVia
	slotTo
	slotDropShadow
	slotTextShadow
	slotCaret
	numSlots
)

// scratch holds the intermediate state of a computation: the CSS variables
// of Tailwind (--tw-*) that several classes feed and that are combined
// when the computation ends, so that class order does not matter between
// them (shadow-lg shadow-indigo-500 equals shadow-indigo-500 shadow-lg).
type scratch struct {
	colors [numSlots]colorSpec

	// Shadows and rings.
	shadowUsed  bool // a shadow-*, inset-shadow-*, ring-* or inset-ring-* class is present
	shadow      []Shadow
	insetShadow []Shadow
	ringW       float32
	ringInset   bool
	ringOffW    float32
	insetRingW  float32

	// Line height: leading-* wins over the line height of text-*.
	leading        text.LineHeight
	leadingSet     bool
	sizeLeading    float32
	sizeLeadingSet bool

	// Outline.
	outlineStyle    BorderStyle
	outlineStyleSet bool
	outlineUsed     bool // an outline width class is present

	// Decoration.
	decoLinesSet bool

	// Transition defaults.
	durSet    bool
	timingSet bool

	// Background layer 0 and its gradient.
	bgImage    bgImage
	bgSize     BackgroundSize
	bgPosX     LP
	bgPosY     LP
	bgPosSet   bool
	bgRepeat   BackgroundRepeat
	bgOrigin   BackgroundBox
	bgOriginOK bool
	bgLayers   []BackgroundLayer
	bgLayersOK bool

	fromPos, viaPos, toPos StopPos
}

type bgImage struct {
	kind     uint8 // 0 none/unset, 1 gradient, 2 url, 3 explicit none, 4 decoded image, 5 list
	gradient Gradient
	url      string
	img      image.Image
	list     []bgImage // kind 5: the layers, topmost first (kinds 1 and 2)
}

// Compute resolves the style of an element: the element defaults, then the
// classes in order of appearance, then the typed props (last wins), on top
// of the inherited text properties of the parent (ARCHITECTURE section 10).
func Compute(cx *Context, in Input) *ComputedStyle {
	th := cx.Theme
	if th == nil {
		th = defaultThemeShared()
	}
	s := new(ComputedStyle)
	initStyle(s, in.Parent, th)
	var sc scratch
	e := env{s: s, sc: &sc, parent: in.Parent, theme: th, viewport: cx.Viewport}
	e.parentFontSize = s.Text.FontSize
	e.fontSize = s.Text.FontSize

	vc := VariantContext{
		ViewportWidth: cx.Viewport.W,
		Dark:          cx.Dark,
		ReducedMotion: cx.ReducedMotion,
		Starting:      in.Starting,
		State:         in.State, Group: in.Group, Peer: in.Peer,
	}

	// Pass 0 applies the font-size ops, so that em lengths in every other
	// op resolve against the element's final font size; pass 1 applies
	// everything else. Ops of different properties do not interact, so
	// splitting by pass keeps "last wins" intact.
	var pseudo Pseudo
	for pass := 0; pass < 2; pass++ {
		wantFont := pass == 0
		for _, op := range kindDefaults(in.Kind) {
			e.run(op, wantFont)
		}
		for _, cl := range in.Classes {
			if cl == nil {
				continue
			}
			for i := range cl.Rules {
				r := &cl.Rules[i]
				if r.Cond.Pseudo != 0 {
					pseudo |= r.Cond.Pseudo
					continue
				}
				if r.Cond.Matches(vc) {
					e.run(r.Op, wantFont)
				}
			}
		}
		for _, op := range in.Props {
			e.run(op, wantFont)
		}
		if pass == 0 {
			e.fontSize = s.Text.FontSize
		}
	}
	e.finalize()
	if pseudo&PseudoPlaceholder != 0 {
		if ps := computePseudo(cx, th, s, in.Classes, vc, PseudoPlaceholder); ps != nil {
			t := ps.Text
			s.Placeholder = &t
		}
	}
	if pseudo&PseudoSelection != 0 {
		if ps := computePseudo(cx, th, s, in.Classes, vc, PseudoSelection); ps != nil {
			sel := SelectionColors{}
			if s.Selection != nil {
				sel = *s.Selection // the inherited colors the classes do not set
			}
			if ps.pseudoBg {
				sel.Background, sel.HasBackground = ps.s.BackgroundColor, true
			}
			if ps.pseudoColor {
				sel.Color, sel.HasColor = ps.s.Text.Color, true
			}
			s.Selection = &sel
		}
	}
	return s
}

// pseudoStyle is the style of a pseudo-element computed by computePseudo,
// with which of its colors the rules set.
type pseudoStyle struct {
	s                     *ComputedStyle
	Text                  TextProps
	pseudoBg, pseudoColor bool
}

// computePseudo computes the style of the pseudo-element kind of an
// element whose style is s: the rules of classes that style it and match
// vc, applied over the text properties s gives it (CSS: ::placeholder and
// ::selection inherit from their element). It returns nil when no rule
// matches.
func computePseudo(cx *Context, th *Theme, s *ComputedStyle, classes []*Classes, vc VariantContext, kind Pseudo) *pseudoStyle {
	ps := new(ComputedStyle)
	initStyle(ps, s, th)
	var sc scratch
	e := env{s: ps, sc: &sc, parent: s, theme: th, viewport: cx.Viewport}
	e.parentFontSize = ps.Text.FontSize
	e.fontSize = ps.Text.FontSize
	matched := false
	for pass := 0; pass < 2; pass++ {
		for _, cl := range classes {
			if cl == nil {
				continue
			}
			for i := range cl.Rules {
				r := &cl.Rules[i]
				if r.Cond.Pseudo&kind == 0 {
					continue
				}
				c := r.Cond
				c.Pseudo = 0
				if c.Matches(vc) {
					matched = true
					e.run(r.Op, pass == 0)
				}
			}
		}
		if pass == 0 {
			e.fontSize = ps.Text.FontSize
		}
	}
	if !matched {
		return nil
	}
	out := &pseudoStyle{s: ps, pseudoBg: sc.colors[slotBg].set, pseudoColor: sc.colors[slotText].set}
	e.finalize()
	out.Text = ps.Text
	return out
}

// run applies op when it belongs to the pass.
func (e *env) run(op Op, fontPass bool) {
	if op.font && !fontPass {
		if op.post != nil {
			op.post(e)
		}
		return
	}
	if op.font != fontPass {
		return
	}
	if op.def != nil {
		if fontPass {
			return
		}
		e.s.Deferred = append(e.s.Deferred, Deferred{Phase: op.phase, Apply: op.def, TextColor: op.textColor})
		return
	}
	if op.apply != nil {
		op.apply(e)
	}
}

// initStyle sets the CSS initial values, then the inherited properties.
func initStyle(s *ComputedStyle, parent *ComputedStyle, th *Theme) {
	s.Display = DisplayBlock
	s.Margin = AllSides(DimPx(0)) // Preflight: margin: 0
	s.FlexShrink = 1
	s.Opacity = 1
	s.Transform.ScaleX, s.Transform.ScaleY = 1, 1
	s.Transform.OriginX, s.Transform.OriginY = PctLP(50), PctLP(50)
	for _, b := range []*Border{&s.Border.Top, &s.Border.Right, &s.Border.Bottom, &s.Border.Left} {
		b.Style = BorderSolid // Preflight: border: 0 solid
	}
	s.Outline.Width = 3 // CSS medium
	s.Transition.Timing = CubicBezier{0.25, 0.1, 0.25, 1}
	if parent != nil {
		s.Text = parent.Text
		s.Caret = parent.Caret
		s.Selection = parent.Selection
		s.Visibility = parent.Visibility
		s.Cursor = parent.Cursor
		s.PointerEvents = parent.PointerEvents
		s.UserSelect = parent.UserSelect
		return
	}
	// The root text style (ADR-038): the theme's sans font at the root
	// size, line-height 1.5, tab size 4, black.
	root := th.RootFontSize
	if root <= 0 {
		root = 16
	}
	s.Text = TextProps{
		Color:       gfx.Black,
		FontFamily:  th.Fonts[th.DefaultFont],
		FontSize:    root,
		FontWeight:  400,
		FontStretch: 100,
		LineHeight:  text.LineHeightMultiple(1.5),
		TabSize:     4,
	}
}

// finalize combines the scratch state into the style.
func (e *env) finalize() {
	s, sc, th := e.s, e.sc, e.theme

	// Text color: currentColor in a text-color class keeps the inherited color.
	if spec := sc.colors[slotText]; spec.set {
		s.Text.Color = spec.resolve(s.Text.Color)
		// currentColor (text-current) keeps the inherited color.
		s.TextColorSet = !spec.current || spec.c.A != 1
	}
	cur := s.Text.Color
	pick := func(slot colorSlot, def gfx.Color) gfx.Color {
		if spec := sc.colors[slot]; spec.set {
			return spec.resolve(cur)
		}
		return def
	}

	s.BackgroundColor = pick(slotBg, gfx.Transparent)

	// Borders: the color defaults to currentColor; style none and hidden
	// give a used width of zero.
	sides := [4]*Border{&s.Border.Top, &s.Border.Right, &s.Border.Bottom, &s.Border.Left}
	for i, b := range sides {
		b.Color = pick(slotBorderTop+colorSlot(i), cur)
		if b.Style == BorderNone || b.Style == BorderHidden {
			b.Width = 0
		}
	}

	// Outline.
	s.Outline.Color = pick(slotOutline, cur)
	switch {
	case sc.outlineStyleSet:
		s.Outline.Style = sc.outlineStyle
	case sc.outlineUsed:
		s.Outline.Style = BorderSolid
	}
	if s.Outline.Style == BorderNone || s.Outline.Style == BorderHidden {
		s.Outline.Width = 0
	}

	// Line height: leading-* wins over the line height that text-* sets.
	switch {
	case sc.leadingSet:
		s.Text.LineHeight = sc.leading
	case sc.sizeLeadingSet:
		s.Text.LineHeight = text.LineHeightMultiple(sc.sizeLeading)
	}

	// Caret color: a color class sets it (caret-current is the text
	// color); otherwise it stays auto or the inherited color.
	if spec := sc.colors[slotCaret]; spec.set {
		s.Caret = CaretColor{Set: true, Color: spec.resolve(cur)}
	}

	// Text decoration color.
	switch {
	case sc.colors[slotDecoration].set:
		s.Text.Decoration.Color = sc.colors[slotDecoration].resolve(cur)
	case sc.decoLinesSet:
		s.Text.Decoration.Color = cur
	}

	// Box shadows, rings and filters.
	if sc.shadowUsed {
		s.BoxShadows = composeBoxShadows(sc, cur)
	}
	if spec := sc.colors[slotDropShadow]; spec.set && len(s.Filter.DropShadow) > 0 {
		s.Filter.DropShadow = recolor(s.Filter.DropShadow, spec.resolve(cur))
	}
	s.Filter.DropShadow = resolveCurrentShadows(s.Filter.DropShadow, cur)
	if spec := sc.colors[slotTextShadow]; spec.set && len(s.Text.Shadow) > 0 {
		s.Text.Shadow = recolor(s.Text.Shadow, spec.resolve(cur))
	}
	s.Text.Shadow = resolveCurrentShadows(s.Text.Shadow, cur)

	e.finalizeBackground(cur)

	// Transition defaults (Tailwind: the bare "transition" utilities use
	// the theme's duration and easing unless duration-* or ease-* ran).
	if s.Transition.Properties != 0 {
		if !sc.durSet {
			s.Transition.Duration = th.DefaultTransition.Duration
		}
		if !sc.timingSet {
			s.Transition.Timing = th.DefaultTransition.Timing
		}
	}
}

// recolor returns a copy of the shadow layers with every color replaced.
func recolor(layers []Shadow, c gfx.Color) []Shadow {
	out := make([]Shadow, len(layers))
	for i, l := range layers {
		l.Color = c
		out[i] = l
	}
	return out
}

// composeBoxShadows builds the box-shadow list the way Tailwind composes
// its --tw-* variables: inset shadow, inset ring, ring offset, ring and
// shadow, topmost first.
func composeBoxShadows(sc *scratch, cur gfx.Color) []Shadow {
	var out []Shadow
	add := func(layers []Shadow, slot colorSlot) {
		if len(layers) == 0 {
			return
		}
		layers = resolveCurrentShadows(layers, cur)
		if spec := sc.colors[slot]; spec.set {
			layers = recolor(layers, spec.resolve(cur))
		}
		out = append(out, layers...)
	}
	add(sc.insetShadow, slotInsetShadow)
	if sc.insetRingW > 0 {
		c := cur
		if spec := sc.colors[slotInsetRing]; spec.set {
			c = spec.resolve(cur)
		}
		out = append(out, Shadow{Spread: sc.insetRingW, Color: c, Inset: true})
	}
	if sc.ringW > 0 {
		if sc.ringOffW > 0 {
			c := gfx.White
			if spec := sc.colors[slotRingOffset]; spec.set {
				c = spec.resolve(cur)
			}
			out = append(out, Shadow{Spread: sc.ringOffW, Color: c, Inset: sc.ringInset})
		}
		c := cur
		if spec := sc.colors[slotRing]; spec.set {
			c = spec.resolve(cur)
		}
		out = append(out, Shadow{Spread: float32(sc.ringW + sc.ringOffW), Color: c, Inset: sc.ringInset})
	}
	add(sc.shadow, slotShadow)
	return out
}

// finalizeBackground builds the image layers and the gradient stops.
func (e *env) finalizeBackground(cur gfx.Color) {
	s, sc := e.s, e.sc
	if sc.bgLayersOK {
		s.Backgrounds = sc.bgLayers
		return
	}
	img := sc.bgImage
	items := []bgImage{img}
	switch img.kind {
	case 1, 2, 4:
	case 5:
		items = img.list
	default:
		return
	}
	// Size, position, repeat and origin apply to every layer (a single
	// CSS value repeats for each layer).
	base := BackgroundLayer{
		Size:   sc.bgSize,
		PosX:   sc.bgPosX,
		PosY:   sc.bgPosY,
		Repeat: sc.bgRepeat,
		Origin: BackgroundPaddingBox,
	}
	if sc.bgOriginOK {
		base.Origin = sc.bgOrigin
	}
	s.Backgrounds = make([]BackgroundLayer, 0, len(items))
	for _, it := range items {
		layer := base
		switch it.kind {
		case 2:
			layer.URL = it.url
		case 4:
			layer.Image = it.img
		default:
			g := it.gradient
			if len(g.Stops) == 0 {
				g.Stops = e.gradientStops(cur)
			}
			layer.Gradient = &g
		}
		s.Backgrounds = append(s.Backgrounds, layer)
	}
}

// gradientStops builds the from-*, via-* and to-* stops (Tailwind: from
// and to default to transparent at 0% and 100%, via sits at 50%).
func (e *env) gradientStops(cur gfx.Color) []ColorStop {
	sc := e.sc
	col := func(slot colorSlot) gfx.Color {
		if spec := sc.colors[slot]; spec.set {
			return spec.resolve(cur)
		}
		return gfx.Transparent
	}
	pos := func(p StopPos, def float32) StopPos {
		if p.Set {
			return p
		}
		return StopPos{Set: true, LP: PctLP(def)}
	}
	stops := []ColorStop{{col(slotFrom), pos(sc.fromPos, 0)}}
	if sc.colors[slotVia].set {
		stops = append(stops, ColorStop{col(slotVia), pos(sc.viaPos, 50)})
	}
	return append(stops, ColorStop{col(slotTo), pos(sc.toPos, 100)})
}
