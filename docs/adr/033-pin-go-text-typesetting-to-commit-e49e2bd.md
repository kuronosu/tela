# ADR-033: Pin `go-text/typesetting` to commit e49e2bd

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Pin `go-text/typesetting` to commit e49e2bd (pseudo-version after v0.3.5) until a release contains it; move to the next tagged release when one exists

## Rationale

v0.3.5's HarfBuzz set digest skips covered lookup subtables, so Inter's class kerning and Noto Sans Arabic's `ccmp` never applied (Arabic did not join); the commit also gives exact bidi embedding levels. Same approved module, no new code paths
