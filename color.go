package tela

import (
	"github.com/kuronosu/tela/internal/gfx"
)

// Color is a non-premultiplied color with float32 components R, G, B and A.
// A (opacity) is in [0, 1]. R, G and B are extended sRGB, like CSS Color 4:
// they are in [0, 1] for colors inside the sRGB gamut, while a color outside
// it, such as a third of the Tailwind palette once converted from OKLCH,
// keeps components below 0 or above 1 and is clipped when painted, as
// browsers do. Clamp before converting to 8-bit values. The zero value is
// transparent black.
//
// Color is an alias of an internal type: use its fields and methods
// (WithAlpha, MulAlpha, String...) and the constructors Hex, RGB, RGBA and
// OKLCH.
type Color = gfx.Color

// Common colors.
var (
	// Transparent is fully transparent black.
	Transparent = gfx.Transparent
	// Black is opaque black.
	Black = gfx.Black
	// White is opaque white.
	White = gfx.White
)

// Hex parses a CSS hex color: "#rgb", "#rgba", "#rrggbb" or "#rrggbbaa". It
// panics with the reason when s is not valid, since a literal that does not
// parse is a programmer error.
//
//	tela.Hex("#1e1e2e")
func Hex(s string) Color {
	c, err := gfx.ParseHex(s)
	if err != nil {
		panic("tela: Hex: " + err.Error())
	}
	return c
}

// RGB returns the opaque color with the given 8-bit components.
func RGB(r, g, b uint8) Color { return gfx.RGB8(r, g, b) }

// RGBA returns the color with the given 8-bit components and an alpha from
// 0 (transparent) to 1 (opaque), like CSS rgba(r, g, b, a).
func RGBA(r, g, b uint8, a float32) Color {
	return Color{R: float32(r) / 255, G: float32(g) / 255, B: float32(b) / 255, A: a}
}

// OKLCH returns the opaque color with OKLCH lightness l (0 to 1), chroma c
// (about 0 to 0.4) and hue h in degrees, the notation of the Tailwind v4
// palette: tela.OKLCH(0.637, 0.237, 25.331) is red-500. Use WithAlpha for
// transparency. The result is not gamut-mapped (see [Color]).
func OKLCH(l, c, h float32) Color { return gfx.OKLCH(l, c, h, 1) }
