# pricing

A pricing page: a heading, a monthly/yearly switch and three plan cards (Starter, a highlighted Pro and Enterprise) with feature lists and check marks. Switching to yearly lowers the prices and updates the "Billed ..." line of every card.

What it shows:

- **State and props.** `Pricing` holds the billing period in `UseState`. `BillingToggle` and `PlanCard` are `tela.CP` components: their props (`ToggleProps`, `PlanProps`) are comparable, and the toggle receives the `tela.Setter` directly.
- **A switch made of divs.** A track and a knob placed with `justify-start` / `justify-end`; the track, the labels and the knob all react to `hover:`. The labels are buttons too.
- **Gradients and elevation.** The highlighted plan is a `bg-linear-to-b` card with `shadow-2xl` and a floating badge placed with `absolute -top-3 left-1/2 -translate-x-1/2`.
- **Icons without SVG.** The check marks are a rotated box with only its right and bottom borders.
- **Dark mode.** Every color comes in a pair; render it with `--dark`.

The cards are fixed-width (340 px) flex items that wrap, so the page degrades from three columns to two and one as the window narrows; a window shorter than the content scrolls like a page.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/pricing` (1280x800).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 [--dark] -o pricing.png ./examples/pricing`.
- Tests: `CGO_ENABLED=0 go test ./examples/pricing`. They check the toggle (prices, "Billed" lines, the knob moving, toggling twice restoring the picture bit by bit), the three plans in a row, that hovering builds nothing, and the goldens (`testdata/pricing_x1.png`, `pricing_x2.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/pricing`).
