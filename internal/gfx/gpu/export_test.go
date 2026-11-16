package gpu

import (
	"slices"
	"testing"
)

// TestChooseExport checks the format a window's buffers get: a format
// without alpha before one with it, the window's order among them,
// explicit modifiers only, and the linear one only when nothing else is
// common.
func TestChooseExport(t *testing.T) {
	const tiledA, tiledB = 0x0300000000606013, 0x0300000000606015
	pairs := func(fourcc uint32, mods ...uint64) []DmabufFormat {
		var out []DmabufFormat
		for _, m := range mods {
			out = append(out, DmabufFormat{Fourcc: fourcc, Modifier: m})
		}
		return out
	}
	device := slices.Concat(pairs(FourccXBGR8888, ModifierLinear, tiledA, tiledB), pairs(FourccXRGB8888, ModifierLinear, tiledA, tiledB),
		pairs(FourccABGR8888, ModifierLinear, tiledA, tiledB), pairs(FourccARGB8888, ModifierLinear, tiledA, tiledB))
	for _, c := range []struct {
		name     string
		accepted []DmabufFormat
		fourcc   uint32
		mods     []uint64
	}{
		{"nothing accepted", nil, 0, nil},
		{"x11: one opaque format, tiled and implicit", pairs(FourccXRGB8888, tiledB, tiledA, ModifierInvalid), FourccXRGB8888, []uint64{tiledB, tiledA}},
		{"alpha listed first", slices.Concat(pairs(FourccARGB8888, tiledA), pairs(FourccXBGR8888, tiledA, ModifierLinear)), FourccXBGR8888, []uint64{tiledA}},
		{"the window's order among opaque formats", slices.Concat(pairs(FourccXRGB8888, tiledA), pairs(FourccXBGR8888, tiledA)), FourccXRGB8888, []uint64{tiledA}},
		{"linear alone", pairs(FourccXRGB8888, ModifierLinear, 0x1234), FourccXRGB8888, []uint64{ModifierLinear}},
		{"only alpha in common", slices.Concat(pairs(FourccXRGB8888, 0x1234), pairs(FourccARGB8888, tiledA)), FourccARGB8888, []uint64{tiledA}},
		{"implicit only", pairs(FourccXRGB8888, ModifierInvalid), 0, nil},
		{"another device's modifiers", pairs(FourccXRGB8888, 0x0100000000000002), 0, nil},
		{"a format the device lacks", pairs(0x30335258 /* XR30 */, tiledA), 0, nil},
		{"duplicates", pairs(FourccXBGR8888, tiledA, tiledA, tiledB), FourccXBGR8888, []uint64{tiledA, tiledB}},
	} {
		fourcc, mods, ok := ChooseExport(c.accepted, device)
		if ok != (c.fourcc != 0) || fourcc != c.fourcc || !slices.Equal(mods, c.mods) {
			t.Errorf("%s: ChooseExport = %#x %#x %v, want %#x %#x", c.name, fourcc, mods, ok, c.fourcc, c.mods)
		}
	}
}
