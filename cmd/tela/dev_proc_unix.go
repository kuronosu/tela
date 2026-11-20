//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// interruptProcess asks a process to stop (SIGINT, which the go command
// forwards to its compilers).
func interruptProcess(p *os.Process) error { return p.Signal(os.Interrupt) }

// childSysProcAttr puts a child in its own process group: Ctrl+C in the
// terminal reaches tela dev, which stops its children itself, instead of
// killing them while a swap may be in flight. A background group must not
// touch the terminal: reading it stops the group with SIGTTIN, and so does
// writing to it with SIGTTOU under stty tostop. So the children's standard
// input is the null device and their output goes through pipes that tela
// dev copies to the terminal (ADR-119).
func childSysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

// removeExe deletes the executable of a build.
func removeExe(path string) error { return os.Remove(path) }

// processAlive reports whether process pid runs (a discovery file of tela
// dev whose process ended is stale). EPERM is a process of another user:
// it runs.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// checkSocketDir checks that the directory of the tools socket at addr is
// private to this user, as toolsListen makes it: a directory, not a
// symbolic link (Lstat), owned by the user, with mode 0700. A discovery
// file is in the user's cache, but the socket it names is sent input and
// asked for screenshots: one in a directory another user can enter or
// replace could be theirs. A missing directory is an error that wraps
// os.ErrNotExist.
func checkSocketDir(addr string) error {
	dir := filepath.Dir(addr)
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("the socket's directory %s is a symbolic link, so not private to you; not connecting", dir)
	case !fi.IsDir():
		return fmt.Errorf("the socket's directory %s is not a directory, so not private to you; not connecting", dir)
	case !ok || int(st.Uid) != os.Getuid() || fi.Mode().Perm() != 0o700:
		owner := -1
		if ok {
			owner = int(st.Uid)
		}
		return fmt.Errorf("the socket's directory %s is not private to you (owner %d, mode %v; want user %d, mode 0700); not connecting", dir, owner, fi.Mode().Perm(), os.Getuid())
	}
	return nil
}

// dialGone reports whether a dial error means that no session listens at
// the address any more: it refused the connection (a socket nobody
// listens on), or the socket is gone.
func dialGone(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT)
}
