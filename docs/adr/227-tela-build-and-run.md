# ADR-227: `tela build` and `tela run` make a development bundle

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-224.

## Decision

`tela build [-o path] [-id ID] [-name NAME] [-tags] [-ldflags] [-gcflags] [-trimpath] [-v] [pkg]` builds a package main with `CGO_ENABLED=0` for the environment's `GOOS`/`GOARCH`. For darwin it writes `<Name>.app` with `Contents/Info.plist` and `Contents/MacOS/<Name>` (`Contents/Resources` only when the packaging phase adds files); elsewhere the plain executable. `-o` is a folder for the bundle or a path ending in `.app`.

The Info.plist has `CFBundleIdentifier` (`App.AppID`), `CFBundleName`/`CFBundleDisplayName` (`App.Title`, else the package name), `CFBundleExecutable`, `CFBundlePackageType` APPL, the module's numeric version (else 0.0.0), `LSMinimumSystemVersion` 13.0, `NSHighResolutionCapable` and `NSSupportsAutomaticGraphicsSwitching`. The ID and title come from the `tela.App` literal that `tela.Main` receives in `main` (looking through wrapping calls such as `desktop.SingleInstance(App)`), else the package-level `var App`, as constant strings read with `go/parser` and `go/types` (no code runs); `-id` and `-name` override. No ID is an error that says to set `AppID` or pass `-id`; the ID must be letters, digits, hyphens and periods.

`tela run [flags] [pkg [args...]]` builds the bundle in `<user cache>/tela/run/<AppID>/<Name>.app` (a stable folder per ID; other `.app` there are removed) and runs the executable inside it directly, with stdin, stdout, stderr and Ctrl+C, forwarding the exit status. Elsewhere it builds in a temporary folder and runs the program, like `go run`. It refuses a target that is not the machine's.

Rebuilding writes only what changed (Info.plist bytes, executable contents, by temp file and rename) and keeps Info.plist keys it does not manage, so registration can edit them in place. `internal/plist` (a leaf, ADR-189) reads and writes the XML property list; the bundle writer is `cmd/tela/internal/appbundle`, for the packaging phase to extend.

**Signing:** nothing runs `codesign`. The Go linker ad-hoc signs darwin/arm64 executables; that signature covers the Mach-O only, not Info.plist or resources. The bundle is for the machine that built it.

## Why

The bundle is what makes `NSBundle.mainBundle` the app on macOS; a stable path keeps Launch Services seeing one app.
