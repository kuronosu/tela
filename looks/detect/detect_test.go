package detect_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kuronosu/tela/looks/detect"
)

func TestFor(t *testing.T) {
	for _, c := range []struct{ goos, desktop, want string }{
		{"windows", "", "Windows 11"},
		{"windows", "KDE", "Windows 11"}, // the OS wins over a stray variable
		{"linux", "KDE", "KDE"},
		{"linux", "kde", "KDE"},
		{"linux", "KDE:plasma", "KDE"},
		{"linux", "GNOME", "GNOME"},
		{"linux", "ubuntu:GNOME", "GNOME"},
		{"linux", "", "GNOME"},
		{"linux", "sway", "GNOME"},
		{"darwin", "", "GNOME"}, // until phase 14 adds Aqua
		{"darwin", "KDE", "GNOME"},
		{"freebsd", "KDE", "GNOME"},
	} {
		if got := detect.For(c.goos, c.desktop).Name; got != c.want {
			t.Errorf("For(%q, %q) = %q, want %q", c.goos, c.desktop, got, c.want)
		}
	}
}

// TestLookReadsTheEnvironment: Look is For with the running system's values.
func TestLookReadsTheEnvironment(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	got := detect.Look().Name
	if want := detect.For(goos(), "KDE").Name; got != want {
		t.Errorf("Look() = %q, want %q", got, want)
	}
}

func goos() string { return runtime.GOOS }

func TestAll(t *testing.T) {
	var names []string
	for _, l := range detect.All() {
		names = append(names, l.Name)
	}
	if got, want := fmt.Sprint(names), "[Windows 11 GNOME KDE]"; got != want {
		t.Errorf("All() names = %s, want %s", got, want)
	}
}
