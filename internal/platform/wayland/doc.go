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
// events, deliver due frames, flush requests, then poll(2) on the display
// socket and an eventfd (Wake, Quit) with no timeout when nothing is due.
// Protocol callbacks never call the app: they update state and queue events,
// which the loop delivers after Dispatch returns, so the handler is never
// called re-entrantly. Nothing runs while idle (0% CPU).
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
