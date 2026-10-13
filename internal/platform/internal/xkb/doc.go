// Package xkb is a minimal pure-Go implementation of the parts of XKB a
// Wayland client needs (no libxkbcommon, no cgo):
//
//   - Keymap: the text keymap a compositor sends with wl_keyboard.keymap
//     (format xkb_v1, as written by libxkbcommon's
//     xkb_keymap_get_as_string: fully resolved xkb_keycodes, xkb_types,
//     xkb_compatibility and xkb_symbols sections).
//   - State: the modifier and group state the compositor reports with
//     wl_keyboard.modifiers, and the lookup of a key's keysym, logical key
//     (platform.Key) and text in that state.
//   - Compose: the libX11 Compose file format and the dead key / Compose
//     key state machine.
//
// # What it implements, and what it leaves to the compositor
//
// On Wayland the compositor runs the keyboard: it executes the XKB actions
// (SetMods, LockGroup, …) and sends the resulting modifier masks and
// effective group. A client only resolves keys with that state, so this
// package parses actions only as far as needed to skip them, and keeps:
//
//   - keycodes and aliases;
//   - virtual modifiers and their mapping to real modifiers, derived like
//     libxkbcommon does: explicit values, plus the modifier map of every
//     key whose interpretation (or explicit virtualMods) binds the virtual
//     modifier;
//   - key types (modifier mask, map and preserve entries, level count),
//     with libxkbcommon's automatic type choice for keys without one;
//   - per key: the keysyms of each group and level, the type of each
//     group, the out-of-range group policy (wrap, clamp, redirect), the
//     modifier map and whether the key repeats (explicit, or from the
//     matching interpretation, like libxkbcommon's ApplyInterpsToKey).
//
// The level of a key follows xkb_state_key_get_syms: the type entry whose
// modifiers equal the effective modifiers masked by the type's modifiers.
// The keysym then goes through the Caps Lock transformation of
// xkb_state_key_get_one_sym (uppercase when Lock is active and not
// consumed by the key's type). Lookups ignore Control (ADR-050: the
// logical key and the text reflect every modifier but Ctrl), so Ctrl never
// selects a level and never produces control characters.
//
// Statements the package does not need (geometry, indicators, actions,
// LED maps, key behaviors) are parsed for syntax and skipped. Unknown
// statements inside known sections are skipped; an error is returned only
// where skipping would change the meaning of a key (an unknown key field,
// an unknown key type, a malformed keysym list).
//
// # Verification
//
// The tests compare the package with libxkbcommon 1.13.2 on keymaps
// dumped by xkbcli for common layouts and options (testdata), over every
// key and a set of modifier states; testdata/oracle holds the small C
// program that produced the expected results (never built by Go).
//
// The keysym tables are generated from libxkbcommon's
// xkbcommon-keysyms.h (see gen_keysyms.go and THIRD_PARTY.md).
package xkb
