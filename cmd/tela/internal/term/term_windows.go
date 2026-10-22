//go:build windows

package term

import "golang.org/x/sys/windows"

func isTerminal(fd uintptr) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}

func cbreak(fd uintptr) (func() error, error) {
	h := windows.Handle(fd)
	var old uint32
	if err := windows.GetConsoleMode(h, &old); err != nil {
		return nil, err
	}
	mode := old &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
	mode |= windows.ENABLE_PROCESSED_INPUT
	if err := windows.SetConsoleMode(h, mode); err != nil {
		return nil, err
	}
	return func() error { return windows.SetConsoleMode(h, old) }, nil
}
