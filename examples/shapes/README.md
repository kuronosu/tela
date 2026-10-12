# shapes

A fixed scene of rounded cards, gradients, shadows, blur and strokes, drawn straight with `internal/gfx` (no text yet).

- Run it in a window: `CGO_ENABLED=0 go run ./examples/shapes` (Wayland or X11; force one with `TELA_BACKEND=wayland|x11`).
- Render it without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot [--size 960x640] [--scale 2] -o shapes.png ./examples/shapes`.
- The goldens are in `testdata/`; regenerate them with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/shapes`.
