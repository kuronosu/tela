//go:build teladev

package core

import "reflect"

// The dev-loop state of a UseAsync slot (ADR-113, ADR-298): its Data is
// kept when T is, and the slot loads again with it shown. Err and the
// run's bookkeeping are not kept.

func (s *asyncSlot[T]) devSlot() (hookKind, string) {
	return hookAsync, devTypeString(reflect.TypeFor[T]())
}

func (s *asyncSlot[T]) devEncode() ([]byte, devWhy) { return devEncodeValue(s.data) }

func (s *asyncSlot[T]) devDecode(b []byte) error {
	v, err := devDecodeValue[T](b)
	if err == nil {
		s.data = v
	}
	return err
}
