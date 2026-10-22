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
// killing them while a swap may be in flight. A background group must not
// touch the terminal: reading it stops the group with SIGTTIN, and so does
// writing to it with SIGTTOU under stty tostop. So the children's standard
// input is the null device and their output goes through pipes that tela
// dev copies to the terminal (ADR-119).
func childSysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
