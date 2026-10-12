package tela

import "github.com/kuronosu/tela/internal/core"

// UseState returns the state of this hook slot and a function that sets it
// (ARCHITECTURE section 7). The first build stores initial; later builds
// return the latest value set. Setting a state rebuilds the component.
//
//	count, setCount := tela.UseState(0)
//
// The setter is safe to call from any goroutine: it queues the update and
// wakes the UI thread. For comparable types, setting a value equal (==) to
// the current one does not rebuild; other types always rebuild.
//
// UseState follows the rules of hooks: call it at the top level of a
// component function, in the same order on every build. It panics when
// called outside a component build or when the call order changed since
// the previous build.
func UseState[T any](initial T) (T, func(T)) { return core.UseState(initial) }
