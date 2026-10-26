# ADR-177: `docs/llms.txt` and how it is kept true

- **Date:** 2026-10-26
- **Status:** Accepted

## Decision

`docs/llms.txt` is one self-contained guide for LLMs (llmstxt.org's shape: an H1, a summary, sections; links only as "more" at the end): the API, the agent loop of `tela mcp`, the `kit` catalog, a complete settings screen, pitfalls and testing (phase 11). It stays under 100 KB (~25k tokens). **Kept true by tests** in `internal/gen/llmsdoc` (test-only, like a tool; it imports `internal/lint`, used otherwise only by `cmd/tela`): every ```go block is type-checked against the module and linted with `tela lint`'s rules (classes, roles, aria tokens, keys, hooks); a block is a whole file (it has a package clause), declarations (wrapped with the usual imports, each used once) or a component body (wrapped in `func snippetBody() tela.Node`); wrong code goes in ```text blocks. The import of the separate module `icons/lucide` is mapped to `examples/llms/settings/lucide`, a subset with the same names, so blocks use its icons. The settings screen is `examples/llms/settings/main.go` verbatim but for that import (with its own goldens). Every struct component of `kit` must be named, and each must have a catalog line `` - `kit.Name{Field, ...}` `` whose fields exist.

## Rationale

The phase-11 criterion gives an agent only `llms.txt` and `tela mcp`, so the file must compile as written; checking it with the linter's loader needs no new code to load packages, and blocks written under `testdata` while the test runs are invisible to `./...`.
