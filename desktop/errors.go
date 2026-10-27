package desktop

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// ErrUnsupported is wrapped by the errors of services the platform or the
// session does not provide: file dialogs without an XDG Desktop Portal, a
// tray icon without a StatusNotifierWatcher (some GNOME sessions), any
// service on a backend that has none. Test for it with errors.Is and fall
// back:
//
//	err := desktop.OpenFile(onPicked, desktop.Filter("Notes", "*.md"))
//	if errors.Is(err, desktop.ErrUnsupported) {
//	    showPathField() // let the user type a path instead
//	}
//
// errors.Is(err, errors.ErrUnsupported) holds too.
var ErrUnsupported = fmt.Errorf("desktop: %w", errors.ErrUnsupported)

// wrap returns err as an error of call what: an unsupported service wraps
// ErrUnsupported (and keeps the platform's message), anything else keeps
// its chain.
func wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errors.ErrUnsupported) {
		return fmt.Errorf("desktop: %s: %w (%v)", what, ErrUnsupported, err)
	}
	return fmt.Errorf("desktop: %s: %w", what, err)
}

// running returns the running app's desktop for call what, as an error
// that says how to call it when no app runs.
func running(what string) (core.Desktop, error) {
	d, err := core.RunningDesktop("desktop." + what)
	if err != nil {
		return core.Desktop{}, err
	}
	if d.Ended() {
		return core.Desktop{}, fmt.Errorf("desktop.%s after the app quit", what)
	}
	return d, nil
}

// window returns the handle of the window a request with parent comes
// from (see core.Desktop.Window).
func window(d core.Desktop, parent tela.WindowHandle) tela.WindowHandle {
	return d.Window(parent)
}
