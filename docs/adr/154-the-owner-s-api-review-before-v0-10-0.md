# ADR-154: The owner's API review before `v0.10.0`

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

The owner's API review before `v0.10.0` (phase 10; amends ADRs 130–153). **D1:** a component reads its direction with an engine hook, `tela.UseDirection()` (the direction at its position, as `UseColorScheme` reads the scheme), so `h.Dir("rtl")` alone mirrors drawing, arrow keys and drags; kit's `Direction` fields stay as an override and `kit.DirectionProvider` is removed before it is released (supersedes that part of ADR-151). **D2:** a `Default…` field is a pointer when the zero value of its type is a valid value: `Select`, `Tabs` and `MenuItem` take `DefaultValue *K`; `ColorPicker`, `ColorInput` and `aria.ColorStateOptions` take `DefaultValue *tela.Color`; values whose zero means "none" stay values (`string`, `bool`, `aria.Date`), so `aria.CalendarOptions.DefaultValue` becomes a `Date` like kit's. **D3:** a boolean's zero value is shadcn's behavior: `Dialog.ShowClose` becomes `HideClose` (the close button shows by default). **D4:** aria's percentages (`ThumbPercent`, `Percent`) stay `float64` from 0 to 100. **D5:** kit keeps both the single-value and the multiple forms of `Slider` and `ToggleGroup`. **D6:** the defaults stay: a sortable column cycles ascending, descending, unsorted; a range picker closes when the range is complete; a context menu opens with its corner at the pointer. **D7:** `Card.Title` stays a `div` and `Card.TitleLevel int` (1–6) makes it a heading. **D8:** `tela.DevMode()` stays. **D9:** the names in ADRs 132–143 stay. **Additions:** `tela.MergeProps(props ...tela.Child) tela.Prop` (several props where one is taken, such as kit's `Props`), `Table.RowProps func(T) tela.Child`, `AriaLabel` on `Input`, `Textarea`, `ColorPicker` and `ColorInput`, `ID` on `ColorPicker` and `ColorInput`, and the README screenshots of the migrated examples

## Rationale

The owner's decisions on the review page of the phase's new API: everything that would break after the tag is settled before it
