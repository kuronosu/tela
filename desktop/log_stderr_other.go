//go:build !windows

package desktop

import "os"

// stderrUsable reports whether the process has a standard error to write
// to: it exists unless the program closed it.
func stderrUsable() bool {
	_, err := os.Stderr.Stat()
	return err == nil
}
