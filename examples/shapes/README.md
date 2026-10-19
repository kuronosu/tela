# shapes

A page of twelve cards that shows what Tela can paint with elements and Tailwind classes: shadows, linear, radial and conic gradients, rings, `blur` and `backdrop-blur`, opacity, rotated and rounded shapes, flex layouts and real text. It is the phase-3 rewrite of the phase-1 showcase, which drew the same page straight into a display list (that scene now lives on as a golden test, `internal/gfx/raster/showcase_test.go`).

**The page is static on purpose:** it has no handlers, `hover:` or state, so it only shows painting. (Pointer events, state and the `hover:`/`active:` variants work since phase 4: see `examples/counter`, `examples/dashboard` and `examples/settings`. Transitions run since phase 6: see `examples/counter`.)

- Run it in a window: `CGO_ENABLED=0 go run ./examples/shapes` (960x640; Wayland or X11, force one with `TELA_BACKEND=wayland|x11`).
- Render it without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot [--size 960x640] [--scale 2] -o shapes.png ./examples/shapes`.
- Print its layout tree: `CGO_ENABLED=0 go run ./cmd/tela inspect ./examples/shapes`.
- The goldens (`testdata/shapes_x1.png`, `shapes_x2.png`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/shapes`.

## Structure

`Shapes()` is the page (the `Preview`): a soft gradient with two blurred glows, and the twelve cards at a fixed 202 x 170 px in a centered `flex-wrap` container: four columns at the default 960x640, three in a tiled window about 700 px wide (`p-5 md:p-10` gives the narrow window its third column). Each card is a function component that returns one node:

| File | Components |
|---|---|
| `cards.go` | `ProfileCard` (clipped avatar, ring), `ChartCard` (bars with `%` heights), `PhotoCard` (gradient picture, `overflow-hidden`), `TogglesCard` (switches), `ProgressCard` (gradient fills), `SearchCard` (`inset-shadow`, `inset-ring`, chips) |
| `effects.go` | `GradientCard` (`bg-linear-to-br`, colored shadow), `SunsetCard` (`bg-radial`, `backdrop-blur`), `ConicCard` (`bg-conic` ring), `GlassCard` (`blur` discs, frosted pane), `ShapesCard` (rotation, per-corner radius, `drop-shadow`), `DropCard` (`border-dashed`) |
| `main.go` | `Shapes`, `Preview`, `main` and the helpers `row`, `card`, `title` |

Small helpers (`card`, `title`, `toggle`, `progress`, `arrowButton`, `magnifier`) take class strings, as a user would write them.

## Substitutions

The phase-1 scene used arbitrary paths, which the CSS subset has no way to express without SVG or `clip-path`. Each such element became a design of the same spirit:

| Phase-1 scene | Here |
|---|---|
| Avatar landscape (clipped hills) | A circle with `overflow-hidden`, a sky gradient, a sun and two overlapping green discs for the hills |
| Photo with waves and a sun | A rounded box with `overflow-hidden`: sky gradient, a disc, and two wide `rounded-[50%]` ellipses for the waves |
| Five-pointed star with a colored drop shadow | A burst: two rounded squares, one rotated 45 degrees, with `drop-shadow` |
| Blob with a colored drop shadow | A square with a different `%` radius on each corner and a colored `shadow-lg` |
| Stroked wave (round caps and joins) | A waveform: thirteen `rounded-full` bars of different heights and colors |
| Stroked zigzag | Ten pills tilted by `rotate-35` and `-rotate-35` in cells of one width, whose round ends meet |
| Search icon strokes | A ring (`border-2 rounded-full`) and a rotated bar |
| Placeholder bars for text | Real short labels |
| Frosted pane faking blur with alpha | Real `backdrop-blur-xl` over `blur-xl` discs |
| Page glows (radial gradients) | Blurred discs (`blur-3xl`) clipped by the page |

## Workarounds

- The cards wrap in a flex row, each in a fixed-size cell (the example predates grid; `grid-cols-*` works since v0.6.0).
- `!p-2.5` (important) is unsupported: the photo card does not use the shared `card` helper, which fixes the padding.
- The conic ring's six stops are an arbitrary `bg-[conic-gradient(...)]`, since Tailwind's `from-`/`via-`/`to-` take one middle stop.
- The knob of the ring and the glass pane use absolute positioning with fixed offsets, as the design is fixed-size.

## Tests

`go test ./examples/shapes` checks that the page produces no class errors or dev warnings at three window sizes (an unknown class fails the test, with core's "did you mean" message), that every card is 202 x 170 px in four columns at 960x640 and three at 701x846, that no text or button sticks out of its card at four window sizes, and the two goldens bit by bit.
