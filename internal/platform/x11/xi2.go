//go:build linux

package x11

import (
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// XInput2 (2.1 or later), hand-written on the connection: pointer motion,
// buttons, enter/leave and smooth scrolling through scroll valuators.
//
// Every window selects XI_Motion, XI_ButtonPress, XI_ButtonRelease,
// XI_Enter and XI_Leave of the master pointers, and leaves the core
// pointer events out of its event mask (a client gets either for a
// window). The root window selects XI_HierarchyChanged and
// XI_DeviceChanged of every device, to keep the scroll valuator table
// current.
//
// Scrolling (XI 2.1 smooth scrolling, as GTK 3, winit and Chromium read
// it): a slave device lists scroll classes (valuator number, vertical or
// horizontal, increment = the valuator distance of one wheel notch).
// Motion events carry the absolute value of the valuators that changed;
// the scroll distance is the difference with the previous value, divided
// by the increment, in notches. Quirks:
//
//   - The previous value is only valid while the pointer is in a window
//     of ours: on XI_Enter the baselines are dropped and the first value
//     after it only sets the baseline (GTK's reset on enter; winit
//     re-queries the devices). XI_DeviceChanged (another slave drives the
//     master, or a device's classes changed) sets them from the values it
//     carries.
//   - The server emulates the legacy wheel buttons 4-7 from scroll
//     valuators and flags them XIPointerEmulated: they are dropped, or
//     every notch would scroll twice (winit drops every emulated button
//     because it handles touch itself; Tela keeps the buttons 1-3 a
//     touchscreen emulates until it has touch input). A device without
//     scroll valuators sends real (unflagged) buttons 4-7, which are
//     wheel steps; for a legacy button on a device with scroll valuators
//     the server emulates the valuator motion instead (flagged too),
//     which is used.
//   - Touchpads report precise distances (ScrollEvent.Delta); wheels report
//     notches (Steps, fractions for high-resolution wheels). XI2 does not
//     say which is which; like GTK 3 a device is a touchpad when its name
//     says so or it has the "libinput Tapping Enabled" or "Synaptics Off"
//     property. Delta is smoothScrollPixels logical pixels per increment:
//     xf86-input-libinput sets the increment to its scroll pixel distance
//     (15 by default), so Delta is libinput's own distance, what a
//     Wayland compositor sends. Xwayland's single "xwayland-pointer"
//     device does not tell a touchpad from a wheel and reports notches.
//
// Sign convention: valuators grow downwards and to the right, the DOM's
// positive directions.

const (
	xiOpSelectEvents   = 46
	xiOpQueryVersion   = 47
	xiOpQueryDevice    = 48
	xiOpListProperties = 56

	xiAllDevices       = 0
	xiAllMasterDevices = 1

	xiDeviceChanged    = 1
	xiButtonPress      = 4
	xiButtonRelease    = 5
	xiMotion           = 6
	xiEnter            = 7
	xiLeave            = 8
	xiHierarchyChanged = 11

	xiPointerEmulated = 1 << 16

	xiValuatorClass = 2
	xiScrollClass   = 3

	xiScrollVertical   = 1
	xiScrollHorizontal = 2

	// xiWindowEvents are the XI2 events every window selects.
	xiWindowEvents = 1<<xiMotion | 1<<xiButtonPress | 1<<xiButtonRelease | 1<<xiEnter | 1<<xiLeave

	// smoothScrollPixels is the precise scroll distance of one scroll
	// increment of a touchpad, in logical pixels (libinput's default
	// scroll pixel distance).
	smoothScrollPixels = 15
)

// xiState is the XInput2 state of the connection.
type xiState struct {
	extension
	// ok: XInput 2.1 or later; windows select XI2 pointer events.
	ok bool
	// devices caches the slave devices by id; a device is queried the
	// first time an event comes from it.
	devices map[uint16]*xiDevice
	// query asks the server for a device (or xiAllDevices); tests replace
	// it.
	query func(id uint16) ([]*xiDevice, error)
	// touchpadProps are the atoms of the properties that mark a touchpad
	// (0 if the server has none).
	touchpadProps [2]xproto.Atom
}

// xiDevice is what the backend knows about an input device.
type xiDevice struct {
	id     uint16
	name   string
	smooth bool // a touchpad: precise scroll distances
	axes   []*scrollAxis
}

// scrollAxis is a scroll valuator of a device.
type scrollAxis struct {
	number     uint16
	horizontal bool
	increment  float64
	value      float64 // last value seen
	valid      bool    // value is a usable baseline
}

func (d *xiDevice) axis(number uint16) *scrollAxis {
	for _, a := range d.axes {
		if a.number == number {
			return a
		}
	}
	return nil
}

// touchpadName reports whether a device name says touchpad (GTK 3's
// name heuristic, for drivers without the properties).
func touchpadName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "touchpad") || strings.Contains(n, "trackpad")
}

// xiSelectEventsRequest selects mask (bits of XI event types) on win for
// a device (or xiAllDevices / xiAllMasterDevices).
func xiSelectEventsRequest(opcode byte, win xproto.Window, device uint16, mask uint32) []byte {
	req := newRequest(opcode, xiOpSelectEvents, 20)
	le.PutUint32(req[4:], uint32(win))
	le.PutUint16(req[8:], 1) // num_masks
	le.PutUint16(req[12:], device)
	le.PutUint16(req[14:], 1) // mask_len (4-byte units)
	le.PutUint32(req[16:], mask)
	return req
}

// initXI2 enables XInput2 when the server has 2.1 or later; otherwise the
// core pointer events stay in use.
func (b *Backend) initXI2() {
	e := b.queryExtension("XInputExtension")
	if !e.present {
		return
	}
	req := newRequest(e.opcode, xiOpQueryVersion, 8)
	le.PutUint16(req[4:], 2)
	le.PutUint16(req[6:], 2) // 2.2: the newest this client knows
	r, err := b.roundTrip(req, 12)
	if err != nil {
		return
	}
	if major, minor := le.Uint16(r[8:]), le.Uint16(r[10:]); major < 2 || major == 2 && minor < 1 {
		return
	}
	b.xi = xiState{extension: e, ok: true, devices: map[uint16]*xiDevice{}, query: b.queryXIDevices}
	for i, name := range []string{"libinput Tapping Enabled", "Synaptics Off"} {
		if r, err := xproto.InternAtom(b.conn, true, uint16(len(name)), name).Reply(); err == nil && r != nil {
			b.xi.touchpadProps[i] = r.Atom
		}
	}
	b.sendRequest(xiSelectEventsRequest(e.opcode, b.root, xiAllDevices, 1<<xiHierarchyChanged|1<<xiDeviceChanged))
	if devs, err := b.xi.query(xiAllDevices); err == nil {
		for _, d := range devs {
			b.xi.devices[d.id] = d
		}
	}
}

// queryXIDevices runs XIQueryDevice, then XIListProperties for the devices
// with scroll axes whose name does not say touchpad (pipelined).
func (b *Backend) queryXIDevices(id uint16) ([]*xiDevice, error) {
	req := newRequest(b.xi.opcode, xiOpQueryDevice, 8)
	le.PutUint16(req[4:], id)
	r, err := b.roundTrip(req, 32)
	if err != nil {
		return nil, err
	}
	devs := parseXIDevices(r)
	if b.xi.touchpadProps == [2]xproto.Atom{} {
		return devs, nil
	}
	type pending struct {
		d      *xiDevice
		cookie func() ([]byte, error)
	}
	var ps []pending
	for _, d := range devs {
		if len(d.axes) == 0 || d.smooth {
			continue
		}
		req := newRequest(b.xi.opcode, xiOpListProperties, 8)
		le.PutUint16(req[4:], d.id)
		cookie := b.conn.NewCookie(true, true)
		b.conn.NewRequest(req, cookie)
		ps = append(ps, pending{d, cookie.Reply})
	}
	for _, p := range ps {
		r, err := p.cookie()
		if err != nil || len(r) < 32 {
			continue
		}
		n := int(le.Uint16(r[8:]))
		for i := range n {
			if 32+4*i+4 > len(r) {
				break
			}
			a := xproto.Atom(le.Uint32(r[32+4*i:]))
			if a != 0 && (a == b.xi.touchpadProps[0] || a == b.xi.touchpadProps[1]) {
				p.d.smooth = true
			}
		}
	}
	return devs, nil
}

// fp3232 decodes an XI FP3232 (signed integral part, then the fraction).
func fp3232(b []byte) float64 {
	return float64(int32(le.Uint32(b))) + float64(le.Uint32(b[4:]))/(1<<32)
}

// fp1616 decodes an XI FP1616.
func fp1616(b []byte) float64 { return float64(int32(le.Uint32(b))) / 65536 }

func pad4(n int) int { return (n + 3) &^ 3 }

// parseXIDevices decodes an XIQueryDevice reply. Malformed tails are
// dropped.
func parseXIDevices(r []byte) []*xiDevice {
	n := int(le.Uint16(r[8:]))
	off := 32
	var devs []*xiDevice
	for range n {
		if off+12 > len(r) {
			break
		}
		d := &xiDevice{id: le.Uint16(r[off:])}
		nClasses := int(le.Uint16(r[off+6:]))
		nameLen := int(le.Uint16(r[off+8:]))
		off += 12
		if off+pad4(nameLen) > len(r) {
			break
		}
		d.name = string(r[off : off+nameLen])
		d.smooth = touchpadName(d.name)
		off += pad4(nameLen)
		axes, size, ok := parseXIClasses(r[off:], nClasses)
		if !ok {
			break
		}
		d.axes = axes
		off += size
		devs = append(devs, d)
	}
	return devs
}

// parseXIClasses decodes n device classes (XIQueryDevice, XI_DeviceChanged)
// into the scroll axes, with the valuator values as their baselines. It
// returns the bytes consumed; ok is false if the data is short.
func parseXIClasses(data []byte, n int) (axes []*scrollAxis, size int, ok bool) {
	values := map[uint16]float64{}
	off := 0
	for range n {
		if off+4 > len(data) {
			return nil, 0, false
		}
		typ, length := le.Uint16(data[off:]), int(le.Uint16(data[off+2:]))*4
		if length < 4 || off+length > len(data) {
			return nil, 0, false
		}
		c := data[off : off+length]
		switch {
		case typ == xiValuatorClass && length >= 44:
			values[le.Uint16(c[6:])] = fp3232(c[28:])
		case typ == xiScrollClass && length >= 24:
			inc := fp3232(c[16:])
			st := le.Uint16(c[8:])
			if inc != 0 && !math.IsNaN(inc) && (st == xiScrollVertical || st == xiScrollHorizontal) {
				axes = append(axes, &scrollAxis{number: le.Uint16(c[6:]), horizontal: st == xiScrollHorizontal, increment: inc})
			}
		}
		off += length
	}
	for _, a := range axes {
		a.value, a.valid = values[a.number]
	}
	return axes, off, true
}

// xiDeviceEvent is a decoded XI_Motion, XI_ButtonPress or XI_ButtonRelease.
type xiDeviceEvent struct {
	evtype    uint16
	sourceid  uint16
	detail    uint32
	window    xproto.Window
	x, y      float64 // physical pixels, relative to window
	flags     uint32
	held      platform.Buttons // buttons held before the event
	mods      uint32           // base | latched real modifiers
	valuators []xiValuator
}

type xiValuator struct {
	number uint16
	value  float64
}

// xiButtons decodes an XI2 button mask (bit n = button n held).
func xiButtons(mask []byte) platform.Buttons {
	var held platform.Buttons
	for n, btn := range map[int]platform.PointerButton{
		1: platform.ButtonPrimary, 2: platform.ButtonMiddle, 3: platform.ButtonSecondary,
		8: platform.ButtonBack, 9: platform.ButtonForward,
	} {
		if n/8 < len(mask) && mask[n/8]&(1<<(n%8)) != 0 {
			held |= btn.Mask()
		}
	}
	return held
}

func parseXIDeviceEvent(raw []byte) (xiDeviceEvent, bool) {
	if len(raw) < 80 {
		return xiDeviceEvent{}, false
	}
	ev := xiDeviceEvent{
		evtype:   le.Uint16(raw[8:]),
		detail:   le.Uint32(raw[16:]),
		window:   xproto.Window(le.Uint32(raw[24:])),
		x:        fp1616(raw[40:]),
		y:        fp1616(raw[44:]),
		sourceid: le.Uint16(raw[52:]),
		flags:    le.Uint32(raw[56:]),
		mods:     le.Uint32(raw[60:]) | le.Uint32(raw[64:]),
	}
	blen, vlen := int(le.Uint16(raw[48:]))*4, int(le.Uint16(raw[50:]))*4
	off := 80
	if off+blen+vlen > len(raw) {
		return xiDeviceEvent{}, false
	}
	ev.held = xiButtons(raw[off : off+blen])
	off += blen
	vmask := raw[off : off+vlen]
	off += vlen
	for i := range vlen * 8 {
		if vmask[i/8]&(1<<(i%8)) == 0 {
			continue
		}
		if off+8 > len(raw) {
			return xiDeviceEvent{}, false
		}
		ev.valuators = append(ev.valuators, xiValuator{uint16(i), fp3232(raw[off:])})
		off += 8
	}
	return ev, true
}

// xiEvent handles an XInput2 GenericEvent.
func (b *Backend) xiEvent(raw []byte) {
	switch evtype := le.Uint16(raw[8:]); evtype {
	case xiMotion, xiButtonPress, xiButtonRelease:
		ev, ok := parseXIDeviceEvent(raw)
		w := b.windows[ev.window]
		if !ok || w == nil {
			return
		}
		p := ptr{w: w, x: ev.x, y: ev.y, held: ev.held, mods: b.kbd.mods(ev.mods)}
		switch {
		case evtype == xiMotion:
			b.xiMotion(p, ev)
		case ev.flags&xiPointerEmulated != 0 && ev.detail >= 4 && ev.detail <= 7:
			// Wheel buttons emulated from scroll valuators: the
			// valuator motion is handled. (Buttons 1-3 emulated from
			// a touchscreen are kept: Tela does not select XI2 touch
			// events, so they are the only report of a tap.)
		default:
			b.ptrButton(p, ev.detail, evtype == xiButtonPress)
		}
	case xiEnter, xiLeave:
		if len(raw) < 72 {
			return
		}
		w := b.windows[xproto.Window(le.Uint32(raw[24:]))]
		blen := int(le.Uint16(raw[50:])) * 4
		if w == nil || 72+blen > len(raw) {
			return
		}
		p := ptr{
			w:    w,
			x:    fp1616(raw[40:]),
			y:    fp1616(raw[44:]),
			held: xiButtons(raw[72 : 72+blen]),
			mods: b.kbd.mods(le.Uint32(raw[52:]) | le.Uint32(raw[56:])),
		}
		mode, detail := raw[18], raw[19]
		if evtype == xiEnter {
			b.xi.resetScroll()
			b.ptrEnter(p, mode, detail)
		} else {
			b.ptrLeave(p, mode, detail)
		}
	case xiDeviceChanged:
		// Bytes 12-13 num_classes, 14-15 sourceid; the classes follow
		// the 32-byte header and describe the source device.
		src := le.Uint16(raw[14:])
		if d := b.xi.devices[src]; d != nil {
			if axes, _, ok := parseXIClasses(raw[32:], int(le.Uint16(raw[12:]))); ok {
				d.axes = axes
			}
		}
	case xiHierarchyChanged:
		// Devices added, removed, attached, enabled: forget them, they
		// are queried again at their next event.
		n := int(le.Uint16(raw[20:]))
		for i := range n {
			if o := 32 + 12*i; o+12 <= len(raw) && le.Uint32(raw[o+8:]) != 0 {
				delete(b.xi.devices, le.Uint16(raw[o:]))
			}
		}
	}
}

// device returns the device with the given id, querying it if unknown.
func (x *xiState) device(id uint16) *xiDevice {
	if d := x.devices[id]; d != nil {
		return d
	}
	d := &xiDevice{id: id} // also cached when the query fails
	if x.query != nil {
		if devs, err := x.query(id); err == nil {
			for _, q := range devs {
				if q.id == id {
					d = q
				}
			}
		}
	}
	x.devices[id] = d
	return d
}

// resetScroll drops every scroll baseline (the pointer entered a window:
// the valuators may have moved while it was elsewhere).
func (x *xiState) resetScroll() {
	for _, d := range x.devices {
		for _, a := range d.axes {
			a.valid = false
		}
	}
}

// xiMotion handles an XI_Motion: pointer motion, then scrolling.
func (b *Backend) xiMotion(p ptr, ev xiDeviceEvent) {
	var dx, dy float64
	var d *xiDevice
	if len(ev.valuators) > 0 {
		d = b.xi.device(ev.sourceid)
		for _, v := range ev.valuators {
			a := d.axis(v.number)
			if a == nil {
				continue
			}
			if !a.valid {
				a.value, a.valid = v.value, true
				continue
			}
			delta := (v.value - a.value) / a.increment
			a.value = v.value
			if a.horizontal {
				dx += delta
			} else {
				dy += delta
			}
		}
	}
	// Scroll-only motion events repeat the position: no move for them.
	if p.w.posf(p.x, p.y) != p.w.lastPos {
		b.ptrMotion(p)
	}
	if dx == 0 && dy == 0 {
		return
	}
	se := platform.ScrollEvent{Kind: platform.PointerMouse, Pos: p.w.lastPos, Mods: p.mods, Time: b.Now()}
	if d.smooth {
		se.Delta = platform.Point{X: float32(dx * smoothScrollPixels), Y: float32(dy * smoothScrollPixels)}
	} else {
		se.Steps = platform.Point{X: float32(dx), Y: float32(dy)}
	}
	b.enqueue(p.w, se)
}
