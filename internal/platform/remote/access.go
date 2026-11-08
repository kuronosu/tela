package remote

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
)

// Platform contract v5 (ADR-246), protocol 9.
//
// A child's Window.SetAccessibility is a message (msgSetAccessibility)
// that the host gives its real window while that window's bridge is
// active and the child is the window's active one. The bridge's
// AccessibilityEvents go to the active child window, which is told again
// at a promotion (Active true for the new build, which then sends its
// whole tree) and after its first ConfigureEvent when the bridge was
// already active; pending children are never told, so they build no
// tree. AccessActionEvents go to the active child window once it sent an
// update since its activation, and are dropped while input is frozen.
//
// Node IDs are per child: a reload's tree restarts from 1, and its IDs
// would name the outgoing build's nodes in the bridge. The host puts the
// child's serial number (its connection order, from 1) in the high bits
// of every ID it gives the real window (accessShift), so the bridge sees
// the new build's nodes as new ones and drops the old build's as
// unreachable, and an action on a node of another build is dropped.

// accessShift is where the child's serial starts in the IDs of the real
// windows' trees. Core numbers nodes from 1 per tree; 2^40 of them is out
// of reach.
const accessShift = 40

// maxAccessNodes bounds the nodes of one update and the children of one
// node (a corrupt count fails the message).
const maxAccessNodes = 1 << 22

// --- child side ---

// SetAccessibility forwards an accessibility update to the host's window.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
	if w.open() {
		w.send(newMsg(msgSetAccessibility), func(e *enc) { e.accessUpdate(&update) })
	}
}

// --- host side ---

// setAccessibility serves a child's msgSetAccessibility.
func (c *Child) setAccessibility(d *dec) error {
	id := d.u32()
	u := d.accessUpdate()
	if d.err == nil {
		if err := try(func() { platform.CheckAccessUpdate(&u) }); err != nil {
			return err
		}
		if err := checkAccessIDs(&u); err != nil {
			return err
		}
	}
	return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
		if !active || !rw.access || !cw.access || rw.closed {
			return // the bridge is not active, or not for this build: dropped
		}
		cw.accessSynced = true
		c.mapAccess(&u)
		rw.w.SetAccessibility(u)
	})
}

// checkAccessIDs fails an update with an ID that has bits at or above
// accessShift, which the host's mapping needs.
func checkAccessIDs(u *platform.AccessUpdate) error {
	const limit = platform.AccessID(1) << accessShift
	bad := u.Root >= limit || u.Focus >= limit
	for i := range u.Nodes {
		n := &u.Nodes[i]
		bad = bad || n.ID >= limit || n.LiveRoot >= limit || n.Controls >= limit || n.ActiveDescendant >= limit
		for _, ch := range n.Children {
			bad = bad || ch >= limit
		}
	}
	if bad {
		return fmt.Errorf("remote: an accessibility node ID of 2^%d or more", accessShift)
	}
	return nil
}

// mapAccess puts c's serial into every ID of u (see the top of
// access.go); 0 stays 0.
func (c *Child) mapAccess(u *platform.AccessUpdate) {
	m := func(id platform.AccessID) platform.AccessID {
		if id == 0 {
			return 0
		}
		return platform.AccessID(c.serial<<accessShift) | id
	}
	u.Root, u.Focus = m(u.Root), m(u.Focus)
	for i := range u.Nodes {
		n := &u.Nodes[i]
		n.ID, n.LiveRoot, n.Controls, n.ActiveDescendant = m(n.ID), m(n.LiveRoot), m(n.Controls), m(n.ActiveDescendant)
		for j, ch := range n.Children {
			n.Children[j] = m(ch)
		}
	}
}

// childAction maps an action of the real window's bridge back to cw's
// node IDs; false drops it: a node of another build, or a child that has
// not sent its tree since it was told the bridge is active.
func (c *Child) childAction(cw *childWin, e platform.AccessActionEvent) (platform.Event, bool) {
	if !cw.access || !cw.accessSynced || uint64(e.Node)>>accessShift != c.serial {
		return nil, false
	}
	e.Node &= 1<<accessShift - 1
	return e, true
}

// accessEvent routes an AccessibilityEvent or AccessActionEvent of the
// real window rw.
func (h *Host) accessEvent(rw *realWin, e platform.Event) {
	switch ev := e.(type) {
	case platform.AccessibilityEvent:
		rw.access = ev.Active
		if rw.active != nil {
			h.syncAccess(rw, rw.active)
		}
	case platform.AccessActionEvent:
		// While input is frozen a swap may be under way: the AT's node is
		// of the shown build, which may be detached before the action
		// would be delivered. The AT gets no outcome and may ask again.
		if !h.frozen && rw.active != nil {
			h.deliver(rw.active, ev)
		}
	}
}

// syncAccess tells cw, rw's active child window, whether rw's bridge is
// active, if it has not been told (deliver drops it before cw's first
// ConfigureEvent).
func (h *Host) syncAccess(rw *realWin, cw *childWin) {
	if rw.active == cw && cw.access != rw.access {
		h.deliver(cw, platform.AccessibilityEvent{Active: rw.access})
	}
}

// --- encoding ---

func (e *enc) accessUpdate(u *platform.AccessUpdate) {
	e.u64(uint64(u.Root))
	e.u64(uint64(u.Focus))
	e.u32(uint32(len(u.Nodes)))
	for i := range u.Nodes {
		e.accessNode(&u.Nodes[i])
	}
	e.u32(uint32(len(u.Announcements)))
	for _, a := range u.Announcements {
		e.str(a.Text)
		e.boolean(a.Assertive)
	}
}

func (e *enc) accessNode(n *platform.AccessNode) {
	e.u64(uint64(n.ID))
	e.u8(uint8(n.Kind))
	for _, s := range [...]string{n.Role, n.Name, n.Description, n.Value, n.Text, n.URL, n.Placeholder, n.KeyShortcuts,
		n.Sort, n.Current, n.HasPopup, n.Orientation, n.AutoComplete, n.Live} {
		e.str(s)
	}
	r := n.Range
	e.f64(r.Now)
	e.f64(r.Min)
	e.f64(r.Max)
	e.boolean(r.HasNow)
	e.boolean(r.HasMin)
	e.boolean(r.HasMax)
	e.u32(uint32(n.States))
	for _, t := range [...]platform.AccessToggle{n.Checked, n.Pressed, n.Selected, n.Expanded} {
		e.u8(uint8(t))
	}
	e.u16(uint16(n.Actions))
	for _, v := range [...]int{n.Level, n.PosInSet, n.SetSize, n.RowCount, n.RowIndex, n.Selection.Anchor, n.Selection.Focus} {
		e.i64(int64(v))
	}
	e.boolean(n.HasSelection)
	for _, id := range [...]platform.AccessID{n.LiveRoot, n.Controls, n.ActiveDescendant} {
		e.u64(uint64(id))
	}
	e.rect(n.Bounds)
	e.u32(uint32(len(n.Children)))
	for _, c := range n.Children {
		e.u64(uint64(c))
	}
}

// accessCount reads a list length of at most maxAccessNodes items of at
// least size bytes each, failing when the message cannot hold them.
func (d *dec) accessCount(size int, what string) int {
	n := d.count(maxAccessNodes, what)
	if d.err == nil && int64(n)*int64(size) > int64(len(d.b)) {
		d.err = errShort
		return 0
	}
	return n
}

func (d *dec) accessUpdate() platform.AccessUpdate {
	u := platform.AccessUpdate{Root: platform.AccessID(d.u64()), Focus: platform.AccessID(d.u64())}
	if n := d.accessCount(64, "accessibility nodes"); n > 0 {
		u.Nodes = make([]platform.AccessNode, n)
		for i := range u.Nodes {
			d.accessNode(&u.Nodes[i])
			if d.err != nil {
				return u
			}
		}
	}
	if n := d.accessCount(5, "announcements"); n > 0 {
		u.Announcements = make([]platform.Announcement, n)
		for i := range u.Announcements {
			u.Announcements[i] = platform.Announcement{Text: d.str(), Assertive: d.boolean()}
		}
	}
	return u
}

func (d *dec) accessNode(n *platform.AccessNode) {
	n.ID = platform.AccessID(d.u64())
	n.Kind = platform.AccessKind(d.u8())
	for _, s := range [...]*string{&n.Role, &n.Name, &n.Description, &n.Value, &n.Text, &n.URL, &n.Placeholder, &n.KeyShortcuts,
		&n.Sort, &n.Current, &n.HasPopup, &n.Orientation, &n.AutoComplete, &n.Live} {
		*s = d.str()
	}
	n.Range = platform.AccessRange{Now: d.f64(), Min: d.f64(), Max: d.f64(), HasNow: d.boolean(), HasMin: d.boolean(), HasMax: d.boolean()}
	n.States = platform.AccessStates(d.u32())
	for _, t := range [...]*platform.AccessToggle{&n.Checked, &n.Pressed, &n.Selected, &n.Expanded} {
		*t = platform.AccessToggle(d.u8())
	}
	n.Actions = platform.AccessActions(d.u16())
	for _, v := range [...]*int{&n.Level, &n.PosInSet, &n.SetSize, &n.RowCount, &n.RowIndex, &n.Selection.Anchor, &n.Selection.Focus} {
		*v = int(d.i64())
	}
	n.HasSelection = d.boolean()
	for _, id := range [...]*platform.AccessID{&n.LiveRoot, &n.Controls, &n.ActiveDescendant} {
		*id = platform.AccessID(d.u64())
	}
	n.Bounds = d.rect()
	if c := d.accessCount(8, "accessibility children"); c > 0 {
		n.Children = make([]platform.AccessID, c)
		for i := range n.Children {
			n.Children[i] = platform.AccessID(d.u64())
		}
	}
}
