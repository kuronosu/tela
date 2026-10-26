# ADR-172: `kit.Field` ControlAfter and `kit.FieldSet`

- **Date:** 2026-10-26
- **Status:** Accepted

## Decision

(Phase 11; amends ADR-145's "API gaps" and ADR-130's `Field`.) New public API, named after shadcn/ui's `field.tsx` at `kit.ShadcnCommit`, for the owner's review:

- `kit.Field.ControlAfter bool`: a Horizontal field puts the control after the label and the description (shadcn's `FieldContent` first, then the `Switch`); a Vertical one ignores it (its order is always label, control, description, error). Same classes; only the child order changes, so the label's `For`, the error, `Invalid` and RTL work as before.
- `kit.FieldSet` (struct component): `Legend`, `LegendVariant`, `Description`, `Children`, `Class`, `LegendClass`, `DescriptionClass`, `ContentClass`, `Props`. A `role="group"` named by the legend (`aria-labelledby`) and described by the description. Recipe `kit.FieldSetStyle` (`Style.FieldSet`): `Root`, `Content`, `ContentWithRadioGroup`, `Legend`, `LegendVariants`, `Description`, `DescriptionAfterLegend`.
- `kit.LegendVariant` with `LegendVariantLegend` (the zero value, `text-base`) and `LegendVariantLabel` (`text-sm`).
- A `RadioGroup` or `ToggleGroup` in a `FieldSet` takes its name from the legend unless it has an `AriaLabel`; a `Field` stops the legend reaching through it.

`FieldGroup` is not added: a `FieldSet` already stacks its children, and elsewhere it is a `Div` with `flex w-full flex-col gap-7`.

## Rationale

The settings example worked around both gaps (`flex-row-reverse`, a caption span plus `AriaLabel`). The legend is not a flex item in a browser's fieldset, so the root is a block and the description and children go in an inner column; the legend has `w-fit` as a browser's does. shadcn's `has-[>[data-slot=radio-group]]:gap-3` is applied for a `RadioGroup` given directly as `Children`; `nth-last-2:-mt-1` is not modeled. Engine unchanged (there is no `fieldset` element). `examples/settings` migrates with pixel-identical goldens (its aria snapshot gains the two groups and the switch rows list the text first).
