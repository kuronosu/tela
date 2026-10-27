//go:build linux

// Package freedesktop implements the desktop services of the Linux
// backends (platform.Desktop, ADR-178, ADR-179) over the D-Bus session
// bus with github.com/godbus/dbus/v5. The Wayland and X11 backends share
// it; they differ only in the parent window identifier they give portal
// dialogs.
//
// # Services
//
//   - OpenURI, OpenPath: the XDG Desktop Portal org.freedesktop.portal.OpenURI
//     interface (OpenURI; OpenFile with an O_PATH file descriptor, which
//     opens folders in the file manager too), falling back to running
//     xdg-open when there is no portal (or no session bus at all).
//   - OpenFiles, SaveFile, PickFolder: org.freedesktop.portal.FileChooser
//     (OpenFile with multiple/directory/filters, SaveFile with
//     current_name/current_folder). The answer is the Response signal of
//     the portal's Request object, subscribed to before the call at the
//     predictable handle path, as the portal documentation requires.
//   - MessageBox: unsupported (the portal has none; core draws one).
//   - Notify, CloseNotification: org.freedesktop.Notifications (actions,
//     the "default" action for a click on the notification, the icon as the
//     image-data hint). ActionInvoked and NotificationClosed become
//     NotificationEvents; the token of ActivationToken is kept for the
//     backend (TakeActivationToken).
//   - NewTray, UpdateTray, RemoveTray: a StatusNotifierItem per tray, on its
//     own connection under the well-known name
//     org.kde.StatusNotifierItem-<pid>-<id>, registered with
//     org.kde.StatusNotifierWatcher, with a com.canonical.dbusmenu menu.
//     The item has no unregister method: removing a tray closes its
//     connection, and the watcher drops the item when its name vanishes
//     (which is why every tray has its own connection). Trays are registered
//     again when a new watcher appears (the panel restarted).
//
// # Threading
//
// The platform.Desktop methods, Dispatch and Close run on the UI thread.
// godbus delivers replies, signals and incoming method calls on its own
// goroutines; they never call the app. They put callbacks and events in an
// inbox guarded by a mutex and call Options.Wake, and the backend's loop
// calls Dispatch, which moves them to the loop (Options.Post and
// Options.Event) on the UI thread. So every answer arrives through the loop,
// never from inside the call that asked for it. The tray state that
// incoming calls read (icon, tooltip, menu layout) is an immutable snapshot
// replaced under the tray's lock.
//
// # Laziness
//
// New does no I/O. The session bus is connected on the first call that
// needs it, so an app that never uses a service makes no D-Bus traffic. A
// missing bus or service is platform.ErrUnsupported, synchronously;
// presence checks ask the bus daemon (NameHasOwner, the activatable names)
// and, for portal interfaces, read their version property once.
package freedesktop
