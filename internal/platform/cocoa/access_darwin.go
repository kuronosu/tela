//go:build darwin

package cocoa

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/utf16x"
)

// The NSAccessibility bridge (ADR-246): TelaView answers for the root of
// the window's tree, one TelaAccessElement (an NSAccessibilityElement
// subclass made at run time) answers for each exposed node, and
// SetAccessibility posts the notifications of each update. access.go holds
// the OS-independent rules. See "Accessibility" in doc.go.
//
// After AccessKit's macOS adapter, and Chromium's and WebKit's mappings:
//
//   - Activation is the first accessibilityChildren,
//     accessibilityFocusedUIElement or accessibilityHitTest: that AppKit
//     sends TelaView (an AT, or Accessibility Inspector, is looking). Its
//     answer is what exists (nothing before the first update); the first
//     update posts a layout change so the AT asks again. AppKit tells no
//     one when the last client goes, so the bridge never deactivates (the
//     contract allows it; AccessKit does the same).
//   - TelaView stays an ignored view (NSView's default), so the window's
//     children are the root's exposed children; their accessibilityParent
//     is the view, which AppKit resolves to the window.
//   - The class implements every method it may need, and
//     isAccessibilitySelectorAllowed: limits them to the node's role,
//     states and actions (axAllows), else a button would offer
//     setAccessibilityValue: and the text methods.
//   - An element whose node is gone or no longer exposed is "dead": it is
//     released, and while an AT still holds it, it answers as nothing
//     (not an element, no role, no children, every action fails). Its
//     dealloc takes it out of the registry.
//   - Text ranges are UTF-16, the selection of the tree code points
//     (access.go converts). Lines are hard lines: the tree has no layout
//     lines.
//   - Actions are requests: they are queued as AccessActionEvents for the
//     next pass, and the outcome comes back as an update.

// accessTrace, when set (by tests), receives each notification posted,
// action queued and message the hosts receive. It may be called on any
// thread.
var accessTrace func(format string, args ...any)

func atracef(format string, args ...any) {
	if accessTrace != nil {
		accessTrace(format, args...)
	}
}

// windowAccess is the bridge state of a window. AppKit sends the
// NSAccessibility messages of an AT in another process on the main
// thread, but an AX client in the process itself is answered on its own
// thread (the AX client test's queries reached the hosts off the main
// thread and found no window), so everything the queries read is safe
// from any thread, as ADR-246 asks of every bridge: the tree is an
// atomic pointer to an immutable value, the elements are guarded by mu,
// the activation and the actions reach the loop through the backend's
// access inbox, and coordinates off the main thread come from the
// view's screen rectangle recorded on the main thread (geom).
type windowAccess struct {
	active atomic.Bool
	closed atomic.Bool
	tree   atomic.Pointer[axTree]
	// geom is the view's bounds in screen coordinates, as last recorded
	// on the main thread (updateGeometry).
	geom atomic.Pointer[darwin.CGRect]
	mu   sync.Mutex
	// elems are the elements made so far, by node (owned: +1). Guarded
	// by mu, which is never held across a message send.
	elems map[platform.AccessID]darwin.ID
}

// axElem is the registry entry of a TelaAccessElement: its window and
// node; w is nil once the element is dead.
type axElem struct {
	w  *Window
	id platform.AccessID
}

// axElems maps the elements to their entries.
var axElems = struct {
	sync.Mutex
	m map[darwin.ID]*axElem
}{m: map[darwin.ID]*axElem{}}

// asel holds the selectors of the bridge.
var asel = struct {
	dealloc, selectorAllowed, actionNames, performAction, hitTest darwin.SEL
	isKindOfClass, contentRect, frame, actionDescription          darwin.SEL
	instancesRespond                                              darwin.SEL
}{
	actionDescription: darwin.Sel("accessibilityActionDescription:"),
	instancesRespond:  darwin.Sel("instancesRespondToSelector:"),
	dealloc:           darwin.Sel("dealloc"),
	selectorAllowed:   darwin.Sel("isAccessibilitySelectorAllowed:"),
	actionNames:       darwin.Sel("accessibilityActionNames"),
	performAction:     darwin.Sel("accessibilityPerformAction:"),
	hitTest:           darwin.Sel("accessibilityHitTest:"),
	isKindOfClass:     darwin.Sel("isKindOfClass:"),
	contentRect:       darwin.Sel("contentRectForFrameRect:"),
	frame:             darwin.Sel("frame"),
}

// axAccess holds the element class and the selector names the gate knows,
// set up once.
var axAccess struct {
	class, super darwin.Class // TelaAccessElement, NSAccessibilityElement (0: no bridge)
	stringClass  darwin.Class
	gated        map[darwin.SEL]string // read-only after setupAccess
	mu           sync.Mutex            // guards strs and superResponds
	strs         map[string]darwin.ID  // constant NSStrings (owned, kept)
	// superResponds caches whether instances of a superclass respond to
	// a selector.
	superResponds map[[2]uintptr]bool
}

// superSend sends cmd to self from the class super when super's
// instances respond to it, and reports whether it did: an unrecognized
// selector raises an Objective-C exception, which aborts the process
// through purego's frames.
func superSend(self darwin.ID, super darwin.Class, cmd darwin.SEL, args ...any) (darwin.ID, bool) {
	key := [2]uintptr{uintptr(super), uintptr(cmd)}
	axAccess.mu.Lock()
	has, ok := axAccess.superResponds[key]
	axAccess.mu.Unlock()
	if !ok {
		has = darwin.SendBool(darwin.ID(super), asel.instancesRespond, cmd)
		axAccess.mu.Lock()
		if axAccess.superResponds == nil {
			axAccess.superResponds = map[[2]uintptr]bool{}
		}
		axAccess.superResponds[key] = has
		axAccess.mu.Unlock()
		if !has {
			atracef("%s does not implement %s", darwin.GoString(darwin.Send(darwin.ID(super), darwin.Sel("description"))), darwin.SelName(cmd))
		}
	}
	if !has {
		return 0, false
	}
	return darwin.SendSuper(self, super, cmd, args...), true
}

// setupAccess registers TelaAccessElement and binds AppKit's
// NSAccessibility functions; without them the bridge never activates.
func setupAccess() {
	super := darwin.GetClass("NSAccessibilityElement")
	if super == 0 || darwin.LoadAccessibility() != nil {
		return
	}
	methods := append(elementMethods(), attributeMethods()...)
	axAccess.gated = map[darwin.SEL]string{}
	for _, m := range methods {
		axAccess.gated[m.Sel] = darwin.SelName(m.Sel)
	}
	cls, err := darwin.RegisterClass("TelaAccessElement", super, nil, methods)
	if err != nil {
		return
	}
	axAccess.class, axAccess.super = cls, super
	axAccess.stringClass = darwin.GetClass("NSString")
	axAccess.strs = map[string]darwin.ID{}
}

// axStr returns a kept NSString of s, for the constant strings (roles,
// notification names).
func axStr(s string) darwin.ID {
	axAccess.mu.Lock()
	defer axAccess.mu.Unlock()
	if v, ok := axAccess.strs[s]; ok {
		return v
	}
	v := darwin.NSString(s)
	axAccess.strs[s] = v
	return v
}

// axNoteName returns the NSString of a notification: AppKit's constant,
// else its value.
func axNoteName(k axNoteKind) darwin.ID {
	names := axNoteNames[k]
	if names[0] != "" {
		if v := darwin.AppKitString(names[0]); v != 0 {
			return v
		}
	}
	return axStr(names[1])
}

// optString returns an autoreleased NSString of s, or nil for "".
func optString(s string) darwin.ID {
	if s == "" {
		return 0
	}
	return darwin.AutoString(s)
}

// open reports whether the window is open, from any thread.
func (w *Window) accOpen() bool {
	if darwin.IsMainThread() {
		return !w.Closed()
	}
	return !w.acc.closed.Load()
}

// activateAccess starts the bridge of w at its first query by an AT:
// each window activates on its own (contract v5: one bridge per window).
// Safe from any thread.
func (w *Window) activateAccess() {
	if axAccess.class == 0 || !w.accOpen() || !w.acc.active.CompareAndSwap(false, true) {
		return
	}
	atracef("activated window %q (main thread %v)", w.opts.Title, darwin.IsMainThread())
	if darwin.IsMainThread() {
		w.updateGeometry()
	}
	w.b.postAccess(w, platform.AccessibilityEvent{Active: true})
}

// updateGeometry records the view's bounds in screen coordinates for the
// queries off the main thread. Main thread.
func (w *Window) updateGeometry() {
	if !w.acc.active.Load() || w.Closed() {
		return
	}
	b := darwin.SendRect(w.view, sel.bounds)
	r := w.liveScreenRect(platform.Rect{W: float32(b.Size.Width), H: float32(b.Size.Height)})
	w.acc.geom.Store(&r)
}

// SetAccessibility applies update to the window's tree and posts what
// changed to the ATs. It is ignored until an AT activated the bridge. An
// update that does not apply to the tree (core sent a child it never
// sent) panics, as headless does.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
	if w.Closed() || !w.acc.active.Load() {
		return
	}
	old := w.acc.tree.Load()
	var prev *platform.AccessTree
	if old != nil {
		prev = old.tree
	}
	t, removed, err := prev.Apply(&update)
	if err != nil {
		panic(fmt.Sprintf("cocoa: SetAccessibility: %v", err))
	}
	// The exposed structure follows the update's changes when there is
	// one of the tree it was applied to (ADR-277).
	var cur *axTree
	if prev != nil && prev.Root() == t.Root() {
		changed := make([]platform.AccessID, len(update.Nodes))
		for i := range update.Nodes {
			changed[i] = update.Nodes[i].ID
		}
		cur = updateAXTree(old, t, changed, removed)
	} else {
		cur = newAXTree(t)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	w.updateGeometry()
	w.acc.tree.Store(cur)
	for _, n := range axNotes(old, cur, &update) {
		w.postNote(n)
	}
	// Elements that are no longer exposed die (their destruction was
	// posted above when an AT could know them).
	if cur.full {
		for id, e := range w.cachedElements() {
			if !cur.exposed(id) {
				w.killElement(id, e)
			}
		}
	} else {
		for _, id := range cur.lost {
			if e, ok := w.cached(id); ok {
				w.killElement(id, e)
			}
		}
	}
}

// cachedElements returns a copy of the elements made so far.
func (w *Window) cachedElements() map[platform.AccessID]darwin.ID {
	w.acc.mu.Lock()
	defer w.acc.mu.Unlock()
	out := make(map[platform.AccessID]darwin.ID, len(w.acc.elems))
	for id, e := range w.acc.elems {
		out[id] = e
	}
	return out
}

// cached returns the element made for node id, if any.
func (w *Window) cached(id platform.AccessID) (darwin.ID, bool) {
	w.acc.mu.Lock()
	defer w.acc.mu.Unlock()
	e, ok := w.acc.elems[id]
	return e, ok
}

// postNote posts one notification. Notes about an element the AT never
// got are skipped (it has nothing to refresh), except the focus, which
// makes the element. Main thread.
func (w *Window) postNote(n axNote) {
	name := axNoteName(n.kind)
	switch n.kind {
	case noteLayout:
		atracef("post %v", n)
		darwin.AccessibilityPostNotification(w.ns, name)
	case noteAnnounce:
		atracef("post %v", n)
		info := darwin.NewDictionary()
		darwin.DictionarySet(info, axStrConst("NSAccessibilityAnnouncementKey", "AXAnnouncementKey"), darwin.AutoString(n.text))
		// NSAccessibilityPriorityLevel: High 90, Medium 50.
		prio := 50
		if n.assertive {
			prio = 90
		}
		darwin.DictionarySet(info, axStrConst("NSAccessibilityPriorityKey", "AXPriorityKey"), darwin.NSNumberInt(prio))
		darwin.AccessibilityPostNotificationWithUserInfo(w.ns, name, info)
	case noteFocus:
		el := w.view
		if n.id != 0 {
			el = w.element(n.id)
		}
		atracef("post %v", n)
		darwin.AccessibilityPostNotification(el, name)
	case noteDestroyed:
		if e, ok := w.cached(n.id); ok {
			atracef("post %v", n)
			darwin.AccessibilityPostNotification(e, name)
			w.killElement(n.id, e)
		}
	default:
		if e, ok := w.cached(n.id); ok {
			atracef("post %v", n)
			darwin.AccessibilityPostNotification(e, name)
		}
	}
}

// axStrConst returns AppKit's NSString constant name, else value.
func axStrConst(name, value string) darwin.ID {
	if v := darwin.AppKitString(name); v != 0 {
		return v
	}
	return axStr(value)
}

// element returns the object that stands for node id: the view for the
// root, else the node's element, made on first use. Safe from any
// thread.
func (w *Window) element(id platform.AccessID) darwin.ID {
	v := w.acc.tree.Load()
	if v == nil || id == 0 || id == v.root() {
		return w.view
	}
	if e, ok := w.cached(id); ok {
		return e
	}
	e := darwin.Send(darwin.SendClass(axAccess.class, sel.alloc), sel.init)
	if e == 0 {
		return 0
	}
	w.acc.mu.Lock()
	if other, ok := w.acc.elems[id]; ok || w.acc.closed.Load() {
		// Another thread made it meanwhile, or the window closed.
		w.acc.mu.Unlock()
		darwin.Release(e)
		return other
	}
	if w.acc.elems == nil {
		w.acc.elems = map[platform.AccessID]darwin.ID{}
	}
	w.acc.elems[id] = e
	w.acc.mu.Unlock()
	axElems.Lock()
	axElems.m[e] = &axElem{w: w, id: id}
	axElems.Unlock()
	return e
}

// elements returns an autoreleased NSArray of the elements of ids.
func (w *Window) elements(ids []platform.AccessID) darwin.ID {
	objs := make([]darwin.ID, 0, len(ids))
	for _, id := range ids {
		if e := w.element(id); e != 0 {
			objs = append(objs, e)
		}
	}
	return darwin.NSArray(objs...)
}

// killElement makes e dead and releases the window's reference. Main
// thread.
func (w *Window) killElement(id platform.AccessID, e darwin.ID) {
	axElems.Lock()
	if x := axElems.m[e]; x != nil {
		x.w = nil
	}
	axElems.Unlock()
	w.acc.mu.Lock()
	delete(w.acc.elems, id)
	w.acc.mu.Unlock()
	darwin.Release(e)
}

// closeAccess kills the elements of a closing window.
func (w *Window) closeAccess() {
	w.acc.closed.Store(true)
	for id, e := range w.cachedElements() {
		w.killElement(id, e)
	}
	w.acc.tree.Store(nil)
	w.acc.active.Store(false)
}

// screenRect converts r, window logical pixels (points), to screen
// coordinates (points, origin at the bottom left of the main screen):
// through AppKit on the main thread, from the recorded view rectangle
// elsewhere.
func (w *Window) screenRect(r platform.Rect) darwin.CGRect {
	if darwin.IsMainThread() {
		return w.liveScreenRect(r)
	}
	g := w.acc.geom.Load()
	if g == nil {
		return darwin.CGRect{}
	}
	x, y, wd, h := screenRectIn(g.Origin.X, g.Origin.Y, g.Size.Height, r)
	return darwin.CGRect{Origin: darwin.CGPoint{X: x, Y: y}, Size: darwin.CGSize{Width: wd, Height: h}}
}

func (w *Window) liveScreenRect(r platform.Rect) darwin.CGRect {
	vr := darwin.CGRect{
		Origin: darwin.CGPoint{X: float64(r.X), Y: float64(r.Y)},
		Size:   darwin.CGSize{Width: float64(r.W), Height: float64(r.H)},
	}
	inWindow := darwin.SendRect(w.view, imesel.convertRectToView, vr, darwin.ID(0))
	return darwin.SendRect(w.ns, imesel.convertRectToScreen, inWindow)
}

// viewPoint converts a screen point to the view's coordinates (window
// logical pixels), like screenRect.
func (w *Window) viewPoint(p darwin.CGPoint) (x, y float32) {
	if !darwin.IsMainThread() {
		g := w.acc.geom.Load()
		if g == nil {
			return -1, -1
		}
		return viewPointIn(g.Origin.X, g.Origin.Y, g.Size.Height, p.X, p.Y)
	}
	inWindow := darwin.SendPoint(w.ns, isel.convertFrom, p)
	v := darwin.SendPoint(w.view, isel.convertPointFromView, inWindow, darwin.ID(0))
	return float32(v.X), float32(v.Y)
}

// axCall is one NSAccessibility message to an element: its window, tree
// and node (nil when the element is dead or its node is not exposed).
type axCall struct {
	w *Window
	v *axTree
	n *platform.AccessNode
}

// lookup returns the call of element self.
func lookup(self darwin.ID) axCall {
	axElems.Lock()
	e := axElems.m[self]
	var c axCall
	if e != nil {
		c.w = e.w
	}
	axElems.Unlock()
	if c.w == nil || !c.w.accOpen() || !c.w.acc.active.Load() {
		return axCall{}
	}
	if c.v = c.w.acc.tree.Load(); c.v != nil {
		c.n = c.v.node(e.id)
	}
	return c
}

// axQuery runs f, an answer of element self, when its node is alive. Like
// imeQuery it pushes no autorelease pool (the answers are autoreleased
// objects AppKit must get alive); a panic stops the loop (on the main
// thread; elsewhere it is traced and the answer is nil).
func axQuery(self darwin.ID, f func(c axCall)) {
	c := lookup(self)
	if c.n == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			if darwin.IsMainThread() {
				c.w.b.fail(r)
			} else {
				atracef("panic off the main thread: %v", r)
			}
		}
	}()
	f(c)
}

// axAct queues the action a on the node of element self when the node
// supports it; fill sets the action's fields. It reports whether it was
// queued (the AT is told the action happened: the outcome arrives as the
// next update).
func axAct(self darwin.ID, a platform.AccessAction, fill func(c axCall, e *platform.AccessActionEvent) bool) bool {
	ok := false
	axQuery(self, func(c axCall) {
		if !c.n.Actions.Has(a) {
			return
		}
		e := platform.AccessActionEvent{Node: c.n.ID, Action: a}
		if fill != nil && !fill(c, &e) {
			return
		}
		if platform.CheckAccessActionEvent(e) != "" {
			return
		}
		atracef("action %v on %d", a, c.n.ID)
		c.w.b.postAccess(c.w, e)
		ok = true
	})
	return ok
}

// accessWindow returns the window of the host self (TelaView or
// TelaWindow) when the bridge can answer for it.
func accessWindow(self darwin.ID) *Window {
	if axAccess.class == 0 {
		return nil
	}
	if w := windowFor(self); w != nil && w.accOpen() {
		return w
	}
	return nil
}

// accessInbox holds the events of the bridge queued off the main thread
// (activations and actions of an in-process AX client), for the next
// pass.
type accessInbox struct {
	sync.Mutex
	items []accessItem
}

type accessItem struct {
	w *Window
	e platform.Event
}

// postAccess queues e, an event of the bridge, for w from any thread and
// signals the run loop.
func (b *Backend) postAccess(w *Window, e platform.Event) {
	if darwin.IsMainThread() {
		b.enqueue(w, e)
	} else {
		b.accIn.Lock()
		b.accIn.items = append(b.accIn.items, accessItem{w, e})
		b.accIn.Unlock()
	}
	b.notify()
}

// drainAccess moves the events queued off the main thread to the loop.
// Main thread.
func (b *Backend) drainAccess() {
	b.accIn.Lock()
	items := b.accIn.items
	b.accIn.items = nil
	b.accIn.Unlock()
	for _, it := range items {
		if !it.w.Closed() {
			b.enqueue(it.w, it.e)
		}
	}
}

// elementMethods are TelaAccessElement's methods.
func elementMethods() []darwin.Method {
	str := func(name string, f func(c axCall) string) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL) darwin.ID {
			var res darwin.ID
			axQuery(self, func(c axCall) { res = optString(f(c)) })
			return res
		}}
	}
	boolean := func(name string, f func(c axCall) bool) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL) bool {
			res := false
			axQuery(self, func(c axCall) { res = f(c) })
			return res
		}}
	}
	obj := func(name string, f func(c axCall) darwin.ID) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL) darwin.ID {
			var res darwin.ID
			axQuery(self, func(c axCall) { res = f(c) })
			return res
		}}
	}
	integer := func(name string, f func(c axCall) int) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL) int {
			res := 0
			axQuery(self, func(c axCall) { res = f(c) })
			return res
		}}
	}
	rng := func(name string, f func(c axCall) darwin.NSRange) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL) darwin.NSRange {
			res := darwin.NSRange{}
			axQuery(self, func(c axCall) { res = f(c) })
			return res
		}}
	}
	act := func(name string, a platform.AccessAction) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL) bool {
			return axAct(self, a, nil)
		}}
	}
	text := func(c axCall) string { s, _ := axText(c.n); return s }
	u16 := func(c axCall) int { return u16Len(text(c)) }
	return []darwin.Method{
		{Sel: asel.dealloc, Fn: func(self darwin.ID, cmd darwin.SEL) {
			axElems.Lock()
			delete(axElems.m, self)
			axElems.Unlock()
			// NSObject's dealloc: every class has it.
			darwin.SendSuper(self, axAccess.super, cmd)
		}},
		{Sel: asel.selectorAllowed, Fn: func(self darwin.ID, cmd darwin.SEL, s darwin.SEL) bool {
			allowed, known := false, false
			if name, ok := axAccess.gated[s]; ok {
				axQuery(self, func(c axCall) { allowed, known = axAllows(c.v, c.n, name) })
			}
			if known {
				return allowed
			}
			// A BOOL comes back in the low byte of the register.
			res, ok := superSend(self, axAccess.super, cmd, s)
			return !ok || res&0xff != 0
		}},
		boolean("isAccessibilityElement", func(axCall) bool { return true }),
		boolean("isAccessibilityEnabled", func(c axCall) bool { return !c.n.Has(platform.AccessDisabled) }),
		boolean("isAccessibilityFocused", func(c axCall) bool { return c.v.focus() == c.n.ID }),
		boolean(axSelRequired, func(c axCall) bool { return c.n.Has(platform.AccessRequired) }),
		boolean(axSelExpanded, func(c axCall) bool { return c.n.Expanded == platform.ToggleTrue }),
		boolean(axSelDisclosed, func(c axCall) bool { return c.n.Expanded == platform.ToggleTrue }),
		boolean(axSelSelected, func(c axCall) bool { return c.n.Selected == platform.ToggleTrue }),
		boolean("isAccessibilityModal", func(c axCall) bool { return c.n.Has(platform.AccessModal) }),
		obj("accessibilityRole", func(c axCall) darwin.ID { r, _ := axRole(c.n); return axStr(r) }),
		obj("accessibilitySubrole", func(c axCall) darwin.ID {
			if _, s := axRole(c.n); s != "" {
				return axStr(s)
			}
			return 0
		}),
		obj(axSelRoleDescription, func(c axCall) darwin.ID {
			if d := axRoleDescription(c.n); d != "" {
				return darwin.AutoString(d)
			}
			r, s := axRole(c.n)
			sub := darwin.ID(0)
			if s != "" {
				sub = axStr(s)
			}
			return darwin.AccessibilityRoleDescription(axStr(r), sub)
		}),
		str(axSelLabel, func(c axCall) string { return c.n.Name }),
		str(axSelHelp, func(c axCall) string { return c.n.Description }),
		str(axSelPlaceholder, func(c axCall) string { return c.n.Placeholder }),
		str(axSelValueDescription, func(c axCall) string { return axValueDescription(c.n) }),
		obj(axSelValue, func(c axCall) darwin.ID {
			switch v := axValueOf(c.n); v.kind {
			case axStringValue:
				return darwin.AutoString(v.str)
			case axNumberValue:
				if v.num == float64(int(v.num)) {
					return darwin.NSNumberInt(int(v.num))
				}
				return darwin.NSNumberFloat(v.num)
			}
			return 0
		}),
		obj(axSelMinValue, func(c axCall) darwin.ID {
			if c.n.Range.HasMin {
				return darwin.NSNumberFloat(c.n.Range.Min)
			}
			return 0
		}),
		obj(axSelMaxValue, func(c axCall) darwin.ID {
			if c.n.Range.HasMax {
				return darwin.NSNumberFloat(c.n.Range.Max)
			}
			return 0
		}),
		obj(axSelURL, func(c axCall) darwin.ID {
			if c.n.URL == "" {
				return 0
			}
			return darwin.URLString(c.n.URL)
		}),
		integer(axSelOrientation, func(c axCall) int {
			// NSAccessibilityOrientation: unknown 0, vertical 1, horizontal 2.
			switch c.n.Orientation {
			case "vertical":
				return 1
			case "horizontal":
				return 2
			}
			return 0
		}),
		integer(axSelIndex, func(c axCall) int { i, _ := c.v.index(c.n.ID); return i }),
		integer(axSelDisclosureLevel, func(c axCall) int { return max(c.n.Level-1, 0) }),
		obj("accessibilityParent", func(c axCall) darwin.ID { return c.w.element(c.v.up(c.n.ID)) }),
		obj("accessibilityChildren", func(c axCall) darwin.ID { return c.w.elements(c.v.kids(c.n.ID)) }),
		obj(axSelSelectedChildren, func(c axCall) darwin.ID { return c.w.elements(c.v.selectedChildren(c.n.ID)) }),
		obj(axSelRows, func(c axCall) darwin.ID { return c.w.elements(c.v.rows(c.n.ID)) }),
		obj("accessibilityWindow", func(c axCall) darwin.ID { return c.w.ns }),
		obj("accessibilityTopLevelUIElement", func(c axCall) darwin.ID { return c.w.ns }),
		{Sel: darwin.Sel("accessibilityFrame"), Fn: func(self darwin.ID, _ darwin.SEL) darwin.CGRect {
			var res darwin.CGRect
			axQuery(self, func(c axCall) { res = c.w.screenRect(c.n.Bounds) })
			return res
		}},
		{Sel: asel.hitTest, Fn: func(self darwin.ID, _ darwin.SEL, p darwin.CGPoint) darwin.ID {
			res := self
			axQuery(self, func(c axCall) {
				x, y := c.w.viewPoint(p)
				if id := c.v.hitTest(c.n.ID, x, y); id != 0 {
					res = c.w.element(id)
				}
			})
			return res
		}},
		// Text.
		integer(axSelNumChars, u16),
		rng(axSelSelectedRange, func(c axCall) darwin.NSRange {
			loc, n, ok := selectionU16(c.n)
			if !ok {
				return darwin.NSRange{Location: uint(u16(c))}
			}
			return darwin.NSRange{Location: uint(loc), Length: uint(n)}
		}),
		str(axSelSelectedText, func(c axCall) string {
			loc, n, ok := selectionU16(c.n)
			if !ok {
				return ""
			}
			return u16Slice(text(c), loc, n)
		}),
		rng(axSelVisibleRange, func(c axCall) darwin.NSRange { return darwin.NSRange{Length: uint(u16(c))} }),
		integer(axSelInsertionLine, func(c axCall) int {
			s := text(c)
			if !c.n.HasSelection {
				return 0
			}
			return lineForIndex(s, utf16x.FromCodePoints(s, c.n.Selection.Focus))
		}),
		{Sel: darwin.Sel(axSelStringForRange), Fn: func(self darwin.ID, _ darwin.SEL, r darwin.NSRange) darwin.ID {
			var res darwin.ID
			axQuery(self, func(c axCall) {
				if loc, n, ok := axRange(r); ok {
					res = darwin.AutoString(u16Slice(text(c), loc, n))
				}
			})
			return res
		}},
		{Sel: darwin.Sel(axSelLineForIndex), Fn: func(self darwin.ID, _ darwin.SEL, i int) int {
			res := 0
			axQuery(self, func(c axCall) { res = lineForIndex(text(c), max(i, 0)) })
			return res
		}},
		{Sel: darwin.Sel(axSelRangeForLine), Fn: func(self darwin.ID, _ darwin.SEL, line int) darwin.NSRange {
			res := darwin.NSRange{Location: darwin.NSNotFound}
			axQuery(self, func(c axCall) {
				if loc, n, ok := rangeForLine(text(c), line); ok {
					res = darwin.NSRange{Location: uint(loc), Length: uint(n)}
				}
			})
			return res
		}},
		// The frame of a range is the element's: the tree has no glyph
		// positions.
		{Sel: darwin.Sel(axSelFrameForRange), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.NSRange) darwin.CGRect {
			var res darwin.CGRect
			axQuery(self, func(c axCall) { res = c.w.screenRect(c.n.Bounds) })
			return res
		}},
		// Actions.
		act(axSelPress, platform.AccessActionDefault),
		act(axSelIncrement, platform.AccessActionIncrement),
		act(axSelDecrement, platform.AccessActionDecrement),
		act(axSelShowMenu, platform.AccessActionShowMenu),
		{Sel: darwin.Sel(axSelSetFocused), Fn: func(self darwin.ID, _ darwin.SEL, focused bool) {
			if focused {
				axAct(self, platform.AccessActionFocus, nil)
			}
		}},
		{Sel: darwin.Sel(axSelSetValue), Fn: func(self darwin.ID, _ darwin.SEL, value darwin.ID) {
			if value == 0 || !darwin.SendBool(value, asel.isKindOfClass, axAccess.stringClass) {
				return
			}
			s := imeString(value)
			axAct(self, platform.AccessActionSetValue, func(c axCall, e *platform.AccessActionEvent) bool {
				if !textField(c.n) || c.n.Has(platform.AccessReadOnly) {
					return false
				}
				e.Value = s
				return true
			})
		}},
		{Sel: darwin.Sel(axSelSetSelectedRange), Fn: func(self darwin.ID, _ darwin.SEL, r darwin.NSRange) {
			axAct(self, platform.AccessActionSetSelection, func(c axCall, e *platform.AccessActionEvent) bool {
				loc, n, ok := axRange(r)
				if !ok {
					return false
				}
				s := text(c)
				e.Selection = platform.AccessSelection{Anchor: u16ToCP(s, loc), Focus: u16ToCP(s, loc+n)}
				return true
			})
		}},
		{Sel: darwin.Sel(axSelSetExpanded), Fn: func(self darwin.ID, _ darwin.SEL, expanded bool) { axExpand(self, expanded) }},
		{Sel: darwin.Sel(axSelSetDisclosed), Fn: func(self darwin.ID, _ darwin.SEL, expanded bool) { axExpand(self, expanded) }},
		{Sel: darwin.Sel(axSelSetSelected), Fn: func(self darwin.ID, _ darwin.SEL, selected bool) {
			// Selecting an item is clicking it (AccessKit's mapping); an
			// item is not unselected by a click.
			axAct(self, platform.AccessActionDefault, func(c axCall, _ *platform.AccessActionEvent) bool {
				return selected && c.n.Selected == platform.ToggleFalse
			})
		}},
		// Scrolling into view has no NSAccessibility method: it is the
		// action AXScrollToVisible of the older, name-based API, whose
		// methods NSAccessibilityElement does not implement (macOS 26:
		// sending super accessibilityActionNames raised "unrecognized
		// selector"). The element answers the whole name-based action API
		// itself, from the node's actions (axActionNames), and never sends
		// super there.
		{Sel: asel.actionNames, Fn: func(self darwin.ID, _ darwin.SEL) darwin.ID {
			out := darwin.NewMutableArray()
			axQuery(self, func(c axCall) {
				for _, name := range axActionNames(c.n) {
					darwin.ArrayAdd(out, axStr(name))
				}
			})
			return out
		}},
		{Sel: asel.performAction, Fn: func(self darwin.ID, _ darwin.SEL, action darwin.ID) {
			if a, ok := axActionNamed(darwin.GoString(action)); ok {
				axAct(self, a, nil)
			}
		}},
		{Sel: asel.actionDescription, Fn: func(self darwin.ID, _ darwin.SEL, action darwin.ID) darwin.ID {
			if action == 0 {
				return 0
			}
			if d := darwin.AccessibilityActionDescription(action); d != 0 {
				return d
			}
			return darwin.AutoString(axActionDescription(darwin.GoString(action)))
		}},
	}
}

// axExpand queues expand or collapse.
func axExpand(self darwin.ID, expanded bool) {
	a := platform.AccessActionCollapse
	if expanded {
		a = platform.AccessActionExpand
	}
	axAct(self, a, nil)
}

// axRange converts an NSRange from AppKit to a UTF-16 location and
// length; false for NSNotFound and absurd values.
func axRange(r darwin.NSRange) (loc, n int, ok bool) {
	if r.Location >= darwin.NSNotFound || r.Location > 1<<31 || r.Length > 1<<31 {
		return 0, 0, false
	}
	return int(r.Location), int(r.Length), true
}
