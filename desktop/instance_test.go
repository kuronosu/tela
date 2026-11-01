package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// SetStartLaunchForTest sets the process's own launch, as tela.Run's
// start does, for the rest of the test (the tests of package
// desktop_test use it).
func SetStartLaunchForTest(t *testing.T, dir string, args ...string) {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	instance.start = &platform.Launch{Args: args, Dir: dir}
	t.Cleanup(func() {
		instance.mu.Lock()
		defer instance.mu.Unlock()
		instance.start = nil
	})
}

// resetInstance undoes what SingleInstance and runStart set.
func resetInstance(t *testing.T) {
	t.Cleanup(func() {
		instance.mu.Lock()
		instance.on, instance.appID, instance.start = false, "", nil
		instance.mu.Unlock()
		appID.mu.Lock()
		appID.set = ""
		appID.mu.Unlock()
	})
}

// Two headless backends of one process play the first and the second
// instance: the first claims the app ID at its start, the second forwards
// its launch and must not open a window, and the first receives it.
func TestSingleInstanceClaim(t *testing.T) {
	resetInstance(t)
	_ = SingleInstance(tela.App{AppID: "com.example.claimtest"})
	first, second := headless.New(headless.Options{}), headless.New(headless.Options{})
	t.Cleanup(first.ReleaseInstances)
	t.Cleanup(second.ReleaseInstances)

	if !runStart(core.RunStart{Backend: first, Log: os.Stderr}) {
		t.Fatal("the first instance was told to exit")
	}
	if got := first.Instances(); !slices.Equal(got, []string{"com.example.claimtest"}) {
		t.Fatalf("first claimed %q", got)
	}
	if l, ok := startLaunch(); !ok || l.Dir == "" {
		t.Errorf("start launch %+v, %v; want this process's", l, ok)
	}
	if runStart(core.RunStart{Backend: second, Log: os.Stderr}) {
		t.Fatal("the second instance was told to run")
	}
	ls := first.Launches()
	wd, _ := os.Getwd()
	if len(ls) != 1 || ls[0].Dir != wd || !slices.Equal(ls[0].Args, os.Args[1:]) {
		t.Errorf("launches forwarded to the first: %+v", ls)
	}
	if len(second.Windows()) != 0 || len(second.Instances()) != 0 {
		t.Error("the second instance opened a window or claimed")
	}
	// A reload of tela dev keeps no start launch, and a claim error runs
	// the app on its own.
	instance.mu.Lock()
	instance.start = nil
	instance.mu.Unlock()
	third := headless.New(headless.Options{})
	third.SetUnsupported(headless.ServiceInstance)
	if !runStart(core.RunStart{Backend: third, DevChild: true, Reload: true, Dev: true, Log: os.Stderr}) {
		t.Error("a claim error ended the app")
	}
	if _, ok := startLaunch(); ok {
		t.Error("a reload kept a start launch")
	}
}

// Without SingleInstance nothing is claimed; the start launch is kept.
func TestRunStartWithoutSingleInstance(t *testing.T) {
	resetInstance(t)
	b := headless.New(headless.Options{})
	t.Cleanup(b.ReleaseInstances)
	if !runStart(core.RunStart{Backend: b, Log: os.Stderr}) || len(b.Instances()) != 0 {
		t.Errorf("claimed %q without SingleInstance", b.Instances())
	}
	if _, ok := startLaunch(); !ok {
		t.Error("no start launch")
	}
}

// The single instance is keyed by SetAppID's ID, else the App's, made a
// valid service name.
func TestInstanceAppID(t *testing.T) {
	resetInstance(t)
	if got := instanceAppID("com.example.App"); got != "com.example.App" {
		t.Errorf("instanceAppID = %q", got)
	}
	SetAppID("My Notes")
	if got := instanceAppID("com.example.App"); got != "My-Notes" {
		t.Errorf("instanceAppID with SetAppID(\"My Notes\") = %q", got)
	}
}

func TestLaunchArguments(t *testing.T) {
	l := Launch{Dir: "/work", Args: []string{"a.txt", "-v", "--flag=x", "C:x", "ab:y", "file://localhost/etc/hosts", "notes:new", "", "--", "--b"}}
	if want := []string{"ab:y", "notes:new"}; !slices.Equal(l.URLs(), want) {
		t.Errorf("URLs = %q, want %q", l.URLs(), want)
	}
	if want := []string{"/work/a.txt", "/work/C:x", "/etc/hosts", "/work/--b"}; runtime.GOOS != "windows" && !slices.Equal(l.Files(), want) {
		t.Errorf("Files = %q, want %q", l.Files(), want)
	}
	if got := (Launch{Args: []string{"rel"}}).Files(); len(got) != 1 || !filepath.IsAbs(got[0]) {
		t.Errorf("Files without Dir = %q, want an absolute path", got)
	}
}

// Without a running app, registration uses the OS's implementation: on
// Linux the user's XDG directories, here temporary ones.
func TestRegisterWithoutApp(t *testing.T) {
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
	t.Setenv("PATH", "") // no update-mime-database: never run a program from a test
	app := ForApp(HandlerApp{ID: "com.example.Installer", Name: "Notes", Exec: "/opt/notes/notes"})
	notes := URLScheme("notes")
	if changed, err := Register(notes, app); !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	entry := filepath.Join(tmp, "data", "applications", "com.example.Installer.desktop")
	b, err := os.ReadFile(entry)
	if err != nil || !strings.Contains(string(b), "x-scheme-handler/notes") || !strings.Contains(string(b), "/opt/notes/notes") {
		t.Fatalf("desktop entry: %v\n%s", err, b)
	}
	if changed, err := Register(notes, app); changed || err != nil {
		t.Errorf("Register again = %v, %v; want nothing written", changed, err)
	}
	if s, err := RegistrationStatus(notes, app); err != nil || !s.Registered {
		t.Errorf("status %+v, %v", s, err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "config", "mimeapps.list")); !os.IsNotExist(err) {
		t.Errorf("Register wrote mimeapps.list (%v); only MakeDefault does", err)
	}
	if r, err := MakeDefault(notes, app); r != DefaultSet || err != nil {
		t.Errorf("MakeDefault = %v, %v", r, err)
	}
	if s, _ := RegistrationStatus(notes, app); !s.Default {
		t.Errorf("status after MakeDefault %+v", s)
	}
	if changed, err := Unregister(notes, app); !changed || err != nil {
		t.Errorf("Unregister = %v, %v", changed, err)
	}
	if s, _ := RegistrationStatus(notes, app); s.Registered || s.Default {
		t.Errorf("status after Unregister %+v", s)
	}
}

func TestNameOf(t *testing.T) {
	for id, want := range map[string]string{"com.example.notes": "Notes", "notes": "Notes", "a.": "A.", "Ünï": "Ünï"} {
		if got := nameOf(id); got != want {
			t.Errorf("nameOf(%q) = %q, want %q", id, got, want)
		}
	}
}
