//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || windows)

package term

func isTerminal(fd uintptr) bool { return false }

func cbreak(fd uintptr) (func() error, error) { return nil, ErrUnsupported }
