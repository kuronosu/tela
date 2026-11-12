//go:build linux

// Package atspi is the accessibility bridge of the Linux backends
// (platform contract v5, ADR-246): AT-SPI2 over D-Bus with
// github.com/godbus/dbus/v5, so that screen readers (Orca) read, navigate
// and operate Tela windows. The Wayland and X11 backends share it; the
// model is AccessKit's (platforms/atspi-common and platforms/unix): the
// bridge keeps its own copy of each window's tree, answers the AT's
// queries from it on godbus goroutines, and sends the AT's requests to
// the UI thread as events.
//
// # Activation
//
// One Bridge per process, shared by its windows; the first window starts
// it (New does no I/O). Its worker goroutine connects to the session bus
// and reads org.a11y.Bus's org.a11y.Status properties IsEnabled and
// ScreenReaderEnabled (missing ones are false), then follows their
// PropertiesChanged and the bus launcher's NameOwnerChanged; nothing
// polls, so an app with no AT costs one idle connection (0% CPU). When
// either property is true it connects to the accessibility bus
// (org.a11y.Bus.GetAddress, or $AT_SPI_BUS_ADDRESS, which also skips the
// session bus), serves the objects below on that connection, embeds the
// application in the registry (org.a11y.atspi.Socket.Embed on
// org.a11y.atspi.Registry, which answers with the desktop, our parent),
// reads the events ATs listen to (Registry.GetRegisteredEvents, then
// EventListenerRegistered and Deregistered), and reports
// platform.AccessibilityEvent{Active: true} to every window. When both
// properties turn false it unembeds, closes the accessibility connection
// and reports Active false; a registry that restarts is embedded again.
// NO_AT_BRIDGE=1 (at-spi2-atk's switch) disables the bridge.
//
// # Threads
//
// The Window methods, Dispatch and Close run on the UI thread. Update
// applies the update to the window's platform.AccessTree there (a
// persistent tree: the cost follows the update, ADR-277) and hands the new
// tree, with the update's changed and removed IDs, to the worker, which
// updates the exposed view from them (view.go; in full for the first tree),
// publishes it in an atomic snapshot and sends the events of the exposed
// nodes it touched (events.go); the UI thread never waits for D-Bus. godbus
// runs each method call on a goroutine of its own, which reads the
// snapshot (object.go). Actions (DoAction, GrabFocus, SetTextContents...)
// are queued with the generation of the activation they belong to and
// moved to the loop by Dispatch (Options.Event), which drops those of an
// earlier activation; the call answers at once ("requested"), as the
// contract says, and the outcome arrives as the next update.
//
// # Objects
//
// The application is /org/a11y/atspi/accessible/root (role application,
// interfaces Accessible and Application; the registry sets its Id). Each
// window is a frame child, /org/a11y/atspi/accessible/wN, named by its
// title (or the root's name), active while the window has the keyboard
// focus. Node ID of window N's tree is /org/a11y/atspi/accessible/wN_ID.
// Every node has Accessible (name, description, parent, children, index,
// role and role name, states, attributes, relations controller-for and
// controlled-by from aria-controls, application, interfaces, locale) and
// Component (extents in screen, window and parent coordinates, in
// physical pixels; Contains, GetAccessibleAtPoint, GrabFocus, ScrollTo).
// Then, by what the node is:
//
//   - Action: the default action ("click"), expand or collapse,
//     increment, decrement, "show-context-menu", as the node lists them.
//   - Value: range widgets; setting CurrentValue is one increment or
//     decrement step toward it (Tela's widgets take steps, not values).
//   - Text: text fields (their value, the caret and selection from the
//     field), texts and the elements that fold their text (view.go);
//     granularities in text.go. Character extents are estimates (the tree
//     has no glyph positions); text attributes are empty.
//   - EditableText: editable fields: SetTextContents, InsertText and
//     DeleteText become a set-value action (not on a password field,
//     whose value is masked, except SetTextContents); cut and paste are
//     not supported.
//   - Selection: listbox, tablist, tree, treegrid, grid, menu, menubar,
//     radiogroup over their direct children; SelectChild performs the
//     child's default action; SelectAll and ClearSelection are not
//     supported.
//
// Roles follow W3C Core-AAM 1.2 (roles.go); a protected field is a
// password text. A query on a node that is gone answers
// org.freedesktop.DBus.Error.UnknownObject, except GetState, which
// answers the defunct state. The Cache interface answers no items.
//
// # Announcements
//
// The live region announcements core computes go out as the
// org.a11y.atspi.Event.Object Announcement signal on the frame
// (at-spi2-core 2.46, Orca 45 and later), with the politeness as detail1;
// the nodes carry the container-live attributes too.
package atspi
