package win32

import (
	"math"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of window menus (menu_windows.go): the menu bar's
// WM_COMMAND decoding, its access keys and the popup menu's position. The
// entries come from buildMenu (tray.go), shared with tray menus.

// Mnemonics of the menu bar. The contract's labels are plain text (no
// '&'), so a bar gets its access keys from the backend, as every level of
// a native menu has them, following Windows' convention per script
// (ADR-234, ADR-252):
//
//   - An item takes the first character of its label that the user types
//     with Alt and that no earlier item of the same menu took: a digit, or
//     a letter of a script that keyboard layouts type directly (Latin,
//     Cyrillic, Greek, Arabic, Hebrew, Thai...). Russian apps mark their
//     own letters ("&Файл"), as English ones do ("&File"); Alt with that
//     key on the user's layout sends the letter (WM_SYSCHAR).
//   - A label without such a character (Chinese, Japanese and Korean,
//     whose characters come from an IME, or symbols only) gets a Latin
//     letter appended in parentheses, as the native apps of those
//     languages do ("ファイル(&F)", "文件(&F)", "파일(&F)"): the letter the
//     item's role has in English menus (the Edit menu's E, Copy's C) when
//     it is free, else the first free one from A to Z, then a digit. Their
//     layouts type Latin letters with Alt, with the IME on or off.
//   - A label whose own characters are all taken by earlier items keeps
//     none, as a native menu would not mark one key twice.
//
// Windows underlines the key when it shows access keys (the "Underline
// keyboard shortcuts and access keys" setting, or after Alt), Alt and the
// key opens the bar's menu, and inside an open menu the key chooses the
// item.

// isMnemonicRune reports whether r can be an access key of its own label:
// an ASCII digit, or a letter the user types with Alt on the layouts of
// its script. Letters of the scripts typed through an IME (Han, Hiragana,
// Katakana, Hangul, Bopomofo, Yi) and the fullwidth forms are not: their
// labels get an appended Latin key instead.
func isMnemonicRune(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case !unicode.IsLetter(r), r >= 0xff00 && r <= 0xffef:
		return false
	}
	return !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo, unicode.Yi)
}

// mnemonic is the access key of a menu item: the character at byte offset
// at of its label, or, when at is -1, key appended to the label as "(&K)";
// neither (at -1, key 0) for none.
type mnemonic struct {
	at  int
	key rune
}

// roleKeys are the access keys of the standard items and menus in English
// Windows menus, which an appended key takes when it is free.
var roleKeys = map[platform.MenuRole]rune{
	platform.RoleUndo: 'U', platform.RoleRedo: 'R', platform.RoleCut: 'T', platform.RoleCopy: 'C',
	platform.RolePaste: 'P', platform.RoleSelectAll: 'A', platform.RoleMinimize: 'N', platform.RoleZoom: 'X',
	platform.RoleClose: 'C', platform.RoleEditMenu: 'E', platform.RoleWindowMenu: 'W', platform.RoleHelpMenu: 'H',
}

// appendKeys are the keys an item without a character of its own may take,
// in order.
const appendKeys = "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

// autoMnemonics returns the access key of each item of a menu level (see
// "Mnemonics of the menu bar"): separators and disabled items get none.
// The result is nil for an empty menu.
func autoMnemonics(items []platform.MenuItem) []mnemonic {
	if len(items) == 0 {
		return nil
	}
	out := make([]mnemonic, len(items))
	taken := map[rune]bool{}
	var bare []int // the items whose label has no character of its own
	for i, it := range items {
		out[i] = mnemonic{at: -1}
		if it.Separator || it.Disabled {
			continue
		}
		own := false
		for off, r := range it.Label {
			if !isMnemonicRune(r) {
				continue
			}
			own = true
			if k := unicode.ToLower(r); !taken[k] {
				taken[k], out[i].at = true, off
				break
			}
		}
		if !own {
			bare = append(bare, i)
		}
	}
	// After every own key, so that an appended key never takes the letter
	// of a later label; the roles' letters first, so that an item before
	// the Edit menu does not take its E.
	for _, i := range bare {
		if k := roleKeys[items[i].Role]; k != 0 && !taken[unicode.ToLower(k)] {
			taken[unicode.ToLower(k)], out[i].key = true, k
		}
	}
	for _, i := range bare {
		if out[i].key != 0 {
			continue
		}
		for _, c := range appendKeys {
			if k := unicode.ToLower(c); !taken[k] {
				taken[k], out[i].key = true, c
				break
			}
		}
	}
	return out
}

// label returns the item's label s with its access key marked (an '&'
// before it, or "(&K)" appended), and the key in lower case (0 for none).
func (m mnemonic) label(s string) (string, rune) {
	switch {
	case m.at >= 0 && m.at < len(s):
		r, _ := utf8.DecodeRuneInString(s[m.at:])
		return menuLabelAt(s, m.at), unicode.ToLower(r)
	case m.key != 0:
		return menuLabel(s) + "(&" + string(m.key) + ")", unicode.ToLower(m.key)
	}
	return menuLabel(s), 0
}

// menuLabelAt is menuLabel with the character at byte offset at marked as
// the mnemonic (an offset below zero marks none).
func menuLabelAt(s string, at int) string {
	if at < 0 || at >= len(s) {
		return menuLabel(s)
	}
	return menuLabel(s[:at]) + "&" + menuLabel(s[at:])
}

// buildBarMenu is buildMenu for a menu bar: every level has access keys,
// and mnemonics lists the keys (lower-cased) of the bar's own items.
func buildBarMenu(items []platform.MenuItem) (entries []menuEntry, cmds []platform.MenuItemID, mnemonics []rune) {
	return buildMenuWith(items, true)
}

// hasMnemonic reports whether Alt and the character c (the WM_SYSCHAR of a
// key, in either case) opens one of the bar's menus.
func hasMnemonic(mnemonics []rune, c uintptr) bool {
	return slices.Contains(mnemonics, unicode.ToLower(rune(c)))
}

// menuCommand decodes WM_COMMAND for a menu bar whose command table is
// cmds (buildMenu): a menu sends 0 in the high word of wParam and no
// control handle in lParam (accelerator tables, which Tela does not use,
// send 1; controls send their handle). ok is false for any other command.
func menuCommand(cmds []platform.MenuItemID, wparam, lparam uintptr) (platform.MenuItemID, bool) {
	if hiword(wparam) != 0 || lparam != 0 {
		return 0, false
	}
	cmd := int(loword(wparam))
	if cmd < 1 || cmd > len(cmds) {
		return 0, false
	}
	return cmds[cmd-1], true
}

// popupCommand maps TrackPopupMenuEx's TPM_RETURNCMD result to the chosen
// item: 0 when the menu was dismissed (or the command is unknown).
func popupCommand(cmds []platform.MenuItemID, cmd uintptr) platform.MenuItemID {
	if cmd < 1 || cmd > uintptr(len(cmds)) {
		return 0
	}
	return cmds[cmd-1]
}

// physicalPoint converts a point in logical pixels to physical ones at
// scale, rounded to the nearest pixel (half away from zero, as
// PhysicalSize does).
func physicalPoint(p platform.Point, scale float64) (x, y int32) {
	return int32(math.Round(float64(p.X) * scale)), int32(math.Round(float64(p.Y) * scale))
}
