package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// DirEnv is the environment variable that moves every app directory under
// one folder, for tests and portable installs: with
// TELA_DESKTOP_DIR=/tmp/x, [ConfigDir] is /tmp/x/<app ID>/config, and
// likewise data, cache, state and logs. telatest.TempDesktopDirs sets it
// to a temporary folder for one test.
const DirEnv = "TELA_DESKTOP_DIR"

// The app ID that names the directories.
var appID struct {
	mu  sync.Mutex
	set string // SetAppID's
	// used is the ID the directories were last resolved with, and
	// fromExe whether it came from the executable's name.
	used    string
	fromExe bool
	warned  bool
}

// SetAppID sets the name of the app's directories and settings
// ([ConfigDir] and the others): use the reverse-DNS name given to
// tela.App.AppID ("com.example.notes"). Call it at the start of main,
// before anything reads a directory or a setting; [PersistApp] calls it
// with the App's AppID. Without it, the directories are named after the
// running app's App.AppID, or before tela.Run (and without an AppID)
// after the executable. Safe from any goroutine.
func SetAppID(id string) {
	if id == "" || strings.ContainsAny(id, `/\:*?"<>|`) || id == "." || id == ".." {
		panic(fmt.Sprintf("desktop.SetAppID(%q): the ID names folders, so it must be non-empty, without path separators or the characters :*?\"<>|; use a reverse-DNS name such as \"com.example.notes\"", id))
	}
	appID.mu.Lock()
	defer appID.mu.Unlock()
	appID.set = id
}

// AppID returns the name of the app's directories: [SetAppID]'s, else the
// running app's tela.App.AppID, else the executable's name (as the
// platform's default AppID). Safe from any goroutine.
func AppID() string {
	id, _ := resolveAppID()
	return id
}

// resolveAppID returns the ID and a warning to give when the name changed
// since the directories were first resolved (an AppID that only the
// running app has, after a directory was read before tela.Run).
func resolveAppID() (id, warning string) {
	appID.mu.Lock()
	defer appID.mu.Unlock()
	fromExe := false
	switch {
	case appID.set != "":
		id = appID.set
	default:
		var ok bool
		if id, ok = core.RunningAppID(); !ok || strings.ContainsAny(id, `/\:*?"<>|`) {
			id, fromExe = platform.DefaultAppID(), true
		}
	}
	if appID.used != "" && appID.used != id && appID.fromExe && !appID.warned {
		appID.warned = true
		warning = fmt.Sprintf("desktop: the app's directories were named %q (the executable) before tela.Run, and are now named %q (App.AppID): settings and window placements saved under one name are not found under the other. Call desktop.SetAppID(%q) at the start of main", appID.used, id, id)
	}
	appID.used, appID.fromExe = id, fromExe
	return id, warning
}

// dirKind is one of the app's directories.
type dirKind uint8

const (
	dirConfig dirKind = iota
	dirData
	dirCache
	dirState
	dirLogs
)

var dirNames = [...]string{"config", "data", "cache", "state", "logs"}

// ConfigDir returns the app's folder for configuration and settings,
// creating it if needed:
//
//   - Linux: $XDG_CONFIG_HOME/<app ID> (~/.config/<app ID>)
//   - Windows: %APPDATA%\<app ID> (roaming)
//   - macOS: ~/Library/Application Support/<app ID>
//
// The app ID is [AppID]'s. [DirEnv] moves every directory for tests.
func ConfigDir() (string, error) { return appDir(dirConfig) }

// DataDir returns the app's folder for documents and data the user would
// miss (a library, a database), creating it if needed:
//
//   - Linux: $XDG_DATA_HOME/<app ID> (~/.local/share/<app ID>)
//   - Windows: %APPDATA%\<app ID> (roaming; the same as ConfigDir)
//   - macOS: ~/Library/Application Support/<app ID> (the same as ConfigDir)
func DataDir() (string, error) { return appDir(dirData) }

// CacheDir returns the app's folder for data it can download or compute
// again (thumbnails, downloads), creating it if needed. The OS or the
// user may empty it:
//
//   - Linux: $XDG_CACHE_HOME/<app ID> (~/.cache/<app ID>)
//   - Windows: %LOCALAPPDATA%\<app ID>\cache
//   - macOS: ~/Library/Caches/<app ID>
func CacheDir() (string, error) { return appDir(dirCache) }

// StateDir returns the app's folder for state worth keeping across runs
// but not worth backing up (window placements, recent files, history),
// creating it if needed:
//
//   - Linux: $XDG_STATE_HOME/<app ID> (~/.local/state/<app ID>)
//   - Windows: %LOCALAPPDATA%\<app ID>
//   - macOS: ~/Library/Application Support/<app ID>
func StateDir() (string, error) { return appDir(dirState) }

// LogDir returns the app's folder for log files, creating it if needed:
//
//   - Linux: $XDG_STATE_HOME/<app ID>/logs
//   - Windows: %LOCALAPPDATA%\<app ID>\logs
//   - macOS: ~/Library/Logs/<app ID>
func LogDir() (string, error) { return appDir(dirLogs) }

// appDir returns directory k of the app, created with mode 0700.
func appDir(k dirKind) (string, error) {
	dir, err := dirPath(k)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("desktop.%s: %w", [...]string{"ConfigDir", "DataDir", "CacheDir", "StateDir", "LogDir"}[k], err)
	}
	return dir, nil
}

// dirPath returns the path of directory k without creating it.
func dirPath(k dirKind) (string, error) {
	id, warning := resolveAppID()
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
	if base := os.Getenv(DirEnv); base != "" {
		return filepath.Join(base, id, dirNames[k]), nil
	}
	return osDir(runtime.GOOS, k, id, os.Getenv)
}

// osDir returns directory k of app id on goos, reading the environment
// with getenv (HOME, the XDG variables, APPDATA, LOCALAPPDATA).
func osDir(goos string, k dirKind, id string, getenv func(string) string) (string, error) {
	home := getenv("HOME")
	if goos == "windows" {
		home = getenv("USERPROFILE")
	}
	need := func(v, name string) (string, error) {
		if v == "" {
			return "", fmt.Errorf("desktop: cannot find the app's %s folder: %s is not set", dirNames[k], name)
		}
		return v, nil
	}
	join := func(elem ...string) string {
		if goos == "windows" {
			return strings.Join(elem, `\`)
		}
		return strings.Join(elem, "/")
	}
	switch goos {
	case "windows":
		roaming, local := getenv("APPDATA"), getenv("LOCALAPPDATA")
		switch k {
		case dirConfig, dirData:
			r, err := need(roaming, "%APPDATA%")
			return join(r, id), err
		case dirCache:
			l, err := need(local, "%LOCALAPPDATA%")
			return join(l, id, "cache"), err
		case dirState:
			l, err := need(local, "%LOCALAPPDATA%")
			return join(l, id), err
		default:
			l, err := need(local, "%LOCALAPPDATA%")
			return join(l, id, "logs"), err
		}
	case "darwin", "ios":
		h, err := need(home, "$HOME")
		if err != nil {
			return "", err
		}
		switch k {
		case dirCache:
			return join(h, "Library", "Caches", id), nil
		case dirLogs:
			return join(h, "Library", "Logs", id), nil
		default:
			return join(h, "Library", "Application Support", id), nil
		}
	default: // Linux and the other Unix systems: the XDG Base Directory spec
		xdg := func(env string, def ...string) (string, error) {
			// Relative paths in the variables are invalid and ignored
			// (the spec).
			if v := getenv(env); v != "" && strings.HasPrefix(v, "/") {
				return v, nil
			}
			h, err := need(home, "$HOME")
			if err != nil {
				return "", err
			}
			return join(append([]string{h}, def...)...), nil
		}
		var base string
		var err error
		switch k {
		case dirConfig:
			base, err = xdg("XDG_CONFIG_HOME", ".config")
		case dirData:
			base, err = xdg("XDG_DATA_HOME", ".local", "share")
		case dirCache:
			base, err = xdg("XDG_CACHE_HOME", ".cache")
		case dirState:
			base, err = xdg("XDG_STATE_HOME", ".local", "state")
		default:
			base, err = xdg("XDG_STATE_HOME", ".local", "state")
			if err == nil {
				return join(base, id, "logs"), nil
			}
		}
		if err != nil {
			return "", err
		}
		return join(base, id), nil
	}
}
