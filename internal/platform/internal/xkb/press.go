package xkb

import "github.com/kuronosu/tela/internal/platform"

// Press resolves a key press with the current state and feeds its keysym
// to the compose state cs (nil, or a state without a table, means no
// Compose file), following the platform contract (package platform,
// "Keyboard"):
//
//   - A dead key that starts or continues a sequence has Key KeyDead and
//     no Text; another key in the middle of a sequence (after the Compose
//     key) keeps its Key and has no Text.
//   - The press that completes a sequence has the result as Key and Text
//     (winit and browsers report the composed character as the key).
//   - A key that breaks a sequence starts again from the beginning (see
//     ComposeState.Feed): it types normally or starts a new sequence.
//   - Without a Compose table, or for a dead key no sequence starts with,
//     a dead key types its spacing character (´ for dead_acute), as both
//     Key and Text.
//
// It does not allocate.
func (s *State) Press(kc uint32, cs *ComposeState) Lookup {
	l := s.Lookup(kc)
	if cs == nil || cs.t == nil {
		return deadFallback(l)
	}
	status, text := cs.Feed(l.Sym)
	switch status {
	case ComposeComposing:
		if l.Sym.IsDead() {
			l.Key = platform.KeyDead
		}
		l.Text = ""
	case ComposeComposed:
		l.Key, l.Text = platform.Key(text), text
	case ComposeNothing:
		l = deadFallback(l)
	}
	return l
}

// Repeat resolves a key repeat: the key's current lookup, without feeding
// compose (a held dead key does not stack accents). A repeated dead key
// stays dead while a Compose table exists, and repeats its spacing
// character otherwise.
func (s *State) Repeat(kc uint32, cs *ComposeState) Lookup {
	l := s.Lookup(kc)
	if l.Sym.IsDead() && cs != nil && cs.t != nil {
		l.Key, l.Text = platform.KeyDead, ""
		return l
	}
	return deadFallback(l)
}

func deadFallback(l Lookup) Lookup {
	if l.Sym.IsDead() {
		if sp, ok := deadSpacingMap()[l.Sym]; ok {
			l.Key, l.Text = platform.Key(sp), sp
		}
	}
	return l
}
