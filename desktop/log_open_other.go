//go:build !windows

package desktop

import "os"

// openLogFile opens the log file at path for appending, creating it.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
}
