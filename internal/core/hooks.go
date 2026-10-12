package core

import (
	"fmt"
	"reflect"
	"sync"
)

// Hooks keep their state in slots on the component Element being built
// (ADR-040): the n-th hook call of a build uses slot n. A component
// function called directly, not through a component node, runs inside its
// caller's build, so its hooks take slots of the caller (ARCHITECTURE
// section 7: a custom hook).
//
// The Element being built is process-wide state: hooks are plain
// functions without a context argument, and Go has no goroutine-local
// storage. buildMu serializes the build phases of all Trees (they rarely
// run at once: one per window, plus tests), so current always belongs to
// the build that is running.

var (
	buildMu sync.Mutex
	current *Element
)

// build runs the function of component el and reconciles its result.
func (t *Tree) build(el *Element) {
	el.needsBuild = false
	t.stats.Builds++
	prev := current
	current = el
	el.hookIdx = 0
	before, first := len(el.hooks), el.builds == 0
	el.builds++
	var out Node
	func() {
		defer func() { current = prev }()
		out = el.comp.build()
	}()
	if !first && el.hookIdx != before {
		// Phase 4 adds the full hook-order check; this catches the
		// common mistake cheaply.
		panic(fmt.Sprintf("tela: %s called %d hooks in this build and %d in the previous one; call hooks at the top level of the component, never inside conditions or loops (or move the conditional part into its own component)",
			t.describe(el), el.hookIdx, before))
	}
	t.reconcileOne(el, out)
}

// currentElement returns the component being built, or panics with an
// actionable message when a hook is called outside a build.
func currentElement(hook string) *Element {
	el := current
	if el == nil {
		panic("tela: " + hook + " called outside a component build; call hooks at the top level of a component function while it builds (ARCHITECTURE section 7)")
	}
	return el
}

// stateSlot is the slot of one UseState call.
type stateSlot[T any] struct {
	value T
	set   func(T)
}

// UseState returns the state of this hook slot and a setter (ARCHITECTURE
// section 7, ADR-040). The first build stores initial; later builds return
// the latest value set.
//
// The setter is safe to call from any goroutine: it enqueues the update
// and wakes the UI thread, which applies it before the next build and
// rebuilds the component. For comparable types, setting a value equal
// (==) to the current one does not rebuild (React's bail-out); other
// types always rebuild. The setter is the same function on every build.
func UseState[T any](initial T) (T, func(T)) {
	el := currentElement("UseState")
	i := el.hookIdx
	el.hookIdx++
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*stateSlot[T])
		if !ok {
			panic(fmt.Sprintf("tela: hook %d of %s was %T in the previous build and is UseState[%s] now; hooks must be called in the same order on every build (never inside conditions or loops)",
				i, el.tree.describe(el), el.hooks[i], reflect.TypeFor[T]()))
		}
		return s.value, s.set
	}
	s := &stateSlot[T]{value: initial}
	comparable := reflect.TypeFor[T]().Comparable()
	t := el.tree
	s.set = func(v T) {
		t.enqueue(update{el: el, apply: func() bool {
			if comparable && safeEqual(any(s.value), any(v)) {
				return false
			}
			s.value = v
			return true
		}})
	}
	el.hooks = append(el.hooks, s)
	return s.value, s.set
}

// update is a state change waiting for the UI thread.
type update struct {
	el *Element
	// apply changes the state and reports whether the component must be
	// built again.
	apply func() bool
}

// enqueue adds a state update. Safe from any goroutine.
func (t *Tree) enqueue(u update) {
	t.mu.Lock()
	t.updates = append(t.updates, u)
	wake := t.wake
	t.mu.Unlock()
	if wake != nil {
		wake()
	}
}

// ApplyUpdates applies the queued state updates on the UI thread and
// marks their components for building. It reports whether any component
// must build. Frame calls it too; hosts call it on a WakeEvent so that a
// frame is requested only when something changed.
func (t *Tree) ApplyUpdates() bool {
	t.mu.Lock()
	us := t.updates
	t.updates = nil
	t.mu.Unlock()
	changed := false
	for _, u := range us {
		if u.el.mounted && u.apply() {
			t.markBuild(u.el)
			changed = true
		}
	}
	return changed
}
