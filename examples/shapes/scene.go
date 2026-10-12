package main

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Scene returns the shapes showcase for a window of w x h logical pixels. It
// is also the temporary contract of `tela snapshot` (a package that has a
// Scene(w, h float32) *gfx.DisplayList can be snapshotted) until the
// Preview() contract arrives with the public API.
//
// The scene is a centered 4x3 grid of cards (3x4 for portrait sizes) on a
// gradient page. Every cell is drawn in a 202x170 design space that is scaled
// to fit the actual cell, so the scene stays balanced at any size. It
// exercises, in this order:
//
//	Shadow cards       shadow-sm .. shadow-2xl, an inset shadow, avatar clip
//	Gradient cards     linear, radial and conic
//	Filters            a blurred, frosted-glass card, drop-shadow on a star
//	                   and a blob
//	Strokes            a dashed border, a curve and a zigzag with round
//	                   caps and joins
//
// The scene has no text (text arrives in phase 2): content is suggested with
// skeleton lines, like a loading state. It uses no clock and no randomness,
// so it always produces the same display list.
func Scene(w, h float32) *gfx.DisplayList {
	b := gfx.NewBuilder()
	drawPage(b, w, h)

	cols, rows := 4, 3
	if w < h {
		cols, rows = 3, 4
	}
	margin := float32(math.Floor(float64(min(w, h) * 0.0625)))
	gap := float32(math.Floor(float64(margin * 0.6)))
	cw := float32(math.Floor(float64((w - 2*margin - float32(cols-1)*gap) / float32(cols))))
	ch := float32(math.Floor(float64((h - 2*margin - float32(rows-1)*gap) / float32(rows))))
	if cw < 1 || ch < 1 {
		return b.Finish()
	}
	gridW := float32(cols)*cw + float32(cols-1)*gap
	gridH := float32(rows)*ch + float32(rows-1)*gap
	x0 := float32(math.Floor(float64((w - gridW) / 2)))
	y0 := float32(math.Floor(float64((h - gridH) / 2)))

	s := min(cw/designW, ch/designH)
	cells := [...]func(*gfx.Builder, float32, float32){
		cardProfile, cardChart, cardPhoto, cardToggles, cardProgress, cardInset,
		cardLinear, cardRadial, cardConic, cardGlass, cardDropShadow, cardStrokes,
	}
	for i, draw := range cells {
		x := x0 + float32(i%cols)*(cw+gap)
		y := y0 + float32(i/cols)*(ch+gap)
		b.PushTransform(gfx.Translate(x, y).Mul(gfx.Scale(s, s)))
		draw(b, cw/s, ch/s)
		b.PopTransform()
	}
	return b.Finish()
}

// Design size of one grid cell, in logical pixels. It is what a 960x640
// window gives each of the 4x3 cells.
const (
	designW = 202
	designH = 170
)

// Tailwind CSS v4 colors (packages/tailwindcss/theme.css), as OKLCH.
var (
	slate50   = gfx.OKLCH(0.984, 0.003, 247.858, 1)
	slate100  = gfx.OKLCH(0.968, 0.007, 247.896, 1)
	slate200  = gfx.OKLCH(0.929, 0.013, 255.508, 1)
	slate300  = gfx.OKLCH(0.869, 0.022, 252.894, 1)
	slate400  = gfx.OKLCH(0.704, 0.04, 256.788, 1)
	slate700  = gfx.OKLCH(0.372, 0.044, 257.287, 1)
	slate800  = gfx.OKLCH(0.279, 0.041, 260.031, 1)
	slate900  = gfx.OKLCH(0.208, 0.042, 265.755, 1)
	slate950  = gfx.OKLCH(0.129, 0.042, 264.695, 1)
	indigo50  = gfx.OKLCH(0.962, 0.018, 272.314, 1)
	indigo100 = gfx.OKLCH(0.93, 0.034, 272.788, 1)
	indigo200 = gfx.OKLCH(0.87, 0.065, 274.039, 1)
	indigo300 = gfx.OKLCH(0.785, 0.115, 274.713, 1)
	indigo400 = gfx.OKLCH(0.673, 0.182, 276.935, 1)
	indigo500 = gfx.OKLCH(0.585, 0.233, 277.117, 1)
	indigo600 = gfx.OKLCH(0.511, 0.262, 276.966, 1)
	violet400 = gfx.OKLCH(0.702, 0.183, 293.541, 1)
	violet600 = gfx.OKLCH(0.541, 0.281, 293.009, 1)
	fuchsia4  = gfx.OKLCH(0.74, 0.238, 322.16, 1)
	fuchsia5  = gfx.OKLCH(0.667, 0.295, 322.15, 1)
	fuchsia6  = gfx.OKLCH(0.591, 0.293, 322.896, 1)
	pink300   = gfx.OKLCH(0.823, 0.12, 346.018, 1)
	rose300   = gfx.OKLCH(0.81, 0.117, 11.638, 1)
	rose400   = gfx.OKLCH(0.712, 0.194, 13.428, 1)
	rose500   = gfx.OKLCH(0.645, 0.246, 16.439, 1)
	orange400 = gfx.OKLCH(0.75, 0.183, 55.934, 1)
	amber200  = gfx.OKLCH(0.924, 0.12, 95.746, 1)
	amber300  = gfx.OKLCH(0.879, 0.169, 91.605, 1)
	amber400  = gfx.OKLCH(0.828, 0.189, 84.429, 1)
	emerald1  = gfx.OKLCH(0.95, 0.052, 163.051, 1)
	emerald4  = gfx.OKLCH(0.765, 0.177, 163.223, 1)
	emerald5  = gfx.OKLCH(0.696, 0.17, 162.48, 1)
	emerald6  = gfx.OKLCH(0.596, 0.145, 163.225, 1)
	teal400   = gfx.OKLCH(0.777, 0.152, 181.912, 1)
	sky200    = gfx.OKLCH(0.901, 0.058, 230.902, 1)
	sky300    = gfx.OKLCH(0.828, 0.111, 230.318, 1)
	sky400    = gfx.OKLCH(0.746, 0.16, 232.661, 1)
	sky500    = gfx.OKLCH(0.685, 0.169, 237.323, 1)
)

// cssShadow is one CSS box-shadow layer: offset-x offset-y blur spread color,
// optionally inset.
type cssShadow struct {
	x, y, blur, spread float32
	color              gfx.Color
	inset              bool
}

func black(a float32) gfx.Color { return gfx.Black.WithAlpha(a) }

// Tailwind CSS v4.3.3 box shadows (--shadow-* and --inset-shadow-*), in CSS
// order (the first layer is on top).
var (
	shadowXS  = []cssShadow{{0, 1, 2, 0, black(0.05), false}}
	shadowSM  = []cssShadow{{0, 1, 3, 0, black(0.1), false}, {0, 1, 2, -1, black(0.1), false}}
	shadowMD  = []cssShadow{{0, 4, 6, -1, black(0.1), false}, {0, 2, 4, -2, black(0.1), false}}
	shadowLG  = []cssShadow{{0, 10, 15, -3, black(0.1), false}, {0, 4, 6, -4, black(0.1), false}}
	shadowXL  = []cssShadow{{0, 20, 25, -5, black(0.1), false}, {0, 8, 10, -6, black(0.1), false}}
	shadow2XL = []cssShadow{{0, 25, 50, -12, black(0.25), false}}
)

// tinted returns the layers of shadows with their color replaced by c
// (Tailwind's shadow-<color>/<opacity> utilities).
func tinted(shadows []cssShadow, c gfx.Color) []cssShadow {
	out := make([]cssShadow, len(shadows))
	for i, s := range shadows {
		s.color = c
		out[i] = s
	}
	return out
}

// drawShadows paints CSS shadow layers for box, last layer first, so that the
// first layer ends up on top. Outer shadows go before the box fill and inset
// shadows after it.
func drawShadows(b *gfx.Builder, box gfx.RRect, layers []cssShadow) {
	for i := len(layers) - 1; i >= 0; i-- {
		l := layers[i]
		b.DrawShadow(gfx.DrawShadow{RRect: box, Offset: gfx.Pt(l.x, l.y), Blur: l.blur, Spread: l.spread, Color: l.color, Inset: l.inset})
	}
}

// --- small helpers ---

func rect(x, y, w, h float32) gfx.Rect { return gfx.RectXYWH(x, y, w, h) }

// pill fills a rounded-full rectangle.
func pill(b *gfx.Builder, x, y, w, h float32, p gfx.Paint) {
	b.FillRRect(gfx.NewRRect(rect(x, y, w, h), h/2), p)
}

// disc fills a circle.
func disc(b *gfx.Builder, cx, cy, r float32, p gfx.Paint) {
	b.FillRRect(gfx.NewRRect(rect(cx-r, cy-r, 2*r, 2*r), r), p)
}

// base paints a card: its outer shadows, its fill and its inset shadows. It
// returns the card's box.
func base(b *gfx.Builder, w, h, radius float32, shadows []cssShadow, fill gfx.Paint) gfx.RRect {
	box := gfx.NewRRect(rect(0, 0, w, h), radius)
	drawShadows(b, box, outerOnly(shadows))
	b.FillRRect(box, fill)
	drawShadows(b, box, innerOnly(shadows))
	return box
}

func outerOnly(s []cssShadow) []cssShadow { return filterShadows(s, false) }
func innerOnly(s []cssShadow) []cssShadow { return filterShadows(s, true) }

func filterShadows(s []cssShadow, inset bool) []cssShadow {
	var out []cssShadow
	for _, l := range s {
		if l.inset == inset {
			out = append(out, l)
		}
	}
	return out
}

// stops builds gradient stops spread evenly.
func stops(colors ...gfx.Color) []gfx.GradientStop {
	out := make([]gfx.GradientStop, len(colors))
	for i, c := range colors {
		out[i] = gfx.AutoStop(c)
	}
	return out
}

// Unit gradient directions (CSS angles). They are constants instead of
// sin/cos calls so that the scene does not depend on the CPU's math.
var (
	toRight  = gfx.Pt(1, 0)
	to160deg = gfx.Pt(0.34202014, 0.93969262)
)

// linearToBR is bg-linear-to-br: "to bottom right", whose 50% line passes
// through the other two corners.
func linearToBR(r gfx.Rect, st []gfx.GradientStop) gfx.LinearGradient {
	w, h := r.Width(), r.Height()
	return linearAlong(r, gfx.Pt(h, w).Normalize(), st)
}

func linearAlong(r gfx.Rect, d gfx.Point, st []gfx.GradientStop) gfx.LinearGradient {
	length := float32(math.Abs(float64(r.Width()*d.X))) + float32(math.Abs(float64(r.Height()*d.Y)))
	c := r.Center()
	half := d.Scale(length / 2)
	return gfx.LinearGradient{Start: c.Sub(half), End: c.Add(half), Stops: st}
}

// topRounded returns r with rounded top corners only.
func topRounded(r gfx.Rect, radius float32) gfx.RRect {
	p := gfx.Pt(radius, radius)
	return gfx.RRectCorners(r, p, p, gfx.Point{}, gfx.Point{})
}

// arrow strokes a right arrow centered at c.
func arrow(b *gfx.Builder, c gfx.Point, size, width float32, col gfx.Color) {
	var p gfx.Path
	p.MoveTo(gfx.Pt(c.X-size, c.Y))
	p.LineTo(gfx.Pt(c.X+size, c.Y))
	p.MoveTo(gfx.Pt(c.X+size*0.15, c.Y-size*0.85))
	p.LineTo(gfx.Pt(c.X+size, c.Y))
	p.LineTo(gfx.Pt(c.X+size*0.15, c.Y+size*0.85))
	st := gfx.NewStrokeStyle(width)
	st.Cap, st.Join = gfx.CapRound, gfx.JoinRound
	b.StrokePath(p, st, col)
}

// --- page ---

// drawPage paints the page background: a diagonal slate-50 to indigo-50
// gradient (interpolated in Oklab) with two soft glows.
func drawPage(b *gfx.Builder, w, h float32) {
	page := gfx.Rect{X1: w, Y1: h}
	b.FillRect(page, linearToBR(page, stops(slate50, indigo50)))
	r := max(w, h) * 0.55
	b.FillRect(page, gfx.RadialGradient{Center: gfx.Pt(w*0.92, h*0.02), Radius: gfx.Pt(r, r),
		Stops: stops(indigo200.WithAlpha(0.55), indigo200.WithAlpha(0))})
	b.FillRect(page, gfx.RadialGradient{Center: gfx.Pt(w*0.05, h*1.0), Radius: gfx.Pt(r, r),
		Stops: stops(sky200.WithAlpha(0.6), sky200.WithAlpha(0))})
}

// --- artwork ---

// landscape paints an image-like picture into r: a vertical sky gradient, a
// sun and two hills. The caller sets the clip.
func landscape(b *gfx.Builder, r gfx.Rect, skyTop, skyBottom, sun, hillBack, hillFront gfx.Color) {
	w, h := r.Width(), r.Height()
	b.FillRect(r, gfx.LinearGradient{Start: gfx.Pt(0, r.Y0), End: gfx.Pt(0, r.Y1), Stops: stops(skyTop, skyBottom)})
	disc(b, r.X0+w*0.68, r.Y0+h*0.36, h*0.16, sun)
	hill := func(y0, c1y, c2y, y1 float32, c gfx.Color) {
		var p gfx.Path
		p.MoveTo(gfx.Pt(r.X0, r.Y0+h*y0))
		p.CubicTo(gfx.Pt(r.X0+w*0.3, r.Y0+h*c1y), gfx.Pt(r.X0+w*0.6, r.Y0+h*c2y), gfx.Pt(r.X1, r.Y0+h*y1))
		p.LineTo(gfx.Pt(r.X1, r.Y1))
		p.LineTo(gfx.Pt(r.X0, r.Y1))
		p.Close()
		b.FillPath(p, gfx.NonZero, c)
	}
	hill(0.72, 0.42, 0.9, 0.56, hillBack)
	hill(0.88, 0.66, 1.05, 0.76, hillFront)
}

// --- cards ---

// cardProfile: shadow-sm, a circular avatar cut by a rounded clip.
func cardProfile(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadowSM, gfx.White)
	const d = 56
	avatar := gfx.NewRRect(rect(16, 16, d, d), d/2)
	b.PushClipRRect(avatar)
	landscape(b, avatar.Rect, sky300, amber200, amber300, emerald4, emerald6)
	b.PopClip()
	b.StrokePath(gfx.CirclePath(gfx.Pt(16+d/2, 16+d/2), d/2+1.5), gfx.NewStrokeStyle(3), indigo500)
	pill(b, 84, 26, 84, 10, slate800)
	pill(b, 84, 44, 56, 8, slate300)
	lw := w - 32
	pill(b, 16, 88, lw, 8, slate200)
	pill(b, 16, 104, lw*0.86, 8, slate200)
	pill(b, 16, 120, lw*0.55, 8, slate200)
	pill(b, 16, h-16-24, 68, 24, indigo500)
	pill(b, 92, h-16-24, 58, 24, slate100)
}

// cardChart: shadow-md, a small bar chart.
func cardChart(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadowMD, gfx.White)
	pill(b, 16, 20, 72, 10, slate800)
	pill(b, 16, 38, 44, 8, slate300)
	pill(b, w-16-44, 18, 44, 20, emerald1)
	disc(b, w-16-44+12, 28, 4, emerald5)
	pill(b, w-16-44+22, 25, 14, 6, emerald6)

	heights := [...]float32{0.34, 0.52, 0.44, 0.72, 0.58, 1, 0.66}
	const gapX = 8
	n := float32(len(heights))
	bw := (w - 32 - (n-1)*gapX) / n
	top, bottom := float32(62), h-16
	for i, f := range heights {
		x := 16 + float32(i)*(bw+gapX)
		bh := (bottom - top) * f
		r := rect(x, bottom-bh, bw, bh)
		var p gfx.Paint = indigo100
		if i == 5 {
			p = gfx.LinearGradient{Start: gfx.Pt(0, r.Y0), End: gfx.Pt(0, r.Y1), Stops: stops(indigo400, indigo600)}
		}
		b.FillRRect(topRounded(r, 6), p)
	}
}

// cardPhoto: shadow-lg, a rounded photo and a round action button.
func cardPhoto(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadowLG, gfx.White)
	photo := gfx.NewRRect(rect(10, 10, w-20, 88), 10)
	b.PushClipRRect(photo)
	landscape(b, photo.Rect, violet400, pink300, amber200, indigo400, indigo600)
	b.PopClip()
	pill(b, 16, 112, 96, 10, slate800)
	pill(b, 16, 130, 64, 8, slate300)
	cx, cy := w-16-17, h-16-17
	disc(b, cx, cy, 17, indigo500)
	arrow(b, gfx.Pt(cx, cy), 6, 2.5, gfx.White)
}

// cardToggles: shadow-xl, a list of switches.
func cardToggles(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadowXL, gfx.White)
	on := [...]bool{true, false, true}
	for i, v := range on {
		y := 24 + float32(i)*48
		track := slate300
		if v {
			track = indigo500
		}
		pill(b, 16, y, 46, 26, track)
		kx := float32(16 + 13)
		if v {
			kx = 16 + 46 - 13
		}
		knob := gfx.NewRRect(rect(kx-11, y+2, 22, 22), 11)
		drawShadows(b, knob, shadowMD)
		b.FillRRect(knob, gfx.White)
		pill(b, 78, y+3, 72-float32(i)*14, 9, slate700)
		pill(b, 78, y+17, 44+float32(i)*6, 7, slate300)
	}
}

// cardProgress: shadow-2xl, progress bars with gradient fills.
func cardProgress(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadow2XL, gfx.White)
	type row struct {
		frac     float32
		from, to gfx.Color
	}
	rows := [...]row{{0.78, indigo500, fuchsia5}, {0.46, sky400, emerald4}, {0.9, amber400, rose500}}
	tw := w - 32
	for i, r := range rows {
		y := 25 + float32(i)*47
		pill(b, 16, y, 66, 9, slate700)
		pill(b, w-16-26, y, 26, 9, slate300)
		pill(b, 16, y+18, tw, 10, slate100)
		fill := rect(16, y+18, tw*r.frac, 10)
		b.FillRRect(gfx.NewRRect(fill, 5), gfx.LinearGradient{Start: gfx.Pt(fill.X0, 0), End: gfx.Pt(fill.X1, 0), Stops: stops(r.from, r.to)})
	}
}

// cardInset: an inset shadow (inset-shadow-sm with a 1px inset ring) on a
// slate-100 well, holding raised chips.
func cardInset(b *gfx.Builder, w, h float32) {
	well := []cssShadow{
		{0, 2, 6, 0, black(0.14), true},
		{0, 0, 0, 1, slate200, true},
	}
	base(b, w, h, 16, well, slate100)

	search := gfx.NewRRect(rect(14, 14, w-28, 34), 17)
	drawShadows(b, search, shadowXS)
	b.FillRRect(search, gfx.White)
	st := gfx.NewStrokeStyle(2)
	st.Cap = gfx.CapRound
	b.StrokePath(gfx.CirclePath(gfx.Pt(36, 30), 5.5), st, slate400)
	var handle gfx.Path
	handle.MoveTo(gfx.Pt(40.2, 34.2))
	handle.LineTo(gfx.Pt(44, 38))
	b.StrokePath(handle, st, slate400)
	pill(b, 54, 27, 70, 8, slate300)

	chips := [...]struct {
		x, y, w float32
		c       gfx.Color
	}{
		{14, 64, 50, indigo500}, {72, 64, 58, emerald5}, {138, 64, 50, rose400},
		{14, 102, 70, amber400}, {92, 102, 46, sky500},
	}
	for _, c := range chips {
		chip := gfx.NewRRect(rect(c.x, c.y, c.w, 28), 14)
		drawShadows(b, chip, shadowSM)
		b.FillRRect(chip, gfx.White)
		disc(b, c.x+14, c.y+14, 5, c.c)
		pill(b, c.x+25, c.y+10, c.w-36, 8, slate300)
	}
	pill(b, 14, h-14-8, (w-28)*0.6, 8, slate300)
}

// cardLinear: bg-linear-to-br from-indigo-500 to-fuchsia-600 with a colored
// shadow-lg, rounded-3xl.
func cardLinear(b *gfx.Builder, w, h float32) {
	box := base(b, w, h, 24, tinted(shadowLG, indigo500.WithAlpha(0.5)),
		linearToBR(rect(0, 0, w, h), stops(indigo500, fuchsia6)))
	b.PushClipRRect(box)
	disc(b, w*0.88, -4, 66, gfx.White.WithAlpha(0.12))
	disc(b, -6, h+10, 74, gfx.White.WithAlpha(0.09))
	b.PopClip()
	badge := gfx.NewRRect(rect(18, 18, 64, 24), 12)
	b.FillRRect(badge, gfx.White.WithAlpha(0.16))
	b.StrokePath(gfx.NewRRect(rect(18.5, 18.5, 63, 23), 11.5).Path(), gfx.NewStrokeStyle(1), gfx.White.WithAlpha(0.35))
	disc(b, 31, 30, 4, gfx.White)
	pill(b, 41, 27, 30, 6, gfx.White.WithAlpha(0.8))
	pill(b, 20, h-76, 118, 14, gfx.White)
	pill(b, 20, h-52, 84, 9, gfx.White.WithAlpha(0.6))
	cx, cy := w-20-19, h-20-19
	disc(b, cx, cy, 19, gfx.White.WithAlpha(0.95))
	arrow(b, gfx.Pt(cx, cy), 7, 2.5, indigo600)
}

// cardRadial: a radial gradient sunset with a frosted pill.
func cardRadial(b *gfx.Builder, w, h float32) {
	g := gfx.RadialGradient{Center: gfx.Pt(w*0.3, h*0.28), Radius: gfx.Pt(w*1.0, h*1.15),
		Stops: []gfx.GradientStop{{Offset: 0, Color: amber200}, {Offset: 0.48, Color: rose400}, {Offset: 1, Color: violet600}}}
	box := base(b, w, h, 16, tinted(shadowXL, rose500.WithAlpha(0.35)), g)
	b.PushClipRRect(box)
	// A soft highlight on top of the gradient.
	b.FillRect(rect(0, 0, w, h), gfx.RadialGradient{Center: gfx.Pt(w*0.3, h*0.28), Radius: gfx.Pt(46, 46),
		Stops: stops(gfx.White.WithAlpha(0.85), gfx.White.WithAlpha(0.0))})
	b.PopClip()
	glass := gfx.NewRRect(rect(14, h-14-34, w-28, 34), 17)
	b.FillRRect(glass, gfx.White.WithAlpha(0.2))
	b.StrokePath(gfx.NewRRect(rect(14.5, h-14-33.5, w-29, 33), 16.5).Path(), gfx.NewStrokeStyle(1), gfx.White.WithAlpha(0.4))
	disc(b, 31, h-14-17, 8, gfx.White)
	pill(b, 47, h-14-21, 74, 8, gfx.White.WithAlpha(0.9))
	pill(b, 47, h-14-11, 44, 6, gfx.White.WithAlpha(0.55))
}

// cardConic: a conic gradient ring on a white card.
func cardConic(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadowMD, gfx.White)
	c := gfx.Pt(w/2, h/2)
	const outer, inner = 56, 36
	var ring gfx.Path
	ring.AddCircle(c, outer)
	ring.AddCircle(c, inner)
	b.FillPath(ring, gfx.EvenOdd, gfx.ConicGradient{Center: c,
		Stops: stops(rose500, amber400, emerald4, sky500, indigo500, fuchsia5, rose500)})
	// A knob at the start of the ring.
	kc := gfx.Pt(c.X, c.Y-(outer+inner)/2)
	knob := gfx.NewRRect(rect(kc.X-9, kc.Y-9, 18, 18), 9)
	drawShadows(b, knob, shadowMD)
	b.FillRRect(knob, gfx.White)
	disc(b, kc.X, kc.Y, 4, rose500)
	// The hole: a small raised disc.
	hole := gfx.NewRRect(rect(c.X-20, c.Y-20, 40, 40), 20)
	b.FillRRect(hole, slate50)
	pill(b, c.X-11, c.Y-4, 22, 8, slate300)
}

// cardGlass: blurred color blobs behind a frosted pane, inside a rounded
// clip. The blobs use a Blur layer; the pane fakes backdrop-blur with
// translucent paint.
func cardGlass(b *gfx.Builder, w, h float32) {
	box := base(b, w, h, 20, shadowXL, linearToBR(rect(0, 0, w, h), stops(slate900, slate950)))
	b.PushClipRRect(box)
	b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 16})
	disc(b, w*0.26, h*0.32, 44, indigo500)
	disc(b, w*0.76, h*0.26, 38, fuchsia5)
	disc(b, w*0.56, h*0.82, 46, sky500)
	disc(b, w*0.12, h*0.86, 24, amber400)
	b.PopLayer()
	b.PopClip()

	pane := gfx.NewRRect(rect(18, 34, w-36, h-68), 16)
	drawShadows(b, pane, []cssShadow{{0, 10, 24, -4, black(0.4), false}})
	b.FillRRect(pane, linearAlong(pane.Rect, to160deg, stops(gfx.White.WithAlpha(0.3), gfx.White.WithAlpha(0.07))))
	b.StrokePath(gfx.NewRRect(rect(18.5, 34.5, w-37, h-69), 15.5).Path(), gfx.NewStrokeStyle(1),
		linearAlong(pane.Rect, to160deg, stops(gfx.White.WithAlpha(0.55), gfx.White.WithAlpha(0.12))))
	disc(b, 44, 62, 12, gfx.White.WithAlpha(0.9))
	pill(b, 64, 54, 70, 9, gfx.White.WithAlpha(0.9))
	pill(b, 64, 69, 46, 7, gfx.White.WithAlpha(0.55))
	pill(b, 32, 92, w-64, 7, gfx.White.WithAlpha(0.35))
	pill(b, 32, 106, (w-64)*0.7, 7, gfx.White.WithAlpha(0.35))
}

// starPath returns a five-pointed star centered at c with the given outer and
// inner radii. The unit vectors are constants so that the geometry does not
// depend on the CPU's math.
func starPath(c gfx.Point, outer, inner float32) gfx.Path {
	// Directions at -90, -54, -18, 18, ... degrees. Even entries are tips and
	// odd entries are notches.
	unit := [10][2]float32{
		{0, -1}, {0.58778525, -0.80901699}, {0.95105652, -0.30901699}, {0.95105652, 0.30901699},
		{0.58778525, 0.80901699}, {0, 1}, {-0.58778525, 0.80901699}, {-0.95105652, 0.30901699},
		{-0.95105652, -0.30901699}, {-0.58778525, -0.80901699},
	}
	var p gfx.Path
	for i, u := range unit {
		r := outer
		if i%2 == 1 {
			r = inner
		}
		pt := gfx.Pt(c.X+float32(r*u[0]), c.Y+float32(r*u[1]))
		if i == 0 {
			p.MoveTo(pt)
		} else {
			p.LineTo(pt)
		}
	}
	p.Close()
	return p
}

// blobPath returns a smooth closed shape through six points around c, with
// the given radius factors (Catmull-Rom spline converted to cubics).
func blobPath(c gfx.Point, r float32, k [6]float32) gfx.Path {
	unit := [6][2]float32{{1, 0}, {0.5, 0.8660254}, {-0.5, 0.8660254}, {-1, 0}, {-0.5, -0.8660254}, {0.5, -0.8660254}}
	var pts [6]gfx.Point
	for i, u := range unit {
		pts[i] = gfx.Pt(c.X+float32(r*k[i]*u[0]), c.Y+float32(r*k[i]*u[1]))
	}
	at := func(i int) gfx.Point { return pts[((i%6)+6)%6] }
	var p gfx.Path
	p.MoveTo(pts[0])
	for i := 0; i < 6; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		c1 := p1.Add(p2.Sub(p0).Scale(1.0 / 6))
		c2 := p2.Sub(p3.Sub(p1).Scale(1.0 / 6))
		p.CubicTo(c1, c2, p2)
	}
	p.Close()
	return p
}

// cardDropShadow: drop-shadow filters on non-rectangular shapes.
func cardDropShadow(b *gfx.Builder, w, h float32) {
	base(b, w, h, 16, shadowMD, gfx.White)

	star := starPath(gfx.Pt(w*0.3, h*0.46), 40, 17)
	starPaint := linearToBR(star.Bounds(), stops(amber300, orange400))
	b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, 9), StdDev: 6, Color: orange400.WithAlpha(0.6)})
	b.FillPath(star, gfx.NonZero, starPaint)
	rounded := gfx.NewStrokeStyle(7)
	rounded.Join = gfx.JoinRound
	b.StrokePath(star, rounded, starPaint)
	b.PopLayer()

	blob := blobPath(gfx.Pt(w*0.72, h*0.46), 40, [6]float32{1, 0.9, 1, 0.88, 1.05, 0.95})
	blobPaint := linearToBR(blob.Bounds(), stops(emerald4, sky500))
	b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, 9), StdDev: 6, Color: sky500.WithAlpha(0.5)})
	b.FillPath(blob, gfx.NonZero, blobPaint)
	b.PopLayer()

	pill(b, 16, h-26, 62, 8, slate200)
	pill(b, w-16-40, h-26, 40, 8, slate200)
}

// cardStrokes: a dashed border (border-dashed) around a drop zone with a
// curve and a zigzag, both stroked with round caps and joins.
func cardStrokes(b *gfx.Builder, w, h float32) {
	box := gfx.NewRRect(rect(0, 0, w, h), 16)
	b.FillRRect(box, gfx.White.WithAlpha(0.6))
	dashed := gfx.NewStrokeStyle(2)
	dashed.Dashes = []float32{7, 6}
	b.StrokePath(gfx.NewRRect(rect(1, 1, w-2, h-2), 15).Path(), dashed, indigo300)

	// A wave made of S-shaped cubics.
	const segs = 3
	x0, y0 := float32(26), h*0.36
	seg := (w - 2*x0) / segs
	amp := float32(15)
	var wave gfx.Path
	wave.MoveTo(gfx.Pt(x0, y0))
	for i := 0; i < segs; i++ {
		x := x0 + float32(i)*seg
		wave.CubicTo(gfx.Pt(x+seg*0.36, y0-amp*1.7), gfx.Pt(x+seg*0.64, y0+amp*1.7), gfx.Pt(x+seg, y0))
	}
	st := gfx.NewStrokeStyle(6)
	st.Cap, st.Join = gfx.CapRound, gfx.JoinRound
	b.StrokePath(wave, st, linearAlong(rect(x0, 0, w-2*x0, 1), toRight, stops(sky400, indigo500, fuchsia5)))
	end := gfx.Pt(x0+segs*seg, y0)
	disc(b, end.X, end.Y, 7, gfx.White)
	disc(b, end.X, end.Y, 4.5, fuchsia5)

	// A zigzag: sharp turns made round by the join.
	zy := h * 0.72
	var zig gfx.Path
	zig.MoveTo(gfx.Pt(x0, zy))
	n := 7
	step := (w - 2*x0) / float32(n)
	for i := 1; i <= n; i++ {
		dy := float32(-13)
		if i%2 == 0 {
			dy = 13
		}
		zig.LineTo(gfx.Pt(x0+float32(i)*step, zy+dy))
	}
	zst := gfx.NewStrokeStyle(5)
	zst.Cap, zst.Join = gfx.CapRound, gfx.JoinRound
	b.StrokePath(zig, zst, emerald5)
}
