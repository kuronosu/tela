# ADR-054: X11 input

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

X11 input (phase 4). The vendored xgb (ADR-025) reads GenericEvents whole, delivers events without a decoder raw and matches errors without a constructor to their request (a checked request no longer blocks forever). XInput2 ≥ 2.1 replaces core pointer events when present: master-pointer motion, buttons, enter/leave; scroll valuators read as in GTK 3/winit (baselines reset on enter and from device changes; emulated wheel buttons dropped, emulated buttons 1–3 kept until touch input exists); touchpads (by name or libinput/Synaptics property) report `Delta` at 15 logical px per increment, other devices fractional `Steps`. Keys are translated with an `xkb.Keymap` built from the core keyboard and modifier mappings (`xkb.NewCoreKeymap`: automatic types, virtual modifiers from the modifier map, AltGr never Alt); the XKB extension is used for state only (detectable auto-repeat, group, modifiers after each key, mapping changes), with a same-timestamp release/press merge when detectable auto-repeat is missing. A keyboard grab by another client releases held keys synthetically without a `FocusEvent`. With two layouts the core mapping cannot tell which owns the AltGr levels: the layout that has them types them, and the other may too; layouts 3 and 4 are not recovered

## Rationale

Same `KeyEvent` contract as Wayland without decoding the full XKB keymap (about 1,200 lines of binary decoding in libxkbcommon-x11); reading `XkbGetMap` would make multi-layout AltGr exact if Xwayland users need it
