# virtuallist

A directory of 100,000 people in one `h.VirtualList`: an avatar with initials, a name and a row number, a role and a city, and on one row in seven a note that wraps. The header's buttons jump to the first row, row 1,000, row 50,000 (centered) and the last row.

What it shows:

- **Only the rows in view are built.** `VirtualList(Count, estimate, renderRow, ...)` builds the rows that intersect the list's viewport plus five on each side (`h.Overscan`); about 20 of the 100,000 exist at any time, so mounting the list costs a few milliseconds. The list's content is as tall as all the rows together, so the scrollbar, Page Down, End and `OnScroll`'s `ScrollHeight` behave as if every row were there.
- **Rows are components with comparable props.** `renderRow` returns `Row{Index: i}`, a struct component whose fields are its props: scrolling past the built rows builds only the rows that enter and unmounts the ones that leave, and a scroll that stays inside them only repaints. A row's state, if it had any, would follow its index.
- **Variable heights, measured on the fly.** `estimate` says 65 px for a row and 110 px for a row with a note; a note's real height depends on how it wraps at the window's width. Each row is measured when it is laid out, and when a row above the top of the viewport turns out taller or shorter than its estimate the list moves its scroll offset by the difference in the same frame, so the rows on screen never jump (scroll up from row 50,000 to see it).
- **`ElementRef.ScrollToIndex`.** The buttons call `list.ScrollToIndex(i, align)` with `tela.AlignStart`, `AlignCenter` and `AlignEnd`; the list keeps the row aligned while the rows around it are measured, so "Row 50,000" ends exactly centered and "Last" exactly at the bottom. `AlignAuto` scrolls only when the row is not fully visible.
- **Semantics.** The list has `Role("list")` and `AriaLabel("People")`, each row `Role("listitem")`; the aria snapshot (`tela inspect`) shows the built rows.
- **Dark mode.** Every color comes in a pair; render it with `--dark`.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/virtuallist` (1280x800). Scroll with the wheel, the touchpad, the scrollbar or the keys (Page Down, End, Home; nothing needs to be focused).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 [--dark] -o virtuallist.png ./examples/virtuallist`.
- Tests: `CGO_ENABLED=0 go test ./examples/virtuallist`. They check that few rows are built, the jump buttons (centered, top and bottom alignment), wheel, Page Down, End and Home, that taller rows are measured and stay contiguous at 500 px, keyboard focus on the buttons, the aria snapshot (`testdata/virtuallist.aria.yml`) and the goldens (`testdata/virtuallist_x1.png`, `virtuallist_x2.png`, and `virtuallist_jump_dark_x1.png` centered on row 50,000; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run 'TestGolden|TestAria' ./examples/virtuallist`).

The data is generated from the row index (`people.go`), so it costs nothing until a row is shown; a real app would page it in from a store.
