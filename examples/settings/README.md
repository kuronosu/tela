# settings

A preferences page: a Notifications card with five switches, an Appearance card with a theme picker (three cards with a miniature of each theme) and a density list (radio-like rows), and a footer with Reset and Save changes buttons.

What it shows:

- **One comparable struct as state.** The whole form is a `Prefs` value in `UseState`; `prefs != saved` is the dirty flag. The footer says "You have unsaved changes" or "All changes saved", and Reset and Save are `Disabled` until there is something to save.
- **Setters and Callbacks in props.** The switches and option cards receive the form's `tela.Setter[Prefs]`; the footer receives two `tela.Callback` handles made with `UseCallback`. All are comparable, so `tela.CP` skips the components whose own props did not change: toggling one switch builds that row and the footer, not the other rows or the Appearance card (`TestRebuildsAreNarrow`).
- **Interaction states.** `Switch` is a button with `hover:`, `active:` and `transition-colors`; "Security alerts" is a locked switch (`Disabled`) with `disabled:opacity-50 disabled:cursor-not-allowed`, and the footer buttons use `disabled:` too. The cursor follows `cursor-*` and `disabled:cursor-not-allowed`.
- **Structural variants.** The rows of the notifications card separate themselves with `border-t first:border-t-0`.
- **Dark mode.** Every color comes in a pair; render it with `--dark`.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/settings` (1280x800).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 [--dark] -o settings.png ./examples/settings`.
- Tests: `CGO_ENABLED=0 go test ./examples/settings`. They drive the page through a headless window: toggling a switch (track, knob and dirty state), hover and press on a switch, the locked switch, Save and Reset, the two radio groups, and the goldens (`testdata/settings_x1.png`, `settings_x2.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/settings`).

At widths below about 900 px the two cards stack; a window shorter than the content scrolls like a page.
