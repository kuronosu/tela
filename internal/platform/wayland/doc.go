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
// timeout. Nothing runs while idle (0% CPU).
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
// Every file but this one is only built on Linux.
package wayland
