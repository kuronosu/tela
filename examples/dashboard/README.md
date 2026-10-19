# dashboard

An analytics dashboard in the glassmorphism style, light and dark: a sticky glass header and sidebar, four stat cards, a bar chart, a donut, a sortable task list, an expanding card and a list of sales, over a colorful backdrop. It is the showcase of Tela's phase-6 vocabulary on one screen, and the "60 fps at 1080p on CPU rendering" target of that phase (see [Performance](#performance)).

| Light | Dark |
|---|---|
| ![light](testdata/dashboard_light_x1.png) | ![dark](testdata/dashboard_dark_x1.png) |

What it shows, by file:

- **Grid, sticky and fixed** (`main.go`, `chrome.go`). The page is `grid lg:grid-cols-[16rem_1fr]`; the card rows are `grid grid-cols-2 xl:grid-cols-4` and `xl:grid-cols-3` with `col-span-2`. The sidebar is `sticky top-0 h-screen`, the header `sticky top-0` over the scrolling content, the help button `fixed right-6 bottom-6`.
- **Glass** (`backdrop.go`, `cards.go`). `bg-white/55` panels with a hairline `border-white/60` over an image registered with `tela.Assets` (`bg-[url(img/aurora-light.jpg)]`, chosen per scheme with `tela.UseColorScheme`; `img/gen.go` draws both), `backdrop-blur-xl` where there is detail behind it, a `bg-clip-text` gradient headline, a `bg-[conic-gradient(...)]` donut with a blurred disc over its middle, and a `mix-blend-overlay` highlight on the gradient card.
- **Transitions** (everywhere). `transition duration-200 hover:` on buttons and rows, `transition-[transform,box-shadow]` lift on the cards, `active:scale-95`, `group-hover:` bar tooltips, and `animate-spin` plus `animate-pulse` for the "Syncing" badge after a period change (`LiveBadge`).
- **Springs and deferred props** (`cards.go`). The numbers, the 12 chart bars and the 48 sparkline bars follow their targets with `tela.UseSpring`; the number is an `h.TextFn`, the heights are `tela.HeightFn`, the goal meter is `tela.WidthFn` and `tela.BgFn` (rose, amber, emerald as it fills). Switching the period or the tab moves all of them without building anything per frame.
- **Layout animations** (`lists.go`, `chrome.go`). The task rows have `AnimateLayout`: sort them, or check one off and it sinks, and every row slides to its new place. The nav pill, the period pill, the chart's tab underline and the dialog's format pill are `LayoutID`s that slide from the item that loses them. The insight card expands with `AnimateLayout` on itself, its header and its details.
- **Overlays** (`overlays.go`). The dialog and the toast render into an `h.Portal`. The dialog enters with `starting:` and leaves with `h.Exit` inside `h.Presence`; the toast runs its own exit with `tela.UsePresence`, and its 4 s countdown is a `tela.UseTween` on the frame clock whose `OnDone` dismisses it.
- **Environment** (`chrome.go`). `dark:` pairs on every color; the Day/Night switch calls `tela.SetColorScheme`.
- **Previews.** `tela snapshot --preview "2 Dialog"` and `"3 Toast"` render those states (`Props` starts the page in any of them).

## The glass budget

A `backdrop-blur` recomputes its pixels whenever what is behind it changes, and a card that scrolls over a fixed backdrop has a different backdrop on every scroll frame. So the page spends blur where there is detail to frost (the header, the dialog, the toast, the donut's disc) and gives the cards, which sit over a smooth image, a translucent fill and an edge. Four things made the difference between 30 and 14 ms per frame at 1080p, and are worth knowing when writing your own glass:

1. Blur per card is the largest cost (about 25 ns per blurred pixel on one core).
2. A `backdrop-blur` inside an element that is a layer (opacity under 1, a blend mode, a filter, an animated transform) turns off the parallel rasterization of the whole frame while it is on screen. The insight card's chips have no blur for this reason.
3. Soft shadows on many cards are not free; the cards have none, and shadows are on the elevated things.
4. A fade over the whole window (a scrim) repaints the whole page on every frame of it, so the dialog's scrim appears at once and only the panel animates; the page's fills do not transition, so switching the theme repaints once.

## Performance

`bench_test.go` drives the page without a window through a scripted session of 935 ticks of a 60 Hz clock (644 frames drawn at 1080p): hover sweeps, a period change, a tab switch, a reorder, scrolling and a fling under the glass header, the dialog, a toast with its countdown, the expanding card and two theme switches. A frame is timed like `core.Host` runs it (input, animate, build/style/layout/paint, rasterization of the damage, the copy to a presentable buffer, effects) and runs only when the tree asked for one. After the session the page must ask for no frames at all.

```sh
CGO_ENABLED=0 go test -run TestSession -v ./examples/dashboard            # the full tables, one session per configuration
CGO_ENABLED=0 go test -run xxx -bench Session -benchtime 1x -count 3 ./examples/dashboard
```

Medians of three runs on a Core Ultra 5 125H (4 CPUs, so 4 raster workers), frame time in ms:

| Configuration | p50 | p95 | p99 | max |
|---|---|---|---|---|
| 1920x1080, scale 1 | 2.1 | 14.6 | 18.8 | 28.5 |
| 1920x1080, scale 2 (3840x2160 px) | 4.2 | 48.5 | 56.0 | 72.2 |
| 1440x900, scale 2 (2880x1800 px) | 5.1 | 36.4 | 41.1 | 55.5 |

At 1080p and scale 1, 5 of 644 frames take longer than 16.7 ms. The frames that dominate the upper percentiles are the ones that move every pixel: scrolling (the cards move over a fixed backdrop, so the damage is nearly the whole window, 14 ms at 1080p), the first frame of the dialog and the theme switch. Everything else (hovers, springs, layout animations, transitions, the toast) is under 7 ms at p95. The worst phase is the rasterizer (11.9 ms at p95); the pipeline (build, style, layout, paint) is under 2 ms at p95.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/dashboard` (1440x900). `TELA_DEBUG=fps` shows the frame times.
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1440x900 --scale 2 [--dark] [--preview "2 Dialog"] -o dashboard.png ./examples/dashboard`.
- Tests: `CGO_ENABLED=0 go test ./examples/dashboard`. The goldens (`testdata/`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/dashboard`.
