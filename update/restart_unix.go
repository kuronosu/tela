//go:build !windows

package update

import (
	"fmt"
	"os"
	"syscall"
)

// startProgramOS replaces the process with the program at exe, given this
// process's arguments and environment. It returns only on failure.
func startProgramOS(exe string) error {
	args := append([]string{exe}, os.Args[1:]...)
	if err := syscall.Exec(exe, args, os.Environ()); err != nil {
		return fmt.Errorf("update: starting %s: %w", exe, err)
	}
	return nil
}
