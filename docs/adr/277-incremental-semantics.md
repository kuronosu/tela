# ADR-277: Incremental semantics and accessibility updates

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-058 and ADR-246.

## Decision

- **Core marks, then builds what changed** (`internal/core/semincr.go`). Each input of an element's node marks it (props, children, text, display and visibility, disabled and invalid states, inertness, the modal outside, a field's editor, scrollability, an `…Fn`'s Value); geometry marks the subtree of a box that moved (`layout.ChangeObserver` for layout results, paint transforms and positions, scroll offsets, sticky and floating shifts, paragraphs laid out again). The focus and viewport are compared per build. An unmarked element with the same inherited state keeps its node and subtree; a node built again that equals its old one keeps the old pointer. Ancestors are built again (names from content, bounds of boxless elements, set positions with `semantics.NumberSet`); elements named from outside their subtree (labelledby, describedby, labels, controls) record what they read and are built in a second pass when it changed.
- **`TELA_CHECK_SEMANTICS=1`** (on in core's tests) compares each tree, and each accessibility update, with a full build and conversion, and panics at the first difference; the AT-SPI bridge compares its views too, and randomized tests (`platformtest.AccessScript`) compare all three bridges' with full builds.
- **The conversion** to `AccessUpdate`s visits only subtrees whose pointer changed. `AccessUpdate` is unchanged (no contract bump).
- **`platform.AccessTree` is persistent** (a HAMT, `internal/platform/internal/idmap`) with parents (`Parent`): `Apply` follows the changed children lists and keeps the pointers of unnamed nodes. The AT-SPI view, the UIA snapshot and the AX tree update from the changed and removed IDs, in persistent maps.

## Why

A one-node change in a 10k-node tree cost a full rebuild at every layer (about 14 ms + 12 ms in core, 3.5 ms in `Apply`, 9 ms in the AT-SPI view); it now costs about 0.1 ms, 2 µs and 3 µs, and bridges still answer from immutable snapshots on other threads.
