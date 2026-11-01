//go:build linux

package freedesktop

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

func TestSettingsFrom(t *testing.T) {
	type kv = map[settingKey]any
	for _, c := range []struct {
		name string
		raw  kv
		want platform.SystemSettings
	}{
		{"empty", kv{}, platform.SystemSettings{}},
		{"portal appearance", kv{
			{nsAppearance, "color-scheme"}: uint32(1),
			{nsAppearance, "accent-color"}: []any{0.2, 0.4, 1.0},
			{nsAppearance, "contrast"}:     uint32(1),
		}, platform.SystemSettings{DarkMode: true, AccentColor: color.NRGBA{51, 102, 255, 255}, HasAccentColor: true, HighContrast: true}},
		{"prefer light", kv{{nsAppearance, "color-scheme"}: uint32(2)}, platform.SystemSettings{}},
		{"accent out of range is unset", kv{{nsAppearance, "accent-color"}: []any{-1.0, -1.0, -1.0}}, platform.SystemSettings{}},
		{"accent NaN is unset", kv{{nsAppearance, "accent-color"}: []any{math.NaN(), 0.5, 0.5}}, platform.SystemSettings{}},
		{"accent of another type", kv{{nsAppearance, "accent-color"}: "blue"}, platform.SystemSettings{}},
		{"appearance wins over GNOME", kv{
			{nsAppearance, "color-scheme"}:     uint32(0),
			{nsGnomeInterface, "color-scheme"}: "prefer-dark",
			{nsAppearance, "accent-color"}:     []any{-1.0, 0.0, 0.0},
			{nsGnomeInterface, "accent-color"}: "blue",
			{nsAppearance, "contrast"}:         uint32(0),
			{nsGnomeA11y, "high-contrast"}:     true,
		}, platform.SystemSettings{}},
		{"GNOME without the appearance namespace", kv{
			{nsGnomeInterface, "color-scheme"}:        "prefer-dark",
			{nsGnomeInterface, "accent-color"}:        "teal",
			{nsGnomeA11y, "high-contrast"}:            true,
			{nsGnomeInterface, "enable-animations"}:   false,
			{nsGnomeInterface, "text-scaling-factor"}: 1.25,
			{nsGnomeMouse, "double-click"}:            int32(400),
		}, platform.SystemSettings{DarkMode: true, AccentColor: color.NRGBA{0x21, 0x90, 0xa4, 0xff}, HasAccentColor: true,
			HighContrast: true, ReducedMotion: true, TextScale: 1.25, DoubleClickInterval: 400 * time.Millisecond}},
		{"GNOME 49 reduced-motion", kv{
			{nsGnomeInterface, "enable-animations"}: true,
			{nsGnomeA11y, "reduced-motion"}:         "reduce",
		}, platform.SystemSettings{ReducedMotion: true}},
		{"animations on", kv{{nsGnomeInterface, "enable-animations"}: true, {nsGnomeA11y, "reduced-motion"}: "no-preference"}, platform.SystemSettings{}},
		{"KDE strings", kv{
			{nsKDE, "AnimationDurationFactor"}: "0",
			{nsKDE, "DoubleClickInterval"}:     "550",
		}, platform.SystemSettings{ReducedMotion: true, DoubleClickInterval: 550 * time.Millisecond}},
		{"KDE slower animations", kv{{nsKDE, "AnimationDurationFactor"}: "0.5"}, platform.SystemSettings{}},
		{"text scale clamped", kv{{nsGnomeInterface, "text-scaling-factor"}: 9.0}, platform.SystemSettings{TextScale: platform.MaxTextScale}},
		{"text scale invalid", kv{{nsGnomeInterface, "text-scaling-factor"}: -1.0}, platform.SystemSettings{}},
		{"text scale NaN", kv{{nsGnomeInterface, "text-scaling-factor"}: math.NaN()}, platform.SystemSettings{}},
		{"double click zero falls back to KDE", kv{
			{nsGnomeMouse, "double-click"}: int32(0),
			{nsKDE, "DoubleClickInterval"}: "300",
		}, platform.SystemSettings{DoubleClickInterval: 300 * time.Millisecond}},
		{"double click negative", kv{{nsGnomeMouse, "double-click"}: int32(-5)}, platform.SystemSettings{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := settingsFrom(c.raw)
			if got != c.want {
				t.Errorf("settingsFrom = %+v\nwant %+v", got, c.want)
			}
			platform.CheckSystemSettings(got)
		})
	}
}

func TestUnwrapVariant(t *testing.T) {
	v := dbus.MakeVariant(dbus.MakeVariant(uint32(1)))
	if got := unwrapVariant(v); got != uint32(1) {
		t.Errorf("unwrapVariant = %#v, want uint32(1)", got)
	}
}

func appearanceSettings() map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{
		nsAppearance: {
			"color-scheme": dbus.MakeVariant(uint32(1)),
			"accent-color": dbus.MakeVariant(rgb{0.2, 0.4, 0.6}),
			"contrast":     dbus.MakeVariant(uint32(0)),
		},
		nsGnomeInterface: {
			"enable-animations":   dbus.MakeVariant(true),
			"text-scaling-factor": dbus.MakeVariant(1.5),
			"icon-theme":          dbus.MakeVariant("Adwaita"),
		},
		nsGnomeMouse: {
			"double-click": dbus.MakeVariant(int32(400)),
		},
		"org.example.other": {"x": dbus.MakeVariant(true)},
	}
}

func TestSettingsPortal(t *testing.T) {
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{settings: appearanceSettings()})
	h := newHarness(t, addr)

	want := platform.SystemSettings{
		DarkMode: true, AccentColor: color.NRGBA{51, 102, 153, 255}, HasAccentColor: true,
		TextScale: 1.5, DoubleClickInterval: 400 * time.Millisecond,
	}
	if got := h.d.SystemSettings(); got != want {
		t.Fatalf("SystemSettings = %+v\nwant %+v", got, want)
	}
	if got := h.d.SystemSettings(); got != want {
		t.Fatalf("SystemSettings again = %+v", got)
	}
	p.mu.Lock()
	if p.readAlls != 1 {
		t.Errorf("ReadAll called %d times, want once", p.readAlls)
	}
	p.mu.Unlock()

	// A change that leaves the settings equal is not reported; the next
	// event is the one of the real change after it.
	p.setSetting(nsGnomeInterface, "icon-theme", "Yaru")
	p.setSetting(nsAppearance, "color-scheme", uint32(2))
	e := h.event("the color scheme change")
	se, ok := e.(platform.SettingsEvent)
	want.DarkMode = false
	if !ok || se.Settings != want {
		t.Fatalf("event = %#v, want SettingsEvent %+v", e, want)
	}
	if got := h.d.SystemSettings(); got != want {
		t.Errorf("SystemSettings after the event = %+v, want %+v", got, want)
	}

	// Values nested in variants, as some backends send them, and an
	// accent color out of range.
	p.setSetting(nsGnomeInterface, "enable-animations", dbus.MakeVariant(false))
	p.setSetting(nsAppearance, "accent-color", rgb{-1, -1, -1})
	want.ReducedMotion = true
	if se := h.event("reduced motion").(platform.SettingsEvent); se.Settings != want {
		t.Fatalf("event = %+v, want %+v", se.Settings, want)
	}
	want.AccentColor, want.HasAccentColor = color.NRGBA{}, false
	if se := h.event("no accent").(platform.SettingsEvent); se.Settings != want {
		t.Fatalf("event = %+v, want %+v", se.Settings, want)
	}
	if got := h.d.SystemSettings(); got != want {
		t.Errorf("SystemSettings = %+v, want %+v", got, want)
	}

	// After Close nothing more is delivered.
	h.d.Close()
	p.setSetting(nsAppearance, "contrast", uint32(1))
	time.Sleep(50 * time.Millisecond)
	h.d.Dispatch()
	if len(h.events) != 0 {
		t.Errorf("events after Close: %+v", h.events)
	}
}

func TestSettingsWithoutPortal(t *testing.T) {
	if s := newHarness(t, "unix:path=/nonexistent/tela-test-bus").d.SystemSettings(); s != (platform.SystemSettings{}) {
		t.Errorf("SystemSettings without a bus = %+v, want zero", s)
	}
	addr := privateBus(t)
	if s := newHarness(t, addr).d.SystemSettings(); s != (platform.SystemSettings{}) {
		t.Errorf("SystemSettings without a portal = %+v, want zero", s)
	}
	// A portal without the Settings interface (xdg-desktop-portal-hyprland
	// alone).
	startPortal13(t, addr, portal13Options{shortcuts: true})
	if s := newHarness(t, addr).d.SystemSettings(); s != (platform.SystemSettings{}) {
		t.Errorf("SystemSettings without the Settings interface = %+v, want zero", s)
	}
}

// rgb is the portal accent-color, a (ddd) struct.
type rgb struct{ R, G, B float64 }
