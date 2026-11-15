package raster

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Test scenes, shared by the golden, region and benchmark tests. They are
// defined in package gfxtest (so that the GPU tests draw the same ones); this
// file and the scenes*_test.go files keep the short names the tests use.
// Each scene has a logical size and draws with the Builder.

type scene struct {
	name string
	w, h int // logical size
	draw func(b *gfx.Builder)
}

// fromGfxtest converts shared scenes to the scene type of this package.
func fromGfxtest(ss []gfxtest.Scene) []scene {
	out := make([]scene, len(ss))
	for i, s := range ss {
		out[i] = scene{s.Name, s.W, s.H, s.Draw}
	}
	return out
}

var (
	slate   = gfxtest.Slate
	indigo  = gfxtest.Indigo
	rose    = gfxtest.Rose
	emerald = gfxtest.Emerald
	amber   = gfxtest.Amber
	sky     = gfxtest.Sky
)

func build(draw func(b *gfx.Builder)) *gfx.DisplayList { return gfxtest.Build(draw) }

func starPath(cx, cy, r float32) gfx.Path { return gfxtest.StarPath(cx, cy, r) }

var scenes = fromGfxtest(gfxtest.Scenes1)
