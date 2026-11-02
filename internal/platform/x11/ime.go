//go:build linux

package x11

import "github.com/kuronosu/tela/internal/platform"

// SetTextInput validates the state and otherwise does nothing: the X11
// backend has no input method support yet (XIM is a task of the Linux and
// Windows parity phase), so no IMEEvent or IMESessionEvent is delivered and
// dead keys and Compose sequences keep working through KeyEvent.Text.
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
}

// ResetTextInput does nothing: see SetTextInput.
func (w *Window) ResetTextInput() {}
