//go:build telagpu && (linux || windows)

package gpu

import "github.com/kuronosu/tela/internal/vk"

// The choices of a swapchain on a window (swapchain.go), apart from the
// Windows code that makes it so that they are tested everywhere.

// chooseSurfaceFormat picks the format of a swapchain's images among the
// surface's: 8-bit BGRA or RGBA without an sRGB format, read as sRGB, since
// a target holds sRGB-encoded values already and the presentation engine
// must not encode them again (ADR-007). BGRA first: it is the format every
// Windows driver lists, and DWM's own; RGBA, the target's, would be copied
// without a blit. A single VK_FORMAT_UNDEFINED means any format. ok is
// false when the surface has neither.
func chooseSurfaceFormat(formats []vk.SurfaceFormatKHR) (f vk.SurfaceFormatKHR, ok bool) {
	if len(formats) == 1 && formats[0].Format == vk.FormatUndefined {
		return vk.SurfaceFormatKHR{Format: vk.FormatB8G8R8A8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKHR}, true
	}
	for _, want := range []vk.Format{vk.FormatB8G8R8A8Unorm, vk.FormatR8G8B8A8Unorm} {
		for _, f := range formats {
			if f.Format == want && f.ColorSpace == vk.ColorSpaceSrgbNonlinearKHR {
				return f, true
			}
		}
	}
	return vk.SurfaceFormatKHR{}, false
}

// chooseCompositeAlpha picks how the presentation engine composites the
// images' alpha: not at all (opaque), else as the window system does
// (inherit), else premultiplied, which the frames of an opaque window are
// too (their alpha is 255). ok is false when the surface supports none of
// them.
func chooseCompositeAlpha(supported vk.CompositeAlphaFlagsKHR) (vk.CompositeAlphaFlagsKHR, bool) {
	for _, a := range []vk.CompositeAlphaFlagsKHR{vk.CompositeAlphaOpaqueKHR, vk.CompositeAlphaInheritKHR, vk.CompositeAlphaPreMultipliedKHR} {
		if supported&a != 0 {
			return a, true
		}
	}
	return 0, false
}
