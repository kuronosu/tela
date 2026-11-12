//go:build linux

package freedesktop

import (
	"image/color"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// prefetchFor starts a prefetched settings read on the bus at addr, as
// PrefetchSettings does on the session bus, and forgets it at the end of
// the test if no Desktop took it.
func prefetchFor(t *testing.T, addr string) {
	t.Helper()
	prefetchSettings(addr)
	t.Cleanup(func() {
		pending.mu.Lock()
		p := pending.p
		pending.p = nil
		pending.mu.Unlock()
		if p != nil {
			p.abandon()
		}
	})
}

// portalSettings is what appearanceSettings gives SystemSettings (as in
// TestSettingsPortal).
var portalSettings = platform.SystemSettings{
	DarkMode: true, AccentColor: color.NRGBA{51, 102, 153, 255}, HasAccentColor: true,
	TextScale: 1.5, DoubleClickInterval: 400 * time.Millisecond, TitleBarButtons: "appmenu:close",
	UIFontFamily: "Noto Sans",
}

// TestPrefetchedSettings checks that a read started before the Desktop
// runs off the caller: the first SystemSettings, made after the slow
// portal answered, takes the read's values without a round trip of its
// own, a change signaled during the read wins over the value read, and
// the changes after it arrive as SettingsEvents of the Desktop.
func TestPrefetchedSettings(t *testing.T) {
	addr := privateBus(t)
	const delay = 300 * time.Millisecond
	p := startPortal13(t, addr, portal13Options{settings: appearanceSettings(), readAllDelay: delay})
	prefetchFor(t, addr)
	select {
	case <-p.readAllCalled:
	case <-time.After(10 * time.Second):
		t.Fatal("the prefetch did not call ReadAll")
	}
	// A change while the portal answers: ReadAll's answer is older.
	p.setSetting(nsAppearance, "contrast", uint32(1))
	// The app's own startup work, as long as the portal takes.
	time.Sleep(delay + 100*time.Millisecond)

	h := newHarness(t, addr)
	start := time.Now()
	got := h.d.SystemSettings()
	if took := time.Since(start); took > delay/2 {
		t.Errorf("SystemSettings took %v after the read ended; want no round trip of its own", took)
	}
	want := portalSettings
	want.HighContrast = true
	if got != want {
		t.Fatalf("SystemSettings = %+v\nwant %+v", got, want)
	}
	p.mu.Lock()
	if p.readAlls != 1 {
		t.Errorf("ReadAll called %d times, want once", p.readAlls)
	}
	p.mu.Unlock()

	p.setSetting(nsAppearance, "color-scheme", uint32(2))
	want.DarkMode = false
	if se := h.event("the color scheme change").(platform.SettingsEvent); se.Settings != want {
		t.Fatalf("event = %+v, want %+v", se.Settings, want)
	}
	if got := h.d.SystemSettings(); got != want {
		t.Errorf("SystemSettings after the event = %+v, want %+v", got, want)
	}
}

// TestPrefetchedSettingsWait checks that the first SystemSettings waits
// for a read still running: the first frame gets the real settings.
func TestPrefetchedSettingsWait(t *testing.T) {
	addr := privateBus(t)
	const delay = 200 * time.Millisecond
	startPortal13(t, addr, portal13Options{settings: appearanceSettings(), readAllDelay: delay})
	prefetchFor(t, addr)
	h := newHarness(t, addr)
	if got := h.d.SystemSettings(); got != portalSettings {
		t.Fatalf("SystemSettings = %+v\nwant %+v", got, portalSettings)
	}
	// The read's connection is the Desktop's: the portal's version is
	// known without asking again.
	if _, ok := h.d.versions[ifaceSettings]; !ok || h.d.conn == nil {
		t.Errorf("the Desktop did not take the read's connection over (conn %v, versions %v)", h.d.conn, h.d.versions)
	}
}

// TestPrefetchedSettingsLate checks the bound of that wait: a portal
// slower than settingsWait leaves the first SystemSettings the zero
// values, and what it reads later arrives as a SettingsEvent.
func TestPrefetchedSettingsLate(t *testing.T) {
	addr := privateBus(t)
	old := settingsWait
	settingsWait = 100 * time.Millisecond
	t.Cleanup(func() { settingsWait = old })
	startPortal13(t, addr, portal13Options{settings: appearanceSettings(), readAllDelay: 600 * time.Millisecond})
	prefetchFor(t, addr)
	h := newHarness(t, addr)
	start := time.Now()
	if got := h.d.SystemSettings(); got != (platform.SystemSettings{}) {
		t.Fatalf("SystemSettings before the read ended = %+v, want zero", got)
	}
	if took := time.Since(start); took > 400*time.Millisecond {
		t.Errorf("SystemSettings waited %v, want about settingsWait (100ms)", took)
	}
	if se := h.event("the late read").(platform.SettingsEvent); se.Settings != portalSettings {
		t.Fatalf("event = %+v\nwant %+v", se.Settings, portalSettings)
	}
	if got := h.d.SystemSettings(); got != portalSettings {
		t.Errorf("SystemSettings after the event = %+v", got)
	}
}

// TestPrefetchedSettingsBeforeBus checks a Desktop whose first use is
// another service (the single-instance claim runs before NewHost): it
// takes the read over there, and SystemSettings reads nothing again.
func TestPrefetchedSettingsBeforeBus(t *testing.T) {
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{settings: appearanceSettings(), readAllDelay: 100 * time.Millisecond})
	prefetchFor(t, addr)
	h := newHarness(t, addr)
	if _, err := h.d.bus(); err != nil {
		t.Fatal(err)
	}
	if h.d.pre != nil {
		t.Fatal("bus() did not take the read over")
	}
	if got := h.d.SystemSettings(); got != portalSettings {
		t.Fatalf("SystemSettings = %+v\nwant %+v", got, portalSettings)
	}
	p.mu.Lock()
	if p.readAlls != 1 {
		t.Errorf("ReadAll called %d times, want once", p.readAlls)
	}
	p.mu.Unlock()
}

// TestPrefetchedSettingsWithoutPortal checks the reads that find no bus
// or no portal: zero values, as without a prefetch, and only a Desktop on
// the read's bus takes it.
func TestPrefetchedSettingsWithoutPortal(t *testing.T) {
	const nobus = "unix:path=/nonexistent/tela-test-bus"
	prefetchFor(t, nobus)
	addr := privateBus(t)
	if h := newHarness(t, addr); h.d.pre != nil {
		t.Fatal("a Desktop on another bus took the read")
	}
	h := newHarness(t, nobus)
	if h.d.pre == nil {
		t.Fatal("the Desktop on the read's bus did not take it")
	}
	if s := h.d.SystemSettings(); s != (platform.SystemSettings{}) {
		t.Errorf("SystemSettings without a bus = %+v, want zero", s)
	}

	prefetchFor(t, addr)
	if s := newHarness(t, addr).d.SystemSettings(); s != (platform.SystemSettings{}) {
		t.Errorf("SystemSettings without a portal = %+v, want zero", s)
	}
}

// TestPrefetchedSettingsClose checks a Desktop closed while the read
// runs: nothing is delivered, and the read's connection closes when it
// ends.
func TestPrefetchedSettingsClose(t *testing.T) {
	addr := privateBus(t)
	startPortal13(t, addr, portal13Options{settings: appearanceSettings(), readAllDelay: 100 * time.Millisecond})
	prefetchFor(t, addr)
	h := newHarness(t, addr)
	p := h.d.pre
	h.d.Close()
	<-p.done
	eventually(t, "the read's connection to close", func() bool { return !p.conn.Connected() })
	h.d.Dispatch()
	if len(h.events) != 0 || len(h.posted) != 0 {
		t.Errorf("delivered after Close: %v events, %v callbacks", h.events, len(h.posted))
	}
}
