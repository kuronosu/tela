# ADR-194: `LayoutJSONOf`, the layout subtree of one element

- **Date:** 2026-10-29
- **Status:** Accepted

## Decision

Amends ADR-174 (codebase review 2026-10-29, C15; public API chosen by the owner). `telatest` gains `func (a *App) LayoutJSONOf(l Locator) []byte`: the layout subtree of the box that `l` resolves to, in the same JSON as `(*App).LayoutJSON` (the rects are the whole tree's, not rebased). It settles like the other locator queries and fails the test when `l` matches nothing or several elements. An element without a box of its own (inline content of a paragraph, `display: contents`, a text) gives the box of its nearest ancestor that has one. `core.Tree.InspectElement(semantics.NodeID)` is the entry point.

`tela mcp`'s `inspect` tool uses it for a `ref`. The instance asks for it through an interface assertion, because the generated test is compiled against the tela version of the user's module (ADR-059), which `tela mcp` does not pin: with an older `telatest` the tool answers with the whole window's layout, says so, and gives the element's bounds.

## Rationale

The server used to cut the subtree out of the whole layout JSON by matching the element's bounds and a hand-kept table of role to primitive (`roleKinds`, stale: four dead entries, no `svg`), which picks the wrong box when several share a rectangle. Core knows the box.

## What it changes

`roleKinds` and `layoutSubtree` are deleted. The wiki API reference lists the new method.
