# ADR-227: `tela build` and `tela run` make a development bundle

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-224.

## Decision

- **`tela build [-o path] [-id] [-name] [pkg]`** builds a package main with `CGO_ENABLED=0` for the environment's target; for darwin it writes `<Name>.app` (`Contents/Info.plist`, `Contents/MacOS/<Name>`), elsewhere the executable.
- **The Info.plist** holds the bundle ID (`App.AppID`), the name (`App.Title`), the module's version and macOS 13. ID and title are read statically from the `tela.App` that `main` passes to `tela.Main` (through wrapping calls), else `var App`; flags override; no ID is an error.
- **`tela run`** builds the bundle in `<user cache>/tela/run/<AppID>/` and runs its executable directly (stdio, signals, exit status); elsewhere it is `go run`.
- **Rebuilds** write only what changed and keep keys they do not manage, so registration can edit the Info.plist (`internal/plist`, `cmd/tela/internal/appbundle`).
- **Signing:** nothing runs `codesign`; the linker's ad-hoc signature covers the executable only.

## Rationale

The bundle makes `NSBundle.mainBundle` the app; a stable folder outside the temporary one is what Launch Services registers.
