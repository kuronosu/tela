package aria

import (
	"github.com/kuronosu/tela"
)

// boolState is the state of a two-state control that its caller may keep
// (controlled) or leave to the hook (uncontrolled), kept across builds:
// the selected state of a toggle, the open state of an overlay.
//
// The rule is the one of every control in this package, and of React Aria
// and React: a nil pointer is "uncontrolled", the hook keeps the state,
// starting at the default; a non-nil pointer is "controlled", the control
// shows what it points to and only asks the callback for a change, so a
// change the caller does not store is refused. The callback is also
// called for an uncontrolled control, to follow it.
type boolState struct {
	cur        bool
	controlled bool
	onChange   func(bool)
	setCur     tela.Setter[bool]
}

// request asks for the next state: an uncontrolled control takes it, a
// controlled one only reports it. It does nothing when the state already
// is next.
func (s *boolState) request(next bool) {
	if next == s.cur {
		return
	}
	if !s.controlled {
		s.cur = next // a second request in the same event sees it
		s.setCur.Set(next)
	}
	if s.onChange != nil {
		s.onChange(next)
	}
}

// useBoolState returns the current state and the state itself, whose
// request method asks for a change (the same value on every build).
func useBoolState(value *bool, def bool, onChange func(bool)) (bool, *boolState) {
	internal, setInternal := tela.UseState(def)
	st := &tela.UseRef(boolState{}).Current
	cur := internal
	if value != nil {
		cur = *value
	}
	st.cur, st.controlled, st.onChange, st.setCur = cur, value != nil, onChange, setInternal
	return cur, st
}
