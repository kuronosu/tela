//go:build linux

package atspi

import (
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// Events: what changed between two snapshots of a window, as AT-SPI
// signals, sent by the worker after it published the new snapshot (an AT
// that asks on receipt sees the new tree). libatspi updates its cache of
// names, states and children from these events, so they say exactly what
// changed, in the order an AT can apply:
//
//   - object:children-changed:remove for each child no longer exposed
//     under a parent that stays (from the last), then
//     children-changed:add for each new one (from the first), with the
//     indices of the list as it is at that point; children that changed
//     order are removed and added again;
//   - object:property-change:accessible-name, -description, -role and
//     -value (range widgets);
//   - object:state-changed:<state> for each AT-SPI state that changed
//     (checked, expanded, selected, pressed, busy, enabled, invalid-entry,
//     showing...), focused last: off on the old focus, then on on the
//     new one;
//   - object:text-changed:delete and insert for a text that changed (the
//     smallest replaced span), object:text-caret-moved and
//     object:text-selection-changed for a field's caret and selection;
//   - object:active-descendant-changed when aria-activedescendant moves;
//   - object:bounds-changed, only when an AT registered for it (it would
//     follow every scroll);
//   - window:activate and deactivate, and the frame's active state, from
//     the window's focus; window:create and destroy and the
//     application's children-changed when windows come and go.
//
// Events are sent only when an AT listens for them (the registry's
// registered events), every one when the registry cannot tell.

// emit sends the AT-SPI event member of interface ifc on path.
func (b *Bridge) emit(path dbus.ObjectPath, ifc, member, detail string, d1, d2 int32, data dbus.Variant) {
	wk := &b.wk
	if wk.a11y == nil {
		return
	}
	cat := "object"
	if ifc == eventWindowIface {
		cat = "window"
	}
	ev := cat + ":" + strings.ToLower(member)
	if detail != "" {
		ev += ":" + normalizeEvent(detail)
	}
	if !wk.listeners.wants(ev) {
		return
	}
	wk.a11y.Emit(path, ifc+"."+member, detail, d1, d2, data, map[string]dbus.Variant{})
}

// boundsListened reports whether an AT asked for bounds-changed events
// explicitly (a listener for all events does not count).
func (b *Bridge) boundsListened() bool {
	l := &b.wk.listeners
	if !l.known {
		return false
	}
	for k := range l.evs {
		if k == "object:boundschanged" {
			return true
		}
	}
	return false
}

var noData = dbus.MakeVariant(int32(0))

// windowEvents reports the change of a window's own state.
func (b *Bridge) windowEvents(w *Window, old, cur *snapshot) {
	if old.title != cur.title && cur.root().n.Name == "" {
		b.emit(w.path, eventObjectIface, "PropertyChange", "accessible-name", 0, 0, dbus.MakeVariant(cur.title))
	}
	if old.active != cur.active {
		b.emit(w.path, eventObjectIface, "StateChanged", "active", boolInt(cur.active), 0, noData)
		member := "Deactivate"
		if cur.active {
			member = "Activate"
		}
		b.emit(w.path, eventWindowIface, member, "", 0, 0, dbus.MakeVariant(cur.title))
		// The focused element is focused again for the AT: Orca reads it
		// after the window's name.
		if cur.active {
			if f := cur.node(cur.view.tree.Focus()); f != nil && f != cur.root() {
				b.emit(w.objectPath(f), eventObjectIface, "StateChanged", "focused", 1, 0, noData)
			}
		}
	}
}

func boolInt(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// treeEvents reports the changes from old to cur of window w's tree.
func (b *Bridge) treeEvents(w *Window, old, cur *snapshot) {
	ov, nv := old.view, cur.view
	bounds := b.boundsListened()
	// Walk the new tree in order, so that parents report before children.
	var walk func(x *vnode)
	walk = func(x *vnode) {
		o := ov.nodes[x.n.ID]
		if x == nv.root {
			o = ov.root
		}
		if o != nil && (x == nv.root) == (o == ov.root) {
			b.nodeEvents(w, old, cur, o, x, bounds)
		}
		for _, id := range x.children {
			if c := nv.nodes[id]; c != nil {
				walk(c)
			}
		}
	}
	walk(nv.root)
	b.focusEvents(w, old, cur)
}

// nodeEvents reports the changes of node o (old) into x (new).
func (b *Bridge) nodeEvents(w *Window, old, cur *snapshot, o, x *vnode, bounds bool) {
	path := w.objectPath(x)
	b.childrenEvents(w, cur, path, o, x)
	on, nn := o.n, x.n
	if x != cur.root() {
		if o.name() != x.name() {
			b.emit(path, eventObjectIface, "PropertyChange", "accessible-name", 0, 0, dbus.MakeVariant(x.name()))
		}
		if on.Description != nn.Description {
			b.emit(path, eventObjectIface, "PropertyChange", "accessible-description", 0, 0, dbus.MakeVariant(nn.Description))
		}
		if o.role != x.role {
			b.emit(path, eventObjectIface, "PropertyChange", "accessible-role", 0, 0, dbus.MakeVariant(x.role))
		}
		if rangeWidget(nn) && (on.Range != nn.Range || on.Value != nn.Value) {
			v, _, _, _ := rangeValues(nn)
			b.emit(path, eventObjectIface, "PropertyChange", "accessible-value", 0, 0, dbus.MakeVariant(v))
		}
		os, ns := nodeStates(old, o), nodeStates(cur, x)
		for st := 1; st < stateLastDefined; st++ {
			if st == stateFocused || os.has(st) == ns.has(st) {
				continue
			}
			b.emit(path, eventObjectIface, "StateChanged", stateNames[st], boolInt(ns.has(st)), 0, noData)
		}
		if nn.ActiveDescendant != on.ActiveDescendant {
			if d := cur.node(nn.ActiveDescendant); d != nil {
				b.emit(path, eventObjectIface, "ActiveDescendantChanged", "", 0, 0, dbus.MakeVariant(b.ref(w, d)))
			}
		}
	}
	if o.hasText && x.hasText {
		b.textEvents(path, o, x)
	}
	if bounds && on.Bounds != nn.Bounds {
		ex, ey, ew, eh := cur.extents(nn.Bounds, coordScreen, nil)
		b.emit(path, eventObjectIface, "BoundsChanged", "", 0, 0, dbus.MakeVariant(extents{ex, ey, ew, eh}))
	}
}

// childrenEvents reports the change of x's exposed children (see the top
// of the file).
func (b *Bridge) childrenEvents(w *Window, cur *snapshot, path dbus.ObjectPath, o, x *vnode) {
	if slices.Equal(o.children, x.children) {
		return
	}
	in := func(list []platform.AccessID) map[platform.AccessID]bool {
		m := make(map[platform.AccessID]bool, len(list))
		for _, id := range list {
			m[id] = true
		}
		return m
	}
	now := in(x.children)
	list := slices.Clone(o.children)
	// Removed children, from the last.
	for i := len(list) - 1; i >= 0; i-- {
		if !now[list[i]] {
			b.emit(path, eventObjectIface, "ChildrenChanged", "remove", int32(i), 0,
				dbus.MakeVariant(ref{b.name(), dbus.ObjectPath(string(w.path) + "_" + itoa(list[i]))}))
			list = slices.Delete(list, i, i+1)
		}
	}
	// The children kept, in the new order: when it differs, they go and
	// come back.
	was := in(list)
	var kept []platform.AccessID
	for _, id := range x.children {
		if was[id] {
			kept = append(kept, id)
		}
	}
	if !slices.Equal(kept, list) {
		for i := len(list) - 1; i >= 0; i-- {
			b.emit(path, eventObjectIface, "ChildrenChanged", "remove", int32(i), 0,
				dbus.MakeVariant(ref{b.name(), dbus.ObjectPath(string(w.path) + "_" + itoa(list[i]))}))
		}
		list = nil
		was = nil
	}
	// Added children, from the first, at their final index.
	for i, id := range x.children {
		if was[id] {
			continue
		}
		b.emit(path, eventObjectIface, "ChildrenChanged", "add", int32(i), 0, dbus.MakeVariant(b.ref(w, cur.node(id))))
	}
}

func itoa(id platform.AccessID) string {
	const digits = "0123456789"
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = digits[id%10]
		id /= 10
	}
	return string(buf[i:])
}

// textEvents reports the change of a text: the replaced span, then a
// field's caret and selection.
func (b *Bridge) textEvents(path dbus.ObjectPath, o, x *vnode) {
	if o.text != x.text {
		or, nr := []rune(o.text), []rune(x.text)
		p := 0
		for p < len(or) && p < len(nr) && or[p] == nr[p] {
			p++
		}
		s := 0
		for s < len(or)-p && s < len(nr)-p && or[len(or)-1-s] == nr[len(nr)-1-s] {
			s++
		}
		if del := or[p : len(or)-s]; len(del) > 0 {
			b.emit(path, eventObjectIface, "TextChanged", "delete", int32(p), int32(len(del)), dbus.MakeVariant(string(del)))
		}
		if ins := nr[p : len(nr)-s]; len(ins) > 0 {
			b.emit(path, eventObjectIface, "TextChanged", "insert", int32(p), int32(len(ins)), dbus.MakeVariant(string(ins)))
		}
	}
	on, nn := o.n, x.n
	if !nn.HasSelection {
		return
	}
	if !on.HasSelection || on.Selection.Focus != nn.Selection.Focus {
		b.emit(path, eventObjectIface, "TextCaretMoved", "", int32(nn.Selection.Focus), 0, noData)
	}
	oldRange := on.HasSelection && on.Selection.Anchor != on.Selection.Focus
	newRange := nn.Selection.Anchor != nn.Selection.Focus
	if (oldRange || newRange) && on.Selection != nn.Selection {
		b.emit(path, eventObjectIface, "TextSelectionChanged", "", 0, 0, noData)
	}
}

// focusEvents reports a change of the focused node: focused off on the
// old one (if it is still there), on on the new one.
func (b *Bridge) focusEvents(w *Window, old, cur *snapshot) {
	of, nf := old.view.tree.Focus(), cur.view.tree.Focus()
	if of == nf && old.view.tree != nil {
		return
	}
	if of != nf {
		if x := cur.node(of); x != nil && x != cur.root() && old.node(of) != nil {
			b.emit(w.objectPath(x), eventObjectIface, "StateChanged", "focused", 0, 0, noData)
		}
	}
	if x := cur.node(nf); x != nil && x != cur.root() {
		b.emit(w.objectPath(x), eventObjectIface, "StateChanged", "focused", 1, 0, noData)
	}
}
