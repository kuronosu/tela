//go:build darwin

package cocoa

import (
	"fmt"
	"sync"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// settingsState is the backend's view of the system settings.
type settingsState struct {
	value    platform.SystemSettings // as last read; SettingsEvent reports changes
	observer darwin.ID               // TelaSettingsObserver (owned); 0 without change tracking
	pending  bool                    // a refresh is posted to the loop
}

// Settings returns the system settings as the backend last read them: at
// New, and again after every change notification (a SettingsEvent reports
// each change).
func (b *Backend) Settings() platform.SystemSettings { return b.settings.value }

// setSel holds the selectors of the settings code.
var setSel = struct {
	effectiveAppearance, bestMatch, isEqualToString, name           darwin.SEL
	controlAccentColor, sRGBColorSpace, colorUsingColorSpace        darwin.SEL
	redComponent, greenComponent, blueComponent                     darwin.SEL
	sharedWorkspace, reduceMotion, increaseContrast, doubleClick    darwin.SEL
	defaultCenter, notificationCenter, addObserverSelector          darwin.SEL
	removeObserver, addObserverKeyPath, removeObserverKeyPath, note darwin.SEL
}{
	effectiveAppearance:   darwin.Sel("effectiveAppearance"),
	bestMatch:             darwin.Sel("bestMatchFromAppearancesWithNames:"),
	isEqualToString:       darwin.Sel("isEqualToString:"),
	name:                  darwin.Sel("name"),
	controlAccentColor:    darwin.Sel("controlAccentColor"),
	sRGBColorSpace:        darwin.Sel("sRGBColorSpace"),
	colorUsingColorSpace:  darwin.Sel("colorUsingColorSpace:"),
	redComponent:          darwin.Sel("redComponent"),
	greenComponent:        darwin.Sel("greenComponent"),
	blueComponent:         darwin.Sel("blueComponent"),
	sharedWorkspace:       darwin.Sel("sharedWorkspace"),
	reduceMotion:          darwin.Sel("accessibilityDisplayShouldReduceMotion"),
	increaseContrast:      darwin.Sel("accessibilityDisplayShouldIncreaseContrast"),
	doubleClick:           darwin.Sel("doubleClickInterval"),
	defaultCenter:         darwin.Sel("defaultCenter"),
	notificationCenter:    darwin.Sel("notificationCenter"),
	addObserverSelector:   darwin.Sel("addObserver:selector:name:object:"),
	removeObserver:        darwin.Sel("removeObserver:"),
	addObserverKeyPath:    darwin.Sel("addObserver:forKeyPath:options:context:"),
	removeObserverKeyPath: darwin.Sel("removeObserver:forKeyPath:"),
	note:                  darwin.Sel("settingsChanged:"),
}

// settingsSetup holds what the settings code creates once per process.
var settingsSetup struct {
	once     sync.Once
	class    darwin.Class // TelaSettingsObserver; 0 if registering failed
	err      error
	darkAqua darwin.ID // NSAppearanceNameDarkAqua
	names    darwin.ID // NSArray of NSAppearanceNameAqua and DarkAqua (retained)
	keyPath  darwin.ID // "effectiveAppearance" (owned)
	// Notification names on the default center and on NSWorkspace's.
	appNotes, workspaceNotes []darwin.ID
}

// appKitName returns AppKit's NSString constant name, or a string of the
// same value (AppKit's notification and appearance names are their own
// identifiers) when the symbol is missing. The result lives for the
// process.
func appKitName(name string) darwin.ID {
	if v := darwin.AppKitString(name); v != 0 {
		return v
	}
	return darwin.NSString(name)
}

// setupSettings creates the appearance names and registers the observer
// class, once. The error is about the class only: without it the settings
// are read but changes are not tracked.
func setupSettings() error {
	s := &settingsSetup
	s.once.Do(func() {
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		s.darkAqua = appKitName("NSAppearanceNameDarkAqua")
		s.names = darwin.Retain(darwin.NSArray(appKitName("NSAppearanceNameAqua"), s.darkAqua))
		s.keyPath = darwin.NSString("effectiveAppearance")
		// The accent color: NSSystemColorsDidChangeNotification (AppKit's
		// documented signal for controlAccentColor). The accessibility
		// display options: NSWorkspace's center. The double-click interval
		// has no notification: it is read again when the app becomes active
		// (the user changes it in System Settings, another app). Dark mode
		// is KVO on NSApp.effectiveAppearance (documented as observable, and
		// what winit uses): it also follows NSApp.appearance set by the app,
		// and, unlike the distributed AppleInterfaceThemeChangedNotification,
		// it fires once the new appearance is in effect.
		s.appNotes = []darwin.ID{
			appKitName("NSSystemColorsDidChangeNotification"),
			appKitName("NSApplicationDidBecomeActiveNotification"),
		}
		s.workspaceNotes = []darwin.ID{appKitName("NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification")}
		s.class, s.err = darwin.RegisterClass("TelaSettingsObserver", darwin.GetClass("NSObject"), nil, []darwin.Method{
			{Sel: setSel.note, Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) { settingsChanged(self) }},
			{Sel: darwin.Sel("observeValueForKeyPath:ofObject:change:context:"),
				Fn: func(self darwin.ID, _ darwin.SEL, _, _, _ darwin.ID, _ uintptr) { settingsChanged(self) }},
		})
		if s.err != nil {
			s.err = fmt.Errorf("cocoa: registering TelaSettingsObserver: %w", s.err)
		}
	})
	return s.err
}

// observers maps TelaSettingsObserver instances to their backends.
var observers = struct {
	sync.Mutex
	m map[darwin.ID]*Backend
}{m: map[darwin.ID]*Backend{}}

// settingsChanged runs on the main thread for every notification and KVO
// change: it schedules one refresh.
func settingsChanged(observer darwin.ID) {
	observers.Lock()
	b := observers.m[observer]
	observers.Unlock()
	if b != nil {
		b.guard(b.scheduleSettingsRefresh)
	}
}

// startSettings reads the settings and starts tracking their changes. New
// calls it last.
func (b *Backend) startSettings() {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	err := setupSettings()
	b.settings.value = readSettings()
	if err != nil {
		return
	}
	s := &settingsSetup
	obs := darwin.Send(darwin.SendClass(s.class, sel.alloc), sel.init)
	observers.Lock()
	observers.m[obs] = b
	observers.Unlock()
	center := darwin.SendClass(darwin.GetClass("NSNotificationCenter"), setSel.defaultCenter)
	for _, n := range s.appNotes {
		darwin.Send(center, setSel.addObserverSelector, obs, setSel.note, n, darwin.ID(0))
	}
	ws := darwin.Send(darwin.SendClass(darwin.GetClass("NSWorkspace"), setSel.sharedWorkspace), setSel.notificationCenter)
	for _, n := range s.workspaceNotes {
		darwin.Send(ws, setSel.addObserverSelector, obs, setSel.note, n, darwin.ID(0))
	}
	darwin.Send(app.ns, setSel.addObserverKeyPath, obs, s.keyPath, uint(0), uintptr(0))
	b.settings.observer = obs
	// A change between New and the first pass (the app finishing its
	// launch) is reported then.
	b.scheduleSettingsRefresh()
}

// stopSettings stops tracking changes (KVO observers must be removed
// before they are freed). shutdown calls it.
func (b *Backend) stopSettings() {
	obs := b.settings.observer
	if obs == 0 {
		return
	}
	b.settings.observer = 0
	darwin.Send(app.ns, setSel.removeObserverKeyPath, obs, settingsSetup.keyPath)
	darwin.Send(darwin.SendClass(darwin.GetClass("NSNotificationCenter"), setSel.defaultCenter), setSel.removeObserver, obs)
	darwin.Send(darwin.Send(darwin.SendClass(darwin.GetClass("NSWorkspace"), setSel.sharedWorkspace), setSel.notificationCenter),
		setSel.removeObserver, obs)
	observers.Lock()
	delete(observers.m, obs)
	observers.Unlock()
	darwin.Release(obs)
}

// scheduleSettingsRefresh posts one refresh to the loop: the several
// notifications of one change (an appearance change also changes system
// colors) coalesce into one read and at most one SettingsEvent.
func (b *Backend) scheduleSettingsRefresh() {
	if b.closed || b.settings.pending {
		return
	}
	b.settings.pending = true
	b.loop.Post(func() {
		b.settings.pending = false
		b.refreshSettings()
	})
	b.notify()
}

// refreshSettings reads the settings and queues a SettingsEvent with the
// whole new value when any changed.
func (b *Backend) refreshSettings() {
	s := readSettings()
	if s == b.settings.value {
		return
	}
	b.settings.value = s
	b.enqueue(nil, platform.SettingsEvent{Settings: s})
}

// readSettings reads the system settings from AppKit (main thread).
func readSettings() platform.SystemSettings {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	var r rawSettings
	if s := &settingsSetup; s.names != 0 {
		if a := darwin.Send(app.ns, setSel.effectiveAppearance); a != 0 {
			best := darwin.Send(a, setSel.bestMatch, s.names)
			r.Dark = best != 0 && darwin.SendBool(best, setSel.isEqualToString, s.darkAqua)
		}
	}
	// controlAccentColor is a dynamic system color: converting it to sRGB
	// resolves it (macOS 10.14+; the "multicolor" setting is blue).
	if c := darwin.SendClass(darwin.GetClass("NSColor"), setSel.controlAccentColor); c != 0 {
		space := darwin.SendClass(darwin.GetClass("NSColorSpace"), setSel.sRGBColorSpace)
		if rgb := darwin.Send(c, setSel.colorUsingColorSpace, space); rgb != 0 {
			r.AccentR = darwin.SendFloat(rgb, setSel.redComponent)
			r.AccentG = darwin.SendFloat(rgb, setSel.greenComponent)
			r.AccentB = darwin.SendFloat(rgb, setSel.blueComponent)
			r.HasAccent = true
		}
	}
	if ws := darwin.SendClass(darwin.GetClass("NSWorkspace"), setSel.sharedWorkspace); ws != 0 {
		r.ReduceMotion = darwin.SendBool(ws, setSel.reduceMotion)
		r.IncreaseContrast = darwin.SendBool(ws, setSel.increaseContrast)
	}
	r.DoubleClick = darwin.SendFloat(darwin.ID(app.event), setSel.doubleClick)
	return settingsFrom(r)
}
