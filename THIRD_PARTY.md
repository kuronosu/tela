# Third-party code

Code ported from other projects and vendored data files. Ported files keep
their original copyright notice. Add one section per source project.

<!--
## <Project>

- **Source:** <URL> (<version, tag or commit>)
- **License:** <SPDX identifier>
- **Used in:** <repo paths>
- **Notes:** <what was ported or vendored, and how it diverges>
-->

## Wayland protocol XML

- **Source:** https://gitlab.freedesktop.org/wayland/wayland (wayland 1.26.0, `protocol/wayland.xml`) and https://gitlab.freedesktop.org/wayland/wayland-protocols (wayland-protocols 1.49)
- **License:** MIT (X11-style; each file carries its own `<copyright>` block)
- **Used in:** `internal/platform/wayland/protocols/xml/` (vendored unchanged), and the Go bindings generated from them in `internal/platform/wayland/protocols/*.go`
- **Notes:** Files: `wayland.xml`, `xdg-shell.xml`, `viewporter.xml`, `fractional-scale-v1.xml`, `xdg-decoration-unstable-v1.xml`, `cursor-shape-v1.xml`, `tablet-v2.xml` (needed by cursor-shape) and `text-input-unstable-v3.xml`. Every generated Go file reproduces its protocol's copyright text in its header, as `wayland-scanner` does.

## go-wayland

- **Source:** https://github.com/rajveermalviya/go-wayland (design reference, shallow clone of `main` at the time of writing)
- **License:** BSD-2-Clause, Copyright 2021 go-wayland authors
- **Used in:** `internal/platform/wayland/wlgen/` and `internal/platform/wayland/wire/`
- **Notes:** Design reference only (XML-driven generator, one typed proxy per interface, request/event split). No source was copied: the generator and the wire runtime are written from scratch against `wayland.dtd` and libwayland's wire format and differ in API (typed event structs and handler setters, descriptor tables, zombie handling, explicit non-blocking `Flush`/`Read`/`Dispatch`). Listed here as a courtesy to the project.
