package platform

import (
	"math"
	"testing"
	"time"
)

func TestRefreshInterval(t *testing.T) {
	for _, c := range []struct {
		hz   float64
		want time.Duration
	}{
		{60, 16666667}, {59.951, 16680289}, {144, 6944444}, {1, time.Second}, {10000, 100 * time.Microsecond},
		{0, 0}, {-60, 0}, {0.5, 0}, {10001, 0}, {math.NaN(), 0}, {math.Inf(1), 0}, {math.Inf(-1), 0},
	} {
		if got := RefreshInterval(c.hz); got != c.want {
			t.Errorf("RefreshInterval(%v) = %d, want %d", c.hz, got, c.want)
		}
	}
}

func TestCheckFrameEvent(t *testing.T) {
	for _, c := range []struct {
		e  FrameEvent
		ok bool
	}{
		{FrameEvent{}, true},
		{FrameEvent{Time: time.Second, Refresh: time.Second / 60}, true},
		{FrameEvent{Refresh: time.Second}, true},
		{FrameEvent{Refresh: 100 * time.Microsecond}, true},
		{FrameEvent{Refresh: -1}, false},
		{FrameEvent{Refresh: 1}, false},
		{FrameEvent{Refresh: time.Second + 1}, false},
		{FrameEvent{Time: -1}, false},
	} {
		if got := CheckFrameEvent(c.e); (got == "") != c.ok {
			t.Errorf("CheckFrameEvent(%+v) = %q, want ok %v", c.e, got, c.ok)
		}
	}
}
