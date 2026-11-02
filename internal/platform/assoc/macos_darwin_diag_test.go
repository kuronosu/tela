//go:build darwin

package assoc

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
)

// TestDarwinDiagnostics logs what Launch Services does with bundles in
// several places (the temporary folder, the user cache where `tela run`
// builds, ~/Applications) and with several executables (a copy of
// /usr/bin/true, of the test binary, a Go program built like tela build's):
// LSRegisterURL's status, how long until LSCopyApplicationURLsForBundleIdentifier
// lists the bundle, whether inUpdate false or `lsregister -f` changes
// anything, and the lsregister -dump records of each ID. It never fails on
// what Launch Services answers: the other darwin tests check behavior.
func TestDarwinDiagnostics(t *testing.T) {
	watchdog(t, 6*time.Minute)
	if err := darwin.LoadLaunchServices(); err != nil {
		t.Fatalf("LoadLaunchServices: %v", err)
	}

	// The bindings against what every Mac has.
	t.Logf("sw_vers:\n%s", runTool(t, 10*time.Second, "/usr/bin/sw_vers"))
	t.Logf("id: %s", strings.TrimSpace(runTool(t, 10*time.Second, "/usr/bin/id")))
	t.Logf("launchctl managername: %s", strings.TrimSpace(runTool(t, 10*time.Second, "/bin/launchctl", "managername")))
	for _, id := range []string{"com.apple.TextEdit", "com.apple.Safari", "com.apple.finder"} {
		t.Logf("LS bundles of %s: %q", id, darwin.LSApplicationPaths(id))
	}
	for _, s := range []string{"mailto", "https"} {
		t.Logf("LS %s: handlers %q, default %q", s, darwin.LSSchemeHandlers(s), darwin.LSDefaultSchemeHandler(s))
	}
	txt := darwin.UTTypeForFilenameExtension("txt")
	t.Logf("LS .txt is %q: handlers %q, default %q", txt, darwin.LSContentTypeHandlers(txt), darwin.LSDefaultContentTypeHandler(txt))

	roots := []struct {
		name string
		root func(*testing.T) string
	}{
		{"temp", tempRoot},
		{"cache", cacheRoot},
		{"~/Applications", homeAppsRoot},
	}
	var ids, summary []string
	for _, r := range roots {
		for _, kind := range []string{exeTrue, exeSelf, exeBuilt} {
			label := r.name + "/" + kind
			app, _ := lsBundleWith(t, r.root(t), kind)
			dir := filepath.Dir(filepath.Dir(filepath.Dir(app.Exec)))
			ids = append(ids, app.ID)
			t.Logf("[%s] codesign -dv:\n%s", label, strings.TrimSpace(runTool(t, 20*time.Second, "/usr/bin/codesign", "-dv", "--verbose=2", app.Exec)))
			t.Logf("[%s] CFURL of the bundle: %q", label, darwin.FileURLRoundTrip(dir))
			listed := func() bool { return len(darwin.LSApplicationPaths(app.ID)) > 0 }

			start := time.Now()
			st := darwin.LSRegisterURL(dir, true)
			t.Logf("[%s] LSRegisterURL(inUpdate true) = %d in %v", label, st, time.Since(start).Round(time.Millisecond))
			how := "LSRegisterURL(true)"
			ok := poll(t, "["+label+"] listed after LSRegisterURL(true)", 5*time.Second, listed)
			if !ok {
				st2 := darwin.LSRegisterURL(dir, false)
				t.Logf("[%s] LSRegisterURL(inUpdate false) = %d", label, st2)
				how = "LSRegisterURL(false)"
				ok = poll(t, "["+label+"] listed after LSRegisterURL(false)", 3*time.Second, listed)
			}
			if !ok {
				out := lsregisterTool(t, 30*time.Second, "-f", dir)
				t.Logf("[%s] lsregister -f: %q", label, truncateLog(out, 500))
				how = "lsregister -f"
				ok = poll(t, "["+label+"] listed after lsregister -f", 5*time.Second, listed)
			}
			if !ok {
				how = "never"
			}
			t.Logf("[%s] LS bundles of %s: %q; status %d", label, app.ID, darwin.LSApplicationPaths(app.ID), st)
			summary = append(summary, fmt.Sprintf("%-22s LSRegisterURL=%d listed by: %s", label, st, how))
		}
	}
	lsDump(t, ids...)
	t.Logf("summary:\n%s", strings.Join(summary, "\n"))
}
