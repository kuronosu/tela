package raster

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Scenes for gradients, images, clips, layers and strokes (T4b), defined in
// package gfxtest. They are rendered as goldens at scales 1 and 2 and used by
// the region tests.

var (
	fuchsia = gfxtest.Fuchsia
	blue    = gfxtest.Blue
	yellow  = gfxtest.Yellow
	red     = gfxtest.Red
	lime    = gfxtest.Lime
)

func stops(cs ...gfx.Color) []gfx.GradientStop { return gfxtest.Stops(cs...) }

var testImage = gfxtest.TestImage

func tightCurve(x, y float32) gfx.Path { return gfxtest.TightCurve(x, y) }

var scenes2 = fromGfxtest(gfxtest.Scenes2)
