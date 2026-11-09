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
//     around the content, which takes no input. The app's resize handles
//     (HitResize… regions, h.WindowResize) are in the input region too
//     while the window can be resized (contract v12, ADR-261), over
//     transparent pixels as well: the outside of a rounded card's corner
//     resizes as its inside does. SetHitRegions sets the region again and
//     commits it at once when they change (a commit without a buffer keeps
//     the shown one), so a handle that came without new pixels takes
//     input too.
//   - given a background blur where the compositor offers one
//     (ext_background_effect_manager_v1 with the blur capability) and the
//     window's backdrop is a material (Mica, Mica Alt, Acrylic;
//     Backdrop.ClientBackdrop): set_blur_region with the input mask's
//     rectangles (not the handles), double-buffered like the input
//     region, so the blur follows the card the way the Cocoa backend's
//     masked vibrancy does (ADR-257). A compositor without it, or that
//     withdraws the capability, shows no backdrop. KDE's older
//     org_kde_kwin_blur is not used: its protocol XML
//     (plasma-wayland-protocols) is LGPL-2.1-or-later, which Tela does not
//     vendor or derive code from (ARCHITECTURE section 2, ADR-261).
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
// rectangles of fb just copied, and sets the surface's input region (and
// blur region) when it changed, the surface's size did or the resize
// handles did, before the commit that shows them. buffer is the physical
// size, surface the logical size of the commit.
func (w *Window) syncInputRegion(fb *image.RGBA, rects []image.Rectangle, surface image.Point) {
	if !w.opts.Transparent {
		return
	}
	in := &w.input
	maskChanged := in.mask.Update(fb, rects)
	handles := w.handleRects(surface)
	if !maskChanged && surface == in.surface && platform.SameRects(handles, in.handles) && !in.blurDirty {
		return
	}
	if maskChanged || surface != in.surface {
		in.rects = inputRegion(in.mask.Rects(maxInputRects), fb.Rect.Size(), surface)
		in.blurDirty = true
	}
	in.surface, in.handles = surface, handles
	w.setInputRegion()
	if in.blurDirty {
		w.syncBlur()
	}
}

// handleRects returns the app's resize handles in surface coordinates of
// the given size, nil while the window cannot be resized (FixedSize,
// maximized, fullscreen).
func (w *Window) handleRects(surface image.Point) []image.Rectangle {
	if w.opts.FixedSize || w.states.maximized || w.states.fullscreen {
		return nil
	}
	return platform.HandleRects(w.ctl.regions, 1, surface)
}

// setInputRegion sets the surface's pending input region: the mask's
// rectangles and the resize handles.
func (w *Window) setInputRegion() {
	region := w.b.compositor.CreateRegion()
	for _, rs := range [][]image.Rectangle{w.input.rects, w.input.handles} {
		for _, r := range rs {
			region.Add(int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
		}
	}
	w.surface.SetInputRegion(region)
	region.Destroy() // the surface keeps a copy
}

// syncHandles sets the input region again after the hit regions changed
// (SetHitRegions), with a commit of its own, when the handles in it did.
// Before the first buffer, or while the window is hidden, the next
// Present does it.
func (w *Window) syncHandles() {
	if !w.opts.Transparent || !w.mapped || w.toplevel == nil {
		return
	}
	handles := w.handleRects(w.input.surface)
	if platform.SameRects(handles, w.input.handles) {
		return
	}
	w.input.handles = handles
	w.setInputRegion()
	w.surface.Commit()
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
// frames, its rectangles and the resize handles in surface coordinates,
// and the surface size the region was last set for; and its background
// blur: the backdrop asked for, the surface's effect object (made at the
// first blur) and whether a blur region is set.
type inputState struct {
	mask     platform.InputMask
	rects    []image.Rectangle
	handles  []image.Rectangle
	surface  image.Point
	backdrop platform.Backdrop
	effect   *protocols.ExtBackgroundEffectSurfaceV1
	blurred  bool
	// blurDirty: the blur region must be set again at the next commit.
	blurDirty bool
}

// --- background blur (ext-background-effect-v1) ---

// backgroundEffectVersion is the ext_background_effect_manager_v1 version
// bound.
const backgroundEffectVersion = 1

// bindBackgroundEffect binds ext_background_effect_manager_v1 and follows
// its capabilities.
func (b *Backend) bindBackgroundEffect(name, version uint32) {
	if b.backgroundEffect != nil {
		return
	}
	b.backgroundEffect = protocols.WlRegistryBind[protocols.ExtBackgroundEffectManagerV1](b.registry, name, min(version, backgroundEffectVersion))
	b.backgroundEffect.SetCapabilitiesHandler(func(e protocols.ExtBackgroundEffectManagerV1CapabilitiesEvent) {
		on := protocols.ExtBackgroundEffectManagerV1Capability(e.Flags).Has(protocols.ExtBackgroundEffectManagerV1CapabilityBlur)
		if on == b.canBlur {
			return
		}
		b.canBlur = on
		for _, w := range b.loop.Windows() {
			w.refreshBlur()
		}
	})
}

// wantsBlur reports whether the window's background is blurred: a
// transparent window whose backdrop is a material, on a compositor that
// blurs.
func (w *Window) wantsBlur() bool {
	return w.opts.Transparent && w.input.backdrop.ClientBackdrop() && w.b.backgroundEffect != nil && w.b.canBlur
}

// syncBlur sets the pending blur region from the input mask's rectangles
// (none when the window is not blurred or the mask is empty), before a
// commit.
func (w *Window) syncBlur() {
	in := &w.input
	in.blurDirty = false
	if !w.wantsBlur() || len(in.rects) == 0 {
		if in.blurred {
			in.effect.SetBlurRegion(nil)
			in.blurred = false
		}
		return
	}
	if in.effect == nil {
		in.effect = w.b.backgroundEffect.GetBackgroundEffect(w.surface)
	}
	region := w.b.compositor.CreateRegion()
	for _, r := range in.rects {
		region.Add(int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}
	in.effect.SetBlurRegion(region)
	region.Destroy()
	in.blurred = true
}

// refreshBlur applies a change of the backdrop or of the compositor's
// capability: at once with a commit of its own on a shown window, else at
// the next Present.
func (w *Window) refreshBlur() {
	if !w.opts.Transparent || w.Closed() {
		return
	}
	w.input.blurDirty = true
	if w.mapped && w.toplevel != nil {
		w.syncBlur()
		w.surface.Commit()
	}
}

// releaseBlur destroys the effect object, before its surface.
func (w *Window) releaseBlur() {
	if w.input.effect != nil {
		w.input.effect.Destroy()
		w.input.effect = nil
	}
}
