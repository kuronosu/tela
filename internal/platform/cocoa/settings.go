package cocoa

import (
	"image/color"
	"math"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// rawSettings are the values AppKit reports the system settings in, as
// settings_darwin.go reads them; settingsFrom turns them into the
// platform's SystemSettings. The pure half is tested on every OS.
type rawSettings struct {
	// Dark: NSApp.effectiveAppearance's best match among Aqua and
	// DarkAqua is DarkAqua (the high-contrast variants match their base).
	Dark bool
	// Accent is NSColor.controlAccentColor converted to sRGB, components
	// in [0, 1], when HasAccent is set.
	AccentR, AccentG, AccentB float64
	HasAccent                 bool
	// ReduceMotion and IncreaseContrast are NSWorkspace's
	// accessibilityDisplayShouldReduceMotion and
	// accessibilityDisplayShouldIncreaseContrast.
	ReduceMotion, IncreaseContrast bool
	// DoubleClick is +[NSEvent doubleClickInterval], in seconds.
	DoubleClick float64
}

// settingsFrom builds the platform settings. macOS has no global text
// scale (apps scale their own text; Dynamic Type is iOS's), so TextScale
// is 1.
func settingsFrom(r rawSettings) platform.SystemSettings {
	s := platform.SystemSettings{
		DarkMode:            r.Dark,
		ReducedMotion:       r.ReduceMotion,
		HighContrast:        r.IncreaseContrast,
		DoubleClickInterval: doubleClickFrom(r.DoubleClick),
		TextScale:           1,
	}
	if r.HasAccent {
		s.AccentColor = color.NRGBA{R: channel8(r.AccentR), G: channel8(r.AccentG), B: channel8(r.AccentB), A: 0xff}
		s.HasAccentColor = true
	}
	return s
}

// channel8 converts a color component in [0, 1] to 8 bits, rounding;
// values outside the range (extended sRGB) are clamped, NaN is 0.
func channel8(v float64) uint8 {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= 1 {
		return 0xff
	}
	return uint8(math.Round(v * 255))
}

// maxDoubleClick bounds the double-click interval: System Settings' slider
// ends at 5 s, so a value above 10 s is taken as invalid (unknown).
const maxDoubleClick = 10 * time.Second

// doubleClickFrom converts NSEvent's interval in seconds to a duration
// with millisecond precision; 0 (unknown) when it is not positive or is
// out of range.
func doubleClickFrom(sec float64) time.Duration {
	if math.IsNaN(sec) || sec <= 0 {
		return 0
	}
	d := time.Duration(math.Round(sec*1000)) * time.Millisecond
	if d <= 0 || d > maxDoubleClick {
		return 0
	}
	return d
}
