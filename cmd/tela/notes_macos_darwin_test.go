//go:build darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
)

// notesAppID is examples/notes's App.AppID: the bundle identifier tela build
// reads from the source. notesScheme is its URL scheme.
const (
	notesAppID  = "dev.telaui.notes"
	notesScheme = "tela-notes"
)

// bundleFolder returns a new folder under <user cache>/tela/test, removed at
// the end of the test: where Launch Services looks at bundles (it ignores
// the ones in /var/folders, where t.TempDir is).
func bundleFolder(t *testing.T) string {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache folder: %v", err)
	}
	base := filepath.Join(cache, "tela", "test")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "notes-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestNotesMacOS is the end-to-end check of examples/notes as a macOS app
// (phase 15), run on macos-latest with
//
//	go test -v -count=1 -timeout 10m -run 'TestNotesMacOS' ./cmd/tela/
//
// It builds notes into a bundle with tela build, runs the bundle's
// executable with its folders in a temporary one (TELA_DESKTOP_DIR, which
// desktop.DataDir honors), and checks, logging every step:
//
//   - (a) the app registered its tela-notes: scheme: its bundle's Info.plist
//     declares it and Launch Services lists the bundle and the scheme. The
//     scheme is not notes:, which macOS's Notes app owns (an app never takes
//     another's association);
//   - (b) starting the bundle's executable again with a link forwards it to
//     the first instance and exits 0, and the note lands in notes.json;
//   - (c) `open 'tela-notes://new?...'`, which Launch Services delivers to
//     the running instance (application:openURLs:), makes a note in
//     notes.json. It runs after (b), once the app has finished launching.
//     It runs on CI (CI is set) or with TELA_E2E_OPEN=1: when
//     Launch Services does not know the running process it starts another
//     Notes outside the test's folders, which is harmless on a runner and
//     not on a Mac somebody uses (elsewhere it is skipped with that
//     reason).
//
// The bundle is built under the user's cache folder (a unique one in
// <user cache>/tela/test, removed at the end), not in a temporary folder:
// Launch Services ignores the bundles that live under /var/folders, so
// neither the registration nor the links would reach it. The process tree is
// killed at the end and the bundle unregistered (lsregister -u). It skips
// without a window server session, in -short mode, and when a Notes is
// already running (it would take the test's launches).
func TestNotesMacOS(t *testing.T) {
	skipShort(t)
	if err := darwin.Load(); err != nil {
		t.Skipf("no AppKit: %v", err)
	}
	session := darwin.CGSessionCopyCurrentDictionary()
	if session == 0 {
		t.Skip("no window server session")
	}
	darwin.CFRelease(session)

	// The instance socket of Notes: a live one means a Notes is running.
	sock := filepath.Join(os.TempDir(), "tela-"+notesAppID+".sock")
	if c, err := net.DialTimeout("unix", sock, time.Second); err == nil {
		c.Close()
		t.Skipf("a Notes is already running (%s answers): it would take this test's launches", sock)
	}

	// --- Build ----------------------------------------------------------
	root := repoRoot(t)
	t.Chdir(root)
	out := bundleFolder(t)
	step := func(format string, args ...any) {
		t.Logf("[%s] %s", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	}
	start := time.Now()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"build", "-v", "-o", out, "./examples/notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela build: status %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	bundle := filepath.Join(out, "Notes.app")
	exe := filepath.Join(bundle, "Contents", "MacOS", "Notes")
	step("tela build wrote %s in %v\n%s%s", bundle, time.Since(start).Round(time.Millisecond), stdout.String(), stderr.String())
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("the bundle has no executable: %v", err)
	}

	// --- Start ----------------------------------------------------------
	desk := t.TempDir() // TELA_DESKTOP_DIR
	library := filepath.Join(desk, notesAppID, "data", "notes.json")
	env := notesEnv(desk)
	var log syncBuffer
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	app := exec.CommandContext(ctx, exe)
	app.Env = env
	app.Dir = desk
	app.Stdout, app.Stderr = &log, &log
	app.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	app.WaitDelay = 5 * time.Second
	if err := app.Start(); err != nil {
		t.Fatalf("starting %s: %v", exe, err)
	}
	started := time.Now()
	exited := make(chan error, 1)
	go func() { exited <- app.Wait() }()
	step("started %s, pid %d, TELA_DESKTOP_DIR %s", exe, app.Process.Pid, desk)
	t.Cleanup(func() {
		// The whole process group, then the bundle in Launch Services.
		_ = syscall.Kill(-app.Process.Pid, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-app.Process.Pid, syscall.SIGKILL)
			<-exited
		}
		forgetBundle(t, bundle)
		if s := log.String(); s != "" {
			t.Logf("the app's output:\n%s", s)
		}
	})

	// It is up when it answers on its instance socket.
	if !waitFor(40*time.Second, func() bool {
		select {
		case err := <-exited:
			t.Fatalf("the app exited before it was up: %v\noutput:\n%s", err, log.String())
		default:
		}
		c, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
		if err == nil {
			c.Close()
		}
		return err == nil
	}) {
		t.Fatalf("the app did not open its instance socket %s within 40s\noutput:\n%s", sock, log.String())
	}
	step("the app is up: it answers on %s (%v after the start)", sock, time.Since(start).Round(time.Millisecond))

	// --- (a) registration ---------------------------------------------
	plistPath := filepath.Join(bundle, "Contents", "Info.plist")
	if !waitFor(30*time.Second, func() bool {
		b, _ := os.ReadFile(plistPath)
		return bytes.Contains(b, []byte("<key>CFBundleURLSchemes</key>")) && bytes.Contains(b, []byte("<string>tela-notes</string>"))
	}) {
		b, _ := os.ReadFile(plistPath)
		t.Fatalf("(a) the app did not declare its tela-notes: scheme in %s within 30s:\n%s\napp output:\n%s", plistPath, b, log.String())
	}
	step("(a) Info.plist declares the tela-notes: scheme")
	if err := darwin.LoadLaunchServices(); err != nil {
		step("(a) SKIPPED the Launch Services check: %v", err)
	} else {
		want, _ := filepath.EvalSymlinks(bundle)
		var paths, handlers []string
		listed := waitFor(30*time.Second, func() bool {
			paths = darwin.LSApplicationPaths(notesAppID)
			handlers = darwin.LSSchemeHandlers(notesScheme)
			return slices.ContainsFunc(paths, func(p string) bool {
				got, _ := filepath.EvalSymlinks(p)
				return got == want
			}) && slices.Contains(handlers, notesAppID)
		})
		if listed {
			step("(a) Launch Services lists %s as %s and as a handler of %s: (handlers %v)", want, notesAppID, notesScheme, handlers)
		} else {
			t.Errorf("(a) Launch Services did not list the bundle %s and the %s: scheme in 30s: paths for %s %v, handlers of %s: %v "+
				"(Launch Services ignores bundles under /var/folders: the bundle must be in the user's cache or ~/Applications)",
				want, notesScheme, notesAppID, paths, notesScheme, handlers)
		}
	}

	// --- (b) a second start forwards its link and exits 0 -------------
	{
		url := notesScheme + "://new?title=Second&text=forwarded+by+a+second+start"
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		second := exec.CommandContext(ctx, exe, url)
		second.Env = env
		second.Dir = desk
		second.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		second.WaitDelay = 5 * time.Second
		var so syncBuffer
		second.Stdout, second.Stderr = &so, &so
		begin := time.Now()
		err := second.Run()
		took := time.Since(begin).Round(time.Millisecond)
		var exit *exec.ExitError
		switch {
		case ctx.Err() != nil:
			_ = syscall.Kill(-second.Process.Pid, syscall.SIGKILL)
			t.Errorf("(b) the second start did not exit within 30s (killed)\noutput:\n%s", so.String())
		case errors.As(err, &exit):
			t.Errorf("(b) the second start exited with status %d after %v, want 0\noutput:\n%s", exit.ExitCode(), took, so.String())
		case err != nil:
			t.Errorf("(b) running the second start: %v", err)
		default:
			step("(b) the second start exited 0 after %v", took)
			if !waitFor(30*time.Second, func() bool { return hasNote(library, "Second", "forwarded by a second start") }) {
				dump, _ := os.ReadFile(library)
				t.Errorf("(b) the second start exited 0, but its link did not reach the first instance's library %s within 30s\nlibrary: %s\napp output:\n%s", library, dump, log.String())
			} else {
				step("(b) the link was forwarded: the note is in %s", library)
			}
		}
		if s := strings.TrimSpace(so.String()); s != "" {
			step("(b) the second start's output:\n%s", s)
		}
	}

	// --- (c) a link through Launch Services ---------------------------
	// After (b): the first instance has handled a launch, so [NSApp run]
	// has finished launching it. The instance socket that (a) waits for
	// opens at the start of Run, before AppKit finishes launching, and an
	// Apple Event sent then fails with procNotFound (-600, "application
	// isn't running"): `open` reported it in phase 26's CI run, when the
	// build first declared the scheme and (a) stopped waiting for the
	// app's own Info.plist edit (which came after its first frame).
	if os.Getenv("CI") == "" && os.Getenv("TELA_E2E_OPEN") == "" {
		step("(c) SKIPPED: `open` may start another Notes through Launch Services outside the test's folders; set CI=1 or TELA_E2E_OPEN=1 to run it")
	} else {
		url := notesScheme + "://new?title=CI&text=from+a+link"
		openURL := func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "open", url)
			cmd.Env = env
			return cmd.CombinedOutput()
		}
		o, err := openURL()
		switch {
		case err != nil:
			// What the next run needs to tell the causes apart: whether
			// the app is still running and how Launch Services sees it,
			// and whether the same `open` works a moment later (it does
			// not count: the check failed).
			running := "running"
			select {
			case err := <-exited:
				running = fmt.Sprintf("exited (%v)", err)
				exited <- err
			default:
			}
			time.Sleep(5 * time.Second)
			o2, err2 := openURL()
			t.Errorf("(c) `open %s` failed %v after the app's start: %v\n%s\nthe first instance (pid %d): %s\n"+
				"Launch Services:\n%s\nthe same open 5s later (diagnostic only): %v\n%s\napp output:\n%s",
				url, time.Since(started).Round(time.Millisecond), err, o, app.Process.Pid, running,
				lsAppInfo(notesAppID), err2, o2, log.String())
		case !waitFor(30*time.Second, func() bool { return hasNote(library, "CI", "from a link") }):
			dump, _ := os.ReadFile(library)
			t.Errorf("(c) `open %s` returned, but no note CI / \"from a link\" reached %s within 30s\nlibrary: %s\nLaunch Services:\n%s\napp output:\n%s", url, library, dump, lsAppInfo(notesAppID), log.String())
		default:
			step("(c) `open %s` reached the running instance: the note is in %s", url, library)
		}
	}

	// The first instance is still the one running.
	select {
	case err := <-exited:
		t.Errorf("the first instance exited during the test: %v\noutput:\n%s", err, log.String())
	default:
		step("the first instance is still running (pid %d); stopping it", app.Process.Pid)
	}
}

// notesEnv is the environment of the app: the caller's without any TELA_
// variable (TELA_BACKEND, TELA_DEBUG...), and TELA_DESKTOP_DIR moving the
// app's folders. The temporary folder stays the user's, which is where
// Launch Services' launches of the bundle look for the single-instance
// socket too.
func notesEnv(desk string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "TELA_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "TELA_DESKTOP_DIR="+desk)
}

// hasNote reports whether the library file has a note with that title and
// text.
func hasNote(library, title, text string) bool {
	b, err := os.ReadFile(library)
	if err != nil {
		return false
	}
	var store struct {
		Notes []struct{ Title, Text string } `json:"notes"`
	}
	if json.Unmarshal(b, &store) != nil {
		return false
	}
	return slices.ContainsFunc(store.Notes, func(n struct{ Title, Text string }) bool { return n.Title == title && n.Text == text })
}

// waitFor polls f every 100 ms until it is true or d passes.
func waitFor(d time.Duration, f func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if f() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// lsAppInfo returns what Launch Services says of the app with bundle ID
// id (its bundles and the handlers of its scheme) and what lsappinfo says
// of its running processes, or why it could not: a diagnostic of a failed
// check.
func lsAppInfo(id string) string {
	var b strings.Builder
	if err := darwin.LoadLaunchServices(); err != nil {
		fmt.Fprintf(&b, "Launch Services: %v\n", err)
	} else {
		fmt.Fprintf(&b, "bundles of %s: %v; handlers of %s: %v (default %q)\n", id, darwin.LSApplicationPaths(id),
			notesScheme, darwin.LSSchemeHandlers(notesScheme), darwin.LSDefaultSchemeHandler(notesScheme))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	found, err := exec.CommandContext(ctx, "lsappinfo", "find", "bundleid="+id).CombinedOutput()
	if err != nil {
		fmt.Fprintf(&b, "lsappinfo find bundleid=%s: %v\n%s", id, err, found)
		return b.String()
	}
	fmt.Fprintf(&b, "lsappinfo find bundleid=%s: %s\n", id, bytes.TrimSpace(found))
	for _, asn := range strings.Fields(string(found)) {
		info, err := exec.CommandContext(ctx, "lsappinfo", "info", "-long", asn).CombinedOutput()
		fmt.Fprintf(&b, "lsappinfo info -long %s: %v\n%s\n", asn, err, info)
	}
	return b.String()
}

// forgetBundle removes the test's bundle from Launch Services, best
// effort: the folder is deleted with the test, and a record of a bundle
// that no longer exists would stay in the user's database.
func forgetBundle(t *testing.T, bundle string) {
	t.Helper()
	const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if o, err := exec.CommandContext(ctx, lsregister, "-u", bundle).CombinedOutput(); err != nil {
		t.Logf("lsregister -u %s: %v\n%s", bundle, err, o)
	}
}
