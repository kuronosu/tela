# ADR-309: tela dev keeps hook internals, renamed components and IME compositions

- **Date:** 2026-11-20
- **Status:** Accepted

## Decision

Phase 29 (amends ADR-113 and ADR-114). **Tela's own state.** The values and hook stores of `aria` and `kit` that hold state in unexported fields get codecs, `GobEncode`/`GobDecode` in `//go:build teladev` files, that save only what a reload must keep (an uncontrolled choice, selection, slider value, color, the focused item of a collection, a menu's checkboxes, `aria.Selection` in the app's own state); `core.DevEncode`/`DevDecode` apply ADR-113's keepable check to what a codec saves. A `UseOnce` that holds a pointer to a Tela type with a codec is kept, decoded into the store the next build's initializer made; other `UseOnce` values stay initializers. What follows the pointer, timers, the typeahead's search and what every build derives from kept state have no codec. **No silent reset.** Every slot a capture cannot keep, silent or warned, is a line of the snapshot (`Resets`), sent by the next build in its state report (`devloop.StateReport.Lines`), and `tela dev -v` prints each (grouped per component and hook). **Renamed components.** A component with no saved node of its own step takes a saved one when: it is a component under the same parent node, unclaimed, at the same key or position, of the same kind (function or struct; never a Provider or Presence); its code is declared at the same `file:line` (the snapshot records it; a rename leaves the declaration in place, another component is declared elsewhere); no component of the captured tree had the new identity; and it is the only such node. The full signature is then checked as for any component.

## Rationale

Codecs keep the slot-matching model (a slot is still its path, index and signature) and leave release builds unchanged; moving hook internals into new kept slots would have changed every aria hook's signature and its rebuild behavior. Reporting through the next build's report needs no new switch: `-v` is the supervisor's. Position and signature alone would move state between two branches of an `if` with the same hooks; the declaration and the "new to the tree" checks are what a rename has and a branch switch lacks.
