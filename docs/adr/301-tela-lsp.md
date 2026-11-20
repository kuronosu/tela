# ADR-301: `tela lsp`

- **Date:** 2026-11-20
- **Status:** Accepted. Amends §17, ADR-173.

## Decision

`tela lsp [-v]` is a Language Server Protocol server over stdio (`Content-Length` framing, JSON-RPC 2.0 on `encoding/json`, UTF-16 positions) that editors run beside gopls for the class strings of Go files: the arguments of `Class`, `Exit` and `RootClass`, fields named `Class` or `…Class`, `ClassAliases` values, and any literal the type checker finds in a class position. **Diagnostics** are `tela lint`'s findings for the open packages, from the unsaved buffers (an overlay for the parser and `go list -overlay`), lenient with type errors, debounced 300 ms; **completion** lists utilities, theme keys, stacked variants, the package's aliases and arbitrary-value snippets, each with its CSS; **hover** shows the CSS a class produces and when it applies, an alias's expansion, or the unknown-class message. The CSS comes from `style.(*Parser).Describe`, a table from class names and the theme (about 61% of the vocabulary; the rest show the utility name).

## Why

Agents get class errors from `tela lint` and `tela mcp`; people typing in an editor need them as they type, with the Tailwind IntelliSense loop they know. Reusing the lint engine keeps one checker.
