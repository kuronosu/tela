// Package remote is a platform backend that runs over IPC: an app (the
// child) uses a [Backend] whose windows live in another process (the
// host), which shows them on the windows of a real backend through a
// [Host]. It is the transport of the dev loop, tela dev (ADR-111): the
// supervisor owns the real window and never restarts, and each rebuild of
// the app is a new child that takes over that window without a visible
// close and reopen. The protocol is a general "remote app" protocol
// (ADR-112): it carries window IDs, so several windows can be mapped, and
// an opaque control channel, so later tools (tela mcp, DevTools) can
// reuse it; only what tela dev needs is implemented.
//
// Dev-loop code only: nothing in a release build imports this package
// (ADR-111). It imports only package platform, its internal packages, the
// standard library and golang.org/x/sys.
//
// # Transport
//
// An AF_UNIX stream socket (Go supports AF_UNIX on Windows 10 and later),
// at a path in $XDG_RUNTIME_DIR or the temporary directory
// ([DefaultAddr]). Messages are length-prefixed frames with a
// hand-written little-endian encoding (protocol.go), opened by a version
// handshake: the child's hello names the protocol version, its process ID
// and a tag; the host's welcome carries its clock and system settings, or
// a rejection. Each side writes from a goroutine fed by a queue (senders
// never block on the socket) and reads on another.
//
// Pixels never go through the socket. Each child window has a shared
// memory segment per framebuffer size (a memfd on Linux and an unlinked
// temporary file on macOS, passed with SCM_RIGHTS; a named file mapping
// on Windows), split into two frame slots; Present copies only the damage
// into a free slot, and the host copies the same damage out into its
// retained frame and releases the slot (see childwindow.go for the
// buffering rules that keep the host from showing a torn frame).
//
// # Child side
//
// [Dial] (or [DialEnv], reading $TELA_REMOTE) returns a Backend that
// implements the platform contract (package platform's documentation) on
// top of the host's backend:
//
//   - NewWindow asks the host for a window with the next ID (1, 2, ...)
//     and every option (the parent as its ID); [Backend.NewKeyedWindow]
//     adds a key. On a swap the host gives the new child's window the
//     real window the old child had under the same key (by ID without
//     one), if it was opened with the same creation-only options
//     (frameless, fixed size, modal, parent); it keeps its size, position
//     and maximized or fullscreen state. Real windows the new child does
//     not take over close at the promotion (ADR-184).
//   - Events of the sealed set are forwarded by the host: ConfigureEvent,
//     WindowStateEvent, FrameEvent (the real window's, paced by its
//     display), focus, close requests, pointer, scroll, key, IME and IME
//     session events, SettingsEvent, and the app-scoped NotificationEvent
//     and TrayEvent (to the active child). WakeEvent, Wake and Quit are
//     local. Event times and FrameEvent.Time are the host's clock, and
//     [Backend.Now] equals it: the handshake measures the offset between
//     the two processes' monotonic clocks (to within half a round trip).
//   - RequestFrame, SetTitle, SetCursor, SetSize, SetTextInput,
//     ResetTextInput, Close (children first) and every window control
//     request (windowctl.go) are forwarded; RestoreToken is the real
//     window's, which the host sends. Clipboard reads are an asynchronous
//     round trip (done runs on the child's UI thread), writes are
//     forwarded. Desktop services are the host's (services.go): calls are
//     round trips on the control channel, dialogs answer later on the UI
//     thread, and tray icons and notifications are held by the host for
//     its children so a swap neither duplicates nor drops them.
//   - Run returns an error when the connection is lost.
//
// The child's loop is internal/loop's (queue, coalesced wake, quit,
// re-entrancy guard), without its frame pacing: the windows are not
// registered with it, since FrameEvents come from the host.
//
// # Host side
//
// [NewHost] listens for children; the Host is the platform.Handler of the
// real backend (it must see every event: run the backend with
// [Host.Run], or pass every event to [Host.HandleEvent]). It works on the
// backend's UI thread; its goroutines (accept, and a reader and a writer
// per child) hand work to it with Wake. [Host.Post] runs a supervisor's
// code there.
//
// Children are active, pending or detached. The first child that connects
// while none is active and input is not frozen becomes active: its frames
// are shown and it receives input. Others are pending: they get their
// windows' ConfigureEvents (the real metrics), FrameEvents when they ask,
// focus changes and settings, and render offscreen; their frames are kept,
// not shown. A swap is:
//
//  1. [Host.Freeze]: input to the real windows is queued from now on.
//  2. [Child.Request] to the active child, for its state (the kinds and
//     payloads belong to the layers above).
//  3. Start the next child; it connects as pending and renders; wait for
//     [Child.FirstFrame] (or for its loop to run with every window
//     hidden).
//  4. [Host.Promote]: the new child's latest frame is presented in full,
//     its title, cursor, text input state and window state requests are
//     applied, its tray icons replace the old child's, it gets the queued
//     input in order, and the old child is detached (the caller stops
//     it). [Host.Unfreeze] instead resumes the old child.
//
// A pending child's actions on the windows (minimize, maximize,
// fullscreen, position, activate) are dropped, so a starting build never
// moves the window under the shown one; the real window's state reaches
// it as WindowStateEvents, the latest one right after its ConfigureEvent.
//
// Each child window has a view of the real window (focus, held keys, IME
// session) through which every event passes, so a child that missed
// events while pending still receives a valid sequence: no release
// without its press, no IME event outside a session, no event before its
// ConfigureEvent.
//
// [HostOptions.Scale] forces the scale children render at (tela dev
// --scale): their ConfigureEvents carry it, with the real logical size,
// and the host resamples their frames to the real physical size with
// nearest-neighbor sampling (damage mapped exactly). Input coordinates
// are logical and need no change. [HostOptions.WindowOptions] rewrites the
// options of a new real window (--size), and [HostOptions.Settings] the
// system settings children see (--dark).
//
// [Host.SetOverlay] installs a hook that draws over each frame right
// before it is presented (tela dev's error overlay, drawn over the last
// good frame: a detached or disconnected child's last frame stays on its
// window). [HostOptions.Input] lets the supervisor consume input events
// (the clicks on that overlay's panel, Escape) before any child sees them;
// the pointer's hover in the child ends and resumes around them.
// [Host.Frame] returns a copy of a window's current frame.
//
// # Control channel
//
// Both sides send requests ([Child.Request], [Backend.Request]: a kind, a
// payload, one reply) and notifications ([Child.Notify],
// [Backend.Notify]) and register handlers ([Host.Handle],
// [Backend.Handle]) that run on their UI thread. This package gives the
// kinds no meaning, except the "tela.remote." kinds it reserves for the
// desktop services: saving and restoring state, panic reports and asset
// roots are defined by the layers above.
package remote
