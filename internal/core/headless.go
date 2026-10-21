package core

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// MaxRenderFrames bounds the frames of a headless render of one screen
// (Render, and the headless entry points of telatest): a tree whose every
// frame asks for another (state set by a layout effect on each build) is
// rendered as of the last of them.
const MaxRenderFrames = 8

// RenderOptions configures Render.
type RenderOptions struct {
	Config
	// Width and Height are the logical size of the viewport; zero means
	// platform.DefaultWindowSize.
	Width, Height float32
	// Scale is the display scale (device pixels per logical pixel); zero
	// means 1.
	Scale float32
}

// Render mounts root in a new Tree without a window, runs every phase for
// the given logical size and scale, and returns the display list (logical
// pixels) and the rendered image (physical pixels, sized by
// platform.PhysicalSize like a window's framebuffer). Goldens, `tela
// snapshot` and telatest use it. Without opts.Fonts only the embedded
// fonts are used, so the result is the same on every machine (ADR-008).
//
// The frame's effects run as in a window: layout effects before paint,
// the others after rasterizing. Then the tree is unmounted, running the
// cleanups. A tree that has work after the first frame (a UseBounds Value
// the frame measured, state a layout effect set) gets more frames, at most
// MaxRenderFrames in all, as a window gives it within a few vsyncs; the
// passive effects of a frame run at the start of the next one, so state
// they set is rendered only when such frames run. Without opts.IDPrefix,
// UseID strings start with "r-", so they do not depend on the trees made
// before in the process.
func Render(root Node, opts RenderOptions) (*gfx.DisplayList, *image.RGBA) {
	size := gfx.Size{W: opts.Width, H: opts.Height}
	if size.W == 0 && size.H == 0 {
		size = gfx.Size{W: platform.DefaultWindowSize.W, H: platform.DefaultWindowSize.H}
	}
	cfg := opts.Config
	if cfg.IDPrefix == "" {
		cfg.IDPrefix = "r-"
	}
	t := NewTree(cfg)
	t.SetViewport(size)
	t.SetScale(opts.Scale)
	t.Mount(root)
	dl, _ := t.Frame()
	for i := 1; i < MaxRenderFrames && t.Dirty(); i++ {
		dl, _ = t.Frame()
	}
	var img *image.RGBA
	if t.Debug() != 0 {
		// Debug overlays are drawn over the frame (debug.go).
		img = t.newImage(opts.Scale)
		var r raster.Renderer
		t.RasterizeFrame(img, &r, true)
	} else {
		img = t.Rasterize(dl, opts.Scale)
	}
	t.FlushEffects()
	t.Unmount()
	return dl, img
}

// Rasterize renders a display list of this tree (from Frame) into a new
// image of the viewport at the given scale (zero means 1), over the
// canvas color. The list snaps box edges for the tree's Scale, which
// should be the same.
func (t *Tree) Rasterize(dl *gfx.DisplayList, scale float32) *image.RGBA {
	img := t.newImage(scale)
	raster.Clear(img, image.Rectangle{}, t.CanvasColor())
	raster.Render(img, dl, raster.Options{Scale: scale})
	return img
}

// newImage returns an empty image of the viewport at the given scale
// (zero means 1).
func (t *Tree) newImage(scale float32) *image.RGBA {
	if scale == 0 {
		scale = 1
	}
	phys := platform.PhysicalSize(platform.Size{W: t.viewport.W, H: t.viewport.H}, float64(scale))
	return image.NewRGBA(image.Rect(0, 0, phys.X, phys.Y))
}
