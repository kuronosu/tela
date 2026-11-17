# ADR-293: `tela lint` reads its dependencies from export data

- **Date:** 2026-11-17
- **Status:** Accepted. Amends ADR-173 (how imports are type-checked).

## Decision

The packages a lint run depends on but does not lint (and that import no linted package) are read from the go command's compiled export data (`go list -export`, `go/importer` for `gc`), still with the standard library only. One importer per set of export files is kept for the process, keyed by the files, which the go command names by content: a changed source, build tag, GOOS or GOARCH is a new key. Linted packages, and every package that imports one, are type-checked from source as before. A dependency without export data (it does not compile, `go list -export` fails, the file cannot be read) falls back to the source path, whose results are unchanged.

## Why

Type-checking about 250 dependencies from source took about 1.3 s per run, and the lint tests paid it in every test. With export data `internal/lint`'s tests went from 8 s to 3 s and `cmd/tela`'s from 6 s to 1.5 s (ADR-290's gate). Findings, `-json`, `-fix` and type-error messages are byte-identical over the repository and the fixtures.

## What it changes

`internal/lint/export.go`; a cold build cache now compiles the standard library before the first lint.
