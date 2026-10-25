package aria

import "testing"

// snapValueToStep follows React Aria's: the nearest multiple of the step
// from the minimum, kept in range, without floating point noise.
func TestSnapValueToStep(t *testing.T) {
	for _, tc := range []struct {
		v, lo, hi, step, want float64
	}{
		{7, 0, 100, 5, 5},
		{7.5, 0, 100, 5, 10},
		{-3, 0, 100, 1, 0},
		{120, 0, 100, 1, 100},
		{0.30000000000000004, 0, 1, 0.1, 0.3},
		{0.16, 0, 1, 0.1, 0.2},
		{13, 3, 100, 5, 13},
		{14, 3, 100, 5, 13},
		{99, 3, 100, 5, 98}, // past the last step from the minimum: the last step
		{-6, -10, 10, 2.5, -5},
		{3.4, -10, 10, 2.5, 2.5},
		{1e-10, 0, 1, 0.01, 0},
	} {
		if got := snapValueToStep(tc.v, tc.lo, tc.hi, tc.step); got != tc.want {
			t.Errorf("snapValueToStep(%g, %g, %g, %g) = %g, want %g", tc.v, tc.lo, tc.hi, tc.step, got, tc.want)
		}
	}
}
