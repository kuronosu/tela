//go:build linux

package wayland

import (
	"image"
	"os"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Transparent windows (WindowOptions.Transparent, ADR-249). Every Wayland
// compositor composites, so a transparent window always reports
// WindowState.Transparent and is:
//
//   - presented in a format with alpha: ABGR8888 (R, G, B, A in memory,
//     image.RGBA's order, copied as is) where wl_shm lists it, else
//     ARGB8888, which every compositor has. wl_shm's ARGB formats are
//     premultiplied, as the framebuffer is. The opaque region is left
//     unset (empty), so the compositor blends the whole surface; the
//     opaque windows keep their X formats, which the compositor may skip
//     blending for.
//   - given the input region of its frames: the pixels at least
//     platform.InputAlpha opaque (platform.InputMask, updated in Present
//     from the damage), as wl_region rectangles in surface coordinates
//     (logical pixels, rounded outward like the damage of wl_surface 3).
//     The region is double-buffered state, so it is set before the commit
//     of the buffer it was computed from: input and pixels change in the
//     same frame. The compositor sends no pointer events outside it (a
//     leave when the pointer crosses its edge), and gives them to what is
//     below. Until the first Present the region is the default: the whole
//     surface, which shows nothing yet.
//   - without resize edges: its edges are usually the transparent margin
//     around the content, which takes no input.
//
// winit (with_transparent) and GTK 4 pick ARGB8888 the same way and leave
// the opaque region empty; GTK sets the input region of its CSD shadow
// margin to the window's frame, as here the mask does.

// maxInputRects bounds the rectangles of a window's input region: past it
// the region covers the mask's rows (platform.InputMask.Rects).
const maxInputRects = 512

// alphaFormat returns the wl_shm format of transparent windows.
// TELA_WAYLAND_SHM_FORMAT=xrgb8888 forces ARGB8888, as it forces XRGB8888
// for opaque windows.
func (b *Backend) alphaFormat() protocols.WlShmFormat {
	if b.shmFormats[protocols.WlShmFormatAbgr8888] && os.Getenv("TELA_WAYLAND_SHM_FORMAT") != "xrgb8888" {
		return protocols.WlShmFormatAbgr8888
	}
	return protocols.WlShmFormatArgb8888
}

// windowFormat returns the wl_shm format of w's buffers.
func (w *Window) windowFormat() protocols.WlShmFormat {
	if w.opts.Transparent {
		return w.b.alphaFormat()
	}
	return w.b.format
}

// syncInputRegion updates the input mask of a transparent window with the
// rectangles of fb just copied, and sets the surface's input region when
// it changed (or the surface's size did), before the commit that shows
// them. buffer is the physical size, surface the logical size of the
// commit.
func (w *Window) syncInputRegion(fb *image.RGBA, rects []image.Rectangle, surface image.Point) {
	if !w.opts.Transparent {
		return
	}
	if !w.input.mask.Update(fb, rects) && surface == w.input.surface {
		return
	}
	w.input.surface = surface
	region := w.b.compositor.CreateRegion()
	for _, r := range inputRegion(w.input.mask.Rects(maxInputRects), fb.Rect.Size(), surface) {
		region.Add(int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}
	w.surface.SetInputRegion(region)
	region.Destroy() // the surface keeps a copy
}

// inputRegion maps the mask's rectangles (physical pixels of a buffer of
// the given size) to surface coordinates, rounded outward, dropping the
// ones that become equal to the previous (the rows of one logical pixel).
func inputRegion(rects []image.Rectangle, buffer, surface image.Point) []image.Rectangle {
	out := make([]image.Rectangle, 0, len(rects))
	for _, r := range rects {
		s := surfaceDamage(r, buffer, surface)
		if s.Empty() || len(out) > 0 && out[len(out)-1] == s {
			continue
		}
		out = append(out, s)
	}
	return out
}

// inputState is a transparent window's input region: the mask of its
// frames and the surface size the region was last set for.
type inputState struct {
	mask    platform.InputMask
	surface image.Point
}
