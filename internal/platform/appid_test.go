package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultAppID(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	exeID := filepath.Base(exe)
	if runtime.GOOS == "windows" {
		exeID = exeID[:len(exeID)-len(filepath.Ext(exeID))]
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"/usr/bin/notes"}, "notes"},
		{[]string{"./demo", "-flag"}, "demo"},
		{[]string{"shapes"}, "shapes"},
		// The invoked name, not the symlink target: os.Args[0] is never
		// resolved.
		{[]string{"/tmp/link-to-something"}, "link-to-something"},
		// Unusable os.Args[0]: the running executable's name.
		{[]string{""}, exeID},
		{[]string{"/"}, exeID},
		{nil, exeID},
	} {
		os.Args = tc.args
		if got := DefaultAppID(); got != tc.want {
			t.Errorf("DefaultAppID() with os.Args %q = %q, want %q", tc.args, got, tc.want)
		}
	}

	if runtime.GOOS == "windows" {
		os.Args = []string{`C:\Apps\Notes.EXE`}
		if got := DefaultAppID(); got != "Notes" {
			t.Errorf("DefaultAppID() for a .exe = %q, want %q", got, "Notes")
		}
	}
}
