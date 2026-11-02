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
// reads from the source.
const notesAppID = "dev.telaui.notes"

// TestNotesMacOS is the end-to-end check of examples/notes as a macOS app
// (phase 15), run on macos-latest with
//
//	go test -v -count=1 -timeout 10m -run 'TestNotesMacOS' ./cmd/tela/
//
// It builds notes into a bundle with tela build, runs the bundle's
// executable with its folders in a temporary one (TELA_DESKTOP_DIR, which
// desktop.DataDir honors), and checks, logging every step:
//
//   - (a) the app registered its notes: scheme: its bundle's Info.plist
//     declares it (always checked) and Launch Services lists the bundle and
//     the scheme (a timing-dependent database: when it does not list them
//     in time the step logs why and does not fail);
//   - (b) `open 'notes://new?...'`, which Launch Services delivers to the
//     running instance (application:openURLs:), makes a note in
//     notes.json. It runs on CI (CI is set) or with TELA_E2E_OPEN=1: when
//     Launch Services does not know the running process it starts another
//     Notes outside the test's folders, which is harmless on a runner and
//     not on a Mac somebody uses;
//   - (c) starting the bundle's executable again with a link forwards it to
//     the first instance and exits 0, and the note lands in notes.json.
//
// The process tree is killed at the end (and the bundle forgotten by
// Launch Services). It skips without a window server session, in -short
// mode, and when a Notes is already running (it would take the test's
// launches).
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
	out := t.TempDir()
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
		return bytes.Contains(b, []byte("<key>CFBundleURLSchemes</key>")) && bytes.Contains(b, []byte("<string>notes</string>"))
	}) {
		b, _ := os.ReadFile(plistPath)
		t.Fatalf("(a) the app did not declare its notes: scheme in %s within 30s:\n%s\napp output:\n%s", plistPath, b, log.String())
	}
	step("(a) Info.plist declares the notes: scheme")
	if err := darwin.LoadLaunchServices(); err != nil {
		step("(a) SKIPPED the Launch Services check: %v", err)
	} else {
		want, _ := filepath.EvalSymlinks(bundle)
		var paths, handlers []string
		listed := waitFor(30*time.Second, func() bool {
			paths = darwin.LSApplicationPaths(notesAppID)
			handlers = darwin.LSSchemeHandlers("notes")
			return slices.ContainsFunc(paths, func(p string) bool {
				got, _ := filepath.EvalSymlinks(p)
				return got == want
			}) && slices.Contains(handlers, notesAppID)
		})
		if listed {
			step("(a) Launch Services lists %s as %s and as a handler of notes: (handlers %v)", want, notesAppID, handlers)
		} else {
			step("(a) SKIPPED the Launch Services check, which did not list the bundle in 30s: paths for %s %v, handlers of notes: %v "+
				"(the registration database updates on its own schedule; Info.plist does declare the scheme)", notesAppID, paths, handlers)
		}
	}

	// --- (b) a link through Launch Services ---------------------------
	if os.Getenv("CI") == "" && os.Getenv("TELA_E2E_OPEN") == "" {
		step("(b) SKIPPED: `open` may start another Notes through Launch Services outside the test's folders; set CI=1 or TELA_E2E_OPEN=1 to run it")
	} else {
		url := "notes://new?title=CI&text=from+a+link"
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "open", url)
		cmd.Env = env
		o, err := cmd.CombinedOutput()
		cancel()
		switch {
		case err != nil:
			step("(b) SKIPPED: `open %s` failed: %v\n%s (Launch Services could not route the link on this runner)", url, err, o)
		case !waitFor(30*time.Second, func() bool { return hasNote(library, "CI", "from a link") }):
			dump, _ := os.ReadFile(library)
			t.Errorf("(b) `open %s` returned, but no note CI / \"from a link\" reached %s within 30s\nlibrary: %s\napp output:\n%s", url, library, dump, log.String())
		default:
			step("(b) `open %s` reached the running instance: the note is in %s", url, library)
		}
	}

	// --- (c) a second start forwards its link and exits 0 -------------
	{
		url := "notes://new?title=Second&text=forwarded+by+a+second+start"
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
			t.Errorf("(c) the second start did not exit within 30s (killed)\noutput:\n%s", so.String())
		case errors.As(err, &exit):
			t.Errorf("(c) the second start exited with status %d after %v, want 0\noutput:\n%s", exit.ExitCode(), took, so.String())
		case err != nil:
			t.Errorf("(c) running the second start: %v", err)
		default:
			step("(c) the second start exited 0 after %v", took)
			if !waitFor(30*time.Second, func() bool { return hasNote(library, "Second", "forwarded by a second start") }) {
				dump, _ := os.ReadFile(library)
				t.Errorf("(c) the second start exited 0, but its link did not reach the first instance's library %s within 30s\nlibrary: %s\napp output:\n%s", library, dump, log.String())
			} else {
				step("(c) the link was forwarded: the note is in %s", library)
			}
		}
		if s := strings.TrimSpace(so.String()); s != "" {
			step("(c) the second start's output:\n%s", s)
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
