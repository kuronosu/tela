package core

import (
	"context"
	"sync"
)

// UseAsync (ADR-298) runs a function that loads a value on a goroutine
// and shows its state: loading, the data or the error. It is built on
// UseGo's machinery (the goroutine routes Post to the component's app, a
// panic in it is a crash report) with the bookkeeping that every caller
// of UseGo writes by hand: a generation counter that drops the result of
// a cancelled or superseded run, and Data, Err and Loading changing
// together in one update.

// Async is the state of one UseAsync call, a small comparable value: two
// builds with nothing changed give == values, so a struct component that
// holds one skips its build. The type must be comparable for ==: a T
// that is not (a slice) makes the value uncomparable, which prop skipping
// reads as always changed.
type Async[T any] struct {
	// Loading is whether a run is in flight: true from the first build
	// until the first run ends, and again while Reload or a change of the
	// dependencies runs. Data is the zero value, or the value of the
	// previous run for Reload, meanwhile.
	Loading bool
	// Err is the error of the latest run that ended, nil after a
	// successful one and after a change of the dependencies.
	Err error
	// Data is the value of the latest successful run (the zero value
	// before the first one and after a change of the dependencies).
	Data T

	h *asyncSlot[T]
}

// Reload starts a new run of the function with the latest build's
// closure, cancelling the context of the one in flight (whose result is
// dropped). Data stays shown, Err until the run ends. It is safe from any
// goroutine; on an unmounted component it does nothing.
func (a Async[T]) Reload() {
	if a.h == nil {
		panic("tela: Reload called on a zero Async; use the Async returned by tela.UseAsync")
	}
	a.h.reload()
}

// asyncSlot is the slot of one UseAsync call. The visible state (data,
// err, loading) changes on the UI thread only, in a build or in an
// update; mu guards what any goroutine reaches (Reload, a finishing run).
type asyncSlot[T any] struct {
	eff effectSlot // starts the first run and the runs of a changed epoch
	el  *Element
	app *Tree

	// UI thread.
	data    T
	err     error
	loading bool
	deps    []any // the latest build's dependencies
	epoch   int   // counts the changes of deps: the effect's dependency

	mu        sync.Mutex
	f         func(ctx context.Context) (T, error) // the latest build's
	gen       uint64                               // the run that may write the state
	cancel    context.CancelFunc                   // of that run
	unmounted bool
}

// UseAsync runs f on a goroutine and returns its state. See tela.UseAsync.
// Without dependencies it runs once (a UseGo without them would start
// again after every build, which UseAsync's own state changes cause).
func UseAsync[T any](f func(ctx context.Context) (T, error), deps ...any) Async[T] {
	if f == nil {
		panic("tela: UseAsync(nil, ...): pass the function that loads the value, a func(ctx context.Context) (T, error) that returns when ctx is done")
	}
	if len(deps) == 0 {
		deps = []any{Once}
	}
	el, i := nextSlot(hookAsync)
	var s *asyncSlot[T]
	if i < len(el.hooks) {
		var ok bool
		if s, ok = el.hooks[i].(*asyncSlot[T]); !ok {
			el.tree.slotMismatch(el, i, "UseAsync["+typeName[T]()+"]")
		}
		s.mu.Lock()
		s.f = f
		s.mu.Unlock()
		if el.tree.depsChanged(el, i, s.deps, deps) {
			s.deps = append(s.deps[:0], deps...)
			s.epoch++
			s.invalidate()
			var zero T
			s.data, s.err, s.loading = zero, nil, true
		}
		s.eff.update(i, s.start, []any{s.epoch})
	} else {
		el.tree.checkDeps(el, i, deps)
		s = &asyncSlot[T]{el: el, app: el.tree.app(), f: f, loading: true,
			deps: append([]any(nil), deps...)}
		s.eff = effectSlot{el: el, kind: hookAsync, effect: s.start, deps: []any{0}, pending: true}
		el.fx = append(el.fx, &s.eff)
		el.hooks = append(el.hooks, s)
		if devStateEnabled {
			devSeed(el, i)
		}
	}
	return Async[T]{Loading: s.loading, Err: s.err, Data: s.data, h: s}
}

// start is the effect: it starts the run of the build that changed the
// dependencies, and its cleanup ends it.
func (s *asyncSlot[T]) start() func() {
	s.run()
	return s.invalidate
}

func (s *asyncSlot[T]) unmount(t *Tree) {
	s.mu.Lock()
	s.unmounted = true
	s.mu.Unlock()
	s.invalidate()
	s.eff.unmount(t)
}

// invalidate cancels the run in flight and drops its result.
func (s *asyncSlot[T]) invalidate() {
	s.mu.Lock()
	s.gen++
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// reload is Async.Reload: mark the slot loading on the UI thread, then
// run. The generation moves first, so a result that arrives in between is
// dropped.
func (s *asyncSlot[T]) reload() {
	s.mu.Lock()
	gone := s.unmounted
	s.mu.Unlock()
	if gone {
		return
	}
	s.invalidate()
	s.el.tree.enqueue(update{el: s.el, apply: func() (bool, any) {
		if s.loading {
			return false, s
		}
		s.loading = true
		return true, s
	}})
	s.run()
}

// run starts a goroutine for f, the run that may write the state until
// the next invalidate.
func (s *asyncSlot[T]) run() {
	s.mu.Lock()
	if s.unmounted {
		s.mu.Unlock()
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.gen++
	gen := s.gen
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	f := s.f
	s.mu.Unlock()
	go func() {
		defer crashRecover(PanicGo) // logtee.go
		goWithApp(ctx, s.app, func(ctx context.Context) {
			data, err := f(ctx)
			s.finish(ctx, gen, data, err)
		})
	}()
}

// current reports whether run gen is the one that may write the state.
func (s *asyncSlot[T]) current(gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen == gen
}

// finish queues the result of run gen, unless it was cancelled or
// superseded; the update checks again on the UI thread, since a newer run
// may have started while it waited in the queue.
func (s *asyncSlot[T]) finish(ctx context.Context, gen uint64, data T, err error) {
	if ctx.Err() != nil || !s.current(gen) {
		return
	}
	s.el.tree.enqueue(update{el: s.el, apply: func() (bool, any) {
		if !s.current(gen) {
			return false, s
		}
		s.loading, s.err = false, err
		if err == nil {
			s.data = data // an error keeps the previous Data, as React Query does
		}
		return true, s
	}})
}
