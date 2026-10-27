# ADR-184: `tela dev` with phase-12 windows and desktop services

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Amends ADR-112 and ADR-115; remote protocol version 3 (details in `internal/platform/remote`).

- **Forwarded:** every `WindowOptions` field (parent as an ID), every window control request, hit regions, icons, restore tokens. A pending child's actions (maximize, move, activate…) are dropped; its state requests (visible, on top, limits, icon, regions) apply at promotion. The host keeps each real window's latest state for children that map onto it later; notification and tray events go to the active child.
- **Child windows by key:** core keys a `tela.Window` by its element path (ADR-113); the main window matches by ID. A new build takes over the real window with its key if frameless, fixed size, modal and parent are unchanged, else opens anew; unclaimed real windows close at promotion.
- **Close to tray:** a child whose windows are all hidden is ready without a frame; a build that keeps state adopts the hidden real windows.
- **Services** are the host's. Tray and notification IDs are the real ones, held by claims: a starting build's k-th `NewTray` takes the active build's k-th tray, an equal `Notify` returns the live one; unclaimed trays go at promotion.

## Rationale

Element paths already identify state across builds; claims stop effects that run again at mount from duplicating icons.
