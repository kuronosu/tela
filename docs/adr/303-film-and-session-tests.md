# ADR-303: Filmed motion and sessions as tests

- **Date:** 2026-11-20
- **Status:** Accepted. Amends §17, ADR-174, ADR-302.

## Decision

- **`telatest.App.Film(span, every, act) []FilmFrame`** (`FilmFrame{At, Image}`, at most `MaxFilmFrames` = 100): settles, runs `act` (may be nil), then captures a frame at 0 and every `every` of the app's clock up to `span`, without settling between them; the app settles normally afterwards.
- **`tela snapshot --film SPAN [--every DUR] [--apng file]`** films the first `SPAN` after the mount; **`tela mcp`'s `film`** films what an action starts. Both give a contact sheet (at most 24 frames, captioned with their time, at most 1600 px a side) and optionally an animated PNG written with the standard library (frames cut to the changed box).
- **`tela mcp`'s `test`** writes the session's acting calls as a `telatest` test: refs become `ByRole` (with `Nth` only for look-alikes), `ByText` or `ByTestID`, never node IDs; after each action, expectations on what changed in the snapshot (at most 8); then `go vet` and `go test -run` it and report. It only replaces files it wrote. Under `--attach`, `film` and `test` answer that they are unavailable.

## Why

Motion is invisible in one screenshot, and what an agent checked by hand should stay as a regression test. Checking what changed, not the whole snapshot, makes the test fail on behavior rather than layout.
