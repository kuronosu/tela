//go:build linux

package keyboard

import (
	"errors"
	"os"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/evdev"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
)

// The parts of the Linux backends' keyboards (Wayland and X11) that do not
// depend on the display protocol. They live in a Linux-only file so that
// the other platforms, which use Held through the headless backend, do not
// link the XKB tables.

// OtherModifiers returns the modifiers that the held keys other than c set
// in km (xkb.Keymap.HeldModifiers of each one's evdev code): what stays
// down when c is released, for the prediction of a release's Mods when
// the state that follows it (Wayland's modifiers event, X11's XKB
// StateNotify) has not arrived.
func (h *Held) OtherModifiers(km *xkb.Keymap, c platform.KeyCode) platform.Modifiers {
	var m platform.Modifiers
	for _, p := range h.keys {
		if p.Code == c {
			continue
		}
		if n, ok := evdev.Code(p.Code); ok {
			m |= km.HeldModifiers(n + evdev.X11Offset)
		}
	}
	return m
}

// LoadCompose returns the Compose table of a Linux backend, read once per
// backend on first use: the table of override when a test sets one; none
// under go test unless TELA_SYSTEM_COMPOSE is set, so that results never
// depend on the machine's locale files; otherwise the user's or the
// locale's Compose file (xkb.LoadComposeTable with the process
// environment). Having no Compose file is not an error (dead keys then
// type their spacing accent, ADR-052); err reports a file that could not
// be read or parsed, with the table that could be read, if any.
func LoadCompose(override func() *xkb.ComposeTable) (*xkb.ComposeTable, error) {
	switch {
	case override != nil:
		return override(), nil
	case underTest() && os.Getenv("TELA_SYSTEM_COMPOSE") == "":
		return nil, nil
	}
	t, _, err := xkb.LoadComposeTable(os.Getenv, xkb.ComposeOptionsFromEnv(os.Getenv))
	if errors.Is(err, xkb.ErrNoComposeFile) {
		err = nil
	}
	return t, err
}
