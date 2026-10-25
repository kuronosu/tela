# native

A file explorer that looks native on Windows 11 (Fluent), GNOME (libadwaita) and KDE (Breeze), each in light and dark, built only on package `kit`. One component tree draws all six looks, and each look is a set of kit recipes.

<p align="center">
  <img src="../../docs/images/native-win11.png" alt="Windows 11" width="32%">
  <img src="../../docs/images/native-adwaita.png" alt="GNOME" width="32%">
  <img src="../../docs/images/native-breeze.png" alt="KDE" width="32%">
</p>

What it shows:

- **A look is the three levels of kit's customization (ADR-129, K17).** `Look` (`look.go`, with `win11.go`, `adwaita.go` and `breeze.go`) holds: a `tela.Theme` (the tokens: colors with a light and a dark value, the base radius, the font), a `kit.Style` (`win11Style()`, `adwaitaStyle()`, `breezeStyle()`, each an edit of `nativeStyle()`, which is `kit.DefaultStyle()` made flat), and a `Frame`, the classes of the few regions the explorer lays out itself (the toolbar row, the sidebar, the details pane, the status bar). Switching look (More menu, "Look") swaps the `kit.StyleProvider`'s style and calls `tela.SetTheme`; nothing in the tree asks which platform it is. There is no `dark:` class anywhere: the dark values are in the tokens (`bg-background`, `text-muted-foreground`, and the example's own `bg-selection`, `bg-field`, `text-kind-pdf`...).
- **What is kit.** Toolbar buttons are `kit.Button{Variant: kit.Ghost, Size: kit.Icon}` with a `kit.Tooltip`; the crumbs are small ghost buttons; the search field is a `kit.Input`; the sidebar is one `kit.ToggleGroup` per section (so the selected place is the group's choice and the arrows move through it); the view switcher is a `kit.ToggleGroup`, and the tiles of the icon view are `kit.Toggle`s; the file list is a `kit.Table` (sortable columns, sticky header, `aria-sort`) inside a `kit.ContextMenu`; the Sort, View, New and More menus are `kit.DropdownMenu`s with radio and checkbox items (the Look group is one); Rename is a `kit.Dialog` and Delete a `kit.AlertDialog`; KDE's zoom is a `kit.Slider`; the separators are `kit.Separator`. The new variants (`Sidebar`, `View`, `Tile`) and sizes of `kit.Toggle` are entries of each recipe's `Variants` and `Sizes`.
- **Icons are Lucide's** (`icons/`, 39 of them, generated with `tela icons gen`). A look does not draw them differently: it colors them with tokens (`text-folder`, `text-kind-doc`...), and a look that wants another shape picks another icon.
- **What is the example's own.** The regions of the window (they are layout, not controls), the column cells of the list, the details pane, the status text, the sidebar's selection mark and the rows' content. The few handlers a table cannot take (a row has no props) are on the cell's content.
- **Real interaction.** Click selects, double click enters a folder, the sidebar, the crumbs and Back / Forward / Up navigate, typing filters the folder, a header sorts (three presses: ascending, descending, back to the name), and the list takes the keyboard: Up, Down, Home, End, Enter (open), Backspace (back), F2 (rename), Delete, Ctrl+X, C and V; Alt+Left, Alt+Right and Alt+Up anywhere, Ctrl+F for the search. New folder, Rename, Delete (with confirmation), Cut, Copy and Paste really change the in-memory file tree (`fs.go`; the tree is persistent, so the state is one `UseReducer`, `state.go`).
- **Fonts.** Each look asks for its platform's font (`Segoe UI Variable Text`, `Adwaita Sans`, `Noto Sans`) in the theme; the embedded Inter is the fallback, and goldens always get Inter.
- **Header bar.** On GNOME the toolbar is a libadwaita header bar drawn inside the client area, with a lead over the sidebar (sidebar toggle and main menu). The OS title bar stays; Tela does not draw window controls.

## Run it

```bash
CGO_ENABLED=0 go run ./examples/native                         # win11 on Windows, adwaita elsewhere; light or dark as the system says
CGO_ENABLED=0 go run ./examples/native -theme=breeze -dark=true
```

`-theme=auto|win11|adwaita|breeze` and `-dark=auto|true|false`; the menu changes the look while it runs.

Without a window, one of the six previews. `tela snapshot` renders in one shot and takes the theme of `var App`, which starts as the platform's look or the one `NATIVE_LOOK` names, so name it:

```bash
NATIVE_LOOK=breeze CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x760 --scale 2 --preview "5 KDE light" -o native.png ./examples/native
```

Tests: `CGO_ENABLED=0 go test ./examples/native`. They mount every look in both schemes with its theme: goldens at 1280x760 and at scale 2 (`testdata/native_<look>_<light|dark>[_x2].png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/native`), the icon view, the context menu and the two dialogs per look, that following the system draws the same pixels as the forced schemes, switching look at run time, and the interactions (select, navigation, sidebar and crumbs, search, keyboard, sort, rename, delete, clipboard, new folder, views, menus, tooltips, hover and selection colors).

## Limits

- The look is an approximation drawn from memory of each platform: colors, radii and metrics are close, not measured. Window controls, native menus, drag and drop and icon theme fidelity are not reproduced; icons are Lucide's outlines, not each platform's.
- A table has no row element, so a row's selection and hover are painted by its cells (which is why Win11's and Adwaita's rounded rows are `first:rounded-l` / `last:rounded-r` on the cells), and columns have no resize handle.
- The headless test window has no right button: the context menu is opened with Shift+F10 in the tests.
- `kit.Input` takes one prop and no `AriaLabel`, so the search field is named by its placeholder.
