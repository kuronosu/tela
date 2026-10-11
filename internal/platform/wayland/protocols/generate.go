// Package protocols holds the Go bindings of the Wayland protocols Tela
// uses, generated from the XML files in xml/ by internal/platform/wayland/wlgen.
//
// The vendored protocol versions are wayland 1.26.0 (wayland.xml) and
// wayland-protocols 1.49 (xdg-shell, viewporter, fractional-scale-v1,
// xdg-decoration-unstable-v1, cursor-shape-v1, tablet-v2 and
// text-input-unstable-v3). To add a protocol, copy its XML into xml/ and run
//
//	go generate ./internal/platform/wayland/...
//
// All protocols live in one package because they reference each other
// (xdg_shell uses wl_surface, cursor_shape_v1 uses tablet_v2). Names are the
// protocol names in CamelCase: wl_surface becomes WlSurface, its set_title
// request SetTitle, its enter event WlSurfaceEnterEvent with the handler
// setter SetEnterHandler, and its enums WlSurfaceError and friends.
//
// A connection is set up with wire.Dial, then NewWlDisplay(conn).GetRegistry().
// Globals are bound with WlRegistryBind. The files are only built on Linux.
package protocols

//go:generate go run ../wlgen -out . xml
