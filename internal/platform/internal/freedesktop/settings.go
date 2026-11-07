//go:build linux

package freedesktop

import (
	"context"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// System settings v2 (ADR-209) from org.freedesktop.portal.Settings,
// shared by the Wayland and X11 backends. The portal is read once, with
// ReadAll, on the first SystemSettings call (core asks before its first
// frame); SettingChanged signals keep the cache current, and a change of
// the derived platform.SystemSettings is delivered as a SettingsEvent
// through the loop.
//
// Where each value comes from (the first key present wins):
//
//   - DarkMode: org.freedesktop.appearance color-scheme (1: prefer dark;
//     0 no preference and 2 prefer light are light), else
//     org.gnome.desktop.interface color-scheme "prefer-dark".
//   - AccentColor: org.freedesktop.appearance accent-color, an (ddd) of
//     sRGB components in 0..1 (anything out of range means unset, as the
//     portal specifies), else GNOME's named org.gnome.desktop.interface
//     accent-color as libadwaita paints it (GNOME 47 and later, when the
//     portal backend does not translate it, like xdg-desktop-portal-gtk).
//   - HighContrast: org.freedesktop.appearance contrast (1: higher
//     contrast), else org.gnome.desktop.a11y.interface high-contrast.
//   - ReducedMotion: org.gnome.desktop.interface enable-animations false,
//     org.gnome.desktop.a11y.interface reduced-motion "reduce" (GNOME 49),
//     or KDE's org.kde.kdeglobals.KDE AnimationDurationFactor 0 (KDE's
//     portal sends kdeglobals values as strings).
//   - TextScale: org.gnome.desktop.interface text-scaling-factor (GNOME's
//     Large Text is 1.25), at most platform.MaxTextScale. KDE has no
//     portal key for a text scale (its font DPI lives in kcmfontsrc,
//     which the portal does not expose), so it stays unknown there.
//   - DoubleClickInterval: org.gnome.desktop.peripherals.mouse
//     double-click, else org.kde.kdeglobals.KDE DoubleClickInterval
//     (milliseconds).
//   - TitleBarButtons: org.gnome.desktop.wm.preferences button-layout,
//     as it is (GTK reads the same key through the portal for
//     gtk-decoration-layout; core parses it, ADR-238).
//
// Without a portal, or with a portal that has no Settings interface
// (xdg-desktop-portal-hyprland alone, for instance), every value stays
// zero (unknown), as before phase 13.

const ifaceSettings = "org.freedesktop.portal.Settings"

// Settings namespaces read.
const (
	nsAppearance     = "org.freedesktop.appearance"
	nsGnomeInterface = "org.gnome.desktop.interface"
	nsGnomeA11y      = "org.gnome.desktop.a11y.interface"
	nsGnomeMouse     = "org.gnome.desktop.peripherals.mouse"
	nsKDE            = "org.kde.kdeglobals.KDE"
	nsGnomeWM        = "org.gnome.desktop.wm.preferences"
)

var settingsNamespaces = []string{nsAppearance, nsGnomeInterface, nsGnomeA11y, nsGnomeMouse, nsKDE, nsGnomeWM}

// settingKey is a (namespace, key) pair of the Settings portal.
type settingKey struct{ ns, key string }

// settingsState is the settings cache of a Desktop.
type settingsState struct {
	// UI thread only.
	read bool                    // the portal was read (or found missing)
	ui   platform.SystemSettings // what SystemSettings returns: the last value delivered

	// Guarded by Desktop.mu: the signal goroutine writes them.
	raw      map[settingKey]any // the portal's values, variants unwrapped
	ready    bool               // the first read is done; changes are reported
	reported platform.SystemSettings
}

// SystemSettings returns the system settings: on the first call it reads
// the Settings portal (a blocking round trip on the UI thread, bounded by
// a timeout; without a portal every value is zero) and subscribes to its
// changes; later calls return the value of the last SettingsEvent
// delivered (or of the first read). UI thread.
func (d *Desktop) SystemSettings() platform.SystemSettings {
	if !d.settings.read {
		d.settings.read = true
		d.settings.ui = d.readSettings()
	}
	return d.settings.ui
}

// readSettings subscribes to SettingChanged, then reads every namespace
// with ReadAll. A change that arrives during the read wins over the read
// value of its key (it is newer).
func (d *Desktop) readSettings() platform.SystemSettings {
	conn, _, err := d.portal(ifaceSettings)
	if err != nil {
		return platform.SystemSettings{}
	}
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	go d.settingSignals(ch)
	// Without the match rule the changes are not followed; the values read
	// still apply.
	addMatch(conn,
		dbus.WithMatchObjectPath(portalPath),
		dbus.WithMatchInterface(ifaceSettings),
		dbus.WithMatchMember("SettingChanged"))

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var all map[string]map[string]dbus.Variant
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, ifaceSettings+".ReadAll", 0, settingsNamespaces).Store(&all); err != nil {
		all = nil // the namespaces are unknown: zero values, changes still followed
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.settings.raw == nil {
		d.settings.raw = map[settingKey]any{}
	}
	for ns, keys := range all {
		for key, v := range keys {
			k := settingKey{ns, key}
			if _, changed := d.settings.raw[k]; !changed {
				d.settings.raw[k] = unwrapVariant(v.Value())
			}
		}
	}
	s := settingsFrom(d.settings.raw)
	d.settings.ready = true
	d.settings.reported = s
	return s
}

// settingSignals handles the SettingChanged signals of the main
// connection until it closes.
func (d *Desktop) settingSignals(ch chan *dbus.Signal) {
	for s := range ch {
		if s.Name != ifaceSettings+".SettingChanged" || s.Path != portalPath {
			continue
		}
		var (
			ns, key string
			v       dbus.Variant
		)
		if dbus.Store(s.Body, &ns, &key, &v) != nil {
			continue
		}
		d.settingChanged(ns, key, unwrapVariant(v.Value()))
	}
}

// settingChanged records a changed value and, when the derived settings
// change, queues a SettingsEvent behind a callback that makes
// SystemSettings return the new value from then on (the loop runs both in
// order on the UI thread). Safe from any goroutine.
func (d *Desktop) settingChanged(ns, key string, v any) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	if d.settings.raw == nil {
		d.settings.raw = map[settingKey]any{}
	}
	d.settings.raw[settingKey{ns, key}] = v
	if !d.settings.ready {
		d.mu.Unlock()
		return
	}
	s := settingsFrom(d.settings.raw)
	if s == d.settings.reported {
		d.mu.Unlock()
		return
	}
	d.settings.reported = s
	d.inbox = append(d.inbox,
		item{fn: func() { d.settings.ui = s }},
		item{ev: platform.SettingsEvent{Settings: s}})
	d.mu.Unlock()
	d.opts.Wake()
}

// unwrapVariant removes the variants around a value: the deprecated Read
// method, and some portal backends, nest one in another.
func unwrapVariant(v any) any {
	for {
		w, ok := v.(dbus.Variant)
		if !ok {
			return v
		}
		v = w.Value()
	}
}

// settingsFrom derives the system settings from the portal's values (see
// the comment at the top of the file).
func settingsFrom(raw map[settingKey]any) platform.SystemSettings {
	var s platform.SystemSettings
	get := func(ns, key string) (any, bool) {
		v, ok := raw[settingKey{ns, key}]
		return v, ok
	}

	if v, ok := get(nsAppearance, "color-scheme"); ok {
		n, _ := number(v)
		s.DarkMode = n == 1
	} else if v, ok := get(nsGnomeInterface, "color-scheme"); ok {
		s.DarkMode = v == "prefer-dark"
	}

	if v, ok := get(nsAppearance, "accent-color"); ok {
		s.AccentColor, s.HasAccentColor = accentColor(v)
	} else if v, ok := get(nsGnomeInterface, "accent-color"); ok {
		if name, ok := v.(string); ok {
			s.AccentColor, s.HasAccentColor = adwaitaAccents[name]
		}
	}

	if v, ok := get(nsAppearance, "contrast"); ok {
		n, _ := number(v)
		s.HighContrast = n == 1
	} else if v, ok := get(nsGnomeA11y, "high-contrast"); ok {
		s.HighContrast, _ = boolean(v)
	}

	if v, ok := get(nsGnomeInterface, "enable-animations"); ok {
		if on, ok := boolean(v); ok && !on {
			s.ReducedMotion = true
		}
	}
	if v, ok := get(nsGnomeA11y, "reduced-motion"); ok && v == "reduce" {
		s.ReducedMotion = true
	}
	if v, ok := get(nsKDE, "AnimationDurationFactor"); ok {
		if f, ok := number(v); ok && f == 0 {
			s.ReducedMotion = true
		}
	}

	if v, ok := get(nsGnomeInterface, "text-scaling-factor"); ok {
		if f, ok := number(v); ok && f > 0 {
			s.TextScale = min(f, platform.MaxTextScale)
		}
	}

	ms, ok := 0.0, false
	if v, found := get(nsGnomeMouse, "double-click"); found {
		ms, ok = number(v)
	}
	if !ok || ms <= 0 {
		if v, found := get(nsKDE, "DoubleClickInterval"); found {
			ms, ok = number(v)
		}
	}
	if ok && ms > 0 && ms < 60_000 {
		s.DoubleClickInterval = time.Duration(ms) * time.Millisecond
	}

	if v, ok := get(nsGnomeWM, "button-layout"); ok {
		if layout, ok := v.(string); ok && len(layout) <= maxButtonLayout {
			s.TitleBarButtons = layout
		}
	}
	return s
}

// maxButtonLayout bounds the button-layout value taken: a real one is a
// few names long.
const maxButtonLayout = 256

// accentColor converts the portal's accent-color, (ddd) in 0..1; any
// component out of range (the portal's "unset") or a value of another
// type gives no color.
func accentColor(v any) (color.NRGBA, bool) {
	parts, ok := v.([]any)
	if !ok || len(parts) != 3 {
		return color.NRGBA{}, false
	}
	var c [3]uint8
	for i, p := range parts {
		f, ok := unwrapVariant(p).(float64)
		if !ok || !(f >= 0 && f <= 1) { // also rejects NaN
			return color.NRGBA{}, false
		}
		c[i] = uint8(math.Round(f * 255))
	}
	return color.NRGBA{c[0], c[1], c[2], 255}, true
}

// adwaitaAccents are the accent colors of GNOME's named accent-color
// setting, as libadwaita paints them (AdwAccentColor's light-scheme
// background colors).
var adwaitaAccents = map[string]color.NRGBA{
	"blue":   {0x35, 0x84, 0xe4, 0xff},
	"teal":   {0x21, 0x90, 0xa4, 0xff},
	"green":  {0x3a, 0x94, 0x4a, 0xff},
	"yellow": {0xc8, 0x88, 0x00, 0xff},
	"orange": {0xed, 0x5b, 0x00, 0xff},
	"red":    {0xe6, 0x2d, 0x42, 0xff},
	"pink":   {0xd5, 0x61, 0x99, 0xff},
	"purple": {0x91, 0x41, 0xac, 0xff},
	"slate":  {0x6f, 0x83, 0x96, 0xff},
}

// number reads a numeric setting: any D-Bus integer or double, or a
// string holding a number (KDE's portal sends kdeglobals entries as
// strings). NaN and infinities are not numbers here.
func number(v any) (float64, bool) {
	var f float64
	switch n := v.(type) {
	case uint8:
		f = float64(n)
	case int16:
		f = float64(n)
	case uint16:
		f = float64(n)
	case int32:
		f = float64(n)
	case uint32:
		f = float64(n)
	case int64:
		f = float64(n)
	case uint64:
		f = float64(n)
	case float64:
		f = n
	case string:
		var err error
		if f, err = strconv.ParseFloat(strings.TrimSpace(n), 64); err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// boolean reads a boolean setting: a D-Bus boolean or the strings "true"
// and "false" (KDE).
func boolean(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}
