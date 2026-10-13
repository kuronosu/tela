// Package keyboard holds the OS-independent part of the platform keyboard
// contract (see package platform, "Keyboard"), shared by the backends: the
// set of keys a window has delivered presses for, which turns into
// synthetic releases when the window loses focus, and the reporting of
// keyboard setup errors. On Linux it also holds what the Wayland and X11
// keyboards share above internal/xkb (linux.go): the Mods prediction of a
// release and the Compose file rules. It imports no backend.
package keyboard

import (
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Held is the set of keys whose press a window received and whose release
// it has not, in press order. A backend keeps one per window (or per
// keyboard focus) and passes every KeyEvent through Accept before queuing
// it. The zero value is an empty set.
type Held struct {
	keys []platform.KeyEvent // the presses, for their Code, Key and Location
}

// Accept updates the set with e and reports whether e may be delivered:
//
//   - A press is always delivered; a press of a key already held (a lost
//     release, or an OS that repeats without flagging it) keeps one entry.
//   - A repeat or a release of a key that is not held is dropped: the key
//     went down before the window had focus, and the contract pairs every
//     delivered release with a delivered press.
func (h *Held) Accept(e platform.KeyEvent) bool {
	i := h.index(e.Code)
	switch {
	case e.Action == platform.KeyRelease:
		if i < 0 {
			return false
		}
		h.keys = slices.Delete(h.keys, i, i+1)
		return true
	case e.Repeat:
		return i >= 0
	case i >= 0:
		h.keys[i] = e
		return true
	default:
		h.keys = append(h.keys, e)
		return true
	}
}

// IsHeld reports whether a press of the physical key c was accepted and
// its release was not.
func (h *Held) IsHeld(c platform.KeyCode) bool { return h.index(c) >= 0 }

// Len returns the number of held keys.
func (h *Held) Len() int { return len(h.keys) }

// ReleaseAll empties the set and returns a synthetic release for every held
// key, in press order, stamped t: what a backend delivers before the
// FocusEvent that reports the loss of focus. The releases keep the Code,
// Key and Location of their press, have no Text and no Mods (the window no
// longer sees the keyboard), and are marked Synthetic.
func (h *Held) ReleaseAll(t time.Duration) []platform.KeyEvent {
	if len(h.keys) == 0 {
		return nil
	}
	out := make([]platform.KeyEvent, len(h.keys))
	for i, p := range h.keys {
		out[i] = platform.KeyEvent{
			Action:    platform.KeyRelease,
			Code:      p.Code,
			Key:       p.Key,
			Location:  p.Location,
			Synthetic: true,
			Time:      t,
		}
	}
	h.keys = nil
	return out
}

func (h *Held) index(c platform.KeyCode) int {
	return slices.IndexFunc(h.keys, func(e platform.KeyEvent) bool { return e.Code == c })
}
