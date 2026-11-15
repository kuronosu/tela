package raster

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// The synthetic font of the text tests lives in package gfxtest (font.go),
// so that the GPU tests draw the same glyphs; this file keeps the short
// names the tests use.

// Glyph IDs of testFont.
const (
	gO     = gfxtest.GlyphO
	gA     = gfxtest.GlyphA
	gS     = gfxtest.GlyphS
	gL     = gfxtest.GlyphL
	gI     = gfxtest.GlyphI
	gBadge = gfxtest.GlyphBadge
	gMedal = gfxtest.GlyphMedal
)

type testFont = gfxtest.TestFont

func newTestFont(name string) *testFont { return gfxtest.NewTestFont(name) }

func badgeList(f *testFont, fg gfx.Color) *gfx.DisplayList { return gfxtest.BadgeList(f, fg) }

// testFontA is the font of the text scenes.
var testFontA = gfxtest.FontA
