package gfxtest

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Scenes1 are the rectangle, path and transform scenes.
var Scenes1 = []Scene{
	{"rects", 160, 120, func(b *gfx.Builder) {
		// Rectangles at fractional positions and sizes.
		for i := 0; i < 6; i++ {
			f := float32(i) / 6
			b.FillRect(gfx.RectXYWH(8+f+float32(i)*24, 8+f, 18+f, 18+f), Indigo)
			b.FillRect(gfx.RectXYWH(8+float32(i)*24, 34+f*0.5, 18.25, 10.5), Rose)
		}
		// Thin slivers: vertical and horizontal, 0.1 to 1 px.
		for i := 0; i < 10; i++ {
			wd := 0.1 * float32(i+1)
			b.FillRect(gfx.RectXYWH(8+float32(i)*6+0.3, 52, wd, 30), Slate)
			b.FillRect(gfx.RectXYWH(72, 52+float32(i)*3+0.6, 80, wd), Slate)
		}
		// Sub-pixel squares.
		for i := 0; i < 8; i++ {
			s := 0.25 + 0.25*float32(i)
			b.FillRect(gfx.RectXYWH(8+float32(i)*8.4, 90.3, s, s), Emerald)
		}
		// A translucent rect over all of them.
		b.FillRect(gfx.Rect{X0: 100.5, Y0: 85.5, X1: 150.25, Y1: 112.75}, Sky.WithAlpha(0.5))
	}},
	{"rrects", 200, 150, func(b *gfx.Builder) {
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(8, 8, 56, 40), 8), Indigo)            // rounded-lg
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(72.5, 8.25, 56, 40), 3.5), Rose)      // small radius, fractional
		b.FillRRect(gfx.RRectXY(gfx.RectXYWH(136.3, 8.6, 56, 40), 24, 10), Emerald) // elliptical
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(8, 56, 120, 24), 999), Amber)         // rounded-full pill
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(136.5, 56.5, 24, 60), 999), Sky)      // vertical pill
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(166.2, 56.2, 28, 28), 999), Slate)    // circle
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(170, 90, 7, 7), 999), Rose)           // tiny circle
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(180.4, 92.4, 3.2, 3.2), 999), Rose)   // sub-3px circle
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(8, 88, 60, 54),                   // per corner
			gfx.Pt(0, 0), gfx.Pt(20, 20), gfx.Pt(6, 6), gfx.Pt(30, 12)), Indigo)
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(76.4, 88.4, 52, 54), // opposite corner boxes overlap
			gfx.Pt(48, 48), gfx.Pt(0, 0), gfx.Pt(48, 48), gfx.Pt(0, 0)), Emerald)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(140, 120, 50, 22), 6), Rose.WithAlpha(0.6)) // translucent
	}},
	{"circles", 160, 100, func(b *gfx.Builder) {
		for i := 0; i < 6; i++ {
			r := 1 + float32(i)*float32(i)
			x := 10 + float32(i)*10 + float32(i*i)*2
			b.FillPath(gfx.CirclePath(gfx.Pt(x+0.3, 30.3), r), gfx.NonZero, Slate)
		}
		b.FillPath(gfx.EllipsePath(gfx.Pt(40, 72), 32, 16), gfx.NonZero, Indigo)
		b.FillPath(gfx.EllipsePath(gfx.Pt(110, 70), 12, 26), gfx.NonZero, Rose.WithAlpha(0.8))
		b.FillPath(gfx.CirclePath(gfx.Pt(140, 70), 14.5), gfx.NonZero, Emerald.WithAlpha(0.7))
	}},
	{"star", 170, 120, func(b *gfx.Builder) {
		// Left column NonZero, right column EvenOdd.
		b.FillPath(StarPath(42, 45, 38), gfx.NonZero, Amber)
		b.FillPath(StarPath(127, 45, 38), gfx.EvenOdd, Amber)
		// Concentric circles wound the same way: a disk with NonZero, a ring
		// with EvenOdd.
		var ring gfx.Path
		ring.AddCircle(gfx.Pt(0, 0), 14)
		ring.AddCircle(gfx.Pt(0, 0), 7)
		b.PushTransform(gfx.Translate(42.5, 102.5))
		b.FillPath(ring, gfx.NonZero, Indigo)
		b.PopTransform()
		b.PushTransform(gfx.Translate(127.5, 102.5))
		b.FillPath(ring, gfx.EvenOdd, Indigo)
		b.PopTransform()
	}},
	{"curves", 180, 110, func(b *gfx.Builder) {
		var q gfx.Path // quadratic blob
		q.MoveTo(gfx.Pt(10, 60))
		q.QuadTo(gfx.Pt(30, 0), gfx.Pt(60, 40))
		q.QuadTo(gfx.Pt(90, 80), gfx.Pt(50, 100))
		q.QuadTo(gfx.Pt(0, 110), gfx.Pt(10, 60))
		q.Close()
		b.FillPath(q, gfx.NonZero, Sky)
		var c gfx.Path // cubic with an inflection and a loop
		c.MoveTo(gfx.Pt(80, 90))
		c.CubicTo(gfx.Pt(100, -20), gfx.Pt(140, 140), gfx.Pt(170, 20))
		c.LineTo(gfx.Pt(170, 100))
		c.Close()
		b.FillPath(c, gfx.NonZero, Rose.WithAlpha(0.85))
		var l gfx.Path // self-intersecting cubic loop, EvenOdd
		l.MoveTo(gfx.Pt(100, 100))
		l.CubicTo(gfx.Pt(180, 40), gfx.Pt(80, 40), gfx.Pt(160, 105))
		b.FillPath(l, gfx.EvenOdd, Slate.WithAlpha(0.7))
		var open gfx.Path // open subpath: closed implicitly
		open.MoveTo(gfx.Pt(20, 15))
		open.CubicTo(gfx.Pt(40, 0), gfx.Pt(60, 30), gfx.Pt(75, 10))
		b.FillPath(open, gfx.NonZero, Emerald)
	}},
	{"transforms", 200, 130, func(b *gfx.Builder) {
		card := gfx.NewRRect(gfx.RectXYWH(-30, -18, 60, 36), 8)
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(45, 40)))
		b.PushTransform(gfx.Translate(45, 40))
		b.FillRRect(card, Indigo)
		b.PopTransform()
		b.PopTransform()
		b.PushTransform(gfx.Translate(140, 40).Mul(gfx.Skew(0.4, 0)))
		b.FillRRect(card, Rose)
		b.PopTransform()
		b.PushTransform(gfx.Translate(45, 100).Mul(gfx.Rotate(-0.2)).Mul(gfx.Scale(1.2, 0.6)))
		b.FillRect(gfx.RectXYWH(-30, -18, 60, 36), Emerald)
		b.PopTransform()
		// Flip + scale keeps the fast paths; the per-corner radii must flip.
		b.PushTransform(gfx.Translate(140, 100).Mul(gfx.Scale(-1.5, 1)))
		b.FillRRect(gfx.RRectCorners(gfx.RectXYWH(-20, -18, 40, 36),
			gfx.Pt(16, 16), gfx.Pt(0, 0), gfx.Pt(4, 4), gfx.Pt(0, 0)), Amber)
		b.PopTransform()
		// Rotation by 90 degrees: rect fast path.
		b.PushTransform(gfx.Translate(190, 70).Mul(gfx.Rotate(math.Pi / 2)))
		b.FillRect(gfx.RectXYWH(0.3, 0.3, 40.4, 6.5), Slate)
		b.PopTransform()
	}},
	{"overlap", 160, 110, func(b *gfx.Builder) {
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, 80, 60), 12), Indigo.WithAlpha(0.6))
		b.FillPath(gfx.CirclePath(gfx.Pt(95, 55), 38), gfx.NonZero, Rose.WithAlpha(0.5))
		b.FillRect(gfx.RectXYWH(40.5, 40.5, 100, 30), Emerald.WithAlpha(0.35))
		b.FillPath(StarPath(70, 70, 30), gfx.EvenOdd, Amber.WithAlpha(0.7))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(20, 80, 120, 20), 999), gfx.White.WithAlpha(0.5))
	}},
}
