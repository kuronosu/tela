//go:build darwin

package cocoa

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
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

// accessTrace, when set (by tests), receives each notification posted.
var accessTrace func(format string, args ...any)

func atracef(format string, args ...any) {
	if accessTrace != nil {
		accessTrace(format, args...)
	}
}

// windowAccess is the bridge state of a window. AppKit sends every
// NSAccessibility message on the main thread, as SetAccessibility runs;
// the tree is still held atomically, as ADR-246 asks of every bridge.
type windowAccess struct {
	active bool
	tree   atomic.Pointer[axTree]
	// elems are the elements made so far, by node (owned: +1).
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
	isKindOfClass, contentRect, frame                             darwin.SEL
}{
	dealloc:         darwin.Sel("dealloc"),
	selectorAllowed: darwin.Sel("isAccessibilitySelectorAllowed:"),
	actionNames:     darwin.Sel("accessibilityActionNames"),
	performAction:   darwin.Sel("accessibilityPerformAction:"),
	hitTest:         darwin.Sel("accessibilityHitTest:"),
	isKindOfClass:   darwin.Sel("isKindOfClass:"),
	contentRect:     darwin.Sel("contentRectForFrameRect:"),
	frame:           darwin.Sel("frame"),
}

// axAccess holds the element class and the selector names the gate knows,
// set up once.
var axAccess struct {
	class, super darwin.Class // TelaAccessElement, NSAccessibilityElement (0: no bridge)
	stringClass  darwin.Class
	gated        map[darwin.SEL]string
	strs         map[string]darwin.ID // constant NSStrings (owned, kept)
}

// axScrollToVisible is the AX action of scrolling into view, which has
// no method of its own in the NSAccessibility protocol.
const axScrollToVisible = "AXScrollToVisible"

// setupAccess registers TelaAccessElement and binds AppKit's
// NSAccessibility functions; without them the bridge never activates.
func setupAccess() {
	super := darwin.GetClass("NSAccessibilityElement")
	if super == 0 || darwin.LoadAccessibility() != nil {
		return
	}
	methods := elementMethods()
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

// activateAccess starts the bridge of w at the first query of an AT.
func (w *Window) activateAccess() {
	if w.acc.active || axAccess.class == 0 || w.Closed() {
		return
	}
	w.acc.active = true
	w.acc.elems = map[platform.AccessID]darwin.ID{}
	atracef("activated")
	w.b.enqueue(w, platform.AccessibilityEvent{Active: true})
	w.b.notify()
}

// SetAccessibility applies update to the window's tree and posts what
// changed to the ATs. It is ignored until an AT activated the bridge. An
// update that does not apply to the tree (core sent a child it never
// sent) panics, as headless does.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
	if w.Closed() || !w.acc.active {
		return
	}
	old := w.acc.tree.Load()
	var prev *platform.AccessTree
	if old != nil {
		prev = old.tree
	}
	t, _, err := prev.Apply(&update)
	if err != nil {
		panic(fmt.Sprintf("cocoa: SetAccessibility: %v", err))
	}
	cur := newAXTree(t)
	w.acc.tree.Store(cur)
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	for _, n := range axNotes(old, cur, &update) {
		w.postNote(n)
	}
	// Elements that are no longer exposed die (their destruction was
	// posted above when an AT could know them).
	for id, e := range w.acc.elems {
		if !cur.exposed(id) {
			w.killElement(id, e)
		}
	}
}

// postNote posts one notification. Notes about an element the AT never
// got are skipped (it has nothing to refresh), except the focus, which
// makes the element.
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
		if e, ok := w.acc.elems[n.id]; ok {
			atracef("post %v", n)
			darwin.AccessibilityPostNotification(e, name)
			w.killElement(n.id, e)
		}
	default:
		if e, ok := w.acc.elems[n.id]; ok {
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
// root, else the node's element, made on first use.
func (w *Window) element(id platform.AccessID) darwin.ID {
	v := w.acc.tree.Load()
	if v == nil || id == 0 || id == v.root() {
		return w.view
	}
	if e, ok := w.acc.elems[id]; ok {
		return e
	}
	e := darwin.Send(darwin.SendClass(axAccess.class, sel.alloc), sel.init)
	if e == 0 {
		return 0
	}
	axElems.Lock()
	axElems.m[e] = &axElem{w: w, id: id}
	axElems.Unlock()
	w.acc.elems[id] = e
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

// killElement makes e dead and releases the window's reference.
func (w *Window) killElement(id platform.AccessID, e darwin.ID) {
	axElems.Lock()
	if x := axElems.m[e]; x != nil {
		x.w = nil
	}
	axElems.Unlock()
	delete(w.acc.elems, id)
	darwin.Release(e)
}

// closeAccess kills the elements of a closing window.
func (w *Window) closeAccess() {
	for id, e := range w.acc.elems {
		w.killElement(id, e)
	}
	w.acc.tree.Store(nil)
	w.acc.active = false
}

// screenRect converts r, window logical pixels (points), to screen
// coordinates (points, origin at the bottom left of the main screen).
func (w *Window) screenRect(r platform.Rect) darwin.CGRect {
	vr := darwin.CGRect{
		Origin: darwin.CGPoint{X: float64(r.X), Y: float64(r.Y)},
		Size:   darwin.CGSize{Width: float64(r.W), Height: float64(r.H)},
	}
	inWindow := darwin.SendRect(w.view, imesel.convertRectToView, vr, darwin.ID(0))
	return darwin.SendRect(w.ns, imesel.convertRectToScreen, inWindow)
}

// viewPoint converts a screen point to the view's coordinates (window
// logical pixels).
func (w *Window) viewPoint(p darwin.CGPoint) (x, y float32) {
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

// lookup returns the call of element self. A message off the main
// thread (AppKit sends none) gets no answer: the elements are made there.
func lookup(self darwin.ID) axCall {
	if !darwin.IsMainThread() {
		return axCall{}
	}
	axElems.Lock()
	e := axElems.m[self]
	var c axCall
	if e != nil {
		c.w = e.w
	}
	axElems.Unlock()
	if c.w == nil || c.w.Closed() || !c.w.acc.active {
		return axCall{}
	}
	if c.v = c.w.acc.tree.Load(); c.v != nil {
		c.n = c.v.node(e.id)
	}
	return c
}

// axQuery runs f, an answer of element self, when its node is alive. Like
// imeQuery it pushes no autorelease pool (the answers are autoreleased
// objects AppKit must get alive); a panic stops the loop.
func axQuery(self darwin.ID, f func(c axCall)) {
	c := lookup(self)
	if c.n == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			c.w.b.fail(r)
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
		c.w.b.enqueue(c.w, e)
		c.w.b.notify()
		ok = true
	})
	return ok
}

// accessWindow returns the window of the view self when the bridge can
// answer for it: the class exists and the message came on the main
// thread.
func accessWindow(self darwin.ID) *Window {
	if axAccess.class == 0 || !darwin.IsMainThread() {
		return nil
	}
	if w := windowFor(self); w != nil && !w.Closed() {
		return w
	}
	return nil
}

// accessViewMethods are TelaView's NSAccessibility methods: the view
// stands for the root of the tree.
func accessViewMethods() []darwin.Method {
	return []darwin.Method{
		{Sel: darwin.Sel("accessibilityChildren"), Fn: func(self darwin.ID, _ darwin.SEL) darwin.ID {
			w := accessWindow(self)
			if w == nil {
				return darwin.NSArray()
			}
			w.activateAccess()
			v := w.acc.tree.Load()
			if v == nil {
				return darwin.NSArray()
			}
			return w.elements(v.kids[v.root()])
		}},
		{Sel: darwin.Sel("accessibilityFocusedUIElement"), Fn: func(self darwin.ID, cmd darwin.SEL) darwin.ID {
			if w := accessWindow(self); w != nil {
				w.activateAccess()
				if v := w.acc.tree.Load(); v != nil {
					if f := v.focus(); f != v.root() {
						return w.element(f)
					}
				}
			}
			return darwin.SendSuper(self, app.nsView, cmd)
		}},
		{Sel: asel.hitTest, Fn: func(self darwin.ID, cmd darwin.SEL, p darwin.CGPoint) darwin.ID {
			if w := accessWindow(self); w != nil {
				w.activateAccess()
				if v := w.acc.tree.Load(); v != nil {
					x, y := w.viewPoint(p)
					if id := v.hitTest(v.root(), x, y); id != 0 {
						return w.element(id)
					}
				}
			}
			return darwin.SendSuper(self, app.nsView, cmd, p)
		}},
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
			return darwin.SendSuper(self, axAccess.super, cmd, s)&0xff != 0
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
		obj("accessibilityParent", func(c axCall) darwin.ID { return c.w.element(c.v.up[c.n.ID]) }),
		obj("accessibilityChildren", func(c axCall) darwin.ID { return c.w.elements(c.v.kids[c.n.ID]) }),
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
			return lineForIndex(s, cpToU16(s, c.n.Selection.Focus))
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
		// action AXScrollToVisible of the older, name-based API.
		{Sel: asel.actionNames, Fn: func(self darwin.ID, cmd darwin.SEL) darwin.ID {
			names := darwin.SendSuper(self, axAccess.super, cmd)
			scroll := false
			axQuery(self, func(c axCall) { scroll = c.n.Actions.Has(platform.AccessActionScrollIntoView) })
			if !scroll {
				return names
			}
			out := darwin.NewMutableArray()
			for _, n := range darwin.ArrayItems(names) {
				darwin.ArrayAdd(out, n)
			}
			darwin.ArrayAdd(out, axStr(axScrollToVisible))
			return out
		}},
		{Sel: asel.performAction, Fn: func(self darwin.ID, cmd darwin.SEL, action darwin.ID) {
			if darwin.GoString(action) == axScrollToVisible {
				axAct(self, platform.AccessActionScrollIntoView, nil)
				return
			}
			darwin.SendSuper(self, axAccess.super, cmd, action)
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
