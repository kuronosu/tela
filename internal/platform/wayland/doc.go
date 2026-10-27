// Package wayland is Tela's Wayland platform backend: a pure-Go client of the
// compositor that implements the platform contract (see package platform)
// over the wire runtime and the generated protocol bindings.
//
// # What it uses
//
//   - wl_compositor (version 4+ for damage_buffer, 6 for
//     preferred_buffer_scale), wl_shm (memfd buffers), xdg_wm_base and
//     xdg_toplevel: required.
//   - wl_seat + wl_pointer: pointer input, grouped by wl_pointer.frame.
//   - wl_seat + wl_keyboard: key input. The compositor's XKB keymap is
//     parsed by internal/xkb (pure Go, no libxkbcommon); dead keys and
//     the Compose key use the user's or locale's Compose file; key repeat
//     is client-side (keyboard.go). Pointer and scroll events carry the
//     seat's keyboard modifiers.
//   - wl_output: integer scale fallback when the compositor has no
//     wl_surface.preferred_buffer_scale.
//   - wp_fractional_scale_manager_v1 and wp_viewporter (optional, version
//     1, used only together): fractional scales (see Scale).
//   - wp_cursor_shape_manager_v1 (optional): cursors. Without it the cursor
//     is left to the compositor; loading an xcursor theme is a later task.
//   - zxdg_decoration_manager_v1 (optional): server-side decorations are
//     requested for decorated windows, client-side ones for frameless
//     windows. Without it, or when the compositor picks client-side,
//     the window reports ClientSideDecorations (see Window control).
//   - wl_data_device_manager (optional, version up to 3): the clipboard
//     (clipboard.go). Without it Clipboard reports an error.
//   - zwp_text_input_manager_v3 (optional, version 1): the IME
//     (textinput.go). Without it SetTextInput only records the state and
//     no IME events arrive (an IME is then reachable only through the
//     keys it forwards).
//   - Window control, all optional (windowctl.go, see Window control):
//     xdg_activation_v1 (Activate), xdg_wm_dialog_v1 (modal windows),
//     xdg_toplevel_icon_manager_v1 (SetIcon) and xdg_session_manager_v1
//     (RestoreToken), version 1, from wayland-protocols 1.49 (staging).
//
// # Loop
//
// Run is a single-threaded poll loop on the UI goroutine: deliver queued
// events, deliver due frames, pace the frame requests that are not due yet,
// flush requests, then poll(2) on the display socket and an eventfd (Wake,
// Quit) with no timeout when nothing is due. Protocol callbacks never call
// the app: they update state and queue events, which the loop delivers after
// Dispatch returns, so the handler is never called re-entrantly. While a
// key is held, its next repeat is one more deadline of the same poll
// timeout, and clipboard transfers add their pipes to the same poll(2).
// Nothing runs while idle (0% CPU).
//
// The queue, the delivery pass and the frame-due rule are the shared
// internal/loop. What is specific to Wayland is the display signal: frame
// callbacks gate requested frames (WindowState.VsyncRequested / VsyncDone),
// and the loop's Pace hook (Window.paceFrame) requests one for a frame that
// is not due yet.
//
// # Frame pacing
//
// Frames follow wl_surface.frame callbacks, so the compositor paces them to
// the display and stops them while the window is hidden. Present requests a
// callback with the buffer it commits. A requested frame is due when no
// callback is outstanding and either a callback fired since the last
// FrameEvent or the window has not had a FrameEvent for one interval
// (1/60 s; the "not drawn recently" fast path).
//
// A FrameEvent handler may skip Present (an animation tick that changes no
// pixel) and request the next frame. Then no callback is outstanding, and
// delivering the request on the next iteration would spin the loop at 100%
// CPU. Instead the backend requests a frame callback with a commit that
// attaches no buffer: the surface keeps its current buffer and state, and
// the FrameEvent arrives with the callback's done, at vsync. Frame
// callbacks are double-buffered surface state, so a commit without a new
// buffer is valid and only applies the callback (plus any pending size
// hints, which are harmless to apply early). Gio, by contrast, requests
// the callback before drawing and relies on the buffer commit of the draw
// to carry it. This needs a buffer on screen (a
// surface without one is not mapped and gets no callbacks) and no configure
// waiting for the buffer that acks it; in those cases requests are paced by
// a 1/60 s timer in the poll timeout instead, like the X11 backend.
//
// # Scale
//
// scaleFor in output.go is the one place that decides the scale, in this
// order: the surface's wp_fractional_scale_v1 preferred_scale (n/120,
// computed as float64(n)/120), wl_surface.preferred_buffer_scale
// (wl_compositor 6+), the largest scale of the outputs the surface is on,
// and before any of them the largest output scale (the initial guess).
// Once a fractional scale arrived, the integer sources are ignored, like
// winit does: compositors send them rounded up (2 for 1.5) for older
// clients.
//
// With both wp_fractional_scale_manager_v1 and wp_viewporter, each window
// gets a wp_fractional_scale_v1 and a wp_viewport when it is created
// (before its initial commit, so a scale sent at once is not missed), and
// every scale, integer or not, is applied the same way: the buffer is
// PhysicalSize(logical, scale) (logical × scale rounded half away from
// zero, the protocol's rule), wl_surface.set_buffer_scale stays 1, and the
// viewport's destination is the logical size. The destination is
// double-buffered like the buffer and is set right before the commit of
// the first buffer of a new size, so the surface never shows a buffer at
// the wrong size (without a destination it would be the buffer's physical
// size). Without either global the backend uses integer scales and
// set_buffer_scale, as in phase 1.
//
// A scale change keeps the logical size: it is announced with a
// ConfigureEvent (the app redraws at the new physical size) and needs no
// ack. A scale may arrive before the first configure (the first
// ConfigureEvent carries it), after it, or while a resize waits for its
// frame; in every case the next buffer has the latest size and scale and
// acks the latest configure. Damage stays in buffer pixels
// (damage_buffer); on a wl_compositor older than 4, wl_surface.damage is
// mapped to surface pixels by the ratio of the logical and buffer sizes,
// rounded outward. Pointer positions and the text-input cursor rectangle
// are surface-local, which with a viewport are logical pixels: nothing
// else changes. Cursors come from wp_cursor_shape_v1, which the compositor
// draws at the output's scale (there is no client cursor surface).
//
// # Clipboard
//
// Clipboard is the wl_data_device selection of the seat whose keyboard
// focuses one of our windows (else the first seat). There is no primary
// selection (middle-click paste) and no drag and drop: offers of a drag
// over our windows are destroyed unused, so the source sees no target.
//
// WriteText creates a wl_data_source offering text/plain;charset=utf-8,
// text/plain, UTF8_STRING, TEXT and STRING (ISO 8859-1, '?' for other
// characters; the rest UTF-8), plus a private type that names the source,
// and calls set_selection with the seat's newest input serial (pointer
// enter, leave or button; keyboard enter, leave or key). Compositors
// accept it only from the client with keyboard focus, and only with a
// serial not older than the current selection's; a refused request is
// silent. The source answers wl_data_source.send until it is cancelled
// (another client took the selection) or replaced by the next WriteText.
//
// ReadText answers our own selection from memory without a round trip,
// as GTK does. A selection event whose offer carries the private type of
// our latest source confirms that the compositor accepted it; any other
// selection event (another client's offer, or none) ends our ownership, so
// after a refused WriteText the next selection event (at the latest, the
// one before the next keyboard enter) restores the real owner. Another
// client's selection is read by wl_data_offer.receive into a pipe, picking
// text/plain;charset=utf-8, then UTF8_STRING, text/plain, TEXT, STRING.
// Declared UTF-8 replaces invalid sequences with U+FFFD; the other types
// are UTF-8 when valid, ISO 8859-1 otherwise; trailing NULs are dropped.
// An empty selection or one without text gives "" and no error.
//
// Transfers never block the UI thread: the pipes are non-blocking and the
// poll loop does their I/O (no goroutines; done is posted to the loop
// queue). A transfer that makes no progress for 5 s ends: a read with an
// error, a write by closing the pipe. Text over 64 MiB is refused. When
// Run returns, transfers in progress are dropped without calling done.
//
// Keyboard focus: the compositor announces the selection only to the
// focused client, right before keyboard enter and whenever it changes
// while focused. A window without focus therefore reads the selection as
// it was at its last focus (which may be stale, or empty if its owner has
// gone), and its WriteText is likely refused. Text editing copies and
// pastes from key and pointer handlers, which run with focus.
//
// # Text input and IME
//
// Each seat gets one zwp_text_input_v3. It follows the seat's keyboard
// focus (enter, leave). Once it has entered a window whose app set a text
// input state, the loop sends enable, the state (surrounding text,
// content type, cursor rectangle) and commit; later changes send only the
// parts that changed, with one commit per loop iteration at most, and
// nothing while the state stays equal. A nil state sends disable and
// commit; a new field (nil then a state) or a new content type restarts
// with disable, commit, enable, state, commit. IMESessionEvents follow the
// enabled state; leave ends the session without requests.
//
// preedit_string, commit_string and delete_surrounding_text are latched
// and delivered as one IMEEvent at done, when they change something. The
// serial of done is compared with our commit count, but a stale done is
// applied like a current one (the protocol's rule; see textinput.go for
// why the state is never held back). Surrounding text longer than the
// protocol's 4000 bytes is cut to a window around the selection.
//
// # Window control
//
// Requests are fire-and-forget; the compositor's xdg_toplevel.configure
// states are the outcome. The configure that ends a sequence reports
// maximized, fullscreen and client-side decorations with the loop's
// UpdateState: after the first ConfigureEvent, and before the
// ConfigureEvent of the same configure afterwards (maximizing changes
// both). A request equal to the last one asked (or to the latest
// configure) sends nothing.
//
//   - Minimize is set_minimized; Wayland cannot report or undo it
//     (State.Minimized stays false; Activate brings the window back).
//     SetAlwaysOnTop and SetPosition do nothing: there is no protocol.
//   - SetMaximized and SetFullscreen (no output: the compositor picks)
//     map to set_/unset_maximized and set_/unset_fullscreen; Maximized and
//     Fullscreen options are asked before the initial commit.
//   - Size limits are set_min_size/set_max_size, committed at once when
//     the window is shown (they are double-buffered); a maximum below the
//     minimum is sent as the minimum (xdg_toplevel rejects the opposite).
//     A floating window outside new limits picks the clamped size, like
//     SetSize; imposed (maximized, fullscreen, tiled) sizes are kept.
//   - SetVisible(false) commits a null buffer, which unmaps the toplevel
//     and discards its attributes (xdg-shell). SetVisible(true) sends
//     them again (title, app id, size limits, parent, maximized and
//     fullscreen requests, decoration mode, modality, icon) with a new
//     initial commit; at its configure, a window of the size it had is
//     mapped with its retained buffer (the swapchain's newest), otherwise
//     the app's ConfigureEvent and next Present map it. Present while
//     hidden, or before that configure, draws nothing. A window that was
//     never mapped (Hidden, or hidden before its first Present) only
//     resumes frames. Frame callbacks pending at the unmap are not waited
//     for. Unmapping clears the activated state (no configure follows).
//   - Activate asks xdg_activation_v1 for a token with the latest input
//     serial and its seat (none before any input, like winit), the
//     focused window of ours as the requesting surface (else the window)
//     and the app id, then activates with it. Focus stealing prevention
//     may turn it into an attention request (sway marks the window
//     urgent). Hidden windows are not activated.
//   - SetIcon sends every image as a square ARGB8888 wl_shm buffer at
//     scale 1 (non-square images centered), then destroys the icon and
//     its buffers right after set_icon, which the protocol allows.
//   - Parent is set_parent, sent only while the parent is mapped (an
//     unmapped parent counts as none, and unmapping drops the relation):
//     at creation, when the parent maps (again), and when the child is
//     shown again. Modal windows get an xdg_dialog_v1 with set_modal.
//     Close closes the children first, in creation order.
//
// Client-side decorations. A frameless window, or any window when the
// compositor has no xdg-decoration or configured the client-side mode,
// has resize edges while mapped and neither maximized, fullscreen nor
// FixedSize: a band of 5 logical pixels inside each edge, with corners
// reaching 16 pixels along the edges (Chromium's Linux frame sizes), left
// out along the edges the window is tiled against (GTK). The pointer
// shows the matching resize cursor there (over the app's SetCursor), and a
// press there starts xdg_toplevel.resize (primary button) and is not
// delivered. The whole surface is the window (no shadow is drawn outside
// it), so the default window geometry is exact and set_window_geometry is
// not sent.
//
// Hit regions. A press in a HitCaption region (with no app button held)
// is the compositor's, as is the rest of its sequence up to the release of
// the last button (motion included; a pointer leave ends it silently).
// Primary: a double click (the DoubleClickInterval setting, 400 ms
// without one, within 4 logical pixels) toggles maximize; otherwise
// xdg_toplevel.move starts with the press serial once the pointer has
// moved 8 logical pixels, GTK's drag threshold (GtkWindowHandle does the
// same), so a click that does not move never starts a compositor grab.
// Secondary: show_window_menu at the pointer. Other regions, the window
// button regions included, are the app's.
//
// Session restore. With xdg_session_manager_v1 the backend opens one
// xdg_session_v1 per process at the first NewWindow (reason launch; the
// session id of that window's restore token, or a new session), and adds
// every window before its initial commit: restore_toplevel for a token of
// this session (the compositor then restores the window and its first
// configure carries the size), add_toplevel with a random 128-bit name
// otherwise. A name is given to one toplevel per process at most (a window
// reopened in the same run gets a new name): name_in_use is a protocol
// error, and whether a closed toplevel's name still counts is not
// specified. RestoreToken is "wayland-session-v1:" + the session id in
// unpadded base64url + ":" + the name, "" until created or restored
// arrives and after replaced (the session is destroyed then). Tokens of
// any other form, or with an id the compositor would reject, are ignored.
// At exit the session is destroyed, not removed, so the next run can
// restore from it.
//
// Every file but this one is only built on Linux.
package wayland
