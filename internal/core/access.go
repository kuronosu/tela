package core

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

// OS accessibility (platform contract v5, ADR-246): what core gives the
// accessibility bridge of each window.
//
// # When it runs
//
// Only while the window's bridge is active, between the
// platform.AccessibilityEvents with Active true and false: until then a
// window has no accessView and nothing of this file runs (Host.syncAccess
// checks one pointer per window after each event; 0% idle CPU, no
// allocation). After every event the Host handles (HandleEvent), each
// window with an active bridge whose tree is clean (no frame or commit
// pending: the work the event left is pushed after the frame that does
// it) and whose semantics tree is stale (a Frame ran, an AriaLabelFn's
// Value changed, or nothing was sent since the activation) builds its
// semantics tree (Tree.Semantics), converts it, and sends the bridge
// what changed. A change that needs no frame (an AriaLabelFn's Value)
// wakes the loop, so it is sent at the end of that loop turn; several
// changes in one turn make one update.
//
// # What is sent
//
// The nodes of the semantics tree as platform.AccessNodes, IDs kept (an
// element's node ID for its life): the root, every element and every
// text that is not hidden (StateHidden: display none, AriaHidden, inert,
// outside the active modal overlay, visibility hidden); a hidden node is
// left out with its hidden descendants, and the descendants that are not
// hidden (visibility visible inside visibility hidden) become children of
// its nearest sent ancestor. Generic containers, none and presentation
// are sent too, as AccessKit does: bridges leave out what
// platform.AccessNode.Ignored reports, so every platform filters alike
// and IDs and bounds stay available. The first update after an
// activation holds every node; later ones the nodes that differ from the
// ones last sent (field by field, children lists included), so a checkbox
// click sends the checkbox, and a new child also its parent. A removed
// node is not reachable from the root anymore. An update goes out only
// when some node, the root or the focus changed, or there is something to
// announce. The focus is the node with StateFocused, else the root.
//
// # Announcements
//
// A live region is a node with Live set (aria-live, or the roles alert,
// status and log); its content is the texts (and the names of images)
// whose LiveRoot is the region, so a nested region has its own. Between
// two updates core compares each region's content, item by item (a text
// node or image by ID):
//
//   - an atomic region (AriaAtomic, or alert and status) whose content
//     changed (an item added or its text changed; a removal alone
//     announces nothing, as aria-relevant's default "additions text") is
//     announced whole: the text of its whole subtree, nested regions
//     included;
//   - another region announces the text of its added and changed items,
//     in tree order;
//   - assertive regions are announced assertive, polite ones polite;
//   - a region that appears after the activation is compared with an
//     empty one, so a region inserted already filled (a toast, an
//     inserted alert) is announced, as browsers announce an inserted
//     alert; one present at the activation is not;
//   - nothing is announced in the activation's update, and a region that
//     is busy (AriaBusy) announces nothing until it is not: it is then
//     compared with its content before it became busy;
//   - empty texts are not announced; white space is collapsed, and block
//     boundaries between items are word breaks.
//
// # Actions
//
// A platform.AccessActionEvent runs the matching action of the node
// (semantics.go: ClickNode, FocusNode, FillNode for set value,
// IncrementNode, DecrementNode, ExpandNode, CollapseNode,
// ScrollNodeIntoView, ScrollNodeBy, SetTextSelectionNode, ShowMenuNode),
// then the updates are applied as after a discrete event (input); the
// outcome reaches the bridge as the next update. An action on a node that
// is gone or cannot do it is logged (Config.Log: dev mode) and ignored,
// and so is an action on a window blocked by a modal child window.
// Child windows have their own trees and bridges and work the same.

// accessView is the accessibility state of a window whose bridge is
// active (winView.acc; nil otherwise). UI thread.
//
// The conversion is incremental (ADR-277): the semantics tree shares the
// subtrees that did not change with the last one (semincr.go), so a node
// whose pointer is the one last converted stands for a subtree that is
// sent already; only the others are converted and compared, the nodes
// that left the tree are found from the children lists that dropped
// them, and only the live regions whose subtree changed are compared.
type accessView struct {
	// first: nothing was sent since the activation.
	first bool
	// sem is the semantics tree last converted (nil before the first).
	sem *semantics.Node
	// sent are the nodes last sent, by ID (with root and focus).
	sent        map[platform.AccessID]platform.AccessNode
	root, focus platform.AccessID
	// conv holds the semantics node last converted for each node ID, sent
	// or hidden.
	conv map[semantics.NodeID]*semantics.Node
	// regions holds the content of each live region as last compared:
	// its items' texts by ID.
	regions map[platform.AccessID]map[platform.AccessID]string
	// cv is the scratch state of a conversion.
	cv accessConv
}

// accessConv is the scratch state of one conversion.
type accessConv struct {
	// nodes are the nodes converted, in tree order; regions the live
	// region roots among them (index in nodes).
	nodes   []platform.AccessNode
	regions []int
	// focus is the converted node with the focused state; lost reports
	// that the node last focused was converted without it.
	focus platform.AccessID
	lost  bool
	// placed holds the IDs put in a converted node's children, dropped
	// the children the converted nodes had and may have lost.
	placed  map[platform.AccessID]struct{}
	dropped []platform.AccessID
}

// accessItem is a text, or an image's name, inside a live region.
type accessItem struct {
	region, id platform.AccessID
	text       string
}

// setAccessActive starts (active) or stops the accessibility work of
// window v, on its platform.AccessibilityEvent.
func (h *Host) setAccessActive(v *winView, active bool) {
	if active == (v.acc != nil) {
		return
	}
	if !active {
		v.acc = nil
		v.tree.OnSemanticsStale(nil)
		return
	}
	v.acc = &accessView{first: true}
	// A change without a frame (an AriaLabelFn's Value) wakes the loop:
	// the update goes out at the end of the turn that handles the wake
	// (syncAccess), with whatever else changed in it.
	v.tree.OnSemanticsStale(h.backend.Wake)
}

// syncAccess sends the bridge of every window with an active one what
// changed (see the top of access.go). It runs after each event.
func (h *Host) syncAccess() {
	if h.ended {
		return
	}
	for i := 0; i < len(h.views); i++ {
		if v := h.views[i]; v.acc != nil && !v.dead.Load() {
			h.pushAccess(v)
		}
	}
}

// pushAccess sends window v's bridge the update its tree needs, if any.
func (h *Host) pushAccess(v *winView) {
	t, a := v.tree, v.acc
	if t.Dirty() {
		return // a frame or a commit follows; the update goes after it
	}
	if !a.first && !t.SemanticsStale() && t.sem.root == a.sem {
		return
	}
	root := t.Semantics()
	if root == a.sem || root.ID == 0 {
		return // unchanged, or nothing laid out yet (no frame since the mount)
	}
	a.sem = root
	u, ok := a.update(root)
	if ok {
		v.win.SetAccessibility(u)
	}
}

// update converts root and returns the update from the nodes last sent,
// and whether there is one to send (see accessView).
func (a *accessView) update(root *semantics.Node) (platform.AccessUpdate, bool) {
	var prev *accessView
	if checkSemantics {
		prev = a.clone()
	}
	u, send := a.convert(root)
	if prev != nil {
		checkAccessUpdate(prev, root, a, u, send)
	}
	return u, send
}

// convert is update without the check.
func (a *accessView) convert(root *semantics.Node) (platform.AccessUpdate, bool) {
	c := &a.cv
	c.nodes, c.regions, c.focus, c.lost, c.dropped = c.nodes[:0], c.regions[:0], 0, false, c.dropped[:0]
	if c.placed == nil {
		c.placed = map[platform.AccessID]struct{}{}
	}
	clear(c.placed)
	if a.sent == nil {
		a.sent = map[platform.AccessID]platform.AccessNode{}
		a.conv = map[semantics.NodeID]*semantics.Node{}
	}
	a.add(root, nil)
	u := platform.AccessUpdate{Root: platform.AccessID(root.ID)}
	for i := range c.nodes {
		n := &c.nodes[i]
		old, ok := a.sent[n.ID]
		if ok {
			for _, id := range old.Children {
				if _, kept := c.placed[id]; !kept {
					c.dropped = append(c.dropped, id)
				}
			}
		}
		if !ok || !old.Equal(n) {
			u.Nodes = append(u.Nodes, *n)
		}
		a.sent[n.ID] = *n
	}
	for _, id := range c.dropped {
		a.drop(id)
	}
	if len(a.conv) > 2*len(a.sent)+1024 {
		a.sweep(root)
	}
	u.Focus = a.focus
	switch {
	case c.focus != 0:
		u.Focus = c.focus
	case a.first || c.lost:
		u.Focus = 0
	}
	if _, ok := a.sent[u.Focus]; !ok {
		u.Focus = 0
	}
	if u.Focus == 0 {
		u.Focus = u.Root
	}
	u.Announcements = a.announce()
	send := a.first || len(u.Nodes) > 0 || u.Root != a.root || u.Focus != a.focus || len(u.Announcements) > 0
	a.first = false
	a.root, a.focus = u.Root, u.Focus
	return u, send
}

// add converts n and its descendants, unless n is the node last
// converted for its ID (its subtree is sent already), appending the ID of
// each node it sends to *children (nil for the root).
func (a *accessView) add(n *semantics.Node, children *[]platform.AccessID) {
	c := &a.cv
	id := platform.AccessID(n.ID)
	same := !a.first && a.conv[n.ID] == n
	a.conv[n.ID] = n
	if n.Kind != semantics.KindRoot && n.Has(semantics.StateHidden) {
		if !same {
			// Not sent: a hidden region is no region, a hidden focus no
			// focus.
			delete(a.regions, id)
			if id == a.focus {
				c.lost = true
			}
		}
		for _, ch := range n.Children {
			a.add(ch, children)
		}
		return
	}
	if children != nil {
		*children = append(*children, id)
		c.placed[id] = struct{}{}
	}
	if same {
		return
	}
	i := len(c.nodes)
	c.nodes = append(c.nodes, accessNode(n))
	switch {
	case n.Has(semantics.StateFocused):
		c.focus = id
	case id == a.focus:
		c.lost = true
	}
	if n.Live != "" && n.Kind == semantics.KindElement {
		c.regions = append(c.regions, i)
	} else {
		delete(a.regions, id)
	}
	var kids []platform.AccessID
	if len(n.Children) > 0 {
		kids = make([]platform.AccessID, 0, len(n.Children))
		for _, ch := range n.Children {
			a.add(ch, &kids)
		}
		if len(kids) == 0 {
			kids = nil
		}
	}
	c.nodes[i].Children = kids
}

// drop forgets node id, which a converted node lost from its children,
// with its descendants, unless the conversion put it elsewhere (a move).
func (a *accessView) drop(id platform.AccessID) {
	if _, moved := a.cv.placed[id]; moved {
		return
	}
	n, ok := a.sent[id]
	if !ok {
		return
	}
	delete(a.sent, id)
	delete(a.conv, semantics.NodeID(id))
	delete(a.regions, id)
	for _, c := range n.Children {
		a.drop(c)
	}
}

// sweep forgets the converted nodes that are no longer in the tree rooted
// at root (hidden nodes that left it), rarely.
func (a *accessView) sweep(root *semantics.Node) {
	keep := make(map[semantics.NodeID]*semantics.Node, len(a.sent))
	root.Walk(func(n *semantics.Node) bool {
		keep[n.ID] = n
		return true
	})
	a.conv = keep
}

// clone returns a copy of a that shares no map with it, for the check.
func (a *accessView) clone() *accessView {
	c := &accessView{first: a.first, root: a.root, focus: a.focus,
		sent: make(map[platform.AccessID]platform.AccessNode, len(a.sent))}
	for id, n := range a.sent {
		c.sent[id] = n
	}
	if a.regions != nil {
		c.regions = make(map[platform.AccessID]map[platform.AccessID]string, len(a.regions))
		for id, m := range a.regions {
			c.regions[id] = maps.Clone(m)
		}
	}
	return c
}

// accessNode returns the AccessNode of n, without its children.
func accessNode(n *semantics.Node) platform.AccessNode {
	an := platform.AccessNode{
		ID:           platform.AccessID(n.ID),
		Role:         n.Role,
		Name:         n.Name,
		Description:  n.Description,
		Value:        n.Value,
		Text:         n.Text,
		URL:          n.URL,
		Placeholder:  n.Placeholder,
		KeyShortcuts: n.KeyShortcuts,
		Range: platform.AccessRange{Now: n.Range.Now, Min: n.Range.Min, Max: n.Range.Max,
			HasNow: n.Range.HasNow, HasMin: n.Range.HasMin, HasMax: n.Range.HasMax},
		Checked:          platform.AccessToggle(n.Checked),
		Pressed:          platform.AccessToggle(n.Pressed),
		Selected:         platform.AccessToggle(n.Selected),
		Expanded:         platform.AccessToggle(n.Expanded),
		Level:            n.Level,
		PosInSet:         n.PosInSet,
		SetSize:          n.SetSize,
		RowCount:         n.RowCount,
		RowIndex:         n.RowIndex,
		Sort:             n.Sort,
		Current:          n.Current,
		HasPopup:         n.HasPopup,
		Orientation:      n.Orientation,
		AutoComplete:     n.AutoComplete,
		Live:             n.Live,
		LiveRoot:         platform.AccessID(n.LiveRoot),
		Controls:         platform.AccessID(n.Controls),
		ActiveDescendant: platform.AccessID(n.ActiveDescendant),
		Bounds:           accessRect(n.Bounds),
	}
	switch n.Kind {
	case semantics.KindRoot:
		an.Kind, an.Role = platform.AccessRoot, ""
	case semantics.KindText:
		an.Kind, an.Role = platform.AccessText, ""
	default:
		if an.Role == "" {
			an.Role = "generic"
		}
	}
	if s := n.Selection; s != nil {
		an.Selection, an.HasSelection = platform.AccessSelection{Anchor: s.Anchor, Focus: s.Focus}, true
	}
	an.States = accessStates(n)
	an.Actions = accessActions(n.Actions)
	return an
}

// accessStateMap maps the semantics states onto the platform's.
var accessStateMap = [...]struct {
	s semantics.State
	a platform.AccessStates
}{
	{semantics.StateFocused, platform.AccessFocused},
	{semantics.StateFocusable, platform.AccessFocusable},
	{semantics.StateDisabled, platform.AccessDisabled},
	{semantics.StateScrollable, platform.AccessScrollable},
	{semantics.StateMultiline, platform.AccessMultiline},
	{semantics.StateReadOnly, platform.AccessReadOnly},
	{semantics.StateRequired, platform.AccessRequired},
	{semantics.StateBusy, platform.AccessBusy},
	{semantics.StateInvalid, platform.AccessInvalid},
	{semantics.StateProtected, platform.AccessProtected},
}

func accessStates(n *semantics.Node) platform.AccessStates {
	var out platform.AccessStates
	for _, m := range accessStateMap {
		if n.States&m.s != 0 {
			out |= m.a
		}
	}
	if n.Modal {
		out |= platform.AccessModal
	}
	if n.MultiSelectable {
		out |= platform.AccessMultiSelectable
	}
	if n.Atomic && n.Live != "" {
		out |= platform.AccessAtomic
	}
	if n.Inline {
		out |= platform.AccessInline
	}
	return out
}

// accessActionMap maps the semantics actions onto the platform's.
var accessActionMap = [...]struct {
	s semantics.Action
	a platform.AccessAction
}{
	{semantics.ActionClick, platform.AccessActionDefault},
	{semantics.ActionFocus, platform.AccessActionFocus},
	{semantics.ActionScrollIntoView, platform.AccessActionScrollIntoView},
	{semantics.ActionScroll, platform.AccessActionScrollBy},
	{semantics.ActionSetValue, platform.AccessActionSetValue},
	{semantics.ActionIncrement, platform.AccessActionIncrement},
	{semantics.ActionDecrement, platform.AccessActionDecrement},
	{semantics.ActionExpand, platform.AccessActionExpand},
	{semantics.ActionCollapse, platform.AccessActionCollapse},
	{semantics.ActionShowMenu, platform.AccessActionShowMenu},
	{semantics.ActionSetSelection, platform.AccessActionSetSelection},
}

func accessActions(s semantics.Action) platform.AccessActions {
	var out platform.AccessActions
	for _, m := range accessActionMap {
		if s&m.s != 0 {
			out |= platform.AccessActionsOf(m.a)
		}
	}
	return out
}

// accessRect returns r as a platform rectangle: finite, with no negative
// size.
func accessRect(r gfx.Rect) platform.Rect {
	fin := func(v float32) float32 {
		if v != v || math.IsInf(float64(v), 0) {
			return 0
		}
		return v
	}
	x0, y0, x1, y1 := fin(r.X0), fin(r.Y0), fin(r.X1), fin(r.Y1)
	return platform.Rect{X: x0, Y: y0, W: max(x1-x0, 0), H: max(y1-y0, 0)}
}

// --- announcements ---

// announce compares the live regions converted (whose subtree changed)
// with their content last compared and returns the announcements (see the
// top of access.go), updating the regions' content. The other regions'
// content did not change.
func (a *accessView) announce() []platform.Announcement {
	c := &a.cv
	var out []platform.Announcement
	for _, i := range c.regions {
		r := &c.nodes[i]
		cur := regionContent(a.sent, r.ID)
		prev, known := a.regions[r.ID]
		if a.regions == nil {
			a.regions = map[platform.AccessID]map[platform.AccessID]string{}
		}
		if a.first {
			a.regions[r.ID] = cur
			continue // the activation's update announces nothing
		}
		if r.Has(platform.AccessBusy) {
			// A busy region keeps the content it had (none if it is new)
			// until it is not.
			if !known {
				prev = map[platform.AccessID]string{}
			}
			a.regions[r.ID] = prev
			continue
		}
		a.regions[r.ID] = cur
		if text, ok := regionNews(a.sent, r, prev, cur); ok {
			out = append(out, platform.Announcement{Text: text, Assertive: r.Live == "assertive"})
		}
	}
	return out
}

// regionContent returns the items of live region id in nodes: the texts
// and image names whose LiveRoot is the region, by ID.
func regionContent(nodes map[platform.AccessID]platform.AccessNode, id platform.AccessID) map[platform.AccessID]string {
	m := map[platform.AccessID]string{}
	var walk func(id platform.AccessID)
	walk = func(x platform.AccessID) {
		n, ok := nodes[x]
		if !ok {
			return
		}
		if n.LiveRoot == id {
			switch {
			case n.Kind == platform.AccessText:
				m[x] = n.Text
			case n.Role == "img" && n.Name != "":
				m[x] = n.Name
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(id)
	return m
}

// regionNews returns the text to announce for region r, whose content
// went from prev to cur, and whether there is one.
func regionNews(nodes map[platform.AccessID]platform.AccessNode, r *platform.AccessNode, prev, cur map[platform.AccessID]string) (string, bool) {
	changed := func(id platform.AccessID, text string) bool {
		old, ok := prev[id]
		return !ok || old != text
	}
	news := false
	for id, text := range cur {
		if changed(id, text) && semantics.TrimSpace(text) != "" {
			news = true
			break
		}
	}
	if !news {
		return "", false
	}
	var text string
	if r.Has(platform.AccessAtomic) {
		text = regionText(nodes, r.ID, func(*platform.AccessNode, string) bool { return true })
	} else {
		text = regionText(nodes, r.ID, func(n *platform.AccessNode, t string) bool {
			return n.LiveRoot == r.ID && changed(n.ID, t)
		})
	}
	return text, text != ""
}

// --- check (TELA_CHECK_SEMANTICS) ---

// checkAccessUpdate panics when the incremental update u (send) that
// turned prev into cur differs from what a full conversion of root
// gives: the nodes sent, the update's nodes, root, focus and
// announcements.
func checkAccessUpdate(prev *accessView, root *semantics.Node, cur *accessView, u platform.AccessUpdate, send bool) {
	fu, fsend, fsent, fregions := accessFull(prev, root)
	fail := func(format string, args ...any) {
		panic(fmt.Sprintf("core: TELA_CHECK_SEMANTICS: the incremental accessibility update differs from a full conversion: "+format, args...))
	}
	if len(fsent) != len(cur.sent) {
		fail("%d nodes sent, full %d", len(cur.sent), len(fsent))
	}
	for id, n := range fsent {
		if m, ok := cur.sent[id]; !ok || !m.Equal(&n) {
			fail("node %d: %+v, full %+v", id, m, n)
		}
	}
	if send != fsend || u.Root != fu.Root || u.Focus != fu.Focus {
		fail("send %v root %d focus %d, full send %v root %d focus %d", send, u.Root, u.Focus, fsend, fu.Root, fu.Focus)
	}
	if len(u.Nodes) != len(fu.Nodes) {
		fail("%d nodes in the update, full %d", len(u.Nodes), len(fu.Nodes))
	}
	for i := range u.Nodes {
		if !u.Nodes[i].Equal(&fu.Nodes[i]) {
			fail("node %d of the update: %+v, full %+v", i, u.Nodes[i], fu.Nodes[i])
		}
	}
	if !slices.Equal(u.Announcements, fu.Announcements) {
		fail("announcements %q, full %q", u.Announcements, fu.Announcements)
	}
	if len(fregions) != len(cur.regions) {
		fail("%d live regions, full %d", len(cur.regions), len(fregions))
	}
	for id, m := range fregions {
		if !maps.Equal(m, cur.regions[id]) {
			fail("live region %d: %q, full %q", id, cur.regions[id], m)
		}
	}
}

// accessFull converts root in full from the state prev (ADR-246's
// conversion, before ADR-277): the update, whether to send it, the nodes
// sent and the regions' content after it.
func accessFull(prev *accessView, root *semantics.Node) (platform.AccessUpdate, bool, map[platform.AccessID]platform.AccessNode, map[platform.AccessID]map[platform.AccessID]string) {
	c := &fullConv{}
	c.add(root, nil)
	u := platform.AccessUpdate{Root: platform.AccessID(root.ID), Focus: c.focus}
	if u.Focus == 0 {
		u.Focus = u.Root
	}
	next := make(map[platform.AccessID]platform.AccessNode, len(c.nodes))
	for i := range c.nodes {
		n := &c.nodes[i]
		if old, ok := prev.sent[n.ID]; !ok || !old.Equal(n) {
			u.Nodes = append(u.Nodes, *n)
		}
		next[n.ID] = *n
	}
	// The items of each region, by ID.
	content := make(map[platform.AccessID]map[platform.AccessID]string, len(c.regions))
	for _, i := range c.regions {
		content[c.nodes[i].ID] = map[platform.AccessID]string{}
	}
	for _, it := range c.items {
		if m := content[it.region]; m != nil {
			m[it.id] = it.text
		}
	}
	for _, i := range c.regions {
		r := &c.nodes[i]
		cur := content[r.ID]
		prevC, known := prev.regions[r.ID]
		if prev.first {
			continue
		}
		if r.Has(platform.AccessBusy) {
			if !known {
				prevC = map[platform.AccessID]string{}
			}
			content[r.ID] = prevC
			continue
		}
		if text, ok := regionNews(next, r, prevC, cur); ok {
			u.Announcements = append(u.Announcements, platform.Announcement{Text: text, Assertive: r.Live == "assertive"})
		}
	}
	send := prev.first || len(u.Nodes) > 0 || u.Root != prev.root || u.Focus != prev.focus || len(u.Announcements) > 0
	return u, send, next, content
}

// fullConv converts a whole semantics tree (accessFull).
type fullConv struct {
	nodes   []platform.AccessNode
	items   []accessItem
	regions []int
	focus   platform.AccessID
}

// add converts n and its descendants, appending the ID of each node it
// sends to *children (nil for the root).
func (c *fullConv) add(n *semantics.Node, children *[]platform.AccessID) {
	if n.Kind != semantics.KindRoot && n.Has(semantics.StateHidden) {
		for _, ch := range n.Children {
			c.add(ch, children)
		}
		return
	}
	id := platform.AccessID(n.ID)
	if children != nil {
		*children = append(*children, id)
	}
	i := len(c.nodes)
	c.nodes = append(c.nodes, accessNode(n))
	if n.Has(semantics.StateFocused) {
		c.focus = id
	}
	if n.Live != "" && n.Kind == semantics.KindElement {
		c.regions = append(c.regions, i)
	}
	if n.LiveRoot != 0 {
		switch {
		case n.Kind == semantics.KindText:
			c.items = append(c.items, accessItem{region: platform.AccessID(n.LiveRoot), id: id, text: n.Text})
		case n.Role == "img" && n.Name != "":
			c.items = append(c.items, accessItem{region: platform.AccessID(n.LiveRoot), id: id, text: n.Name})
		}
	}
	var kids []platform.AccessID
	if len(n.Children) > 0 {
		kids = make([]platform.AccessID, 0, len(n.Children))
		for _, ch := range n.Children {
			c.add(ch, &kids)
		}
		if len(kids) == 0 {
			kids = nil
		}
	}
	c.nodes[i].Children = kids
}

// regionText returns the text of the items under node id (texts, and the
// names of images) that keep accepts, in tree order, with white space
// collapsed and a word break at every block boundary and wherever an
// item left out stood between two kept ones.
func regionText(nodes map[platform.AccessID]platform.AccessNode, id platform.AccessID, keep func(n *platform.AccessNode, text string) bool) string {
	var b strings.Builder
	gap := false
	var walk func(id platform.AccessID)
	walk = func(id platform.AccessID) {
		n, ok := nodes[id]
		if !ok {
			return
		}
		block := n.Kind == platform.AccessElement && !n.Has(platform.AccessInline)
		if block {
			gap = true
		}
		text, item := "", false
		switch {
		case n.Kind == platform.AccessText:
			text, item = n.Text, true
		case n.Role == "img" && n.Name != "":
			text, item = n.Name, true
		}
		if item {
			if keep(&n, text) {
				if gap && b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(text)
				gap = false
			} else {
				gap = true
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
		if block {
			gap = true
		}
	}
	walk(id)
	return semantics.NormalizeWhiteSpace(b.String())
}

// accessAction performs an assistive technology's request on window v
// (see the top of access.go).
func (h *Host) accessAction(v *winView, e platform.AccessActionEvent) {
	if v.acc == nil {
		return // the bridge stopped: the node IDs name nothing sent
	}
	t := v.tree
	id := semantics.NodeID(e.Node)
	var err error
	switch e.Action {
	case platform.AccessActionDefault:
		err = t.ClickNode(id)
	case platform.AccessActionFocus:
		err = t.FocusNode(id)
	case platform.AccessActionSetValue:
		err = t.FillNode(id, e.Value)
	case platform.AccessActionIncrement:
		err = t.IncrementNode(id)
	case platform.AccessActionDecrement:
		err = t.DecrementNode(id)
	case platform.AccessActionExpand:
		err = t.ExpandNode(id)
	case platform.AccessActionCollapse:
		err = t.CollapseNode(id)
	case platform.AccessActionScrollIntoView:
		err = t.ScrollNodeIntoView(id)
	case platform.AccessActionScrollBy:
		err = t.ScrollNodeBy(id, e.DX, e.DY)
	case platform.AccessActionSetSelection:
		err = t.SetTextSelectionNode(id, e.Selection.Anchor, e.Selection.Focus)
	case platform.AccessActionShowMenu:
		err = t.ShowMenuNode(id)
	default:
		return
	}
	t.flushDiscrete()
	if err != nil {
		t.logf("tela: accessibility action %v on node %d: %v", e.Action, e.Node, err)
	}
}
