package win32

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of OpenURI and OpenPath (shell_windows.go calls
// ShellExecuteW).

// checkShellURI returns an error if uri cannot be given to OpenURI on
// Windows: the contract's rules (platform.CheckURI), and no one-letter
// scheme. ShellExecuteW takes "C:/Windows/notepad.exe" for a path and runs
// it, so a URI that parses with a drive letter as its scheme is refused
// like a file: URI.
func checkShellURI(uri string) error {
	if err := platform.CheckURI(uri); err != nil {
		return err
	}
	if u, err := url.Parse(uri); err == nil && len(u.Scheme) == 1 {
		return fmt.Errorf("platform: OpenURI: %q is a drive path, not a URI; open local files with OpenPath", uri)
	}
	return nil
}

// errNoApplication is wrapped by the error of ShellExecuteW's
// SE_ERR_NOASSOC: no application is registered for the file type or the
// URI scheme.
var errNoApplication = errors.New("no application is associated with it")

// shellExecuteError maps the result of ShellExecuteW, an HINSTANCE that is
// an error code when it is at most 32, to an error (nil for success).
// Missing files wrap fs.ErrNotExist and denied access fs.ErrPermission.
func shellExecuteError(code uintptr, target string) error {
	if code > 32 {
		return nil
	}
	var reason error
	switch code {
	case 0, 8: // out of memory or resources; SE_ERR_OOM
		reason = errors.New("the system is out of memory or resources")
	case 2, 3: // ERROR_FILE_NOT_FOUND / SE_ERR_FNF, ERROR_PATH_NOT_FOUND / SE_ERR_PNF
		reason = fs.ErrNotExist
	case 5: // SE_ERR_ACCESSDENIED
		reason = fs.ErrPermission
	case 11: // ERROR_BAD_FORMAT
		reason = errors.New("it is not a valid Win32 application")
	case 26: // SE_ERR_SHARE
		reason = errors.New("sharing violation")
	case 27: // SE_ERR_ASSOCINCOMPLETE
		reason = errors.New("its file association is incomplete or invalid")
	case 28, 29, 30: // SE_ERR_DDETIMEOUT, SE_ERR_DDEFAIL, SE_ERR_DDEBUSY
		reason = errors.New("the DDE transaction with the application failed")
	case 31: // SE_ERR_NOASSOC
		reason = errNoApplication
	case 32: // SE_ERR_DLLNOTFOUND
		reason = errors.New("a required DLL was not found")
	default:
		reason = fmt.Errorf("ShellExecuteW error %d", code)
	}
	return fmt.Errorf("win32: opening %q: %w", target, reason)
}
