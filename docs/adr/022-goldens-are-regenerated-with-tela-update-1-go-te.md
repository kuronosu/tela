# ADR-022: Goldens are regenerated with `TELA_UPDATE=1 go test ./...`, not an `-update` flag

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Goldens are regenerated with `TELA_UPDATE=1 go test ./...`, not an `-update` flag

## Rationale

`go test ./... -update` fails in every test package that does not define the flag
