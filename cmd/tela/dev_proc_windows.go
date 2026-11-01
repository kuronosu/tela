//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// interruptProcess stops a process. Windows has no SIGINT for another
// process group that a console program can send portably, so the go
// command is killed (its compilers end with their pipes).
func interruptProcess(p *os.Process) error { return p.Kill() }

// childSysProcAttr starts a child in a new process group, so that Ctrl+C in
// the console reaches tela dev, which stops its children itself (a process
// group created this way ignores CTRL_C_EVENT).
func childSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// removeExe deletes the executable of a build. Windows refuses for a
// moment after its process exited (the image is still mapped) or while an
// antivirus scans a new file (ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION),
// so it retries for a while, as cmd/go's robustio does.
func removeExe(path string) error {
	deadline := time.Now().Add(2 * time.Second)
	for delay := time.Millisecond; ; delay = min(2*delay, 100*time.Millisecond) {
		err := os.Remove(path)
		if err == nil || !(errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
	}
}
