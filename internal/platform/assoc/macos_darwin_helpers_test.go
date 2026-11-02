//go:build darwin

package assoc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/plist"
)

// Helpers of the darwin tests: bundles in several places with several
// kinds of executable, and lsregister for what the API does not show.

// The executables a test bundle can have.
const (
	exeTrue  = "true"  // a copy of /usr/bin/true: Apple-signed, universal, another identifier
	exeSelf  = "self"  // a copy of the test binary (ad-hoc signed by the linker on arm64)
	exeBuilt = "built" // a Go program built for the test, as tela build makes one
)

// tempRoot returns t.TempDir() (under /var/folders).
func tempRoot(t *testing.T) string { return t.TempDir() }

// cacheRoot returns a fresh folder in the user cache, where `tela run`
// puts its bundles, removed with the test.
func cacheRoot(t *testing.T) string {
	t.Helper()
	c, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return freshDir(t, c)
}

func freshDir(t *testing.T, parent string) string {
	t.Helper()
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(parent, "tela-assoc-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

var (
	builtOnce sync.Once
	builtExe  string
	builtErr  error
)

// builtProgram builds a tiny Go program once, with CGO_ENABLED=0, so the
// linker ad-hoc signs it on arm64 as it does tela build's.
func builtProgram() (string, error) {
	builtOnce.Do(func() {
		dir, err := os.MkdirTemp("", "tela-assoc-prog-")
		if err != nil {
			builtErr = err
			return
		}
		src := filepath.Join(dir, "main.go")
		if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
			builtErr = err
			return
		}
		goTool, err := exec.LookPath("go")
		if err != nil {
			builtErr = err
			return
		}
		out := filepath.Join(dir, "prog")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, goTool, "build", "-o", out, src)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
		if o, err := cmd.CombinedOutput(); err != nil {
			builtErr = fmt.Errorf("go build: %v\n%s", err, o)
			return
		}
		builtExe = out
	})
	return builtExe, builtErr
}

// lsBundle writes a bundle with a unique ID where `tela run` puts its own
// (the user cache), with a Go program as tela build makes one, and returns
// the app.
func lsBundle(t *testing.T) (platform.HandlerApp, string) {
	t.Helper()
	return lsBundleWith(t, cacheRoot(t), exeBuilt)
}

// lsBundleWith writes a bundle with a unique ID in root, whose executable
// is of the given kind; it is unregistered from Launch Services at the
// end (lsregister -u, best effort).
func lsBundleWith(t *testing.T, root, kind string) (platform.HandlerApp, string) {
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
	exe := writeBundle(t, root, name, id, plist.Dict{
		"CFBundleDisplayName":           name,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleShortVersionString":    "1.0",
		"CFBundleVersion":               "1",
		"LSMinimumSystemVersion":        "13.0",
	})
	src := "/usr/bin/true"
	switch kind {
	case exeSelf:
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		src = self
	case exeBuilt:
		p, err := builtProgram()
		if err != nil {
			t.Logf("no built program (%v): using the test binary", err)
			if p, err = os.Executable(); err != nil {
				t.Fatal(err)
			}
		}
		src = p
	}
	bin, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	t.Cleanup(func() { lsregisterTool(t, 20*time.Second, "-u", dir) })
	t.Logf("bundle %s (executable %s, from %s), ID %s", dir, kind, src, id)
	return platform.HandlerApp{ID: id, Name: name, Exec: exe}, rnd
}

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// lsregisterTool runs lsregister with args and returns its output; a
// failure is logged.
func lsregisterTool(t *testing.T, limit time.Duration, args ...string) string {
	t.Helper()
	return runTool(t, limit, lsregister, args...)
}

// runTool runs a program and returns its combined output; a failure is
// logged.
func runTool(t *testing.T, limit time.Duration, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	o, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		t.Logf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, truncateLog(string(o), 2000))
	}
	return string(o)
}

func truncateLog(s string, n int) string {
	if len(s) > n {
		return s[:n] + fmt.Sprintf("\n[... %d more bytes]", len(s)-n)
	}
	return s
}

// lsDump logs the records of `lsregister -dump` that mention each needle
// (a bundle ID, a path), or says there are none.
func lsDump(t *testing.T, needles ...string) {
	t.Helper()
	start := time.Now()
	out := lsregisterTool(t, 2*time.Minute, "-dump")
	t.Logf("lsregister -dump: %d bytes in %v", len(out), time.Since(start).Round(time.Millisecond))
	records := strings.Split(out, "\n----")
	for _, n := range needles {
		found := 0
		for _, r := range records {
			if !strings.Contains(r, n) {
				continue
			}
			found++
			if found <= 2 {
				t.Logf("lsregister -dump record mentioning %q:\n%s", n, truncateLog(r, 5000))
			}
		}
		t.Logf("lsregister -dump: %d records mention %q", found, n)
	}
}
