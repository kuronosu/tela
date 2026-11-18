# ADR-295: `tela package`

- **Date:** 2026-11-17
- **Status:** Accepted. Amends ADR-127 and ADR-227.

## Decision

- **`tela package [-format list] [-os list] [-arch list] [-o dir] [-sign] [pkg]`** builds and packages for each target from one machine (the owner, 2026-11-17). Formats, written in pure Go: Linux `appimage` (the type 2 layout: a squashfs image written by Tela after Tela's own pure-Go runtime, since the AppImage project's runtime links libfuse3, LGPL; the runtime extracts the payload into the user cache under a lock, checks its integrity, execs it and removes old versions, with no FUSE; the owner, 2026-11-17), `deb`, `flatpak` (a manifest and the build directory; the `.flatpak` bundle only when `flatpak-builder` is installed); Windows `msix` (zip, `AppxManifest.xml`, block map) with an `.appinstaller` file for its updates, and `zip` (the portable program that updates itself); macOS `app` (the bundle in a `.zip`), `dmg` only through `hdiutil` on macOS. No MSI.
- **Metadata** from the declaration (ADR-294): `.desktop` entry and AppStream metainfo, `Info.plist` with schemes, document types and usage strings and the entitlements, the MSIX manifest with capabilities, protocols and file types, the toast activator (ADR-297).
- **Signing hooks:** `-sign` runs the OS tools when installed (`codesign` with the hardened runtime and `notarytool` on macOS, `signtool` on Windows) with identities from flags; `-sign-cmd` runs any command on each file to sign. Unsigned outputs are still written, and say so.
- **Windows targets are GUI programs** (`-H=windowsgui`) in `tela build` and `tela package`; `-console` keeps the console. Logs and panics go to files (ADR-297).
- Output names: `<name>-<version>-<os>-<arch>.<ext>`, deterministic bytes for the same inputs (sorted entries, fixed times from the commit).

## Why

Packaging every platform from one Linux machine without tools; the formats left to OS tools are the ones that cannot be written cheaply.
