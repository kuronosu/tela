# ADR-205: `tela.Node` is the one interface every renderable implements

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

`tela.Node` becomes `interface{ Build() Node }`. Elements, text, fragments and the nodes of `C`/`CP`/`Memo`/`Map` stay one internal struct (renamed so compiler errors read `core.Node`); a struct component (`kit.Button{...}`) is a `Node` itself and becomes a component node where it is consumed. `tela.Component` stays as a documented alias of `Node`; `tela.Child` stays a distinct named type with the same method set, so the dev check of ADR-088 still tells `Component` fields from `Child` fields. `nil` is the hole (`tela.Node{}` and `IsZero` go, as do `Node.Key` and `Node.String`). `Range` and `VirtualList` take `func(int) tela.Node`; the two `Preview` forms of ADR-175 become one. Elements stay functions (`Div(...)`) and components stay structs. Supersedes the Node/Component part of ADR-088, including its "Left open: `tela.Base`"; amends ADR-047, 091, 094, 097 and 175.

## Rationale

A function or `Build` declared to return `Node` could not return a struct component, so code wrapped it in `Fragment(...)` (60 single-argument wraps in `examples/`, 12 of them unnecessary). Every retained-mode Go UI library makes components the tree type. The study (five approaches, counts and benchmarks) found no measurable cost in core's hot paths and two fewer allocations per removed wrap. Lost: a `Prop` returned where a node is expected compiles; existing run-time panics and a `tela lint` rule catch it.

## What it changes

Source-breaking, shipped in the next minor version: `tela.Node{}` → `nil`, `n.IsZero()` → `n == nil`, `case tela.Node:` also matches components, unions over `Node | Component` no longer compile, and `n1 == n2` can panic for a non-comparable component. `tela lint` gains a `fragment` rule (with `-fix`) that removes redundant single-argument `Fragment`s, and a `prop-node` rule.
