package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The start of tela.Run gives the OS services the app's ID (ADR-217):
// SetAppID's, else the App's, as a valid service name.
func TestRunStartSetsAppID(t *testing.T) {
	resetInstance(t)
	b := headless.New(headless.Options{})
	t.Cleanup(b.ReleaseInstances)
	if !runStart(core.RunStart{Backend: b, AppID: "com.example.App", Log: os.Stderr}) {
		t.Fatal("runStart ended the app")
	}
	if got := b.DesktopAppID(); got != "com.example.App" {
		t.Errorf("the backend's app ID = %q, want the App's", got)
	}
	SetAppID("My Notes")
	b2 := headless.New(headless.Options{})
	t.Cleanup(b2.ReleaseInstances)
	runStart(core.RunStart{Backend: b2, AppID: "com.example.App", Log: os.Stderr})
	if got := b2.DesktopAppID(); got != "My-Notes" {
		t.Errorf("the backend's app ID with SetAppID(\"My Notes\") = %q", got)
	}
}

// Without a running app, RegisterApp writes the app's own desktop entry
// in the XDG directories (temporary ones here), with no file types.
func TestRegisterAppWithoutApp(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the OS implementation is tested on Linux, in temporary XDG directories")
	}
	if _, _, _, ok := core.RunningServices(); ok {
		t.Fatal("an app is running")
	}
	resetInstance(t)
	tmp := t.TempDir()
	for env, dir := range map[string]string{"XDG_DATA_HOME": "data", "XDG_CONFIG_HOME": "config", "XDG_DATA_DIRS": "sysdata", "XDG_CONFIG_DIRS": "sysconfig"} {
		t.Setenv(env, filepath.Join(tmp, dir))
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("PATH", "")
	SetAppID("com.example.Installer")
	app := ForApp(HandlerApp{Exec: "/opt/notes/notes"})
	if changed, err := RegisterApp(app); !changed || err != nil {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	entry := filepath.Join(tmp, "data", "applications", "com.example.Installer.desktop")
	b, err := os.ReadFile(entry)
	if err != nil || !strings.Contains(string(b), "NoDisplay=true\n") || !strings.Contains(string(b), "Name=Installer\n") ||
		!strings.Contains(string(b), `Exec=/opt/notes/notes`+"\n") || strings.Contains(string(b), "MimeType") {
		t.Fatalf("desktop entry: %v\n%s", err, b)
	}
	if s, err := AppStatus(); err != nil || s != (AppRegistration{Registered: true, Requested: true}) {
		t.Errorf("AppStatus = %+v, %v", s, err)
	}
	if changed, err := UnregisterApp(); !changed || err != nil {
		t.Errorf("UnregisterApp = %v, %v", changed, err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Errorf("the entry is still there (%v)", err)
	}
}
