# board

A small kanban board: three columns (To do, In progress, Done) of cards with a title, a note, a tag, a comment count and an assignee. Cards can be dragged to another column, dropped between two cards, or reordered inside their column.

What it shows:

- **Drag with `OnDragStart` / `OnDrag` / `OnDragEnd`.** The dragged card follows the pointer with `tela.TranslateFn` reading a `tela.UseValue` (`DragState.Offset`, set from `e.OffsetX/OffsetY`), so a pointer move only repaints that card: `TestCardFollowsThePointer` checks 0 builds and 0 layouts over ten moves.
- **Lifting.** While dragged the card is `relative z-50 rotate-2 shadow-2xl ring-2` and `pointer-events-none`: it paints above the other columns, and the pointer reaches the column and the card under it.
- **Drop targets by pointer boundaries.** Columns and cards set `DragState.Over` from `OnMouseEnter` / `OnMouseLeave` while a drag is active; the column under the pointer gets a ring and the card it hovers gets an insertion line (an absolutely positioned bar, so nothing shifts under the pointer). On `OnDragEnd` the board moves the card to the last target (`Board.Move`); releasing outside any column puts it back.
- **A comparable board.** Columns are fixed-size arrays of card IDs, so the `Board` is a comparable value in `UseState`, and the `tela.CP` lanes and cards skip their build when their props do not change.
- **`tela.Post` for ordering.** The drop runs inside `tela.Post`, which shares one queue with `Value` updates, so it sees the `Over` set by the last move even if no frame ran in between.
- **Dark mode.** Every color comes in a pair; render it with `--dark`.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/board` (1280x800).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 [--dark] -o board.png ./examples/board`.
- Tests: `CGO_ENABLED=0 go test ./examples/board`. They cover the model (`Board.Move`), dragging to another column, between cards, inside a column and outside the board, the drop-target ring, a click not moving anything, and the goldens (`testdata/board_x1.png`, `board_x2.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/board`).

A column holds at most eight cards; a window shorter than the columns scrolls like a page.
