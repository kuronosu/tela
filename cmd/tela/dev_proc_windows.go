//go:build windows

package main

import (
	"os"
	"syscall"

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
