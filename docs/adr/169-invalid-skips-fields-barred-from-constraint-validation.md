# ADR-169: `invalid:` skips fields barred from constraint validation

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

(v0.10.4; amends ADR-069 and ADR-131.) A ReadOnly or disabled `Input` or `Textarea` (Disabled on it or on an ancestor) never has the `invalid:` state, nor `group-invalid:`/`peer-invalid:`, as CSS `:invalid` in HTML and Chromium; a change of an element's Disabled refreshes the field states below it (which also fixes a stale `read-only:`). The semantics invalid state already followed this; `aria-invalid:` and `h.AriaInvalid` stay driven by the prop alone.

## Rationale

HTML bars read-only and disabled fields from constraint validation, so a Tailwind form that styles `invalid:` showed errors on fields the user cannot change.
