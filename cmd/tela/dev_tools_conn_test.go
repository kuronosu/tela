package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform/remote"
	"github.com/kuronosu/tela/internal/testutil/wine"
)

// The tools socket's connections (dev_tools.go): what a client that goes
// away, half-closes or leaves input held leaves behind, and the trust and
// clean-up of the discovery files (dev_tools_discover.go).

// bareTools starts a tools server without a host: enough for the calls
// answered without one (info).
func bareTools(t *testing.T, pkgDir string) (*toolsServer, string) {
	t.Helper()
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT)")
	ts := newToolsServer("./bare", pkgDir, t.TempDir(), func(string, ...any) {})
	ln, addr, cleanup, err := toolsListen()
	if err != nil {
		t.Fatal(err)
	}
	ts.ln, ts.addr, ts.cleanup, ts.started = ln, addr, cleanup, time.Now()
	go ts.accept()
	t.Cleanup(ts.close)
	return ts, addr
}

func (ts *toolsServer) connCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.conns)
}

func infoRequests(n int) []byte {
	var b bytes.Buffer
	for i := range n {
		fmt.Fprintf(&b, `{"jsonrpc":"2.0","id":%d,"method":"info"}`+"\n", i)
	}
	return b.Bytes()
}

// A client that sends more requests than the queue holds and goes away
// without reading: the server forgets it (it used to block on the full
// queue of a connection whose writer had stopped, holding it forever).
func TestToolsConnGoneWithQueuedReplies(t *testing.T) {
	ts, addr := bareTools(t, "")
	for range 3 {
		nc, err := net.Dial("unix", addr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := nc.Write(infoRequests(400)); err != nil {
			t.Fatal(err)
		}
		nc.Close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for ts.connCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d connections left after their clients went away", ts.connCount())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A client that half-closes (it sent everything; socat does this) gets
// every reply before the server closes the connection.
func TestToolsConnHalfClose(t *testing.T) {
	_, addr := bareTools(t, "")
	nc, err := net.Dial("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	const n = 5000
	go func() {
		nc.Write(infoRequests(n))
		nc.(*net.UnixConn).CloseWrite()
	}()
	nc.SetReadDeadline(time.Now().Add(devWait))
	sc := bufio.NewScanner(nc)
	got := 0
	for sc.Scan() {
		var r struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID != nil && len(r.Result) > 0 {
			got++
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if got != n {
		t.Errorf("%d replies of %d before the server closed the connection", got, n)
	}
}

// info and the discovery file name the absolute directory of the package
// tela dev runs.
func TestToolsInfoDir(t *testing.T) {
	_, addr := bareTools(t, "/abs/app")
	c, err := dialTools(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	info, err := c.Info(context.Background())
	if err != nil || info.Dir != "/abs/app" || info.Pkg != "./bare" {
		t.Errorf("info %+v, %v", info, err)
	}
}

// A child that asked to start instrumented but started with the
// instrumentation off (its answer came too late) is turned on when it
// takes over: the calls about its tree work.
func TestDevToolsLateStart(t *testing.T) {
	dir := t.TempDir()
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.tools, c.toolsDir = true, dir })
	h.fb.set(1, "tools")
	h.fb.set(2, "toolslate")
	h.expect("the first build", 1, 0, 0)
	s, err := findDevSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	waitBuild(t, c, 1)
	if err := c.Instrument(ctx, true); err != nil {
		t.Fatal(err)
	}
	h.s.push(evKey{'r'})
	waitBuild(t, c, 2)
	if els, err := c.Elements(ctx, 0, false); err != nil || len(els) != 1 || !strings.Contains(els[0].Text, "build 2") {
		t.Errorf("elements of the child that started late: %+v, %v", els, err)
	}
}

// What a client held is let go when it disconnects: the buttons and keys
// its input pressed are released in the app, its pick mode and highlight
// end. A click's count is at most 3.
func TestDevToolsReleaseOnDisconnect(t *testing.T) {
	dir := t.TempDir()
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.tools, c.toolsDir = true, dir })
	h.fb.set(1, "tools")
	fake := make(chan string, 256)
	for _, kind := range []string{"fake.up", "fake.release", "fake.highlight"} {
		h.s.host.Handle(kind, func(c *remote.Child, p []byte) ([]byte, error) {
			fake <- kind + " " + string(p)
			return nil, nil
		})
	}
	expectFake := func(want string) {
		t.Helper()
		timeout := time.After(devWait)
		for {
			select {
			case got := <-fake:
				if got == want {
					return
				}
				t.Logf("fake: %s", got)
			case <-timeout:
				t.Fatalf("the fake child did not report %q", want)
			}
		}
	}
	h.expect("the first build", 1, 0, 0)
	s, err := findDevSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	waitBuild(t, c, 1)
	if err := c.Clicks(ctx, 0, 10, 10, "", 10); err != nil {
		t.Fatal(err)
	}
	h.expect("a click of count 10", 1, 3, 3)
	if err := c.Pointer(ctx, 0, "down", 10, 10, "right"); err != nil {
		t.Fatal(err)
	}
	if err := c.Key(ctx, 0, "Shift", "press"); err != nil {
		t.Fatal(err)
	}
	if err := c.Pick(ctx, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Highlight(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	expectFake("fake.highlight 1")
	// Another client keeps the instrumentation on, so that the pick mode
	// does not end only with it.
	c2, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if err := c2.Instrument(ctx, true); err != nil {
		t.Fatal(err)
	}
	c.Close()
	<-c.Done()
	expectFake(fmt.Sprintf("fake.up %d", 2)) // platform.ButtonSecondary
	expectFake("fake.release Shift")
	expectFake("fake.highlight 0")
	// The pick mode ended: a click reaches the app.
	if err := c2.Click(ctx, 0, 10, 10); err != nil {
		t.Fatal(err)
	}
	h.expect("a click after the picker left", 1, 4, 4)
}

// drag lets go of the button when a move fails.
func TestAttachDragReleasesOnError(t *testing.T) {
	app := &failMoveApp{fakeApp: newFakeApp()}
	s := newAttachSession("", func(string, ...any) {})
	s.dial = func(string) (attachConn, error) { return app, nil }
	s.quiet, s.first, s.max, s.poll = 5*time.Millisecond, 20*time.Millisecond, 300*time.Millisecond, time.Millisecond
	s.mustTool(t, "snapshot", `{}`)
	text, res := s.tool(t, "drag", `{"from": {"ref": "e1"}, "dx": 40}`)
	if !res.IsError || !strings.Contains(text, "the move failed") {
		t.Errorf("drag with a failing move: %v %s", res.IsError, text)
	}
	wantIn(t, "drag calls", app.called(), "pointer 1 down 60,70", "pointer 1 up 60,70")
}

// failMoveApp is a fakeApp whose pointer moves fail.
type failMoveApp struct{ *fakeApp }

func (f *failMoveApp) Pointer(ctx context.Context, win uint32, action string, x, y float32, button string) error {
	if action == "move" {
		return errors.New("the move failed")
	}
	return f.fakeApp.Pointer(ctx, win, action, x, y, button)
}

// The attached lint tool lints the directory tela dev runs, wherever tela
// mcp --attach runs.
func TestAttachLintUsesTheSessionDir(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "lint", "dirty"))
	if err != nil {
		t.Fatal(err)
	}
	app := &dirApp{fakeApp: newFakeApp(), dir: dir}
	s := newAttachSession("", func(string, ...any) {})
	s.dial = func(string) (attachConn, error) { return app, nil }
	s.quiet, s.first, s.max, s.poll = 5*time.Millisecond, 20*time.Millisecond, 300*time.Millisecond, time.Millisecond
	text, res := s.tool(t, "lint", `{}`)
	if res.IsError || !strings.Contains(text, "problem") || !strings.Contains(text, filepath.Join(dir, "dirty.go")) {
		t.Errorf("lint of the session's directory: %v %s", res.IsError, text)
	}
}

// dirApp is a fakeApp whose info names a package directory.
type dirApp struct {
	*fakeApp
	dir string
}

func (f *dirApp) Info(ctx context.Context) (toolsInfo, error) {
	i, err := f.fakeApp.Info(ctx)
	i.Pkg, i.Dir = "./not-here", f.dir
	return i, err
}

// A discovery file is removed only when its socket refuses the connection
// or is gone, not on another error; the socket directory of a session
// whose process ended goes with its file.
func TestToolsDiscoveryCleanup(t *testing.T) {
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT)")
	files := filepath.Join(t.TempDir(), "dev")
	private := t.TempDir()
	os.Chmod(private, 0o700)
	long := filepath.Join(private, strings.Repeat("x", 200)) // too long for an AF_UNIX address: EINVAL
	path, err := writeDevSession(files, devSession{Addr: long, Pkg: "./app", PID: os.Getpid(), Started: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := findDevSession(files); err == nil {
		t.Error("a session whose socket cannot be dialed was found")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the discovery file of a socket that failed to dial with %v was removed", err)
	}
	os.Remove(path)

	// A session whose process ended.
	ln, addr, cleanup, err := toolsListen()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	ln.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gone := exec.Command(exe, "-test.run=^$")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDevSession(files, devSession{Addr: addr, PID: gone.Process.Pid, Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if ss, err := listDevSessions(files); err != nil || len(ss) != 0 {
		t.Errorf("sessions %+v, %v", ss, err)
	}
	if _, err := os.Stat(filepath.Dir(addr)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket directory of the ended session is still there: %v", err)
	}
}
