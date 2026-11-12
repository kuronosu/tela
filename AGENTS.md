# Agent instructions

Tela is a pure-Go desktop UI framework: no cgo, no webview, React-like API, Tailwind-style styling.

## At the start of every session

1. Read `docs/ARCHITECTURE.md` in full (§1–§20, about 16k tokens). It is the source of truth for the design.
2. Read `docs/ROADMAP.md` to see the current status, the phases left (with the open limitations as their tasks) and the next step.
3. Before starting a task, read its references in ARCHITECTURE §20, the ADR index `docs/adr/README.md`, and only the ADRs the task touches (`docs/adr/NNN-*.md`). Do not read the whole `docs/adr/` folder or `docs/HISTORY.md` (ADR-165).

## At the end of every session

- In `docs/ROADMAP.md`: check off completed tasks; keep "Current status" to a few lines; add each limitation found as a task of the planned phase whose subject covers it, or else to "Known limitations" (small gaps, non-blocking) or "By design or outside Tela"; never open a new phase to collect them, only the owner opens one (ADR-267); remove what was fixed; put checks that need hardware the project lacks under "Live checks"; update "Next step" (ADR-218).
- Add user-visible changes to the `Unreleased` section of `CHANGELOG.md`.
- Append one line to the session log at the end of `docs/HISTORY.md`.
- Do not record CI results or what was pushed (run outcomes, "green on <hash>", pushed hashes) in the roadmap, and never make a commit only to note them: every commit runs CI again. A CI failure that needs work becomes a task.
- When a phase starts, it takes the next number; future phases have none. When a phase is done, move its section from the roadmap to HISTORY.md.

## At the end of a phase

After the phase's done criteria pass and its review fixes are in:
- Move the `Unreleased` entries of `CHANGELOG.md` into a new version, with the date and the link line: the next minor (`v0.N.0`) if anything in it breaks the public API, otherwise the next patch (`v0.N.M`); the owner may release a small break, such as a rename, as a patch (ask).
- Propose the tag to the owner; once approved, create it annotated on the closing commit (`git tag -a v0.N.M -m "Phase N: <name>"`). Never push tags, and never push `main` (see "Pushing" under Rules).

Fixes and work between phases are released the same way, with the same rule for the number. See ADR-049 and ADR-208.

## Rules

- **Never use cgo.** Run everything with `CGO_ENABLED=0`; a dependency that needs cgo is not acceptable. The one exception is the race detector, which needs cgo: `CGO_ENABLED=1 go test -race` is allowed as a verification tool only (the main session runs it on packages with concurrency, such as `internal/core`, `tela` and `internal/platform/...`); nothing is ever built or shipped with cgo.
- **Approved dependencies only.** See ARCHITECTURE §2; a new one needs an ADR.
- **Ported code** keeps its original copyright notice and is listed in `THIRD_PARTY.md`.
- **Do not rewrite `ARCHITECTURE.md`.** To change a decision, add an ADR file `docs/adr/NNN-title.md` with the next number, add its row to `docs/adr/README.md`, and minimally adjust the affected section. ADRs are short (decision, why, what it changes; about 1 KB): details go to doc comments and tests. Small fixes (typos, clarifications) are fine.
- **Respect layer dependency rules** (ARCHITECTURE §5). Nothing in `internal/` becomes public without an ADR.
- **Everything in English:** code, comments, commits and docs.
- **Visual work is tested with headless goldens.** When you change rendering, generate a snapshot and look at it.
- **Pushing (ADR-220).** By default nothing is pushed: the owner pushes. When the owner allows it in a session, the main session may push commits to a work branch (never `main`) to run CI there, and watch the runs with `gh` (`gh run watch`, `gh run view --log-failed`). Never push tags, never force-push, never push `main`; merging the branch into `main` stays local, and the owner pushes it. Subagents never push.

## Delegation (Claude Code)

The main session plans, delegates, verifies and updates the roadmap. The owner picks its model (Opus or Fable). Implementation goes to the project subagents in `.claude/agents/`, which have model and effort fixed in their frontmatter:

| Agent | Model / effort | Use for |
|---|---|---|
| `tela-hard` | Opus / xhigh | Text layout (Parley-style), flex/block/grid vs Taffy fixtures, `core` (trees, reconciler, scheduler), hooks, `UseValue`/prop skipping, GPU backend |
| `tela-core` | Opus / high | Rasterizer, shadows/blur, platform interface (`internal/platform`), Wayland/X11/Cocoa backends, XKB, gestures, focus, scrolling/damage, IME and text editing, layout animations, stacking contexts, accessibility backends |
| `tela-impl` | Sonnet / high | `gfx` basic types, Wayland XML generator, Tailwind generator and class parser, `tela`/`h` API surface, Win32 backend, transitions/springs, filters, `telatest`, debug overlays, `aria`, `kit`, CLI, `llms.txt`, examples |
| `tela-chore` | Haiku / low | Boilerplate, renames, lint fixes, CI config, codebase searches, roadmap updates |

A phase in `docs/ROADMAP.md` may name a model and effort for its agents' tasks (for example the GPU backend: Fable / high). Pass them as `model` and `effort` in each Agent call of that phase instead of editing the frontmatter, which stays the default.

Main-session workflow per task:

1. **Plan.** Split the phase into tasks of a size worth a fresh agent (one package or one algorithm). Don't delegate tiny edits; do them directly.
2. **Brief.** Give each agent a self-contained prompt: goal, files/packages in scope, the ARCHITECTURE sections to read (agents always read §1–§5 and §19, then only those, ADR-219), done criteria. Agents start without conversation context.
3. **Sequence.** Run agents sequentially by default, because layers depend on each other. Run in parallel only for independent packages, each with `isolation: "worktree"`, then integrate.
4. **Verify.** Never accept a report at face value: rerun the build/vet/test/cross-compile commands below and look at any snapshot.
5. **Record.** Update `docs/ROADMAP.md` yourself, or via `tela-chore`.

If an agent reports a design conflict, the main session decides. If the decision changes ARCHITECTURE, record it as an ADR.

## Commands

```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 staticcheck ./...                     # lint; config in staticcheck.conf
CGO_ENABLED=0 go test ./...
TELA_UPDATE=1 CGO_ENABLED=0 go test ./...           # regenerate goldens (review the diff)
TELA_LIVE=1 CGO_ENABLED=0 go test ./internal/platform/...  # tests that open real windows
CGO_ENABLED=1 go test -race -timeout 30m ./internal/core/... . ./internal/platform/...  # race detector (verification only; internal/core takes ~15 min)
CGO_ENABLED=0 GOARCH=arm64 go test -short -timeout 40m -exec qemu-aarch64-static ./internal/gfx/...  # pixel determinism (ADR-027)

# Cross-compilation (must always build)
CGO_ENABLED=0 GOOS=windows go build ./...
CGO_ENABLED=0 GOOS=darwin  go build ./...

# Windows via Wine (if installed)
CGO_ENABLED=0 GOOS=windows go test -exec wine ./...
```
