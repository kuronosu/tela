# ADR-165: ADRs in their own files, a short roadmap, reading on demand

- **Date:** 2026-10-25
- **Status:** Accepted
- **Amends:** AGENTS.md's session rules and the "do not rewrite ARCHITECTURE.md" rule (the move is mechanical, no ADR text changed)

## Decision

- The decision log leaves `docs/ARCHITECTURE.md` §21: every ADR is a file `docs/adr/NNN-title.md` (date, status, decision, rationale, text moved verbatim), indexed in `docs/adr/README.md` with one line each and the superseded ones marked. ARCHITECTURE keeps §1–§20 and a pointer.
- `docs/ROADMAP.md` keeps only what is current: status in a few lines, the next step, the known limitations still open (one list by area), and the phases not done. The finished phases' checklists and per-phase status paragraphs and the session log move to `docs/HISTORY.md`; a session appends its log line there.
- Sessions and agents read ARCHITECTURE (§1–§20) and the ROADMAP in full, the ADR index, and only the ADRs their task touches; nobody reads the whole ADR folder or HISTORY.
- New ADRs are short: the decision, why, what it changes (about 1 KB). Details go to doc comments and tests.

## Rationale

ARCHITECTURE had grown to 585 KB (about 150k tokens), 91% of it the ADR log, and AGENTS.md and the agent definitions told every session and every agent to read it in full; the ROADMAP added about 20k tokens, mostly history already in the CHANGELOG and the ADRs. A session of four agents spent most of its tokens before any work. After the split the design (§1–§20) is about 14k tokens and the roadmap a few thousand.
