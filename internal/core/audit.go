package core

import (
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// What telatest's audit reads beyond the semantics tree (ADR-306, ADR-312):
// the laid-out text of the screen with where it is clipped and the colors
// it is seen in. Everything is in window coordinates as the last frame
// painted it. Call these on the UI thread after a settled frame and after
// Semantics, which gives the elements their node IDs.

// AuditRun is a run of one paragraph's text in one color.
type AuditRun struct {
	// Text is the run, trimmed.
	Text string
	// Fg and Bg are the colors it is seen in: the text over what is
	// behind it, as the frame composites them (a faded group, a
	// translucent background...). Known is false when what is behind the
	// text cannot be told from the styles (an image, a gradient, a
	// backdrop blur, a filter, a blend mode, or an out-of-flow box
	// with no opaque background) or the text is not a plain color.
	Fg, Bg gfx.Color
	Known  bool
	// Size and Weight are its font size and weight; Opacity is the product
	// of the opacities of the boxes around it (1 when none fades it).
	Size, Weight, Opacity float32
}

// AuditText is the text of one paragraph box.
type AuditText struct {
	// Node is the semantics node of the nearest element around the text
	// that has one in the last semantics tree, 0 for none.
	Node semantics.NodeID
	// Kind and Classes are the primitive that holds the text ("div") and
	// its classes; Src is where the app made it ("" when the tree is not
	// instrumented).
	Kind, Classes, Src string
	// Text is the paragraph's text.
	Text string
	// Lines are where the text is on screen, one rectangle per line: the
	// advance of the line, and the height of its glyphs (not of its line
	// box, which a tight container may cut without cutting any ink).
	Lines []gfx.Rect
	// Clip is the part of the window the text is visible in: the
	// intersection of the overflow clips of the boxes around it and the
	// window, unbounded (±Inf) along an axis where a scroll container
	// reaches the text.
	Clip gfx.Rect
	// Visible is Clip with the scroll containers' clips too: the part of
	// the window the text is in sight in at the current scroll offsets.
	// Scroller numbers the nearest scroll container around the text, 0 for
	// none; texts of one scroller share its content, in sight or not.
	Visible  gfx.Rect
	Scroller int
	// ClipBy describes the box whose clip is the tightest on each side
	// (left, top, right, bottom): what clips the text there. Its Kind is
	// empty where the window does, or where nothing does.
	ClipBy [4]AuditBox
	// Truncates reports a text-overflow or line-clamp that cuts it on
	// purpose, or a line already shown with an ellipsis.
	Truncates bool
	// Flow reports a text in normal flow: no box around it is absolutely
	// positioned, fixed, sticky or in a portal, so another text over it is
	// not an overlay. Layer numbers the portal it is in (0: the window).
	Flow  bool
	Layer int
	// Index and Outer number the paragraphs of the result; Outer is the
	// paragraph that holds this one as an atomic inline (a button in a
	// sentence), -1 for none.
	Index, Outer int
	// Runs are its colors.
	Runs []AuditRun
}

// AuditBox names a box that clips: its kind, classes and source line.
type AuditBox struct{ Kind, Classes, Src string }

// auditInf is how far an unbounded clip reaches.
const auditInf = float32(math.MaxFloat32 / 4)

// AuditTexts returns the text of every displayed paragraph of the tree
// that is not under a transform or fully transparent, in tree order. It
// is for tools; an app does not call it.
func (t *Tree) AuditTexts() []AuditText {
	if t.rootBox == nil || t.rootBox.cs == nil {
		return nil
	}
	var out []AuditText
	byBox := map[*Box]int{}
	layers := map[*Box]int{}
	scrollers := map[*Box]int{}
	var walk func(b *Box, skip bool)
	walk = func(b *Box, skip bool) {
		if b.cs == nil || b.ls.Display == layout.DisplayNone {
			return
		}
		if b != t.rootBox {
			if cs := paintStyle(b); cs != nil && (!cs.Transform.IsIdentity() || cs.Opacity == 0) {
				return // moved or invisible: not what a rule can judge from layout
			}
		}
		if b.portal {
			layers[b] = len(layers) + 1
		}
		if b.mode == modeParagraph && b.para != nil && b.para.lay != nil && !b.para.empty && !skip {
			if at, ok := t.auditText(b, layers, scrollers); ok {
				at.Index = len(out)
				at.Outer = -1
				for p := b.parent; p != nil; p = p.parent {
					if i, ok := byBox[p]; ok {
						at.Outer = i
						break
					}
				}
				byBox[b] = at.Index
				out = append(out, at)
			}
		}
		for _, k := range b.kids {
			walk(k, skip)
		}
	}
	walk(t.rootBox, false)
	return out
}

// auditText describes the paragraph box b.
func (t *Tree) auditText(b *Box, layers, scrollers map[*Box]int) (AuditText, bool) {
	p := b.para
	cs := paintStyle(b)
	if cs == nil || cs.Visibility != style.VisibilityVisible || b.owner == nil || !t.inTree(b) {
		return AuditText{}, false
	}
	if strings.TrimSpace(p.text) == "" {
		return AuditText{}, false // only an inline block: its own texts are judged
	}
	at := AuditText{Text: p.text, Flow: true}
	owner := b.owner
	if owner.kind == elemPrimitive && owner.box != nil {
		at.Kind = owner.box.kind.String()
		at.Classes = strings.Join(owner.node.d.classes, " ")
	}
	if t.ins != nil {
		at.Src = t.ins.appSite(owner).Rel()
	}
	for e := owner; e != nil; e = e.parent {
		if e.semID != 0 && t.sem.byID[semantics.NodeID(e.semID)] == e {
			at.Node = semantics.NodeID(e.semID)
			break
		}
	}
	// Flow and layer, and the transforms that would move the text.
	for a := b; a != nil; a = a.parent {
		acs := paintStyle(a)
		if acs == nil {
			continue
		}
		if a != t.rootBox && !acs.Transform.IsIdentity() {
			return AuditText{}, false
		}
		switch acs.Position {
		case style.PositionAbsolute, style.PositionFixed, style.PositionSticky:
			at.Flow = false
		}
		if a.portal || a.flt != nil {
			at.Flow = false
			if l, ok := layers[a]; ok && at.Layer == 0 {
				at.Layer = l
			}
		}
	}
	at.Truncates = cs.TextOverflow != 0 || cs.LineClamp > 0

	// The lines, broken for the box's final width as painting does (a
	// paragraph that was last measured at another width has the lines of
	// that one).
	l := b.ld.Layout()
	p.prepare(t, max(l.Size.W-l.Border.Left-l.Border.Right-l.Padding.Left-l.Padding.Right, 0))
	o := t.paraContentOrigin(b, false)
	lay := p.lay
	em := p.em
	if em <= 0 {
		em = 16
	}
	for i := range lay.NumLines() {
		ln := lay.Line(i)
		if ln.Width <= 0 {
			continue
		}
		if ln.Ellipsis {
			at.Truncates = true
		}
		at.Lines = append(at.Lines, gfx.Rect{
			X0: o.X + ln.X, X1: o.X + ln.X + ln.Width,
			Y0: o.Y + ln.Baseline - 0.8*em, Y1: o.Y + ln.Baseline + 0.2*em,
		})
	}
	if len(at.Lines) == 0 {
		return AuditText{}, false
	}
	at.Clip, at.Visible, at.ClipBy = t.auditClip(b)
	for c := b; c != nil; c = containingBlock(c) {
		if cs := paintStyle(c); cs != nil && c != t.rootBox && (isScrollOverflow(cs.OverflowX) || isScrollOverflow(cs.OverflowY)) {
			if _, ok := scrollers[c]; !ok {
				scrollers[c] = len(scrollers) + 1
			}
			at.Scroller = scrollers[c]
			break
		}
	}
	at.Runs = t.auditRuns(b)
	return at, true
}

// auditBoxOf describes box c for a message.
func (t *Tree) auditBoxOf(c *Box) AuditBox {
	var ab AuditBox
	if c.el != nil && c.el.kind == elemPrimitive {
		ab.Kind = c.kind.String()
		ab.Classes = strings.Join(c.el.node.d.classes, " ")
		if t.ins != nil {
			ab.Src = t.ins.appSite(c.el).Rel()
		}
	}
	return ab
}

// auditClip returns the part of the window box b's content is visible in:
// along each axis the overflow clips of b and the boxes that contain it
// (the containing-block chain a painter clips by), until a scroll
// container reaches it (its content is a scroll away, whatever lies
// outside), then the window. by describes the box that gives each side.
func (t *Tree) auditClip(b *Box) (clip, visible gfx.Rect, by [4]AuditBox) {
	clip = gfx.Rect{X0: -auditInf, Y0: -auditInf, X1: auditInf, Y1: auditInf}
	visible = clip
	var absX, absY bool
	var who [4]*Box // left, top, right, bottom
	for c := b; c != nil; {
		cs := paintStyle(c)
		if cs != nil && c != t.rootBox {
			l := c.ld.Layout()
			org := t.semOrigin(c)
			pad := gfx.Rect{X0: org.X + l.Border.Left, Y0: org.Y + l.Border.Top,
				X1: org.X + l.Size.W - l.Border.Right, Y1: org.Y + l.Size.H - l.Border.Bottom}
			if !absX {
				switch cs.OverflowX {
				case style.OverflowHidden, style.OverflowClip:
					if pad.X0 > clip.X0 {
						clip.X0, who[0] = pad.X0, c
					}
					if pad.X1 < clip.X1 {
						clip.X1, who[2] = pad.X1, c
					}
				case style.OverflowScroll, style.OverflowAuto:
					absX = true
				}
			}
			if !absY {
				switch cs.OverflowY {
				case style.OverflowHidden, style.OverflowClip:
					if pad.Y0 > clip.Y0 {
						clip.Y0, who[1] = pad.Y0, c
					}
					if pad.Y1 < clip.Y1 {
						clip.Y1, who[3] = pad.Y1, c
					}
				case style.OverflowScroll, style.OverflowAuto:
					absY = true
				}
			}
		}
		if absX && absY {
			break
		}
		cb := containingBlock(c)
		if cs != nil && cs.Position == style.PositionFixed && cb == t.rootBox {
			break
		}
		c = cb
	}
	if !absX {
		if clip.X0 < 0 {
			clip.X0, who[0] = 0, nil
		}
		if clip.X1 > t.viewport.W {
			clip.X1, who[2] = t.viewport.W, nil
		}
	}
	if !absY {
		if clip.Y0 < 0 {
			clip.Y0, who[1] = 0, nil
		}
		if clip.Y1 > t.viewport.H {
			clip.Y1, who[3] = t.viewport.H, nil
		}
	}
	for i, w := range who {
		if w != nil {
			by[i] = t.auditBoxOf(w)
		}
	}
	// What is in sight: every overflow clip around the text, scroll
	// containers included, and the window.
	for c := b; c != nil; c = containingBlock(c) {
		cs := paintStyle(c)
		if cs == nil || c == t.rootBox {
			continue
		}
		l := c.ld.Layout()
		org := t.semOrigin(c)
		pad := gfx.Rect{X0: org.X + l.Border.Left, Y0: org.Y + l.Border.Top,
			X1: org.X + l.Size.W - l.Border.Right, Y1: org.Y + l.Size.H - l.Border.Bottom}
		if cs.OverflowX != style.OverflowVisible {
			visible.X0, visible.X1 = max(visible.X0, pad.X0), min(visible.X1, pad.X1)
		}
		if cs.OverflowY != style.OverflowVisible {
			visible.Y0, visible.Y1 = max(visible.Y0, pad.Y0), min(visible.Y1, pad.Y1)
		}
	}
	visible = visible.Intersect(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H})
	return clip, visible, by
}

// isScrollOverflow reports whether an overflow value scrolls.
func isScrollOverflow(o style.Overflow) bool {
	return o == style.OverflowScroll || o == style.OverflowAuto
}

// auditGroup is a group of the compositor of auditRuns: the premultiplied color
// accumulated in it, its opacity and how much of it is of unknown colors.
type auditGroup struct {
	r, g, b, a float32 // premultiplied
	unk        float32 // the share of the pixel that is unknown content, 0..1
	opacity    float32
}

// over paints color c (straight alpha) of coverage a over the group.
func (g *auditGroup) over(c gfx.Color, a float32) {
	a *= c.A
	g.r, g.g, g.b = c.R*a+g.r*(1-a), c.G*a+g.g*(1-a), c.B*a+g.b*(1-a)
	g.a = a + g.a*(1-a)
	g.unk *= 1 - a
}

// unknown paints something whose color is not known: it covers a.
func (g *auditGroup) unknown(a float32) {
	g.unk = a + g.unk*(1-a)
	g.a = a + g.a*(1-a)
	g.r, g.g, g.b = g.r*(1-a), g.g*(1-a), g.b*(1-a) // the unknown share carries no color
}

// auditRuns composites, for each color of box b's text, the text and what
// is behind it. It walks the boxes around the text from the window down,
// painting each one's background into a group (a fading box, opacity
// below 1, opens one), as the frame does.
func (t *Tree) auditRuns(b *Box) []AuditRun {
	p := b.para
	var chain []*Box
	for a := b; a != nil; a = a.parent {
		chain = append(chain, a)
	}
	// Run the compositor twice: with the text color over the innermost
	// group, and without, which is what is behind the text.
	type result struct {
		c   gfx.Color
		unk float32
	}
	compose := func(text *gfx.Color) result {
		groups := []auditGroup{{r: 1, g: 1, b: 1, a: 1, opacity: 1}} // the white canvas
		for i := len(chain) - 1; i >= 0; i-- {
			a := chain[i]
			cs := paintStyle(a)
			if cs == nil {
				continue
			}
			if cs.Opacity < 1 {
				groups = append(groups, auditGroup{opacity: cs.Opacity})
			}
			g := &groups[len(groups)-1]
			switch cs.Position {
			case style.PositionAbsolute, style.PositionFixed:
				if a != t.rootBox {
					// What is behind an out-of-flow box is anything.
					*g = auditGroup{opacity: g.opacity}
					g.unknown(1)
				}
			}
			if a.portal || a.flt != nil {
				*g = auditGroup{opacity: g.opacity}
				g.unknown(1)
			}
			if cs.BackdropFilter.Set != 0 || cs.Filter.Set != 0 || cs.MixBlendMode != gfx.SrcOver {
				g.unknown(1)
			}
			if bg := cs.BackgroundColor.Clamp(); bg.A > 0 {
				g.over(bg, 1)
			}
			if len(cs.Backgrounds) > 0 {
				g.unknown(1)
			}
		}
		if text != nil {
			groups[len(groups)-1].over(*text, 1)
		}
		for len(groups) > 1 {
			c := groups[len(groups)-1]
			groups = groups[:len(groups)-1]
			g := &groups[len(groups)-1]
			o := c.opacity
			g.r, g.g, g.b = c.r*o+g.r*(1-c.a*o), c.g*o+g.g*(1-c.a*o), c.b*o+g.b*(1-c.a*o)
			g.unk = c.unk*o + g.unk*(1-c.a*o)
			g.a = c.a*o + g.a*(1-c.a*o)
		}
		g := groups[0]
		if g.a <= 0 {
			return result{unk: 1}
		}
		return result{c: gfx.Color{R: g.r / g.a, G: g.g / g.a, B: g.b / g.a, A: 1}, unk: g.unk}
	}

	opacity := float32(1)
	for _, a := range chain {
		if cs := paintStyle(a); cs != nil && cs.Opacity < 1 {
			opacity *= cs.Opacity
		}
	}
	bgRes := compose(nil)
	var runs []AuditRun
	add := func(start, end int, st *gfxStyle) {
		if start >= end || end > len(p.text) {
			return
		}
		s := strings.TrimSpace(p.text[start:end])
		if s == "" {
			return
		}
		run := AuditRun{Text: s, Size: st.size, Weight: st.weight, Opacity: opacity}
		paint := st.paint
		if paint == nil {
			paint = gfx.Black
		}
		if col, ok := paint.(gfx.Color); ok && bgRes.unk < 0.02 {
			fg := compose(&col)
			if fg.unk < 0.02 {
				run.Fg, run.Bg, run.Known = fg.c, bgRes.c, true
			}
		}
		runs = append(runs, run)
	}
	def := gfxStyle{size: p.def.Size, weight: p.def.Weight, paint: p.def.Paint}
	if def.size <= 0 {
		def.size = 16
	}
	if def.weight <= 0 {
		def.weight = 400
	}
	pos := 0
	for _, sp := range p.spans {
		add(pos, sp.Start, &def)
		st := gfxStyle{size: sp.Style.Size, weight: sp.Style.Weight, paint: sp.Style.Paint}
		if st.size <= 0 {
			st.size = def.size
		}
		if st.weight <= 0 {
			st.weight = 400
		}
		add(sp.Start, sp.End, &st)
		pos = sp.End
	}
	add(pos, len(p.text), &def)
	return runs
}

// gfxStyle is the part of a text style the contrast rule reads.
type gfxStyle struct {
	size, weight float32
	paint        gfx.Paint
}

// Blur takes the keyboard focus away, as the app losing it to nothing
// would: no element is focused afterwards, so that the next Tab starts
// at the first one. telatest's focus audit uses it.
func (t *Tree) Blur() { t.setFocus(nil, focusScript) }

// AuditFlow says where an element with a semantics node is stacked: Flow
// is false when a box around it is absolutely positioned, fixed, sticky,
// floating or in a portal (it may lie over other content on purpose), and
// Layer numbers the portal it is in, 0 for the window itself.
type AuditFlow struct {
	Flow  bool
	Layer int
}

// AuditFlows returns the AuditFlow of every element that has a node in the
// last semantics tree and a box of its own in the tree.
func (t *Tree) AuditFlows() map[semantics.NodeID]AuditFlow {
	out := map[semantics.NodeID]AuditFlow{}
	if t.rootBox == nil || t.rootBox.cs == nil {
		return out
	}
	layers := map[*Box]int{}
	var walk func(b *Box)
	walk = func(b *Box) {
		if b.portal {
			layers[b] = len(layers) + 1
		}
		if b.el != nil && b.el.semID != 0 && t.sem.byID[semantics.NodeID(b.el.semID)] == b.el {
			f := AuditFlow{Flow: true}
			for a := b; a != nil; a = a.parent {
				if acs := paintStyle(a); acs != nil {
					switch acs.Position {
					case style.PositionAbsolute, style.PositionFixed, style.PositionSticky:
						f.Flow = false
					}
				}
				if a.portal || a.flt != nil {
					f.Flow = false
					if l, ok := layers[a]; ok && f.Layer == 0 {
						f.Layer = l
					}
				}
			}
			out[semantics.NodeID(b.el.semID)] = f
		}
		for _, k := range b.kids {
			walk(k)
		}
	}
	walk(t.rootBox)
	return out
}
