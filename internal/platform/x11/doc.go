//go:build linux

// Package x11 is the X11 platform backend, the Linux fallback when no
// Wayland compositor is available. It speaks the core X11 protocol through
// github.com/jezek/xgb (pure Go) and implements the platform contract
// documented in package platform.
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
