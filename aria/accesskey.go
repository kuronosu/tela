package aria

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/platform"
)

// AccessKey is the access key (mnemonic) of a menu entry and where its
// label shows it ([AccessKeys]): Windows and GTK underline it while the
// keyboard's cues are shown, Alt and the key open a menu bar's menu, and
// the key alone chooses an item of an open menu (ADR-260).
type AccessKey struct {
	// Key is the key in lower case; 0 for none.
	Key rune
	// At is the byte offset in the label of the character that shows the
	// key, or -1 when the label has none of its own: the key is then
	// appended to it as "(K)" ("ファイル(F)"), as the native menus of
	// Chinese, Japanese and Korean do.
	At int
}

// Split splits label around the character that shows the key: before,
// the character and after, so that the character can be underlined. An
// appended key gives the label and "(" as before, the key in upper case
// and ")". An entry with no key gives the label and two empty strings.
func (a AccessKey) Split(label string) (before, key, after string) {
	switch {
	case a.At >= 0 && a.At < len(label):
		_, n := utf8.DecodeRuneInString(label[a.At:])
		return label[:a.At], label[a.At : a.At+n], label[a.At+n:]
	case a.Key != 0 && a.At < 0:
		return label + "(", string(unicode.ToUpper(a.Key)), ")"
	}
	return label, "", ""
}

// AccessKeyItem is an entry of a menu level for [AccessKeys].
type AccessKeyItem struct {
	// Label is the entry's text.
	Label string
	// Key is the entry's own access key, a letter or a digit (kit's
	// MenuItem.AccessKey); 0 for none.
	Key rune
	// Role is the entry's standard role, whose letter in English menus an
	// appended key takes when it is free (Copy's C, the Edit menu's E).
	Role desktop.Role
	// Disabled entries get no key of their own choosing.
	Disabled bool
}

// AccessKeys returns the access key of each entry of one menu level (the
// triggers of a menu bar, or the items of one menu or submenu). With auto
// unset only an entry's own Key counts, as in a context menu or a menu
// button's menu (Windows' popup menus). With auto set every entry gets
// one, as every level of a menu bar has them, by the rule of the Win32
// backend's native bars (Windows' convention per script): an entry's own
// Key first, then the first character of its label that the user types
// with Alt and no earlier entry took (a digit, or a letter of a script
// that keyboard layouts type directly), and for a label with no such
// character (Chinese, Japanese, Korean) a Latin letter appended in
// parentheses: the role's English letter when it is free, else the first
// free one from A to Z, then a digit. A label whose characters are all
// taken, and a disabled entry without a Key, get none.
func AccessKeys(items []AccessKeyItem, auto bool) []AccessKey {
	out := make([]AccessKey, len(items))
	if auto {
		pi := make([]platform.MenuItem, len(items))
		for i, it := range items {
			pi[i] = platform.MenuItem{Label: it.Label, AccessKey: it.Key, Role: platform.MenuRole(it.Role), Disabled: it.Disabled}
		}
		for i, m := range platform.AutoAccessKeys(pi) {
			out[i] = accessKeyOf(items[i].Label, m)
		}
		return out
	}
	for i, it := range items {
		out[i] = AccessKey{At: -1}
		if it.Key != 0 {
			out[i] = accessKeyOf(it.Label, platform.ExplicitAccessKey(it.Label, it.Key))
		}
	}
	return out
}

// accessKeyOf is the AccessKey of a platform mark on label.
func accessKeyOf(label string, m platform.AccessKeyMark) AccessKey {
	switch {
	case m.At >= 0 && m.At < len(label):
		r, _ := utf8.DecodeRuneInString(label[m.At:])
		return AccessKey{Key: unicode.ToLower(r), At: m.At}
	case m.Key != 0:
		return AccessKey{Key: unicode.ToLower(m.Key), At: -1}
	}
	return AccessKey{At: -1}
}

// accessKeyMatches returns the entries among keys whose access key (key)
// is the one e types: the character of its logical key, in either case,
// else, when the layout types no ASCII character there, the letter or
// digit of its physical key ("KeyF", "Digit1"), so that Alt+F reaches
// "&File" on a Cyrillic or Greek layout too, as Chromium matches
// Windows' virtual-key codes.
func accessKeyMatches[K comparable](keys []K, key func(K) rune, usable func(K) bool, e tela.KeyboardEvent) []K {
	if key == nil {
		return nil
	}
	match := func(r rune) []K {
		var out []K
		for _, k := range keys {
			if c := key(k); c != 0 && unicode.ToLower(c) == r && usable(k) {
				out = append(out, k)
			}
		}
		return out
	}
	s := string(e.Key)
	if r, n := utf8.DecodeRuneInString(s); n > 0 && n == len(s) && unicode.IsPrint(r) && r != ' ' {
		if m := match(unicode.ToLower(r)); len(m) > 0 || r < utf8.RuneSelf {
			// A layout that types Latin there (AZERTY's A on the key
			// of Q) means its own letter.
			return m
		}
	}
	code := string(e.Code)
	switch {
	case strings.HasPrefix(code, "Key") && len(code) == 4 && code[3] >= 'A' && code[3] <= 'Z':
		return match(unicode.ToLower(rune(code[3])))
	case strings.HasPrefix(code, "Digit") && len(code) == 6 && code[5] >= '0' && code[5] <= '9':
		return match(rune(code[5]))
	}
	return nil
}
