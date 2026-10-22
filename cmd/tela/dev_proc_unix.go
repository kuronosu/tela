//go:build !windows

package main

import (
	"os"
	"syscall"
)

// interruptProcess asks a process to stop (SIGINT, which the go command
// forwards to its compilers).
func interruptProcess(p *os.Process) error { return p.Signal(os.Interrupt) }

// childSysProcAttr puts a child in its own process group: Ctrl+C in the
// terminal reaches tela dev, which stops its children itself, instead of
// killing them while a swap may be in flight. The children do not read the
// terminal (their standard input is the null device), so being in a
// background group does not stop them; only their output is shared.
func childSysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
