package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The folders follow each OS's conventions: XDG on Linux (absolute
// variables only), %APPDATA% and %LOCALAPPDATA% on Windows, ~/Library on
// macOS.
func TestOSDirs(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	linux := env(map[string]string{"HOME": "/home/ana"})
	xdg := env(map[string]string{"HOME": "/home/ana", "XDG_CONFIG_HOME": "/cfg", "XDG_DATA_HOME": "relative/ignored", "XDG_CACHE_HOME": "/c", "XDG_STATE_HOME": "/s"})
	win := env(map[string]string{"USERPROFILE": `C:\Users\ana`, "APPDATA": `C:\Users\ana\AppData\Roaming`, "LOCALAPPDATA": `C:\Users\ana\AppData\Local`})
	mac := env(map[string]string{"HOME": "/Users/ana"})
	for _, c := range []struct {
		goos string
		env  func(string) string
		k    dirKind
		want string
	}{
		{"linux", linux, dirConfig, "/home/ana/.config/com.example.notes"},
		{"linux", linux, dirData, "/home/ana/.local/share/com.example.notes"},
		{"linux", linux, dirCache, "/home/ana/.cache/com.example.notes"},
		{"linux", linux, dirState, "/home/ana/.local/state/com.example.notes"},
		{"linux", linux, dirLogs, "/home/ana/.local/state/com.example.notes/logs"},
		{"linux", xdg, dirConfig, "/cfg/com.example.notes"},
		{"linux", xdg, dirData, "/home/ana/.local/share/com.example.notes"},
		{"linux", xdg, dirCache, "/c/com.example.notes"},
		{"linux", xdg, dirLogs, "/s/com.example.notes/logs"},
		{"freebsd", linux, dirConfig, "/home/ana/.config/com.example.notes"},
		{"windows", win, dirConfig, `C:\Users\ana\AppData\Roaming\com.example.notes`},
		{"windows", win, dirData, `C:\Users\ana\AppData\Roaming\com.example.notes`},
		{"windows", win, dirCache, `C:\Users\ana\AppData\Local\com.example.notes\cache`},
		{"windows", win, dirState, `C:\Users\ana\AppData\Local\com.example.notes`},
		{"windows", win, dirLogs, `C:\Users\ana\AppData\Local\com.example.notes\logs`},
		{"darwin", mac, dirConfig, "/Users/ana/Library/Application Support/com.example.notes"},
		{"darwin", mac, dirData, "/Users/ana/Library/Application Support/com.example.notes"},
		{"darwin", mac, dirCache, "/Users/ana/Library/Caches/com.example.notes"},
		{"darwin", mac, dirLogs, "/Users/ana/Library/Logs/com.example.notes"},
	} {
		got, err := osDir(c.goos, c.k, "com.example.notes", c.env)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %q, %v; want %q", c.goos, dirNames[c.k], got, err, c.want)
		}
	}
	if _, err := osDir("linux", dirConfig, "x", env(nil)); err == nil || !strings.Contains(err.Error(), "$HOME") {
		t.Errorf("no HOME: %v", err)
	}
	if _, err := osDir("windows", dirCache, "x", env(nil)); err == nil || !strings.Contains(err.Error(), "LOCALAPPDATA") {
		t.Errorf("no LOCALAPPDATA: %v", err)
	}
}

// DirEnv moves every folder under one directory, and the folders are
// created on demand with mode 0700.
func TestDirEnv(t *testing.T) {
	base := t.TempDir()
	t.Setenv(DirEnv, base)
	SetAppID("com.example.paths")
	t.Cleanup(func() { resetAppID() })
	for _, c := range []struct {
		f    func() (string, error)
		name string
	}{{ConfigDir, "config"}, {DataDir, "data"}, {CacheDir, "cache"}, {StateDir, "state"}, {LogDir, "logs"}} {
		dir, err := c.f()
		want := filepath.Join(base, "com.example.paths", c.name)
		if err != nil || dir != want {
			t.Errorf("%s: %q, %v; want %q", c.name, dir, err, want)
			continue
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("%s was not created: %v", dir, err)
		}
	}
	if AppID() != "com.example.paths" {
		t.Errorf("AppID() = %q", AppID())
	}
}

// SetAppID rejects names that are not one folder name.
func TestSetAppIDChecks(t *testing.T) {
	for _, id := range []string{"", "a/b", `a\b`, "..", "a:b"} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(r.(string), "reverse-DNS") {
					t.Errorf("SetAppID(%q): %v", id, r)
				}
			}()
			SetAppID(id)
		}()
	}
}

// Without SetAppID and an app, the folders are named after the
// executable.
func TestAppIDFromExecutable(t *testing.T) {
	resetAppID()
	t.Cleanup(resetAppID)
	if id := AppID(); id == "" || strings.ContainsAny(id, `/\`) {
		t.Errorf("AppID() = %q, want the executable's name", id)
	}
}

func resetAppID() {
	appID.mu.Lock()
	defer appID.mu.Unlock()
	appID.set, appID.used, appID.fromExe, appID.warned = "", "", false, false
}
