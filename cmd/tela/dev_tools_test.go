package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform/remote"
	"github.com/kuronosu/tela/internal/testutil/wine"
)

// The tools socket of tela dev (ADR-299, dev_tools.go): its discovery
// files, its log, its relay to a fake child, and a real app end to end.

func TestToolsDiscovery(t *testing.T) {
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT)")
	dir := filepath.Join(t.TempDir(), "dev")
	ln, addr, cleanup, err := toolsListen()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer ln.Close()
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			nc.Close()
		}
	}()
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Dir(addr))
		if err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("the socket's directory: %v, %v; want a private one (0700)", fi.Mode(), err)
		}
	}

	if s, err := findDevSession(dir); err == nil || !strings.Contains(err.Error(), "no tela dev session") {
		t.Errorf("with no session: %+v, %v", s, err)
	}
	started := time.Now().Round(0)
	path, err := writeDevSession(dir, devSession{Addr: addr, Pkg: "./app", PID: os.Getpid(), Started: started})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("the discovery file: %v, %v; want 0600", fi.Mode(), err)
		}
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("the discovery directory: %v, %v; want 0700", fi.Mode(), err)
		}
	}

	// Stale files: a process that ended, a file that is not JSON; another
	// file is left alone.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gone := exec.Command(exe, "-test.run=^$")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDevSession(dir, devSession{Addr: addr, PID: gone.Process.Pid, Started: started}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "12.json"), []byte("{"), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	ss, err := listDevSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 1 || ss[0].Addr != addr || ss[0].Pkg != "./app" || ss[0].PID != os.Getpid() || !ss[0].Started.Equal(started) {
		t.Fatalf("sessions %+v, want this process's", ss)
	}
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if want := []string{filepath.Base(path), "notes.txt"}; !slices.Equal(names, want) {
		t.Errorf("files left %v, want %v (the stale ones removed)", names, want)
	}
	if s, err := findDevSession(dir); err != nil || s.Addr != addr {
		t.Errorf("findDevSession: %+v, %v", s, err)
	}

	// A second live session: a client must name one.
	if ppid := os.Getppid(); processAlive(ppid) {
		if _, err := writeDevSession(dir, devSession{Addr: addr, Pkg: "./other", PID: ppid, Started: started.Add(time.Second)}); err != nil {
			t.Fatal(err)
		}
		if _, err := findDevSession(dir); err == nil || !strings.Contains(err.Error(), "2 tela dev sessions") || !strings.Contains(err.Error(), "./other") {
			t.Errorf("with two sessions: %v", err)
		}
		// One whose socket does not answer is dropped.
		if _, err := writeDevSession(dir, devSession{Addr: addr + ".gone", Pkg: "./other", PID: ppid, Started: started}); err != nil {
			t.Fatal(err)
		}
		if s, err := findDevSession(dir); err != nil || s.Pkg != "./app" {
			t.Errorf("with a dead socket: %+v, %v", s, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "12.json")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the unreadable file is still there: %v", err)
		}
	}
	if processAlive(gone.Process.Pid) {
		t.Errorf("processAlive(%d) of a process that exited", gone.Process.Pid)
	}
}

func TestToolsLog(t *testing.T) {
	var l toolsLog
	var got [][]toolsLogEntry
	l.added = func(es []toolsLogEntry) { got = append(got, es) }
	var fwd bytes.Buffer
	w := l.stderr(3, &fwd)
	long := strings.Repeat("x", toolsLineMax+10)
	parts := []string{
		"first ", "line\nsecond\r\n\n",
		"2026/11/20 10:04:05 WARN careful n=1\n",
		"time=2026-11-20T10:04:05.000Z level=ERROR msg=broken\n",
		"panic: boom\n",
		"a note that says INFO in it\n",
		long + "\n",
		"unfinished",
	}
	for _, p := range parts {
		if _, err := w.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	if fwd.String() != strings.Join(parts, "") {
		t.Errorf("forwarded %q", fwd.String())
	}
	l.flush() // the lines wait for a twin among the app's entries
	es, last := l.since(0)
	type line struct {
		msg   string
		level slog.Level
	}
	var lines []line
	for _, e := range es {
		if e.Source != "stderr" || e.Build != 3 || e.At.IsZero() {
			t.Errorf("entry %+v", e)
		}
		lines = append(lines, line{e.Msg, e.Level})
	}
	want := []line{
		{"first line", slog.LevelInfo}, {"second", slog.LevelInfo},
		{"2026/11/20 10:04:05 WARN careful n=1", slog.LevelWarn},
		{"time=2026-11-20T10:04:05.000Z level=ERROR msg=broken", slog.LevelError},
		{"panic: boom", slog.LevelError},
		{"a note that says INFO in it", slog.LevelInfo},
		{long[:toolsLineMax], slog.LevelInfo},
	}
	if !slices.Equal(lines, want) {
		t.Errorf("lines\n%v\nwant\n%v", lines, want)
	}
	if last != 7 || len(got) == 0 {
		t.Errorf("last %d, %d additions", last, len(got))
	}
	if es, _ := l.since(5); len(es) != 2 || es[0].Seq != 6 {
		t.Errorf("since 5: %+v", es)
	}

	// The ring keeps the last toolsLogMax entries.
	for range toolsLogMax {
		l.add([]toolsLogEntry{{Msg: "more"}})
	}
	es, last = l.since(0)
	if len(es) != toolsLogMax || es[0].Seq != last-toolsLogMax+1 || es[len(es)-1].Seq != last {
		t.Errorf("after overflow: %d entries, %d..%d, last %d", len(es), es[0].Seq, es[len(es)-1].Seq, last)
	}
}

// A dev warning reaches the log twice, on the child's standard error and
// among its tree's entries, in either order: the log keeps the tree's
// entry (level, window) once, and a line with no twin after the wait.
func TestToolsLogTwins(t *testing.T) {
	ts := newToolsServer("./app", "", t.TempDir(), func(string, ...any) {})
	ts.build = 2
	w := ts.log.stderr(2, nil)
	app := func(msg string) {
		raw, _ := json.Marshal(core.LogEntry{Seq: 1, At: time.Now(), Level: slog.LevelWarn, Msg: msg})
		ts.appLogs([]devloop.ToolsLogBatch{{Window: 1, Entries: []json.RawMessage{raw}}})
	}
	// Standard error first, then the tree's entry.
	w.Write([]byte("tela: Item has no key\n"))
	app("tela: Item has no key")
	// The tree's entry first, then standard error; a message of two lines.
	app("tela: unknown class \"p-13\"\n  did you mean \"p-12\"?")
	w.Write([]byte("tela: unknown class \"p-13\"\n  did you mean \"p-12\"?\n"))
	// A line of the app's own, and one of another build that looks the same.
	w.Write([]byte("listening on :8080\n"))
	ts.log.stderr(1, nil).Write([]byte("tela: Item has no key\n"))
	// The lines without a twin come after the wait; nothing comes after.
	deadline := time.Now().Add(5 * time.Second)
	for {
		es, _ := ts.log.since(0)
		if len(es) >= 4 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(600 * time.Millisecond) // twice the wait (toolsLogGrace)
	es, _ := ts.log.since(0)
	var got []string
	for _, e := range es {
		got = append(got, fmt.Sprintf("%s %d %s", e.Source, e.Build, strings.ReplaceAll(e.Msg, "\n", "|")))
	}
	slices.Sort(got)
	want := []string{
		`app 2 tela: Item has no key`,
		`app 2 tela: unknown class "p-13"|  did you mean "p-12"?`,
		`stderr 1 tela: Item has no key`,
		`stderr 2 listening on :8080`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("entries\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The supervisor's own lines are in the log, a failed build as an error.
func TestToolsLogSupervisorLines(t *testing.T) {
	s := newSupervisor(supervisorConfig{opts: devOptions{Pkg: "./app"}, log: io.Discard, stdout: io.Discard, stderr: io.Discard,
		tools: true, toolsDir: t.TempDir()})
	s.log.printf("building…")
	terminalView{s.log}.show(problem{kind: problemBuild, summary: "build failed", text: "main.go:3:1: syntax error"})
	es, _ := s.tools.log.since(0)
	if len(es) != 2 || es[0].Source != "tela dev" || es[0].Msg != "building…" || es[0].Level != slog.LevelInfo ||
		es[1].Source != "tela dev" || es[1].Level != slog.LevelError || es[1].Msg != "build failed:\nmain.go:3:1: syntax error" {
		t.Errorf("entries %+v", es)
	}
}

// waitNote reads c's notifications until one of method satisfies ok, for
// at most devWait; it returns its parameters.
func waitNote(t *testing.T, c *toolsClient, method string, ok func(json.RawMessage) bool) json.RawMessage {
	t.Helper()
	timeout := time.After(devWait)
	for {
		select {
		case n, open := <-c.Notes():
			if !open {
				t.Fatalf("the connection ended while waiting for %s", method)
			}
			if n.Method == method && (ok == nil || ok(n.Params)) {
				return n.Params
			}
		case <-timeout:
			t.Fatalf("no %s notification", method)
		}
	}
}

// waitBuild waits until the session shows build gen (info's build, which
// the supervisor sets when it promoted the build: the first frame comes
// before).
func waitBuild(t *testing.T, c *toolsClient, gen int) {
	t.Helper()
	deadline := time.Now().Add(devWait)
	for {
		info, err := c.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Build == gen {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the session shows build %d, want %d", info.Build, gen)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestDevToolsRelay drives the tools socket of a supervisor whose fake
// children answer the DevTools kinds (dev_fake_test.go): window-level calls
// answered by the supervisor, tree-level calls relayed to the active child
// with the instrumentation turned on and off, input injected into the
// host, notifications, and a reload.
func TestDevToolsRelay(t *testing.T) {
	dir := t.TempDir()
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.tools, c.toolsDir = true, dir })
	h.fb.set(1, "tools")
	h.fb.set(2, "tools")
	fake := make(chan string, 256)
	for _, kind := range []string{"fake.instrument", "fake.key", "fake.focus"} {
		h.s.host.Handle(kind, func(c *remote.Child, p []byte) ([]byte, error) {
			fake <- c.Tag() + " " + kind + " " + string(p)
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
	if err != nil || s.PID != os.Getpid() || s.Pkg != "./fake" {
		t.Fatalf("session %+v, %v", s, err)
	}
	c, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	waitBuild(t, c, 1)

	info, err := c.Info(ctx)
	if err != nil || info.Pkg != "./fake" || info.Build != 1 || info.Protocol != toolsProtocol {
		t.Errorf("info %+v, %v", info, err)
	}
	ws, err := c.Windows(ctx)
	if err != nil || len(ws) != 1 || ws[0] != (toolsWindow{ID: 1, Title: "fake 1", Width: 100, Height: 80, Scale: 1, Visible: true, Active: true}) {
		t.Fatalf("windows %+v, %v", ws, err)
	}
	if err := c.Subscribe(ctx, "frames", "reload", "logs"); err != nil {
		t.Fatal(err)
	}
	if err := c.Subscribe(ctx, "nope"); err == nil || !strings.Contains(err.Error(), `unknown event "nope"`) {
		t.Errorf("subscribe to an unknown event: %v", err)
	}

	// Tree-level calls: the instrumentation turns on first.
	els, err := c.Elements(ctx, 0, false)
	if err != nil || len(els) != 1 || els[0].Text != "window 0 build 1" {
		t.Fatalf("elements %+v, %v", els, err)
	}
	expectFake("1 fake.instrument true")
	var fr toolsFrames
	json.Unmarshal(waitNote(t, c, "frames", nil), &fr)
	if fr.Window != 1 || len(fr.Profiles) != 1 || fr.Profiles[0].Frame != 1 || !slices.Equal(fr.Profiles[0].BuiltIDs, []core.ElementID{1}) {
		t.Errorf("frames %+v", fr)
	}
	el, err := c.Element(ctx, 1, 1)
	if err != nil || len(el.Recent) != 1 || el.Recent[0].Reasons[0].Cause != core.CauseState {
		t.Errorf("element %+v, %v", el, err)
	}
	if _, err := c.Element(ctx, 1, 99); err == nil || !strings.Contains(err.Error(), "no element 99") {
		t.Errorf("an unknown element: %v", err)
	}
	if sem, err := c.Semantics(ctx, 1); err != nil || !slices.Equal(sem.Elements, []core.ElementID{1}) || !strings.Contains(sem.YAML, "button") {
		t.Errorf("semantics %+v, %v", sem, err)
	}
	if in, err := c.Inspect(ctx, 1, 0); err != nil || in.Kind != "root" {
		t.Errorf("inspect %+v, %v", in, err)
	}
	if ps, err := c.Profiles(ctx, 1, 5); err != nil || len(ps) != 1 || ps[0].Frame != 6 {
		t.Errorf("profiles %+v, %v", ps, err)
	}
	if err := c.Highlight(ctx, 1, 1); err != nil {
		t.Error(err)
	}
	if err := c.call(ctx, "nope", nil, nil); err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Errorf("an unknown method: %v", err)
	}
	es, _, err := c.Logs(ctx, 0)
	if err != nil || !slices.ContainsFunc(es, func(e toolsLogEntry) bool {
		return e.Source == "app" && e.Msg == "fake warning" && e.Level == slog.LevelWarn && e.Window == 1 && e.Build == 1
	}) {
		t.Errorf("logs %+v, %v", es, err)
	}

	// Input, through the host as the real window's.
	if err := c.Click(ctx, 0, 10, 10); err != nil {
		t.Fatal(err)
	}
	h.expect("an injected click", 1, 1, 1)
	if err := c.Pick(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Click(ctx, 1, 20, 20); err != nil {
		t.Fatal(err)
	}
	var picked toolsPicked
	json.Unmarshal(waitNote(t, c, "picked", nil), &picked)
	if picked.Window != 1 || picked.ID != 7 {
		t.Errorf("picked %+v", picked)
	}
	if err := c.Text(ctx, 0, "hi"); err != nil {
		t.Fatal(err)
	}
	expectFake("1 fake.focus true") // the window had no focus: the app is lent it
	expectFake("1 fake.key h|h")
	expectFake("1 fake.key i|i")
	expectFake("1 fake.focus false") // and gets it back once the keys are typed
	focused := func() bool {
		t.Helper()
		ws, err := c.Windows(ctx)
		if err != nil || len(ws) != 1 {
			t.Fatalf("windows %+v, %v", ws, err)
		}
		return ws[0].Focused
	}
	if focused() {
		t.Error("the window is focused after typing into it; the OS never focused it")
	}
	if err := c.Key(ctx, 0, "Control+s", ""); err != nil {
		t.Fatal(err)
	}
	expectFake("1 fake.focus true")
	expectFake("1 fake.key Control|")
	expectFake("1 fake.key s|s")
	expectFake("1 fake.focus false")
	// A key held across calls keeps the lent focus until its release.
	if err := c.Key(ctx, 0, "Shift", "press"); err != nil {
		t.Fatal(err)
	}
	expectFake("1 fake.focus true")
	expectFake("1 fake.key Shift|")
	if !focused() {
		t.Error("the focus went back while Shift is held")
	}
	if err := c.Key(ctx, 0, "Shift", "release"); err != nil {
		t.Fatal(err)
	}
	expectFake("1 fake.focus false")
	if focused() {
		t.Error("the window is focused after Shift's release")
	}
	if err := c.Key(ctx, 0, "Ctrl+s", ""); err == nil || !strings.Contains(err.Error(), `"Ctrl" is not a modifier`) {
		t.Errorf("a bad combination: %v", err)
	}
	if err := c.Scroll(ctx, 0, 5, 5, 0, 30); err != nil {
		t.Error(err)
	}
	if err := c.Pointer(ctx, 0, "fling", 1, 1, ""); err == nil {
		t.Error("an unknown pointer action was accepted")
	}
	img, err := c.Screenshot(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b != image.Rect(0, 0, 100, 80) {
		t.Errorf("screenshot of %v", b)
	}
	if r, g, b, _ := img.At(1, 1).RGBA(); r>>8 != 1 || g>>8 != 1 || b>>8 != 1 {
		t.Errorf("screenshot shows %v, want build 1 with one click", img.At(1, 1))
	}
	if _, err := c.Screenshot(ctx, 9); err == nil || !strings.Contains(err.Error(), "no window 9") {
		t.Errorf("a screenshot of no window: %v", err)
	}

	// A second client holds the instrumentation too.
	c2, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := c2.Instrument(ctx, true); err != nil {
		t.Fatal(err)
	}

	// A reload: the new child is instrumented, the clients are told.
	h.s.push(evKey{'r'})
	var rl struct{ Build int }
	json.Unmarshal(waitNote(t, c, "reload", nil), &rl)
	if rl.Build != 2 {
		t.Errorf("reload %+v", rl)
	}
	expectFake("2 fake.instrument true")
	if els, err := c.Elements(ctx, 1, false); err != nil || len(els) != 1 || els[0].Text != "window 1 build 2" {
		t.Errorf("elements after the reload %+v, %v", els, err)
	}

	// The instrumentation turns off when the last client goes.
	c.Close()
	<-c.Done()
	if err := c2.Instrument(ctx, false); err != nil {
		t.Fatal(err)
	}
	expectFake("2 fake.instrument false")
	if err := c2.Unsubscribe(ctx, "frames"); err != nil {
		t.Error(err)
	}
	c2.Close()

	// The discovery file goes with the session.
	h.s.push(evKey{'q'})
	h.wait()
	if ss, _ := listDevSessions(dir); len(ss) != 0 {
		t.Errorf("sessions left after quit: %+v", ss)
	}
	if _, err := net.Dial("unix", s.Addr); err == nil {
		t.Error("the tools socket still answers after quit")
	}
}

// TestDevToolsEndToEnd attaches a client to the tools socket of a real tela
// dev of testdata/devapp (dev_e2e_test.go): it lists the windows and the
// elements, injects a click and finds the frame that rebuilt the counter
// with its cause, picks a button with an injected click, types into a
// field, and sees a reload after a source edit, with the instrumentation
// on in the new build.
func TestDevToolsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	e := startE2E(t, func(c *supervisorConfig) { c.tools, c.toolsDir = true, dir })
	e.expect(0, "devapp: counter", "devapp: counter n=0 pkg=0")
	e.until("the first frame", func() bool { return e.window().Frame() != nil })
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

	ws, err := c.Windows(ctx)
	if err != nil || len(ws) != 1 || ws[0].Title != "devapp" || ws[0].Width != 480 || ws[0].Height != 400 || !ws[0].Active {
		t.Fatalf("windows %+v, %v", ws, err)
	}
	win := ws[0].ID
	if err := c.Subscribe(ctx, "picked", "frames", "reload"); err != nil {
		t.Fatal(err)
	}
	els, err := c.Elements(ctx, win, false)
	if err != nil {
		t.Fatal(err)
	}
	find := func(els []core.ElementInfo, ok func(core.ElementInfo) bool) core.ElementInfo {
		t.Helper()
		i := slices.IndexFunc(els, ok)
		if i < 0 {
			t.Fatalf("no such element among %d", len(els))
		}
		return els[i]
	}
	counter := find(els, func(el core.ElementInfo) bool { return el.Kind == "main.Counter" && el.Component })
	label := find(els, func(el core.ElementInfo) bool { return el.Kind == "text" && el.Text == "Increment" })
	button := find(els, func(el core.ElementInfo) bool { return el.ID == label.Parent })
	if button.Kind != "button" || button.Bounds == nil {
		t.Fatalf("the parent of the Increment text: %+v", button)
	}
	cx, cy := (button.Bounds.X0+button.Bounds.X1)/2, (button.Bounds.Y0+button.Bounds.Y1)/2
	if cy < 48 || cy > 88 {
		t.Errorf("the Increment button is at %v; devapp says y 48 to 88", *button.Bounds)
	}
	// The layout of the root a level at a time, and the element list as
	// changes from a version.
	root, err := c.InspectDepth(ctx, win, 0, 0)
	if err != nil || root.Kind != "root" || len(root.Children) != 0 || root.More == 0 {
		t.Errorf("inspect of the root at depth 0: %d children, more %d, %v", len(root.Children), root.More, err)
	}
	full, err := c.ElementsSince(ctx, win, 0)
	if err != nil || !full.Full || full.Version == 0 || len(full.Elements) != len(els) {
		t.Fatalf("elements since 0: full %v, version %d, %d of %d, %v", full.Full, full.Version, len(full.Elements), len(els), err)
	}
	ps, err := c.Profiles(ctx, win, 0)
	if err != nil {
		t.Fatal(err)
	}
	since := 0
	if len(ps) > 0 {
		since = ps[len(ps)-1].Frame
	}

	// A click through the host rebuilds the counter, for its state.
	if err := c.Click(ctx, win, cx, cy); err != nil {
		t.Fatal(err)
	}
	e.expect(0, "devapp: counter", "devapp: counter n=1 pkg=1")
	var frame core.FrameProfile
	e.until("the frame of the click", func() bool {
		ps, err := c.Profiles(ctx, win, since)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range ps {
			if slices.Contains(p.BuiltIDs, counter.ID) {
				frame = p
				return true
			}
		}
		return false
	})
	if frame.Built == 0 || frame.Duration <= 0 {
		t.Errorf("the frame of the click: %+v", frame)
	}
	// Only what the click changed comes, the counter's text among it.
	d, err := c.ElementsSince(ctx, win, full.Version)
	if err != nil || d.Full || d.Version == full.Version || len(d.Elements) == 0 || len(d.Elements) >= len(full.Elements)/2 ||
		!slices.ContainsFunc(d.Elements, func(e core.ElementInfo) bool { return e.ID == counter.ID }) {
		t.Errorf("elements since the click: full %v, version %d (was %d), %d of %d elements, %v", d.Full, d.Version, full.Version, len(d.Elements), len(full.Elements), err)
	}
	info, err := c.Element(ctx, win, counter.ID)
	if err != nil || len(info.Recent) == 0 {
		t.Fatalf("the counter: %+v, %v", info, err)
	}
	last := info.Recent[len(info.Recent)-1]
	if last.Frame != frame.Frame || len(last.Reasons) == 0 || last.Reasons[0].Cause != core.CauseState || last.Reasons[0].HookKind != "UseState" {
		t.Errorf("why the counter built in frame %d: %+v", frame.Frame, last)
	}
	waitNote(t, c, "frames", nil)

	// Pick mode: the click picks the Sidebar button and does not reach it.
	if err := c.Pick(ctx, win, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Click(ctx, win, 60, 148); err != nil {
		t.Fatal(err)
	}
	var picked toolsPicked
	json.Unmarshal(waitNote(t, c, "picked", nil), &picked)
	if picked.Window != win || picked.ID == 0 || picked.Element == nil || picked.Element.Kind != "button" {
		t.Errorf("picked %+v (element %+v)", picked, picked.Element)
	}
	if strings.Contains(e.out.String(), "devapp: sidebar n=1") {
		t.Error("the click that picked reached the app")
	}

	sem, err := c.Semantics(ctx, win)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(sem.YAML, "\n"), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, `button "Increment"`) })
	if i < 0 || len(sem.Elements) != len(lines) || sem.Elements[i] != button.ID {
		t.Errorf("semantics: line %d of\n%s\nelements %v, want the button %d", i, sem.YAML, sem.Elements, button.ID)
	}
	img, err := c.Screenshot(ctx, win)
	if err != nil || img.Bounds().Dx() != 480 || img.Bounds().Dy() != 400 {
		t.Errorf("screenshot %v, %v", img.Bounds(), err)
	}

	// Typing: the field takes the focus from a click, the keys follow.
	if err := c.Click(ctx, win, 60, 188); err != nil {
		t.Fatal(err)
	}
	if err := c.Text(ctx, win, "hi"); err != nil {
		t.Fatal(err)
	}
	e.expect(0, "devapp: input", `devapp: input "hi"`)

	// A reload: the clients are told, and the new build is instrumented.
	_, mark := e.edit(func(s string) string { return strings.Replace(s, `Text("Version A")`, `Text("Version B")`, 1) })
	var rl struct{ Build int }
	json.Unmarshal(waitNote(t, c, "reload", nil), &rl)
	if rl.Build < 2 {
		t.Errorf("reload %+v", rl)
	}
	e.expect(mark, "devapp: counter", "devapp: counter n=1 pkg=1")
	els, err = c.Elements(ctx, win, false)
	if err != nil || !slices.ContainsFunc(els, func(el core.ElementInfo) bool { return el.Kind == "text" && el.Text == "Version B" }) {
		t.Errorf("elements after the reload: %d, %v", len(els), err)
	}
	// The new child started instrumented: its first build is recorded.
	counter = find(els, func(el core.ElementInfo) bool { return el.Kind == "main.Counter" && el.Component })
	if info, err = c.Element(ctx, win, counter.ID); err != nil || len(info.Recent) == 0 || info.Recent[0].Frame > 1 || len(info.Recent[0].Reasons) == 0 ||
		(info.Recent[0].Reasons[0].Cause != core.CauseReload && info.Recent[0].Reasons[0].Cause != core.CauseMount) {
		t.Errorf("the counter's first build in the new child is not recorded: %+v, %v", info.Recent, err)
	}
	// The dev warning about Mixed's func slot is printed on the child's
	// standard error and kept by its instrumented tree: the log has it once.
	// (tela dev's own report of the slot's reset, "state: … resets", is
	// another line, from the supervisor.)
	es, _, err := c.Logs(ctx, 0)
	var twins []toolsLogEntry
	for _, e := range es {
		if e.Source != "tela dev" && strings.Contains(e.Msg, "holds a func") {
			twins = append(twins, e)
		}
	}
	if err != nil || len(twins) != 1 {
		t.Errorf("the logs have %d entries about Mixed's func slot, want 1 (%v):\n%+v", len(twins), err, es)
	}
	// The supervisor's own lines are there too.
	if !slices.ContainsFunc(es, func(e toolsLogEntry) bool { return e.Source == "tela dev" && strings.Contains(e.Msg, "reloaded") }) {
		t.Errorf("the logs have no line of tela dev's own about the reload:\n%+v", es)
	}
}
