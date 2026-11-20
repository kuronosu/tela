//go:build !windows

// Windows has no owner and mode bits to check: the socket's directory
// inherits the per-user temporary directory's ACL.

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A discovery file is trusted only when its socket's directory is private
// to the user: a directory others can enter, or a symbolic link, is
// refused with a clear error.
func TestToolsDiscoveryTrust(t *testing.T) {
	base := t.TempDir()
	open := filepath.Join(base, "open")
	os.Mkdir(open, 0o755)
	os.Chmod(open, 0o755) // whatever the umask
	private := filepath.Join(base, "private")
	os.Mkdir(private, 0o700)
	link := filepath.Join(base, "link")
	if err := os.Symlink(private, link); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", filepath.Join(open, "tools.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	for _, d := range []string{open, link} {
		files := filepath.Join(t.TempDir(), "dev")
		path, err := writeDevSession(files, devSession{Addr: filepath.Join(d, "tools.sock"), Pkg: "./app", PID: os.Getpid(), Started: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := findDevSession(files); err == nil || !strings.Contains(err.Error(), "not private") {
			t.Errorf("a socket in %s: %v; want a refusal", filepath.Base(d), err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the discovery file was removed: %v", err)
		}
	}
	if err := checkSocketDir(filepath.Join(private, "tools.sock")); err != nil {
		t.Errorf("a private directory: %v", err)
	}
}
