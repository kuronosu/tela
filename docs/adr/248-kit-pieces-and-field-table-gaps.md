# ADR-248: Kit pieces, Field and Table gaps, animation-delay (phase 20)

- **Date:** 2026-11-08
- **Status:** Accepted

## Decision

Amends ADR-129 (new components and fields), ADR-131 (`aria-sort` `other` and `none` get variants), ADR-076 and §10 (animation-delay, a second arbitrary property).

- **`kit.Alert`**: `Icon`, `Title`, `Description`, `Variant` default or destructive, role `alert` (`Role: "status"` or `"note"` for a notice that is not an interruption). Tones `kit.Success`, `Warning`, `Info` are variants of Badge and Alert (Tailwind emerald, amber, sky at low opacity: shadcn has no such tokens). `Badge.Dot`.
- **`Button.Loading`**: the icon (or `IconEnd`) becomes a spinner, else the content stays under a centered one (`text-transparent`, so name and width are kept); `aria-busy`, no `OnClick`, type `button`, `pointer-events-none`, still focusable.
- **`Input.Icon`/`IconEnd`** in a wrapper with logical insets (with an icon, `Class` is the wrapper's and `InputClass` the input's); **`Separator.Caption`** (no separator role: its children are presentational).
- **`Avatar.Name`** (alt, initials) and **`Color`** (`kit.AvatarAuto`: FNV-1a of the name over the recipe's `AutoColors`, or a name from `Colors`); **`AvatarGroup`** (`Max`, `Total`, ring in the background color, "+N").
- **`Field`**: `LabelContent` and `DescriptionContent` (as `Column.HeaderContent`), `LabelEnd`, `DescriptionEnd` (slots at the line's end); a RadioGroup or ToggleGroup inside is labelled by the Field's label.
- **`Table`**: `AlwaysSorted`, `HideUnsortedIcon` (the arrow keeps its place), `kit.Other`; unsorted sortable headers say `aria-sort="none"`.
- **Engine**: `aria-[sort=other]:` and `aria-[sort=none]:` (with `group-` and `peer-`; `none` matches an element that says it; its node's `Sort` stays empty).
- **Animation delay**: `delay-*` also sets `animation-delay` (tw-animate-css, which shadcn projects load; Tailwind v4's sets only the transition's), `animation-delay-<ms>` (negative with `-`), and the arbitrary property `[animation-delay:150ms]`, what LLMs write for bouncing dots. It replaces the shorthand's delay; changing it does not restart a running animation.

## Why

Gaps an LLM or an app hits writing shadcn code; the tones and the delay follow the dominant vocabulary, and the variants make the sort states styleable.
