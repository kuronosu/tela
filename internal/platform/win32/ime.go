//go:build windows

package win32

import "github.com/kuronosu/tela/internal/platform"

// SetTextInput validates the state and otherwise does nothing yet: IMM32
// (WM_IME_* messages, ImmSetCompositionWindow, ImmSetCandidateWindow) is
// mapped onto the contract later in phase 5, as package platform describes
// ("Text input and IME").
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
}

// ResetTextInput does nothing until IMM32 is supported.
func (w *Window) ResetTextInput() {}
