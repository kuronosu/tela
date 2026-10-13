# dashboard

An admin dashboard in the style of shadcn/ui's dashboard example, in its zinc theme with dark mode: a header with a team switcher, navigation, search and avatar; a title row with a date range and a button; tabs; four stat cards; a bar chart built from divs and a list of recent sales with avatar initials.

What it shows:

- **Small components with props.** `StatCard`, `SaleRow` and `Bar` take a comparable struct through `tela.CP`, so they skip their build when it does not change; `NavLink` and `Tab` receive the parent's `tela.Setter` in their props, which is comparable too.
- **State.** The header navigation and the tabs are `UseState`; a click restyles the active item.
- **Interaction variants.** `hover:` on links and buttons, and `group-hover:` on the chart: hovering a column recolors its bar and reveals its value (`opacity-0 group-hover:opacity-100`) without building anything.
- **Dark mode.** Every color comes in a pair (`bg-white dark:bg-zinc-900`); render it with `--dark`.
- **Responsive without grid.** The stat cards are flex items with `w-[calc(50%-8px)] lg:w-[calc(25%-12px)]`, and the nav and search hide below `md:` and `lg:`.
- **Icons without SVG.** The stat icons are drawn with borders, rounded divs and a Unicode `$`.

The chart's gridlines, y labels and bars are plain flex layouts: the bars are flex items whose height is a percentage (`h-[74%]`) of an absolutely positioned plot.

A window shorter than the content clips it (scrolling arrives in phase 4); the page is designed for 1280x800 and wraps down to about 700 px wide.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/dashboard` (1280x800).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 [--dark] -o dashboard.png ./examples/dashboard`.
- Its layout: `CGO_ENABLED=0 go run ./cmd/tela inspect --size 1280x800 ./examples/dashboard`.
- Tests: `CGO_ENABLED=0 go test ./examples/dashboard`. The goldens (`testdata/dashboard_x1.png`, `dashboard_x2.png`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/dashboard`.
