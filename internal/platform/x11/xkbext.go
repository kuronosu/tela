//go:build linux

package x11

// The XKB extension, hand-written on the connection and used for state
// only (key translation uses the core keyboard mapping, see keyboard.go):
//
//   - UseExtension makes the client XKB-aware: core key and button events
//     then carry the effective group in state bits 13-14 (the server
//     strips it for other clients), which is the group the keys resolve
//     in.
//   - PerClientFlags turns on detectable auto-repeat: held keys repeat as
//     presses only, without the fake release before each one (winit,
//     GTK and Qt do the same). Without it, keyboard.go merges a release
//     and a press of the same key with the same timestamp.
//   - SelectEvents asks for StateNotify (modifier and group changes, which
//     give a key event the modifiers after it), MapNotify and
//     NewKeyboardNotify (the mapping changed: rebuild the keymap). An
//     XKB-aware client that selects MapNotify no longer gets the core
//     MappingNotify for those changes.

const (
	xkbUseExtension   = 0
	xkbSelectEvents   = 1
	xkbPerClientFlags = 21

	xkbUseCoreKbd = 0x100

	// Event types (byte 1 of an XKB event) and their select masks.
	xkbNewKeyboardNotify = 0
	xkbMapNotify         = 1
	xkbStateNotify       = 2

	xkbPCFDetectableAutoRepeat = 1

	// StateNotify components: the modifier and group parts (base,
	// latched, locked, effective).
	xkbStateModsAndGroup = 0xff
	xkbStateAll          = 0x3fff
	// MapNotify parts that change key translation: key types, key
	// symbols, modifier map.
	xkbMapTranslation = 0x07
	xkbMapAll         = 0xff
)

// xkbExt is the XKB extension state of the connection.
type xkbExt struct {
	extension
	// ok: UseExtension succeeded, so core event states carry the XKB
	// group in bits 13-14.
	ok bool
	// detectable: detectable auto-repeat is on (repeats without fake
	// releases).
	detectable bool
}

func xkbUseExtensionRequest(opcode byte) []byte {
	req := newRequest(opcode, xkbUseExtension, 8)
	le.PutUint16(req[4:], 1) // wanted 1.0
	return req
}

func xkbPerClientFlagsRequest(opcode byte) []byte {
	req := newRequest(opcode, xkbPerClientFlags, 28)
	le.PutUint16(req[4:], xkbUseCoreKbd)
	le.PutUint32(req[8:], xkbPCFDetectableAutoRepeat)  // change
	le.PutUint32(req[12:], xkbPCFDetectableAutoRepeat) // value
	return req
}

// xkbSelectEventsRequest selects every NewKeyboardNotify, the MapNotify
// of the translation parts, and the modifier and group parts of
// StateNotify. MapNotify is selected by affectMap and map; the details
// that follow the header belong to the event types that are neither
// cleared nor selected whole, in bit order (here StateNotify only).
func xkbSelectEventsRequest(opcode byte) []byte {
	req := newRequest(opcode, xkbSelectEvents, 20)
	le.PutUint16(req[4:], xkbUseCoreKbd)
	le.PutUint16(req[6:], 1<<xkbNewKeyboardNotify|1<<xkbMapNotify|1<<xkbStateNotify) // affectWhich
	le.PutUint16(req[8:], 0)                                                         // clear
	le.PutUint16(req[10:], 1<<xkbNewKeyboardNotify)                                  // selectAll
	le.PutUint16(req[12:], xkbMapAll)                                                // affectMap
	le.PutUint16(req[14:], xkbMapTranslation)                                        // map
	le.PutUint16(req[16:], xkbStateAll)                                              // affectState
	le.PutUint16(req[18:], xkbStateModsAndGroup)                                     // stateDetails
	return req
}

// initXKB enables the XKB extension when the server has it. Failures leave
// the core protocol behavior (fake releases between repeats, the group
// from Mode_switch).
func (b *Backend) initXKB() {
	e := b.queryExtension("XKEYBOARD")
	if !e.present {
		return
	}
	r, err := b.roundTrip(xkbUseExtensionRequest(e.opcode), 32)
	if err != nil || r[1] == 0 {
		return
	}
	b.xkb = xkbExt{extension: e, ok: true}
	if r, err := b.roundTrip(xkbPerClientFlagsRequest(e.opcode), 32); err == nil {
		supported, value := le.Uint32(r[8:]), le.Uint32(r[12:])
		b.xkb.detectable = supported&value&xkbPCFDetectableAutoRepeat != 0
	}
	b.sendRequest(xkbSelectEventsRequest(e.opcode))
}

// xkbEvent handles an XKB event (32 bytes; byte 1 is the XKB type).
func (b *Backend) xkbEvent(raw []byte) {
	switch raw[1] {
	case xkbNewKeyboardNotify, xkbMapNotify:
		b.kbd.mappingChanged()
	case xkbStateNotify:
		// Bytes: 10 baseMods, 11 latchedMods, 26-27 changed, 28 keycode
		// of the key that caused the change.
		held := uint32(raw[10]) | uint32(raw[11])
		b.kbd.stateNotify(held, raw[28])
	}
}
