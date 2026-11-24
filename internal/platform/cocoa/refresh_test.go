package cocoa

import (
	"math"
	"testing"
	"time"
)

func TestRefreshFrom(t *testing.T) {
	for _, c := range []struct {
		min  float64
		fps  int
		want time.Duration
	}{
		{1.0 / 120, 120, 8333333},
		{1001.0 / 60000, 60, 16683333}, // 59.94 Hz: the exact interval wins
		{0, 60, 16666667},
		{-1, 60, 16666667},
		{math.NaN(), 60, 16666667},
		{math.Inf(1), 60, 16666667},
		{1e-9, 144, 6944444},
		{0, 0, 0},
		{0, -1, 0},
	} {
		if got := refreshFrom(c.min, c.fps); got != c.want {
			t.Errorf("refreshFrom(%v, %d) = %d, want %d", c.min, c.fps, got, c.want)
		}
	}
}
