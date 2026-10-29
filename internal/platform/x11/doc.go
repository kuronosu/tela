//go:build linux

// Package x11 is the X11 platform backend, the Linux fallback when no
// Wayland compositor is available. It speaks the X11 protocol through a
// vendored copy of github.com/jezek/xgb (pure Go; internal/xgb, with its
// core, xproto and bigreq packages, ADR-025) and implements the platform
// contract documented in package platform. The vendored reader is patched
// to read GenericEvents whole and to deliver the events and errors it
// cannot decode (XInput2, XKB) raw instead of dropping them; the XInput2
// and XKB requests this package needs are encoded by hand (ext.go,
// xi2.go, xkbext.go).
//
// # Input
//
// Pointer: XInput2 2.1 or later when the server has it (xi2.go: motion,
// buttons, enter/leave of the master pointers, and smooth scrolling from
// scroll valuators: Delta for touchpads, fractional Steps for wheels),
// the core pointer events otherwise. Both go through the same helpers
// (events.go), including the implicit-grab rules for leave.
//
// Keyboard (keyboard.go): core KeyPress/KeyRelease translated with a
// keymap built from the core keyboard mapping (xkb.NewCoreKeymap) and the
// same internal/xkb lookups, Compose and keyboard.Held as the Wayland
// backend, so KeyEvents follow the same contract. The XKB extension
// (xkbext.go) is used for state only: detectable auto-repeat, the group
// in core event states, StateNotify for the modifiers after a key, and
// MapNotify/NewKeyboardNotify to rebuild the keymap. Pointer and scroll
// Mods come from the real modifier mapping (Alt and Super are where their
// keys are bound; AltGr is never Alt).
//
// # Window control
//
// windowctl.go implements the phase 12 requests (ADR-178, ADR-179) with
// ICCCM and EWMH:
//
//   - A withdrawn window (not mapped yet, or hidden with SetVisible) is
//     not managed, so requests only update the wanted state, which is
//     written right before every MapWindow: _NET_WM_STATE (maximized,
//     fullscreen, above, modal), WM_HINTS initial state (Iconic after a
//     Minimize), the position (WM_NORMAL_HINTS USPosition and the
//     window's own x, y). The window manager removes _NET_WM_STATE when it
//     withdraws a window, so hide/show keeps the state this way, as GTK
//     does. Hiding is UnmapWindow plus the synthetic UnmapNotify of ICCCM
//     4.1.4.
//   - A managed window gets client messages to the root window:
//     _NET_WM_STATE add/remove (source indication 1), WM_CHANGE_STATE
//     IconicState for Minimize, _NET_ACTIVE_WINDOW with the X time of the
//     latest key or button press and the app's focused window. An
//     iconified window comes back by MapWindow (ICCCM), which Activate,
//     SetMaximized and SetFullscreen do. Requests equal to the wanted
//     state, which follows what the window manager reports, are free.
//   - State is read back at every PropertyNotify of _NET_WM_STATE (only
//     when _NET_SUPPORTED lists it: maximized needs both MAXIMIZED_VERT
//     and _HORZ, as in GTK; fullscreen; HIDDEN is minimized) and WM_STATE
//     (Iconic is minimized), and queued with internal/loop UpdateState.
//     Minimized or unmapped windows get no frames.
//   - Positions are the content area's root coordinates. WM_NORMAL_HINTS
//     always has StaticGravity, so positions given to the window manager
//     are the client window's. They are reported from ConfigureNotify
//     while the parent is the root, and with one TranslateCoordinates per
//     batch once a reparenting window manager put the window in a frame
//     (xgb drops the SendEvent bit, so synthetic events cannot be told
//     apart; GTK translates the same way).
//   - Size limits are PMinSize/PMaxSize in WM_NORMAL_HINTS (a zero
//     component is 1 or 32767); the icon is _NET_WM_ICON with every
//     size, sent as one BIG-REQUESTS ChangeProperty (or Replace plus
//     Appends) because xgb's request has a 16-bit length.
//   - Frameless windows set _MOTIF_WM_HINTS without decorations and report
//     ClientSideDecorations. Unless maximized, fullscreen or FixedSize, a
//     band of 6 logical pixels along their edges (16 along each edge from
//     a corner resizes diagonally) shows the resize cursor and a press
//     there starts _NET_WM_MOVERESIZE with the edge.
//   - Hit regions: a press of any button in a HitCaption region, when the
//     app holds no button, is the OS's: primary starts _NET_WM_MOVERESIZE
//     move (a second one within content's double-click rule,
//     platform.CaptionClicks: 500 ms, as the backend does not read
//     XSETTINGS yet, and 5 logical pixels, toggles maximize), secondary sends
//     _GTK_SHOW_WINDOW_MENU (mutter, KWin, Xfwm4, Openbox) when
//     _NET_SUPPORTED lists it. Before a move or menu request the implicit
//     grab is released with UngrabPointer and XIUngrabDevice (an XI2
//     press holds an XI2 grab, which the core request does not release).
//     The rest of the sequence is not delivered; the window manager's
//     grab yields a plain PointerLeave (no cancel: the app saw no press),
//     and buttons the server no longer reports held end the sequence.
//     Without _NET_WM_MOVERESIZE nothing moves, but the press is still
//     not delivered.
//   - Parents are WM_TRANSIENT_FOR; modal children also have
//     _NET_WM_WINDOW_TYPE_DIALOG and _NET_WM_STATE_MODAL. Close closes the
//     children first. RestoreToken is "": X11 restores by position.
//
// The requests and the two queries go through a small interface
// (wmServer), so unit tests check them without a server.
//
// # Loop
//
// xgb runs its own reader goroutine and only exposes events through a
// blocking WaitForEvent, so the UI thread cannot poll the socket itself. A
// pump goroutine calls WaitForEvent and forwards every event on a channel.
// Run blocks in a select over that channel, a notify channel (buffer 1,
// non-blocking sends from Wake and Quit) and a frame timer that is armed only
// while a frame is requested but not yet due. Nothing ticks while the app is
// idle (0% CPU): the pump is parked in a channel receive and xgb's reader in
// a socket read.
//
// Each loop iteration drains every event already available, translates
// them (coalescing consecutive pointer motion and repeated ConfigureNotify),
// answers Expose and _NET_WM_PING without involving the app, and then
// delivers, in order: a WakeEvent if Wake was called, the queued events,
// and the FrameEvents that are due. The queue, the coalescing primitives,
// the delivery pass and the pacing deadline are the shared internal/loop;
// this package only reads X events and arms the timer.
//
// # Frames
//
// The core protocol has no frame callbacks (the Present extension is a
// later optimization), so frames are paced by a timer at a fixed 60 Hz: a
// requested frame is delivered on the next iteration if the window has not
// had a frame in the last interval, otherwise when the interval elapses.
// A ConfigureEvent's frame is delivered immediately.
//
// # Sizes and scale
//
// X11 sizes are physical pixels. The scale is Xft.dpi / 96, read from the
// RESOURCE_MANAGER property of the root window (the X resource database set
// by xrdb, GTK and Qt settings daemons), and 1 when Xft.dpi is not set.
// Logical sizes are platform.LogicalSize(physical, scale). A change of
// Xft.dpi at runtime (PropertyNotify on the root window) re-delivers a
// ConfigureEvent with the new scale to every window.
//
// # Present
//
// Present converts the damaged pixels to the server's 32-bpp ZPixmap layout
// (B, G, R, X bytes: depth 24 TrueColor with LSBFirst image byte order,
// checked by New) into a retained copy, and sends them with PutImage
// requests split to the maximum request length (BIG-REQUESTS is enabled
// when the server has it). The retained copy answers Expose. A window is
// mapped at its first Present; the pixels of that Present reach the screen
// through the Expose that follows the map. MIT-SHM is a later optimization.
package x11
