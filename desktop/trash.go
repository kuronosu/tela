package desktop

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// The trash (ADR-216).

// MoveToTrash moves the file or folder at path to the user's trash (the
// Recycle Bin on Windows, the Trash of the Linux desktop), where the user
// can restore it. A relative path is taken from the working directory.
// It never deletes: when the trash cannot take the file (a desktop
// without one, a file on a network drive or on another disk than the
// trash), the file stays where it is and the error wraps
// [ErrUnsupported], so the app can tell the user the file is still there.
// A path that does not exist is an error wrapping fs.ErrNotExist.
//
// Ask the user first, and call it from a goroutine: it blocks on file
// I/O and the desktop's file operation, and dev mode warns when it runs
// on the UI thread.
//
//	go func() {
//	    err := desktop.MoveToTrash(path)
//	    tela.Post(func() {
//	        switch {
//	        case err == nil:
//	            toast.Show("Moved " + filepath.Base(path) + " to the Trash")
//	        case errors.Is(err, desktop.ErrUnsupported):
//	            toast.Show("The file is still in " + filepath.Dir(path))
//	        default:
//	            toast.Error("Could not move the file", kit.ToastDescription(err.Error()))
//	        }
//	    })
//	}()
//
// It needs a running app (tela.Run or telatest). In tests, telatest's
// headless trash moves the file into a temporary folder and
// App.Trashed lists what went there; App.FailTrash and
// App.Unsupported(telatest.Trash) play the failures.
func MoveToTrash(path string) error {
	svc, err := trashServices()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if path == "" || err != nil {
		abs = path // the platform's check reports it
	}
	return wrap(fmt.Sprintf("MoveToTrash(%q)", path), svc.MoveToTrash(abs))
}

// trashServices returns the running app's services for MoveToTrash,
// warning in dev mode when the call runs on the UI thread.
func trashServices() (platform.Desktop, error) {
	svc, d, onUI, ok := core.RunningServices()
	if !ok {
		return nil, errors.New("desktop.MoveToTrash needs a running app (tela.Run, or telatest in tests)")
	}
	if onUI {
		d.WarnAnyGoroutine("desktop-trash-ui", func() string {
			return "desktop.MoveToTrash runs on the UI thread: it blocks on the file system and the desktop's file operation, freezing the app; call it from a goroutine and give the result back with tela.Post or a state setter"
		})
	}
	return svc, nil
}
