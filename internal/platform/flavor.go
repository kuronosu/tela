package platform

import "runtime"

// Flavor is a platform's conventions for keys and text editing (ADR-221):
// which modifier is the primary one of shortcuts (Command on a Mac,
// Control elsewhere), the key bindings of text fields and their word
// rules. It is separate from the backend that runs, so the conventions of
// any platform can be tested on any OS: a Tree takes its flavor from
// core.Config.Flavor and telatest.PlatformKeys sets it.
//
// The zero value, FlavorHost, means the flavor of the OS the program runs
// on ([HostFlavor]); Resolve turns it into one of the others.
type Flavor uint8

// The flavors.
const (
	// FlavorHost is the flavor of the OS the program runs on.
	FlavorHost Flavor = iota
	// FlavorLinux is the conventions of Linux and the BSDs (GTK's and
	// Chromium's on Linux): Control shortcuts.
	FlavorLinux
	// FlavorWindows is Windows' conventions: Control shortcuts, Windows'
	// word rules.
	FlavorWindows
	// FlavorMac is macOS's conventions: Command shortcuts, Option by word.
	FlavorMac
)

// hostFlavor is FlavorOf(runtime.GOOS), computed once.
var hostFlavor = FlavorOf(runtime.GOOS)

// HostFlavor returns the flavor of the OS the program runs on: FlavorMac
// on darwin, FlavorWindows on windows, FlavorLinux anywhere else. It is the
// only place the key and editing conventions read runtime.GOOS.
func HostFlavor() Flavor { return hostFlavor }

// FlavorOf returns the flavor of the OS named by goos, a runtime.GOOS
// value: FlavorMac for "darwin", FlavorWindows for "windows", FlavorLinux
// for anything else.
func FlavorOf(goos string) Flavor {
	switch goos {
	case "darwin":
		return FlavorMac
	case "windows":
		return FlavorWindows
	}
	return FlavorLinux
}

// Resolve returns f, or [HostFlavor] for FlavorHost (and for an unknown
// value).
func (f Flavor) Resolve() Flavor {
	switch f {
	case FlavorLinux, FlavorWindows, FlavorMac:
		return f
	}
	return HostFlavor()
}

// CmdOrCtrl reports whether the primary modifier of shortcuts is held:
// Super (Command) under FlavorMac, Ctrl under the others (FlavorHost is
// resolved first). It tests that one modifier, as React Aria's
// isCtrlKeyPressed and Playwright's ControlOrMeta do.
func (f Flavor) CmdOrCtrl(ctrl, meta bool) bool {
	if f.Resolve() == FlavorMac {
		return meta
	}
	return ctrl
}

// String returns "host", "linux", "windows" or "mac".
func (f Flavor) String() string {
	switch f {
	case FlavorHost:
		return "host"
	case FlavorLinux:
		return "linux"
	case FlavorWindows:
		return "windows"
	case FlavorMac:
		return "mac"
	}
	return "?"
}
