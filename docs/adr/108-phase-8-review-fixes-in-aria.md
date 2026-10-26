# ADR-108: Phase-8 review fixes in `aria`

- **Date:** 2026-10-21
- **Status:** Accepted

## Decision

Phase-8 review fixes in `aria` (amends ADR-101, ADR-104 and ADR-105). **Collections.** When the focused item goes, the focus is repaired with the disabled keys of the build that removed it (`sync` rebuilt them after the repair) and follows react-stately's `useListState`: the focused item's old index, moved back by the number of items that went when more than one did, then the first enabled item from there on, else going back; a collection that had the focus keeps it even when the engine blurred it (an item disabled in the same build). Without `TextValue`, `UseListBox`, `UseMenu` and `UseTabs` search the key as `fmt.Sprint` prints it, as `UseSelect` and `UseCombobox` did; `TabsOptions.TextValue` is new for that. **Tooltip.** Only Enter and Space on the trigger close it (React Aria's `usePress` starts a press on those keys); Shift, the arrows and shortcuts leave it open. **Cleanup.** `orDefault` and the hand-written defaults of `Placement`, `Type` and `ButtonLabel` use `cmp.Or`.

## Rationale

The repair used the previous build's disabled set, so a focus could land on an item the same build disabled; React Aria's rule is kept (no nearest-by-distance guess), minus its endless loop when every later item is disabled. The default `TextValue` removes the silent gap found building `examples/aria`; typeahead on a tab list is React Aria's too. A Space typed within the one-second search window is a character of the search (React Aria's `useTypeSelect`), and `Selection.Len` of `aria.All` is 0: both are documented, not changed. Rendering is unchanged: every golden is bit-identical
