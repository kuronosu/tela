# ADR-220: The main session may push to a work branch

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-208 ("agents never push").

## Decision

By default nothing is pushed: the owner pushes `main` and the tags. When the owner allows it in a session, the main session may push commits to a work branch, never `main`, so CI runs there (`ci.yml` runs on every branch; the site deploys only from `main` and tags). It watches the runs with `gh`. It never pushes tags, never force-pushes and never pushes `main`. Merging the branch into `main` stays local, and the owner pushes `main`. Subagents never push.

## Rationale

The macOS backend is verified only on `macos-latest` CI (ADR-218), and the owner has no Mac. If every iteration waited for the owner to push, each failure would cost a round trip through the owner. A work branch keeps `main`, the website and the releases where the owner alone moves them.

## What it changes

AGENTS.md: the end-of-phase step and a "Pushing" rule.
