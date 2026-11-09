package kit

import (
	"slices"
	"unicode"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// Access keys of the menus (ADR-260): the keys come from aria.AccessKeys
// and the labels show them underlined while the keyboard's cues are on.

// levelAccessKeys returns the access keys of the entries of one menu level
// (its items, radio items and submenu triggers, in order): every entry's
// with auto (a window's menu bar), else those with an AccessKey. Nil when
// none has one.
func levelAccessKeys[K comparable](flat *menuFlat[K], auto bool) map[K]aria.AccessKey {
	items := make([]aria.AccessKeyItem, len(flat.keys))
	has := auto
	for i, k := range flat.keys {
		it := flat.entries[k].item
		items[i] = aria.AccessKeyItem{Label: flat.text[k], Key: it.AccessKey, Role: it.Role, Disabled: slices.Contains(flat.disabled, k)}
		has = has || it.AccessKey != 0
	}
	if !has {
		return nil
	}
	out := map[K]aria.AccessKey{}
	for i, ak := range aria.AccessKeys(items, auto) {
		if ak.Key != 0 {
			out[flat.keys[i]] = ak
		}
	}
	return out
}

// accessLabel is a label with its access key ak: the key's character
// underlined by class while show is set. A key appended to the label
// ("ファイル(F)") is always there, as in the native menus of Chinese,
// Japanese and Korean, and only its letter's underline follows show. The
// parts are spans of one paragraph, shaped together (an underline does not
// change the shaping), so showing the cues moves nothing.
func accessLabel(label string, ak aria.AccessKey, show bool, class string) tela.Child {
	if ak.Key == 0 || (!show && ak.At >= 0) {
		return h.Text(label)
	}
	before, key, after := ak.Split(label)
	var mark tela.Child = h.Text(key)
	if show {
		mark = h.Span(cls(class), h.Text(key))
	}
	return h.Span(h.Text(before), mark, h.Text(after))
}

// validAccessKey reports whether k can be an AccessKey: an ASCII digit,
// or a letter of a script that keyboard layouts type directly (not Han,
// Hiragana, Katakana, Hangul, Bopomofo or Yi, which come from an IME, nor
// the fullwidth forms), desktop.MenuItem's rule.
func validAccessKey(k rune) bool {
	switch {
	case k >= '0' && k <= '9':
		return true
	case !unicode.IsLetter(k), k >= 0xff00 && k <= 0xffef, k >= 0x3000 && k <= 0x30ff:
		return false
	}
	return !unicode.In(k, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo, unicode.Yi)
}
