# ADR-306: `telatest`'s accessibility audit

- **Date:** 2026-11-20
- **Status:** Accepted. Amends §16, §17, ADR-246.

## Decision

`telatest.App` gains `ExpectAccessible(opts...)` (fails the test) and `Audit(opts...) []Violation` (returns them, as `axe.analyze()` does), built from the rules of the kit audit of ADR-246, which now calls them. Names follow axe-core and `@axe-core/playwright`'s `AxeBuilder`: `Rules()` lists `Rule{ID, Description, Impact, Tags, Default}`; options `WithRules(ids...)`, `WithTags(tags...)` (run only those, non-default rules included), `DisableRules(ids...)` (the way to accept one violation in one call) and `Exclude(locator)` (a subtree); an unknown ID fails with the closest ones. The rules run on the semantics tree (settled first) and skip hidden nodes: `aria-command-name`, `aria-input-field-name`, `aria-toggle-field-name`, `aria-dialog-name`, `aria-progressbar-name`, `aria-meter-name`, `aria-tooltip-name`, `aria-treeitem-name`, `role-img-alt` (a name by role group), `focusable-name`, `focus-order-semantics` (a focusable element has a role), `aria-required-attr` (checked, combobox expanded, slider value, tab selected, heading level), `aria-valid-attr-value` (`AriaControls` and `AriaActiveDescendant` point at the tree), `set-position` (an item is inside its set) and `target-size` (WCAG 2.5.8: 24x24 or a 24 px circle of space; inline, disabled, nested and hidden targets exempt; the equivalent-target exception is not applied). Impact is axe's four levels; tags are axe's (`wcag2a`, `wcag2aa`, `wcag22aa`, `wcag412`, `aria`, `best-practice`) plus `tela`.

A `Violation` is `Rule, Impact, Message, Fix, Element, Source, Bounds`: the element is its line of the aria snapshot (role, name, states), the source `file:line` when the app is mounted with `Instrument()`. `ExpectAccessible` fails with every violation: `1. aria-command-name (critical): a button needs an accessible name`, then `element: button [src=settings.go:42]` and `fix: ...`. `Violation` marshals to JSON (`rule, impact, message, fix, element, src, x, y, width, height`).

## Why

The audit lived in a test of `telatest`; apps need the same check for their own screens and overlays, with the rule's name to search for and a way to accept an exception without turning the audit off. axe's names and options are what test authors and agents know.
