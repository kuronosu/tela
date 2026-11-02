//go:build darwin || linux

package cocoa

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The lock file and socket half of single instance, without AppKit: two
// claims in one process behave as two processes (flock locks conflict
// between open files, not processes). Every wait is bounded.

// instanceDir is a short folder for the files (t.TempDir can be longer
// than a socket path allows on macOS).
func instanceDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "ti")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func waitLaunch(t *testing.T, ch <-chan platform.Launch) (platform.Launch, bool) {
	t.Helper()
	select {
	case l := <-ch:
		return l, true
	case <-time.After(5 * time.Second):
		t.Errorf("no launch reached the first instance within 5 s")
		return platform.Launch{}, false
	}
}

func TestInstanceClaimAndForward(t *testing.T) {
	dir := instanceDir(t)
	got := make(chan platform.Launch, 4)
	first, primary, err := claimInstance(dir, "dev.telaui.Test", platform.Launch{}, func(l platform.Launch) bool { got <- l; return true }, nil)
	if err != nil || !primary {
		t.Fatalf("first claim = %v, %v; want primary", primary, err)
	}
	defer first.close()
	_, sock, _ := instancePaths(dir, "dev.telaui.Test")
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm() != 0o600 || fi.Mode().Type() != fs.ModeSocket {
		t.Errorf("the socket: %v, %v; want a socket of mode 0600", fi, err)
	}

	want := platform.Launch{Args: []string{"notes:open?id=3", "a b.txt"}, Dir: "/Users/me", Token: "tok"}
	var helloPID int
	second, primary, err := claimInstance(dir, "dev.telaui.Test", want, nil, func(pid int) { helloPID = pid })
	if err != nil || primary || second != nil {
		t.Fatalf("second claim = %v, %v, %v; want forwarded", second, primary, err)
	}
	if helloPID != os.Getpid() {
		t.Errorf("hello gave PID %d, want %d", helloPID, os.Getpid())
	}
	if l, ok := waitLaunch(t, got); ok && !reflect.DeepEqual(l, want) {
		t.Errorf("forwarded launch = %+v, want %+v", l, want)
	}

	// Another app ID is another instance.
	other, primary, err := claimInstance(dir, "dev.telaui.Other", platform.Launch{}, nil, nil)
	if err != nil || !primary {
		t.Errorf("claim of another ID = %v, %v; want primary", primary, err)
	}
	other.close()

	// Closing ends the claim: the socket goes, the next claim is primary.
	first.close()
	first.close() // idempotent
	if _, err := os.Lstat(sock); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket after close: %v; want it removed", err)
	}
	again, primary, err := claimInstance(dir, "dev.telaui.Test", platform.Launch{}, nil, nil)
	if err != nil || !primary {
		t.Errorf("claim after close = %v, %v; want primary", primary, err)
	}
	again.close()
}

// TestInstanceStaleSocket: a first process that crashed leaves its socket
// file (and an unlocked lock file); the next claim replaces them.
func TestInstanceStaleSocket(t *testing.T) {
	dir := instanceDir(t)
	lock, sock, _ := instancePaths(dir, "dev.telaui.Stale")
	if err := os.WriteFile(lock, []byte("12345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ln.SetUnlinkOnClose(false)
	ln.Close()
	if _, err := os.Lstat(sock); err != nil {
		t.Fatalf("the stale socket is missing: %v", err)
	}
	got := make(chan platform.Launch, 1)
	c, primary, err := claimInstance(dir, "dev.telaui.Stale", platform.Launch{}, func(l platform.Launch) bool { got <- l; return true }, nil)
	if err != nil || !primary {
		t.Fatalf("claim over a stale socket = %v, %v; want primary", primary, err)
	}
	defer c.close()
	if _, primary, err := claimInstance(dir, "dev.telaui.Stale", platform.Launch{Args: []string{"x"}}, nil, nil); err != nil || primary {
		t.Errorf("second claim = %v, %v; want forwarded", primary, err)
	}
	if l, ok := waitLaunch(t, got); ok && !reflect.DeepEqual(l.Args, []string{"x"}) {
		t.Errorf("forwarded launch = %+v", l)
	}
}

// TestInstanceLockHeldWithoutListener: while the lock's holder does not
// listen, a claim retries, and takes the claim once the holder exits.
func TestInstanceLockHeldWithoutListener(t *testing.T) {
	dir := instanceDir(t)
	lock, _, _ := instancePaths(dir, "dev.telaui.Starting")
	f, err := os.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(300*time.Millisecond, func() { f.Close() }) // the holder exits
	start := time.Now()
	c, primary, err := claimInstance(dir, "dev.telaui.Starting", platform.Launch{}, nil, nil)
	if err != nil || !primary {
		t.Fatalf("claim = %v, %v after %v; want primary once the lock is free", primary, err, time.Since(start))
	}
	c.close()
	t.Logf("claimed after %v", time.Since(start).Round(time.Millisecond))
}

// TestInstanceHostilePeer: a connection that sends garbage or nothing does
// not stop the first instance from serving the next one.
func TestInstanceHostilePeer(t *testing.T) {
	dir := instanceDir(t)
	got := make(chan platform.Launch, 1)
	c, _, err := claimInstance(dir, "dev.telaui.Peer", platform.Launch{}, func(l platform.Launch) bool { got <- l; return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	_, sock, _ := instancePaths(dir, "dev.telaui.Peer")
	idle, err := net.Dial("unix", sock) // connects and says nothing
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	bad, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = bad.Write([]byte{0xff, 0xff, 0xff, 0xff, 1, 2})
	bad.Close()
	if _, primary, err := claimInstance(dir, "dev.telaui.Peer", platform.Launch{Args: []string{"ok"}}, nil, nil); err != nil || primary {
		t.Errorf("claim after hostile peers = %v, %v; want forwarded", primary, err)
	}
	if l, ok := waitLaunch(t, got); ok && !reflect.DeepEqual(l.Args, []string{"ok"}) {
		t.Errorf("forwarded launch = %+v", l)
	}
}

// TestInstanceRefusedLaunch: a launch the first instance does not take
// (it is shutting down) is not acknowledged, so the second process gets
// an error, runs on its own and loses nothing.
func TestInstanceRefusedLaunch(t *testing.T) {
	dir := instanceDir(t)
	c, _, err := claimInstance(dir, "dev.telaui.Refuse", platform.Launch{}, func(platform.Launch) bool { return false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	_, primary, err := claimInstance(dir, "dev.telaui.Refuse", platform.Launch{Args: []string{"x"}}, nil, nil)
	if err == nil || primary {
		t.Errorf("claim whose launch was refused = %v, %v; want an error", primary, err)
	} else {
		t.Logf("refused: %v", err)
	}
}
