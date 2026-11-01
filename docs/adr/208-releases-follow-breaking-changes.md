# ADR-208: Releases follow the public API, not the phases

- **Date:** 2026-10-31
- **Status:** Accepted. Amends ADR-049.

## Decision

While Tela is v0, a release that breaks the public API raises the minor version (`v0.N.0`). Every other release raises the patch version (`v0.N.M`), whether it brings features, fixes or a whole phase. Phases stay the unit of planning, but they no longer own a version number.

`v0.13.0` releases the work done between phases 12 and 13: ADR-205's `tela.Node` change, the `looks` module, the examples consolidation and the website. Phase 13 is then `v0.13.1`, unless it breaks the public API.

Tags stay annotated. The owner approves them and pushes them; agents never push.

## Rationale

The website (ADR-206, ADR-207) builds its docs from the latest tag and tells readers to `go install …@latest`. Fifty-six commits, one of them breaking, waited for phase 13, so the landing page showed code that did not compile with the version people install. A patch release could not carry a breaking change (`go get -u=patch` must stay safe), and a minor version per phase would have forced the website to wait.

## What it changes

- `AGENTS.md`'s end-of-phase steps and the header of `CHANGELOG.md` follow this rule.
- The phase headings in `docs/ROADMAP.md` drop their fixed versions.
