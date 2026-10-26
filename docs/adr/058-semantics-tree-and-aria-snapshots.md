# ADR-058: Semantics tree and aria snapshots

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Semantics tree and aria snapshots (amends §8 phase 7 and ADR-047). `internal/semantics` defines the node (role, name, description, value, range, toggles, states, actions, window bounds) and serializes Playwright 1.61's aria snapshot (default mode, ported). `core` builds the tree on demand from the Element and Box trees: HTML-AAM implicit roles, AccName 1.2 as Playwright computes it, Playwright's state rules, stable node IDs; nothing runs per frame (a full 10k-element build takes about 3.4 ms); incremental `needsSemantics` waits for the phase-8 accessibility backends. Hidden subtrees are kept with a hidden state and left out of the YAML. A textbox's value is its text content until text inputs exist. `ClickNode` (`element.click()` semantics), `FocusNode` and `ScrollNodeIntoView` are the internal API of `telatest` and `tela mcp`. `h` gains `Role`, `Alt` and the `Aria…` props (label, labelledby, description, hidden, checked, pressed, selected, expanded, disabled, level, value now/min/max/text); unknown roles panic in dev mode with a suggestion. `tela inspect` defaults to `--aria` as §17 specifies; `--layout` keeps the JSON

## Rationale

Playwright's format is what LLMs know, and the YAML is checked byte for byte against Playwright itself on 15 case trees; building on demand costs nothing at idle. The `h` prop names follow React's `role`/`aria-*` attributes (chosen while the owner was away; to review)
