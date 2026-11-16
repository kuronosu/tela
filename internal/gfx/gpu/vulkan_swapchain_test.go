//go:build telagpu && (linux || windows)

package gpu

import (
	"testing"

	"github.com/kuronosu/tela/internal/vk"
)

// TestChooseSurfaceFormat checks the format of a window's swapchain: 8-bit
// BGRA, else RGBA, in the sRGB color space and never an sRGB format.
func TestChooseSurfaceFormat(t *testing.T) {
	f := func(format vk.Format, cs vk.ColorSpaceKHR) vk.SurfaceFormatKHR {
		return vk.SurfaceFormatKHR{Format: format, ColorSpace: cs}
	}
	const srgb = vk.ColorSpaceSrgbNonlinearKHR
	const formatB8G8R8A8Srgb vk.Format = 50 // VK_FORMAT_B8G8R8A8_SRGB
	const formatA2B10G10R10 vk.Format = 64  // VK_FORMAT_A2B10G10R10_UNORM_PACK32
	for _, c := range []struct {
		name    string
		formats []vk.SurfaceFormatKHR
		want    vk.Format
		ok      bool
	}{
		{"NVIDIA", []vk.SurfaceFormatKHR{f(vk.FormatB8G8R8A8Unorm, srgb), f(formatB8G8R8A8Srgb, srgb), f(formatA2B10G10R10, srgb)}, vk.FormatB8G8R8A8Unorm, true},
		{"sRGB format first", []vk.SurfaceFormatKHR{f(formatB8G8R8A8Srgb, srgb), f(vk.FormatR8G8B8A8Unorm, srgb), f(vk.FormatB8G8R8A8Unorm, srgb)}, vk.FormatB8G8R8A8Unorm, true},
		{"RGBA only", []vk.SurfaceFormatKHR{f(formatB8G8R8A8Srgb, srgb), f(vk.FormatR8G8B8A8Unorm, srgb)}, vk.FormatR8G8B8A8Unorm, true},
		{"another color space", []vk.SurfaceFormatKHR{f(vk.FormatB8G8R8A8Unorm, 1000104002)}, 0, false},
		{"any", []vk.SurfaceFormatKHR{f(vk.FormatUndefined, srgb)}, vk.FormatB8G8R8A8Unorm, true},
		{"none", nil, 0, false},
	} {
		got, ok := chooseSurfaceFormat(c.formats)
		if ok != c.ok || ok && (got.Format != c.want || got.ColorSpace != srgb) {
			t.Errorf("%s: %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestChooseCompositeAlpha checks the alpha mode: opaque where the surface
// has it.
func TestChooseCompositeAlpha(t *testing.T) {
	for _, c := range []struct {
		supported, want vk.CompositeAlphaFlagsKHR
		ok              bool
	}{
		{vk.CompositeAlphaOpaqueKHR | vk.CompositeAlphaPreMultipliedKHR, vk.CompositeAlphaOpaqueKHR, true},
		{vk.CompositeAlphaInheritKHR | vk.CompositeAlphaPreMultipliedKHR, vk.CompositeAlphaInheritKHR, true},
		{vk.CompositeAlphaPreMultipliedKHR, vk.CompositeAlphaPreMultipliedKHR, true},
		{vk.CompositeAlphaPostMultipliedKHR, 0, false},
	} {
		got, ok := chooseCompositeAlpha(c.supported)
		if got != c.want || ok != c.ok {
			t.Errorf("%#x: %#x %v, want %#x %v", c.supported, got, ok, c.want, c.ok)
		}
	}
}
