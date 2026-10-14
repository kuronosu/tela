package win32

import (
	"slices"
	"strings"
	"time"
	"unicode"
	utf16pkg "unicode/utf16"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// The keyboard translation turns the messages of one key press into one
// platform.KeyEvent. Windows spreads a key over several messages, which a
// KeyEvent needs all of (ADR-050, winit's model):
//
//	WM_KEYDOWN / WM_SYSKEYDOWN     the physical key (scan code), the virtual
//	                               key, the repeat flag
//	WM_DEADCHAR / WM_SYSDEADCHAR   (dead keys) the key only starts a sequence
//	WM_CHAR / WM_SYSCHAR           zero or more UTF-16 code units of text
//
// TranslateMessage generates the character messages and posts them before
// the key message is dispatched, so when the key message arrives its
// characters are already waiting in the queue, ahead of any other posted or
// input message. The translator peeks at the queue to know whether
// characters follow: if so the press stays pending and the character
// messages complete it; if not it is delivered at once.
//
// This file is the OS-independent half: it decides, from the facts of the
// messages and a few answers of the OS (keyEnv), which events exist and what
// they carry. keyboard_windows.go supplies the OS. The contract being
// implemented is documented in package platform ("Keyboard").
//
//   - Code is the physical key from the scan code (scancode.go).
//   - Key is the character the key produces with every modifier but Ctrl and
//     Alt, asked from the layout (ToUnicodeEx on a copy of the keyboard
//     state, without Ctrl and Alt unless AltGr, and without changing the
//     kernel's dead-key state); or the name of the virtual key (vkey.go).
//     After a dead key the character messages give the composed text, which
//     replaces it (ADR-056 and winit: Key "é").
//   - Text is what the character messages carry (so a dead key and its
//     partner type "´x" when they do not compose), or, when Ctrl is held
//     (Windows turns the letters into control characters), the layout's
//     character. Control characters are never reported.
//   - AltGr arrives as a fake left Ctrl immediately followed by the right
//     Alt with the same message time. The fake Ctrl produces no event, and
//     neither Ctrl nor Alt is ever reported for AltGr (Mods included).
//   - A lone Alt or F10 would enter the menu loop of a window that has no
//     menu: the caller swallows the system key messages, except Alt+F4 and
//     Alt+Space (the system menu), which down reports for DefWindowProc.

// charKind says what the layout produces for a key.
type charKind uint8

const (
	charNone charKind = iota // no character
	charText                 // text
	charDead                 // a dead key; the accompanying text is its spacing accent
)

// keyMsg is a key message that is waiting in the queue after the one being
// handled.
type keyMsg struct {
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
}

// keyEnv is what the translation asks of the OS.
type keyEnv interface {
	now() time.Duration
	// modifiers returns the Shift, Ctrl, Alt and Super keys held, with
	// AltGr reported as neither Ctrl nor Alt.
	modifiers() platform.Modifiers
	// altGrLayout reports whether the active layout has an AltGr key.
	altGrLayout() bool
	// lang is the primary language id of the active layout.
	lang() uint16
	// scancodeOf maps a virtual key to its extended scan code (0 if none).
	scancodeOf(vk uint32) uint32
	// char asks the layout what vk produces at scan code sc (the low
	// byte) with the Shift and CapsLock state of the keyboard, without Ctrl
	// and Alt unless AltGr is held, and without disturbing the dead-key
	// state.
	char(vk, sc uint32) (string, charKind)
	// peekKey returns the next WM_KEYDOWN, WM_KEYUP, WM_SYSKEYDOWN or
	// WM_SYSKEYUP in the queue of the window without removing it.
	peekKey() (keyMsg, bool)
	// peekChar returns the next WM_CHAR, WM_SYSCHAR, WM_DEADCHAR or
	// WM_SYSDEADCHAR in the queue of the window without removing it, whatever
	// key message is queued before it: TranslateMessage posts the characters
	// of a key press, but a peek that looks at the input queue first (the
	// release of the key may be waiting there already) would not see them.
	peekChar() (keyMsg, bool)
	// messageTime is the time of the message being handled.
	messageTime() uint32
	// keyDown reports whether the virtual key is physically down now.
	keyDown(vk uint32) bool
}

// pendingPress is a press waiting for its character messages.
type pendingPress struct {
	ev    platform.KeyEvent // everything but Key and Text
	vk    uint32
	cand  string   // the layout's character ("" for none or dead)
	dead  bool     // the layout says the key is dead
	ctrl  bool     // Ctrl or Super is held: Text comes from the layout
	units []uint16 // the code units of the WM_CHAR messages
	pack  bool     // a VK_PACKET (injected Unicode text): no layout
	named bool     // the key has a name (Enter, an arrow): Key is set
	dmsg  bool     // a dead-char message was seen: a dead key
}

// heldKey is what the translator remembers of a delivered press: enough to
// ask whether the key is still down and to write its release.
type heldKey struct {
	code platform.KeyCode
	vk   uint32
	key  platform.Key
	loc  platform.KeyLocation
}

// keyTranslator is the keyboard state of one window.
type keyTranslator struct {
	emit    func(platform.KeyEvent)
	held    keyboard.Held // the contract: paired presses and releases
	keys    []heldKey     // the delivered presses without release, in press order
	pending *pendingPress
}

func newKeyTranslator(emit func(platform.KeyEvent)) *keyTranslator {
	return &keyTranslator{emit: emit}
}

// keyInfo identifies the physical key of a key message.
type keyInfo struct {
	vk   uint32
	sc   uint32
	code platform.KeyCode
}

func (k *keyTranslator) identify(env keyEnv, wparam, lparam uintptr) keyInfo {
	vk := uint32(wparam & 0xFF)
	sc := scancodeFromLParam(lparam)
	if sc == 0 && vk != vkPacket {
		// Media keys and some remote desktops report none.
		sc = env.scancodeOf(vk)
	}
	if vk == vkPacket {
		return keyInfo{vk: vk, code: platform.UnidentifiedKeyCode(0x10000 | uint32(hiword(wparam)))}
	}
	return keyInfo{vk: vk, sc: sc, code: codeFromScancode(sc)}
}

// isFakeCtrl reports whether a left Ctrl message is the fake one that
// Windows sends before the right Alt of AltGr: the next key message is the
// same kind for the right Alt and carries the same time.
func isFakeCtrl(env keyEnv, ki keyInfo, msg uint32) bool {
	if ki.vk != vkControl || ki.sc != 0x1D || !env.altGrLayout() {
		return false
	}
	n, ok := env.peekKey()
	return ok && n.message == msg && scancodeFromLParam(n.lparam) == 0xE038 && n.time == env.messageTime()
}

// scKeyMenu is the WM_SYSCOMMAND code (SC_KEYMENU) that DefWindowProc
// generates when a lone Alt, F10 or Alt and a character selects a menu.
const scKeyMenu = 0xF100

// swallowSysCommand reports whether a WM_SYSCOMMAND must not reach
// DefWindowProc. The window has no menu bar, so the keyboard commands that
// would enter the menu loop (a lone Alt, F10, Alt and a mnemonic, which also
// beeps) are dropped; the window (system) menu on Alt+Space, SC_KEYMENU with
// the space as its character, is kept.
func swallowSysCommand(wparam, lparam uintptr) bool {
	return wparam&0xFFF0 == scKeyMenu && lparam != ' '
}

// isSystemShortcut reports whether a key message is Alt+F4 or Alt+Space: the
// keys that the window system owns (closing the window, its menu) and that
// go to DefWindowProc, not to the app.
func isSystemShortcut(msg, vk uint32, lparam uintptr) bool {
	return msg == wmSysKeyDown && (vk == vkF4 || vk == vkSpace) && lparam&(1<<29) != 0
}

// down handles WM_KEYDOWN and WM_SYSKEYDOWN. It reports whether the message
// must go to DefWindowProc (Alt+F4 closes the window, Alt+Space opens the
// system menu) instead of being swallowed.
func (k *keyTranslator) down(env keyEnv, msg uint32, wparam, lparam uintptr) (pass bool) {
	k.finish(env)
	ki := k.identify(env, wparam, lparam)
	if isSystemShortcut(msg, ki.vk, lparam) {
		return true
	}
	if isFakeShift(ki.sc) || isFakeCtrl(env, ki, msg) {
		return false
	}
	repeat := keyRepeat(lparam)
	if repeat && nonRepeating(ki.code) {
		return false
	}
	ev := platform.KeyEvent{
		Action:   platform.KeyPress,
		Code:     ki.code,
		Location: ki.code.Location(),
		Repeat:   repeat,
		Mods:     env.modifiers(),
		Time:     env.now(),
	}
	p := &pendingPress{ev: ev, vk: ki.vk, ctrl: ev.Mods&(platform.ModCtrl|platform.ModSuper) != 0}
	switch key, ok := namedKey(ki.vk, ki.code, env.altGrLayout(), env.lang()); {
	case ok:
		// A named key's Key is its name. Windows still makes characters of
		// some of them (Enter is "\r", Backspace "\b"), which are not text;
		// but after a dead key it types the accent first, and that is.
		p.ev.Key, p.named = key, true
	case ki.vk == vkPacket:
		p.pack = true
	default:
		var kind charKind
		p.cand, kind = env.char(ki.vk, ki.sc)
		if kind == charNone && numpadVK(ki.vk) {
			p.cand, kind = numpadFallback(ki.vk), charText
		}
		if kind == charDead {
			p.dead, p.cand = true, ""
		}
	}
	k.pending = p
	if _, ok := env.peekChar(); !ok {
		k.finish(env)
	}
	return false
}

// char handles WM_CHAR and WM_SYSCHAR: a code unit of the pending press's
// text. A character without a pending press is not ours (an IME result, a
// PostMessage) and is left to the caller.
func (k *keyTranslator) char(env keyEnv, wparam uintptr) (handled bool) {
	p := k.pending
	if p == nil {
		return false
	}
	p.units = append(p.units, uint16(wparam))
	if n, ok := env.peekChar(); !ok || (n.message != wmChar && n.message != wmSysChar) {
		k.finish(env)
	}
	return true
}

// deadChar handles WM_DEADCHAR and WM_SYSDEADCHAR: the pending press is a
// dead key, and nothing else follows it.
func (k *keyTranslator) deadChar(env keyEnv) (handled bool) {
	p := k.pending
	if p == nil {
		return false
	}
	p.dmsg = true
	k.finish(env)
	return true
}

// finish completes the pending press, if there is one, with what its
// character messages said.
func (k *keyTranslator) finish(env keyEnv) {
	p := k.pending
	if p == nil {
		return
	}
	k.pending = nil
	ev := p.ev
	text := stripControls(string(utf16pkg.Decode(p.units)))
	switch {
	case p.named:
		// A key the IME consumed (VK_PROCESSKEY) never types: its
		// characters, if the IME passes any, belong to the composition.
		if !p.ctrl && ev.Key != platform.KeyProcess {
			ev.Text = text
		}
	case p.dmsg:
		// A dead key: the accent is not typed until the next key.
		ev.Key, ev.Text = platform.KeyDead, ""
	case p.pack:
		ev.Key, ev.Text = keyFromText(text, ""), text
		if text == "" {
			ev.Key = platform.KeyUnidentified
		}
	case p.ctrl || len(p.units) == 0:
		// Ctrl turns characters into control codes: use the layout's.
		ev.Text = p.cand
		ev.Key = keyFromText(p.cand, "")
		if p.dead {
			ev.Key = platform.KeyDead
		}
		if ev.Key == "" {
			ev.Key = platform.KeyUnidentified
		}
	default:
		ev.Text = text
		ev.Key = keyFromText(text, p.cand)
		if ev.Key == "" {
			// Only control characters (a layout quirk): the layout's key.
			ev.Key = keyFromText(p.cand, "")
			if p.dead {
				ev.Key = platform.KeyDead
			}
			if ev.Key == "" {
				ev.Key = platform.KeyUnidentified
			}
		}
	}
	k.accept(ev, p.vk)
}

// keyFromText chooses the logical key for the text a press typed. A single
// character is the key ("é" after a dead acute accent). More than one means
// that a dead key did not compose with this one and typed its accent before
// the character (Windows): the key is then what the layout says for it, the
// fallback (winit), or the first character of the text if the layout has
// none.
func keyFromText(text, fallback string) platform.Key {
	switch {
	case text == "":
		return platform.Key(fallback)
	case clusters(text) == 1:
		return platform.Key(text)
	case fallback != "":
		return platform.Key(fallback)
	}
	for i := range text {
		if i > 0 {
			return platform.Key(text[:i])
		}
	}
	return platform.Key(text)
}

// clusters approximates the number of user-perceived characters of text: the
// runes that do not extend the previous one (combining marks, joiners,
// variation selectors and what follows a joiner).
func clusters(text string) int {
	n := 0
	joined := false
	for _, r := range text {
		switch {
		case r == 0x200D: // ZWJ
			joined = true
			continue
		case joined, unicode.Is(unicode.M, r), r >= 0xFE00 && r <= 0xFE0F, r >= 0x1F3FB && r <= 0x1F3FF:
			joined = false
			continue
		}
		n++
	}
	return n
}

// stripControls removes the control characters (C0, DEL, C1): Enter, Tab,
// Backspace, Escape and Ctrl+letter are never text (ADR-050).
func stripControls(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool { return r < 0x20 || (r >= 0x7F && r < 0xA0) }

// up handles WM_KEYUP and WM_SYSKEYUP.
func (k *keyTranslator) up(env keyEnv, msg uint32, wparam, lparam uintptr) {
	k.finish(env)
	ki := k.identify(env, wparam, lparam)
	if isFakeShift(ki.sc) || isFakeCtrl(env, ki, msg) {
		return
	}
	ev := platform.KeyEvent{
		Action:   platform.KeyRelease,
		Code:     ki.code,
		Location: ki.code.Location(),
		Mods:     env.modifiers(),
		Time:     env.now(),
	}
	if key, ok := namedKey(ki.vk, ki.code, env.altGrLayout(), env.lang()); ok {
		ev.Key = key
	} else if h, ok := k.heldPress(ki.code); ok && h.key == platform.KeyDead {
		// A dead key's state is pending in the kernel while it is released,
		// and asking the layout again would answer with the accent.
		ev.Key = platform.KeyDead
	} else if ki.vk == vkPacket {
		ev.Key = platform.KeyUnidentified
	} else {
		text, kind := env.char(ki.vk, ki.sc)
		if kind == charNone && numpadVK(ki.vk) {
			text, kind = numpadFallback(ki.vk), charText
		}
		switch {
		case kind == charDead:
			ev.Key = platform.KeyDead
		case text == "":
			ev.Key = platform.KeyUnidentified
		default:
			ev.Key = keyFromText(stripControls(text), "")
			if ev.Key == "" {
				ev.Key = platform.KeyUnidentified
			}
		}
	}
	k.accept(ev, ki.vk)
}

// heldPress returns what the translator remembers of a held key.
func (k *keyTranslator) heldPress(c platform.KeyCode) (heldKey, bool) {
	i := slices.IndexFunc(k.keys, func(h heldKey) bool { return h.code == c })
	if i < 0 {
		return heldKey{}, false
	}
	return k.keys[i], true
}

// accept delivers ev if the contract allows it: a release needs a delivered
// press of the same key, and a repeat needs the key to be held.
func (k *keyTranslator) accept(ev platform.KeyEvent, vk uint32) {
	if !k.held.Accept(ev) {
		return
	}
	i := slices.IndexFunc(k.keys, func(h heldKey) bool { return h.code == ev.Code })
	switch {
	case ev.Action == platform.KeyRelease:
		k.keys = slices.Delete(k.keys, i, i+1)
	case i >= 0:
		k.keys[i].key = ev.Key // a press of a held key replaces its entry
	default:
		k.keys = append(k.keys, heldKey{code: ev.Code, vk: vk, key: ev.Key, loc: ev.Location})
	}
	k.emit(ev)
}

// focusLost delivers the synthetic releases of the held keys, in press order.
// The caller delivers the FocusEvent after them.
func (k *keyTranslator) focusLost(env keyEnv) {
	k.finish(env)
	for _, ev := range k.held.ReleaseAll(env.now()) {
		k.emit(ev)
	}
	k.keys = nil
}

// releaseStale delivers a synthetic release, in press order, for every held
// key that is no longer down: a modal loop of the system (the window menu, a
// move or resize) swallowed its release.
func (k *keyTranslator) releaseStale(env keyEnv) {
	k.finish(env)
	for _, h := range slices.Clone(k.keys) {
		if env.keyDown(h.vk) {
			continue
		}
		ev := platform.KeyEvent{
			Action:    platform.KeyRelease,
			Code:      h.code,
			Key:       h.key,
			Location:  h.loc,
			Synthetic: true,
			Time:      env.now(),
		}
		if k.held.Accept(ev) {
			i := slices.IndexFunc(k.keys, func(e heldKey) bool { return e.code == h.code })
			k.keys = slices.Delete(k.keys, i, i+1)
			k.emit(ev)
		}
	}
}
