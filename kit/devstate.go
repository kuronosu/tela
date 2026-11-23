//go:build teladev

package kit

import "github.com/kuronosu/tela/internal/core"

// Codecs for tela dev (ADR-309), as in package aria's devstate.go: the
// store of a menu holds the uncontrolled checkbox items and radio groups
// in unexported fields, which a reload reset silently. Compiled only with
// the teladev build tag.

// devMenu is what a menu's store keeps: the checkbox items and radio groups
// the user set, which the next build shows over their defaults.
type devMenu[K comparable] struct {
	Checked map[K]bool
	Radio   map[K]K
}

func (s *menuStore[K]) GobEncode() ([]byte, error) {
	return core.DevEncode(devMenu[K]{s.checked, s.radio})
}

func (s *menuStore[K]) GobDecode(b []byte) error {
	var d devMenu[K]
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	if d.Checked != nil {
		s.checked = d.Checked
	}
	if d.Radio != nil {
		s.radio = d.Radio
	}
	return nil
}
