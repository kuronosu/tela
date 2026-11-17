//go:build windows

package desktop

import "syscall"

// stderrUsable reports whether the process has a standard error to write
// to: a GUI-subsystem program (-ldflags=-H=windowsgui, ADR-127) started
// without redirection has none, and GetStdHandle answers 0.
func stderrUsable() bool {
	h, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	return err == nil && h != 0 && h != syscall.InvalidHandle
}
