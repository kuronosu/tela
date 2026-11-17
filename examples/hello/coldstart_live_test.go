//go:build linux

package main

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestLiveColdStart times cold starts, the budget of ARCHITECTURE §18: from
// the start of a fresh process to its main window on screen, for release
// builds (-trimpath, so dev mode is off) of examples/hello and
// examples/kit on Wayland and on X11 (Xwayland, DISPLAY=:0), and of
// examples/hello built with the tag telagpu on each GPU (the difference
// from the same binary with TELA_RENDERER=cpu is what opening the device
// costs the UI thread before the first frame).
//
//	TELA_LIVE=1 go test -run TestLiveColdStart -v ./examples/hello
//
// It needs Hyprland: the time on screen is Hyprland's openwindow event,
// which its event socket sends when it maps the window, and a backend
// maps a window at its first Present. The clock starts right before the
// fork (a millisecond or so before the process's own start time, which
// /proc gives only to 10 ms). The binaries go under the repository's
// ignored bin/ (Hyprland does not see processes started from a sandbox's
// scratch directory). Put the windows out of the way first, for example:
//
//	hyprctl eval 'hl.window_rule({ name = "tela-coldstart", match = { title = "^(Tela|Tela - kit)$" }, workspace = "7 silent" })'
//
// Each configuration runs once to warm the page cache, then seven times;
// the test logs every run and the median. It sends the windows no input
// and ends each process with SIGTERM.
func TestLiveColdStart(t *testing.T) {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if os.Getenv("TELA_LIVE") == "" || sig == "" {
		t.Skip("a live measurement on Hyprland: set TELA_LIVE=1 in a Hyprland session")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	build := func(name, pkg string, tags ...string) string {
		out := filepath.Join(root, "bin", "coldstart-"+name)
		args := []string{"build", "-trimpath", "-o", out}
		if len(tags) > 0 {
			args = append(args, "-tags", strings.Join(tags, ","))
		}
		cmd := exec.Command("go", append(args, pkg)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", pkg, err, b)
		}
		return out
	}
	hello := build("hello", "./examples/hello")
	kit := build("kit", "./examples/kit")
	helloGPU := build("hello-gpu", "./examples/hello", "telagpu")

	ev := hyprEvents(t, filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "hypr", sig, ".socket2.sock"))
	cases := []struct {
		name, bin, title string
		env              []string
	}{
		{"hello/wayland", hello, "Tela", []string{"TELA_BACKEND=wayland"}},
		{"hello/x11", hello, "Tela", []string{"TELA_BACKEND=x11", "DISPLAY=:0"}},
		{"kit/wayland", kit, "Tela - kit", []string{"TELA_BACKEND=wayland"}},
		{"kit/x11", kit, "Tela - kit", []string{"TELA_BACKEND=x11", "DISPLAY=:0"}},
		{"hello-telagpu/wayland/cpu", helloGPU, "Tela", []string{"TELA_BACKEND=wayland", "TELA_RENDERER=cpu"}},
		{"hello-telagpu/wayland/NVIDIA", helloGPU, "Tela", []string{"TELA_BACKEND=wayland", "TELA_GPU_DEVICE=NVIDIA"}},
		{"hello-telagpu/wayland/Intel", helloGPU, "Tela", []string{"TELA_BACKEND=wayland", "TELA_GPU_DEVICE=Intel"}},
	}
	const runs = 7
	for _, c := range cases {
		var ds []time.Duration
		for i := range runs + 1 {
			d := coldStart(t, ev, c.bin, c.title, c.env)
			if i > 0 { // the first run warms the page cache
				ds = append(ds, d)
			}
		}
		sorted := slices.Clone(ds)
		slices.Sort(sorted)
		var parts []string
		for _, d := range ds {
			parts = append(parts, d.Round(100*time.Microsecond).String())
		}
		t.Logf("%-30s median %v (runs: %s)", c.name, sorted[len(sorted)/2].Round(100*time.Microsecond), strings.Join(parts, " "))
	}
}

// hyprEvent is a line of Hyprland's event socket and when it was read.
type hyprEvent struct {
	at   time.Time
	line string
}

// hyprEvents reads Hyprland's event socket until the test ends.
func hyprEvents(t *testing.T, path string) <-chan hyprEvent {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("Hyprland's event socket: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	ch := make(chan hyprEvent, 256)
	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			ch <- hyprEvent{time.Now(), sc.Text()}
		}
		close(ch)
	}()
	return ch
}

// coldStart starts bin with env and returns the time until Hyprland maps
// its window titled title, then ends the process and waits for the window
// to close.
func coldStart(t *testing.T, ev <-chan hyprEvent, bin, title string, env []string) time.Duration {
	t.Helper()
	// Events of earlier runs are not this one's.
	for drained := false; !drained; {
		select {
		case <-ev:
		default:
			drained = true
		}
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	}()
	timeout := time.After(20 * time.Second)
	var addr string
	var took time.Duration
	for addr == "" {
		select {
		case e, ok := <-ev:
			if !ok {
				t.Fatal("Hyprland's event socket closed")
			}
			// openwindow>>ADDRESS,WORKSPACE,CLASS,TITLE
			rest, ok := strings.CutPrefix(e.line, "openwindow>>")
			if f := strings.SplitN(rest, ",", 4); ok && len(f) == 4 && f[3] == title {
				addr, took = f[0], e.at.Sub(t0)
			}
		case <-timeout:
			t.Fatalf("%s: no window %q on screen after 20 s", bin, title)
		}
	}
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()
	for closed := false; !closed; {
		select {
		case e, ok := <-ev:
			closed = !ok || e.line == "closewindow>>"+addr
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the window %s did not close", bin, addr)
		}
	}
	time.Sleep(300 * time.Millisecond)
	return took
}
