# ADR-049: Versioning

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Versioning: SemVer tags on the Go module (`vX.Y.Z`), v0 until the public API is stable (at the earliest after phase 7); one minor version per completed roadmap phase (v0.1.0 = phase 1, v0.2.0 = phase 2, v0.3.0 = phase 3, tagged retroactively on the commits that closed them), patch versions for fixes after a phase closes; never v2+ without a reason (it changes the module path). Compatibility covers only the public API (`tela`, `h`, CLI flags, the `Preview()`/`Previews` contract, the `tela inspect` JSON). `CHANGELOG.md` (Keep a Changelog) is curated by hand with an Unreleased section; `tela version` reads the version from the build information. Tags are annotated, created only with the owner's approval, and never pushed by agents

## Rationale

Phases are the natural release unit of this project; tags give `go get` and `tela version` meaningful versions once the repository is published
