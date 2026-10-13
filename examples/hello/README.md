# hello

The smallest screen that looks like something: a headline, a tagline and a rule, styled only with Tailwind classes. It has no state. It is the hero image of the repository README (`docs/images/hello.png`).

- Run it in a window: `CGO_ENABLED=0 go run ./examples/hello` (800x450).
- Render it without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 800x450 --scale 2 -o hello.png ./examples/hello`.
- Tests: `CGO_ENABLED=0 go test ./examples/hello`. The goldens (`testdata/hello_x1.png`, `hello_x2.png`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/hello`.
