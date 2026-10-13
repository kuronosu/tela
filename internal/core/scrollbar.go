package core

import (
	"cmp"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// Scrollbars are drawn by Tela, over the scroll container's padding box
// (ARCHITECTURE section 13: no native controls).
//
// # Overlay bars (the default)
//
// Thin rounded thumbs over the content, along the right and bottom edges
// of the padding box, for each user-scrollable axis whose content
// overflows. They appear while the container scrolls (by the user or a
// program) and while a mouse or pen is within overlayNear pixels of the
// edge, and fade out over overlayFade after overlayHold without activity
// (a timer, then frames only while fading). Hovered, a bar grows from
// overlayThin to overlayThick and shows a faint track; a thumb is
// draggable and a press on the track pages toward the pointer. Overlay
// bars take no layout space: overflow: auto and overflow: scroll reserve
// nothing (Chromium's overlay scrollbars, macOS, GTK).
//
// # Classic bars
//
// With Config.ClassicScrollbars (tela.App.ClassicScrollbars) or the class
// scrollbar-classic, every scroll container draws always-visible bars in
// a gutter of classicGutter pixels, with a Windows 11 look (a light or
// dark track, a rounded thumb that darkens when hovered and pressed).
// overflow: scroll always reserves the gutter; overflow: auto reserves it
// only while its content overflows in that axis (browsers with classic
// scrollbars; scroll.go's scrollPhase lays out again), and the gutter
// shrinks the content box. scrollbar-overlay turns a container back to
// overlay bars. Classic bars have no arrow buttons.
//
// # Hit-testing and colors
//
// A press on a visible bar hits the scroll container, before anything
// painted under the bar (hit.go): pointer and mouse events fire on the
// container, but the press does not move the focus and does not click.
// Colors follow the dark: environment (system dark mode).

// Scrollbar sizes and timings (logical pixels).
const (
	// overlayThin and overlayThick are the overlay thumb's thickness,
	// idle and hovered (Chromium's overlay thumb is 10 px pressed and
	// thinner idle); overlayMargin separates it from the edge.
	overlayThin   = 6
	overlayThick  = 10
	overlayMargin = 2
	// overlayStrip is the width of the strip along the edge that hits a
	// visible overlay bar; overlayNear the distance from the edge at which
	// a mouse shows the bars.
	overlayStrip = overlayThick + 2*overlayMargin
	overlayNear  = 20
	// overlayEdge is the width of the thumb's contrasting edge.
	overlayEdge = 1
	// overlayHold is how long overlay bars stay after the last activity,
	// and overlayFade how long they take to fade out.
	overlayHold = time.Second
	overlayFade = 200 * time.Millisecond
	// classicGutter is the gutter of classic bars (Chromium's Windows 11
	// Fluent scrollbar is 15 px wide); classicThumb and classicThumbHover
	// are the thumb's thickness, classicInset its distance from the ends
	// of the track.
	classicGutter     = 15
	classicThumb      = 7
	classicThumbHover = 9
	classicInset      = 2
	// minThumb is the shortest thumb.
	minThumb = 24
)

// barPart is a part of a scrollbar under the pointer.
type barPart uint8

const (
	barNone barPart = iota
	barTrack
	barThumb
	barCorner // between two classic bars
)

// barHit is a press target on a scrollbar.
type barHit struct {
	s    *scroller
	axis int // 0: horizontal bar, 1: vertical bar
	part barPart
	at   gfx.Point // the point hit, in the bars' coordinates
}

// barState is the visibility and interaction state of a container's bars.
type barState struct {
	// overlay reports overlay bars (decided by the last paint).
	overlay bool
	// active: an activity was seen at activeAt and the bars have not
	// faded since.
	active   bool
	activeAt time.Duration
	// near and over: a mouse or pen is near the edge of, or over, the bar
	// of each axis. thumb: over the thumb.
	near, over, thumb [2]bool
	// drag is the dragged axis + 1, 0 for none.
	drag int
	// alpha is the overlay bars' opacity.
	alpha float32
}

// held reports whether the bars are kept visible regardless of time.
func (b *barState) held() bool {
	return b.near[0] || b.near[1] || b.over[0] || b.over[1] || b.drag != 0
}

// fadeAt reports when visible overlay bars start fading, if nothing keeps
// them.
func (b *barState) fadeAt() (time.Duration, bool) {
	if !b.overlay || !b.active || b.held() || b.alpha < 1 {
		return 0, false
	}
	return b.activeAt + overlayHold, true
}

// fading reports whether overlay bars are fading at now (frames needed).
func (b *barState) fading(now time.Duration) bool {
	return b.overlay && b.active && !b.held() && now >= b.activeAt+overlayHold
}

// visible reports whether bars of axis i can be seen and hit: classic
// bars always, overlay bars while they are not faded.
func (s *scroller) barVisible(i int) bool {
	return s.geo[i].ok && (!s.bars.overlay || s.bars.alpha > 0)
}

// barActivity records scrolling of s: its overlay bars show and their
// hold restarts.
func (t *Tree) barActivity(s *scroller) {
	b := &s.bars
	if !(s.userX && s.max.X > 0 || s.userY && s.max.Y > 0) {
		return
	}
	b.active, b.activeAt = true, t.scr.now
	t.barsActive(s)
	if b.alpha != 1 {
		b.alpha = 1
		t.markPaint(s.box)
	}
}

// fadeBars updates the opacity of the overlay bars of s at now.
func (t *Tree) fadeBars(s *scroller, now time.Duration) {
	b := &s.bars
	if !b.overlay || !b.active {
		return
	}
	a := float32(1)
	if !b.held() {
		if el := now - b.activeAt - overlayHold; el > 0 {
			a = max(1-float32(float64(el)/float64(overlayFade)), 0)
		}
	}
	if a != b.alpha {
		b.alpha = a
		t.markPaint(s.box)
	}
	if a == 0 {
		b.active = false
	}
}

// tickScroll lets overlay bars whose hold ended at or before now start
// fading (the Host's timer).
func (t *Tree) tickScroll(now time.Duration) {
	t.scr.now = max(t.scr.now, now)
	for _, s := range t.scr.sets[setBars] {
		if at, ok := s.bars.fadeAt(); ok && at <= t.scr.now {
			t.requestRepaint()
			return
		}
	}
}

// hoverBars updates which bars a mouse or pen is near or over (after a
// move, a leave, or content moving under it): overlay bars show near the
// edge and grow over a bar; classic thumbs change color.
func (t *Tree) hoverBars(ps *pointerState) {
	if ps.key.kind == platform.PointerTouch {
		return
	}
	// Only the containers on the pointer's hover chain can be near or
	// over, and only those already near or over can stop being; the
	// others keep no state. In list order.
	st := &t.scr
	buf := append(st.hoverBuf[:0], st.sets[setHover]...)
	for _, el := range ps.over {
		if el.kind == elemPrimitive && el.box != nil && el.box.scr != nil && el.box.scr.at[setAll] != 0 {
			buf = append(buf, el.box.scr)
		}
	}
	slices.SortFunc(buf, func(x, y *scroller) int { return cmp.Compare(x.seq, y.seq) })
	buf = slices.Compact(buf)
	for _, s := range buf {
		var near, over, thumb [2]bool
		if ps.inside && s.box.el != nil && t.hovered(ps, s.box.el) {
			pos := toSpace(ps.pos, nil, s.box.space)
			for i := range 2 {
				g := &s.geo[i]
				if !g.ok {
					continue
				}
				near[i] = s.bars.overlay && g.near.Contains(pos)
				// Near the edge shows the bar at once, so it can be over it.
				over[i] = (s.barVisible(i) || near[i]) && g.hit.Contains(pos)
				thumb[i] = over[i] && g.thumbHit().Contains(pos)
			}
		}
		b := &s.bars
		if near == b.near && over == b.over && thumb == b.thumb {
			continue
		}
		wasHeld := b.held()
		b.near, b.over, b.thumb = near, over, thumb
		if near != [2]bool{} || over != [2]bool{} || thumb != [2]bool{} {
			st.add(setHover, s)
		} else {
			st.remove(setHover, s)
		}
		if b.held() && b.overlay {
			if !b.active || b.alpha != 1 {
				b.active, b.alpha = true, 1
				t.barsActive(s)
			}
		} else if wasHeld && b.active {
			b.activeAt = t.scr.now // the hold starts when the pointer leaves
		}
		t.markPaint(s.box)
	}
	clear(buf)
	st.hoverBuf = buf[:0]
}

// hovered reports whether el is on the hover chain of ps.
func (t *Tree) hovered(ps *pointerState, el *Element) bool {
	for _, x := range ps.over {
		if x == el {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Geometry.

// barGeom is the painted geometry of one bar, in the coordinates of the
// scroll space its container is painted in (layer.go).
type barGeom struct {
	ok bool
	// track is where the bar is drawn (classic: the gutter; overlay: the
	// thumb's lane); hit is what a press hits; near shows overlay bars.
	track, hit, near gfx.Rect
	// thumb is the thumb (empty when the content does not overflow).
	thumb gfx.Rect
	// start and length are the thumb's lane along the axis, thumbLen the
	// thumb's length: the thumb travels length - thumbLen.
	start, length, thumbLen float32
}

// thumbHit returns the thumb grown across the whole hit strip.
func (g *barGeom) thumbHit() gfx.Rect {
	if g.thumb.Empty() {
		return gfx.Rect{}
	}
	r := g.thumb
	if r.Width() < r.Height() { // vertical
		r.X0, r.X1 = g.hit.X0, g.hit.X1
	} else {
		r.Y0, r.Y1 = g.hit.Y0, g.hit.Y1
	}
	return r
}

// barGeometry computes into geo the bars of scroll container bx for this
// paint, from its snapped geometry g.
func (p *painter) barGeometry(bx *Box, g *boxGeom, geo *[2]barGeom) {
	s := bx.scr
	*geo = [2]barGeom{}
	s.bars.overlay = !p.t.classicBars(bx.pcs)
	p.t.barsActive(s)
	pr := g.padding.Rect
	if pr.Empty() {
		return
	}
	l := bx.ld.Layout()
	if !s.bars.overlay {
		gw, gh := p.snapWidth(l.ScrollbarSize.W), p.snapWidth(l.ScrollbarSize.H)
		if gw > 0 {
			tr := gfx.Rect{X0: pr.X1 - gw, Y0: pr.Y0, X1: pr.X1, Y1: max(pr.Y1-gh, pr.Y0)}
			geo[1] = barGeom{ok: true, track: tr, hit: tr,
				start: tr.Y0 + classicInset, length: max(tr.Height()-2*classicInset, 0)}
		}
		if gh > 0 {
			tr := gfx.Rect{X0: pr.X0, Y0: pr.Y1 - gh, X1: max(pr.X1-gw, pr.X0), Y1: pr.Y1}
			geo[0] = barGeom{ok: true, track: tr, hit: tr,
				start: tr.X0 + classicInset, length: max(tr.Width()-2*classicInset, 0)}
		}
	} else {
		show := [2]bool{s.userX && s.max.X > 0, s.userY && s.max.Y > 0}
		// Each bar stops short of the other's strip when both show.
		endY, endX := pr.Y1, pr.X1
		if show[0] && show[1] {
			endY, endX = pr.Y1-overlayStrip, pr.X1-overlayStrip
		}
		if show[1] {
			hit := gfx.Rect{X0: pr.X1 - overlayStrip, Y0: pr.Y0, X1: pr.X1, Y1: endY}
			th := float32(overlayThin)
			if s.bars.over[1] || s.bars.drag == 2 {
				th = overlayThick
			}
			tr := gfx.Rect{X0: pr.X1 - overlayMargin - th, Y0: pr.Y0 + overlayMargin, X1: pr.X1 - overlayMargin, Y1: endY - overlayMargin}
			geo[1] = barGeom{ok: true, track: tr, hit: hit,
				near:  gfx.Rect{X0: pr.X1 - overlayNear, Y0: pr.Y0, X1: pr.X1, Y1: pr.Y1},
				start: tr.Y0, length: max(tr.Height(), 0)}
		}
		if show[0] {
			hit := gfx.Rect{X0: pr.X0, Y0: pr.Y1 - overlayStrip, X1: endX, Y1: pr.Y1}
			th := float32(overlayThin)
			if s.bars.over[0] || s.bars.drag == 1 {
				th = overlayThick
			}
			tr := gfx.Rect{X0: pr.X0 + overlayMargin, Y0: pr.Y1 - overlayMargin - th, X1: endX - overlayMargin, Y1: pr.Y1 - overlayMargin}
			geo[0] = barGeom{ok: true, track: tr, hit: hit,
				near:  gfx.Rect{X0: pr.X0, Y0: pr.Y1 - overlayNear, X1: pr.X1, Y1: pr.Y1},
				start: tr.X0, length: max(tr.Width(), 0)}
		}
	}
	// The thumbs: their length is the visible part of the content, their
	// position the offset's part of the largest one.
	for i := range 2 {
		gb := &geo[i]
		mx := axis(s.max, i)
		if !gb.ok || mx <= 0 || gb.length <= 0 {
			continue
		}
		client := s.client.W
		if i == 1 {
			client = s.client.H
		}
		tl := gb.length
		if total := client + mx; total > 0 {
			tl = float32(float64(gb.length) * float64(client) / float64(total))
		}
		tl = min(max(tl, minThumb), gb.length)
		gb.thumbLen = tl
		f := min(max(axis(s.pos, i)/mx, 0), 1)
		a0 := p.snap(gb.start + float32(float64(f)*float64(gb.length-tl)))
		a1 := p.snap(a0 + tl)
		tr := gb.track
		if !s.bars.overlay {
			// Centered across the gutter.
			th := float32(classicThumb)
			if s.bars.thumb[i] || s.bars.drag == i+1 {
				th = classicThumbHover
			}
			if i == 1 {
				c := (tr.X0 + tr.X1) / 2
				tr.X0, tr.X1 = c-th/2, c+th/2
			} else {
				c := (tr.Y0 + tr.Y1) / 2
				tr.Y0, tr.Y1 = c-th/2, c+th/2
			}
		}
		if i == 1 {
			gb.thumb = gfx.Rect{X0: tr.X0, Y0: a0, X1: tr.X1, Y1: a1}
		} else {
			gb.thumb = gfx.Rect{X0: a0, Y0: tr.Y0, X1: a1, Y1: tr.Y1}
		}
	}
}

// ---------------------------------------------------------------------
// Painting.

// Scrollbar colors (premultiplied alpha is applied by the rasterizer).
var (
	// Classic, after Windows 11's light and dark scrollbars.
	classicTrackLight  = gfx.RGB8(0xf3, 0xf3, 0xf3)
	classicThumbLight  = gfx.RGB8(0xa3, 0xa3, 0xa3)
	classicHoverLight  = gfx.RGB8(0x86, 0x86, 0x86)
	classicActiveLight = gfx.RGB8(0x6e, 0x6e, 0x6e)
	classicTrackDark   = gfx.RGB8(0x2b, 0x2b, 0x2b)
	classicThumbDark   = gfx.RGB8(0x6b, 0x6b, 0x6b)
	classicHoverDark   = gfx.RGB8(0x8a, 0x8a, 0x8a)
	classicActiveDark  = gfx.RGB8(0xa6, 0xa6, 0xa6)
)

// overlayColor returns an overlay bar color: black on light, white on
// dark, with alpha a.
func overlayColor(dark bool, a float32) gfx.Color {
	if dark {
		return gfx.Color{R: 1, G: 1, B: 1, A: a}
	}
	return gfx.Color{A: a}
}

// paintScrollbars paints the bars of scroll container bx above its
// content: in a layer of their own (layer.go) when they can change
// without a full recording (the content scrolls, or classic bars react to
// the pointer).
func (p *painter) paintScrollbars(bx *Box) {
	s := bx.scr
	if s == nil || bx.pcs.Visibility != style.VisibilityVisible {
		return
	}
	if s.chunked || p.t.classicBars(bx.pcs) {
		p.openLayer(layerBars, bx, p.space)
		p.drawBars(bx)
		p.closeLayer()
		return
	}
	p.drawBars(bx)
}

// drawBars paints the bars of scroll container bx, and records their
// geometry on screen for hit-testing (scroller.geo).
func (p *painter) drawBars(bx *Box) {
	s := bx.scr
	if bx.pcs.Visibility != style.VisibilityVisible {
		s.geo = [2]barGeom{}
		return
	}
	g := p.geom(bx)
	p.barGeometry(bx, &g, &s.geo)
	geo := &s.geo
	if !geo[0].ok && !geo[1].ok {
		return
	}
	if s.bars.overlay && s.bars.alpha <= 0 {
		return
	}
	dark := p.t.cx.Dark
	// The bars stay inside the rounded padding box.
	rounded := !g.padding.IsRect()
	if rounded {
		p.b.PushClipRRect(g.padding)
	}
	for i := range 2 {
		gb := &geo[i]
		if !gb.ok {
			continue
		}
		pressed, hover := s.bars.drag == i+1, s.bars.thumb[i]
		if s.bars.overlay {
			a := s.bars.alpha
			if s.bars.over[i] || pressed {
				p.pill(gb.track, overlayColor(dark, 0.08*a))
			}
			alpha := float32(0.4)
			switch {
			case pressed:
				alpha = 0.7
			case hover:
				alpha = 0.55
			}
			if dark {
				alpha += 0.05
			}
			// A contrasting edge keeps the thumb visible on content of
			// its own color (Chromium's overlay thumb has a stroke too).
			p.pill(gb.thumb.Outset(overlayEdge), overlayColor(!dark, 0.5*a))
			p.pill(gb.thumb, overlayColor(dark, alpha*a))
			continue
		}
		track, thumb := classicTrackLight, classicThumbLight
		switch {
		case dark && pressed:
			track, thumb = classicTrackDark, classicActiveDark
		case dark && hover:
			track, thumb = classicTrackDark, classicHoverDark
		case dark:
			track, thumb = classicTrackDark, classicThumbDark
		case pressed:
			thumb = classicActiveLight
		case hover:
			thumb = classicHoverLight
		}
		p.b.FillRect(gb.track, track)
		p.pill(gb.thumb, thumb)
	}
	if !s.bars.overlay && geo[0].ok && geo[1].ok {
		// The corner between the two gutters.
		c := gfx.Rect{X0: geo[1].track.X0, Y0: geo[0].track.Y0, X1: geo[1].track.X1, Y1: geo[0].track.Y1}
		col := classicTrackLight
		if dark {
			col = classicTrackDark
		}
		p.b.FillRect(c, col)
	}
	if rounded {
		p.b.PopClip()
	}
}

// pill fills r with fully rounded ends.
func (p *painter) pill(r gfx.Rect, c gfx.Color) {
	if r.Empty() || c.A <= 0 {
		return
	}
	rad := min(r.Width(), r.Height()) / 2
	p.b.FillRRect(gfx.RRect{Rect: r, Radii: [4]gfx.Point{{X: rad, Y: rad}, {X: rad, Y: rad}, {X: rad, Y: rad}, {X: rad, Y: rad}}}, c)
}

// ---------------------------------------------------------------------
// Hit-testing and interaction.

// barAt returns the part of the bars of s under p (in the scroll space s's
// box is painted in, as painted last), for hit-testing.
func (s *scroller) barAt(p gfx.Point) (int, barPart) {
	cs := s.box.pcs
	if cs == nil || cs.PointerEvents == style.PointerEventsNone || cs.Visibility != style.VisibilityVisible {
		return 0, barNone
	}
	for _, i := range [2]int{1, 0} {
		g := &s.geo[i]
		if !s.barVisible(i) || !g.hit.Contains(p) {
			continue
		}
		if g.thumbHit().Contains(p) {
			return i, barThumb
		}
		return i, barTrack
	}
	if !s.bars.overlay && s.geo[0].ok && s.geo[1].ok {
		c := gfx.Rect{X0: s.geo[1].track.X0, Y0: s.geo[0].track.Y0, X1: s.geo[1].track.X1, Y1: s.geo[0].track.Y1}
		if c.Contains(p) {
			return 0, barCorner
		}
	}
	return 0, barNone
}

// barPress handles a primary press on a scrollbar: a thumb press returns
// the eager drag recognizer that moves it, a track press pages toward the
// pointer at once.
func (t *Tree) barPress(hit barHit) gesture.Recognizer {
	s, i := hit.s, hit.axis
	g := s.geo[i]
	switch hit.part {
	case barThumb:
		var start float32
		travel := g.length - g.thumbLen
		move := func(u gesture.DragUpdate) {
			if s.box.scr != s || travel <= 0 {
				return
			}
			off := u.Offset.X
			if i == 1 {
				off = u.Offset.Y
			}
			p := s.pos
			setAxis(&p, i, start+float32(float64(off)*float64(axis(s.max, i))/float64(travel)))
			t.scrollDirect(s, p)
		}
		return &gesture.Drag{
			Eager: true,
			OnStart: func(gesture.DragUpdate) {
				s.stop()
				start = axis(s.pos, i)
				s.bars.drag = i + 1
				t.markPaint(s.box)
			},
			OnUpdate: move,
			OnEnd: func(u gesture.DragUpdate) {
				move(u)
				if s.box.scr == s {
					s.bars.drag = 0
					s.bars.activeAt = t.scr.now
					t.markPaint(s.box)
				}
			},
		}
	case barTrack:
		// Page toward the pointer (Chromium; macOS and GTK can jump
		// instead).
		at := hit.at.X
		if i == 1 {
			at = hit.at.Y
		}
		c := s.client.W
		if i == 1 {
			c = s.client.H
		}
		dir := float32(1)
		if mid := (axis(g.thumb.Origin(), i) + axis(gfx.Point{X: g.thumb.X1, Y: g.thumb.Y1}, i)) / 2; at < mid {
			dir = -1
		}
		var d gfx.Point
		setAxis(&d, i, dir*max(float32(float64(c)*pageFraction), 1))
		t.scrollUser(s, d)
	}
	return nil
}
