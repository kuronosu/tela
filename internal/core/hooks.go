package core

import (
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
// Its effects are queued after those of the components built inside it,
// so children's effects run before their parents' (React's order). The
// Values the function reads become the component's subscriptions (a
// change builds it again).
func (t *Tree) build(el *Element) {
	el.needsBuild = false
	t.stats.Builds++
	prev := current
	current = el
	el.hookIdx = 0
	before, first := len(el.hooks), el.builds == 0
	el.builds++
	var out Node
	mark := t.beginTrack()
	func() {
		done := false
		defer func() {
			current = prev
			if !done { // the function panicked: keep the old subscriptions
				t.endTrack(mark)
			}
		}()
		out = el.comp.build()
		done = true
	}()
	t.observe(&el.vobs, obsBuild, el, nil, t.endTrack(mark))
	if !first && el.hookIdx != before {
		t.hookCountPanic(el, el.hookIdx, before)
	}
	t.reconcileOne(el, out)
	el.dirtyKids = false
	t.queueEffects(el)
}

// currentElement returns the component being built, or panics with an
// actionable message when a hook is called outside a build.
func currentElement(hook string) *Element {
	el := current
	if el == nil {
		panic("tela: " + hook + " called outside a component build; call hooks at the top level of a component function while it builds (ARCHITECTURE section 7), not in event handlers, effects or goroutines")
	}
	return el
}

// nextSlot returns the component being built and the index of the next
// hook slot, after the dev-mode order check.
func nextSlot(kind hookKind) (*Element, int) {
	el := currentElement(kind.String())
	i := el.hookIdx
	el.hookIdx++
	if el.tree.dev {
		el.tree.checkHook(el, i, kind)
	}
	return el, i
}

// typeName names T for messages.
func typeName[T any]() string { return reflect.TypeFor[T]().String() }

// ---------------------------------------------------------------------
// State.

// stateSlot is the slot of one UseState (or UseStateSet, UseStateUpdate)
// call.
type stateSlot[T any] struct {
	value T
	el    *Element
	// comparable is whether T is comparable: an update to a value == to
	// the current one does not rebuild.
	comparable bool
	// The method values of UseStateSet and UseStateUpdate, made once.
	setFn    func(T)
	updateFn func(func(T) T)
}

// store sets the state to v and reports whether it changed: for a
// comparable T, a value == to the current one is not a change (React's
// bail-out; ADR-044); other types always change.
func (s *stateSlot[T]) store(v T) bool {
	if s.comparable && safeEqual(any(s.value), any(v)) { // perf: boxes both values
		return false
	}
	s.value = v
	return true
}

// Setter sets the state of one UseState slot. It is a small comparable
// value, the same on every build of the slot, so it can go in props
// compared by tela.CP. Its methods are safe from any goroutine: they
// queue the update and wake the UI thread, which applies the queued
// updates in order before the next build.
type Setter[T any] struct{ s *stateSlot[T] }

// Set queues setting the state to v. For comparable types, a value == to
// the state when the update applies does not rebuild.
func (st Setter[T]) Set(v T) {
	s := st.slot("Set")
	s.el.tree.enqueue(update{el: s.el, apply: func() bool { return s.store(v) }})
}

// Update queues f, which receives the state left by the previous queued
// update and returns the new one: two queued Update(inc) add 2. f runs
// on the UI thread when the update applies, and must be pure (no side
// effects: it may run later than the call).
func (st Setter[T]) Update(f func(T) T) {
	s := st.slot("Update")
	if f == nil {
		panic("tela: Setter.Update(nil): pass a function that returns the new state from the current one, such as setCount.Update(func(n int) int { return n + 1 })")
	}
	s.el.tree.enqueue(update{el: s.el, apply: func() bool { return s.store(f(s.value)) }})
}

func (st Setter[T]) slot(method string) *stateSlot[T] {
	if st.s == nil {
		panic("tela: " + method + " called on a zero Setter; use the Setter returned by tela.UseState (a Setter declared without it is not tied to any state)")
	}
	return st.s
}

// useStateSlot returns the state slot of the current hook call.
func useStateSlot[T any](initial T) *stateSlot[T] {
	el, i := nextSlot(hookState)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*stateSlot[T])
		if !ok {
			el.tree.slotMismatch(el, i, "UseState["+typeName[T]()+"]")
		}
		return s
	}
	s := &stateSlot[T]{value: initial, el: el, comparable: reflect.TypeFor[T]().Comparable()}
	el.hooks = append(el.hooks, s)
	return s
}

// UseState returns the state of this hook slot and its Setter
// (ARCHITECTURE section 7, ADR-040). The first build stores initial;
// later builds return the latest value set.
func UseState[T any](initial T) (T, Setter[T]) {
	s := useStateSlot(initial)
	return s.value, Setter[T]{s}
}

// UseStateSet is UseState returning the Setter's Set method value, made
// once per slot (builds do not allocate it).
func UseStateSet[T any](initial T) (T, func(T)) {
	s := useStateSlot(initial)
	if s.setFn == nil {
		s.setFn = Setter[T]{s}.Set
	}
	return s.value, s.setFn
}

// UseStateUpdate is UseState returning the Setter's Update method value,
// made once per slot.
func UseStateUpdate[T any](initial T) (T, func(func(T) T)) {
	s := useStateSlot(initial)
	if s.updateFn == nil {
		s.updateFn = Setter[T]{s}.Update
	}
	return s.value, s.updateFn
}

// ---------------------------------------------------------------------
// Reducers.

// reducerSlot is the slot of one UseReducer call.
type reducerSlot[S, A any] struct {
	stateSlot[S]
	// reducer is the latest build's: queued actions apply with it.
	reducer func(S, A) S
}

func (s *reducerSlot[S, A]) dispatch(a A) {
	s.el.tree.enqueue(update{el: s.el, apply: func() bool { return s.store(s.reducer(s.value, a)) }})
}

// dispatchTarget is a reducer slot of any state type with actions A.
type dispatchTarget[A any] interface{ dispatch(A) }

// Dispatcher sends actions to one UseReducer slot. Like Setter, it is a
// small comparable value, the same on every build of the slot, and safe
// from any goroutine.
type Dispatcher[A any] struct{ t dispatchTarget[A] }

// Dispatch queues action a. When the update applies on the UI thread, the
// reducer of the latest build computes the new state from the state left
// by the previous queued update; for comparable states, a result == to
// the current state does not rebuild.
func (d Dispatcher[A]) Dispatch(a A) {
	if d.t == nil {
		panic("tela: Dispatch called on a zero Dispatcher; use the Dispatcher returned by tela.UseReducer")
	}
	d.t.dispatch(a)
}

// UseReducer returns the state of this hook slot and a Dispatcher whose
// actions update it with reducer (React's useReducer). The first build
// stores initial. reducer runs on the UI thread and must be pure.
func UseReducer[S, A any](reducer func(S, A) S, initial S) (S, Dispatcher[A]) {
	if reducer == nil {
		panic("tela: UseReducer(nil, ...): pass a reducer func(state S, action A) S that returns the new state")
	}
	el, i := nextSlot(hookReducer)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*reducerSlot[S, A])
		if !ok {
			el.tree.slotMismatch(el, i, "UseReducer["+typeName[S]()+", "+typeName[A]()+"]")
		}
		s.reducer = reducer
		return s.value, Dispatcher[A]{s}
	}
	s := &reducerSlot[S, A]{reducer: reducer}
	s.value, s.el, s.comparable = initial, el, reflect.TypeFor[S]().Comparable()
	el.hooks = append(el.hooks, s)
	return s.value, Dispatcher[A]{s}
}

// ---------------------------------------------------------------------
// The update queue.

// update is a state change, a Value change or a posted function waiting
// for the UI thread.
type update struct {
	el *Element
	// apply changes the state and reports whether the component must be
	// built again.
	apply func() bool
	// set changes a Value of el (apply is nil): it invalidates the
	// Value's readers itself and reports whether it invalidated any.
	set func() bool
	// post is a function given to Post (el, apply and set are nil).
	post func()
}

// enqueue adds an update. Safe from any goroutine.
func (t *Tree) enqueue(u update) {
	t.mu.Lock()
	t.updates = append(t.updates, u)
	wake := t.wake
	t.mu.Unlock()
	if wake != nil {
		wake()
	}
}

// maxUpdatePasses bounds the passes of ApplyUpdates over updates queued
// while it runs (by posted functions), so a function that posts itself
// cannot keep the UI thread in one call.
const maxUpdatePasses = 16

// ApplyUpdates applies the queued state and Value updates and runs the
// posted functions on the UI thread, in the order they were queued; it
// marks the components whose state changed for building, and invalidates
// the readers of the Values that changed in the phase they read them in.
// Updates of unmounted components are dropped. Updates queued meanwhile
// (by a posted function) are applied too. It reports whether anything
// must be built, laid out or painted again. Frame calls it too; hosts call
// it on a WakeEvent so that a frame is requested only when something
// changed.
func (t *Tree) ApplyUpdates() bool {
	changed := false
	for pass := 0; pass < maxUpdatePasses; pass++ {
		t.mu.Lock()
		us := t.updates
		t.updates = nil
		t.mu.Unlock()
		if len(us) == 0 {
			break
		}
		for _, u := range us {
			switch {
			case u.post != nil:
				u.post()
			case !u.el.mounted:
				// Dropped: the component is gone.
			case u.set != nil:
				if u.set() {
					changed = true
				}
			case u.apply():
				t.markBuild(u.el)
				changed = true
			}
		}
	}
	return changed
}

// ---------------------------------------------------------------------
// Post.

// posts routes Post to the Tree of the running app.
var posts struct {
	mu      sync.Mutex
	target  *Tree
	pending []func()
}

// Post runs fn on the UI thread of the running app, in order with the
// state updates queued before and after it. Safe from any goroutine.
//
// The app is the Tree that receives posts (ReceivePosts; Run makes its
// window's tree receive them). Until one does (before Run starts, or in
// headless trees and tests that do not call ReceivePosts), the functions
// are kept in order and handed to the next Tree that receives posts.
func Post(fn func()) {
	if fn == nil {
		panic("tela: Post(nil): pass the function to run on the UI thread")
	}
	posts.mu.Lock()
	defer posts.mu.Unlock()
	if t := posts.target; t != nil {
		t.enqueue(update{post: fn})
		return
	}
	posts.pending = append(posts.pending, fn)
}

// ReceivePosts makes t the Tree that runs the functions given to Post: in
// ApplyUpdates (on a WakeEvent or at the start of a Frame), in order with
// the state updates. The functions posted before are queued now. stop
// ends it (if t still receives posts); later posts wait for the next
// receiver.
func (t *Tree) ReceivePosts() (stop func()) {
	posts.mu.Lock()
	defer posts.mu.Unlock()
	posts.target = t
	if len(posts.pending) > 0 {
		pending := posts.pending
		posts.pending = nil
		t.mu.Lock()
		for _, fn := range pending {
			t.updates = append(t.updates, update{post: fn})
		}
		wake := t.wake
		t.mu.Unlock()
		if wake != nil {
			wake()
		}
	}
	return func() {
		posts.mu.Lock()
		defer posts.mu.Unlock()
		if posts.target == t {
			posts.target = nil
		}
	}
}
