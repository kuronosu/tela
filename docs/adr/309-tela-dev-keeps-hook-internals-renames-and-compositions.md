# ADR-309: tela dev keeps hook internals, renamed components and IME compositions

- **Date:** 2026-11-20
- **Status:** Accepted

## Decision

Phase 29 (amends ADR-112, ADR-113, ADR-114). **Tela's state.** Values and hook stores of `aria` and `kit` with unexported fields get `GobEncode`/`GobDecode` codecs in `teladev` files, saving what a reload must keep (an uncontrolled choice, selection, slider value, color, focused item, menu checkboxes, an `aria.Selection` in app state) through `core.DevEncode`, which applies ADR-113's keepable check. A `UseOnce` holding a pointer to a Tela type with a codec is decoded into the store the next initializer made. Every slot a capture cannot keep, silent or not, is a line of the next build's state report (`devloop.StateReport.Lines`) that `tela dev -v` prints. **Renames.** A component with no saved node of its own step takes one under the same parent node that is unclaimed, at its key or position, of its kind (function or struct), declared at the same `file:line` (recorded in the snapshot), when no component of the captured tree had its identity and no other node qualifies; the signature is then checked as usual. **IME.** `edit.DevState` saves the state a composition started from and the composition, restored in the field that gets the focus back; `remote.Host.Promote` keeps the real window's IME session when the new child shows the same field (purpose, multiline, surrounding text) and tells it the session is active; otherwise the session restarts and the child first gets an active-then-ended session, which finishes a restored composition keeping its text (a blur's rule). Remote protocol unchanged.

## Rationale

Codecs keep the slot model and release builds unchanged; new kept slots would have changed every aria hook's signature. Position and signature alone would trade state between two branches of an `if`; a rename keeps its declaration and is new to the tree. The IME holds the preedit only while its session lives; restoring it from the snapshot shows it in the first frame, and the explicit end keeps a restored composition from waiting for an IME that is gone.
