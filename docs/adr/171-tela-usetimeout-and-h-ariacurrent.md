# ADR-171: `tela.UseTimeout` and `h.AriaCurrent`

- **Date:** 2026-10-26
- **Status:** Accepted

## Decision

(Phase 11 API gaps; the public shapes are the owner's; amends ADR-085 and ADR-131.) New public names: **`tela.UseTimeout(d time.Duration, f func(), deps ...any)`**, **`h.AriaCurrent(value string)`** and **`telatest.Current(value string)`**.

`UseTimeout` is `UseInterval`'s one-shot sibling, on the same frame-clock machinery (`intervalSlot`, `NextDeadline`, `TickGestures`): it calls `f` once on the UI thread `d` after the frame that first shows the component, with no goroutine (`App.Advance` moves it; a headless `Render` never runs it). `d <= 0` cancels it; a change of `d` or of `deps` (compared like `UseEffect`'s) restarts it from that build's frame, even after it fired; with no deps and the same `d` it fires once per mount; `f` is always the latest build's; unmounting cancels. After it fires nothing waits, so the app idles. The engine is `core.UseTimeoutAfter` and a new hook kind `hookTimeoutAfter` (named `UseTimeout`), because `core.UseTimeout()` already is the `Timeout` handle of package `aria` (a different hook, rejected on a slot mismatch).

`AriaCurrent` takes `page`, `step`, `location`, `date`, `time` and `true`; `false` and `""` mean none; tokens are ASCII case-insensitive, and unknown ones panic in dev mode with the closest valid one and are ignored otherwise, like `AriaSort`. `semantics.Node.Current` carries it ("" for none). **Playwright 1.61.1's aria snapshot does not print it** (its `toAriaNode` and `createKey` have no current field; `aria-current` only appears in a list of global attributes), so neither does Tela's. The variants `aria-[current=page]:`, `step`, `location`, `date`, `time` and `true` (also `group-` and `peer-`) test one new flag each (`StateAriaCurrent…`, in `AriaStates`), so `style.StateFlags` is now a `uint64` (the six flags would have been the 33rd to 38th bit). There is no bare `aria-current:`, as in Tailwind v4: it gets an error naming the form to use, and a typo in the token gets a did-you-mean (the same for `aria-[sort=…]:`). `telatest.Current` follows `Sort` ("false" matches an unset element; its failure names `h.AriaCurrent`).

## Rationale

Notices and toasts need a delay without a goroutine or a periodic timer to cancel, and navigation links, steppers and calendars style their current item with `aria-current`, which Tailwind users write as `aria-[current=page]:`.

_Note (phase-11 review): in dev messages the internal timer hook of `aria` is named "aria's Timeout", so a swap with `tela.UseTimeout` names two hooks._
