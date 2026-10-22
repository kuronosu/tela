package core

import (
	"reflect"
	"strconv"
	"sync"
)

// ---------------------------------------------------------------------
// UseCallback.

// cbSlot is the slot of one UseCallback call.
type cbSlot[F any] struct {
	latest F // the latest build's function
	fwd    F // calls latest; made once per slot
}

// Callback is a stable handle on the latest function given to
// UseCallback in one hook slot (React's useEffectEvent, a "latest ref").
// It is a small comparable value, the same on every build of the slot,
// so a props struct holding one stays comparable and tela.CP can skip
// the component that receives it.
type Callback[F any] struct{ s *cbSlot[F] }

// Fn returns the slot's forwarding function: one function of type F,
// the same on every build, that calls the latest function given to
// UseCallback at the time it is called. Call it on the UI thread (event
// handlers and effects run there); from other goroutines, use Post.
func (c Callback[F]) Fn() F {
	if c.s == nil {
		panic("tela: Fn called on a zero Callback; use the Callback returned by tela.UseCallback")
	}
	return c.s.fwd
}

// IsZero reports whether c is the zero Callback, which is not tied to any
// UseCallback slot: Fn panics. A Callback left unset in a props struct,
// or read from a context default without a Provider, is zero.
func (c Callback[F]) IsZero() bool { return c.s == nil }

func (c Callback[F]) zeroHandle() bool { return c.s == nil }

// UseCallback stores fn as the latest function of this hook slot and
// returns the slot's Callback, whose Fn always calls the latest fn. F
// must be a func type. There are no dependencies: the forwarder never
// changes, and what it calls is always the latest build's fn.
func UseCallback[F any](fn F) Callback[F] {
	el, i := nextSlot(hookCallback)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*cbSlot[F])
		if !ok {
			el.tree.slotMismatch(el, i, "UseCallback["+typeName[F]()+"]")
		}
		checkFunc(fn)
		s.latest = fn
		return Callback[F]{s}
	}
	if k := reflect.TypeFor[F]().Kind(); k != reflect.Func {
		panic("tela: UseCallback[" + typeName[F]() + "]: the argument must be a function, such as tela.UseCallback(func(e tela.MouseEvent) { ... }); for other values use tela.UseRef or tela.UseMemo")
	}
	checkFunc(fn)
	s := &cbSlot[F]{latest: fn}
	s.fwd = forwarder(&s.latest)
	el.hooks = append(el.hooks, s)
	return Callback[F]{s}
}

// checkFunc panics when fn is a nil function.
func checkFunc[F any](fn F) {
	if reflect.ValueOf(any(fn)).IsNil() {
		panic("tela: UseCallback(nil): pass the function the Callback calls")
	}
}

// forwarders holds the forwarder makers registered by other packages, by
// function type.
var forwarders sync.Map // reflect.Type → func(latest any) any

// RegisterForwarder registers how to build the UseCallback forwarder of
// function type F without reflection: mk returns a function of type F
// that calls *latest. Package tela registers its event handler types.
func RegisterForwarder[F any](mk func(latest *F) F) {
	forwarders.Store(reflect.TypeFor[F](), func(l any) any { return mk(l.(*F)) })
}

// forwarder returns a function of type F that calls *latest: direct
// closures for common shapes and registered types, reflect.MakeFunc for
// the rest.
func forwarder[F any](latest *F) F {
	var f F
	switch p := any(&f).(type) {
	case *func():
		l := any(latest).(*func())
		*p = func() { (*l)() }
	case *func(bool):
		l := any(latest).(*func(bool))
		*p = func(v bool) { (*l)(v) }
	case *func(int):
		l := any(latest).(*func(int))
		*p = func(v int) { (*l)(v) }
	case *func(float32):
		l := any(latest).(*func(float32))
		*p = func(v float32) { (*l)(v) }
	case *func(float64):
		l := any(latest).(*func(float64))
		*p = func(v float64) { (*l)(v) }
	case *func(string):
		l := any(latest).(*func(string))
		*p = func(v string) { (*l)(v) }
	case *func(any):
		l := any(latest).(*func(any))
		*p = func(v any) { (*l)(v) }
	case *func() error:
		l := any(latest).(*func() error)
		*p = func() error { return (*l)() }
	default:
		ty := reflect.TypeFor[F]()
		if mk, ok := forwarders.Load(ty); ok {
			return mk.(func(any) any)(latest).(F)
		}
		lv := reflect.ValueOf(latest).Elem()
		variadic := ty.IsVariadic()
		return reflect.MakeFunc(ty, func(args []reflect.Value) []reflect.Value {
			if variadic {
				return lv.CallSlice(args)
			}
			return lv.Call(args)
		}).Interface().(F)
	}
	return f
}

// ---------------------------------------------------------------------
// UseRef.

// Ref is a mutable box that persists across builds: the same pointer on
// every build of its hook slot. Changing Current does not rebuild.
type Ref[T any] struct {
	Current T
}

// UseRef returns the Ref of this hook slot; the first build sets
// Current to initial.
func UseRef[T any](initial T) *Ref[T] {
	el, i := nextSlot(hookRef)
	if i < len(el.hooks) {
		r, ok := el.hooks[i].(*Ref[T])
		if !ok {
			el.tree.slotMismatch(el, i, "UseRef["+typeName[T]()+"]")
		}
		return r
	}
	r := &Ref[T]{Current: initial}
	el.hooks = append(el.hooks, r)
	if devStateEnabled {
		devSeed(el, i)
	}
	return r
}

// ---------------------------------------------------------------------
// UseID.

// idSlot is the slot of one UseID call.
type idSlot struct{ id string }

// UseID returns a string that identifies this hook slot: the same on
// every build, different from every other UseID of the process. It is
// the Tree's ID prefix (Config.IDPrefix) followed by a counter of the
// Tree's UseID slots in the order they were created, so the same mount
// order gives the same IDs.
func UseID() string {
	el, i := nextSlot(hookID)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*idSlot)
		if !ok {
			el.tree.slotMismatch(el, i, "UseID")
		}
		return s.id
	}
	t := el.tree
	s := &idSlot{id: t.idPrefix + strconv.FormatUint(t.idNext, 36)}
	t.idNext++
	el.hooks = append(el.hooks, s)
	return s.id
}
