package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMCPRebuildCancelledWhileMounting cancels a call after the build
// finished and before the instance mounted the screen. Nothing is wrong
// with the sources, so the next call must build again and succeed, not
// report the cancellation as the problem of the package until a file
// changes.
func TestMCPRebuildCancelledWhileMounting(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := mcpAppModule(t)
	t.Chdir(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logf := func(format string, args ...any) {
		// The line before the instance starts.
		if strings.HasPrefix(format, "build %d in") {
			cancel()
		}
	}
	s := newMCPSession(goBin, mcpOptions{Pkg: "./app", W: 480, H: 320, Scale: 1}, t.TempDir(), logf, nil)
	defer s.close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, problem := s.ensure(ctx); problem == "" {
		t.Fatal("the first call was cancelled while the screen mounted, and went on")
	}
	if s.broken != "" {
		t.Fatalf("the cancellation is recorded as a problem of the sources: %q", s.broken)
	}
	notes, problem := s.ensure(context.Background())
	if problem != "" {
		t.Fatalf("the next call after a cancelled build fails until a file changes: %s", problem)
	}
	if s.child == nil || !s.child.alive() || !strings.Contains(strings.Join(notes, "\n"), "Built and mounted") {
		t.Errorf("the next call did not build and mount the screen: notes %v, child %v", notes, s.child)
	}
}

// TestMCPOpenFallbackStartsASegment checks that when open cannot mount a
// screen and mounts the previous one again (state reset), the recording
// starts a new mount: the test tool writes a second Mount, not a replay of
// the steps over a state they did not reach.
func TestMCPOpenFallbackStartsASegment(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := fixtureModule(t, "mountfail")
	t.Chdir(dir)
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", W: 320, H: 200, Scale: 1, Preview: "Good"})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if r := c.tool("click", map[string]any{"role": "button", "name": "Count: 0"}); r.isError {
		t.Fatalf("click:\n%s", r.text)
	}
	r := c.tool("open", map[string]any{"preview": "Bad"})
	if !r.isError || !strings.Contains(r.text, "The previous screen was mounted again") {
		t.Fatalf("open of a screen that cannot be mounted:\n%s", r.text)
	}
	if r := c.tool("click", map[string]any{"role": "button", "name": "Count: 0"}); r.isError {
		t.Fatalf("the click after the fallback (the count was reset):\n%s", r.text)
	}
	if r := c.tool("test", map[string]any{"name": "fallback"}); r.isError {
		t.Fatalf("test:\n%s", r.text)
	}
	b, err := os.ReadFile(filepath.Join(dir, "app", "fallback_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "telatest.Mount("); n != 2 {
		t.Errorf("%d mounts in the test, want 2 (the fallback mounted the screen afresh):\n%s", n, b)
	}
}

// goWrapper writes a script that stands for the go command: it runs the
// real one, after what the test asks for through the environment (which the
// tool's go vet and go test inherit).
func goWrapper(t *testing.T, realGo string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the wrapper of the go command is a shell script")
	}
	script := `#!/bin/sh
if [ "$1" = vet ]; then
  if [ -n "$MCPTEST_EDIT" ]; then echo "// edited while go vet ran" >> "$MCPTEST_EDIT"; fi
  if [ -n "$MCPTEST_VETFAIL" ]; then echo "$MCPTEST_VETFAIL:3:1: boom"; exit 1; fi
fi
exec "` + realGo + `" "$@"
`
	path := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestMCPTestToolFiles checks what the test tool does with the files and
// the fingerprint around its work: a symlink is refused and not written
// through, a test that does not vet puts back the file it would have
// replaced, and a source edit made while go vet and go test run still
// rebuilds on the next call.
func TestMCPTestToolFiles(t *testing.T) {
	skipShort(t)
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	goBin := goWrapper(t, realGo)
	dir := mcpAppModule(t)
	t.Chdir(dir)
	appDir := filepath.Join(dir, "app")
	// Files in the package from the start: adding one later would rebuild
	// the screen. orig_test.go is one the tool wrote; link_test.go points
	// at it.
	orig := testgenMarker + ": the original.\npackage mcpapp\n"
	if err := os.WriteFile(filepath.Join(appDir, "orig_test.go"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("orig_test.go", filepath.Join(appDir, "link_test.go")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", W: 480, H: 320, Scale: 1})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if r := c.tool("click", map[string]any{"role": "button", "name": "Count: 0"}); r.isError {
		t.Fatalf("click:\n%s", r.text)
	}

	// A symlink is not written through, and the file it points at is kept.
	if r := c.tool("test", map[string]any{"name": "flow", "path": "link_test.go"}); !r.isError || !strings.Contains(r.text, "not a regular file") {
		t.Errorf("test into a symlink: %v\n%s", r.isError, r.text)
	}
	if b, _ := os.ReadFile(filepath.Join(appDir, "orig_test.go")); string(b) != orig {
		t.Errorf("a file was written through a symlink:\n%s", b)
	}

	// A good test, written once.
	r := c.tool("test", map[string]any{"name": "flow"})
	if r.isError || !strings.Contains(r.text, "TestFlow passes") {
		t.Fatalf("test:\n%s", r.text)
	}
	flow := filepath.Join(appDir, "flow_test.go")
	good, err := os.ReadFile(flow)
	if err != nil {
		t.Fatal(err)
	}

	// A test that does not vet: the file it would have replaced comes back,
	// and a new file is removed.
	t.Setenv("MCPTEST_VETFAIL", "flow_test.go")
	r = c.tool("test", map[string]any{"name": "flow"})
	if !r.isError || !strings.Contains(r.text, "was put back") {
		t.Errorf("test that does not vet over a file of the tool's: %v\n%s", r.isError, r.text)
	}
	if b, err := os.ReadFile(flow); err != nil || string(b) != string(good) {
		t.Errorf("the previous good test was not put back (%v):\n%s", err, b)
	}
	t.Setenv("MCPTEST_VETFAIL", "other_test.go")
	r = c.tool("test", map[string]any{"name": "other"})
	if !r.isError || !strings.Contains(r.text, "it was not kept") {
		t.Errorf("test that does not vet, a new file: %v\n%s", r.isError, r.text)
	}
	if _, err := os.Lstat(filepath.Join(appDir, "other_test.go")); err == nil {
		t.Error("the failed new test was kept")
	}
	t.Setenv("MCPTEST_VETFAIL", "")
	// Neither the write nor the restore rebuilt the screen.
	if r := c.tool("snapshot", nil); r.isError || strings.Contains(r.text, "rebuilt") {
		t.Fatalf("the test tool's own writes rebuilt the screen:\n%s", r.text)
	}

	// The source is edited while go vet runs: the next call rebuilds.
	t.Setenv("MCPTEST_EDIT", filepath.Join(appDir, "mcpapp.go"))
	if r := c.tool("test", map[string]any{"name": "flow"}); r.isError {
		t.Fatalf("test with an edit during vet:\n%s", r.text)
	}
	t.Setenv("MCPTEST_EDIT", "")
	if r := c.tool("snapshot", nil); r.isError || !strings.Contains(r.text, "rebuilt") {
		t.Errorf("an edit made while the test ran was swallowed:\n%s", r.text)
	}
}
