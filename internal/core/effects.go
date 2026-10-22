package core

import "context"

// Effects (ARCHITECTURE sections 7 and 8). A build records which effect
// slots must run (their dependencies changed); when the build ends they
// are queued on the Tree after the effects of the components built inside
// it, so a commit runs children's effects before their parents', in
// declaration order within a component (React's order).
//
// Layout effects run in Frame after layout and before paint; passive
// effects (UseEffect) after the frame is presented: a Host calls
// FlushEffects after Present, Render after rasterizing, and tests call it
// themselves. Each commit runs, in order: the cleanups of the effects of
// unmounted components (parents first, as React), the cleanups of the
// effects about to run again, then the effects. Effects of a component
// unmounted before its commit never run. Frame and Mount first flush
// the passive effects of a previous frame that was never presented.

// effectSlot is the slot of one UseEffect or UseLayoutEffect call (and of
// the effect hooks built on them: UseMount, UseEffectDo, UseUpdateEffect,
// UseGo, UseInterval).
type effectSlot struct {
	el      *Element
	kind    hookKind
	layout  bool
	effect  func() func()
	cleanup func() // from the last run of effect
	deps    []any  // a copy of the latest build's dependencies
	pending bool   // queued to run at the next commit
}

// unmounter is a hook slot with work to do when its component unmounts.
type unmounter interface{ unmount(t *Tree) }

// unmount hands the cleanup of the effect's last run to the next commit.
func (s *effectSlot) unmount(t *Tree) {
	c := s.cleanup
	if c == nil {
		return
	}
	s.cleanup = nil
	if s.layout {
		t.layoutGone = append(t.layoutGone, c)
	} else {
		t.passiveGone = append(t.passiveGone, c)
	}
}

// UseEffect runs effect after the frame is presented, when its
// dependencies changed since the previous build (ARCHITECTURE section 7).
// effect may return a cleanup, which runs before the effect runs again
// and when the component unmounts.
//
// Dependencies follow React: none runs the effect after every build;
// Once (a constant dependency, React's []) runs it only after the first
// build, and so does a single nil (deprecated in favor of Once and
// UseMount, and warned about in dev mode); otherwise the values are
// compared with == against the previous build's. Values that cannot be
// compared (functions, slices, maps) count as changed on every build.
func UseEffect(effect func() func(), deps ...any) { useEffect(hookEffect, effect, deps) }

// UseLayoutEffect is UseEffect, but the effect runs in the frame after
// layout and before paint.
func UseLayoutEffect(effect func() func(), deps ...any) {
	useEffect(hookLayoutEffect, effect, deps)
}

func useEffect(kind hookKind, effect func() func(), deps []any) {
	if effect == nil {
		panic("tela: " + kind.String() + "(nil): pass the effect, a func() func() that returns its cleanup or nil")
	}
	el, i := nextSlot(kind)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*effectSlot)
		if !ok || s.kind != kind {
			el.tree.slotMismatch(el, i, kind.String())
		}
		s.update(i, effect, deps)
		return
	}
	el.hooks = append(el.hooks, newEffectSlot(el, i, kind, effect, deps))
}

// newEffectSlot returns effect slot i of el, created by this build. It
// runs at the next commit, except an update effect's, which skips the
// first build.
func newEffectSlot(el *Element, i int, kind hookKind, effect func() func(), deps []any) *effectSlot {
	t := el.tree
	t.checkDeps(el, i, deps)
	t.checkNilDeps(el, i, kind, deps)
	s := &effectSlot{el: el, kind: kind, layout: kind == hookLayoutEffect, effect: effect,
		deps: append([]any(nil), deps...), pending: kind != hookUpdateEffect}
	if s.pending {
		el.fx = append(el.fx, s)
	}
	return s
}

// update records the effect and the dependencies of a later build of
// slot i: the effect runs at the next commit when they changed.
func (s *effectSlot) update(i int, effect func() func(), deps []any) {
	el := s.el
	changed := el.tree.depsChanged(el, i, s.deps, deps)
	if changed {
		s.deps = append(s.deps[:0], deps...)
	}
	if changed || s.pending {
		s.effect = effect
	}
	if changed && !s.pending {
		s.pending = true
		el.fx = append(el.fx, s)
	}
}

// onceDep is the type of Once.
type onceDep struct{}

// String names the value in messages.
func (onceDep) String() string { return "tela.Once" }

// Once is the dependency that never changes (tela.Once): UseEffect(f,
// Once), UseLayoutEffect, UseGo, UseEffectDo and UseMemo with it run or
// compute once, after or on the first build (React's []). Its type is
// unexported, so it is equal to itself on every build and cannot be
// confused with a value of the user's.
var Once = onceDep{}

// mountOnly is the dependency list of UseMount: Once, so the effect runs
// after the first build only.
var mountOnly = []any{Once}

// UseMount runs effect once, after the frame that shows the component for
// the first time; the cleanup it returns (nil for none) runs when the
// component unmounts. It is UseEffect with a constant dependency.
func UseMount(effect func() func()) { useEffect(hookMount, effect, mountOnly) }

// UseEffectDo is UseEffect for an effect without a cleanup.
func UseEffectDo(f func(), deps ...any) {
	if f == nil {
		panic("tela: UseEffectDo(nil, ...): pass the function to run after the frame is presented")
	}
	useEffect(hookEffectDo, func() func() { f(); return nil }, deps)
}

// UseUpdateEffect is UseEffect without the run of the first build: the
// effect runs after the later builds whose dependencies changed (React's
// useUpdateEffect), and its cleanup before it runs again and on unmount.
func UseUpdateEffect(effect func() func(), deps ...any) {
	useEffect(hookUpdateEffect, effect, deps)
}

// UseGo runs f on a goroutine of its own after the frame is presented,
// when its dependencies changed, like UseEffect; the cleanup cancels ctx
// (when the dependencies change again or the component unmounts) before
// the next run starts. The goroutine is not waited for: f must return
// soon after ctx is done.
func UseGo(f func(ctx context.Context), deps ...any) {
	if f == nil {
		panic("tela: UseGo(nil, ...): pass the function to run on a goroutine, a func(ctx context.Context) that returns when ctx is done")
	}
	useEffect(hookGo, func() func() {
		ctx, cancel := context.WithCancel(context.Background())
		go f(ctx)
		return cancel
	}, deps)
}

// queueEffects moves the effects recorded by the build of el to the
// Tree's lists for the next commit.
func (t *Tree) queueEffects(el *Element) {
	for _, s := range el.fx {
		if s.layout {
			t.layoutFx = append(t.layoutFx, s)
		} else {
			t.passiveFx = append(t.passiveFx, s)
		}
	}
	clear(el.fx)
	el.fx = el.fx[:0]
}

// commitLayout runs the layout effects of the builds since the previous
// commit and hands their passive effects to FlushEffects. Frame calls it
// after layout.
func (t *Tree) commitLayout() {
	if len(t.layoutFx) == 0 && len(t.passiveFx) == 0 && len(t.layoutGone) == 0 && len(t.passiveGone) == 0 {
		return
	}
	t.readyGone = append(t.readyGone, t.passiveGone...)
	t.readyFx = append(t.readyFx, t.passiveFx...)
	t.passiveGone, t.passiveFx = nil, nil
	gone, fx := t.layoutGone, t.layoutFx
	t.layoutGone, t.layoutFx = nil, nil
	runEffects(gone, fx)
}

// FlushEffects runs the passive effects (UseEffect) of the frames that
// ran since the previous flush, with the cleanups that precede them. A
// Host calls it after presenting a frame, and Render after rasterizing;
// tests that drive a Tree with Frame call it to run them. It runs on the
// UI thread; state set by an effect is applied in a later frame. The
// effects run as handlers do (handlerDepth): a click one makes
// (ElementRef.Click) queues its updates for the host's next step (the
// setters wake it) instead of building in the middle of the commit. A
// caret reveal they ask for (ElementRef.Focus on a text field) waits
// until they end (field.go): then it runs at once when they left the
// layout current (no frame work, no update queued), else in the next
// frame, which applies their updates; in a frame, after its layout.
func (t *Tree) FlushEffects() {
	if len(t.readyGone) == 0 && len(t.readyFx) == 0 {
		return
	}
	gone, fx := t.readyGone, t.readyFx
	t.readyGone, t.readyFx = nil, nil
	// held: the caller holds the reveals (a frame, which runs them after
	// its layout); else the hold ends with the effects, which run them.
	held := t.frev.hold
	t.runPassive(gone, fx, held)
	if !held {
		t.revealAfterEffects()
	}
}

// runPassive runs a commit of passive effects as handlers run
// (handlerDepth), holding the caret reveals they ask for. held is the hold
// of FlushEffects' caller, restored when they end: when the caller does
// not hold the reveals and an effect panics, the next frame runs those
// held, since FlushEffects does not get to.
func (t *Tree) runPassive(gone []func(), fx []*effectSlot, held bool) {
	t.handlerDepth++
	t.frev.hold = true
	done := false
	defer func() {
		t.handlerDepth--
		t.frev.hold = held
		if !done && !held && len(t.frev.q) > 0 {
			t.requestRepaint() // an effect panicked: the next frame reveals
		}
	}()
	runEffects(gone, fx)
	done = true
}

// runEffects runs one commit: the cleanups of unmounted components, the
// cleanups of the effects about to run again, then the effects.
func runEffects(gone []func(), fx []*effectSlot) {
	for _, f := range gone {
		f()
	}
	for _, s := range fx {
		if c := s.cleanup; c != nil && s.el.mounted {
			s.cleanup = nil
			c()
		}
	}
	for _, s := range fx {
		if !s.pending {
			continue // already run by an earlier commit of the same list
		}
		s.pending = false
		if s.el.mounted {
			s.cleanup = s.effect()
		}
	}
}

// dropEffects runs the pending cleanups of unmounted components now and
// forgets the effects that never ran (Unmount).
func (t *Tree) dropEffects() {
	gone := append(append(t.layoutGone, t.readyGone...), t.passiveGone...)
	t.layoutGone, t.readyGone, t.passiveGone = nil, nil, nil
	for _, list := range [][]*effectSlot{t.layoutFx, t.passiveFx, t.readyFx} {
		for _, s := range list {
			s.pending = false
		}
	}
	t.layoutFx, t.passiveFx, t.readyFx = nil, nil, nil
	for _, f := range gone {
		f()
	}
}

// ---------------------------------------------------------------------
// Memo.

// memoSlot is the slot of one UseMemo call.
type memoSlot[T any] struct {
	value T
	deps  []any
}

// UseMemo returns compute(), computed on the first build and again only
// when the dependencies changed (compared like UseEffect's; none
// computes it on every build).
func UseMemo[T any](compute func() T, deps ...any) T {
	if compute == nil {
		panic("tela: UseMemo(nil, ...): pass the function that computes the value")
	}
	el, i := nextSlot(hookMemo)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*memoSlot[T])
		if !ok {
			el.tree.slotMismatch(el, i, "UseMemo["+typeName[T]()+"]")
		}
		if el.tree.depsChanged(el, i, s.deps, deps) {
			s.deps = append(s.deps[:0], deps...)
			s.value = compute()
		}
		return s.value
	}
	el.tree.checkDeps(el, i, deps)
	el.tree.checkNilDeps(el, i, hookMemo, deps)
	s := &memoSlot[T]{deps: append([]any(nil), deps...)}
	el.hooks = append(el.hooks, s)
	s.value = compute()
	return s.value
}

// onceSlot is the slot of one UseOnce call.
type onceSlot[T any] struct{ value T }

// UseOnce returns init(), called on the first build only; later builds
// return the same value. It is UseMemo without dependencies, for a value
// that a component creates once (a client, a buffer, a struct of
// custom-hook state).
func UseOnce[T any](init func() T) T {
	if init == nil {
		panic("tela: UseOnce(nil): pass the function that creates the value")
	}
	el, i := nextSlot(hookOnce)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*onceSlot[T])
		if !ok {
			el.tree.slotMismatch(el, i, "UseOnce["+typeName[T]()+"]")
		}
		return s.value
	}
	s := &onceSlot[T]{}
	el.hooks = append(el.hooks, s)
	s.value = init()
	return s.value
}
