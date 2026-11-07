package win32

import "testing"

func TestMenuAppMode(t *testing.T) {
	for _, c := range []struct {
		dark, hc bool
		want     appMode
	}{
		{true, false, appModeForceDark},
		{false, false, appModeForceLight},
		{true, true, appModeDefault},
		{false, true, appModeDefault},
	} {
		if got := menuAppMode(c.dark, c.hc); got != c.want {
			t.Errorf("menuAppMode(dark=%v, hc=%v) = %d, want %d", c.dark, c.hc, got, c.want)
		}
	}
}

func TestMenuThemeSupported(t *testing.T) {
	for build, want := range map[uint32]bool{17763: false, 18362: true, 19045: true, 26200: true, 0: false} {
		if got := menuThemeSupported(build); got != want {
			t.Errorf("menuThemeSupported(%d) = %v, want %v", build, got, want)
		}
	}
}
