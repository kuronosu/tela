// Package scene is a test package of cmd/tela with a phase-1 screen: a
// Scene function that builds a display list directly. It stands for the
// packages written before the Preview contract.
package scene

import "github.com/kuronosu/tela/internal/gfx"

// Scene draws a gradient page with a rounded card, a shadow and a disc. It
// uses no text, so it needs no fonts, and no randomness.
func Scene(w, h float32) *gfx.DisplayList {
	b := gfx.NewBuilder()
	page := gfx.Rect{X1: w, Y1: h}
	b.FillRect(page, gfx.LinearGradient{
		Start: gfx.Pt(0, 0), End: gfx.Pt(w, h),
		Stops: []gfx.GradientStop{gfx.AutoStop(gfx.MustParseHex("#f8fafc")), gfx.AutoStop(gfx.MustParseHex("#e0e7ff"))},
	})
	card := gfx.NewRRect(gfx.RectXYWH(w*0.15, h*0.2, w*0.7, h*0.6), 12)
	b.DrawShadow(gfx.DrawShadow{RRect: card, Offset: gfx.Pt(0, 8), Blur: 16, Spread: -4, Color: gfx.Black.WithAlpha(0.25)})
	b.FillRRect(card, gfx.White)
	r := h * 0.14
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(w*0.5-r, h*0.5-r, 2*r, 2*r), r), gfx.MustParseHex("#6366f1"))
	return b.Finish()
}
