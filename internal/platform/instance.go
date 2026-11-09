package platform

import (
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Launch is how a process of the app was started: what a second instance
// forwards to the first ([Desktop.ClaimInstance], [LaunchEvent]), and what
// the layers above give the app for its own start ([CurrentLaunch]). It is
// plain data.
type Launch struct {
	// Args are the command-line arguments after the program name (a file
	// to open, a deep link such as "notes:open?id=3").
	Args []string
	// Dir is the working directory of the process, for relative paths in
	// Args.
	Dir string
	// Token is the activation token the launcher gave the process, which
	// lets the first instance raise its window past focus stealing
	// prevention ([Window.ActivateWithToken]): XDG_ACTIVATION_TOKEN on
	// Wayland, DESKTOP_STARTUP_ID on X11; "" when there is none (a
	// terminal) and on Windows, where the second process calls
	// AllowSetForegroundWindow for the first instead.
	Token string
}

// Clone returns a copy of l whose Args the caller may modify.
func (l Launch) Clone() Launch {
	l.Args = slices.Clone(l.Args)
	return l
}

// CurrentLaunch returns the Launch of this process: os.Args[1:], the
// working directory, and the activation token from XDG_ACTIVATION_TOKEN,
// else DESKTOP_STARTUP_ID. It does not modify the environment.
func CurrentLaunch() Launch {
	l := Launch{Token: os.Getenv("XDG_ACTIVATION_TOKEN")}
	if len(os.Args) > 1 {
		l.Args = slices.Clone(os.Args[1:])
	}
	if l.Token == "" {
		l.Token = os.Getenv("DESKTOP_STARTUP_ID")
	}
	l.Dir, _ = os.Getwd()
	return l
}

// LaunchEvent is delivered, with a nil Window, when the app is asked to
// open something while it runs: to the first instance of an app that
// claimed single instance ([Desktop.ClaimInstance]) when another process of
// the app was started (LaunchForwarded), and to any app the OS hands links
// or files (LaunchOpened). The app opens what Launch.Args name and raises a
// window with Launch.Token.
type LaunchEvent struct {
	Launch Launch
	// Source says where the launch comes from.
	Source LaunchSource
	// AtStart marks a LaunchOpened that started the process: the OS
	// started the app to open these links or files (contract v11,
	// ADR-259). Only Cocoa knows (application:openURLs: before
	// applicationDidFinishLaunching:); the others always leave it false.
	AtStart bool
}

// LaunchSource says where a [LaunchEvent] comes from.
type LaunchSource uint8

const (
	// LaunchForwarded: another process of the app was started and
	// forwarded its own launch (its command line) to this one, the single
	// instance (Desktop.ClaimInstance), then exited.
	LaunchForwarded LaunchSource = iota
	// LaunchOpened: the OS handed the running app links or files to open,
	// without another process of the app: macOS Launch Services (a link
	// clicked in another app, a file opened from the Finder, the Dock or
	// "Open With", including the one that started the app, which arrives
	// after the start), and on Linux the org.freedesktop.Application
	// Activate and Open calls of the desktop's launchers.
	LaunchOpened
)

func (s LaunchSource) String() string {
	switch s {
	case LaunchForwarded:
		return "forwarded"
	case LaunchOpened:
		return "opened"
	}
	return "LaunchSource(invalid)"
}

// CheckAppID panics with an actionable message if id cannot identify an
// app to the OS services (single instance, secrets, registration): empty,
// longer than 255 bytes, or holding control characters, spaces, '/' or
// '\'. Backends derive the OS names from it (a D-Bus name, a mutex name,
// a .desktop file name, a ProgID). what names the call.
func CheckAppID(what, id string) {
	bad := id == "" || len(id) > 255 || strings.IndexFunc(id, func(r rune) bool {
		return !unicode.IsPrint(r) || unicode.IsSpace(r) || r == '/' || r == '\\'
	}) >= 0
	if bad {
		panicf("platform: %s: invalid app ID %q; use a reverse-DNS name such as \"com.example.Notes\" (no spaces, '/' or '\\')", what, id)
	}
}

// ServiceAppID returns id made valid for CheckAppID, for an app ID the
// layers above did not choose (an executable's name such as "My Notes"):
// spaces, characters that are not printable, '/' and '\' become '-', an
// empty id is "tela", and it is cut to 255 bytes (on a rune boundary). A
// valid id is returned unchanged.
func ServiceAppID(id string) string {
	id = strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) || unicode.IsSpace(r) || r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, id)
	if id == "" {
		return "tela"
	}
	for len(id) > 255 {
		_, size := utf8.DecodeLastRuneInString(id)
		id = id[:len(id)-size]
	}
	return id
}
