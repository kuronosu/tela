package assoc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/plist"
)

// TestMacOSTempDir: Launch Services ignores bundles in temporary folders,
// so the calls that register refuse them and Status reports them not
// registered. The temporary folder is named through a symbolic link, as
// /var is /private/var.
func TestMacOSTempDir(t *testing.T) {
	l := newMemLS()
	m := MacOS(l).(*macOS)
	m.settle = 0
	base := t.TempDir()
	real := filepath.Join(base, "private-tmp")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(base, "tmp")
	if err := os.Symlink(real, tmp); err != nil {
		tmp = real // no symbolic links (Windows without the privilege)
	}
	m.tempDirs = func() []string { return []string{"", tmp} }
	exe := writeBundle(t, filepath.Join(real, "T"), "Notes", "com.example.Notes", nil)
	app := platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: exe}
	_, raw := readInfo(t, exe)

	check := func(call string, err error) {
		t.Helper()
		if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "temporary folder") ||
			!strings.Contains(err.Error(), "tela run") || !strings.Contains(err.Error(), "tela build -o") {
			t.Errorf("%s in a temporary folder = %v", call, err)
		}
	}
	_, err := m.Register(app, scheme, platform.RegisterOptions{})
	check("Register", err)
	r, err := m.MakeDefault(app, noteFile)
	check("MakeDefault", err)
	if r != platform.DefaultNone {
		t.Errorf("MakeDefault = %v", r)
	}
	_, err = m.RegisterApp(app, platform.RegisterOptions{})
	check("RegisterApp", err)
	if _, raw2 := readInfo(t, exe); !bytes.Equal(raw, raw2) || l.calls() != 0 {
		t.Error("a refused call wrote or registered")
	}

	// Even with the Info.plist declaring the scheme, and Launch Services
	// listing another copy of the app, this bundle is not registered.
	other := writeBundle(t, t.TempDir(), "Notes", "com.example.Notes", plist.Dict{
		keyURLTypes: plist.Array{plist.Dict{"CFBundleURLSchemes": plist.Array{"notes"}}},
	})
	if err := l.RegisterBundle(filepath.Dir(filepath.Dir(filepath.Dir(other)))); err != nil {
		t.Fatal(err)
	}
	data, err := plist.Marshal(plist.Dict{"CFBundleIdentifier": "com.example.Notes",
		keyURLTypes: plist.Array{plist.Dict{"CFBundleURLSchemes": plist.Array{"notes"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(exe)), "Info.plist"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err := m.Status(app, scheme); s.Registered || s.Default || err != nil {
		t.Errorf("Status in a temporary folder = %+v, %v", s, err)
	}
	if st, err := m.AppStatus(app); !st.Registered || st.Other == "" || err != nil {
		t.Errorf("AppStatus in a temporary folder = %+v, %v (the other copy is known)", st, err)
	}

	// Next to the temporary folder, outside it: registered.
	app.Exec = writeBundle(t, filepath.Join(base, "private-tmp-apps"), "Notes", "com.example.Notes", nil)
	if changed, err := m.Register(app, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register outside the temporary folder = %v, %v", changed, err)
	}
}
