# pricing

A pricing page built only on package `kit`, in its default (shadcn) style: a heading with a `kit.Badge`, a monthly/yearly segmented control (`kit.ToggleGroup`) and three plan cards (`kit.Card`: Starter, an inverted highlighted Pro with a floating badge, and Enterprise) with feature lists, check icons and a `kit.Button` each. Switching to yearly lowers the prices and updates the "Billed ..." line of every card.

What it shows:

- **State and props.** `Pricing` holds the billing period in `UseState`. `BillingToggle` and `PlanCard` are comparable struct components (`Yearly bool`, the plan's ID, and the `tela.Setter` for the toggle), so a rebuild of the page that changes neither skips them.
- **A segmented control, not a switch.** The period is a `kit.ToggleGroup[string]` with `DisallowEmptySelection`: a radio group of toggle buttons with arrow keys, and a `kit.Badge` ("Save 20%") inside the yearly option.
- **A card per plan with the kit's slots.** `Title`, `Description`, `Children` (price and features) and `Footer` (the button); the highlighted plan only adds classes (`bg-primary text-primary-foreground`, `DescriptionClass`) and a badge placed with `absolute -top-3 left-1/2 -translate-x-1/2`.
- **Icons from a Lucide subset.** The checks are `icons.Check`, generated with `tela icons gen` into `icons/` (the Lucide license is there and in `THIRD_PARTY.md`), on a round backing colored with a class.
- **Theme tokens.** `bg-background`, `text-muted-foreground`, `bg-primary` and `kit.Theme()` give the dark mode with no `dark:` pairs, except two emerald accents; render it with `--dark`.

The cards are fixed-width (340 px) flex items that wrap, so the page degrades from three columns to two and one as the window narrows; a window shorter than the content scrolls like a page.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/pricing` (1280x800).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 [--dark] -o pricing.png ./examples/pricing`.
- Its aria snapshot (`testdata/pricing.aria.yml`): `CGO_ENABLED=0 go run ./cmd/tela inspect --size 1280x800 ./examples/pricing`.
- Tests: `CGO_ENABLED=0 go test ./examples/pricing`. They check the segmented control (prices, "Billed" lines, arrow keys, toggling twice restoring the picture bit by bit), the three plans in a row, that hovering builds nothing, and the goldens (`testdata/pricing_x1.png`, `pricing_x2.png`, `pricing_dark_x1.png`, `pricing_dark_x2.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/pricing`).
