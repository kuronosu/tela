package core

import (
	"reflect"
	"slices"
	"sync/atomic"
	"time"
)

// Enter and exit animations (ARCHITECTURE section 12, ADR-080), after
// Motion's AnimatePresence.
//
// # Presence
//
// A Presence is a component Element without a function, like a Provider:
// its children become children of its parent's box. When a build removes
// one of its children (by key, or by position for an h.If hole), the
// child is kept mounted while it exits, at its old position among the
// children (it keeps its place in the layout: Motion's default, not
// popLayout), with its last node: it is not updated by the parent's
// builds, but its own components still build when their state changes.
// A child exits when it has work to do: primitives with Exit classes in
// its subtree (not inside a nested Presence, whose children exit only
// with it), or components that called UsePresence. A child without either
// unmounts at once, as anywhere else.
//
// The exit applies the Exit classes of those primitives on top of their
// classes, so their transitions (T4's implicit transitions) run to the
// exit style, and rebuilds the UsePresence components with present false.
// It ends when the transitions of those primitives have ended and every
// UsePresence component has called done; then the child unmounts. A child
// added back while it exits is the same Element again: the exit is
// cancelled, the Exit classes removed, so the transitions run back from
// where they are, and UsePresence reports present again.
//
// # An exiting child is inert
//
// While a child exits, it and its subtree are inert (HTML's inert): they
// are not hit by the pointer, cannot take the focus (the focus inside
// them is lost as on unmount, and their focus scopes end, restoring the
// focus), and are left out of the semantics tree (aria snapshots,
// locators).
//
// # When an exit ends
//
// The check runs after the style phase of the frame that starts the exit
// (exit classes without a transition, reduced motion and trees without a
// clock end at once, in that frame), in the animation phase of every
// frame (transitions that ended), and when a UsePresence component calls
// done.

// presenceMarker identifies Presence nodes: its code pointer is their
// component type.
func presenceMarker() {}

var presenceTyp = reflect.ValueOf(presenceMarker).Pointer()

// Presence returns a node whose children are kept mounted while they exit
// (tela.h.Presence). It has no box; only Key is accepted as a prop.
func Presence(children ...Child) Node {
	d := &desc{kind: nodeComponent, comp: &compDesc{typ: presenceTyp, presence: true}, children: make([]Node, 0, len(children))}
	for _, c := range children {
		d.addGroupChild(c, "Presence", "it has no box (put the prop on its children)")
	}
	return Node{d}
}

// Exit adds classes that a primitive takes when the child of a Presence it
// belongs to is removed: the element transitions to them before the child
// unmounts (tela.h.Exit). Exit may repeat; the strings concatenate.
func Exit(classes string) Prop {
	return Prop{name: "Exit", apply: func(d *desc) {
		if classes == "" {
			return
		}
		if d.exit == "" {
			d.exit = classes
		} else {
			d.exit += " " + classes
		}
	}}
}

// isPresence reports whether el is a Presence element.
func (el *Element) isPresence() bool {
	return el.kind == elemComponent && el.comp != nil && el.comp.presence
}

// presenceChild is the state of a child of a Presence: the UsePresence
// components in its subtree and its exit, when it exits.
type presenceChild struct {
	users []*presenceSlot
	exit  *exitState
}

// exitState is the exit of a child of a Presence.
type exitState struct {
	el *Element
	// boxes are the primitives whose Exit classes apply.
	boxes []*Element
	// armed: a style phase ran since the exit began, so the transitions
	// it starts are known.
	armed bool
	// gone: the exit ended or was cancelled.
	gone bool
}

// presenceState is the Tree's list of exits in progress (UI thread).
type presenceState struct {
	exits []*exitState
}

func (el *Element) presChild() *presenceChild {
	if el.pchild == nil {
		el.pchild = &presenceChild{}
	}
	return el.pchild
}

// exiting reports whether el is a child of a Presence that exits.
func (el *Element) exiting() bool { return el.pchild != nil && el.pchild.exit != nil }

// mountPresence mounts the Presence Element el.
func (t *Tree) mountPresence(el *Element) { t.reconcilePresence(el, el.node.d.children) }

// updatePresence brings Presence el up to date with n.
func (t *Tree) updatePresence(el *Element, n Node) {
	d := n.d
	if el.node.d == d {
		if el.dirtyKids {
			t.descend(el)
		}
		return
	}
	el.node, el.comp = n, d.comp
	t.reconcilePresence(el, d.children)
	el.dirtyKids = false
}

// reconcilePresence reconciles the children of Presence p: children that
// left begin their exit and stay, at their old index (Motion's merge),
// and exiting children that are back cancel it.
func (t *Tree) reconcilePresence(p *Element, children []Node) {
	buf := t.getFlat()
	entries := flatten(buf[:0], children)
	if len(p.children) > 0 {
		entries = t.presenceEntries(p, entries)
	}
	t.reconcileEntries(p, entries)
	t.putFlat(entries)
}

// presenceEntries returns entries with the exiting children of p inserted
// at their old indices.
func (t *Tree) presenceEntries(p *Element, entries []flatEntry) []flatEntry {
	ids := make(map[childID]int, len(entries))
	for i := range entries {
		if _, dup := ids[entries[i].id()]; !dup {
			ids[entries[i].id()] = i
		}
	}
	type kept struct {
		at int
		el *Element
	}
	var keep []kept
	for i, o := range p.children {
		j, found := ids[o.id()]
		switch {
		case found && o.sameType(entries[j].n):
			if o.exiting() {
				t.cancelExit(o) // added back
			}
		case found:
			// The same identity with another type: replaced at once (the
			// exit of an exiting one ends with its unmount).
		case o.exiting() || t.beginExit(o):
			keep = append(keep, kept{i, o})
		}
	}
	if len(keep) == 0 {
		return entries
	}
	out := entries
	for _, k := range keep {
		o := k.el
		e := flatEntry{n: o.node, scope: o.scope, sub: o.sub, key: o.key, keyed: o.keyed}
		out = slices.Insert(out, min(k.at, len(out)), e)
	}
	return out
}

// collectExit appends the primitives of el's subtree with Exit classes,
// not entering nested Presences.
func collectExit(el *Element, out *[]*Element) {
	if el.kind == elemPrimitive && el.node.d.exit != "" {
		*out = append(*out, el)
	}
	if el.isPresence() {
		return
	}
	for _, c := range el.children {
		collectExit(c, out)
	}
}

// beginExit starts the exit of o, a child of a Presence that a build
// removed, and reports whether it has one (Exit classes or UsePresence
// components); without one o is unmounted by the reconciliation.
func (t *Tree) beginExit(o *Element) bool {
	var boxes []*Element
	collectExit(o, &boxes)
	if len(boxes) == 0 && (o.pchild == nil || len(o.pchild.users) == 0) {
		return false
	}
	pc := o.presChild()
	ex := &exitState{el: o, boxes: boxes}
	pc.exit = ex
	t.pres.exits = append(t.pres.exits, ex)
	setInert(o, true)
	t.inertFocus(o)
	for _, b := range boxes {
		b.exitOn = true
		b.box.classesDirty = true
		t.markStyle(b)
	}
	for _, u := range pc.users {
		u.gen.Add(1)
		u.done = false
		t.markBuild(u.el)
	}
	return true
}

// cancelExit ends the exit of o, added back to its Presence: its Exit
// classes are removed (its transitions run back from where they are) and
// its UsePresence components build again with present true.
func (t *Tree) cancelExit(o *Element) {
	ex := o.pchild.exit
	o.pchild.exit = nil
	ex.gone = true
	setInert(o, false)
	t.restoreScopes(o)
	for _, b := range ex.boxes {
		if b.mounted {
			b.exitOn = false
			b.box.classesDirty = true
			t.markStyle(b)
		}
	}
	for _, u := range o.pchild.users {
		u.gen.Add(1)
		t.markBuild(u.el)
	}
}

// setInert marks el's subtree inert (on) or not; the subtrees of nested
// exiting children stay inert.
func setInert(el *Element, on bool) {
	el.inert = on
	for _, c := range el.children {
		setInert(c, on || c.exiting())
	}
}

// inertFocus handles the focus of o's subtree becoming inert, as an
// unmount would (unmountFocus): its focus scopes end (restoring the
// focus) and a focused element in it loses the focus without OnBlur.
func (t *Tree) inertFocus(o *Element) {
	f := &t.foc
	var walk func(e *Element)
	walk = func(e *Element) {
		if e.kind == elemPrimitive {
			if s := e.fscope; s != nil {
				t.removeScope(s)
				e.fscope = nil
				if s.opts.restore && s.prev != nil && (f.el == nil || isInside(f.el, e)) {
					f.restores = append(f.restores, s.prev)
				}
			}
			if f.space == e {
				t.cancelSpace()
			}
			if f.auto == e {
				f.auto = nil
			}
			if f.start == e {
				f.start = nil
			}
		}
		for _, c := range e.children {
			walk(c)
		}
	}
	walk(o)
	if f.el != nil && isInside(f.el, o) {
		f.el = nil
		t.syncFocus()
	}
}

// restoreScopes adds back the focus scopes of o's subtree, whose exit was
// cancelled.
func (t *Tree) restoreScopes(o *Element) {
	var walk func(e *Element)
	walk = func(e *Element) {
		if e.kind == elemPrimitive && e.mounted && e.fscope == nil {
			if sc := e.node.d.scope; sc != nil {
				t.addScope(e, *sc)
			}
		}
		for _, c := range e.children {
			walk(c)
		}
	}
	walk(o)
}

// exitClasses returns the class strings of primitive el: its Class props,
// and its Exit classes while its exit runs.
func exitClasses(el *Element) []string {
	d := el.node.d
	if !el.exitOn || d.exit == "" {
		return d.classes
	}
	return append(slices.Clip(d.classes), d.exit)
}

// exitDone reports whether the exit ex is over at now: every UsePresence
// component called done and the transitions of its Exit primitives ended.
func (t *Tree) exitDone(ex *exitState, now time.Duration) bool {
	for _, u := range ex.el.pchild.users {
		if !u.done {
			return false
		}
	}
	for _, b := range ex.boxes {
		if !b.mounted || b.box.anim == nil {
			continue
		}
		for i := range b.box.anim.trans {
			if now < b.box.anim.trans[i].End() {
				return false
			}
		}
	}
	return true
}

// checkExits ends the exits that are over at now. arm marks the exits a
// style phase has seen (the transitions they start are known); exits not
// armed yet wait.
func (t *Tree) checkExits(now time.Duration, arm bool) bool {
	if len(t.pres.exits) == 0 {
		return false
	}
	ended := false
	live := t.pres.exits[:0]
	for _, ex := range t.pres.exits {
		switch {
		case ex.gone || !ex.el.mounted:
			continue
		case arm:
			ex.armed = true
		}
		if ex.armed && t.exitDone(ex, now) {
			t.finishExit(ex)
			ended = true
			continue
		}
		live = append(live, ex)
	}
	clear(t.pres.exits[len(live):])
	t.pres.exits = live
	return ended
}

// finishExit unmounts the child whose exit ended.
func (t *Tree) finishExit(ex *exitState) {
	ex.gone = true
	o := ex.el
	o.pchild.exit = nil
	p := o.parent
	kids := make([]*Element, 0, len(p.children)-1)
	for _, c := range p.children {
		if c != o {
			kids = append(kids, c)
		}
	}
	p.children = kids
	t.unmount(o)
	pr := p.primitive()
	t.markBoxes(pr)
	t.markStructural(pr)
	t.markAutoDir(p)
}

// presencePhase ends, after the style phase, the exits whose Exit
// classes started no transition (or whose components called done). It
// reports whether it unmounted anything.
func (t *Tree) presencePhase() bool {
	return t.checkExits(t.anim.tk.Now(), true)
}

// ---------------------------------------------------------------------
// UsePresence.

// presenceSlot is the slot of one UsePresence call.
type presenceSlot struct {
	el *Element
	// root is the child of a Presence the component is in (nil outside
	// any Presence).
	root *Element
	// done: the component called done during the current exit.
	done bool
	// gen counts the exits and cancellations of root; done applies only
	// to the exit it was called in. Read by doneFn on any goroutine.
	gen    atomic.Uint64
	doneFn func()
}

func (s *presenceSlot) unmount(t *Tree) {
	if s.root == nil || s.root.pchild == nil {
		return
	}
	pc := s.root.pchild
	if i := slices.Index(pc.users, s); i >= 0 {
		pc.users = slices.Delete(pc.users, i, i+1)
	}
	if pc.exit != nil && !pc.exit.gone && s.root.mounted {
		t.dirty() // the exit may be over without this component: check it
	}
}

// markDone records a done call made during exit gen, on the UI thread,
// and ends the exit when it is over.
func (s *presenceSlot) markDone(gen uint64) {
	if gen != s.gen.Load() || !s.el.mounted || s.root == nil || !s.root.exiting() {
		return
	}
	s.done = true
	t := s.el.tree
	if ex := s.root.pchild.exit; ex.armed && t.exitDone(ex, t.anim.tk.Now()) {
		t.checkExits(t.anim.tk.Now(), false)
	}
}

// presenceRoot returns the child of the nearest Presence above (or at)
// el, or nil.
func presenceRoot(el *Element) *Element {
	for e := el; e.parent != nil; e = e.parent {
		if e.parent.isPresence() {
			return e
		}
	}
	return nil
}

// UsePresence reports whether the component is present (tela.UsePresence):
// false while the child of a Presence it is in exits. done ends the
// component's part of the exit; it is the same function on every build,
// safe from any goroutine, and does nothing while the component is
// present. Outside any Presence the component is always present.
func UsePresence() (present bool, done func()) {
	el, i := nextSlot(hookPresence)
	var s *presenceSlot
	if i < len(el.hooks) {
		var ok bool
		s, ok = el.hooks[i].(*presenceSlot)
		if !ok {
			el.tree.slotMismatch(el, i, "UsePresence")
		}
	} else {
		s = &presenceSlot{el: el, root: presenceRoot(el)}
		t := el.tree
		s.doneFn = func() {
			g := s.gen.Load()
			t.enqueue(update{post: func() { s.markDone(g) }})
		}
		if s.root != nil {
			pc := s.root.presChild()
			pc.users = append(pc.users, s)
		}
		el.hooks = append(el.hooks, s)
	}
	return s.root == nil || !s.root.exiting(), s.doneFn
}

// UsePresenceTween is the entrance and exit of a component inside a
// Presence child as one Value (tela.UsePresenceTween): 0 to 1 when it
// mounts, 1 to 0 while its child exits, calling done when the exit
// arrives. It is a tween unless the options are spring options
// (Stiffness, Damping, Bounce, Velocity or Spring), then a spring. An
// OnDone option runs before done, after each of the two motions.
func UsePresenceTween(opts ...AnimOption) *Value[float32] {
	present, done := UsePresence()
	target := float32(1)
	if !present {
		target = 0
	}
	all := make([]AnimOption, 0, len(opts)+2)
	all = append(all, Initial(float32(0)))
	spring := false
	var user []func()
	for _, o := range opts {
		switch op := o.o; {
		case op == nil:
		case op.kind == optOnDone && op.fn != nil:
			user = append(user, op.fn)
		default:
			switch op.kind {
			case optStiffness, optDamping, optBounce, optSpring, optVelocity:
				spring = true
			}
			all = append(all, o)
		}
	}
	all = append(all, OnDone(func() {
		for _, f := range user {
			f()
		}
		done()
	}))
	if spring {
		return UseSpring(target, all...)
	}
	return UseTween(target, all...)
}
