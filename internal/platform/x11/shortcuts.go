//go:build linux

package x11

import (
	"errors"
	"fmt"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Global shortcuts on X11: the GlobalShortcuts portal when the session
// has one (freedesktop), else passive key grabs on the root window, as
// GNOME's keybinder, xbindkeys and Qt's QxtGlobalShortcut do:
//
//   - The accelerator's key is every keycode whose unshifted key in the
//     first layout is Accelerator.Key (KeyEvent.Key without modifiers,
//     looked up in the keymap the keyboard uses); its modifiers are the
//     real modifiers Shift, Control and those Alt and Super are bound to
//     (Mod1 and Mod4 by default).
//   - Each keycode is grabbed once per combination of the Lock and NumLock
//     masks, because X matches grabs on the exact modifier state.
//   - A grab that fails with BadAccess (another client holds the
//     combination) is ShortcutTaken, the grabs already made are released;
//     two shortcuts of the app with the same combination make the second
//     one Taken too. Success is ShortcutBound with the accelerator as the
//     trigger.
//   - Presses and releases arrive at the root window (owner_events
//     false). A press of a key already pressed is a server repeat and is
//     dropped; without XKB detectable auto-repeat, a release followed by
//     a press of the same key at the same time is one too.
//   - The grabs follow keyboard mapping changes (a status event when the
//     outcome changes), and go away with UnregisterShortcut and when Run
//     returns.

// shortcutPortal is the part of the freedesktop desktop the shortcuts
// try first.
type shortcutPortal interface {
	RegisterShortcut(s platform.Shortcut) error
	UnregisterShortcut(id string) error
}

// shortcuts is the X11 side of the global shortcuts.
type shortcuts struct {
	portalMissing bool            // the portal returned ErrUnsupported
	viaPortal     map[string]bool // IDs registered with the portal
	grabbed       []*keyShortcut  // registration order

	active   *keyShortcut // the shortcut whose key is down
	activeKC byte
	// Without detectable auto-repeat: a release waiting to see whether a
	// press of the same key at the same time follows.
	release     xproto.KeyReleaseEvent
	haveRelease bool
}

// keyShortcut is a shortcut implemented with key grabs.
type keyShortcut struct {
	s      platform.Shortcut
	kcs    []byte // the grabbed keycodes; nil when not bound
	mods   uint16
	locks  []uint16
	status platform.ShortcutStatus
}

// desktop is the X11 Desktop: the freedesktop services, with global
// shortcuts that fall back to key grabs.
type desktop struct {
	platform.Desktop
	b *Backend
}

func (d desktop) RegisterShortcut(s platform.Shortcut) error {
	platform.CheckShortcut(s)
	return d.b.registerShortcut(d.Desktop, s)
}

func (d desktop) UnregisterShortcut(id string) error {
	platform.CheckShortcutID("UnregisterShortcut", id)
	return d.b.unregisterShortcut(d.Desktop, id)
}

// registerShortcut registers s with the portal, or with key grabs when
// the session has no portal.
func (b *Backend) registerShortcut(portal shortcutPortal, s platform.Shortcut) error {
	sc := &b.integ.sc
	if i := sc.index(s.ID); i >= 0 {
		b.ungrabShortcut(sc.grabbed[i])
		sc.grabbed = slices.Delete(sc.grabbed, i, i+1)
	}
	if !sc.portalMissing {
		err := portal.RegisterShortcut(s)
		if err == nil {
			if sc.viaPortal == nil {
				sc.viaPortal = map[string]bool{}
			}
			sc.viaPortal[s.ID] = true
			return nil
		}
		if !errors.Is(err, errors.ErrUnsupported) {
			return err
		}
		sc.portalMissing = true
	}
	k := &keyShortcut{s: s}
	reason := b.grabShortcut(k)
	sc.grabbed = append(sc.grabbed, k)
	b.enqueue(nil, k.statusEvent(reason))
	b.signal()
	return nil
}

// unregisterShortcut removes a shortcut; unknown IDs are ignored.
func (b *Backend) unregisterShortcut(portal shortcutPortal, id string) error {
	sc := &b.integ.sc
	if i := sc.index(id); i >= 0 {
		b.ungrabShortcut(sc.grabbed[i])
		sc.grabbed = slices.Delete(sc.grabbed, i, i+1)
		return nil
	}
	if sc.viaPortal[id] {
		delete(sc.viaPortal, id)
		return portal.UnregisterShortcut(id)
	}
	return nil
}

func (sc *shortcuts) index(id string) int {
	return slices.IndexFunc(sc.grabbed, func(k *keyShortcut) bool { return k.s.ID == id })
}

// statusEvent is the ShortcutStatusEvent of k's current status.
func (k *keyShortcut) statusEvent(reason string) platform.ShortcutStatusEvent {
	e := platform.ShortcutStatusEvent{ID: k.s.ID, Status: k.status, Reason: reason}
	if k.status == platform.ShortcutBound {
		e.Trigger, e.Reason = k.s.Accelerator.String(), ""
	}
	return e
}

// grabShortcut grabs k's combination and sets its status; it returns the
// reason of a status other than bound.
func (b *Backend) grabShortcut(k *keyShortcut) string {
	k.kcs = nil
	km := b.kbd.km()
	if km == nil {
		k.status = platform.ShortcutFailed
		return "the keyboard mapping could not be read"
	}
	kcs, mods, locks := accelGrabs(km, k.s.Accelerator)
	if len(kcs) == 0 {
		k.status = platform.ShortcutFailed
		return fmt.Sprintf("no key of the keyboard layout types %q", k.s.Accelerator.Key)
	}
	for _, o := range b.integ.sc.grabbed {
		if o != k && o.kcs != nil && o.mods == mods && slices.ContainsFunc(kcs, func(kc byte) bool { return slices.Contains(o.kcs, kc) }) {
			k.status = platform.ShortcutTaken
			return fmt.Sprintf("the app's shortcut %q uses %s", o.s.ID, k.s.Accelerator)
		}
	}
	type grab struct {
		kc   byte
		mods uint16
	}
	var done []grab
	for _, kc := range kcs {
		for _, l := range locks {
			err := b.xs.grabKey(kc, mods|l)
			if err == nil {
				done = append(done, grab{kc, mods | l})
				continue
			}
			for _, g := range done {
				b.xs.ungrabKey(g.kc, g.mods)
			}
			if isAccessError(err) {
				k.status = platform.ShortcutTaken
				return fmt.Sprintf("another application grabbed %s", k.s.Accelerator)
			}
			k.status = platform.ShortcutFailed
			return fmt.Sprintf("XGrabKey failed: %v", err)
		}
	}
	k.kcs, k.mods, k.locks = kcs, mods, locks
	k.status = platform.ShortcutBound
	return ""
}

// ungrabShortcut releases k's grabs.
func (b *Backend) ungrabShortcut(k *keyShortcut) {
	for _, kc := range k.kcs {
		for _, l := range k.locks {
			b.xs.ungrabKey(kc, k.mods|l)
		}
	}
	k.kcs = nil
	if b.integ.sc.active == k {
		b.integ.sc.active = nil
	}
}

// remapShortcuts grabs every shortcut again after the keyboard mapping
// changed, reporting the shortcuts whose status changed.
func (b *Backend) remapShortcuts() {
	sc := &b.integ.sc
	if len(sc.grabbed) == 0 {
		return
	}
	for _, k := range sc.grabbed {
		b.ungrabShortcut(k)
	}
	for _, k := range sc.grabbed {
		old := k.status
		reason := b.grabShortcut(k)
		if k.status != old {
			b.enqueue(nil, k.statusEvent(reason))
		}
	}
}

// ungrabShortcuts releases every grab (Run returns).
func (b *Backend) ungrabShortcuts() {
	for _, k := range b.integ.sc.grabbed {
		b.ungrabShortcut(k)
	}
	b.integ.sc.grabbed = nil
}

// accelGrabs returns the keycodes and modifiers of a, and the lock
// combinations to grab each with.
func accelGrabs(km *xkb.Keymap, a platform.Accelerator) (kcs []byte, mods uint16, locks []uint16) {
	st := xkb.NewState(km)
	for kc := 8; kc < 256; kc++ {
		if st.Lookup(uint32(kc)).Key == a.Key {
			kcs = append(kcs, byte(kc))
		}
	}
	mask := func(name string, def uint32) uint16 {
		if m, ok := km.ModMask(name); ok && m != 0 && m <= 0xff {
			return uint16(m)
		}
		return uint16(def)
	}
	if a.Mods&platform.ModShift != 0 {
		mods |= xproto.ModMaskShift
	}
	if a.Mods&platform.ModCtrl != 0 {
		mods |= xproto.ModMaskControl
	}
	if a.Mods&platform.ModAlt != 0 {
		mods |= mask("Alt", xproto.ModMask1)
	}
	if a.Mods&platform.ModSuper != 0 {
		mods |= mask("Super", xproto.ModMask4)
	}
	num := mask("NumLock", xproto.ModMask2)
	locks = []uint16{0, xproto.ModMaskLock}
	if num&^mods != 0 && num != xproto.ModMaskLock {
		locks = append(locks, num, num|xproto.ModMaskLock)
	}
	return kcs, mods, locks
}

// shortcutKey handles a key event on the root window (a grabbed
// shortcut). It reports whether the event was one.
func (b *Backend) shortcutKey(ev any) bool {
	sc := &b.integ.sc
	switch ev := ev.(type) {
	case xproto.KeyPressEvent:
		if ev.Event != b.root {
			return false
		}
		kc := byte(ev.Detail)
		if sc.haveRelease {
			sc.haveRelease = false
			if sc.release.Detail == ev.Detail && sc.release.Time == ev.Time {
				return true // a server repeat: release and press dropped
			}
			b.shortcutRelease(byte(sc.release.Detail))
		}
		if sc.active != nil && sc.activeKC == kc {
			return true // a repeat
		}
		state := ev.State & 0xff
		for _, k := range sc.grabbed {
			if k.kcs == nil || !slices.Contains(k.kcs, kc) {
				continue
			}
			for _, l := range k.locks {
				if state == k.mods|l {
					sc.active, sc.activeKC = k, kc
					b.enqueue(nil, platform.ShortcutEvent{ID: k.s.ID, Action: platform.ShortcutPressed, Time: b.Now()})
					return true
				}
			}
		}
		return true
	case xproto.KeyReleaseEvent:
		if ev.Event != b.root {
			return false
		}
		if !b.xkb.detectable {
			sc.flushRelease(b)
			sc.release, sc.haveRelease = ev, true
			return true
		}
		b.shortcutRelease(byte(ev.Detail))
		return true
	}
	return false
}

// flushRelease applies a release that waited for a possible repeat press.
func (sc *shortcuts) flushRelease(b *Backend) {
	if sc.haveRelease {
		sc.haveRelease = false
		b.shortcutRelease(byte(sc.release.Detail))
	}
}

// shortcutRelease reports the release of the active shortcut's key.
func (b *Backend) shortcutRelease(kc byte) {
	sc := &b.integ.sc
	if sc.active == nil || sc.activeKC != kc {
		return
	}
	k := sc.active
	sc.active = nil
	b.enqueue(nil, platform.ShortcutEvent{ID: k.s.ID, Action: platform.ShortcutReleased, Time: b.Now()})
}
