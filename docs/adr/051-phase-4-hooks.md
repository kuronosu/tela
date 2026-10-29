# ADR-051: Phase-4 hooks

- **Date:** 2026-10-12
- **Status:** Accepted; the forwarder registry (core.RegisterForwarder) is superseded by ADR-190

## Decision

Phase-4 hooks (supersedes ADR-040's `func(T)` setter; amends §3.1, whose React vocabulary yields here to Go's comparability). `UseState` returns a comparable `Setter[T]` (`Set`, `Update`); `UseStateSet`/`UseStateUpdate` return its cached method values; `UseReducer` returns a comparable `Dispatcher[A]`. Updates, reducers and `Post` apply FIFO on the UI thread, each seeing the state left by the previous one; comparable states skip the rebuild on `==`; updates to unmounted components are dropped. `UseEffect`, `UseLayoutEffect` and `UseMemo` take `deps ...any`: none = every build, a single `nil` = mount only (React's `[]`), otherwise element-wise `==`; a different count or a non-comparable value counts as changed and is reported in dev mode. Effects follow React's commit order; passive effects run after `Present` (`Render`: after rasterizing, then unmount). `UseCallback(fn)` returns a comparable `Callback[F]` whose `Fn()` forwards to the latest `fn` (no deps; React's `useEffectEvent`). `UseRef` returns `*Ref[T]`; `UseContext` tracks readers and a provider value that is not `==` rebuilds only them; `UseId` is a tree prefix plus a counter in mount order. `C` never skips on props; `CP[P comparable]` skips on `==` props and `Memo` on `equal`, when no state or context they read changed; any component skips the very same node. The build phase descends from the root along dirty marks (tree order). Dev mode records each hook slot's kind and call site and panics with both sites on order changes, and warns about anonymous `CP`/`Memo` functions. Public types are generic aliases of `core` types

## Rationale

Owner's decision. Comparable handles keep props comparable, so `CP` can skip children that receive setters and callbacks; Go funcs are not comparable. `Update`/`Dispatch` avoid stale closures when several updates queue before a rebuild. Tree-order building keeps effect order and context propagation correct through skipped subtrees
