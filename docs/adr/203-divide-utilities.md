# ADR-203: `divide-*` utilities

- **Date:** 2026-10-30
- **Status:** Accepted (amends §10: `divide-*` is no longer in "Not implemented")

## Decision

Tailwind v4's divide utilities work: `divide-x`, `divide-y`, `divide-x-<n>`, `divide-y-<n>` (whole numbers, or `[<length>]`), `divide-x-reverse`, `divide-y-reverse`, `divide-<color>[/<opacity>]` (palette, tokens, arbitrary), `divide-solid|dashed|dotted|double|none`, with any variant. They follow tailwindcss 4.3.3, which compiles to `:where(.divide-x > :not(:last-child))`: every child but the last gets `border-inline-end-width` (`divide-x`) or `border-bottom-width` (`divide-y`), and `-reverse` moves the line to the inline start or the top; the color is `border-color` and the style `border-style` of the child. v3's `> * + *` with the top and left side is gone. The inline side follows the child's `dir`.

## Why

The `border-t first:border-t-0` workaround needs a class on every child and breaks when the order changes.

## What it changes

`ComputedStyle.Divide` (not inherited): the parent's classes set it; each child that is not the last (`StateLast` clear) applies it in `Compute` after the element defaults and before its own classes, which is how a child's border classes win (`:where()` has no specificity). A token color stays symbolic until the child resolves it, for the child's scheme. `core` gives a child of a parent with divide its structural state, restyles the children when the parent's `Divide` changes and, like `first:`/`last:`, when its children change (`Classes.Divide`, `box.divides`). Children are the style parent's primitive children, as for `last:`; `display: none` ones count, as in CSS. Verified against Chromium (`TestChromiumDivide`). Not done: `divide-inherit`, `divide-x-(--var)`. Differences: a child's own `border-b` does not reset the style to solid when `divide-dashed` is on the parent, and `em` widths use the parent's font size.
