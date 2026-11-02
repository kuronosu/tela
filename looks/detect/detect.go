// Package detect picks the look of the desktop the program runs on.
//
// It is a package of its own because package looks defines the types that
// win11, adwaita and breeze import, so looks cannot import them back; detect
// sits above all four. An app that wants one fixed look imports that look's
// package and nothing else; an app that follows the platform imports detect,
// which links the three.
package detect

import (
	"os"
	"runtime"
	"strings"

	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/adwaita"
	"github.com/kuronosu/tela/looks/breeze"
	"github.com/kuronosu/tela/looks/win11"
)

// Look returns the look of the running desktop:
//
//   - Windows: Windows 11.
//   - Linux with KDE in $XDG_CURRENT_DESKTOP (it is a list separated by
//     colons, such as "KDE" or "ubuntu:GNOME"): KDE (Breeze).
//   - Linux otherwise (GNOME, and every desktop that is neither): GNOME
//     (libadwaita).
//   - macOS: GNOME, until the macOS backend adds an Aqua look; the call
//     will then return it.
//   - Any other system: GNOME.
func Look() looks.Look {
	return For(runtime.GOOS, os.Getenv("XDG_CURRENT_DESKTOP"))
}

// For is [Look] for a given operating system (a runtime.GOOS value) and a
// value of $XDG_CURRENT_DESKTOP, for tests and for an app that lets its user
// name the desktop to imitate.
func For(goos, xdgCurrentDesktop string) looks.Look {
	switch goos {
	case "windows":
		return win11.Look()
	case "linux":
		if strings.Contains(strings.ToUpper(xdgCurrentDesktop), "KDE") {
			return breeze.Look()
		}
	}
	return adwaita.Look()
}

// All returns the looks that [Look] can return, in the order Windows 11, GNOME
// and KDE: a list for a menu of looks. Each call returns new ones.
func All() []looks.Look {
	return []looks.Look{win11.Look(), adwaita.Look(), breeze.Look()}
}
