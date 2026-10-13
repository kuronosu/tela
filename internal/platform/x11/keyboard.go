//go:build linux

package x11

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/evdev"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// kbd is the X11 keyboard: the keymap built from the core keyboard
// mapping, Compose, keyboard focus, the keys delivered as pressed, and
// the key event waiting for its modifiers. It follows the platform
// contract (package platform, "Keyboard") exactly like the Wayland
// backend, through the same internal/xkb lookups (State.Press, Repeat,
// Lookup) and internal/keyboard.Held.
//
// Quirks handled here (see also winit's and Gio's X11 keyboard code):
//
//   - Key translation uses the core mapping (GetKeyboardMapping and
//     GetModifierMapping, xkb.NewCoreKeymap), rebuilt after MappingNotify
//     or the XKB MapNotify / NewKeyboardNotify. The group comes from the
//     event state (bits 13-14, XKB-aware clients only) or, without XKB,
//     from the modifier bound to Mode_switch.
//   - Repeat comes from the X server. With XKB detectable auto-repeat a
//     held key sends presses only; a press of a key already delivered as
//     pressed is a repeat (Repeat true, Key and Text for the current
//     modifiers, no compose). Without it the server sends a release and
//     a press with the same timestamp per repeat: the release waits for
//     the next event and the pair becomes one repeat. Keys the keymap
//     marks as non-repeating never repeat.
//   - KeyEvent.Mods is the state after the event: the XKB StateNotify that
//     follows a modifier key gives it, so a key event waits (pending)
//     until the next event; if none comes in the same read, the Mods of a
//     modifier key are predicted from its modifier map, like Wayland.
//   - Focus: FocusIn and FocusOut, without the grab/ungrab pairs, decide
//     which window gets keys. On a real FocusOut the held keys get
//     synthetic releases before FocusEvent{false}. A keyboard grab by
//     another client (NotifyGrab: a window manager's Alt+Tab switcher)
//     keeps the window focused but takes the keyboard, so the held keys
//     get their synthetic releases then.
//   - Keys held when focus arrives produce no presses: KeymapNotify, which
//     follows FocusIn, lists the keys already down, and their server
//     repeats are dropped until they are released.
type kbd struct {
	b *Backend

	// fetch reads the core keyboard mapping (a round trip); nil in tests
	// that use no mapping.
	fetch   func() (xkb.CoreMapping, error)
	keymap  *xkb.Keymap // nil: keys only have physical codes
	state   *xkb.State  // lookup state, set from each event's state field
	loaded  bool        // fetch was tried
	dirty   bool        // the mapping changed since it was read
	compose *xkb.ComposeState

	composeLoaded bool
	composeTable  *xkb.ComposeTable
	composeLoader func() *xkb.ComposeTable // tests
	err           error                    // the last keyboard setup error

	focus *Window       // window with keyboard focus
	held  keyboard.Held // presses delivered to focus and not released
	stale [256]bool     // keys down before focus arrived

	// Pending key event: queued once the next event shows whether a
	// StateNotify belongs to it.
	pending    platform.KeyEvent
	pendingKC  byte
	pendingWin *Window
	hasPending bool

	// Without detectable auto-repeat: a release waiting to see whether a
	// press of the same key at the same time follows (a server repeat).
	release    xproto.KeyReleaseEvent
	hasRelease bool
}

// mappingChanged records that the keyboard mapping changed; the keymap is
// read again before the next use.
func (k *kbd) mappingChanged() { k.dirty = true }

// km returns the current keymap, reading the mapping if needed.
func (k *kbd) km() *xkb.Keymap {
	if k.loaded && !k.dirty {
		return k.keymap
	}
	k.loaded, k.dirty = true, false
	if k.fetch != nil {
		m, err := k.fetch()
		var km *xkb.Keymap
		if err == nil {
			km, err = xkb.NewCoreKeymap(m)
		}
		if err != nil {
			k.err = keyboard.Report(fmt.Errorf("x11: cannot read the keyboard mapping (keys have physical codes only): %w", err))
		} else {
			k.keymap, k.state = km, xkb.NewState(km)
		}
	}
	if k.keymap != nil {
		k.loadCompose()
		if k.compose != nil {
			k.compose.Reset()
		}
	}
	return k.keymap
}

// lookupState returns the lookup state for an event's state field: its
// modifiers, and the group of bits 13-14 (XKB) or of Mode_switch.
func (k *kbd) lookupState(state uint16) *xkb.State {
	mods := uint32(state & 0xff)
	var group uint32
	if k.b.xkb.ok {
		group = uint32(state>>13) & 3
	} else if ms, _ := k.keymap.ModMask("ModeSwitch"); ms != 0 && mods&ms != 0 {
		group = 1
	}
	k.state.UpdateMask(mods, 0, 0, group)
	return k.state
}

// mods returns the shortcut modifiers of a mask of real modifiers (an
// event's state field, or XInput2's base and latched modifiers) through
// the real modifier mapping: Alt and Super are the modifiers their keys
// are bound to, and AltGr is never Alt.
func (k *kbd) mods(real uint32) platform.Modifiers {
	if km := k.km(); km != nil {
		return km.ModifiersOf(real & 0xff)
	}
	// No mapping: the xkeyboard-config defaults.
	var m platform.Modifiers
	if real&xproto.KeyButMaskShift != 0 {
		m |= platform.ModShift
	}
	if real&xproto.KeyButMaskControl != 0 {
		m |= platform.ModCtrl
	}
	if real&xproto.KeyButMaskMod1 != 0 {
		m |= platform.ModAlt
	}
	if real&xproto.KeyButMaskMod4 != 0 {
		m |= platform.ModSuper
	}
	return m
}

// keyPress translates a core KeyPress.
func (k *kbd) keyPress(ev xproto.KeyPressEvent) {
	w := k.b.windows[ev.Event]
	kc := byte(ev.Detail)
	if w == nil || w != k.focus || k.stale[kc] {
		return
	}
	code := evdev.X11KeyCode(uint32(kc))
	repeat := k.held.IsHeld(code)
	km := k.km()
	if repeat && km != nil && !km.Repeats(uint32(kc)) {
		return
	}
	e := platform.KeyEvent{
		Action:   platform.KeyPress,
		Code:     code,
		Key:      platform.KeyUnidentified,
		Location: code.Location(),
		Repeat:   repeat,
		Mods:     k.mods(uint32(ev.State)),
		Time:     k.b.Now(),
	}
	if km != nil {
		st := k.lookupState(ev.State)
		var l xkb.Lookup
		if repeat {
			l = st.Repeat(uint32(kc), k.compose)
		} else {
			l = st.Press(uint32(kc), k.compose)
			e.Mods |= km.HeldModifiers(uint32(kc))
		}
		e.Key, e.Text = l.Key, l.Text
	}
	if k.held.Accept(e) {
		k.setPending(w, e, kc)
	}
}

// keyRelease translates a core KeyRelease.
func (k *kbd) keyRelease(ev xproto.KeyReleaseEvent) {
	if !k.b.xkb.detectable {
		// Maybe the first half of a server repeat: beforeEvent decides
		// with the next event.
		k.release, k.hasRelease = ev, true
		return
	}
	k.applyRelease(ev)
}

func (k *kbd) applyRelease(ev xproto.KeyReleaseEvent) {
	kc := byte(ev.Detail)
	k.stale[kc] = false
	w := k.b.windows[ev.Event]
	if w == nil || w != k.focus {
		return
	}
	code := evdev.X11KeyCode(uint32(kc))
	e := platform.KeyEvent{
		Action:   platform.KeyRelease,
		Code:     code,
		Key:      platform.KeyUnidentified,
		Location: code.Location(),
		Mods:     k.mods(uint32(ev.State)),
		Time:     k.b.Now(),
	}
	if km := k.km(); km != nil {
		e.Key = k.lookupState(ev.State).Lookup(uint32(kc)).Key
		e.Mods &^= km.HeldModifiers(uint32(kc)) &^ k.held.OtherModifiers(km, code)
	}
	if k.held.Accept(e) {
		k.setPending(w, e, kc)
	}
}

// beforeEvent runs before every translated event: it resolves a release
// waiting for a repeat (fallback without detectable auto-repeat) and
// queues the pending key event unless ev is the StateNotify that may
// belong to it. It reports false when ev was consumed (the press of a
// repeat pair).
func (k *kbd) beforeEvent(ev any) bool {
	if k.hasRelease {
		k.hasRelease = false
		if p, ok := ev.(xproto.KeyPressEvent); ok && p.Detail == k.release.Detail && p.Time == k.release.Time && p.Event == k.release.Event {
			// A server repeat without detectable auto-repeat: drop the
			// release; the press of a held key is a repeat.
			k.keyPress(p)
			return false
		}
		k.applyRelease(k.release)
	}
	if k.hasPending && !k.b.isStateNotify(ev) {
		k.flush()
	}
	return true
}

// flushAll resolves a waiting release and queues the pending key event:
// nothing more arrived in this read.
func (k *kbd) flushAll() {
	if k.hasRelease {
		k.hasRelease = false
		k.applyRelease(k.release)
	}
	k.flush()
}

func (k *kbd) setPending(w *Window, e platform.KeyEvent, kc byte) {
	k.flush()
	k.pending, k.pendingKC, k.pendingWin, k.hasPending = e, kc, w, true
}

// flush queues the pending key event.
func (k *kbd) flush() {
	if !k.hasPending {
		return
	}
	k.hasPending = false
	if k.pendingWin == k.focus && k.focus != nil {
		k.b.enqueue(k.focus, k.pending)
	}
}

// stateNotify takes the modifiers after a key event from the XKB
// StateNotify it caused (held: base and latched real modifiers).
func (k *kbd) stateNotify(held uint32, keycode byte) {
	if k.hasPending && keycode == k.pendingKC && k.keymap != nil {
		k.pending.Mods = k.keymap.ModifiersOf(held)
	}
	k.flush()
}

// keymapNotify records the keys down when focus or the pointer arrives
// (bit n of keys is keycode 8+n): those not delivered as pressed are
// stale, and their server repeats are not presses.
func (k *kbd) keymapNotify(keys []byte) {
	for kc := 8; kc < 256; kc++ {
		i := kc - 8
		down := i/8 < len(keys) && keys[i/8]&(1<<(i%8)) != 0
		k.stale[kc] = down && !k.held.IsHeld(evdev.X11KeyCode(uint32(kc)))
	}
}

// focusIn gives w the keyboard focus.
func (k *kbd) focusIn(w *Window) {
	if k.focus != nil && k.focus != w {
		k.b.blur(k.focus) // a FocusOut was missed
	}
	k.focus = w
	k.held = keyboard.Held{}
	clear(k.stale[:])
	if k.compose != nil {
		k.compose.Reset()
	}
}

// releaseHeld queues synthetic releases of the held keys of the focused
// window (focus loss, or a grab that takes the keyboard).
func (k *kbd) releaseHeld() {
	k.flushAll()
	if k.compose != nil {
		k.compose.Reset()
	}
	if k.focus == nil {
		return
	}
	for _, r := range k.held.ReleaseAll(k.b.Now()) {
		k.b.enqueue(k.focus, r)
	}
}

// focusOut ends the keyboard focus of w: synthetic releases first.
func (k *kbd) focusOut(w *Window) {
	if k.focus != w {
		return
	}
	k.releaseHeld()
	k.focus = nil
}

// dropWindow forgets a window that is closing: nothing more is delivered
// to it, not even synthetic releases.
func (k *kbd) dropWindow(w *Window) {
	if k.pendingWin == w {
		k.hasPending = false
	}
	if k.focus == w {
		k.focus = nil
		k.held = keyboard.Held{}
		k.hasRelease = false
	}
}

// blur reports that w lost the keyboard focus: synthetic releases, then
// FocusEvent{false}.
func (b *Backend) blur(w *Window) {
	b.kbd.focusOut(w)
	if w.focused {
		w.focused = false
		b.enqueue(w, platform.FocusEvent{Focused: false})
	}
}

// isStateNotify reports whether ev is an XKB StateNotify.
func (b *Backend) isStateNotify(ev any) bool {
	raw, ok := ev.(xgb.RawEvent)
	return ok && b.xkb.present && len(raw) >= 32 && raw.Code() == b.xkb.firstEvent && raw[1] == xkbStateNotify
}

// coreMapping reads the core keyboard and modifier mappings (two round
// trips).
func (b *Backend) coreMapping() (xkb.CoreMapping, error) {
	setup := xproto.Setup(b.conn)
	first, last := setup.MinKeycode, setup.MaxKeycode
	if last < first {
		return xkb.CoreMapping{}, errors.New("empty keycode range")
	}
	kc := xproto.GetKeyboardMapping(b.conn, first, byte(int(last)-int(first)+1))
	mc := xproto.GetModifierMapping(b.conn)
	kr, err := kc.Reply()
	if err != nil {
		return xkb.CoreMapping{}, err
	}
	mr, err := mc.Reply()
	if err != nil {
		return xkb.CoreMapping{}, err
	}
	m := xkb.CoreMapping{MinKeycode: uint32(first), KeysymsPerKeycode: int(kr.KeysymsPerKeycode)}
	m.Keysyms = make([]xkb.Keysym, len(kr.Keysyms))
	for i, s := range kr.Keysyms {
		m.Keysyms[i] = xkb.Keysym(s)
	}
	per := int(mr.KeycodesPerModifier)
	for mod := range 8 {
		for _, c := range mr.Keycodes[min(mod*per, len(mr.Keycodes)):min((mod+1)*per, len(mr.Keycodes))] {
			if c != 0 {
				m.Modifiers[mod] = append(m.Modifiers[mod], uint32(c))
			}
		}
	}
	return m, nil
}

// loadCompose loads the Compose table on first use, with the rules of
// keyboard.LoadCompose (tests set composeLoader).
func (k *kbd) loadCompose() {
	if k.composeLoaded {
		return
	}
	k.composeLoaded = true
	t, err := keyboard.LoadCompose(k.composeLoader)
	if err != nil {
		k.err = keyboard.Report(fmt.Errorf("x11: cannot read the Compose file (dead keys type their accent): %w", err))
	}
	k.composeTable = t
	k.compose = xkb.NewComposeState(k.composeTable)
}
