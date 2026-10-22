package remote

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TestWindowOptionsOverride checks HostOptions.WindowOptions (tela dev
// --size): the real window gets the rewritten options, and the child is
// configured with the real window's size.
func TestWindowOptionsOverride(t *testing.T) {
	var gotID uint32
	p := newPair(t, HostOptions{WindowOptions: func(id uint32, o platform.WindowOptions) platform.WindowOptions {
		gotID = id
		o.Size = platform.Size{W: 300, H: 200}
		return o
	}}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{Title: "sized", Size: platform.Size{W: 100, H: 80}})
	ce := a.configured()
	if want := (platform.Size{W: 300, H: 200}); ce.Metrics.Size != want {
		t.Errorf("child configured with %v, want %v", ce.Metrics.Size, want)
	}
	hw := p.window(0)
	if o := hw.Options(); o.Size != (platform.Size{W: 300, H: 200}) || o.Title != "sized" {
		t.Errorf("real window options %+v", o)
	}
	p.ui(func() {})
	if gotID != 1 {
		t.Errorf("WindowOptions got window ID %d, want 1", gotID)
	}
}

// TestWindowOptionsInvalid checks that an override returning invalid
// options fails the window like a backend error instead of panicking.
func TestWindowOptionsInvalid(t *testing.T) {
	p := newPair(t, HostOptions{WindowOptions: func(id uint32, o platform.WindowOptions) platform.WindowOptions {
		o.Size = platform.Size{W: -1, H: 5}
		return o
	}}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{})
	c := p.child(0)
	select {
	case <-c.Done():
	case <-time.After(waitTimeout):
		t.Fatal("the child was not closed after its window failed")
	}
	p.mu.Lock()
	n := len(p.errors)
	p.errors = nil // expected
	p.mu.Unlock()
	if n == 0 {
		t.Errorf("no error reported for the invalid window options")
	}
}

// TestSettingsOverride checks HostOptions.Settings (tela dev --dark): the
// welcome and every SettingsEvent carry the rewritten settings, and a change
// that the override hides is not forwarded.
func TestSettingsOverride(t *testing.T) {
	dark := func(s platform.SystemSettings) platform.SystemSettings { s.DarkMode = true; return s }
	p := newPair(t, HostOptions{Settings: dark}, headless.Options{})
	a := p.dial("a").start()
	if !a.b.Settings().DarkMode {
		t.Fatalf("welcome settings %+v, want DarkMode", a.b.Settings())
	}
	a.newWindow(platform.WindowOptions{})
	a.configured()
	a.take()

	// The system turns dark mode on and off: hidden by the override.
	p.hb.SetSettings(platform.SystemSettings{DarkMode: true})
	p.hb.SetSettings(platform.SystemSettings{DarkMode: false})
	// Reduced motion is not overridden: forwarded, with DarkMode still on.
	want := platform.SystemSettings{DarkMode: true, ReducedMotion: true}
	p.hb.SetSettings(platform.SystemSettings{ReducedMotion: true})
	var got []platform.SystemSettings
	waitFor(t, "a SettingsEvent", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		got = got[:0]
		for _, e := range a.events {
			if se, ok := e.e.(platform.SettingsEvent); ok {
				got = append(got, se.Settings)
			}
		}
		return len(got) > 0
	})
	time.Sleep(20 * time.Millisecond) // nothing else should follow
	a.mu.Lock()
	n := 0
	for _, e := range a.events {
		if _, ok := e.e.(platform.SettingsEvent); ok {
			n++
		}
	}
	a.mu.Unlock()
	if n != 1 || got[0] != want {
		t.Errorf("SettingsEvents %+v (%d in all), want one %+v", got, n, want)
	}
	if s := a.b.Settings(); s != want {
		t.Errorf("Backend.Settings() = %+v, want %+v", s, want)
	}
}
