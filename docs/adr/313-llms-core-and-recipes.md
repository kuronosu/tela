# ADR-313: `llms.txt` is a core and recipes

- **Date:** 2026-11-22
- **Status:** Accepted. Amends ADR-177 (one file under 100 KB).

## Decision

- **Layout.** `docs/llms.txt` is the core every agent reads, at most 36,000 bytes (about 9k tokens): start, the agent loop (with `tela check`), elements, components and hooks, styling, kit's rules, testing basics, the index and the pitfalls. `docs/llms/<name>.md` are recipes read for one question (kit, settings-screen, components, styling, animation, windows, desktop-integration, testing, accessibility, i18n, dev), each with a title, a "Questions:" line and its own limit in `llmsdoc`'s `recipeMaxBytes`. The core's `## Recipes` lists them, one line each.
- **Served** at `telaui.dev/llms.txt`, `/llms/<name>.md` and `/llms-full.txt` (everything in one, for tools that cannot follow links; written by the site build, not committed). `tela new`'s AGENTS.md and `tela mcp`'s instructions name the core and the recipes.
- **Tests** (`internal/gen/llmsdoc`): sizes; the index against the files; every ```go block of every file compiles and passes `tela lint`; the kit catalog; the settings screen.

## Why

The file had reached its limit and an agent read all of it for a one-screen question; new features had nowhere to go.

## What it changes

`docs/llms.txt`, `docs/llms/`, `internal/gen/llmsdoc`, the site's sync script and workflow paths, `tela new`'s AGENTS.md, `tela mcp`'s instructions, §17.
