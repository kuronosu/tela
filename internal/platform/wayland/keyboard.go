//go:build linux

package wayland

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/evdev"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// maxKeymapSize bounds the keymap the compositor may hand over. Real
// keymaps are about 70 KiB.
const maxKeymapSize = 16 << 20

// kbd is the keyboard of a seat: the bound wl_keyboard, its keymap and
// modifier state, keyboard focus, the keys delivered as pressed, and
// client-side key repeat.
//
// Quirks handled here, as winit and Gio do:
//
//   - wl_keyboard.modifiers follows the key event that changed the
//     modifiers. KeyEvent.Mods is the state after the event, so a key
//     event waits (pending) until the next keyboard event or the end of
//     the dispatch; a modifiers event in between sets its Mods. If the
//     modifiers event comes in a later read, the Mods of a modifier key
//     are predicted from its modifier map (xkb.Keymap.HeldModifiers).
//   - The keys array of wl_keyboard.enter lists keys held when focus
//     arrives; they produce no presses (ADR-050), and their releases are
//     dropped by keyboard.Held.
//   - wl_keyboard.leave only says focus is gone: the keys still held get
//     synthetic releases, before the FocusEvent of the window if it loses
//     focus (see Window.updateFocus).
//   - Repeat is client-side: wl_keyboard.repeat_info gives rate and delay
//     (rate 0 disables repeat); the deadline is folded into the poll
//     timeout, so no timer runs while no key is held.
type kbd struct {
	s  *seat
	wl *protocols.WlKeyboard

	keymap  *xkb.Keymap // nil before the keymap event, or for no_keymap
	state   *xkb.State
	compose *xkb.ComposeState

	focus *Window
	held  keyboard.Held

	// Pending key event: queued once the following event shows whether a
	// modifiers event belongs to it.
	pending    platform.KeyEvent
	hasPending bool

	// Repeat: rate in keys per second (0: off) and delay; the held key
	// (evdev code) and the time of its next repeat.
	rate       int32
	delay      time.Duration
	repeating  bool
	repeatCode uint32
	repeatNext time.Duration
}

func (s *seat) bindKeyboard() {
	k := &kbd{s: s, rate: 25, delay: 600 * time.Millisecond} // until repeat_info: the usual X11 defaults
	k.wl = s.wl.GetKeyboard()
	k.wl.SetKeymapHandler(k.onKeymap)
	k.wl.SetEnterHandler(k.onEnter)
	k.wl.SetLeaveHandler(k.onLeave)
	k.wl.SetKeyHandler(k.onKey)
	k.wl.SetModifiersHandler(k.onModifiers)
	k.wl.SetRepeatInfoHandler(k.onRepeatInfo) // version 4+; older seats keep the defaults
	if s.b.composeLoaded {
		k.compose = xkb.NewComposeState(s.b.compose)
	}
	s.kbd = k
}

// release ends keyboard use (capability removed, seat removed, backend
// shutting down): the focused window loses keyboard focus as on leave.
func (k *kbd) release() {
	k.loseFocus(k.s.b.Now())
	if k.wl.Version() >= 3 && !k.wl.Destroyed() {
		k.wl.Release()
	}
}

func (k *kbd) onKeymap(e protocols.WlKeyboardKeymapEvent) {
	k.flush()
	defer wire.CloseFd(e.Fd)
	if e.Format != protocols.WlKeyboardKeymapFormatXkbV1 {
		// no_keymap: keys keep their physical code and have no logical
		// key or text.
		k.setKeymap(nil)
		return
	}
	text, err := readKeymap(e.Fd, e.Size)
	if err == nil {
		var km *xkb.Keymap
		if km, err = xkb.ParseKeymap(text); err == nil {
			k.setKeymap(km)
			k.s.b.composeTable() // load it now, not at the first key
			return
		}
	}
	k.s.b.keyboardErr = keyboard.Report(fmt.Errorf("wayland: cannot use the compositor's keymap: %w", err))
	k.setKeymap(nil)
}

func (k *kbd) setKeymap(km *xkb.Keymap) {
	k.keymap = km
	k.state = nil
	if km != nil {
		k.state = xkb.NewState(km)
	}
	if k.compose != nil {
		k.compose.Reset()
	}
	k.updatePointerMods()
}

// readKeymap maps the keymap read-only and copies it. Since wl_keyboard
// version 7 the fd must be mapped MAP_PRIVATE (MAP_SHARED may fail on a
// sealed memfd), which every version accepts.
func readKeymap(fd int, size uint32) ([]byte, error) {
	if size == 0 || size > maxKeymapSize {
		return nil, fmt.Errorf("keymap size %d out of range", size)
	}
	m, err := unix.Mmap(fd, 0, int(size), unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		// Not mappable (a pipe in some test harnesses): read it until
		// size bytes or EOF. pread would fail on a pipe (ESPIPE).
		buf := make([]byte, size)
		n := 0
		for n < len(buf) {
			r, rerr := unix.Read(fd, buf[n:])
			if rerr == unix.EINTR {
				continue
			}
			if rerr != nil {
				return nil, fmt.Errorf("cannot map (%v) or read the keymap: %w", err, rerr)
			}
			if r == 0 {
				break // EOF: the keymap is shorter than announced
			}
			n += r
		}
		return buf[:n], nil
	}
	defer unix.Munmap(m)
	return append([]byte(nil), m...), nil
}

func (k *kbd) onEnter(e protocols.WlKeyboardEnterEvent) {
	k.flush()
	b := k.s.b
	k.s.inputSerial(e.Serial)
	w := b.surfaces[e.Surface]
	if k.focus != nil {
		k.loseFocus(b.Now()) // a missed leave
	}
	if w == nil {
		return // not ours (or already destroyed)
	}
	k.focus = w
	k.held = keyboard.Held{}
	if k.compose != nil {
		k.compose.Reset()
	}
	w.kbdFocus++
	w.updateFocus()
	// e.Keys (keys already held) is ignored on purpose: see kbd.
}

func (k *kbd) onLeave(e protocols.WlKeyboardLeaveEvent) {
	k.flush()
	b := k.s.b
	k.s.inputSerial(e.Serial)
	k.loseFocus(b.Now())
	// The wl_keyboard logical state resets: no modifiers, first group.
	if k.state != nil {
		k.state.UpdateMask(0, 0, 0, 0)
	}
	k.updatePointerMods()
}

// loseFocus ends keyboard focus: synthetic releases of the held keys,
// then the window's focus is recomputed (its FocusEvent follows the
// releases).
func (k *kbd) loseFocus(now time.Duration) {
	k.flush()
	k.stopRepeat()
	if k.compose != nil {
		k.compose.Reset()
	}
	w := k.focus
	if w == nil {
		return
	}
	k.focus = nil
	for _, r := range k.held.ReleaseAll(now) {
		k.s.b.enqueue(w, r)
	}
	w.kbdFocus--
	w.updateFocus()
}

// dropWindow forgets a window that is closing: nothing more is delivered
// to it, not even synthetic releases.
func (k *kbd) dropWindow(w *Window) {
	if k.hasPending && k.focus == w {
		k.hasPending = false
	}
	if k.focus == w {
		k.focus = nil
		k.held = keyboard.Held{}
		k.stopRepeat()
	}
}

func (k *kbd) onKey(e protocols.WlKeyboardKeyEvent) {
	k.flush()
	b := k.s.b
	k.s.inputSerial(e.Serial)
	if k.focus == nil {
		return
	}
	t := b.eventTime(e.Time)
	code := evdev.KeyCode(e.Key)
	kc := e.Key + evdev.X11Offset
	ev := platform.KeyEvent{
		Action:   platform.KeyRelease,
		Code:     code,
		Key:      platform.KeyUnidentified,
		Location: code.Location(),
		Time:     t,
	}
	pressed := e.State == protocols.WlKeyboardKeyStatePressed
	var predicted platform.Modifiers
	if k.state != nil {
		mods := k.state.Modifiers()
		held := k.keymap.HeldModifiers(kc)
		if pressed {
			l := k.state.Press(kc, k.compose)
			ev.Action, ev.Key, ev.Text = platform.KeyPress, l.Key, l.Text
			predicted = mods | held
		} else {
			ev.Key = k.state.Lookup(kc).Key
			predicted = mods &^ (held &^ k.held.OtherModifiers(k.keymap, code))
		}
	} else if pressed {
		ev.Action = platform.KeyPress
	}
	ev.Mods = predicted
	if !k.held.Accept(ev) {
		return
	}
	switch {
	case pressed && k.rate > 0 && k.keymap != nil && k.keymap.Repeats(kc):
		k.repeating, k.repeatCode, k.repeatNext = true, e.Key, t+k.delay
	case !pressed && k.repeating && e.Key == k.repeatCode:
		// The release of the repeating key stops it. Keys that do not
		// repeat (modifiers, locks) neither start nor stop a repeat:
		// holding "a" and pressing Shift repeats "A" (GTK, winit).
		k.stopRepeat()
	}
	k.pending, k.hasPending = ev, true
}

func (k *kbd) onModifiers(e protocols.WlKeyboardModifiersEvent) {
	if k.state != nil {
		k.state.UpdateMask(e.ModsDepressed, e.ModsLatched, e.ModsLocked, e.Group)
	}
	if k.hasPending && k.state != nil {
		// The modifiers changed by the pending key: its Mods is the state
		// after it.
		k.pending.Mods = k.state.Modifiers()
	}
	k.flush()
	k.updatePointerMods()
}

func (k *kbd) onRepeatInfo(e protocols.WlKeyboardRepeatInfoEvent) {
	k.rate = max(0, e.Rate)
	k.delay = time.Duration(max(0, e.Delay)) * time.Millisecond
	if k.rate == 0 {
		k.stopRepeat()
	}
}

// flush queues the pending key event.
func (k *kbd) flush() {
	if !k.hasPending {
		return
	}
	k.hasPending = false
	if k.focus != nil {
		k.s.b.enqueue(k.focus, k.pending)
	}
}

func (k *kbd) stopRepeat() { k.repeating = false }

// repeatInterval is the time between repeats.
func (k *kbd) repeatInterval() time.Duration { return time.Second / time.Duration(k.rate) }

// repeat queues the repeat of the held key if it is due at now, and
// schedules the next one. A loop that woke late (a long frame) queues one
// repeat and skips the missed ones rather than bursting them.
func (k *kbd) repeat(now time.Duration) {
	if !k.repeating || k.focus == nil || k.rate <= 0 || now < k.repeatNext {
		return
	}
	at := k.repeatNext
	interval := k.repeatInterval()
	k.repeatNext += interval
	if k.repeatNext <= now {
		k.repeatNext = now + interval
	}
	code := evdev.KeyCode(k.repeatCode)
	ev := platform.KeyEvent{
		Action:   platform.KeyPress,
		Code:     code,
		Key:      platform.KeyUnidentified,
		Location: code.Location(),
		Repeat:   true,
		Time:     at,
	}
	if k.state != nil {
		l := k.state.Repeat(k.repeatCode+evdev.X11Offset, k.compose)
		ev.Key, ev.Text, ev.Mods = l.Key, l.Text, k.state.Modifiers()
	}
	if k.held.Accept(ev) {
		k.s.b.enqueue(k.focus, ev)
	}
}

// updatePointerMods gives the seat's pointer the current modifiers.
func (k *kbd) updatePointerMods() {
	var m platform.Modifiers
	if k.state != nil {
		m = k.state.Modifiers()
	}
	k.s.ptr.mods = m
}

// --- backend-wide keyboard state ---

// composeTable returns the Compose table, loading it on first use with the
// rules of keyboard.LoadCompose (tests set Backend.composeLoader).
func (b *Backend) composeTable() *xkb.ComposeTable {
	if !b.composeLoaded {
		b.composeLoaded = true
		t, err := keyboard.LoadCompose(b.composeLoader)
		if err != nil {
			b.keyboardErr = keyboard.Report(fmt.Errorf("wayland: cannot read the Compose file (dead keys type their accent): %w", err))
		}
		b.compose = t
		for _, s := range b.seats {
			if s.kbd != nil {
				s.kbd.compose = xkb.NewComposeState(b.compose)
			}
		}
	}
	return b.compose
}

// keyRepeats queues the key repeats due at now.
func (b *Backend) keyRepeats(now time.Duration) {
	for _, s := range b.seats {
		if s.kbd != nil {
			s.kbd.repeat(now)
		}
	}
}

// repeatDeadline returns the earliest pending key repeat.
func (b *Backend) repeatDeadline() (time.Duration, bool) {
	var d time.Duration
	ok := false
	for _, s := range b.seats {
		if k := s.kbd; k != nil && k.repeating && k.focus != nil && k.rate > 0 {
			if !ok || k.repeatNext < d {
				d, ok = k.repeatNext, true
			}
		}
	}
	return d, ok
}

// flushKeys queues the key events still pending after a dispatch.
func (b *Backend) flushKeys() {
	for _, s := range b.seats {
		if s.kbd != nil {
			s.kbd.flush()
		}
	}
}
