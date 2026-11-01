package platform

import (
	"os"
	"slices"
	"strings"
	"unicode"
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

// LaunchEvent is delivered, with a nil Window, to the first instance of an
// app that claimed single instance ([Desktop.ClaimInstance]) when another
// process of the app was started: the app opens what Launch.Args name and
// raises a window with Launch.Token.
type LaunchEvent struct {
	Launch Launch
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
