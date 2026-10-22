// Package term puts the terminal in a mode that delivers single key
// presses (tela dev's r, R and q) and restores it.
//
// On Unix it clears ICANON and ECHO with termios and keeps ISIG and output
// processing, so Ctrl+C is still a signal and "\n" still starts a new line
// for the output of the supervisor and its children (cbreak mode, not raw
// mode). On Windows it clears ENABLE_LINE_INPUT and ENABLE_ECHO_INPUT of
// the console input mode and keeps ENABLE_PROCESSED_INPUT (Ctrl+C).
package term

import (
	"errors"
	"sync"
)

// ErrUnsupported is returned where the terminal mode cannot be changed.
var ErrUnsupported = errors.New("term: not supported on this system")

// State is a terminal mode to restore.
type State struct {
	once    sync.Once
	restore func() error
	err     error
}

// Restore puts the terminal back as it was. Safe to call more than once and
// from any goroutine; only the first call acts.
func (s *State) Restore() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() { s.err = s.restore() })
	return s.err
}

// IsTerminal reports whether fd is a terminal (a console on Windows).
func IsTerminal(fd uintptr) bool { return isTerminal(fd) }

// Cbreak switches the terminal of fd to deliver key presses one by one,
// without echo, and returns the state to restore.
func Cbreak(fd uintptr) (*State, error) {
	restore, err := cbreak(fd)
	if err != nil {
		return nil, err
	}
	return &State{restore: restore}, nil
}
