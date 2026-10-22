//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package term

import "golang.org/x/sys/unix"

func isTerminal(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), ioctlGetTermios)
	return err == nil
}

func cbreak(fd uintptr) (func() error, error) {
	old, err := unix.IoctlGetTermios(int(fd), ioctlGetTermios)
	if err != nil {
		return nil, err
	}
	t := *old
	t.Lflag &^= unix.ICANON | unix.ECHO
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(int(fd), ioctlSetTermios, &t); err != nil {
		return nil, err
	}
	return func() error { return unix.IoctlSetTermios(int(fd), ioctlSetTermios, old) }, nil
}
