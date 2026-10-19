# layout

Tela's layout animations (ARCHITECTURE section 12, ADR-081) on one screen:

- **A task list** whose rows have `h.AnimateLayout`: Rotate moves the last task to the top and Reverse reverses the list; every row is laid out at its new place at once and drawn sliding there on a spring. Pressing Reverse while the rotation runs starts the new motion from where the rows are drawn, at their speed.
- **A tab bar** whose underline is rendered by the selected tab with `h.LayoutID("tab-underline")`: when another tab is selected, the old underline unmounts and the new one mounts in the same commit, so the new one starts from the old one's box and slides (and stretches) to its tab.
- **An expanding card** with `h.AnimateLayout`: its size change is drawn as a scale toward the new size, with its rounded corners and shadow corrected; its header and details have `AnimateLayout` too, so they keep their own size instead of stretching, and the card's `overflow-hidden` reveals the details as it grows. The note below the card follows its bottom edge.

Run it in a window: `CGO_ENABLED=0 go run ./examples/layout`.

`layout_test.go` captures frames with `telatest.NoSettle` (the clock moves only with `App.Advance`): `testdata/rotate_50ms.png`, `rotate_100ms.png`, `reverse_80ms.png` (the reversal interrupting the rotation), `tabs_40ms.png`, `tabs_90ms.png`, `expand_30ms.png`, `expand_80ms.png` and `collapse_40ms.png`, plus the screens at rest. Regenerate them with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/layout` and look at them before committing.
