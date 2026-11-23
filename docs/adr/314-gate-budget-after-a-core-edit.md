# ADR-314: The gate's budget after an edit of `internal/core` is 200 s

- **Date:** 2026-11-22
- **Status:** Accepted. Amends ADR-290.

## Decision

The gate's budget warm after an edit of `internal/core` on the owner's machine (12 threads) is 200 s, not 60 s. No check is traded for time: vet and staticcheck keep running for every target (linux with and without `telagpu`, windows, darwin, arm64 `telagpu`, the notes module). After an edit of a package nothing depends on, the budget stays 10 s, timed at phase close (§18). `totalBudget` in `internal/gate/main.go` is 200 s; going over is still a warning.

## Why

Measured on 2026-11-22: 181–188 s after a body edit of `internal/core` and 174 s after an export-data change, about 1,800 s of CPU (vet 564 s, staticcheck 596 s, tests 550 s) with little duplicated (about 4%); the floor on 12 threads is about 150 s. Reaching 60 s needed dropping checks (vet and staticcheck for the other systems only on packages whose files differ from Linux's, or staticcheck for windows and darwin in CI only), which loses findings the owner wants before CI. What is given up is the agent's wait after a core edit.

## What it changes

ADR-290's budget line, ARCHITECTURE §18, `totalBudget`.
