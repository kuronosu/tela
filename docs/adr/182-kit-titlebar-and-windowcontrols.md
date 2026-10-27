# ADR-182: `kit.TitleBar` and `kit.WindowControls`

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Kit components for the custom title bars of ADR-178 and ADR-180 (phase 12; amends ADR-129's component list).

- **`kit.WindowControls`**: minimize, maximize (named and drawn as "Restore" while `UseWindow().State().Maximized`) and close, each a real `h.Button` with `h.WindowControl(...)`, so a click acts with no handler and the keyboard works. Fields: `Variant`, `HideMinimize`, `HideMaximize`, `State`, icon slots (`MinimizeIcon`, `MaximizeIcon`, `RestoreIcon`, `CloseIcon`), label fields (`MinimizeLabel`... `CloseLabel`, default English), `AriaLabel` of the group (role `group`, "Window controls"), `Class`, `Props`.
- **`kit.TitleBar`**: a flex or grid row with `h.WindowDrag()`; slots `Title`, `Leading`, `Children` (content beside the title), `Trailing` (all `tela.Component`), `ControlsSide` ("start"/"end"), `HideMinimize/HideMaximize`, `State`, part classes, `Class`, `Props`. Interactive slots are holes by themselves; drag, double click and menu are the OS's. It dims while `!Focused`, loses its outline while `Maximized || Fullscreen`, mirrors under `rtl` (the controls sit last in the row, first on macOS). It does not hide itself: an app shows it when `State().ClientSideDecorations` says so.
- **Looks** are `Variant`s: `kit.Windows` (32 px, left title, flat 46 px buttons, red close hover), `kit.GNOME` (47 px, centered title, round buttons), `kit.MacOS` (40 px, dots at the start, glyphs on hover). `Default` is chosen by `runtime.GOOS` (windows, darwin, else GNOME); an app adds looks to the recipes (`Style.TitleBar`, `Style.WindowControls`, per-look maps). Glyphs are SVGs inside `kit` (not Lucide).
- **`State *tela.WindowState`** overrides the window's state, for previews and tests: with no window (`tela snapshot`) the state is focused, not maximized.

## Rationale

The buttons must be real buttons (aria's `UseButton` on a div does not trigger window controls from the keyboard). Looks as variants reuse the recipe mechanism; the gallery and goldens name looks explicitly so they match on every OS.
