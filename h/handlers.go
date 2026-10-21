package h

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// onEvent is the prop of every handler: it accepts a handler with the
// event, func(E), or without it, func(), which most handlers want (a click
// that only sets state). event is the name core knows it by, name the
// prop's, for the message of a nil handler. F may be a named func type
// (type ClickHandler func(tela.MouseEvent)), which is converted to its
// underlying type.
//
// A [tela.StopHandler], which tela.StopPropagation returns, is a named func
// type, so it satisfies ~func() with no change to the constraints: the
// switch below tells it from a plain func() by its dynamic type, and its
// handler stops the event's propagation before it calls f (ADR-097). That
// is why E needs the StopPropagation method every event has.
func onEvent[E interface{ StopPropagation() }, F ~func() | ~func(E)](event, name string, handler F) tela.Prop {
	var fn func(E)
	switch f := any(handler).(type) {
	case func(E):
		fn = f
	case func():
		if f != nil {
			fn = func(E) { f() }
		}
	case tela.StopHandler:
		if f != nil {
			fn = func(e E) { e.StopPropagation(); f() }
		}
	default:
		fn = namedHandler[E](f)
	}
	if fn == nil {
		var e E
		panic(fmt.Sprintf("h: %s(nil): pass a handler, for example %s(func() { ... }) or %s(func(e %s) { ... })", name, name, name, strings.Replace(fmt.Sprintf("%T", e), "core.", "tela.", 1)))
	}
	return core.On(event, fn)
}

// namedHandler converts h, a value of a named func type whose underlying
// type is func() or func(E), to a func(E); nil stays nil. The type
// constraint of the callers guarantees one of the two shapes.
func namedHandler[E any](h any) func(E) {
	v := reflect.ValueOf(h)
	if v.IsNil() {
		return nil
	}
	if v.Type().NumIn() == 0 {
		f := v.Convert(reflect.TypeFor[func()]()).Interface().(func())
		return func(E) { f() }
	}
	return v.Convert(reflect.TypeFor[func(E)]()).Interface().(func(E))
}
