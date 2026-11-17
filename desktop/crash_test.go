package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// The crash tests run the test binary again as a small app (the child),
// on the headless backend, in dev mode off, with its folders in a
// temporary directory: tela.Run is the code under test, and a panic or a
// fatal error ends the process.
const childEnv = "TELA_DESKTOP_TEST_CHILD"

func TestMain(m *testing.M) {
	if mode := os.Getenv(childEnv); mode != "" {
		runChild(mode)
		os.Exit(0)
	}
	// Whatever a test makes the app write goes to a temporary directory,
	// not to the user's.
	dir, err := os.MkdirTemp("", "tela-desktop-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv(DirEnv, dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// crashChild is the app of the child process.
type crashChild struct{ mode string }

func (c crashChild) Build() tela.Node {
	tela.UseMount(func() func() {
		switch c.mode {
		case "ui":
			panic("boom ui")
		case "fatal":
			debug.SetMaxStack(1 << 20)
			var f func(int) int
			f = func(n int) int { return f(n+1) + 1 }
			f(0)
		case "clean", "deliver":
			tela.Post(tela.Quit)
		}
		return nil
	})
	tela.UseGo(func(ctx context.Context) {
		if c.mode == "go" {
			panic(errors.New("boom go"))
		}
	}, tela.Once)
	return h.Div()
}

// runChild is the main of the child: mode says what the app does.
func runChild(mode string) {
	SetAppID("com.example.crashy")
	if mode == "deliver" {
		OnCrash(func(c Crash) {
			b, _ := json.Marshal(c)
			fmt.Printf("CRASH %s\n", b)
		})
	}
	if err := tela.Run(tela.App{AppID: "com.example.crashy", Root: crashChild{mode}}); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(3)
	}
}

// child runs the app in mode, in dir, and returns its output.
func child(t *testing.T, dir, mode string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+mode, DirEnv+"="+dir, "TELA_DEV=0", "TELA_BACKEND=headless")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the child (%s) did not end; stderr:\n%s", mode, errb.String())
	}
	return out.String(), errb.String(), err
}

func logsOf(dir string) string { return filepath.Join(dir, "com.example.crashy", "logs") }

func glob(t *testing.T, dir, pattern string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func crashesIn(t *testing.T, stdout string) []Crash {
	t.Helper()
	var cs []Crash
	for line := range strings.SplitSeq(stdout, "\n") {
		if j, ok := strings.CutPrefix(line, "CRASH "); ok {
			var c Crash
			if err := json.Unmarshal([]byte(j), &c); err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			cs = append(cs, c)
		}
	}
	return cs
}

// A panic in an effect on the UI thread writes a report and still ends
// the program as a panic does; the next start delivers it once.
func TestCrashPanicOnUIThread(t *testing.T) {
	dir := t.TempDir()
	_, stderr, err := child(t, dir, "ui")
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 2 {
		t.Fatalf("the child ended with %v, want exit status 2 of a panic", err)
	}
	if !strings.Contains(stderr, "panic: boom ui") {
		t.Errorf("the panic is not on the standard error:\n%s", stderr)
	}
	reports := glob(t, logsOf(dir), "crash-*.txt")
	if len(reports) != 1 {
		t.Fatalf("reports = %v, want one", reports)
	}
	if left := glob(t, logsOf(dir), "fatal-*.tmp"); len(left) != 0 {
		t.Errorf("the runtime's file was left behind after a report of its own: %v", left)
	}
	b, _ := os.ReadFile(reports[0])
	for _, want := range []string{"Tela crash report\n", "\nversion: (unstamped)\n", "\nos: " + runtime.GOOS + "/" + runtime.GOARCH + "\n", "\ngo: go",
		"\napp: com.example.crashy\n", "panic on the UI thread", "\n\npanic: boom ui\n\ngoroutine ", "crashChild.Build"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the report lacks %q:\n%s", want, b)
		}
	}

	// The next start gives it to OnCrash, once.
	out, stderr, err := child(t, dir, "deliver")
	if err != nil {
		t.Fatalf("deliver: %v\n%s", err, stderr)
	}
	cs := crashesIn(t, out)
	if len(cs) != 1 {
		t.Fatalf("delivered %d reports, want 1:\n%s", len(cs), out)
	}
	c := cs[0]
	if c.Version != "" || c.Path != reports[0] || !strings.Contains(c.Report, "panic: boom ui") || time.Since(c.Time) > time.Minute || c.Time.IsZero() {
		t.Errorf("Crash = %+v", c)
	}
	if _, err := os.Stat(reports[0]); err == nil {
		t.Errorf("%s was not marked handled", reports[0])
	}
	if h := glob(t, logsOf(dir), "crash-*.txt.handled"); len(h) != 1 {
		t.Errorf("handled = %v", h)
	}
	out, _, err = child(t, dir, "deliver")
	if err != nil || len(crashesIn(t, out)) != 0 {
		t.Errorf("a handled report was delivered again: %v\n%s", err, out)
	}
}

// A panic of a UseGo goroutine too, with its own stack.
func TestCrashPanicInUseGo(t *testing.T) {
	dir := t.TempDir()
	_, stderr, err := child(t, dir, "go")
	if err == nil || !strings.Contains(stderr, "panic: boom go") {
		t.Fatalf("the child ended with %v; stderr:\n%s", err, stderr)
	}
	reports := glob(t, logsOf(dir), "crash-*.txt")
	if len(reports) != 1 {
		t.Fatalf("reports = %v, want one", reports)
	}
	b, _ := os.ReadFile(reports[0])
	if !strings.Contains(string(b), "panic on a UseGo goroutine") || !strings.Contains(string(b), "panic: boom go") {
		t.Errorf("report:\n%s", b)
	}
}

// A fatal error of the runtime (here a stack overflow, which no panic
// handler sees) is written by the runtime to the file Tela gave it, and
// the next start turns it into a report.
func TestCrashFatalError(t *testing.T) {
	dir := t.TempDir()
	_, stderr, err := child(t, dir, "fatal")
	if err == nil || !strings.Contains(stderr, "stack overflow") {
		t.Fatalf("the child ended with %v; stderr:\n%s", err, stderr)
	}
	if got := glob(t, logsOf(dir), "crash-*.txt"); len(got) != 0 {
		t.Errorf("a report was written before the next start: %v", got)
	}
	out, stderr, err := child(t, dir, "deliver")
	if err != nil {
		t.Fatalf("deliver: %v\n%s", err, stderr)
	}
	cs := crashesIn(t, out)
	if len(cs) != 1 {
		t.Fatalf("delivered %d reports, want 1:\n%s", len(cs), out)
	}
	if !strings.Contains(cs[0].Report, "runtime stack:") || !strings.Contains(cs[0].Report, "\ngoroutine ") || !strings.Contains(cs[0].Report, "Tela crash report") ||
		cs[0].Time.IsZero() || time.Since(cs[0].Time) > time.Minute {
		t.Errorf("Crash = %+v", cs[0])
	}
	if left := glob(t, logsOf(dir), "fatal-*.tmp"); len(left) != 0 {
		t.Errorf("the runtime's file was left behind: %v", left)
	}
}

// A run that ends well leaves nothing behind and delivers nothing.
func TestCrashCleanRun(t *testing.T) {
	dir := t.TempDir()
	if _, stderr, err := child(t, dir, "clean"); err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	if left, _ := filepath.Glob(filepath.Join(logsOf(dir), "*")); len(left) != 0 {
		t.Errorf("a clean run left %v", left)
	}
}

// Dev mode (the default of a build that is not a release) writes no
// reports: the panic is on the terminal.
func TestCrashDevModeWritesNone(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"=ui", DirEnv+"="+dir, "TELA_DEV=1", "TELA_BACKEND=headless")
	if err := cmd.Run(); err == nil {
		t.Fatal("the child did not panic")
	}
	if left, _ := filepath.Glob(filepath.Join(logsOf(dir), "*")); len(left) != 0 {
		t.Errorf("dev mode wrote %v", left)
	}
}

// Delivery order and the pruning of handled reports, with reports made by
// hand.
func TestCrashDeliveryOrderAndPruning(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())
	SetAppIDForTest(t, "com.example.order")
	dir, err := LogDir()
	if err != nil {
		t.Fatal(err)
	}
	for i := 12; i > 0; i-- {
		r := fmt.Sprintf("Tela crash report\ntime: 2026-01-%02dT10:00:00Z\nversion: 1.%d.0\n\npanic: n%d\n", i, i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("crash-202601%02dT100000.000Z.txt", i)), []byte(r), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A report from before OnCrash is kept for the app that sets it later.
	deliverCrashes()
	if len(glob(t, dir, "crash-*.txt")) != 12 {
		t.Fatal("reports were taken without a function")
	}
	var got []string
	t.Cleanup(func() { OnCrash(nil) })
	crashes.mu.Lock()
	crashes.cb = func(c Crash) { got = append(got, c.Version) }
	crashes.mu.Unlock()
	deliverCrashes()
	if len(got) != 12 || got[0] != "1.1.0" || got[11] != "1.12.0" {
		t.Errorf("delivered %v, want 1.1.0 to 1.12.0 in order", got)
	}
	if h := glob(t, dir, "crash-*.txt.handled"); len(h) != handledKept || filepath.Base(h[0]) != "crash-20260103T100000.000Z.txt.handled" {
		t.Errorf("handled = %v, want the newest %d", h, handledKept)
	}
}

// A callback that panics still marks its report handled (delivered once).
func TestCrashCallbackPanicMarksHandled(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())
	SetAppIDForTest(t, "com.example.cbpanic")
	dir, _ := LogDir()
	path := filepath.Join(dir, "crash-20260101T100000.000Z.txt")
	if err := os.WriteFile(path, []byte("Tela crash report\nversion: 1\n\npanic: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	crashes.mu.Lock()
	crashes.cb = func(Crash) { panic("callback") }
	crashes.mu.Unlock()
	t.Cleanup(func() { OnCrash(nil) })
	func() {
		defer func() { _ = recover() }()
		deliverCrashes()
	}()
	if _, err := os.Stat(path + ".handled"); err != nil {
		t.Errorf("the report is not marked handled: %v", err)
	}
}
