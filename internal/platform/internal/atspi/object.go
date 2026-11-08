//go:build linux

package atspi

import (
	"fmt"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// The objects the bridge serves on the accessibility bus. godbus calls
// handler (the connection's dbus.Handler) on its own goroutines, one per
// method call; every answer comes from the windows' snapshots and the
// atomics of Bridge, never from the UI thread.
//
// Paths: the application is /org/a11y/atspi/accessible/root (as GTK and
// AccessKit register it), window N's frame /org/a11y/atspi/accessible/wN
// and node ID of its tree /org/a11y/atspi/accessible/wN_ID.

// ref is an object reference, (so) on the bus.
type ref struct {
	Name string
	Path dbus.ObjectPath
}

// relation is one entry of GetRelationSet, (ua(so)).
type relation struct {
	Type    uint32
	Targets []ref
}

// extents is a rectangle, (iiii).
type extents struct{ X, Y, W, H int32 }

// actionInfo is one entry of GetActions, (sss).
type actionInfo struct{ Name, Description, KeyBinding string }

// cacheItem is one entry of Cache.GetItems.
type cacheItem struct {
	Self, App, Parent ref
	Index, Count      int32
	Interfaces        []string
	Name              string
	Role              uint32
	Description       string
	States            []uint32
}

// boundedRange is one entry of Text.GetBoundedRanges, (iisv).
type boundedRange struct {
	Start, End int32
	Text       string
	Value      dbus.Variant
}

func (b *Bridge) name() string {
	if p := b.unique.Load(); p != nil {
		return *p
	}
	return ""
}

func (b *Bridge) appRef() ref  { return ref{b.name(), appRootPath} }
func (b *Bridge) nullRef() ref { return ref{b.name(), nullPath} }

// ref returns the reference of node x of window w: the frame for the
// root (or nil).
func (b *Bridge) ref(w *Window, x *vnode) ref {
	return ref{b.name(), w.objectPath(x)}
}

func (w *Window) objectPath(x *vnode) dbus.ObjectPath {
	if x == nil || x.n.Kind == platform.AccessRoot {
		return w.path
	}
	return dbus.ObjectPath(fmt.Sprintf("%s_%d", w.path, x.n.ID))
}

// liveWindows returns the application's windows.
func (b *Bridge) liveWindows() []*Window {
	b.wmu.RLock()
	defer b.wmu.RUnlock()
	return slices.Clone(b.windows)
}

func (b *Bridge) window(id uint32) *Window {
	b.wmu.RLock()
	defer b.wmu.RUnlock()
	for _, w := range b.windows {
		if w.id == id {
			return w
		}
	}
	return nil
}

func (b *Bridge) locale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := b.getenv(k); v != "" {
			if i := strings.IndexAny(v, ".@"); i >= 0 {
				v = v[:i]
			}
			return v
		}
	}
	return "C"
}

// toolkitVersion is the version of the Tela module the app was built
// with ("" in tests, "(devel)" in a checkout).
func toolkitVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/kuronosu/tela" {
				return d.Version
			}
		}
		if bi.Main.Path == "github.com/kuronosu/tela" {
			return bi.Main.Version
		}
	}
	return ""
}

func dbusErr(name, msg string) *dbus.Error { return &dbus.Error{Name: name, Body: []any{msg}} }

var errGone = dbusErr(errUnknownObject, "the accessible object is gone")

// --- dispatch ---

// handler is the dbus.Handler of the accessibility connection.
type handler struct{ b *Bridge }

func (h *handler) LookupObject(path dbus.ObjectPath) (dbus.ServerObject, bool) {
	switch path {
	case appRootPath:
		return &object{b: h.b, kind: objApp}, true
	case cachePath:
		return &object{b: h.b, kind: objCache}, true
	}
	p := string(path)
	if !strings.HasPrefix(p, accessiblePrefix+"w") {
		return nil, false
	}
	p = p[len(accessiblePrefix)+1:]
	ws, ns, node := strings.Cut(p, "_")
	wid, err := strconv.ParseUint(ws, 10, 32)
	if err != nil {
		return nil, false
	}
	o := &object{b: h.b, kind: objFrame, wid: uint32(wid)}
	if node {
		id, err := strconv.ParseUint(ns, 10, 64)
		if err != nil || id == 0 {
			return nil, false
		}
		o.kind, o.id = objNode, platform.AccessID(id)
	}
	return o, true
}

type objKind uint8

const (
	objApp objKind = iota
	objFrame
	objNode
	objCache
)

// object is a path of the bridge: the application, a frame, a node or the
// cache.
type object struct {
	b    *Bridge
	kind objKind
	wid  uint32
	id   platform.AccessID
}

func (o *object) LookupInterface(name string) (dbus.Interface, bool) {
	return &iface{o: o, name: name}, true
}

type iface struct {
	o    *object
	name string
}

func (i *iface) LookupMethod(name string) (dbus.Method, bool) {
	return &method{o: i.o, iface: i.name, name: name}, true
}

// method is a method call on an object; it decodes no arguments itself
// (the body is passed as godbus decoded it).
type method struct {
	o     *object
	iface string
	name  string
}

func (m *method) DecodeArguments(_ *dbus.Conn, _ string, _ *dbus.Message, args []any) ([]any, error) {
	return args, nil
}
func (m *method) NumArguments() int     { return 0 }
func (m *method) NumReturns() int       { return 0 }
func (m *method) ArgumentValue(int) any { return nil }
func (m *method) ReturnValue(int) any   { return nil }
func (m *method) Call(args ...any) ([]any, error) {
	c := &call{b: m.o.b, o: m.o, args: args}
	if err := c.resolve(); err != nil {
		if m.iface == ifaceAccessible && m.name == "GetState" {
			var s stateSet
			s.set(stateDefunct)
			return []any{s.words()}, nil
		}
		return nil, err
	}
	out, err := c.dispatch(m.iface, m.name)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// call is one method call with its target resolved.
type call struct {
	b    *Bridge
	o    *object
	args []any
	w    *Window
	s    *snapshot
	v    *vnode // the node (the root for a frame); nil for the app and the cache
}

func (c *call) resolve() error {
	switch c.o.kind {
	case objApp, objCache:
		return nil
	}
	w := c.b.window(c.o.wid)
	if w == nil || w.removed.Load() {
		return errGone
	}
	c.w, c.s = w, w.snap.Load()
	if c.o.kind == objFrame {
		c.v = c.s.root()
		return nil
	}
	c.v = c.s.node(c.o.id)
	if c.v == nil {
		return errGone
	}
	return nil
}

// interfaces returns the interfaces of the call's object.
func (c *call) interfaces() []string {
	switch c.o.kind {
	case objApp:
		return []string{ifaceAccessible, ifaceApplication}
	case objCache:
		return []string{ifaceCache}
	}
	out := []string{ifaceAccessible, ifaceComponent}
	n := c.v.n
	if len(nodeActions(n)) > 0 {
		out = append(out, ifaceAction)
	}
	if c.v.hasText {
		out = append(out, ifaceText)
	}
	if editable(n) {
		out = append(out, ifaceEditableText)
	}
	if rangeWidget(n) {
		out = append(out, ifaceValue)
	}
	if selectionContainer(n) {
		out = append(out, ifaceSelection)
	}
	return out
}

func (c *call) has(iface string) bool { return slices.Contains(c.interfaces(), iface) }

func (c *call) dispatch(ifc, name string) ([]any, error) {
	switch ifc {
	case propsIface:
		return c.properties(name)
	case introspectIface:
		if name == "Introspect" {
			return []any{c.introspect()}, nil
		}
	case "":
		// No interface in the call: the first one that has the method.
		for _, i := range c.interfaces() {
			if out, err := c.dispatch(i, name); err == nil || !isUnknownMethod(err) {
				return out, err
			}
		}
	default:
		if !c.has(ifc) {
			return nil, dbusErr("org.freedesktop.DBus.Error.UnknownInterface", "the object has no interface "+ifc)
		}
		var f func(string) ([]any, error)
		switch ifc {
		case ifaceAccessible:
			f = c.accessible
		case ifaceApplication:
			f = c.application
		case ifaceComponent:
			f = c.component
		case ifaceAction:
			f = c.actionIface
		case ifaceValue:
			f = c.value
		case ifaceText:
			f = c.text
		case ifaceEditableText:
			f = c.editableText
		case ifaceSelection:
			f = c.selection
		case ifaceCache:
			f = c.cache
		}
		if f != nil {
			return f(name)
		}
	}
	return nil, unknownMethod(ifc, name)
}

func unknownMethod(ifc, name string) *dbus.Error {
	return dbusErr(errUnknownMethod, fmt.Sprintf("no method %s.%s", ifc, name))
}

func isUnknownMethod(err error) bool {
	e, ok := err.(*dbus.Error)
	return ok && e.Name == errUnknownMethod
}

// --- arguments ---

var errArgs = dbusErr(errInvalidArgs, "invalid arguments")

func (c *call) int(i int) (int, error) {
	if i >= len(c.args) {
		return 0, errArgs
	}
	switch v := c.args[i].(type) {
	case int32:
		return int(v), nil
	case uint32:
		return int(v), nil
	case int16:
		return int(v), nil
	case int64:
		return int(v), nil
	}
	return 0, errArgs
}

func (c *call) str(i int) (string, error) {
	if i >= len(c.args) {
		return "", errArgs
	}
	s, ok := c.args[i].(string)
	if !ok {
		return "", errArgs
	}
	return s, nil
}

// ints reads n integer arguments.
func (c *call) ints(n int) ([]int, error) {
	out := make([]int, n)
	for i := range out {
		v, err := c.int(i)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// --- actions ---

// request posts an AT's request on the call's node to the UI thread.
func (c *call) request(e platform.AccessActionEvent) bool {
	if c.v == nil || c.v.n.ID == 0 {
		return false
	}
	e.Node = c.v.n.ID
	if platform.CheckAccessActionEvent(e) != "" {
		return false
	}
	c.b.post(inboxItem{w: c.w, gen: c.b.gen.Load(), action: e})
	return true
}

// perform requests action a when the node supports it.
func (c *call) perform(a platform.AccessAction) bool {
	if c.v == nil || !c.v.n.Actions.Has(a) {
		return false
	}
	return c.request(platform.AccessActionEvent{Action: a})
}

// --- org.freedesktop.DBus.Properties ---

func (c *call) properties(name string) ([]any, error) {
	switch name {
	case "Get":
		ifc, err := c.str(0)
		if err != nil {
			return nil, err
		}
		prop, err := c.str(1)
		if err != nil {
			return nil, err
		}
		all, err := c.props(ifc)
		if err != nil {
			return nil, err
		}
		v, ok := all[prop]
		if !ok {
			return nil, dbusErr(errUnknownProp, "no property "+ifc+"."+prop)
		}
		return []any{v}, nil
	case "GetAll":
		ifc, err := c.str(0)
		if err != nil {
			return nil, err
		}
		all, err := c.props(ifc)
		if err != nil {
			return nil, err
		}
		return []any{all}, nil
	case "Set":
		ifc, err := c.str(0)
		if err != nil {
			return nil, err
		}
		prop, err := c.str(1)
		if err != nil || len(c.args) < 3 {
			return nil, errArgs
		}
		v, ok := c.args[2].(dbus.Variant)
		if !ok {
			return nil, errArgs
		}
		return nil, c.setProp(ifc, prop, v)
	}
	return nil, unknownMethod(propsIface, name)
}

// props returns the properties of interface ifc of the object.
func (c *call) props(ifc string) (map[string]dbus.Variant, error) {
	if !c.has(ifc) {
		return nil, dbusErr("org.freedesktop.DBus.Error.UnknownInterface", "the object has no interface "+ifc)
	}
	mv := dbus.MakeVariant
	switch ifc {
	case ifaceAccessible:
		name, desc, parent, count := c.b.appName, "", c.b.desktopRef(), int32(len(c.b.liveWindows()))
		if c.v != nil {
			name, desc, parent, count = c.nodeName(), c.v.n.Description, c.parentRef(), int32(len(c.v.children))
		}
		return map[string]dbus.Variant{
			"Name": mv(name), "Description": mv(desc), "Parent": mv(parent), "ChildCount": mv(count),
			"Locale": mv(c.b.locale()), "AccessibleId": mv(""), "HelpText": mv(""),
		}, nil
	case ifaceApplication:
		v := toolkitVersion()
		return map[string]dbus.Variant{
			"ToolkitName": mv("Tela"), "Version": mv(v), "ToolkitVersion": mv(v),
			"AtspiVersion": mv("2.1"), "InterfaceVersion": mv(uint32(1)), "Id": mv(c.b.appID.Load()),
		}, nil
	case ifaceAction:
		return map[string]dbus.Variant{"NActions": mv(int32(len(nodeActions(c.v.n))))}, nil
	case ifaceValue:
		cur, lo, hi, step := rangeValues(c.v.n)
		return map[string]dbus.Variant{
			"MinimumValue": mv(lo), "MaximumValue": mv(hi), "MinimumIncrement": mv(step),
			"CurrentValue": mv(cur), "Text": mv(c.v.n.Value),
		}, nil
	case ifaceText:
		rs := []rune(c.v.text)
		return map[string]dbus.Variant{
			"CharacterCount": mv(int32(len(rs))), "CaretOffset": mv(int32(caretOffset(c.v, len(rs)))),
		}, nil
	case ifaceSelection:
		return map[string]dbus.Variant{"NSelectedChildren": mv(int32(len(c.selected())))}, nil
	}
	return map[string]dbus.Variant{}, nil
}

func (c *call) setProp(ifc, prop string, v dbus.Variant) error {
	switch {
	case c.o.kind == objApp && ifc == ifaceApplication && prop == "Id":
		// The registry numbers the applications it embeds.
		id, ok := v.Value().(int32)
		if !ok {
			return errArgs
		}
		c.b.appID.Store(id)
		return nil
	case ifc == ifaceValue && prop == "CurrentValue" && c.v != nil && rangeWidget(c.v.n):
		// Tela's range widgets take steps, not values: a value above the
		// current one is one increment, below one decrement.
		want, ok := v.Value().(float64)
		if !ok {
			return errArgs
		}
		cur, _, _, _ := rangeValues(c.v.n)
		switch {
		case want > cur && c.perform(platform.AccessActionIncrement):
			return nil
		case want < cur && c.perform(platform.AccessActionDecrement):
			return nil
		case want == cur:
			return nil
		}
		return dbusErr("org.freedesktop.DBus.Error.Failed", "the value cannot be set")
	}
	return dbusErr("org.freedesktop.DBus.Error.PropertyReadOnly", "the property "+ifc+"."+prop+" cannot be set")
}

func (b *Bridge) desktopRef() ref {
	if d := b.desktop.Load(); d != nil {
		return *d
	}
	return ref{registryName, desktopRootPath}
}

// --- org.a11y.atspi.Accessible ---

func (c *call) nodeName() string {
	if c.v == c.s.root() {
		if c.v.n.Name != "" {
			return c.v.n.Name
		}
		return c.s.title
	}
	return c.v.name()
}

func (c *call) parentRef() ref {
	if c.v == c.s.root() {
		return c.b.appRef()
	}
	return c.b.ref(c.w, c.s.node(c.v.parent))
}

func (c *call) childRefs() []ref {
	if c.o.kind == objApp {
		ws := c.b.liveWindows()
		out := make([]ref, 0, len(ws))
		for _, w := range ws {
			out = append(out, c.b.ref(w, nil))
		}
		return out
	}
	out := make([]ref, 0, len(c.v.children))
	for _, id := range c.v.children {
		out = append(out, c.b.ref(c.w, c.s.node(id)))
	}
	return out
}

func (c *call) accessible(name string) ([]any, error) {
	app := c.o.kind == objApp
	switch name {
	case "GetChildAtIndex":
		i, err := c.int(0)
		if err != nil {
			return nil, err
		}
		kids := c.childRefs()
		if i < 0 || i >= len(kids) {
			return []any{c.b.nullRef()}, nil
		}
		return []any{kids[i]}, nil
	case "GetChildren":
		return []any{c.childRefs()}, nil
	case "GetIndexInParent":
		switch {
		case app:
			return []any{int32(-1)}, nil
		case c.v == c.s.root():
			return []any{int32(c.b.windowIndex(c.w))}, nil
		}
		return []any{int32(c.v.index)}, nil
	case "GetRelationSet":
		return []any{c.relations()}, nil
	case "GetRole":
		return []any{c.role()}, nil
	case "GetRoleName", "GetLocalizedRoleName":
		return []any{roleName(c.role())}, nil
	case "GetState":
		if app {
			return []any{stateSet(0).words()}, nil
		}
		return []any{nodeStates(c.s, c.v).words()}, nil
	case "GetAttributes":
		if app {
			return []any{map[string]string{"toolkit": "Tela"}}, nil
		}
		return []any{nodeAttributes(c.s.view.tree, c.v.n)}, nil
	case "GetApplication":
		return []any{c.b.appRef()}, nil
	case "GetInterfaces":
		return []any{c.interfaces()}, nil
	}
	return nil, unknownMethod(ifaceAccessible, name)
}

func (c *call) role() uint32 {
	if c.o.kind == objApp {
		return roleApplication
	}
	return c.v.role
}

func (c *call) relations() []relation {
	out := []relation{}
	if c.v == nil {
		return out
	}
	if id := c.v.n.Controls; id != 0 {
		if t := c.s.node(id); t != nil {
			out = append(out, relation{relationControllerFor, []ref{c.b.ref(c.w, t)}})
		}
	}
	if ids := c.s.view.controlledBy[c.v.n.ID]; len(ids) > 0 {
		r := relation{Type: relationControlledBy}
		for _, id := range ids {
			r.Targets = append(r.Targets, c.b.ref(c.w, c.s.node(id)))
		}
		out = append(out, r)
	}
	return out
}

// --- org.a11y.atspi.Application ---

func (c *call) application(name string) ([]any, error) {
	switch name {
	case "GetLocale":
		return []any{c.b.locale()}, nil
	case "GetApplicationBusAddress":
		// No peer-to-peer connection: the AT talks over the bus.
		return []any{""}, nil
	}
	return nil, unknownMethod(ifaceApplication, name)
}

// --- org.a11y.atspi.Component ---

func (c *call) parentNode() *vnode {
	if c.v == c.s.root() {
		return nil
	}
	return c.s.node(c.v.parent)
}

func (c *call) extents(coord int) extents {
	x, y, w, h := c.s.extents(c.v.n.Bounds, uint32(coord), c.parentNode())
	return extents{x, y, w, h}
}

func (c *call) component(name string) ([]any, error) {
	switch name {
	case "Contains":
		a, err := c.ints(3)
		if err != nil {
			return nil, err
		}
		px, py := c.s.point(int32(a[0]), int32(a[1]), uint32(a[2]), c.parentNode())
		return []any{contains(c.v.n.Bounds, px, py)}, nil
	case "GetAccessibleAtPoint":
		a, err := c.ints(3)
		if err != nil {
			return nil, err
		}
		px, py := c.s.point(int32(a[0]), int32(a[1]), uint32(a[2]), c.parentNode())
		if c.v != c.s.root() && !contains(c.v.n.Bounds, px, py) {
			return []any{c.b.nullRef()}, nil
		}
		hit := c.s.view.hit(c.v, px, py)
		if hit == c.v {
			return []any{c.b.nullRef()}, nil
		}
		return []any{c.b.ref(c.w, hit)}, nil
	case "GetExtents":
		coord, err := c.int(0)
		if err != nil {
			return nil, err
		}
		return []any{c.extents(coord)}, nil
	case "GetPosition":
		coord, err := c.int(0)
		if err != nil {
			return nil, err
		}
		e := c.extents(coord)
		return []any{e.X, e.Y}, nil
	case "GetSize":
		e := c.extents(coordWindow)
		return []any{e.W, e.H}, nil
	case "GetLayer":
		if c.v == c.s.root() {
			return []any{uint32(layerWindow)}, nil
		}
		return []any{uint32(layerWidget)}, nil
	case "GetMDIZOrder":
		return []any{int16(0)}, nil
	case "GetAlpha":
		return []any{1.0}, nil
	case "GrabFocus":
		return []any{c.perform(platform.AccessActionFocus)}, nil
	case "ScrollTo":
		return []any{c.perform(platform.AccessActionScrollIntoView)}, nil
	case "ScrollToPoint", "SetExtents", "SetPosition", "SetSize":
		return []any{false}, nil
	}
	return nil, unknownMethod(ifaceComponent, name)
}

// --- org.a11y.atspi.Action ---

func (c *call) actionIface(name string) ([]any, error) {
	acts := nodeActions(c.v.n)
	if name == "GetActions" {
		out := make([]actionInfo, len(acts))
		for i, a := range acts {
			out[i] = actionInfo{a.description, a.description, ""}
		}
		return []any{out}, nil
	}
	i, err := c.int(0)
	if err != nil {
		return nil, err
	}
	var a nodeAction
	if i >= 0 && i < len(acts) {
		a = acts[i]
	}
	switch name {
	case "GetName":
		return []any{a.name}, nil
	case "GetLocalizedName", "GetDescription":
		return []any{a.description}, nil
	case "GetKeyBinding":
		return []any{""}, nil
	case "DoAction":
		if a.action == 0 {
			return []any{false}, nil
		}
		return []any{c.request(platform.AccessActionEvent{Action: a.action})}, nil
	}
	return nil, unknownMethod(ifaceAction, name)
}

// --- org.a11y.atspi.Value (properties only) ---

func (c *call) value(name string) ([]any, error) {
	return nil, unknownMethod(ifaceValue, name)
}

// rangeValues returns the current, minimum and maximum values and the
// minimum increment of range widget n. Tela does not know a widget's
// step: it is 1 for integral ranges, else a hundredth of the range.
func rangeValues(n *platform.AccessNode) (cur, lo, hi, step float64) {
	r := n.Range
	if r.HasNow {
		cur = r.Now
	}
	if r.HasMin {
		lo = r.Min
	}
	hi = lo
	if r.HasMax {
		hi = r.Max
	} else if n.Role == "progressbar" || n.Role == "meter" || n.Role == "scrollbar" {
		hi = 100
	}
	step = 1
	if !integral(cur) || !integral(lo) || !integral(hi) {
		step = (hi - lo) / 100
	}
	return cur, lo, hi, step
}

func integral(v float64) bool { return v == float64(int64(v)) }

// --- org.a11y.atspi.Text ---

// caretOffset is the caret of node x's text (count characters): a text
// field's selection focus, -1 when there is none (AccessKit's answer
// for text without a caret).
func caretOffset(x *vnode, count int) int {
	if !x.n.HasSelection {
		return -1
	}
	return min(max(x.n.Selection.Focus, 0), count)
}

func (c *call) text(name string) ([]any, error) {
	rs := []rune(c.v.text)
	n := len(rs)
	seg := func(start, end int) []any {
		return []any{string(rs[start:end]), int32(start), int32(end)}
	}
	switch name {
	case "GetText":
		a, err := c.ints(2)
		if err != nil {
			return nil, err
		}
		s, e := clampRange(rs, a[0], a[1])
		return []any{string(rs[s:e])}, nil
	case "GetStringAtOffset":
		a, err := c.ints(2)
		if err != nil {
			return nil, err
		}
		return seg(stringAt(rs, a[0], uint32(a[1]))), nil
	case "GetTextAtOffset", "GetTextBeforeOffset", "GetTextAfterOffset":
		a, err := c.ints(2)
		if err != nil {
			return nil, err
		}
		which := map[string]int{"GetTextAtOffset": 0, "GetTextBeforeOffset": -1, "GetTextAfterOffset": 1}[name]
		return seg(segment(rs, a[0], uint32(a[1]), which)), nil
	case "GetCharacterAtOffset":
		o, err := c.int(0)
		if err != nil {
			return nil, err
		}
		if o < 0 || o >= n {
			return []any{int32(0)}, nil
		}
		return []any{int32(rs[o])}, nil
	case "SetCaretOffset":
		o, err := c.int(0)
		if err != nil {
			return nil, err
		}
		o = min(max(o, 0), n)
		return []any{c.setSelection(o, o)}, nil
	case "GetNSelections":
		if s, e, ok := c.textSelection(); ok && s != e {
			return []any{int32(1)}, nil
		}
		return []any{int32(0)}, nil
	case "GetSelection":
		if s, e, ok := c.textSelection(); ok && s != e {
			return []any{int32(s), int32(e)}, nil
		}
		return []any{int32(0), int32(0)}, nil
	case "AddSelection":
		a, err := c.ints(2)
		if err != nil {
			return nil, err
		}
		return []any{c.setSelection(clampOffset(a[0], n), clampOffset(a[1], n))}, nil
	case "SetSelection":
		a, err := c.ints(3)
		if err != nil {
			return nil, err
		}
		if a[0] != 0 {
			return []any{false}, nil
		}
		return []any{c.setSelection(clampOffset(a[1], n), clampOffset(a[2], n))}, nil
	case "RemoveSelection":
		_, _, ok := c.textSelection()
		if !ok {
			return []any{false}, nil
		}
		f := clampOffset(c.v.n.Selection.Focus, n)
		return []any{c.setSelection(f, f)}, nil
	case "GetAttributes", "GetAttributeRun":
		return []any{map[string]string{}, int32(0), int32(n)}, nil
	case "GetAttributeValue":
		return []any{""}, nil
	case "GetDefaultAttributes", "GetDefaultAttributeSet":
		return []any{map[string]string{}}, nil
	case "GetCharacterExtents":
		a, err := c.ints(2)
		if err != nil {
			return nil, err
		}
		r := charRect(c.v.n.Bounds, rs, a[0])
		x, y, w, h := c.s.extents(r, uint32(a[1]), c.parentNode())
		return []any{x, y, w, h}, nil
	case "GetRangeExtents":
		a, err := c.ints(3)
		if err != nil {
			return nil, err
		}
		s, e := clampRange(rs, a[0], a[1])
		r := charRect(c.v.n.Bounds, rs, s)
		if e > s {
			r = union(r, charRect(c.v.n.Bounds, rs, e-1))
		}
		x, y, w, h := c.s.extents(r, uint32(a[2]), c.parentNode())
		return []any{x, y, w, h}, nil
	case "GetOffsetAtPoint":
		a, err := c.ints(3)
		if err != nil {
			return nil, err
		}
		px, py := c.s.point(int32(a[0]), int32(a[1]), uint32(a[2]), c.parentNode())
		return []any{int32(offsetAt(c.v.n.Bounds, rs, px, py))}, nil
	case "GetBoundedRanges":
		return []any{[]boundedRange{}}, nil
	case "ScrollSubstringTo":
		return []any{c.perform(platform.AccessActionScrollIntoView)}, nil
	case "ScrollSubstringToPoint":
		return []any{false}, nil
	}
	return nil, unknownMethod(ifaceText, name)
}

func clampOffset(o, n int) int { return min(max(o, 0), n) }

// textSelection returns the ordered selection of a text field.
func (c *call) textSelection() (start, end int, ok bool) {
	if !c.v.n.HasSelection {
		return 0, 0, false
	}
	n := len([]rune(c.v.text))
	a, f := clampOffset(c.v.n.Selection.Anchor, n), clampOffset(c.v.n.Selection.Focus, n)
	return min(a, f), max(a, f), true
}

func (c *call) setSelection(anchor, focus int) bool {
	if !c.v.n.Actions.Has(platform.AccessActionSetSelection) {
		return false
	}
	return c.request(platform.AccessActionEvent{Action: platform.AccessActionSetSelection,
		Selection: platform.AccessSelection{Anchor: anchor, Focus: focus}})
}

// charRect approximates the box of character o of text inside bounds:
// the tree holds no glyph positions, so lines (split at line feeds) share
// the height and characters a line's width evenly.
func charRect(bounds platform.Rect, text []rune, o int) platform.Rect {
	lines, line, col, longest := 1, 0, 0, 0
	cur := 0
	for i, r := range text {
		if i == o {
			line, col = lines-1, cur
		}
		if r == '\n' {
			longest = max(longest, cur)
			lines++
			cur = 0
			continue
		}
		cur++
	}
	longest = max(longest, cur, 1)
	if o >= len(text) {
		line, col = lines-1, cur
	}
	w, h := bounds.W/float32(longest), bounds.H/float32(lines)
	return platform.Rect{X: bounds.X + float32(col)*w, Y: bounds.Y + float32(line)*h, W: w, H: h}
}

// offsetAt inverts charRect: the character at the point, -1 outside.
func offsetAt(bounds platform.Rect, text []rune, x, y float32) int {
	if !contains(bounds, x, y) {
		return -1
	}
	for i := range text {
		if contains(charRect(bounds, text, i), x, y) {
			return i
		}
	}
	return len(text)
}

func union(a, b platform.Rect) platform.Rect {
	x0, y0 := min(a.X, b.X), min(a.Y, b.Y)
	x1, y1 := max(a.X+a.W, b.X+b.W), max(a.Y+a.H, b.Y+b.H)
	return platform.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// --- org.a11y.atspi.EditableText ---

func (c *call) editableText(name string) ([]any, error) {
	set := func(s string) []any {
		return []any{c.request(platform.AccessActionEvent{Action: platform.AccessActionSetValue, Value: s})}
	}
	rs := []rune(c.v.text)
	// A password field's Value is masked: only replacing it all works.
	masked := c.v.n.Has(platform.AccessProtected)
	switch name {
	case "SetTextContents":
		s, err := c.str(0)
		if err != nil {
			return nil, err
		}
		return set(s), nil
	case "InsertText":
		pos, err := c.int(0)
		if err != nil {
			return nil, err
		}
		s, err := c.str(1)
		if err != nil {
			return nil, err
		}
		length, err := c.int(2)
		if err != nil {
			return nil, err
		}
		if masked {
			return []any{false}, nil
		}
		ins := []rune(s)
		if length >= 0 && length < len(ins) {
			ins = ins[:length]
		}
		pos = clampOffset(pos, len(rs))
		return set(string(rs[:pos]) + string(ins) + string(rs[pos:])), nil
	case "DeleteText":
		a, err := c.ints(2)
		if err != nil {
			return nil, err
		}
		if masked {
			return []any{false}, nil
		}
		s, e := clampRange(rs, a[0], a[1])
		return set(string(rs[:s]) + string(rs[e:])), nil
	case "CopyText":
		return nil, nil
	case "CutText", "PasteText":
		return []any{false}, nil
	}
	return nil, unknownMethod(ifaceEditableText, name)
}

// --- org.a11y.atspi.Selection ---

// isSelected reports whether child x of a selection container is
// selected (checked, for a radiogroup's radios).
func isSelected(x *vnode) bool {
	if x.n.Selected == platform.ToggleTrue {
		return true
	}
	return x.n.Role == "radio" && x.n.Checked == platform.ToggleTrue
}

// selected returns the selected children of the container.
func (c *call) selected() []*vnode {
	var out []*vnode
	for _, id := range c.v.children {
		if x := c.s.node(id); x != nil && isSelected(x) {
			out = append(out, x)
		}
	}
	return out
}

func (c *call) selection(name string) ([]any, error) {
	i, err := c.int(0)
	if name == "SelectAll" || name == "ClearSelection" {
		// Tela has no select-all action for a container.
		return []any{false}, nil
	}
	if err != nil {
		return nil, err
	}
	child := func(i int) *vnode {
		if i < 0 || i >= len(c.v.children) {
			return nil
		}
		return c.s.node(c.v.children[i])
	}
	// click performs the default action of item x (which selects an
	// option, a tab or a radio; it toggles an item of a multi-selectable
	// container).
	click := func(x *vnode) bool {
		if x == nil || !x.n.Actions.Has(platform.AccessActionDefault) {
			return false
		}
		cc := *c
		cc.v = x
		return cc.request(platform.AccessActionEvent{Action: platform.AccessActionDefault})
	}
	multi := c.v.n.Has(platform.AccessMultiSelectable)
	switch name {
	case "GetSelectedChild":
		sel := c.selected()
		if i < 0 || i >= len(sel) {
			return []any{c.b.nullRef()}, nil
		}
		return []any{c.b.ref(c.w, sel[i])}, nil
	case "SelectChild":
		x := child(i)
		if x != nil && isSelected(x) {
			return []any{true}, nil
		}
		return []any{click(x)}, nil
	case "DeselectSelectedChild":
		sel := c.selected()
		if !multi || i < 0 || i >= len(sel) {
			return []any{false}, nil
		}
		return []any{click(sel[i])}, nil
	case "DeselectChild":
		x := child(i)
		if !multi || x == nil || !isSelected(x) {
			return []any{false}, nil
		}
		return []any{click(x)}, nil
	case "IsChildSelected":
		x := child(i)
		return []any{x != nil && isSelected(x)}, nil
	}
	return nil, unknownMethod(ifaceSelection, name)
}

// --- org.a11y.atspi.Cache ---

// cache answers GetItems with nothing: ATs query the objects themselves
// (AccessKit does not serve a cache either).
func (c *call) cache(name string) ([]any, error) {
	if name == "GetItems" {
		return []any{[]cacheItem{}}, nil
	}
	return nil, unknownMethod(ifaceCache, name)
}

// --- org.freedesktop.DBus.Introspectable ---

func (c *call) introspect() string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">` + "\n<node>\n")
	for _, i := range append(c.interfaces(), propsIface, introspectIface) {
		fmt.Fprintf(&b, "  <interface name=%q/>\n", i)
	}
	b.WriteString("</node>\n")
	return b.String()
}
