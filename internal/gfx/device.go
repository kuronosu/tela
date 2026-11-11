package gfx

import "math"

// DeviceTolerance returns how far a device coordinate v (in device pixels)
// computed from float32 logical values may be from the value it stands for:
// 1/1024 px, growing with |v| to absorb the relative error of float32
// (|v|/2^20: 0.1 px at 100,000 px), capped at 1/8 px. raster.GridTolerance
// is the same bound.
func DeviceTolerance(v float64) float64 {
	return min(max(1.0/1024, math.Abs(v)/(1<<20)), 1.0/8)
}

// RoundDevice rounds a device coordinate v to whole device pixels, halves
// up, as Blink rounds a LayoutUnit (pixel-snapped box edges) and Skia a
// glyph's y; a v within DeviceTolerance below a half counts as the half.
// Blink's layout is in fixed point, so a half device pixel is exact there;
// Tela's logical positions are float32, so the same position at x1.5
// (n + 1/3 logical px) can come out a hair below the half and, rounded
// plainly, one pixel lower: a baseline drawn one row higher, a border box
// one pixel larger (ADR-273).
func RoundDevice(v float64) float64 {
	return math.Floor(v + 0.5 + DeviceTolerance(v))
}
