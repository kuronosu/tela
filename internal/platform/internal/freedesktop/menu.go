//go:build linux

package freedesktop

import (
	"reflect"
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// menuNode is an item of a com.canonical.dbusmenu layout. The root has
// id 0 and holds the top-level items.
type menuNode struct {
	id       int32
	props    map[string]dbus.Variant
	children []*menuNode
	// item is the app's ID of an item that can be activated (enabled, not
	// a separator, no submenu); 0 otherwise.
	item platform.MenuItemID
}

// menuIDs assigns dbusmenu ids (int32, root 0) to the items of a tray's
// successive menus. An activatable item keeps its id across updates (it is
// keyed by its MenuItemID), so a click on a menu the host built before an
// update still reaches the same item; separators and submenus get fresh
// ids each time, which stale clicks cannot match.
type menuIDs struct {
	byItem map[platform.MenuItemID]int32
	next   int32
}

func (m *menuIDs) fresh() int32 {
	m.next++
	return m.next
}

// build returns the layout of items and an index of its nodes by id.
func (m *menuIDs) build(items []platform.MenuItem) (*menuNode, map[int32]*menuNode) {
	if m.byItem == nil {
		m.byItem = map[platform.MenuItemID]int32{}
	}
	root := &menuNode{props: map[string]dbus.Variant{
		"children-display": dbus.MakeVariant("submenu"),
	}}
	nodes := map[int32]*menuNode{0: root}
	var walk func(parent *menuNode, items []platform.MenuItem)
	walk = func(parent *menuNode, items []platform.MenuItem) {
		for _, it := range items {
			n := &menuNode{props: map[string]dbus.Variant{
				"visible": dbus.MakeVariant(true),
			}}
			switch {
			case it.Separator:
				n.id = m.fresh()
				n.props["type"] = dbus.MakeVariant("separator")
			case len(it.Submenu) > 0:
				n.id = m.fresh()
				n.props["label"] = dbus.MakeVariant(menuLabel(it.Label))
				n.props["enabled"] = dbus.MakeVariant(!it.Disabled)
				n.props["children-display"] = dbus.MakeVariant("submenu")
				walk(n, it.Submenu)
			default:
				id, ok := m.byItem[it.ID]
				if !ok {
					id = m.fresh()
					m.byItem[it.ID] = id
				}
				n.id = id
				n.props["label"] = dbus.MakeVariant(menuLabel(it.Label))
				n.props["enabled"] = dbus.MakeVariant(!it.Disabled)
				if it.Checkable {
					state := int32(0)
					if it.Checked {
						state = 1
					}
					n.props["toggle-type"] = dbus.MakeVariant("checkmark")
					n.props["toggle-state"] = dbus.MakeVariant(state)
				}
				if !it.Disabled {
					n.item = it.ID
				}
			}
			parent.children = append(parent.children, n)
			nodes[n.id] = n
		}
	}
	walk(root, items)
	return root, nodes
}

// menuLabel escapes the dbusmenu mnemonic character: "_" marks the next
// letter as the access key, and "__" is a literal underscore.
func menuLabel(s string) string { return strings.ReplaceAll(s, "_", "__") }

// layoutItem is the D-Bus form of a layout node, (ia{sv}av): id,
// properties, and the children as variants of the same structure.
type layoutItem struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// layout returns n as a layoutItem with children down to depth levels (-1:
// all; 0: none), keeping only the named properties (none named: all).
func (n *menuNode) layout(depth int32, names []string) layoutItem {
	li := layoutItem{ID: n.id, Props: n.filter(names), Children: []dbus.Variant{}}
	if depth == 0 {
		return li
	}
	for _, c := range n.children {
		li.Children = append(li.Children, dbus.MakeVariant(c.layout(depth-1, names)))
	}
	return li
}

// filter returns the properties of n restricted to names (all if empty).
func (n *menuNode) filter(names []string) map[string]dbus.Variant {
	out := make(map[string]dbus.Variant, len(n.props))
	for k, v := range n.props {
		if len(names) == 0 || slices.Contains(names, k) {
			out[k] = v
		}
	}
	return out
}

// equalMenus reports whether two menus are the same (the layout need not
// be sent again).
func equalMenus(a, b []platform.MenuItem) bool {
	return slices.EqualFunc(a, b, func(x, y platform.MenuItem) bool {
		if x.Separator || y.Separator {
			return x.Separator == y.Separator
		}
		return x.ID == y.ID && x.Label == y.Label && x.Checkable == y.Checkable &&
			x.Checked == y.Checked && x.Disabled == y.Disabled && equalMenus(x.Submenu, y.Submenu)
	})
}

// itemPropNames is the D-Bus form (ias) of an item's removed properties in
// ItemsPropertiesUpdated.
type itemPropNames struct {
	ID    int32
	Names []string
}

// propChanges returns what changed in the properties of the items that are
// in both layouts (same id): the arguments of ItemsPropertiesUpdated, by
// id. A host is told about new structure with LayoutUpdated, but GNOME's
// AppIndicator extension then reads only each item's type again, so a new
// label of an item that keeps its id (a "Show"/"Hide" item following the
// window) reaches it only through this signal.
func propChanges(old, cur map[int32]*menuNode) (updated []itemProps, removed []itemPropNames) {
	ids := make([]int32, 0, len(cur))
	for id := range cur {
		if _, ok := old[id]; ok && id != 0 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		o, n := old[id].props, cur[id].props
		changed := map[string]dbus.Variant{}
		for k, v := range n {
			if ov, ok := o[k]; !ok || !reflect.DeepEqual(ov.Value(), v.Value()) {
				changed[k] = v
			}
		}
		var gone []string
		for k := range o {
			if _, ok := n[k]; !ok {
				gone = append(gone, k)
			}
		}
		if len(changed) > 0 {
			updated = append(updated, itemProps{ID: id, Props: changed})
		}
		if len(gone) > 0 {
			slices.Sort(gone)
			removed = append(removed, itemPropNames{ID: id, Names: gone})
		}
	}
	return updated, removed
}
