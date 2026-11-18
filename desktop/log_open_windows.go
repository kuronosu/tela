//go:build windows

package desktop

import (
	"os"
	"syscall"
)

// fileReadAttributes is FILE_READ_ATTRIBUTES, which (*os.File).Stat needs;
// package syscall does not name it.
const fileReadAttributes = 0x80

// openLogFile opens the log file at path for appending, creating it, as
// os.OpenFile(path, O_WRONLY|O_CREATE|O_APPEND) does (FILE_APPEND_DATA
// without FILE_WRITE_DATA: every write goes to the end), but shared for
// deletion too (FILE_SHARE_DELETE), which os.OpenFile never asks for
// (golang/go#32088). Without it, Windows refuses to rename or delete the
// file while this process holds it: another process of the app (a second
// instance, the new version update.Restart starts) could not rotate it,
// and a test's temporary folder could not be removed. With it, Windows
// behaves as Unix does: the file can be renamed and removed while open,
// and this process goes on writing to it under its new name.
func openLogFile(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(p,
		syscall.FILE_APPEND_DATA|fileReadAttributes|syscall.SYNCHRONIZE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
