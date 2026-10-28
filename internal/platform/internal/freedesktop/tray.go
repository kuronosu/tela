//go:build linux

package freedesktop

import (
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/kuronosu/tela/internal/platform"
)

// StatusNotifierItem and dbusmenu names. The item lives at the path hosts
// assume for an item registered by bus name.
const (
	itemIface = "org.kde.StatusNotifierItem"
	itemPath  = dbus.ObjectPath("/StatusNotifierItem")
	menuIface = "com.canonical.dbusmenu"
	menuPath  = dbus.ObjectPath("/MenuBar")
	// noMenuPath is the Menu of an item without a menu (KDE's convention:
	// hosts call ContextMenu instead of showing an empty menu).
	noMenuPath = dbus.ObjectPath("/NO_DBUSMENU")

	propsIface = "org.freedesktop.DBus.Properties"
)

// tray is one StatusNotifierItem. Each has its own connection: an item
// cannot be unregistered, and the watcher drops it when its bus name
// vanishes, so RemoveTray closes the connection; and an item registered
// by name lives at /StatusNotifierItem, which only one item per connection
// can use.
type tray struct {
	d    *Desktop
	id   platform.TrayID
	name string // org.kde.StatusNotifierItem-<pid>-<id>
	conn *dbus.Conn

	// UI thread only.
	opts platform.TrayOptions // the latest options (icons not kept)
	ids  menuIDs

	mu   sync.Mutex // guards snap, read by godbus goroutines
	snap *traySnapshot
}

// traySnapshot is what the item's D-Bus methods read; immutable once
// published.
type traySnapshot struct {
	pixmaps  []pixmap
	tooltip  string
	root     *menuNode // nil: no menu
	nodes    map[int32]*menuNode
	revision uint32
}

func (t *tray) snapshot() *traySnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

// NewTray adds a StatusNotifierItem and registers it with the watcher.
// Without a watcher (no panel with a tray) it returns ErrUnsupported.
func (d *Desktop) NewTray(opts platform.TrayOptions) (platform.TrayID, error) {
	platform.CheckTrayOptions(opts)
	conn, err := d.bus()
	if err != nil {
		return 0, err
	}
	if !d.has(conn, watcherName) {
		return 0, unsupported("no StatusNotifierWatcher (%s) on the session bus: the desktop has no tray that shows StatusNotifierItems", watcherName)
	}
	if !d.watcherMatched {
		// A panel that restarts brings a new watcher, which knows no
		// items: they register again (onNameOwnerChanged).
		if err := addMatch(conn,
			dbus.WithMatchSender(busIface),
			dbus.WithMatchInterface(busIface),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, watcherName)); err != nil {
			return 0, unsupported("cannot watch the StatusNotifierWatcher (%v)", err)
		}
		d.watcherMatched = true
	}
	tc, err := d.dial()
	if err != nil {
		return 0, err
	}
	d.lastTray++
	t := &tray{d: d, id: d.lastTray, conn: tc}
	t.name = fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), t.id)
	t.set(opts)
	if err := t.export(); err != nil {
		tc.Close()
		return 0, fmt.Errorf("freedesktop: cannot export the tray icon: %w", err)
	}
	reply, err := tc.RequestName(t.name, dbus.NameFlagDoNotQueue)
	if err == nil && reply != dbus.RequestNameReplyPrimaryOwner {
		err = fmt.Errorf("the bus answered %v", reply)
	}
	if err != nil {
		tc.Close()
		return 0, fmt.Errorf("freedesktop: cannot own the name %s: %w", t.name, err)
	}
	d.mu.Lock()
	d.trays[t.id] = t
	d.mu.Unlock()
	t.register()
	return t.id, nil
}

// UpdateTray replaces the icon, tooltip and menu, and tells the host what
// changed.
func (d *Desktop) UpdateTray(id platform.TrayID, opts platform.TrayOptions) error {
	platform.CheckTrayOptions(opts)
	d.mu.Lock()
	t := d.trays[id]
	d.mu.Unlock()
	if t == nil {
		if d.isClosed() {
			return errClosed
		}
		return fmt.Errorf("freedesktop: UpdateTray: tray %d does not exist", id)
	}
	old := t.snapshot()
	hadMenu := len(t.opts.Menu) > 0
	menuChanged := !equalMenus(t.opts.Menu, opts.Menu)
	t.set(opts)
	s := t.snapshot()
	var invalidated []string
	if !equalPixmaps(old.pixmaps, s.pixmaps) {
		t.conn.Emit(itemPath, itemIface+".NewIcon")
		invalidated = append(invalidated, "IconPixmap")
	}
	if old.tooltip != s.tooltip {
		t.conn.Emit(itemPath, itemIface+".NewToolTip")
		invalidated = append(invalidated, "ToolTip")
	}
	if hadMenu != (len(opts.Menu) > 0) {
		invalidated = append(invalidated, "Menu")
	}
	if len(invalidated) > 0 {
		t.conn.Emit(itemPath, propsIface+".PropertiesChanged", itemIface, map[string]dbus.Variant{}, invalidated)
	}
	if menuChanged {
		t.conn.Emit(menuPath, menuIface+".LayoutUpdated", s.revision, int32(0))
		if old != nil {
			if updated, removed := propChanges(old.nodes, s.nodes); len(updated)+len(removed) > 0 {
				if updated == nil {
					updated = []itemProps{}
				}
				if removed == nil {
					removed = []itemPropNames{}
				}
				t.conn.Emit(menuPath, menuIface+".ItemsPropertiesUpdated", updated, removed)
			}
		}
	}
	return nil
}

// RemoveTray closes the tray's connection, which removes it from the host.
func (d *Desktop) RemoveTray(id platform.TrayID) error {
	d.mu.Lock()
	t := d.trays[id]
	delete(d.trays, id)
	closed := d.closed
	d.mu.Unlock()
	if t == nil && closed {
		return errClosed
	}
	if t != nil {
		t.close()
	}
	return nil
}

// set builds and publishes the snapshot of opts. UI thread.
func (t *tray) set(opts platform.TrayOptions) {
	s := &traySnapshot{
		pixmaps: trayPixmaps(opts.Icon),
		tooltip: opts.Tooltip,
	}
	old := t.snapshot()
	switch {
	case old == nil:
		s.revision = 1
	case equalMenus(t.opts.Menu, opts.Menu):
		s.revision, s.root, s.nodes = old.revision, old.root, old.nodes
	default:
		s.revision = old.revision + 1
	}
	if s.root == nil && len(opts.Menu) > 0 {
		s.root, s.nodes = t.ids.build(opts.Menu)
	}
	t.opts = platform.TrayOptions{Tooltip: opts.Tooltip, Menu: cloneMenu(opts.Menu)}
	t.mu.Lock()
	t.snap = s
	t.mu.Unlock()
}

func cloneMenu(items []platform.MenuItem) []platform.MenuItem {
	if items == nil {
		return nil
	}
	out := make([]platform.MenuItem, len(items))
	copy(out, items)
	for i := range out {
		out[i].Submenu = cloneMenu(out[i].Submenu)
	}
	return out
}

// register registers the item with the watcher. Safe from any goroutine.
func (t *tray) register() {
	t.conn.Object(watcherName, watcherPath).Go(watcherIface+".RegisterStatusNotifierItem", 0, nil, t.name)
}

func (t *tray) close() { t.conn.Close() }

// onNameOwnerChanged registers every tray again when a watcher appears.
func (d *Desktop) onNameOwnerChanged(s *dbus.Signal) {
	var name, oldOwner, newOwner string
	if dbus.Store(s.Body, &name, &oldOwner, &newOwner) != nil || name != watcherName || newOwner == "" {
		return
	}
	d.mu.Lock()
	trays := make([]*tray, 0, len(d.trays))
	for _, t := range d.trays {
		trays = append(trays, t)
	}
	d.mu.Unlock()
	for _, t := range trays {
		t.register()
	}
}

// --- exported objects ---

// toolTip is the ToolTip property, (sa(iiay)ss): icon name, icon pixmaps,
// title, description.
type toolTip struct {
	IconName    string
	IconPixmaps []pixmap
	Title       string
	Description string
}

// itemProperties returns the StatusNotifierItem properties.
func (t *tray) itemProperties() map[string]dbus.Variant {
	s := t.snapshot()
	menu := noMenuPath
	if s.root != nil {
		menu = menuPath
	}
	pixmaps := s.pixmaps
	if pixmaps == nil {
		pixmaps = []pixmap{}
	}
	return map[string]dbus.Variant{
		"Category":            dbus.MakeVariant("ApplicationStatus"),
		"Id":                  dbus.MakeVariant(t.d.opts.AppID),
		"Title":               dbus.MakeVariant(t.d.opts.AppID),
		"Status":              dbus.MakeVariant("Active"),
		"WindowId":            dbus.MakeVariant(int32(0)),
		"IconName":            dbus.MakeVariant(""),
		"IconPixmap":          dbus.MakeVariant(pixmaps),
		"IconThemePath":       dbus.MakeVariant(""),
		"OverlayIconName":     dbus.MakeVariant(""),
		"OverlayIconPixmap":   dbus.MakeVariant([]pixmap{}),
		"AttentionIconName":   dbus.MakeVariant(""),
		"AttentionIconPixmap": dbus.MakeVariant([]pixmap{}),
		"AttentionMovieName":  dbus.MakeVariant(""),
		"ToolTip":             dbus.MakeVariant(toolTip{IconPixmaps: []pixmap{}, Title: s.tooltip}),
		"ItemIsMenu":          dbus.MakeVariant(false),
		"Menu":                dbus.MakeVariant(menu),
	}
}

// menuProperties returns the dbusmenu object's properties.
func (t *tray) menuProperties() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"Version":       dbus.MakeVariant(uint32(3)),
		"TextDirection": dbus.MakeVariant("ltr"),
		"Status":        dbus.MakeVariant("normal"),
		"IconThemePath": dbus.MakeVariant([]string{}),
	}
}

// export exports the item and its menu on the tray's connection.
func (t *tray) export() error {
	itemTable := map[string]any{
		"Activate": func(x, y int32) *dbus.Error {
			t.d.push(item{ev: platform.TrayEvent{Tray: t.id, Kind: platform.TrayActivated}})
			return nil
		},
		// A middle click and a request to show the menu: the contract has
		// no event for the first, and hosts show the dbusmenu themselves.
		"SecondaryActivate": func(x, y int32) *dbus.Error { return nil },
		"ContextMenu":       func(x, y int32) *dbus.Error { return nil },
		"Scroll":            func(delta int32, orientation string) *dbus.Error { return nil },
		// KDE hosts send an XDG activation token before Activate on
		// Wayland.
		"ProvideXdgActivationToken": func(token string) *dbus.Error {
			t.d.setActivationToken(token)
			return nil
		},
	}
	menuTable := map[string]any{
		"GetLayout":          t.getLayout,
		"GetGroupProperties": t.getGroupProperties,
		"GetProperty":        t.getProperty,
		"Event":              t.event,
		"EventGroup":         t.eventGroup,
		"AboutToShow":        func(id int32) (bool, *dbus.Error) { return false, nil },
		"AboutToShowGroup": func(ids []int32) ([]int32, []int32, *dbus.Error) {
			return []int32{}, []int32{}, nil
		},
	}
	for _, e := range []struct {
		path   dbus.ObjectPath
		iface  string
		table  map[string]any
		props  func() map[string]dbus.Variant
		intros string
	}{
		{itemPath, itemIface, itemTable, t.itemProperties, itemIntrospection},
		{menuPath, menuIface, menuTable, t.menuProperties, menuIntrospection},
	} {
		if err := t.conn.ExportMethodTable(e.table, e.path, e.iface); err != nil {
			return err
		}
		if err := exportProperties(t.conn, e.path, e.iface, e.props); err != nil {
			return err
		}
		if err := t.conn.Export(introspect.Introspectable(e.intros), e.path, "org.freedesktop.DBus.Introspectable"); err != nil {
			return err
		}
	}
	return nil
}

// exportProperties exports a read-only org.freedesktop.DBus.Properties
// for the interface iface at path, whose values get computes.
func exportProperties(conn *dbus.Conn, path dbus.ObjectPath, iface string, get func() map[string]dbus.Variant) error {
	unknownIface := func(i string) *dbus.Error {
		e := dbus.MakeUnknownInterfaceError(i)
		return &e
	}
	return conn.ExportMethodTable(map[string]any{
		"Get": func(i, name string) (dbus.Variant, *dbus.Error) {
			if i != iface && i != "" {
				return dbus.Variant{}, unknownIface(i)
			}
			v, ok := get()[name]
			if !ok {
				return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{"no property " + name})
			}
			return v, nil
		},
		"GetAll": func(i string) (map[string]dbus.Variant, *dbus.Error) {
			if i != iface && i != "" {
				return nil, unknownIface(i)
			}
			return get(), nil
		},
		"Set": func(i, name string, v dbus.Variant) *dbus.Error {
			return dbus.NewError("org.freedesktop.DBus.Error.PropertyReadOnly", []any{name + " is read-only"})
		},
	}, path, propsIface)
}

func errUnknownItem(id int32) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{fmt.Sprintf("no menu item %d", id)})
}

// getLayout is GetLayout(i parentId, i recursionDepth, as propertyNames)
// → (u revision, (ia{sv}av) layout).
func (t *tray) getLayout(parent, depth int32, names []string) (uint32, layoutItem, *dbus.Error) {
	s := t.snapshot()
	n := s.nodes[parent]
	if parent == 0 && n == nil {
		n = &menuNode{props: map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}}
	}
	if n == nil {
		return 0, layoutItem{}, errUnknownItem(parent)
	}
	return s.revision, n.layout(depth, names), nil
}

// itemProps is an entry of GetGroupProperties' answer, (ia{sv}).
type itemProps struct {
	ID    int32
	Props map[string]dbus.Variant
}

// getGroupProperties is GetGroupProperties(ai ids, as propertyNames) →
// a(ia{sv}); no ids means every item. Unknown ids are skipped.
func (t *tray) getGroupProperties(ids []int32, names []string) ([]itemProps, *dbus.Error) {
	s := t.snapshot()
	out := []itemProps{}
	if len(ids) == 0 {
		for id := range s.nodes {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if n := s.nodes[id]; n != nil {
			out = append(out, itemProps{id, n.filter(names)})
		}
	}
	return out, nil
}

// getProperty is GetProperty(i id, s name) → v.
func (t *tray) getProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	n := t.snapshot().nodes[id]
	if n == nil {
		return dbus.Variant{}, errUnknownItem(id)
	}
	v, ok := n.props[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"no property " + name})
	}
	return v, nil
}

// event is Event(i id, s eventId, v data, u timestamp). A "clicked" on an
// item that can be activated is a TrayMenuItem event; other events
// ("hovered", "opened", "closed") are ignored.
func (t *tray) event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	n := t.snapshot().nodes[id]
	if n == nil {
		return errUnknownItem(id)
	}
	if eventID == "clicked" && n.item != 0 {
		t.d.push(item{ev: platform.TrayEvent{Tray: t.id, Kind: platform.TrayMenuItem, Item: n.item}})
	}
	return nil
}

// menuEvent is an entry of EventGroup, (isvu).
type menuEvent struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}

// eventGroup is EventGroup(a(isvu) events) → ai idErrors: the ids that
// were not found.
func (t *tray) eventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	missing := []int32{}
	for _, e := range events {
		if t.event(e.ID, e.EventID, e.Data, e.Timestamp) != nil {
			missing = append(missing, e.ID)
		}
	}
	if len(events) > 0 && len(missing) == len(events) {
		return missing, errUnknownItem(missing[0])
	}
	return missing, nil
}

// Introspection data of the exported objects (some hosts introspect
// before calling).
const itemIntrospection = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
 <interface name="org.kde.StatusNotifierItem">
  <property name="Category" type="s" access="read"/>
  <property name="Id" type="s" access="read"/>
  <property name="Title" type="s" access="read"/>
  <property name="Status" type="s" access="read"/>
  <property name="WindowId" type="i" access="read"/>
  <property name="IconName" type="s" access="read"/>
  <property name="IconPixmap" type="a(iiay)" access="read"/>
  <property name="IconThemePath" type="s" access="read"/>
  <property name="OverlayIconName" type="s" access="read"/>
  <property name="OverlayIconPixmap" type="a(iiay)" access="read"/>
  <property name="AttentionIconName" type="s" access="read"/>
  <property name="AttentionIconPixmap" type="a(iiay)" access="read"/>
  <property name="AttentionMovieName" type="s" access="read"/>
  <property name="ToolTip" type="(sa(iiay)ss)" access="read"/>
  <property name="ItemIsMenu" type="b" access="read"/>
  <property name="Menu" type="o" access="read"/>
  <method name="ContextMenu"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
  <method name="Activate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
  <method name="SecondaryActivate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
  <method name="Scroll"><arg name="delta" type="i" direction="in"/><arg name="orientation" type="s" direction="in"/></method>
  <method name="ProvideXdgActivationToken"><arg name="token" type="s" direction="in"/></method>
  <signal name="NewTitle"/>
  <signal name="NewIcon"/>
  <signal name="NewAttentionIcon"/>
  <signal name="NewOverlayIcon"/>
  <signal name="NewToolTip"/>
  <signal name="NewStatus"><arg name="status" type="s"/></signal>
 </interface>
 <interface name="org.freedesktop.DBus.Properties">
  <method name="Get"><arg name="interface" type="s" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="out"/></method>
  <method name="GetAll"><arg name="interface" type="s" direction="in"/><arg name="properties" type="a{sv}" direction="out"/></method>
  <method name="Set"><arg name="interface" type="s" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="in"/></method>
  <signal name="PropertiesChanged"><arg name="interface" type="s"/><arg name="changed" type="a{sv}"/><arg name="invalidated" type="as"/></signal>
 </interface>
 <interface name="org.freedesktop.DBus.Introspectable">
  <method name="Introspect"><arg name="data" type="s" direction="out"/></method>
 </interface>
</node>`

const menuIntrospection = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
 <interface name="com.canonical.dbusmenu">
  <property name="Version" type="u" access="read"/>
  <property name="TextDirection" type="s" access="read"/>
  <property name="Status" type="s" access="read"/>
  <property name="IconThemePath" type="as" access="read"/>
  <method name="GetLayout"><arg name="parentId" type="i" direction="in"/><arg name="recursionDepth" type="i" direction="in"/><arg name="propertyNames" type="as" direction="in"/><arg name="revision" type="u" direction="out"/><arg name="layout" type="(ia{sv}av)" direction="out"/></method>
  <method name="GetGroupProperties"><arg name="ids" type="ai" direction="in"/><arg name="propertyNames" type="as" direction="in"/><arg name="properties" type="a(ia{sv})" direction="out"/></method>
  <method name="GetProperty"><arg name="id" type="i" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="out"/></method>
  <method name="Event"><arg name="id" type="i" direction="in"/><arg name="eventId" type="s" direction="in"/><arg name="data" type="v" direction="in"/><arg name="timestamp" type="u" direction="in"/></method>
  <method name="EventGroup"><arg name="events" type="a(isvu)" direction="in"/><arg name="idErrors" type="ai" direction="out"/></method>
  <method name="AboutToShow"><arg name="id" type="i" direction="in"/><arg name="needUpdate" type="b" direction="out"/></method>
  <method name="AboutToShowGroup"><arg name="ids" type="ai" direction="in"/><arg name="updatesNeeded" type="ai" direction="out"/><arg name="idErrors" type="ai" direction="out"/></method>
  <signal name="ItemsPropertiesUpdated"><arg name="updatedProps" type="a(ia{sv})"/><arg name="removedProps" type="a(ias)"/></signal>
  <signal name="LayoutUpdated"><arg name="revision" type="u"/><arg name="parent" type="i"/></signal>
  <signal name="ItemActivationRequested"><arg name="id" type="i"/><arg name="timestamp" type="u"/></signal>
 </interface>
 <interface name="org.freedesktop.DBus.Properties">
  <method name="Get"><arg name="interface" type="s" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="out"/></method>
  <method name="GetAll"><arg name="interface" type="s" direction="in"/><arg name="properties" type="a{sv}" direction="out"/></method>
  <method name="Set"><arg name="interface" type="s" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="in"/></method>
  <signal name="PropertiesChanged"><arg name="interface" type="s"/><arg name="changed" type="a{sv}"/><arg name="invalidated" type="as"/></signal>
 </interface>
 <interface name="org.freedesktop.DBus.Introspectable">
  <method name="Introspect"><arg name="data" type="s" direction="out"/></method>
 </interface>
</node>`
