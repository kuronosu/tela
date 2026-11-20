package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFilmPath(t *testing.T) {
	mod := t.TempDir()
	for in, want := range map[string]string{
		"motion.png":                filepath.Join(mod, "motion.png"),
		"out/../out/a.PNG":          filepath.Join(mod, "out", "a.PNG"),
		filepath.Join(mod, "x.png"): filepath.Join(mod, "x.png"),
	} {
		got, err := filmPath(mod, in)
		if err != nil || got != want {
			t.Errorf("filmPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, want := range map[string]string{
		"../x.png":                        "inside the module",
		filepath.Join(mod, "..", "x.png"): "inside the module",
		"motion.gif":                      "is a .png file",
		"motion":                          "is a .png file",
	} {
		if _, err := filmPath(mod, in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("filmPath(%q): error = %v, want %q", in, err, want)
		}
	}
}
