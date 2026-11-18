package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// startProgramOS starts the program at exe with this process's arguments,
// in its folder, and does not wait for it. Windows has no exec: the
// caller ends this process.
func startProgramOS(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update: starting %s: %w", exe, err)
	}
	return cmd.Process.Release()
}
