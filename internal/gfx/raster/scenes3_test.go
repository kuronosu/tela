package raster

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Scenes for shadows and layer filters (T6), defined in package gfxtest.

var twGray100 = gfxtest.TwGray100

type cssShadow = gfxtest.CSSShadow

type card = gfxtest.Card

var twShadows = gfxtest.TwShadows

func drawShadows(b *gfx.Builder, box gfx.RRect, layers []cssShadow) {
	gfxtest.DrawShadows(b, box, layers)
}

func twCards() []card { return gfxtest.TwCards() }

func drawTailwindScene(b *gfx.Builder) { gfxtest.DrawTailwindScene(b) }

var scenes3 = fromGfxtest(gfxtest.Scenes3)
