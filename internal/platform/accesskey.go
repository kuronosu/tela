package platform

import "unicode"

// AccessKeyMark is the access key of a menu item as its label shows it
// (AutoAccessKeys): the character at byte offset At of the label, or,
// when At is -1, Key appended to the label as "(K)"; neither (At -1, Key
// 0) for none.
type AccessKeyMark struct {
	At  int
	Key rune
}

// None reports whether the item has no access key.
func (m AccessKeyMark) None() bool { return m.At < 0 && m.Key == 0 }

// ExplicitAccessKey is the mark of an item's own AccessKey k (ADR-259):
// the first character of label equal to k in either case, else k appended
// (upper-cased, as "(F)").
func ExplicitAccessKey(label string, k rune) AccessKeyMark {
	lk := unicode.ToLower(k)
	for off, r := range label {
		if unicode.ToLower(r) == lk {
			return AccessKeyMark{At: off}
		}
	}
	return AccessKeyMark{At: -1, Key: unicode.ToUpper(k)}
}

// roleAccessKeys are the access keys of the standard items and menus in
// English Windows menus, which an appended key takes when it is free.
var roleAccessKeys = map[MenuRole]rune{
	RoleUndo: 'U', RoleRedo: 'R', RoleCut: 'T', RoleCopy: 'C',
	RolePaste: 'P', RoleSelectAll: 'A', RoleMinimize: 'N', RoleZoom: 'X',
	RoleClose: 'C', RoleEditMenu: 'E', RoleWindowMenu: 'W', RoleHelpMenu: 'H',
}

// appendAccessKeys are the keys an item without a character of its own
// may take, in order.
const appendAccessKeys = "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

// AutoAccessKeys returns the access key of each item of a menu level, as
// every level of a menu bar has them (the Win32 backend's native bars and
// the drawn bars of package aria, ADR-234, ADR-252, ADR-260), following
// Windows' convention per script:
//
//   - An item's own AccessKey comes first (taken before the others
//     choose, even when two items share it).
//   - Another item takes the first character of its label that the user
//     types with Alt (CheckAccessKey) and that no earlier item of the same
//     menu took: a digit, or a letter of a script that keyboard layouts
//     type directly (Latin, Cyrillic, Greek, Arabic, Hebrew, Thai...).
//   - A label without such a character (Chinese, Japanese and Korean,
//     whose characters come from an IME, or symbols only) gets a Latin
//     letter appended in parentheses, as the native apps of those
//     languages do ("ファイル(&F)"): the letter the item's role has in
//     English menus (the Edit menu's E, Copy's C) when it is free, else
//     the first free one from A to Z, then a digit.
//   - A label whose own characters are all taken by earlier items keeps
//     none, as a native menu would not mark one key twice.
//
// Separators and disabled items without an AccessKey get none. The
// result is nil for an empty menu.
func AutoAccessKeys(items []MenuItem) []AccessKeyMark {
	if len(items) == 0 {
		return nil
	}
	out := make([]AccessKeyMark, len(items))
	taken := map[rune]bool{}
	for i, it := range items {
		out[i] = AccessKeyMark{At: -1}
		if k := it.AccessKey; k != 0 && !it.Separator {
			out[i] = ExplicitAccessKey(it.Label, k)
			taken[unicode.ToLower(k)] = true
		}
	}
	var bare []int // the items whose label has no character of its own
	for i, it := range items {
		if it.Separator || it.Disabled || it.AccessKey != 0 {
			continue
		}
		own := false
		for off, r := range it.Label {
			if !CheckAccessKey(r) {
				continue
			}
			own = true
			if k := unicode.ToLower(r); !taken[k] {
				taken[k], out[i].At = true, off
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
		if k := roleAccessKeys[items[i].Role]; k != 0 && !taken[unicode.ToLower(k)] {
			taken[unicode.ToLower(k)], out[i].Key = true, k
		}
	}
	for _, i := range bare {
		if out[i].Key != 0 {
			continue
		}
		for _, c := range appendAccessKeys {
			if k := unicode.ToLower(c); !taken[k] {
				taken[k], out[i].Key = true, c
				break
			}
		}
	}
	return out
}
