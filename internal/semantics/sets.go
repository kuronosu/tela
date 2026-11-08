package semantics

// Positions in sets (aria-posinset and aria-setsize), computed as
// Chromium's accessibility tree does (ui/accessibility/ax_node.cc,
// GetPosInSet and GetSetSize over the ordered set items), simplified.
//
// A set is a container of one of the set roles (listbox, menu, menubar,
// tablist, radiogroup, tree, list, directory, group) and its items are the
// descendants of the matching item roles (setItems) that are not hidden,
// found through every other node but not inside an item or a nested set
// container: an option of a listbox, the menu items of a menu or menubar
// (menuitem, menuitemcheckbox and menuitemradio count together), a tab of
// a tablist, a radio of a radiogroup, a tree item of a tree, a list item of
// a list or directory, and the options, menu items, tree items and list
// items of a group (a group of options in a listbox, of radios in a menu,
// the children of a tree item, are sets of their own). Rows are not: a
// table numbers them with AriaRowIndex.
//
// An item's position is its AriaPosInSet when given, else one more than
// the previous item's (1 for the first); the set's size is the
// container's AriaSetSize when given (-1: unknown), else the number of
// items or the largest position if that is larger; an item's own
// AriaSetSize wins. Items that are not in a set container (an option in
// a plain Div) get none. Chromium also numbers radios without a
// radiogroup by their name and tree items by aria-level; Tela does not.

// setClass is a class of item roles that count together in a set.
type setClass uint8

const (
	setNone setClass = iota
	setOption
	setMenuItem
	setTab
	setRadio
	setTreeItem
	setListItem
)

// itemClass returns the set class of an item role.
func itemClass(role string) setClass {
	switch role {
	case "option":
		return setOption
	case "menuitem", "menuitemcheckbox", "menuitemradio":
		return setMenuItem
	case "tab":
		return setTab
	case "radio":
		return setRadio
	case "treeitem":
		return setTreeItem
	case "listitem":
		return setListItem
	}
	return setNone
}

// setContainer reports whether role is a set container, and whether it
// takes items of class c.
func setContainer(role string, c setClass) (container, takes bool) {
	switch role {
	case "listbox":
		return true, c == setOption
	case "menu", "menubar":
		return true, c == setMenuItem
	case "tablist":
		return true, c == setTab
	case "radiogroup":
		return true, c == setRadio
	case "tree":
		return true, c == setTreeItem
	case "list", "directory":
		return true, c == setListItem
	case "group":
		return true, c == setOption || c == setMenuItem || c == setTreeItem || c == setListItem
	}
	return false, false
}

// FillSetPositions computes the PosInSet and SetSize of the items of every
// set in the tree rooted at root (see the comment above). The values set
// on the nodes before the call are the explicit ones (AriaPosInSet,
// AriaSetSize); it must run once per tree.
func FillSetPositions(root *Node) {
	var items []*Node
	root.Walk(func(n *Node) bool {
		if n.Kind != KindElement || n.Has(StateHidden) {
			return n.Kind == KindRoot
		}
		if c, _ := setContainer(n.Role, setNone); !c {
			return true
		}
		items = collectItems(n, items[:0])
		for _, c := range [...]setClass{setOption, setMenuItem, setTab, setRadio, setTreeItem, setListItem} {
			if _, takes := setContainer(n.Role, c); takes {
				number(n, items, c)
			}
		}
		return true
	})
}

// collectItems appends to items the item descendants of container n (see
// the comment above).
func collectItems(n *Node, items []*Node) []*Node {
	for _, c := range n.Children {
		if c.Kind != KindElement || c.Has(StateHidden) {
			continue
		}
		if itemClass(c.Role) != setNone {
			items = append(items, c)
			continue
		}
		if container, _ := setContainer(c.Role, setNone); container {
			continue
		}
		items = collectItems(c, items)
	}
	return items
}

// number numbers the items of class c of the set of container n.
func number(n *Node, items []*Node, c setClass) {
	pos, count, largest := 0, 0, 0
	for _, it := range items {
		if itemClass(it.Role) != c {
			continue
		}
		count++
		if it.PosInSet > 0 {
			pos = it.PosInSet
		} else {
			pos++
			it.PosInSet = pos
		}
		largest = max(largest, pos)
	}
	if count == 0 {
		return
	}
	size := n.SetSize
	if size == 0 {
		size = max(count, largest)
	}
	for _, it := range items {
		if itemClass(it.Role) == c && it.SetSize == 0 {
			it.SetSize = size
		}
	}
}
