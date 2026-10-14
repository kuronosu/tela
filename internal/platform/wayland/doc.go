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
//   - wp_cursor_shape_manager_v1 (optional): cursors. Without it the cursor
//     is left to the compositor; loading an xcursor theme is a later task.
//   - zxdg_decoration_manager_v1 (optional): server-side decorations are
//     requested when present. Client-side decorations are not drawn yet.
//   - wl_data_device_manager (optional, version up to 3): the clipboard
//     (clipboard.go). Without it Clipboard reports an error.
//   - zwp_text_input_manager_v3 (optional, version 1): the IME
//     (textinput.go). Without it SetTextInput only records the state and
//     no IME events arrive (an IME is then reachable only through the
//     keys it forwards).
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
// Phase 1 supports integer scales only: wl_surface.preferred_buffer_scale
// when the compositor is version 6+, otherwise the largest scale of the
// outputs the surface is on, applied with wl_surface.set_buffer_scale.
// Fractional scaling (wp_fractional_scale_v1 + wp_viewporter) arrives in
// phase 6; scaleFor in output.go is the one place that decides the scale.
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
// Every file but this one is only built on Linux.
package wayland
