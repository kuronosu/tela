# dashboard

"Frosted graphite": an analytics dashboard, dark first, in the manner of Apple's materials and Raycast. A calm dune wallpaper, three fixed panes of real glass over it (the rail, the main pane and the Pulse pane) and one amber accent. Only the content inside a pane scrolls, so what each blur samples never moves. It is the screen of the GPU backend's criterion: the GPU backend must draw it at 60 fps at scale 2, and the CPU rasterizer does not (see [Performance](#performance)).

| Dark | Light |
|---|---|
| ![dark](testdata/overview_dark_x1.png) | ![light](testdata/overview_light_x1.png) |
| ![board](testdata/board_dark.png) | ![palette](testdata/palette_light.png) |

## What it has

- **Overview** (`overview.go`). A headline with metric tabs (a `kit.Tabs` whose amber line slides between the tabs, a `LayoutID`), the number and a spectrum of 48 bars, each with its value in a tooltip; four figures with sparklines; the 30 latest orders in a `kit.Table` that scrolls in place, with a link to the orders view; and the Pulse pane: the monthly goal as a ring, the traffic, the activity feed and the weekly report, whose Edit button opens the report dialog. The period switch (7D, 30D, 90D) moves every number.
- **Tasks** (`tasks.go`, `board.go`, `actions.go`, `model.go`). One model, the board (three columns of task IDs, a comparable array), shown two ways. The table has a checkbox that finishes a task (its card moves to Done, and back) and columns that sort; finished tasks stay last, and every cell slides to its new place (`AnimateLayout`). The board's cards are dragged between columns, dropped between two cards or reordered in their column; a card's ellipsis (`kit.DropdownMenu`) and the card itself (`kit.ContextMenu`, Shift+F10 from the keyboard) have Move to (a full column is disabled), Edit and Delete; Edit, New task (the sprint strip and every column's +) and Delete open a `kit.Dialog` or a `kit.AlertDialog`, and answer with a toast. The sprint strip counts what is done with a bar that follows a spring.
- **Orders** (`overview.go`). 240 orders in a table that fills the pane, filtered by status.
- **The shell** (`shell.go`, `main.go`). The rail (a button per view, each with a tooltip and an amber bar that slides to the active one), the top bar (the view's own segmented control, the search button that opens the palette, the theme button, the notifications in a `kit.Popover`), the toasts (`kit.Toaster`), and Ctrl+K (the palette) and Ctrl+1, 2, 3 (the views).
- **The command palette and the tooltips** (`palette.go`) are built directly on `aria` hooks, with no kit component: the palette is an overlay (`UseOverlay`, modal) holding a dialog (`UseDialog`) around a combobox (`UseCombobox`) whose list is always open and filters the commands while the input keeps the focus (the arrows move a virtual focus, `aria-activedescendant`); a tooltip is `UseTooltipTrigger` placed by `UseFloating`. The commands navigate, open the report and new-task dialogs, export the orders (a toast) and switch the theme.

Everything else is package `kit` in a recipe of its own (`style.go`): the kit's theme with the graphite tokens, dark first (`pane`, `pane-border`, `menu` and `accent-text` besides shadcn/ui's names), and `kit.DefaultStyle()` edited (hairline cards, an amber primary button, the segmented control with a pill that slides, small uppercase tabs, the glass dialogs, nearly opaque menus) and applied with `kit.StyleProvider`. The numbers, the 48 bars, the 96 sparkline bars and the sprint bar follow springs read by deferred props (`tela.UseSpring` with `h.TextFn`, `tela.HeightFn`, and `kit.Progress{ValueFn}` for the sprint bar), so they move without building anything per frame; the segmented controls draw their pill with `kit.ToggleGroup`'s `Indicator`.

The previews: `1 Overview` (the peak's tooltip open, as in the goldens), `2 Board`, `3 Tasks`, `4 Command palette`, `5 Report dialog`, `6 Toast` (a toast is state set by an effect, which `tela snapshot` does not draw: `TestGoldenStates` shows it) and `7 Orders`. `img/gen.go` draws the two wallpapers from arithmetic alone.

## The glass budget

A `backdrop-blur` is computed again for every pixel that is painted again under it, and on the CPU that is the largest cost of the page (about 25 ns per blurred pixel on one core). The rasterizer keeps a filter's output while its input does not change (`raster.BackdropCache`, ADR-200), so only a change under the glass computes the blur again. The design is heavy on purpose, as the GPU backend's target, but scrolling and resting must stay cheap, so:

1. **Blur only where the backdrop is fixed.** The three panes blur the wallpaper, which never moves, and only their content scrolls. The damage of a frame stays what changed (a scrolled table, a bar that grows), and a pane at rest costs nothing.
2. **No `backdrop-blur` inside an element that is a layer** (opacity under 1, a blend mode, a filter, an animated transform): it turns off the parallel rasterization of the whole frame while it is on screen. The menus, the select's list, the notifications, the tooltips and the toasts animate in and out, so they use the nearly opaque `menu` fill and no blur; the dialogs and the palette blur and animate only their panel's entrance, a few frames.
3. **No fade over the whole window.** A scrim that fades repaints the whole window on every frame of it: the dialogs' scrim and the palette's appear at once. The palette's veil is a plain translucent scrim, not the blurred veil of the proposal: with `backdrop-blur-[3px]` under it, moving the highlight between two commands cost 31 ms at 1080p (81 ms at scale 2) instead of 24 (54).
4. **No transition on what a drag crosses.** A transition restyles the page on each of its frames, and a frame that restyles records every layer again; the cards and the columns change at once when the pointer crosses them, so a drag moves one layer and records little (`TestDragMoveRecordsLittle`).
5. **Tables build every row.** A `kit.Table` without `RowHeight` is not virtualized (ADR-199; this page sets none), and the overview lays out again on every frame of its springs: the overview shows the 30 latest orders, the orders view all 240 (240 rows on the overview made its pipeline five times slower).

The damage of a change under a pane also grows by the blur's reach (about 120 px on each side at `backdrop-blur-2xl`); content painted above the blur, such as a dragged card, does not grow it (ADR-200).

## Performance

`bench_test.go` drives the page without a window through a scripted session of 1,155 ticks of a 60 Hz clock: hover sweeps over the rail, the spectrum and the rows, a period change, a metric tab, scrolling and a fling in the latest orders, the report dialog and its toast, the palette, finishing and sorting a task, dragging a card across the board, and two theme switches. A frame is timed like `core.Host` runs it (input, animate, build/style/layout/paint, rasterization of the damage, the copy to a presentable buffer, effects) and runs only when the tree asked for one. After the session the page must ask for no frames at all.

```sh
TELA_PERF=1 CGO_ENABLED=0 go test -run TestSession -v ./examples/dashboard   # the full tables, one session per configuration (without TELA_PERF, 1080p at scale 1 only)
CGO_ENABLED=0 go test -run xxx -bench Session -benchtime 1x -count 3 ./examples/dashboard
CGO_ENABLED=0 go test -run xxx -bench 'FullFrame|ScrollFrame|PaletteFrame|ThemeFrame|ThemeSwitch' ./examples/dashboard
```

Medians of three runs on an i5-10400 (12 threads, so 12 raster workers) with a desktop running, frame time in ms (phase 24, 2026-11-11):

| Configuration | frames | p50 | p95 | p99 | max | over 16.7 ms |
|---|---|---|---|---|---|---|
| 1920x1080, scale 1 | 700 | 2.1 | 9.7 | 22.8 | 58.3 | 17 |
| 1920x1080, scale 2 (3840x2160 px) | 700 | 4.4 | 30.9 | 77.0 | 123.6 | 121 |
| 1440x900, scale 2 (2880x1800 px) | 673 | 4.7 | 26.7 | 54.0 | 91.7 | 92 |

Before phase 24 (v0.14.3, same machine and session, 1080p x1) the session ran 726 frames at p50 4.4 and p95 19.3 ms: the theme switch faded for ~28 frames, scrolls and the palette's scrim were full frames, and every frame that built, restyled or laid out placed and recorded the whole page. The pipeline (build, style, layout, paint) now has a p50 of 0.3 ms and a p95 of 1.4 ms at 1080p x1; the worst phase is always the rasterizer.

| Benchmark (ms per frame) | 1080p x1 | 1080p x2 | 1440x900 x2 |
|---|---|---|---|
| `BenchmarkFullFrame` (the whole window, forced) | 26.4 | 87.0 | 57.7 |
| `BenchmarkScrollFrame` (the orders table scrolling under the glass main pane) | 11.9 | 33.1 | 21.3 |
| `BenchmarkPaletteFrame` (the palette's first frame, its scrim frame) | 5.7, 6.0 | 16.6, 14.8 | |
| `BenchmarkThemeFrame` (raster of the switch after a rest; switched back within a second) | 27.9; 15.9 | 89.8; 62.0 | |

A scroll is drawn over a snapshot the previous frame took of the page under the moving rows, and a scrim or dialog over the framebuffer (ADR-275), so neither rasterizes the whole window. The theme switch is one frame (ADR-276) and still a full one here, because the dashboard's wallpaper image differs per scheme, so every pixel under the glass changes; switching back within a second reuses the blurred glass at scale 1, and a page whose background is the same in both schemes keeps its blur (core's `TestSchemeSwitchUnderGlass`). About 25 ms of the switch's pipeline is decoding the other scheme's wallpaper, which the image cache released while it was not shown (`BenchmarkThemeSwitch`: 0 boxes laid out, 0 paragraphs shaped). Per scene at 1080p, scale 1 (p50, p95 in ms): hover sweep 0.9, 1.1; period 2.4, 6.4; metric tab 2.0, 4.8; scroll and fling 2.7, 4.1; dialog and toast 1.3, 9.3; palette 0.4, 8.3; finish and sort 4.7, 20.0; board drag 6.2, 16.9; theme switch 57 (two frames); settle 0.6, 1.7. The frames that stay full are the view switches (the board, the palette's jump to Tasks), the board's lane highlight at drag start, the sinking row of finish and sort, the frame that removes a dialog's scrim, and the theme switch.

So the CPU path holds 60 fps at scale 1 for all but 17 of 700 frames; at scale 2 its median frame is 4.4 ms but one in six frames is over 16.7 ms: the gap the GPU backend has to close. The page at rest asks for no frames (`TestIdle`, `TestGoesIdle`, and the end of every session).

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/dashboard` (1440x900). `TELA_DEBUG=fps` shows the frame times.
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1440x900 --scale 2 [--dark] [--preview "4 Command palette"] -o dashboard.png ./examples/dashboard`.
- Tests: `CGO_ENABLED=0 go test ./examples/dashboard`. `board_test.go` has the board's (from the old `examples/board`), `palette_test.go` the palette's and the tooltips', `drag_bench_test.go` the drag's damage and bench, `damage_test.go` and `idle_test.go` check every frame against a full rendering and that the page goes idle. The goldens (`testdata/`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run 'Golden|Aria' ./examples/dashboard`.
