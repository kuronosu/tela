# ADR-219: Subagents read ARCHITECTURE by section

- **Date:** 2026-11-01
- **Status:** Accepted. Amends ADR-165.

## Decision

The project subagents (`tela-hard`, `tela-core`, `tela-impl`) no longer read `docs/ARCHITECTURE.md` in full. They always read §1–§5 (what Tela is, hard constraints, principles, glossary, layers) and §19 (conventions). Beyond those, they read the sections their brief names and the matching rows of §20. They read any other section once their work reaches its subject. They still read the ADR index and the ADRs their task touches. The main session keeps reading ARCHITECTURE in full, and every brief names the sections the task needs (AGENTS.md, "Brief").

## Rationale

ARCHITECTURE has grown to about 16k tokens, and every agent read all of it before starting: about 25k tokens per agent with the ADR index, and several agents per phase. The rules an agent must never break are in §2, §5 and §19, and the rest is per-subject design that a task touches one or two sections of. Reading those costs about 6–8k tokens. The main session, which plans and verifies, still sees the whole design, so a rule from an unread section is caught when it checks the work.

## What it changes

Step 1 of the three agent definitions, and the "Brief" step of AGENTS.md's delegation workflow.
