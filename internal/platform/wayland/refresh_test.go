//go:build linux

package wayland

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

const (
	hz60  = 16666667 * time.Nanosecond
	hz144 = 6944444 * time.Nanosecond
)

func TestRefreshFor(t *testing.T) {
	out := func(mhz int32) *output { return &output{outputProps: outputProps{scale: 1, refresh: mhz}} }
	for _, c := range []struct {
		name    string
		entered []*output
		want    time.Duration
	}{
		{"no output", nil, 0},
		{"one", []*output{out(60000)}, hz60},
		{"mHz", []*output{out(59951)}, 16680289},
		{"the fastest", []*output{out(60000), out(144000), out(75000)}, hz144},
		{"unknown", []*output{out(0)}, 0},
		{"one unknown", []*output{out(0), out(60000)}, hz60},
		{"negative", []*output{out(-60000)}, 0},
		{"absurd", []*output{out(1)}, 0},
	} {
		if got := refreshFor(c.entered); got != c.want {
			t.Errorf("%s: refreshFor = %v, want %v", c.name, got, c.want)
		}
	}
}

// refreshHarness is a harness whose only output has wl_output version ver,
// with one window that is on no output yet. frame requests a frame and
// returns its Refresh.
func refreshHarness(t *testing.T, ver uint32) (h *harness, surface, output uint32, frame func() time.Duration) {
	t.Helper()
	g := standardGlobals(1)
	for i := range g {
		if g[i].iface == "wl_output" {
			g[i].version = ver
			if ver < 2 {
				g[i].onBind = nil // no scale, no done
			}
		}
	}
	h = newHarness(t, g)
	w := openWindow(t, h, platform.WindowOptions{}, 100, 50, 1)
	expect[platform.ConfigureEvent](h)
	if got := expect[platform.FrameEvent](h).Refresh; got != 0 {
		t.Errorf("Refresh = %v on no output, want 0", got)
	}
	frame = func() time.Duration {
		t.Helper()
		h.ui(func() {}) // the events sent so far were dispatched, if they arrived
		h.ui(w.RequestFrame)
		return expect[platform.FrameEvent](h).Refresh
	}
	return h, h.fc.id("wl_surface"), h.fc.id("wl_output"), frame
}

// waitRefresh draws frames until one has the Refresh want.
func waitRefresh(t *testing.T, what string, frame func() time.Duration, want time.Duration) {
	t.Helper()
	var got time.Duration
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if got = frame(); got == want {
			return
		}
	}
	t.Errorf("%s: Refresh = %v, want %v", what, got, want)
}

// Events of wl_output: geometry 0, mode 1 (flags, width, height, refresh),
// done 2, scale 3; of wl_surface: enter 0, leave 1.
const modeCurrent, modePreferred = uint32(1), uint32(2)

func TestOutputRefresh(t *testing.T) {
	h, surface, output, frame := refreshHarness(t, 4)
	h.fc.send(output, 1, modeCurrent|modePreferred, int32(1920), int32(1080), int32(60000))
	h.fc.send(output, 1, modePreferred, int32(1920), int32(1080), int32(240000)) // not current
	h.fc.send(output, 2)
	h.fc.send(surface, 0, output) // enter
	waitRefresh(t, "entered", frame, hz60)

	// A mode change counts from done on: the scale sent after the mode
	// shows that the mode was read.
	h.fc.send(output, 1, modeCurrent, int32(1920), int32(1080), int32(144000))
	h.fc.sendRaw(event(surface, 2, int32(2))) // preferred_buffer_scale 2
	if ce := expect[platform.ConfigureEvent](h); ce.Scale != 2 {
		t.Errorf("scale %v, want 2", ce.Scale)
	}
	if got := expect[platform.FrameEvent](h).Refresh; got != hz60 {
		t.Errorf("Refresh = %v before done, want %v", got, hz60)
	}
	h.fc.send(output, 2)
	waitRefresh(t, "mode changed", frame, hz144)

	h.fc.send(surface, 1, output) // leave
	waitRefresh(t, "left", frame, 0)
	h.fc.send(surface, 0, output)
	waitRefresh(t, "entered again", frame, hz144)

	// The output is unplugged.
	for i, g := range h.fc.globals {
		if g.iface == "wl_output" {
			h.fc.send(h.fc.id("wl_registry"), 1, uint32(i+1)) // global_remove
		}
	}
	waitRefresh(t, "removed", frame, 0)
}

// Version 1 has no done: a mode applies at once.
func TestOutputRefreshVersion1(t *testing.T) {
	h, surface, output, frame := refreshHarness(t, 1)
	h.fc.send(surface, 0, output)
	h.fc.send(output, 1, modeCurrent, int32(1920), int32(1080), int32(144000))
	waitRefresh(t, "mode", frame, hz144)
	h.fc.send(output, 1, modeCurrent, int32(1920), int32(1080), int32(0))
	waitRefresh(t, "a mode without a rate", frame, 0)
}
