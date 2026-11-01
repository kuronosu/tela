package xkb

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// AcceleratorKeysym returns the keysym of an accelerator's key
// (platform.Accelerator.Key): the keysym a global shortcut is bound to,
// by name in the XDG shortcuts format of the GlobalShortcuts portal
// (Keysym.String: "k", "F1", "Return", "XF86AudioPlay") or by keycode
// with XGrabKey. A named key gives its main keysym, never a keypad one
// (KeyEnter is Return, not KP_Enter; KeyPrintScreen is Print, not
// Sys_Req); a one-character key gives the keysym that types it ("k" is
// k, " " is space, "/" is slash, "é" is eacute, "€" is EuroSign). ok is
// false for keys without a keysym (KeyUnidentified, a modifier the
// accelerator rules exclude anyway, a string of several characters).
func AcceleratorKeysym(k platform.Key) (ks Keysym, ok bool) {
	if k.IsNamed() {
		ks, ok = namedKeyKeysyms()[k]
		return ks, ok
	}
	r, size := utf8.DecodeRuneInString(string(k))
	if r == utf8.RuneError || size != len(k) {
		return NoSymbol, false
	}
	ks = UTF32ToKeysym(r)
	return ks, ks != NoSymbol
}

// preferredKeysyms names the keysym of a named key that several keysyms
// produce, where the first non-keypad entry of namedKeysyms is not the
// key's usual one.
var preferredKeysyms = map[platform.Key]string{
	platform.KeyEnter:       "Return", // not Linefeed
	platform.KeyPrintScreen: "Print",  // not Sys_Req (Alt+Print)
	platform.KeyTab:         "Tab",    // not ISO_Left_Tab (Shift+Tab)
	platform.KeyClear:       "Clear",  // not Begin
}

var namedKeyKeysyms = sync.OnceValue(func() map[platform.Key]Keysym {
	m := make(map[platform.Key]Keysym, len(namedKeysyms))
	for _, n := range namedKeysyms {
		if _, ok := m[n.key]; ok || strings.HasPrefix(n.name, "KP_") {
			continue
		}
		if want, ok := preferredKeysyms[n.key]; ok && want != n.name {
			continue
		}
		ks, ok := KeysymFromName(n.name)
		if !ok {
			panic("xkb: unknown keysym name in namedKeysyms: " + n.name)
		}
		m[n.key] = ks
	}
	return m
})
