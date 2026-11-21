# ADR-311: A wait moves the app's clock only for what is on it

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-307 (waits).

## Decision

Between two failed tries, a `telatest` wait moves the app's clock one frame while work is due or a frame is owed, to the next deadline when nothing is due before it (ADR-307), and not at all when nothing is on the clock: what the check waits for then comes from a goroutine, and only real time passes. The caret's blink is not a deadline a wait jumps to (`core.Tree.NextWaitDeadline` is `NextDeadline` without it); blinks on the way to another deadline still run. Settling reads the `UseAsync` runs in flight before the work, so a run that ends between the two reads is never missed.

## Why

`examples/notes`' goldens caught the editor's caret in its off phase under load: one try that failed while a goroutine was still running moved the clock to the caret's next blink, so the number of failed tries, and the machine's speed, chose the phase. A clock that moves only for work on it keeps a golden after a wait independent of real time.

## What it changes

`telatest`'s `idle` and `settle`, its doc comments; `core.Tree.NextWaitDeadline`.
