package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultAppID returns the application ID backends use when
// WindowOptions.AppID is empty: the base name of the executable as the user
// invoked it (os.Args[0]), falling back to the base name of os.Executable,
// and to "tela" if neither is usable. On Windows a trailing ".exe" is
// dropped.
//
// os.Args[0] is preferred over os.Executable because it is the name the user
// typed or the launcher ran, and it is not resolved through symlinks: a
// binary installed as /usr/bin/notes -> /opt/notes/notes-1.2 is "notes", the
// name its .desktop file and WM_CLASS matching expect (Xlib's default
// WM_CLASS and GTK's default program name use argv[0] the same way).
// os.Executable resolves symlinks on Linux, which would report "notes-1.2".
//
// Every backend calls this one function, so a window gets the same default
// AppID (Wayland xdg_toplevel.app_id, X11 WM_CLASS instance) everywhere.
func DefaultAppID() string {
	if len(os.Args) > 0 {
		if id := appIDFromPath(os.Args[0]); id != "" {
			return id
		}
	}
	if exe, err := os.Executable(); err == nil {
		if id := appIDFromPath(exe); id != "" {
			return id
		}
	}
	return "tela"
}

// appIDFromPath returns the base name of an executable path, or "" when the
// path has no usable base name.
func appIDFromPath(p string) string {
	if p == "" {
		return ""
	}
	base := filepath.Base(p)
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(base), ".exe") {
		base = base[:len(base)-len(".exe")]
	}
	switch base {
	case "", ".", "..", string(filepath.Separator):
		return ""
	}
	return base
}
