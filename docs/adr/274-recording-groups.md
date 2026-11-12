# ADR-274: Groups: recording proportional to the change

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-204 (and how ADR-081's corrections, ADR-082's recolored text and ADR-250's gradient are recorded).

## Decision

- **Groups** (`internal/core/record.go`): a box whose painting weighs 3 or more (the boxes it paints in its list outside groups, a drawing as 2, and the participants its stacking context paints) is a `group` layer inside the list that paints it, with that list's space, interest rect and culling, and stays one while it paints more than itself. Decided when boxes are placed, so full and partial recordings make the same groups and compose the same commands. A group whose box moves to another list is adopted there with its groups when its old list is being recorded or cannot be kept; unopened layers are freed at the end of the paint.
- A stacking context reads the children of the boxes it collects through as a dependency of its own, not as their placement.
- No more full recordings for a mounted `TextColorFn` (the text and drawings under a text color, or an inheritance of one, that changes are stamped), a box that starts or stops being a layer, a scroll container whose content starts or stops moving (the boxes are placed again), and layout-animation corrections (compared as inputs).
- `sameStyle` compares the slices a style computed again makes anew by content and deferred props by phase and kind.

## Why

Measured (loaded machine, medians of 5 interleaved runs; commands recorded per frame, pipeline time): the dashboard's rail hover (1080p x1) 475 → 17, 620 → 393 µs. `BenchmarkRecordingFollowsTheChange` (~190 commands in the root list): a hover 162 → 2.6, 343 → 67 µs; a hoisted badge mounting 185 → 24, 509 → 225 µs; a `TextColorFn` mounting 188 → 6, 380 → 88 µs, its Value 192 → 6, 337 → 28 µs; a box becoming a layer 193 → 51, 372 → 80 µs; a layout animation with corrections 192 → 11, 341 → 26 µs; equal `BgFn`, `BackgroundGradientFn` and `TranslateFn` built again 8 → 0.

## What it changes

The dashboard's overview has 121 groups; a frame that records everything allocates 17% more objects (+5% bytes, time within the noise: 2.95 → 2.97 ms); a box is 40 bytes larger, a layer 64. `Tree.Layers` lists groups (the layers overlay counts them with the layer that holds them). Same pixels: `TELA_CHECK_LAYERS=1` and the damage checks pass in core, kit and the dashboard.
