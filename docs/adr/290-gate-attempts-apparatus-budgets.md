# ADR-290: One gate, three attempts, apparatus by approval, counted budgets

- **Date:** 2026-11-16
- **Status:** Accepted. Amends ADR-219 (what agents run before finishing) and ADR-267 (where the limitations live).

## Decision

- **One gate.** `go run ./internal/gate` runs gofmt (tracked files), vet and build for three systems, the `telagpu` checks, staticcheck at a pinned version, `tela lint`, `go test -short` through Go's test cache, and the word budgets of the documents every session reads. It reports every failure, the time of each step, known flakes (a failing test named in the roadmap), changed goldens and added apparatus. CI's lint job runs the same program. Budget: 60 s warm after an edit of `internal/core` on the owner's machine. The full suite stays for the end of a batch, phase close and CI.
- **Three attempts per failure.** After three, the agent stops and reports each attempt (hypothesis, change, result) with a way out: a test that reproduces it, a roadmap task, or a decision for the main session. Regenerating goldens, raising a tolerance, skipping a test, lengthening a timeout or retrying inside a test is descoping, not a fix. The main session delegates one failure at most three times, then asks the owner.
- **Apparatus by approval.** Agents may remove apparatus (CI jobs, tools, agent instructions, `TELA_*` switches, skips, lint ignores, tolerance overrides, documents) and may add it only with the owner's approval; the gate lists it.
- **Budgets are trades.** Raising one names what was given up, in an ADR. Performance budgets are counted where the result is deterministic (idle frames, work and allocations per interaction, binary size), so they run in the gate; the rest are timed at phase close on the owner's machine (§18).
- **Limitations leave the roadmap.** Known limitations and live checks move to `docs/LIMITATIONS.md`, read only for the area a task works in; the roadmap (budget 6,000 words) keeps status, phases and the next step. Cutting them to fit lost about 60 distinct problems, so they moved instead.

## Why

Measured 2026-11-16 (12 threads): the full suite takes 185 s and `-short` 135 s, while a warm cached run takes 2 s; `gofmt -l .` takes 48 s because it walks `site/node_modules`; the local staticcheck was broken by a Go upgrade. Subagents ran five separate commands, and the documents read at every session start had reached about 19,400 words. Tela's determinism lets more budgets be counted instead of timed.

## What it changes

`internal/gate`, CI's lint job, `AGENTS.md` (Rules, Commands, Verify), the agent definitions' "Before finishing" and report, §5, §18, `docs/ROADMAP.md` and the new `docs/LIMITATIONS.md`.
