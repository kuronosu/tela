package core

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/style"
)

// Phase-deferred reads (ARCHITECTURE section 7, ADR-015), after Jetpack
// Compose's snapshot observation: a Value records where it is read, and a
// change redoes only the phase that read it.
//
// The Tree tracks reads while its UI thread evaluates an observer:
//
//   - a component build (Tree.build): the component is built again;
//   - the layout-phase deferred props of a box (WidthFn, HeightFn,
//     evaluated by mapLayoutStyle): the box's layout style is mapped again
//     and the box laid out, without building or computing styles;
//   - the paint-phase deferred props of a box (TranslateFn, OpacityFn,
//     evaluated by painter.place): the box is repainted, without building,
//     computing styles or laying out.
//
// Reads anywhere else (event handlers, effects, goroutines) are not
// tracked. Each evaluation replaces the observer's subscriptions with the
// Values it read this time, so a closure that stops reading a Value stops
// being invalidated by it; unmounting the reader or the Value's owner
// removes the subscriptions on both sides.
//
// Hooks have no context argument and Go has no goroutine-local storage, so
// Get cannot tell the UI thread from another goroutine. It records a read
// whenever the Value's Tree is tracking one (Tree.trackDepth), under the
// Tree's trackMu. A Get from another goroutine that races with a tracked
// evaluation of the same Tree is therefore recorded as a read of that
// evaluation: one extra rebuild, relayout or repaint when the Value
// changes, until the observer's next evaluation drops it. Reads are never
// missed. Reads of a Value made by another Tree are not tracked (each
// Tree tracks the reads of its own Values).

// obsKind is what a Value change redoes for an observer.
type obsKind uint8

const (
	obsBuild  obsKind = iota // a component build: build again
	obsLayout                // a box's layout-phase deferred props: lay out again
	obsPaint                 // a box's paint-phase deferred props: repaint
	obsText                  // a TextFn: assemble its paragraph again (textfn.go)
	obsField                 // a text field bound to the Value (BindValue): show it
)

// observer is an evaluation whose Value reads are tracked: the build of a
// component (el), or the layout or paint deferred props of a box (box).
type observer struct {
	kind obsKind
	el   *Element
	box  *Box
	// subs are the subscriptions of the last evaluation, one per Value.
	subs []*valueSub
}

// valueSub subscribes an observer to a Value. It is in both lists, at the
// recorded indices, so either side removes it in O(1).
type valueSub struct {
	v          *valueCore
	obs        *observer
	vidx, oidx int32
}

// valueCore is the part of a Value that does not depend on its type: its
// owner and its subscriptions. Everything but el is UI-thread state.
type valueCore struct {
	// el is the component whose UseValue slot this is (immutable).
	el   *Element
	subs []*valueSub
	// mark is scratch state of Tree.resubscribe.
	mark uint64
}

func (v *valueCore) addSub(s *valueSub) {
	s.vidx = int32(len(v.subs))
	v.subs = append(v.subs, s)
}

func (v *valueCore) removeSub(s *valueSub) {
	i := s.vidx
	if i < 0 {
		return
	}
	last := len(v.subs) - 1
	moved := v.subs[last]
	v.subs[i] = moved
	moved.vidx = i
	v.subs[last] = nil
	v.subs = v.subs[:last]
	s.vidx = -1
}

func (o *observer) removeSub(s *valueSub) {
	i := s.oidx
	if i < 0 {
		return
	}
	last := len(o.subs) - 1
	moved := o.subs[last]
	o.subs[i] = moved
	moved.oidx = i
	o.subs[last] = nil
	o.subs = o.subs[:last]
	s.oidx = -1
}

// Value is an observable value held by one UseValue hook slot
// (ARCHITECTURE section 7). Get reads it and subscribes what is being
// evaluated on the UI thread; Set and Update queue changes, safe from any
// goroutine. A *Value is the same pointer on every build of its slot, so
// it is comparable and can go in props compared by tela.CP.
type Value[T any] struct {
	valueCore
	// cur is the value last applied by the UI thread: a pointer that is
	// never written after it is stored, so Get from any goroutine reads a
	// whole value without a lock.
	cur atomic.Pointer[T]
	// comparable is whether T is comparable: a new value == to the current
	// one changes nothing.
	comparable bool
	// pend are the Set and Update calls queued and not applied yet, in
	// the order of the update queue (they are queued under pmu), for
	// Latest. The UI thread removes an operation when it commits it.
	pmu  sync.Mutex
	pend []valueOp[T]
	// anim is the running animation of the Value (Animate, UseSpring,
	// UseTween; valueanim.go), nil when none (UI thread).
	anim valueAnimator
}

// valueOp is a queued change of a Value: a new value (f nil) or a
// function of the value left by the operations before it.
type valueOp[T any] struct {
	val T
	f   func(T) T
}

func (o valueOp[T]) apply(cur T) T {
	if o.f != nil {
		return o.f(cur)
	}
	return o.val
}

// Get returns the value. On the UI thread, while a component builds or a
// deferred style closure is evaluated in layout or paint, the read
// subscribes what is being evaluated: when the value changes, that
// component is built again, or that box laid out or repainted, and
// nothing else. Elsewhere (event handlers, effects) nothing is subscribed.
//
// Get is safe from any goroutine. It returns the value the UI thread
// applied last: Set and Update calls still waiting in the queue are not
// visible until the UI thread applies them (on its next wake or frame).
func (v *Value[T]) Get() T {
	p := v.load("Get")
	if t := v.el.tree; t.trackDepth.Load() > 0 {
		t.recordRead(&v.valueCore)
	}
	return *p
}

// Set queues setting the value to x. Safe from any goroutine; the UI
// thread applies queued updates in order, with the state updates. For
// comparable types, a value == to the current one when the update applies
// changes nothing.
func (v *Value[T]) Set(x T) {
	v.load("Set")
	v.queue(valueOp[T]{val: x})
}

// Latest returns the value as it will be once the UI thread applies the
// Set and Update calls queued so far: the applied value (what Get
// returns) with every pending operation applied to it, in order. Use it
// in code that reads a Value it just changed, or that another goroutine
// is changing, before the next frame applies the change:
//
//	pos.Set(x)
//	pos.Latest() // x, while pos.Get() is still the old value
//
// An Update function runs again here, on the calling goroutine, which is
// why it must be pure (and must not call Latest of its own Value). Latest is safe from any goroutine and never
// subscribes what is being evaluated, so it does not rebuild or relayout
// anything when the value changes; reading a Value in a build or in a
// deferred style closure is what Get is for. A change that has not been
// queued yet (a Set on another goroutine that has not returned) is not
// seen; one made through UseScroll or UseBounds is applied by the UI
// thread directly and is seen by Get as soon as it is made.
func (v *Value[T]) Latest() T {
	v.load("Latest")
	v.pmu.Lock()
	x := *v.cur.Load()
	var ops []valueOp[T]
	if len(v.pend) > 0 {
		ops = append(ops, v.pend...)
	}
	v.pmu.Unlock()
	for _, o := range ops {
		x = o.apply(x)
	}
	return x
}

// queue appends op to the pending operations and to the update queue,
// atomically, so that both orders agree.
func (v *Value[T]) queue(op valueOp[T]) {
	v.pmu.Lock()
	v.pend = append(v.pend, op)
	v.el.tree.enqueue(update{el: v.el, set: v.applyPending})
	v.pmu.Unlock()
}

// applyPending commits the oldest pending operation, on the UI thread, and
// invalidates the readers of the Value if it changed.
func (v *Value[T]) applyPending() bool {
	v.pmu.Lock()
	if len(v.pend) == 0 {
		v.pmu.Unlock()
		return false
	}
	op := v.pend[0]
	v.pmu.Unlock()
	// A Set or Update interrupts an animation of the Value.
	v.stopAnim()
	// Outside the lock, so that f may call Latest; only the UI thread
	// changes cur.
	p := new(T)
	*p = op.apply(*v.cur.Load())
	v.pmu.Lock()
	n := len(v.pend) - 1
	copy(v.pend, v.pend[1:])
	v.pend[n] = valueOp[T]{}
	v.pend = v.pend[:n]
	same := v.comparable && safeEqual(any(*v.cur.Load()), any(*p)) // perf: boxes both values
	if !same {
		v.cur.Store(p)
	}
	v.pmu.Unlock()
	return !same && v.el.tree.valueChanged(&v.valueCore)
}

// Update queues f, which receives the value as left by the updates queued
// before it and returns the new one: two queued Update(inc) add 2. f runs
// on the UI thread when the update applies (and in Latest, on the
// caller's goroutine) and must be pure.
func (v *Value[T]) Update(f func(T) T) {
	v.load("Update")
	if f == nil {
		panic("tela: Value.Update(nil): pass a function that returns the new value from the current one, such as v.Update(func(x float64) float64 { return x + 1 })")
	}
	v.queue(valueOp[T]{f: f})
}

// load returns the current value's pointer, or panics with an actionable
// message for a Value not made by UseValue.
func (v *Value[T]) load(method string) *T {
	if v == nil {
		panic("tela: " + method + " called on a nil *Value; use the Value returned by tela.UseValue")
	}
	p := v.cur.Load()
	if p == nil {
		panic("tela: " + method + " called on a Value not made by tela.UseValue; use the *Value it returns")
	}
	return p
}

// store applies a new value on the UI thread and invalidates the readers.
// It reports whether any reader was invalidated.
func (v *Value[T]) store(p *T) bool {
	if v.comparable && safeEqual(any(*v.cur.Load()), any(*p)) { // perf: boxes both values
		return false
	}
	v.cur.Store(p)
	return v.el.tree.valueChanged(&v.valueCore)
}

// unmount removes the subscriptions to the Value when its component
// unmounts; queued updates of it are dropped (ApplyUpdates).
func (v *Value[T]) unmount(*Tree) {
	v.stopAnim()
	for _, s := range v.subs {
		s.obs.removeSub(s)
		s.vidx = -1
	}
	clear(v.subs)
	v.subs = nil
}

// storeUI sets the value on the UI thread at once (not through the update
// queue) and invalidates its readers: UseScroll's values follow the scroll
// offsets this way.
func (v *Value[T]) storeUI(x T) {
	p := new(T)
	*p = x
	v.store(p)
}

// newValue returns a Value owned by component el holding initial.
func newValue[T any](el *Element, initial T) *Value[T] {
	v := &Value[T]{comparable: reflect.TypeFor[T]().Comparable()}
	v.el = el
	p := new(T)
	*p = initial
	v.cur.Store(p)
	return v
}

// UseValue returns the Value of this hook slot (ARCHITECTURE section 7,
// phase-deferred reads). The first build stores initial; the same *Value
// is returned on every build.
func UseValue[T any](initial T) *Value[T] {
	el, i := nextSlot(hookValue)
	if i < len(el.hooks) {
		v, ok := el.hooks[i].(*Value[T])
		if !ok {
			el.tree.slotMismatch(el, i, "UseValue["+typeName[T]()+"]")
		}
		return v
	}
	v := newValue(el, initial)
	el.hooks = append(el.hooks, v)
	return v
}

// ---------------------------------------------------------------------
// Tracking.

// beginTrack starts tracking the reads of an evaluation on the UI thread.
// Evaluations nest: the returned mark separates the reads of this one.
func (t *Tree) beginTrack() int {
	t.trackMu.Lock()
	mark := len(t.reads)
	t.trackDepth.Add(1)
	t.trackMu.Unlock()
	return mark
}

// endTrack ends the evaluation begun at mark and returns the Values it
// read, in a scratch slice valid until the next endTrack.
func (t *Tree) endTrack(mark int) []*valueCore {
	t.trackMu.Lock()
	t.trackDepth.Add(-1)
	got := append(t.readsTmp[:0], t.reads[mark:]...)
	clear(t.reads[mark:])
	t.reads = t.reads[:mark]
	t.trackMu.Unlock()
	t.readsTmp = got
	return got
}

// recordRead records a read of v by the evaluation being tracked. Safe
// from any goroutine.
func (t *Tree) recordRead(v *valueCore) {
	t.trackMu.Lock()
	if t.trackDepth.Load() > 0 {
		t.reads = append(t.reads, v)
	}
	t.trackMu.Unlock()
}

// observe makes the subscriptions of the observer in *op those of reads,
// creating the observer on its first read.
func (t *Tree) observe(op **observer, kind obsKind, el *Element, b *Box, reads []*valueCore) {
	o := *op
	if o == nil {
		if len(reads) == 0 {
			return
		}
		o = &observer{kind: kind, el: el, box: b}
		*op = o
	}
	t.resubscribe(o, reads)
}

// resubscribe replaces the subscriptions of o with one per distinct Value
// in reads, keeping those that did not change (no allocation when an
// evaluation reads what the previous one read). Values whose component
// was unmounted are not subscribed: they never change again.
func (t *Tree) resubscribe(o *observer, reads []*valueCore) {
	if len(reads) == 0 && len(o.subs) == 0 {
		return
	}
	t.evalID++
	id := t.evalID
	// Distinct live Values read: mark == id.
	n := 0
	for _, v := range reads {
		if v.mark == id || !v.el.mounted {
			continue
		}
		v.mark = id
		reads[n] = v
		n++
	}
	reads = reads[:n]
	// Keep the subscriptions to Values read again (their mark is reset,
	// so the loop below skips them); drop the others.
	kept := o.subs[:0]
	for _, s := range o.subs {
		if s.v.mark == id {
			s.v.mark = 0
			s.oidx = int32(len(kept))
			kept = append(kept, s)
			continue
		}
		s.v.removeSub(s)
		s.oidx = -1
	}
	clear(o.subs[len(kept):])
	o.subs = kept
	for _, v := range reads {
		if v.mark != id {
			continue
		}
		v.mark = 0
		s := &valueSub{v: v, obs: o, oidx: int32(len(o.subs))}
		v.addSub(s)
		o.subs = append(o.subs, s)
	}
}

// dropObserver removes every subscription of o: its component unmounted,
// or its box no longer has deferred props of its phase.
func dropObserver(o *observer) {
	if o == nil {
		return
	}
	for _, s := range o.subs {
		s.v.removeSub(s)
		s.oidx = -1
	}
	clear(o.subs)
	o.subs = o.subs[:0]
}

// valueChanged invalidates the readers of v after a change applied on the
// UI thread, each in the phase it read v in. It reports whether any
// reader was invalidated.
func (t *Tree) valueChanged(v *valueCore) bool {
	hit := false
	for _, s := range v.subs {
		o := s.obs
		switch o.kind {
		case obsBuild:
			if o.el.mounted {
				t.markBuild(o.el)
				hit = true
			}
		case obsLayout:
			// A box out of the box tree (display: none, inline content)
			// is mapped again when it comes back.
			if o.box.parent != nil {
				t.markRelayout(o.box)
				hit = true
			}
		case obsPaint:
			// An inline box (inPara) paints in its paragraph's layer, and
			// a display: contents one's TextColorFn colors its children's
			// text (colorfn.go).
			if o.box.parent != nil || o.box.inPara != nil || contentsBox(o.box) {
				t.markPaint(o.box)
				hit = true
			}
		case obsText:
			if o.el.mounted {
				o.el.textStale = true
				t.markBoxes(o.el.primitive())
				hit = true
			}
		case obsField:
			t.bindChanged(o.box)
			hit = true
		}
	}
	return hit
}

// ---------------------------------------------------------------------
// Deferred props of boxes.

// hasPhase reports whether cs has deferred props of phase.
func hasPhase(cs *style.ComputedStyle, phase style.Phase) bool {
	for i := range cs.Deferred {
		if cs.Deferred[i].Phase == phase {
			return true
		}
	}
	return false
}

// resolveDeferred returns the style of box b with its deferred props of
// phase (layout or paint) applied, tracking the Values they read.
func (t *Tree) resolveDeferred(b *Box, phase style.Phase) *style.ComputedStyle {
	mark := t.beginTrack()
	var cs *style.ComputedStyle
	func() {
		done := false
		defer func() {
			if !done { // a closure panicked: keep the old subscriptions
				t.endTrack(mark)
			}
		}()
		cs = b.cs.Resolved(phase)
		done = true
	}()
	reads := t.endTrack(mark)
	if phase == style.PhaseLayout {
		t.observe(&b.layoutObs, obsLayout, nil, b, reads)
	} else {
		t.observe(&b.paintObs, obsPaint, nil, b, reads)
	}
	return cs
}

// markRelayout queues box b for mapping its layout style again: a Value
// read by its layout-phase deferred props changed.
func (t *Tree) markRelayout(b *Box) {
	if b.relayoutQueued {
		return
	}
	b.relayoutQueued = true
	t.relayoutQ = append(t.relayoutQ, b)
	t.dirty()
}

// remapOne maps the layout style of box b again (its layout-phase
// deferred props), marking it dirty for layout when the result changed.
func (t *Tree) remapOne(b *Box) {
	p := b.parent
	if p == nil || b.el == nil || !b.el.mounted || b.cs == nil || p.cs == nil {
		return
	}
	ls := mapLayoutStyle(b, p, hasSpaceAfter(b, spaceLast(p)))
	keepGrid(&ls, &b.ls)
	if ls != b.ls {
		b.ls = ls
		t.engine.MarkDirty(b)
	}
}

// markPaint records that box b must be repainted: a Value read by its
// paint-phase deferred props changed. Only paint is redone: a frame whose
// only work is repainting skips build, style, structure and layout.
func (t *Tree) markPaint(b *Box) {
	if !b.needsPaint {
		b.needsPaint = true
		t.paintQ = append(t.paintQ, b)
	}
	t.repaint = true
	if t.inFrame || t.asked || t.onDirty == nil {
		return
	}
	t.asked = true
	t.onDirty()
}
