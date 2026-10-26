# ADR-110: Second phase-8 review in `aria`

- **Date:** 2026-10-21
- **Status:** Accepted

## Decision

Second phase-8 review in `aria` (amends ADR-104 and ADR-108). **Tabs have no typeahead.** ADR-108 gave `UseTabs` the `fmt.Sprint` default `TextValue`, which turned typeahead on, but typeahead only moves the focus and a tab list selects on focus (automatic activation): typing "s" focused "Security" while "Account" stayed selected with its panel shown. React Aria's `TabsKeyboardDelegate` has no `getKeyForSearch`, so `UseTabs` now disables typeahead (`disableType`) and `TabsOptions.TextValue`, which nothing else used, is removed; it shipped in the local v0.8.0 tag, which is not pushed. **Cleanup.** `collState.sync` always fills `textValue` (`sprintKey`), so the nil checks in `keyForSearch` and `typeAhead` and the doc saying typeahead needs `TextValue` are gone (only `DisallowTypeAhead` turns it off); `UseSelect` takes `sprintKey` instead of its own copy of the rule; the tooltip's trigger uses `isSpace`.

## Rationale

A hook whose items select on focus cannot have a search that only focuses, and the reference has none. Rendering is unchanged: every golden is bit-identical
