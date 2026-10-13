package core

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Drawing of the debug overlays (debug.go). Everything is built in logical
// pixels with filled rectangles snapped to device pixels, so lines are one
// crisp device pixel wide at any scale; text is JetBrains Mono on a dark
// pill, which reads on light and dark applications alike.

// Chrome DevTools' box model colors (the layout overlay).
var (
	colContent = gfx.RGBA8(111, 168, 220, 168)
	colPadding = gfx.RGBA8(147, 196, 125, 140)
	colBorder  = gfx.RGBA8(255, 229, 153, 168)
	colMargin  = gfx.RGBA8(246, 178, 107, 168)

	colBoxLine   = gfx.RGBA8(37, 99, 235, 120) // outline of every border box
	colHoverLine = gfx.RGBA8(37, 99, 235, 255)
	colFlash     = gfx.RGBA8(34, 197, 94, 255)
	colPill      = gfx.RGBA8(15, 23, 42, 235)
	colPillText  = gfx.RGBA8(241, 245, 249, 255)
	colPillEdge  = gfx.RGBA8(255, 255, 255, 70) // keeps a pill apart from a dark app
	colSemantics = gfx.RGBA8(217, 70, 239, 255)
	colBudget    = gfx.RGBA8(148, 163, 184, 150)
	colIdle      = gfx.RGBA8(100, 116, 139, 200)
	colFast      = gfx.RGBA8(34, 197, 94, 255)
	colSlow      = gfx.RGBA8(245, 158, 11, 255)
	colJank      = gfx.RGBA8(239, 68, 68, 255)

	// layerColors are the borders of the layers by kind (layerKindNames).
	layerColors = [...]gfx.Color{
		gfx.RGBA8(59, 130, 246, 255), // root
		gfx.RGBA8(168, 85, 247, 255), // scroll
		gfx.RGBA8(236, 72, 153, 255), // deferred
		gfx.RGBA8(20, 184, 166, 255), // scrollbars
	}
)

// Label metrics (logical pixels).
const (
	labelSize    = 11
	labelLine    = 14
	labelPadX    = 5
	labelPadY    = 2
	labelAccent  = 3
	maxLabels    = 250
	maxLabelRune = 44

	// otherBoxAlpha scales the box model colors of the boxes the pointer
	// is not over.
	otherBoxAlpha = 0.28
	// flashAlpha is the opacity of a flash that just happened.
	flashAlpha = 0.3

	meterW        = 136
	meterBars     = 60
	meterBarsH    = 24
	meterBudget   = 16667 * time.Microsecond
	meterMaxScale = 2 * meterBudget
)

// labelKey identifies a shaped label.
type labelKey struct {
	s    string
	bold bool
}

// ovLabel is a label waiting to be placed.
type ovLabel struct {
	text   string
	box    gfx.Rect // the thing it names: the label sits at its top-left corner
	accent gfx.Color
	// above asks to sit above the box (when there is room) instead of
	// inside its top-left corner; top (the hovered element's label)
	// also places the label first and draws it last.
	above, top bool
}

// overlayScratch is the label storage reused between frames.
type overlayScratch struct {
	labels []ovLabel
	placed []gfx.Rect
}

// overlay builds one overlay list.
type overlay struct {
	t      *Tree
	b      *gfx.Builder
	scale  float32
	win    gfx.Rect
	labels []ovLabel
	placed []gfx.Rect
}

// buildOverlay builds the overlay list of the frame composed last.
func (t *Tree) buildOverlay(bounds image.Rectangle) *gfx.DisplayList {
	d := &t.dbg
	if d.b == nil {
		d.b = gfx.NewBuilder()
	}
	o := overlay{
		t: t, b: d.b, scale: t.lay.cur.scale,
		win:    gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H},
		labels: d.scr.labels[:0], placed: d.scr.placed[:0],
	}
	if d.flags&DebugLayout != 0 {
		o.layout()
	}
	if d.flags&DebugPaintFlash != 0 {
		o.flashes()
	}
	if d.flags&DebugLayers != 0 {
		o.layers()
	}
	if d.flags&DebugSemantics != 0 {
		o.semantics()
	}
	if d.flags&DebugFPS != 0 {
		o.placed = append(o.placed, o.meterRect()) // labels keep clear of it
	}
	o.placeLabels()
	if d.flags&DebugFPS != 0 {
		o.meter()
	}
	clear(o.labels)
	d.scr.labels, d.scr.placed = o.labels[:0], o.placed[:0]
	if len(d.labels) > 512 {
		clear(d.labels) // bound the shaped labels (the meter's numbers change)
	}
	return o.b.Finish()
}

// ---------------------------------------------------------------------
// Primitives.

// snap rounds a coordinate to a device pixel.
func (o *overlay) snap(v float32) float32 {
	d := float32(v * o.scale)
	return float32(math.Floor(float64(d)+0.5)) / o.scale
}

func (o *overlay) snapRect(r gfx.Rect) gfx.Rect {
	return gfx.Rect{X0: o.snap(r.X0), Y0: o.snap(r.Y0), X1: o.snap(r.X1), Y1: o.snap(r.Y1)}
}

// fill fills r (clipped to clip) with c.
func (o *overlay) fill(r, clip gfx.Rect, c gfx.Color) {
	r = r.Intersect(clip)
	if r.Empty() || c.A <= 0 {
		return
	}
	o.b.FillRect(r, c)
}

// ring fills the part of outer that is outside inner.
func (o *overlay) ring(outer, inner, clip gfx.Rect, c gfx.Color) {
	inner = inner.Intersect(outer)
	if inner.Empty() {
		o.fill(outer, clip, c)
		return
	}
	o.fill(gfx.Rect{X0: outer.X0, Y0: outer.Y0, X1: outer.X1, Y1: inner.Y0}, clip, c)
	o.fill(gfx.Rect{X0: outer.X0, Y0: inner.Y1, X1: outer.X1, Y1: outer.Y1}, clip, c)
	o.fill(gfx.Rect{X0: outer.X0, Y0: inner.Y0, X1: inner.X0, Y1: inner.Y1}, clip, c)
	o.fill(gfx.Rect{X0: inner.X1, Y0: inner.Y0, X1: outer.X1, Y1: inner.Y1}, clip, c)
}

// outline draws a line of width w inside r.
func (o *overlay) outline(r, clip gfx.Rect, w float32, c gfx.Color) {
	w = max(w, 1/o.scale)
	if r.Width() <= 2*w || r.Height() <= 2*w {
		o.fill(r, clip, c)
		return
	}
	o.ring(r, r.Inset(w), clip, c)
}

// ---------------------------------------------------------------------
// Layout.

// hoveredBox returns the box under the primary mouse pointer, other than
// the root: the element it is over, or the nearest ancestor with a box.
func (t *Tree) hoveredBox() *Box {
	for k, ps := range t.in.ptrs {
		if k.kind != platform.PointerMouse || !ps.inside || len(ps.over) == 0 {
			continue
		}
		for el := ps.over[0]; el != nil; el = el.styleParent() {
			if el.kind == elemPrimitive && el.box != nil && t.ownBox(el, false) && t.inTree(el.box) {
				if el.box == t.rootBox {
					return nil
				}
				return el.box
			}
		}
	}
	return nil
}

// layout draws the box model of every box, and emphasizes the hovered one.
func (o *overlay) layout() {
	t := o.t
	hov := t.hoveredBox()
	var hovRect, hovClip gfx.Rect
	var walk func(bx *Box, clip gfx.Rect)
	walk = func(bx *Box, clip gfx.Rect) {
		if bx.cs == nil || bx.cs.Display == style.DisplayNone || !t.inTree(bx) {
			return
		}
		r := t.boxWindowRect(bx)
		if bx != t.rootBox {
			if bx == hov {
				hovRect, hovClip = r, clip
			} else {
				o.boxModel(bx, r, clip, otherBoxAlpha)
			}
		}
		if cs := paintStyle(bx); cs != nil && clipsOverflow(cs) {
			l := bx.ld.Layout()
			clip = clip.Intersect(r.InsetEdges(l.Border.Left, l.Border.Top, l.Border.Right, l.Border.Bottom))
			if clip.Empty() {
				return
			}
		}
		for _, k := range bx.kids {
			walk(k, clip)
		}
	}
	walk(t.rootBox, o.win)
	if hov != nil && !hovRect.Empty() {
		o.boxModel(hov, hovRect, hovClip, 1)
		o.labels = append(o.labels, ovLabel{text: hoverLabel(hov, hovRect), box: hovRect, accent: colHoverLine, top: true})
	}
}

// boxModel draws the margin, border, padding and content areas of bx,
// whose border box is r, with the colors scaled by k.
func (o *overlay) boxModel(bx *Box, r, clip gfx.Rect, k float32) {
	l := bx.ld.Layout()
	m, bd, p := l.Margin, l.Border, l.Padding
	border := o.snapRect(r)
	margin := o.snapRect(r.InsetEdges(-m.Left, -m.Top, -m.Right, -m.Bottom))
	if !margin.Intersects(clip) {
		return
	}
	padding := o.snapRect(r.InsetEdges(bd.Left, bd.Top, bd.Right, bd.Bottom))
	content := o.snapRect(r.InsetEdges(bd.Left+p.Left, bd.Top+p.Top, bd.Right+p.Right, bd.Bottom+p.Bottom))
	o.ring(margin, border, clip, colMargin.MulAlpha(k))
	o.ring(border, padding, clip, colBorder.MulAlpha(k))
	o.ring(padding, content, clip, colPadding.MulAlpha(k))
	if k == 1 || len(bx.kids) == 0 {
		// The content of a box with children is covered by theirs.
		o.fill(content, clip, colContent.MulAlpha(k))
	}
	if k == 1 {
		o.outline(border, clip, 1, colHoverLine)
	} else {
		o.outline(border, clip, 1/o.scale, colBoxLine)
	}
}

// hoverLabel describes the hovered box: "div.flex.gap-4  320×48".
func hoverLabel(bx *Box, r gfx.Rect) string {
	name := "anonymous"
	if bx.el != nil {
		name = bx.kind.String()
		if cl := strings.Fields(strings.Join(bx.el.node.d.classes, " ")); len(cl) > 0 {
			if len(cl) > 3 {
				cl = append(cl[:3:3], "…")
			}
			name += "." + strings.Join(cl, ".")
		}
	}
	return fmt.Sprintf("%s  %s×%s", truncateRunes(name, maxLabelRune), fmtPx(r.Width()), fmtPx(r.Height()))
}

// fmtPx formats a length with at most one decimal.
func fmtPx(v float32) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, "0")
	return strings.TrimSuffix(s, ".")
}

// truncateRunes shortens s to n runes, ending with an ellipsis.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// ---------------------------------------------------------------------
// Paint flash.

// flashes draws the regions repainted recently, fading out; the ones that
// finished fading are dropped.
func (o *overlay) flashes() {
	d := &o.t.dbg
	now := o.t.scr.now
	live := d.flashes[:0]
	for _, f := range d.flashes {
		if now-f.at < flashFade {
			live = append(live, f)
		}
	}
	clear(d.flashes[len(live):])
	d.flashes = live
	for _, f := range live {
		a := 1 - float32(float64(now-f.at)/float64(flashFade))
		if a <= 0 {
			continue
		}
		r := o.snapRect(f.r)
		o.fill(r, o.win, colFlash.MulAlpha(flashAlpha*a))
		o.outline(r, o.win, 1, colFlash.MulAlpha(a))
	}
}

// ---------------------------------------------------------------------
// Layers.

// layers draws the borders of the layers with a label each.
func (o *overlay) layers() {
	var drawn []gfx.Rect
	for _, li := range o.t.Layers() {
		kind := 0
		for i, n := range layerKindNames {
			if n == li.Kind {
				kind = i
			}
		}
		var box *Box
		if li.Element != nil {
			box = li.Element.box
		}
		r := li.Bounds
		if kind == int(layerChunk) && box != nil {
			// What a scroll layer shows is its container's port, not
			// just the ink of the commands it recorded.
			r = o.t.boxWindowRect(box)
		}
		r = r.Intersect(o.win)
		if r.Empty() {
			continue
		}
		// Layers with the same rectangle (the root and the viewport's
		// content) are drawn one inside the other.
		r = o.snapRect(r)
		inset := float32(0)
		for _, d := range drawn {
			if d == r {
				inset += 3
			}
		}
		drawn = append(drawn, r)
		col := layerColors[kind]
		w := float32(1)
		if li.Recorded {
			w = 2
		}
		o.outline(r.Inset(inset), o.win, w, col)
		label := li.Kind
		switch {
		case box == o.t.rootBox && kind == int(layerChunk):
			label += " viewport"
		case box != nil && box != o.t.rootBox:
			label += " " + box.kind.String()
		}
		label += fmt.Sprintf(" · %d cmds", li.Commands)
		if li.Recorded {
			label += " · rec"
		}
		o.labels = append(o.labels, ovLabel{text: label, box: r.Inset(inset), accent: col})
	}
}

// ---------------------------------------------------------------------
// Semantics.

// semantics draws the bounds and role "name" of the semantics nodes that
// have a role of their own and are not hidden.
func (o *overlay) semantics() {
	root := o.t.Semantics()
	root.Walk(func(n *semantics.Node) bool {
		if n.Kind != semantics.KindElement {
			return true
		}
		if n.Has(semantics.StateHidden) {
			return false
		}
		if n.Flattened() || n.Bounds.Empty() {
			return true
		}
		r := n.Bounds.Intersect(o.win)
		if r.Empty() {
			return true
		}
		o.outline(o.snapRect(r), o.win, 1, colSemantics.MulAlpha(0.85))
		label := n.Role
		if n.Name != "" {
			label += fmt.Sprintf(" %q", truncateRunes(n.Name, 28))
		}
		o.labels = append(o.labels, ovLabel{text: label, box: r, accent: colSemantics, above: true})
		return true
	})
}

// ---------------------------------------------------------------------
// Labels.

// label returns the shaped label s (JetBrains Mono on one line), cached.
func (o *overlay) label(s string, bold bool) *text.Layout {
	d := &o.t.dbg
	k := labelKey{s, bold}
	if l := d.labels[k]; l != nil {
		return l
	}
	weight := float32(500)
	if bold {
		weight = 700
	}
	l := text.NewLayout(o.t.fonts, &text.Paragraph{
		Text:  s,
		Style: text.ParagraphStyle{WhiteSpace: text.WhiteSpacePre},
		Default: text.TextStyle{
			Families:   []string{"JetBrains Mono", "monospace"},
			Size:       labelSize,
			Weight:     weight,
			LineHeight: text.LineHeightPixels(labelLine),
			Paint:      colPillText,
		},
	})
	if d.labels == nil {
		d.labels = map[labelKey]*text.Layout{}
	}
	d.labels[k] = l
	return l
}

// pill draws a label's background and text with its top-left corner at p,
// and returns its rectangle.
func (o *overlay) pill(p gfx.Point, s string, accent gfx.Color, bold bool) gfx.Rect {
	l := o.label(s, bold)
	w := l.Width() + 2*labelPadX
	if accent.A > 0 {
		w += labelAccent
	}
	r := o.snapRect(gfx.RectXYWH(p.X, p.Y, w, labelLine+2*labelPadY))
	o.b.FillRRect(gfx.NewRRect(r.Outset(1/o.scale), 3+1/o.scale), colPillEdge)
	o.b.FillRRect(gfx.NewRRect(r, 3), colPill)
	tx := r.X0 + labelPadX
	if accent.A > 0 {
		o.b.FillRRect(gfx.NewRRect(gfx.Rect{X0: r.X0 + 2, Y0: r.Y0 + 3, X1: r.X0 + 2 + labelAccent - 1, Y1: r.Y1 - 3}, 1), accent)
		tx += labelAccent
	}
	l.Paint(o.b, gfx.Point{X: tx, Y: r.Y0 + labelPadY})
	return r
}

// placeLabels places and draws the labels: each at the top-left corner of
// the box it names, moved down (or, for the hovered element, above or
// below the box) until it covers no other label, and kept inside the
// window. The hovered element's label is placed first and drawn last.
func (o *overlay) placeLabels() {
	if len(o.labels) == 0 {
		return
	}
	labels := o.labels
	if len(labels) > maxLabels {
		labels = labels[:maxLabels]
	}
	order := make([]int, len(labels))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		switch {
		case labels[a].top == labels[b].top:
			return 0
		case labels[a].top:
			return -1
		}
		return 1
	})
	type placedLabel struct {
		at  gfx.Point
		idx int
	}
	out := make([]placedLabel, 0, len(labels))
	h := float32(labelLine + 2*labelPadY)
	for _, i := range order {
		lb := &labels[i]
		w := o.label(lb.text, false).Width() + 2*labelPadX + labelAccent
		fit := func(p gfx.Point) gfx.Point {
			p.X = max(min(p.X, o.win.X1-w), o.win.X0)
			p.Y = max(min(p.Y, o.win.Y1-h), o.win.Y0)
			return p
		}
		free := func(p gfx.Point) bool {
			r := gfx.RectXYWH(p.X, p.Y, w, h)
			for _, q := range o.placed {
				if r.Intersects(q) {
					return false
				}
			}
			return true
		}
		var cands []gfx.Point
		if lb.above || lb.top {
			cands = append(cands, gfx.Point{X: lb.box.X0, Y: lb.box.Y0 - h}, gfx.Point{X: lb.box.X0, Y: lb.box.Y1})
		}
		for k := range 8 {
			cands = append(cands, gfx.Point{X: lb.box.X0, Y: lb.box.Y0 + float32(k)*h})
		}
		at := fit(cands[0])
		for _, c := range cands {
			c = fit(c)
			if free(c) {
				at = c
				break
			}
		}
		o.placed = append(o.placed, gfx.RectXYWH(at.X, at.Y, w, h))
		out = append(out, placedLabel{at, i})
	}
	// Draw in the order given, the hovered element's last.
	slices.SortStableFunc(out, func(a, b placedLabel) int {
		switch ta, tb := labels[a.idx].top, labels[b.idx].top; {
		case ta == tb:
			return a.idx - b.idx
		case ta:
			return 1
		}
		return -1
	})
	for _, p := range out {
		lb := &labels[p.idx]
		o.pill(p.at, lb.text, lb.accent, false)
	}
}

// ---------------------------------------------------------------------
// FPS meter.

// meterRect is where the frame meter is: the top-right corner.
func (o *overlay) meterRect() gfx.Rect {
	const pad = 8
	x1 := o.win.X1 - pad
	h := float32(5 + labelLine + 5 + meterBarsH + 4 + labelLine + 5)
	return o.snapRect(gfx.Rect{X0: x1 - meterW, Y0: pad, X1: x1, Y1: pad + h})
}

// meter draws the frame meter (in meterRect): frames per second of the
// application's updates (frames that were not idle in the last second), the
// work of the latest ones, a sparkline of the last meterBars frames
// against the 60 Hz budget, and the share of incremental and full frames.
func (o *overlay) meter() {
	d := &o.t.dbg
	n := len(d.frames)
	frames := make([]fpsSample, 0, n)
	if n == fpsHistory {
		frames = append(frames, d.frames[d.next:]...)
		frames = append(frames, d.frames[:d.next]...)
	} else {
		frames = append(frames, d.frames...)
	}
	var fps, nonIdle, inc, full int
	var workSum time.Duration
	var workN int
	if n > 0 {
		last := frames[n-1].at
		for i := n - 1; i >= 0; i-- {
			s := frames[i]
			if s.kind == DebugFrameIdle {
				continue
			}
			nonIdle++
			if s.kind == DebugFrameFull {
				full++
			} else {
				inc++
			}
			if last-s.at < time.Second {
				fps++
			}
			if workN < 10 {
				workSum += s.work
				workN++
			}
		}
	}
	ms := float64(0)
	if workN > 0 {
		ms = float64(workSum) / float64(workN) / float64(time.Millisecond)
	}
	line1 := fmt.Sprintf("%d fps  %.1f ms", fps, ms)
	line3 := "inc – full –"
	if nonIdle > 0 {
		line3 = fmt.Sprintf("inc %d%% full %d%%", (inc*100+nonIdle/2)/nonIdle, (full*100+nonIdle/2)/nonIdle)
	}

	panel := o.meterRect()
	o.b.FillRRect(gfx.NewRRect(panel.Outset(1/o.scale), 5+1/o.scale), colPillEdge)
	o.b.FillRRect(gfx.NewRRect(panel, 5), colPill)
	l1, l3 := o.label(line1, true), o.label(line3, false)
	l1.Paint(o.b, gfx.Point{X: panel.X0 + 8, Y: panel.Y0 + 5})
	sy0 := panel.Y0 + 5 + labelLine + 5
	sx0 := panel.X0 + 8
	// The budget line, then one bar per frame.
	o.fill(gfx.Rect{X0: sx0, Y0: o.snap(sy0 + meterBarsH/2), X1: sx0 + 2*meterBars, Y1: o.snap(sy0+meterBarsH/2) + 1/o.scale}, o.win, colBudget)
	start := max(len(frames)-meterBars, 0)
	for i, s := range frames[start:] {
		x := sx0 + float32(2*(meterBars-(len(frames)-start))) + float32(2*i)
		if s.kind == DebugFrameIdle {
			o.fill(gfx.Rect{X0: x, Y0: sy0 + meterBarsH - 1, X1: x + 1, Y1: sy0 + meterBarsH}, o.win, colIdle)
			continue
		}
		f := float32(float64(s.work) / float64(meterMaxScale))
		bh := max(min(f, 1)*meterBarsH, 1)
		col := colFast
		switch {
		case s.work > meterMaxScale:
			col = colJank
		case s.work > meterBudget:
			col = colSlow
		}
		o.fill(gfx.Rect{X0: x, Y0: sy0 + meterBarsH - bh, X1: x + 1, Y1: sy0 + meterBarsH}, o.win, col)
	}
	l3.Paint(o.b, gfx.Point{X: panel.X0 + 8, Y: sy0 + meterBarsH + 4})
}
