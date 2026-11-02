//go:build darwin

package cocoa

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// appearanceName returns the name of NSApp.effectiveAppearance.
func appearanceName() string {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	return darwin.GoString(darwin.Send(darwin.Send(app.ns, setSel.effectiveAppearance), setSel.name))
}

// setAppAppearance sets NSApp.appearance to the named appearance, or back
// to the system's with "".
func setAppAppearance(name string) {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	var a darwin.ID
	if name != "" {
		a = darwin.SendClass(darwin.GetClass("NSAppearance"), darwin.Sel("appearanceNamed:"), appKitName(name))
	}
	darwin.Send(app.ns, darwin.Sel("setAppearance:"), a)
}

// TestSettingsRead logs the settings read on this Mac and checks what
// every Mac reports.
func TestSettingsRead(t *testing.T) {
	b := newBackend(t)
	var s platform.SystemSettings
	var name string
	onMain(func() {
		s = b.Settings()
		name = appearanceName()
		b.Quit()
		_ = b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {})) // shuts down: removes the observer
	})
	t.Logf("effective appearance %q", name)
	t.Logf("settings: dark %v, accent %v (has %v), reduced motion %v, high contrast %v, double click %v, text scale %v",
		s.DarkMode, s.AccentColor, s.HasAccentColor, s.ReducedMotion, s.HighContrast, s.DoubleClickInterval, s.TextScale)
	platform.CheckSystemSettings(s)
	if !s.HasAccentColor || s.AccentColor.A != 0xff {
		t.Errorf("no opaque accent color (%v, %v): NSColor.controlAccentColor exists on every supported macOS", s.AccentColor, s.HasAccentColor)
	}
	if s.DoubleClickInterval <= 0 {
		t.Errorf("DoubleClickInterval = %v, want +[NSEvent doubleClickInterval]", s.DoubleClickInterval)
	}
	if s.TextScale != 1 {
		t.Errorf("TextScale = %v, want 1 (macOS has no system text scale)", s.TextScale)
	}
}

// TestSettingsChangeEvent changes the app's appearance (NSApp.appearance,
// which NSApp.effectiveAppearance follows) to the opposite of the
// system's and back: each change must produce exactly one SettingsEvent
// with the new DarkMode, and Settings must agree with it.
func TestSettingsChangeEvent(t *testing.T) {
	b := newBackend(t)
	defer onMain(func() { setAppAppearance("") })
	const (
		waiting  = iota // before the script
		toTarget        // appearance set to the opposite
		back            // appearance set back to the system's
		quiet           // back again: checking nothing else arrives
	)
	phase := waiting
	var original platform.SystemSettings
	var events, early []platform.SystemSettings
	var quit func()
	// watchdog wakes the loop after d; the wake counts only if no later
	// watchdog started meanwhile (the phase did not move on).
	gen := int64(0)
	var fired atomic.Int64
	watchdog := func(d time.Duration) {
		gen++
		g := gen
		time.AfterFunc(d, func() {
			fired.Store(g)
			b.Wake()
		})
	}
	runScript(t, b, func(q func()) {
		quit = q
		original = b.Settings()
		target := "NSAppearanceNameDarkAqua"
		if original.DarkMode {
			target = "NSAppearanceNameAqua"
		}
		t.Logf("system appearance %q (dark %v): setting NSApp.appearance to %s", appearanceName(), original.DarkMode, target)
		phase = toTarget
		watchdog(5 * time.Second)
		setAppAppearance(target)
	}, func(e platform.Event) {
		switch e := e.(type) {
		case platform.SettingsEvent:
			s := e.Settings
			t.Logf("SettingsEvent in phase %d: dark %v, accent %v, reduced motion %v, high contrast %v, double click %v (appearance %q)",
				phase, s.DarkMode, s.AccentColor, s.ReducedMotion, s.HighContrast, s.DoubleClickInterval, appearanceName())
			if b.Settings() != s {
				t.Errorf("Settings() = %+v during a SettingsEvent with %+v", b.Settings(), s)
			}
			switch phase {
			case waiting:
				early = append(early, s)
			case toTarget:
				events = append(events, s)
				if s.DarkMode == original.DarkMode {
					t.Errorf("SettingsEvent after the appearance change has DarkMode %v, want %v", s.DarkMode, !original.DarkMode)
					return
				}
				phase = back
				watchdog(5 * time.Second)
				setAppAppearance("")
			case back:
				events = append(events, s)
				if s.DarkMode != original.DarkMode {
					t.Errorf("SettingsEvent after restoring the appearance has DarkMode %v, want %v", s.DarkMode, original.DarkMode)
					return
				}
				phase = quiet
				watchdog(300 * time.Millisecond)
			case quiet:
				events = append(events, s)
				t.Errorf("an extra SettingsEvent arrived after the two changes: %+v", s)
			}
		case platform.WakeEvent:
			if fired.Load() != gen {
				return // a watchdog of an earlier phase
			}
			switch phase {
			case toTarget, back:
				fresh := readSettings()
				t.Errorf("no SettingsEvent 5 s after changing the appearance (phase %d): effective appearance %q, a fresh read says dark %v, Settings() says %v",
					phase, appearanceName(), fresh.DarkMode, b.Settings().DarkMode)
				quit()
			case quiet:
				quit()
			}
		}
	})
	if len(early) > 0 {
		t.Logf("%d SettingsEvent(s) before the script (the app finishing its launch): %+v", len(early), early)
	}
	if phase != quiet {
		t.Errorf("ended in phase %d with %d SettingsEvent(s); want one per change", phase, len(events))
	}
	if len(events) != 2 {
		t.Errorf("got %d SettingsEvents for two appearance changes, want 2: %+v", len(events), events)
	}
}
