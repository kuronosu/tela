# native

A file explorer that looks native on Windows 11 (Fluent), GNOME (libadwaita) and KDE (Breeze), each in light and dark. One component tree draws all six looks.

<p align="center">
  <img src="../../docs/images/native-win11.png" alt="Windows 11" width="32%">
  <img src="../../docs/images/native-adwaita.png" alt="GNOME" width="32%">
  <img src="../../docs/images/native-breeze.png" alt="KDE" width="32%">
</p>

What it shows:

- **A theme is data, not a tree.** `Theme` (`theme.go`, with `Win11`, `Adwaita` and `Breeze` in `theme_*.go`) holds a `Look`: one class string for each part of the window (toolbar, path bar, sidebar item, selected row, status bar...), split into a `Base` (geometry and type) and the `Light` and `Dark` colors appended to it. Colors are arbitrary values (`bg-[#cce8ff]`, `text-black/55`); radii, heights and gaps come from the same strings. Besides classes a theme says which columns show and in what order (Windows: name, date, type, size; GNOME: name, size, type, modified; KDE: name, size, date, type), how dates and sizes read (`10/9/2026 2:22 PM` and `58 KB`; `Fri` and `59.4 kB`; `2026-10-09 14:22` and `58.0 KiB`), what the status bar says and which sidebar sections exist. The tree in `explorer.go` never asks which platform it is.
- **Light, dark and auto.** `Light` and `Dark` pick one color set. `Auto` writes both, the dark one as `dark:` classes, so the window follows the system's setting (and a settings change) with no code. `TestAutoFollowsTheSystem` checks that Auto draws the same pixels as the forced modes.
- **Icons without assets.** Folders, dog-eared file pages (with a mark per kind), the sidebar glyphs and the toolbar glyphs are boxes: borders, rotated squares for chevrons, rotated squares clipped by `overflow-hidden` for triangles. Glyphs use `bg-current` / `border-current`, so they take the text color of their button and its hover and disabled states. (`icons.go`)
- **Real interaction.** Click selects, double click enters a folder, the sidebar, the path bar's crumbs and Back / Forward / Up navigate, typing in the search field filters the folder, a column header sorts (twice reverses), and the list takes the keyboard: Up, Down, Home, End, Enter (open), Backspace (back); Alt+Left, Alt+Right and Alt+Up anywhere, Ctrl+F for the search. The state is one `UseReducer` (`state.go`) over an in-memory file tree (`fs.go`).
- **Fonts.** Each theme asks for its platform's font first (`font-['Segoe_UI_Variable_Text','Segoe_UI',Inter,sans-serif]`, Adwaita Sans / Cantarell, Noto Sans) and Inter after; a window uses the system font when installed, and snapshots and goldens always get Inter.
- **Header bar.** On GNOME the toolbar is a libadwaita header bar drawn inside the client area, with a lead over the sidebar. The OS title bar stays; Tela does not draw window controls or move the window by its header bar.

## Run it

```bash
CGO_ENABLED=0 go run ./examples/native                         # win11 on Windows, adwaita elsewhere; light or dark as the system says
CGO_ENABLED=0 go run ./examples/native -theme=breeze -dark=true
```

`-theme=auto|win11|adwaita|breeze` and `-dark=auto|true|false`.

Without a window, one of the six previews:

```bash
CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x760 --scale 2 --preview "1 Windows 11 light" -o native.png ./examples/native
```

Tests: `CGO_ENABLED=0 go test ./examples/native`. They mount every theme in both schemes: goldens at 1280x760 (`testdata/native_<theme>_<light|dark>.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run 'TestGolden' ./examples/native`), the Auto/Light/Dark equivalence, and the interactions (select, navigation, sidebar and path bar, search, keyboard, sort, hover and selection colors).

## Limits

- `dark:` follows the system, but package `tela` exposes no way to read whether the system is dark, so the example cannot pick a different class set in Go; it writes the dark values as `dark:` twins instead.
- The look is an approximation drawn from memory of each platform: colors, radii and metrics are close, not measured. Window controls, native menus, context menus, drag and drop, icon theme fidelity and platform fonts are not reproduced.
