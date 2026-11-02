//go:build darwin

package assoc

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/plist"
)

// The macOS associations against the real Launch Services, with a
// temporary bundle of a unique ID (a copy of /usr/bin/true as its
// executable). What Launch Services answers is logged, since only CI's
// macOS runners run this. Launch Services has no public call to forget a
// bundle: the bundle is unregistered and deleted, and the database drops
// its record later.

// lsBundle writes a temporary bundle with a unique ID and returns the app.
func lsBundle(t *testing.T) (platform.HandlerApp, string) {
	t.Helper()
	if err := darwin.LoadLaunchServices(); err != nil {
		t.Fatalf("LoadLaunchServices: %v", err)
	}
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	rnd := hex.EncodeToString(b[:])
	id := "dev.telaui.test.assoc" + rnd
	name := "TelaAssoc" + rnd
	exe := writeBundle(t, t.TempDir(), name, id, plist.Dict{
		"CFBundleDisplayName":           name,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleShortVersionString":    "1.0",
		"CFBundleVersion":               "1",
		"LSMinimumSystemVersion":        "13.0",
	})
	bin, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		self, err2 := os.Executable()
		if err2 != nil {
			t.Fatal(err, err2)
		}
		if bin, err = os.ReadFile(self); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exe, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Logf("bundle %s, ID %s", filepath.Dir(filepath.Dir(filepath.Dir(exe))), id)
	return platform.HandlerApp{ID: id, Name: name, Exec: exe}, rnd
}

// watchdog makes a call that does not return fail the test binary, with
// every goroutine's stack, instead of hanging until go test's timeout.
func watchdog(t *testing.T, d time.Duration) {
	timer := time.AfterFunc(d, func() {
		panic("assoc: Launch Services did not answer within " + d.String())
	})
	t.Cleanup(func() { timer.Stop() })
}

// poll calls cond until it holds or limit passes, and logs how long it
// took; it reports whether cond held.
func poll(t *testing.T, what string, limit time.Duration, cond func() bool) bool {
	t.Helper()
	start := time.Now()
	for {
		if cond() {
			t.Logf("%s after %v", what, time.Since(start).Round(time.Millisecond))
			return true
		}
		if time.Since(start) > limit {
			t.Logf("%s: not after %v", what, limit)
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func logLS(t *testing.T, app platform.HandlerApp, as platform.Association, uti string) {
	t.Helper()
	if as.Scheme != "" {
		t.Logf("  LS %s: handlers %q, default %q", as, darwin.LSSchemeHandlers(as.Scheme), darwin.LSDefaultSchemeHandler(as.Scheme))
	} else {
		t.Logf("  LS %s = %q: handlers %q, default %q", as.Extension, uti, darwin.LSContentTypeHandlers(uti), darwin.LSDefaultContentTypeHandler(uti))
	}
	t.Logf("  LS bundles of %s: %q", app.ID, darwin.LSApplicationPaths(app.ID))
}

func lsStatus(t *testing.T, a platform.Associations, app platform.HandlerApp, as platform.Association) platform.AssociationStatus {
	t.Helper()
	s, err := a.Status(app, as)
	if err != nil {
		t.Fatalf("Status(%v): %v", as, err)
	}
	return s
}

func TestDarwinNotABundle(t *testing.T) {
	watchdog(t, time.Minute)
	app := platform.HandlerApp{ID: "dev.telaui.test.assoc", Name: "X"}
	_, err := Native().Status(app, platform.Association{Scheme: "telatest-none"})
	t.Logf("Status from the test binary: %v", err)
	if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "tela run") {
		t.Errorf("Status outside a bundle = %v, want ErrUnsupported naming tela run", err)
	}
}

func TestDarwinRegisterApp(t *testing.T) {
	watchdog(t, 2*time.Minute)
	app, _ := lsBundle(t)
	a := Native()
	st, err := a.AppStatus(app)
	t.Logf("AppStatus before = %+v, %v", st, err)
	changed, err := a.RegisterApp(app, platform.RegisterOptions{})
	t.Logf("RegisterApp = %v, %v", changed, err)
	if err != nil || !changed {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	poll(t, "Launch Services knows the bundle", 15*time.Second, func() bool {
		st, err = a.AppStatus(app)
		return err == nil && st.Registered
	})
	t.Logf("AppStatus = %+v, %v; paths %q", st, err, darwin.LSApplicationPaths(app.ID))
	if !st.Registered || err != nil {
		t.Errorf("AppStatus after RegisterApp = %+v, %v", st, err)
	}
	changed, err = a.RegisterApp(app, platform.RegisterOptions{})
	t.Logf("second RegisterApp = %v, %v", changed, err)
	if changed || err != nil {
		t.Errorf("second RegisterApp = %v, %v, want false, nil", changed, err)
	}
	if changed, err := a.UnregisterApp(app); changed || err != nil {
		t.Errorf("UnregisterApp = %v, %v", changed, err)
	}
}

func TestDarwinScheme(t *testing.T) {
	watchdog(t, 2*time.Minute)
	app, rnd := lsBundle(t)
	a := Native()
	as := platform.Association{Scheme: "telatest-" + rnd}
	t.Cleanup(func() { _, _ = a.Unregister(app, as) })

	t.Logf("Status before = %+v", lsStatus(t, a, app, as))
	changed, err := a.Register(app, as, platform.RegisterOptions{})
	t.Logf("Register = %v, %v", changed, err)
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	var st platform.AssociationStatus
	ok := poll(t, "Launch Services lists the bundle for the scheme", 15*time.Second, func() bool {
		st = lsStatus(t, a, app, as)
		return st.Registered
	})
	logLS(t, app, as, "")
	t.Logf("Status after Register = %+v (the only candidate is the handler by Launch Services' own rule)", st)
	if !ok {
		t.Fatalf("Status after Register = %+v, want Registered", st)
	}
	changed, err = a.Register(app, as, platform.RegisterOptions{})
	t.Logf("second Register = %v, %v", changed, err)
	if changed || err != nil {
		t.Errorf("second Register = %v, %v, want false, nil", changed, err)
	}

	r, err := a.MakeDefault(app, as)
	t.Logf("MakeDefault = %v, %v", r, err)
	if r != platform.DefaultSet || err != nil {
		t.Errorf("MakeDefault = %v, %v", r, err)
	}
	st = lsStatus(t, a, app, as)
	t.Logf("Status after MakeDefault = %+v", st)
	if !st.Registered || !st.Default {
		t.Errorf("Status after MakeDefault = %+v, want registered and the default", st)
	}

	changed, err = a.Unregister(app, as)
	t.Logf("Unregister = %v, %v", changed, err)
	if !changed || err != nil {
		t.Fatalf("Unregister = %v, %v", changed, err)
	}
	if !poll(t, "Launch Services drops the scheme", 15*time.Second, func() bool {
		st = lsStatus(t, a, app, as)
		return !st.Registered
	}) {
		t.Errorf("Status after Unregister = %+v", st)
	}
	logLS(t, app, as, "")
	t.Logf("Status after Unregister = %+v", st)
}

func TestDarwinFileType(t *testing.T) {
	watchdog(t, 2*time.Minute)
	app, rnd := lsBundle(t)
	a := Native()
	ext := "tt" + rnd
	as := platform.Association{Extension: "." + ext, MIMEType: "application/x-telatest-" + rnd, Description: "Tela test document"}
	t.Cleanup(func() { _, _ = a.Unregister(app, as) })

	t.Logf("before: %s is %q", ext, darwin.UTTypeForFilenameExtension(ext))
	t.Logf("Status before = %+v", lsStatus(t, a, app, as))
	changed, err := a.Register(app, as, platform.RegisterOptions{})
	t.Logf("Register = %v, %v", changed, err)
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	info, raw := readInfo(t, app.Exec)
	t.Logf("Info.plist:\n%s", raw)
	want := ownTypeID(app.ID, ext)
	if u, _ := declaredType(info, ext); u != want {
		t.Errorf("declared type = %q, want %q", u, want)
	}
	if !poll(t, "Launch Services knows the imported type", 15*time.Second, func() bool {
		return strings.EqualFold(darwin.UTTypeForFilenameExtension(ext), want)
	}) {
		t.Errorf("%s is %q, want %q", ext, darwin.UTTypeForFilenameExtension(ext), want)
	}
	var st platform.AssociationStatus
	ok := poll(t, "Launch Services lists the bundle for the type", 15*time.Second, func() bool {
		st = lsStatus(t, a, app, as)
		return st.Registered
	})
	logLS(t, app, as, want)
	t.Logf("Status after Register = %+v (rank Alternate; the only handler)", st)
	if !ok {
		t.Fatalf("Status after Register = %+v, want Registered", st)
	}
	changed, err = a.Register(app, as, platform.RegisterOptions{})
	t.Logf("second Register = %v, %v", changed, err)
	if changed || err != nil {
		t.Errorf("second Register = %v, %v, want false, nil", changed, err)
	}

	r, err := a.MakeDefault(app, as)
	t.Logf("MakeDefault = %v, %v", r, err)
	if r != platform.DefaultSet || err != nil {
		t.Errorf("MakeDefault = %v, %v", r, err)
	}
	st = lsStatus(t, a, app, as)
	t.Logf("Status after MakeDefault = %+v", st)
	if !st.Registered || !st.Default {
		t.Errorf("Status after MakeDefault = %+v, want registered and the default", st)
	}

	changed, err = a.Unregister(app, as)
	t.Logf("Unregister = %v, %v", changed, err)
	if !changed || err != nil {
		t.Fatalf("Unregister = %v, %v", changed, err)
	}
	if info, _ := readInfo(t, app.Exec); info[keyImported] != nil || info[keyDocTypes] != nil {
		t.Errorf("Info.plist after Unregister: %v", info)
	}
	if !poll(t, "Launch Services drops the type", 15*time.Second, func() bool {
		st = lsStatus(t, a, app, as)
		return !st.Registered
	}) {
		t.Errorf("Status after Unregister = %+v", st)
	}
	t.Logf("after Unregister: %s is %q, Status %+v", ext, darwin.UTTypeForFilenameExtension(ext), st)
}

// TestDarwinKnownType: a type other apps open (.txt): registering makes the
// app an Alternate candidate, never the default.
func TestDarwinKnownType(t *testing.T) {
	watchdog(t, 2*time.Minute)
	app, _ := lsBundle(t)
	a := Native()
	as := platform.Association{Extension: ".txt", MIMEType: "text/plain", Description: "Text document"}
	t.Cleanup(func() { _, _ = a.Unregister(app, as) })
	uti := darwin.UTTypeForFilenameExtension("txt")
	t.Logf(".txt is %q", uti)
	before := lsStatus(t, a, app, as)
	t.Logf("Status before = %+v", before)
	changed, err := a.Register(app, as, platform.RegisterOptions{})
	t.Logf("Register = %v, %v", changed, err)
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	if info, _ := readInfo(t, app.Exec); info[keyImported] != nil {
		t.Errorf("a type was imported for .txt: %v", info[keyImported])
	}
	var st platform.AssociationStatus
	poll(t, "Launch Services lists the bundle for .txt", 15*time.Second, func() bool {
		st = lsStatus(t, a, app, as)
		return st.Registered
	})
	logLS(t, app, as, uti)
	t.Logf("Status after Register = %+v", st)
	if !st.Registered || st.Default || (before.Other != "" && st.Other == "") {
		t.Errorf("Status after Register = %+v, want registered, not the default, the other app named", st)
	}
	if changed, err := a.Unregister(app, as); !changed || err != nil {
		t.Errorf("Unregister = %v, %v", changed, err)
	}
}
