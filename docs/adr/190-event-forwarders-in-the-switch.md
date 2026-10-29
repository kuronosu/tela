# ADR-190: Event forwarders live in core's type switch

- **Date:** 2026-10-29
- **Status:** Accepted

## Decision

Supersedes the forwarder-registry part of ADR-051. `core.RegisterForwarder`, the `forwarders` map and package tela's `init` are deleted. `UseCallback` builds the forwarder of every event handler type (`func(E)` for `MouseEvent`, `PointerEvent`, `DragEvent`, `KeyboardEvent`, `FocusEvent`, `WheelEvent`, `ScrollEvent`, `InputEvent`, `ChangeEvent`, `CompositionEvent`, `SelectEvent` and `SubmitEvent`) as a plain closure through a named case in `core`'s type switch in `forwarder`; any other function type still falls back to `reflect.MakeFunc`.

## Why

The registry existed because the event types once lived in package tela (ADR-051). They are `core` types now, so `core` can name them. The registry had also drifted: the five field event types were never registered, so their handlers went through `reflect.MakeFunc` and allocated on every call. A global mutable registry and its `init` ordering bought nothing.

## What it changes

Adding an event type touches one package. `TestEventCallbacksDoNotAllocate` covers all 12 types (it measured 9 allocations per round before, 0 after). No public API changes.
