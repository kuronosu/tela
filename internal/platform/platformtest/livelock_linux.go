//go:build linux

package platformtest

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

var liveLock struct {
	once sync.Once
	f    *os.File // kept open: the lock lives until the process exits
}

// LockLive serializes the live tests (real windows) of the packages that
// call it before opening their first window. `go test ./...` runs the
// test binaries of several packages at once, and windows of two packages
// on the same desktop take keyboard focus from each other: the X11
// keyboard test, typing with xdotool, lost a Shift press while a Wayland
// test's windows came and went.
//
// The first call in a process takes an exclusive flock on a file in the
// temporary directory, waiting for the process that holds it, and keeps
// it until the process exits; later calls return at once. If the file
// cannot be locked the tests run unserialized (logged).
func LockLive(t testing.TB) {
	t.Helper()
	liveLock.once.Do(func() {
		name := filepath.Join(os.TempDir(), "tela-live.lock")
		f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Logf("live tests not serialized with other packages: %v", err)
			return
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			t.Logf("waiting for the live tests of another package (%s)", name)
			err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		}
		if err != nil {
			f.Close()
			t.Logf("live tests not serialized with other packages: %v", err)
			return
		}
		liveLock.f = f
	})
}
