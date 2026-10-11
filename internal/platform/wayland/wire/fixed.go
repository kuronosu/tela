//go:build linux

package wire

import "math"

// Fixed is a signed 24.8 fixed-point number, the Wayland "fixed" type.
type Fixed int32

// FixedFromFloat converts f to the nearest representable Fixed. Values outside
// the 24.8 range saturate.
func FixedFromFloat(f float64) Fixed {
	v := math.RoundToEven(f * 256)
	switch {
	case v >= math.MaxInt32:
		return Fixed(math.MaxInt32)
	case v <= math.MinInt32:
		return Fixed(math.MinInt32)
	}
	return Fixed(int32(v))
}

// FixedFromInt converts an integer to Fixed.
func FixedFromInt(i int) Fixed { return Fixed(int32(i) << 8) }

// Float64 returns the value as a float64 (exact).
func (f Fixed) Float64() float64 { return float64(f) / 256 }

// Int returns the value truncated toward zero.
func (f Fixed) Int() int { return int(f) / 256 }
