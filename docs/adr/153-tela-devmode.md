# ADR-153: `tela.DevMode()`

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

`tela.DevMode()` (phase 10; amends ADR-152's finding 8; decided by the main session while the owner was away, listed for review). `tela.DevMode() bool` reports the dev mode where it is called: in a build, the mode of the tree being built (`App.Dev`, `telatest.NoDev`), elsewhere the process default with `TELA_DEV` overriding (ADR-085, ADR-093); it is `core.DevNow`. Package `kit` reads it instead of importing `internal/core`, so `kit` imports only public packages again (ADR-129)

## Rationale

ADR-152 replaced kit's panic-and-recover probe with `core.DevNow`, which broke ADR-129's rule that kit files import nothing from `internal/` (the precondition of a later copied-source distribution). A public accessor keeps the per-tree precision and the rule, costs one exported function, and serves any component library that validates its own props as Tela validates classes and roles
