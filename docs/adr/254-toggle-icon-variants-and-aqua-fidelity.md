# ADR-254: Toggle icon variants and Aqua fidelity

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-129, ADR-197 and ADR-223.

## Decision

- `kit.ToggleStyle.IconVariants map[Variant]string`: classes added to
  `Icon` for the glyph of one variant, in `Toggle` and in the items of
  `ToggleGroup`. A variant with no entry adds nothing (a nil map is fine), so
  recipes written before it keep working; `DefaultStyle` has an empty map.
- `looks.Regions.SearchField`: classes added to the search input after the
  recipe's, for a field the style's `Input` does not draw (empty: nothing).
- `looks/aqua` uses them: the glyphs of the `looks.Sidebar` toggles are
  `text-primary` in both designs, selected or not (a source list's tinted
  symbols); Sequoia's search field is borderless gray (it keeps the ring and
  a field fill on focus).
- The token `destructive-text` (every look defines it; the others follow
  `destructive`): Tahoe's dark is `#ff7a7d`, and its recipes use it for red
  text (Destructive button, destructive menu items, alerts, field errors).
  The Destructive button of Tahoe's dark has a black 50% tint instead of the
  white glass, because over a saturated backdrop the glass is a mid-tone
  where no red reaches 4.5:1 (1.2:1 before, 4.9:1 now, measured on the
  sheet's pixels; `TestDestructiveTextContrast`).
- The gallery's pop-up button takes `ChevronsUpDown` in the macOS looks
  (Sequoia's accent tile, Tahoe's plain glyph).

## Why

Per-variant glyph color is how macOS tints a sidebar without touching the
toolbar's; the roadmap item asked for it. Descendant variants (`[&_svg]:`)
do not exist in Tela, so the recipe needs its own field.
