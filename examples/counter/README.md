# counter

The Counter of ARCHITECTURE section 1, verbatim: a card on a stone background with a "COUNT" label, a large number and a full-width button, styled only with Tailwind classes. It is the phase-3 showcase and its done criterion: the render must match the same markup in HTML + Tailwind CSS, as Chromium draws it.

**It is interactive** (since phase 4): a click increments the count (`UseState` and a `tela.Setter`), and `hover:bg-stone-700` and `active:scale-95` restyle the button under the pointer. `transition` animates the changes (150 ms) on the frame clock. `h/events_test.go` drives this component through a headless window and checks the hover, press and click.

- Run it in a window: `CGO_ENABLED=0 go run ./examples/counter` (800x600).
- Render it without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 800x600 [--scale 2] -o counter.png ./examples/counter` (`Preview()` returns `Counter()`, which wraps `counter` in `tela.C`, so its `UseState` works).
- Print its aria snapshot (`testdata/counter.aria.yml`): `CGO_ENABLED=0 go run ./cmd/tela inspect --size 800x600 ./examples/counter`; add `--layout` for its layout tree as JSON.
- The goldens (`testdata/counter_x1.png`, `counter_x2.png`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestCounterGolden ./examples/counter`.

## Comparison with Chromium

`testdata/chromium/` holds the reference: `counter.html` (the same markup, in a `<body>` as tall as the viewport, which `h-full` needs), `counter.css` (compiled from it by the Tailwind CLI v4.3.3, with `--font-sans: "Inter"` and Inter loaded from `internal/assets/fonts` by `@font-face`), Chromium's screenshots at device scale 1 and 2 (`--force-color-profile=srgb --hide-scrollbars`, viewport 800x600), and `layout.json` with the boxes Chromium measured (`getBoundingClientRect`, and a `Range` over the texts; the page pins html and body to 600 px, because Chromium's `--dump-dom` window is shorter than the one requested).

`go test ./examples/counter` runs without Chromium and checks:

- `TestCounterHTML`: the HTML is up to date and its classes are the ones of `Counter`.
- `TestCounterLayoutChromium`: the outer div, card, label, heading and button and the baselines and edges of the three texts agree with Chromium within 0.02 CSS px (measured at most 0.0125), at scale 1 and 2 (both round font metrics in device pixels, ADR-159).
- `TestCounterPixelsChromium`: the render against the screenshots, whole and by region (label, heading, button, card edge, card body, shadow, canvas), with the measured bounds documented in `compare_test.go`. On failure it writes `testdata/chromium_x1_actual.png` and `chromium_x1_diff.png` (differences times 8); set `TELA_COUNTER_DUMP=1` to get them always.
- `TestCounterGolden`: Tela's own render, bit by bit.

Regenerate the reference (needs `chromium` and the Tailwind CLI) with

    TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
      go test -run TestRegenerateChromium ./examples/counter/
