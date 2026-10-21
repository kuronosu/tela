package h

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// onEvent is the prop of every handler: it accepts a handler with the
// event, func(E), or without it, func(), which most handlers want (a click
// that only sets state). event is the name core knows it by, name the
// prop's, for the message of a nil handler.
func onEvent[E any, F func() | func(E)](event, name string, handler F) tela.Prop {
	var fn func(E)
	switch f := any(handler).(type) {
	case func(E):
		fn = f
	case func():
		if f != nil {
			fn = func(E) { f() }
		}
	}
	if fn == nil {
		var e E
		panic(fmt.Sprintf("h: %s(nil): pass a handler, for example %s(func() { ... }) or %s(func(e %s) { ... })", name, name, name, strings.Replace(fmt.Sprintf("%T", e), "core.", "tela.", 1)))
	}
	return core.On(event, fn)
}
