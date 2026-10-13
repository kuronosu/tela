//go:build linux

package x11

import (
	"bytes"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

const xiOpcode = 131

// The builders below write XInput2 wire data as an X server does
// (XI2proto.h): little-endian, FP1616 positions, FP3232 valuator values.

func putFP3232(b []byte, v float64) {
	i := math.Floor(v)
	le.PutUint32(b, uint32(int32(i)))
	le.PutUint32(b[4:], uint32((v-i)*(1<<32)))
}

func putFP1616(b []byte, v float64) { le.PutUint32(b, uint32(int32(v*65536))) }

func genericEvent(evtype uint16, size int) xgb.RawEvent {
	size = pad4(size)
	e := make(xgb.RawEvent, size)
	e[0], e[1] = xgb.GenericEventCode, xiOpcode
	le.PutUint32(e[4:], uint32((size-32)/4))
	le.PutUint16(e[8:], evtype)
	le.PutUint16(e[10:], 2) // the master pointer
	return e
}

type xiEv struct {
	evtype    uint16
	source    uint16
	detail    uint32
	x, y      float64 // physical
	flags     uint32
	buttons   []int // held before the event
	mods      uint32
	valuators map[int]float64
}

func (v xiEv) bytes() xgb.RawEvent {
	maxV := -1
	for n := range v.valuators {
		maxV = max(maxV, n)
	}
	vwords := (maxV + 32) / 32
	e := genericEvent(v.evtype, 80+4+4*vwords+8*len(v.valuators))
	le.PutUint32(e[16:], v.detail)
	le.PutUint32(e[24:], 7) // event window
	putFP1616(e[40:], v.x)
	putFP1616(e[44:], v.y)
	le.PutUint16(e[48:], 1)
	le.PutUint16(e[50:], uint16(vwords))
	le.PutUint16(e[52:], v.source)
	le.PutUint32(e[56:], v.flags)
	le.PutUint32(e[60:], v.mods)
	for _, n := range v.buttons {
		e[80+n/8] |= 1 << (n % 8)
	}
	off := 84 + 4*vwords
	for n := 0; n <= maxV; n++ {
		val, ok := v.valuators[n]
		if !ok {
			continue
		}
		e[84+n/8] |= 1 << (n % 8)
		putFP3232(e[off:], val)
		off += 8
	}
	return e
}

func xiCrossing(evtype uint16, x, y float64, mode, detail byte, buttons []int, mods uint32) xgb.RawEvent {
	e := genericEvent(evtype, 76)
	le.PutUint16(e[16:], 10)
	e[18], e[19] = mode, detail
	le.PutUint32(e[24:], 7)
	putFP1616(e[40:], x)
	putFP1616(e[44:], y)
	le.PutUint16(e[50:], 1)
	le.PutUint32(e[52:], mods)
	for _, n := range buttons {
		e[72+n/8] |= 1 << (n % 8)
	}
	return e
}

func valuatorClass(number uint16, value float64) []byte {
	c := make([]byte, 44)
	le.PutUint16(c, xiValuatorClass)
	le.PutUint16(c[2:], 11)
	le.PutUint16(c[6:], number)
	putFP3232(c[28:], value)
	return c
}

func scrollClass(number, typ uint16, increment float64) []byte {
	c := make([]byte, 24)
	le.PutUint16(c, xiScrollClass)
	le.PutUint16(c[2:], 6)
	le.PutUint16(c[6:], number)
	le.PutUint16(c[8:], typ)
	putFP3232(c[16:], increment)
	return c
}

func buttonClass() []byte {
	c := make([]byte, 16) // button class with a 1-word state mask, no labels counted here
	le.PutUint16(c, 1)
	le.PutUint16(c[2:], 4)
	return c
}

func deviceChanged(source uint16, classes ...[]byte) xgb.RawEvent {
	body := bytes.Join(classes, nil)
	e := genericEvent(xiDeviceChanged, 32+len(body))
	// xXIDeviceChangedEvent (XI2proto.h): time at 12, num_classes at 16,
	// sourceid at 18, reason at 20. A non-zero time catches decoders that
	// read the class count or the source from the time field.
	le.PutUint32(e[12:], 0xdeadbeef)
	le.PutUint16(e[16:], uint16(len(classes)))
	le.PutUint16(e[18:], source)
	e[20] = 1 // XISlaveSwitch
	copy(e[32:], body)
	return e
}

func hierarchyChanged(ids ...uint16) xgb.RawEvent {
	e := genericEvent(xiHierarchyChanged, 32+12*len(ids))
	le.PutUint16(e[20:], uint16(len(ids)))
	for i, id := range ids {
		le.PutUint16(e[32+12*i:], id)
		le.PutUint32(e[32+12*i+8:], 1<<3) // XISlaveRemoved
	}
	return e
}

// queryDeviceReply writes an XIQueryDevice reply.
func queryDeviceReply(devs ...struct {
	id      uint16
	name    string
	classes [][]byte
}) []byte {
	r := make([]byte, 32)
	r[0] = 1
	le.PutUint16(r[8:], uint16(len(devs)))
	for _, d := range devs {
		info := make([]byte, 12)
		le.PutUint16(info, d.id)
		le.PutUint16(info[2:], 3) // XISlavePointer
		le.PutUint16(info[6:], uint16(len(d.classes)))
		le.PutUint16(info[8:], uint16(len(d.name)))
		info[10] = 1
		r = append(r, info...)
		name := make([]byte, pad4(len(d.name)))
		copy(name, d.name)
		r = append(r, name...)
		r = append(r, bytes.Join(d.classes, nil)...)
	}
	le.PutUint32(r[4:], uint32((len(r)-32)/4))
	return r
}

type devSpec = struct {
	id      uint16
	name    string
	classes [][]byte
}

func TestParseXIDevices(t *testing.T) {
	r := queryDeviceReply(
		devSpec{2, "Virtual core pointer", [][]byte{buttonClass(), valuatorClass(0, 100), valuatorClass(1, 50)}},
		devSpec{10, "Logitech USB Receiver Mouse", [][]byte{
			buttonClass(), valuatorClass(2, 7), valuatorClass(3, 1000.5),
			scrollClass(2, xiScrollHorizontal, 120), scrollClass(3, xiScrollVertical, 120),
		}},
		devSpec{11, "SynPS/2 Synaptics TouchPad", [][]byte{valuatorClass(3, -4), scrollClass(3, xiScrollVertical, 15)}},
		devSpec{12, "bad scroll class", [][]byte{scrollClass(3, xiScrollVertical, 0), scrollClass(4, 9, 1)}},
	)
	devs := parseXIDevices(r)
	if len(devs) != 4 {
		t.Fatalf("%d devices, want 4", len(devs))
	}
	if d := devs[0]; d.id != 2 || d.name != "Virtual core pointer" || len(d.axes) != 0 || d.smooth {
		t.Errorf("device 2 = %+v", d)
	}
	d := devs[1]
	if d.id != 10 || d.smooth || len(d.axes) != 2 {
		t.Fatalf("device 10 = %+v", d)
	}
	if a := d.axis(2); a == nil || !a.horizontal || a.increment != 120 || a.value != 7 || !a.valid {
		t.Errorf("horizontal axis = %+v", a)
	}
	if a := d.axis(3); a == nil || a.horizontal || a.increment != 120 || a.value != 1000.5 || !a.valid {
		t.Errorf("vertical axis = %+v", a)
	}
	if d := devs[2]; !d.smooth || d.axis(3) == nil || d.axis(3).value != -4 {
		t.Errorf("touchpad = %+v", d)
	}
	if d := devs[3]; len(d.axes) != 0 {
		t.Errorf("zero increment or unknown scroll type kept: %+v", d.axes)
	}
	// Truncated replies stop parsing without panicking.
	for n := 32; n < len(r); n += 7 {
		parseXIDevices(r[:n])
	}
}

func TestXI2Requests(t *testing.T) {
	got := xiSelectEventsRequest(xiOpcode, 0x1234, xiAllMasterDevices, xiWindowEvents)
	want := []byte{xiOpcode, 46, 5, 0, 0x34, 0x12, 0, 0, 1, 0, 0, 0, 1, 0, 1, 0, 0xf0, 0x01, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("XISelectEvents = % x\nwant % x", got, want)
	}
	got = xkbSelectEventsRequest(135)
	want = []byte{135, 1, 5, 0, 0x00, 0x01, 0x07, 0, 0, 0, 0x01, 0, 0xff, 0, 0x07, 0, 0xff, 0x3f, 0xff, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("XkbSelectEvents = % x\nwant % x", got, want)
	}
	got = xkbPerClientFlagsRequest(135)
	want = []byte{135, 21, 7, 0, 0x00, 0x01, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("XkbPerClientFlags = % x\nwant % x", got, want)
	}
	if got := xkbUseExtensionRequest(135); !bytes.Equal(got, []byte{135, 0, 2, 0, 1, 0, 0, 0}) {
		t.Errorf("XkbUseExtension = % x", got)
	}
}

// xiBackend returns a fake backend at scale 2 using XInput2, with a
// fake device query.
func xiBackend(t *testing.T) (*Backend, func(...xgb.Event) string, *int) {
	t.Helper()
	b, _ := fakeBackend(2)
	queries := 0
	devices := map[uint16]*xiDevice{
		// The reply's valuator values are the current ones (the
		// baselines).
		10: {id: 10, name: "Logitech USB Receiver Mouse", axes: []*scrollAxis{
			{number: 2, horizontal: true, increment: 120, value: 50, valid: true},
			{number: 3, increment: 120, value: 1000, valid: true},
		}},
		11: {id: 11, name: "SynPS/2 Synaptics TouchPad", smooth: true, axes: []*scrollAxis{
			{number: 2, horizontal: true, increment: 15, valid: true},
			{number: 3, increment: 15, valid: true},
		}},
		12: {id: 12, name: "Old wheel mouse"},
	}
	b.xi = xiState{
		extension: extension{present: true, opcode: xiOpcode},
		ok:        true,
		devices:   map[uint16]*xiDevice{},
		query: func(id uint16) ([]*xiDevice, error) {
			queries++
			d := *devices[id] // a fresh copy, like a new reply
			var axes []*scrollAxis
			for _, a := range d.axes {
				c := *a
				axes = append(axes, &c)
			}
			d.axes = axes
			return []*xiDevice{&d}, nil
		},
	}
	r := &recorder{}
	feed := func(evs ...xgb.Event) string {
		for _, e := range evs {
			b.translate(xevent{ev: e})
		}
		b.kbd.flushAll()
		b.dispatch(r)
		return r.take()
	}
	feed()
	return b, feed, &queries
}

func TestXI2Scroll(t *testing.T) {
	b, feed, queries := xiBackend(t)
	motion := func(source uint16, x float64, vals map[int]float64) xgb.RawEvent {
		return xiEv{evtype: xiMotion, source: source, x: x, y: 40, valuators: vals}.bytes()
	}
	for _, c := range []struct {
		name string
		evs  []xgb.Event
		want string
	}{
		{"enter", []xgb.Event{xiCrossing(xiEnter, 20, 40, 0, xproto.NotifyDetailNonlinear, nil, 0)},
			"pointer enter 10,20 btn=0 held=0 mods=0"},
		// The first event of a device queries it; the reply has the
		// current values, so nothing scrolls.
		{"first event", []xgb.Event{motion(10, 20, map[int]float64{3: 1000, 2: 50})}, ""},
		{"wheel notch down", []xgb.Event{motion(10, 20, map[int]float64{3: 1120})}, "scroll 0,1 at 10,20"},
		{"high-resolution half notch up", []xgb.Event{motion(10, 20, map[int]float64{3: 1060})}, "scroll 0,-0.5 at 10,20"},
		{"horizontal", []xgb.Event{motion(10, 20, map[int]float64{2: 290})}, "scroll 2,0 at 10,20"},
		// The wheel buttons the server emulates from the valuators are
		// dropped (else every notch scrolls twice).
		{"emulated buttons", []xgb.Event{
			xiEv{evtype: xiButtonPress, source: 10, detail: 5, x: 20, y: 40, flags: xiPointerEmulated}.bytes(),
			xiEv{evtype: xiButtonRelease, source: 10, detail: 5, x: 20, y: 40, flags: xiPointerEmulated}.bytes(),
		}, ""},
		{"move and scroll", []xgb.Event{motion(10, 22, map[int]float64{0: 22, 1: 40, 3: 1180})},
			"pointer move 11,20 btn=0 held=0 mods=0; scroll 0,1 at 11,20"},
		// A touchpad: precise pixels. Its first event queries the device,
		// whose reply carries the current values (valid baselines).
		{"touchpad", []xgb.Event{motion(11, 22, map[int]float64{3: 45})}, "scroll 0,0 at 11,20 delta 0,45"},
		{"touchpad with Shift", []xgb.Event{xiEv{evtype: xiMotion, source: 11, x: 22, y: 40, mods: 1, valuators: map[int]float64{2: -7.5}}.bytes()},
			"scroll 0,0 at 11,20 delta -7.5,0 mods=1"},
		// A device without scroll valuators sends real wheel buttons.
		{"legacy wheel", []xgb.Event{
			xiEv{evtype: xiButtonPress, source: 12, detail: 4, x: 22, y: 40}.bytes(),
			xiEv{evtype: xiButtonRelease, source: 12, detail: 4, x: 22, y: 40}.bytes(),
		}, "scroll 0,-1 at 11,20"},
		// Another slave drives the master: XI_DeviceChanged carries the
		// current values.
		{"device changed", []xgb.Event{
			deviceChanged(10, buttonClass(), valuatorClass(3, 5000), scrollClass(3, xiScrollVertical, 120)),
			motion(10, 22, map[int]float64{3: 5240}),
		}, "scroll 0,2 at 11,20"},
		// Leaving and entering again drops the baselines.
		{"leave, enter", []xgb.Event{
			xiCrossing(xiLeave, 22, 40, 0, xproto.NotifyDetailNonlinear, nil, 0),
			xiCrossing(xiEnter, 22, 40, 0, xproto.NotifyDetailNonlinear, nil, 0),
			motion(10, 22, map[int]float64{3: 9000}),
			motion(10, 22, map[int]float64{3: 9120}),
		}, "pointer leave 11,20 btn=0 held=0 mods=0; pointer enter 11,20 btn=0 held=0 mods=0; scroll 0,1 at 11,20"},
	} {
		if got := feed(c.evs...); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	if *queries != 2 {
		t.Errorf("%d device queries, want 2 (devices 10 and 11, once each; 12 sent only buttons)", *queries)
	}
	// A hierarchy change forgets the device: it is queried again.
	feed(hierarchyChanged(11), motion(11, 22, map[int]float64{3: 60}))
	if *queries != 3 {
		t.Errorf("%d device queries after a hierarchy change, want 3", *queries)
	}
	if _, ok := b.nextDeadline(); ok || b.ready() {
		t.Errorf("idle backend has a deadline or work after scrolling")
	}
}

// TestXI2Buttons: XI2 button, motion and crossing events give the same
// platform events as the core ones (TestTranslateAndDispatch,
// TestLeaveDeferredDuringImplicitGrab), with the side buttons from the XI2
// button mask and Mods from the base and latched modifiers.
func TestXI2Buttons(t *testing.T) {
	_, feed, _ := xiBackend(t)
	ev := func(evtype uint16, detail uint32, x float64, held []int, mods uint32) xgb.RawEvent {
		return xiEv{evtype: evtype, source: 10, detail: detail, x: x, y: 20, buttons: held, mods: mods}.bytes()
	}
	got := feed(
		xiCrossing(xiEnter, 20, 20, 0, xproto.NotifyDetailNonlinear, nil, 0),
		ev(xiMotion, 0, 22.5, nil, 1),
		ev(xiButtonPress, 1, 22.5, nil, 0),
		ev(xiButtonPress, 8, 22.5, []int{1}, 0),
		ev(xiButtonRelease, 1, 22.5, []int{1, 8}, 0),
		ev(xiButtonRelease, 8, 22.5, []int{8}, 0),
		// Drag out and release outside: the leave follows the release.
		ev(xiButtonPress, 1, 22.5, nil, 0),
		xiCrossing(xiLeave, -2, 20, 0, xproto.NotifyDetailNonlinear, []int{1}, 0),
		ev(xiMotion, 0, -40, []int{1}, 0),
		ev(xiButtonRelease, 1, -40, []int{1}, 0),
	)
	want := lines(
		"pointer enter 10,10 btn=0 held=0 mods=0",
		"pointer move 11.25,10 btn=0 held=0 mods=1",
		"pointer down 11.25,10 btn=1 held=1 mods=0",
		"pointer down 11.25,10 btn=4 held=1001 mods=0",
		"pointer up 11.25,10 btn=1 held=1000 mods=0",
		"pointer up 11.25,10 btn=4 held=0 mods=0",
		"pointer down 11.25,10 btn=1 held=1 mods=0",
		"pointer move -20,10 btn=0 held=1 mods=0",
		"pointer up -20,10 btn=1 held=0 mods=0",
		"pointer leave -20,10 btn=0 held=0 mods=0")
	if got != want {
		t.Errorf("XI2 buttons:\n got %s\nwant %s", got, want)
	}
	// A touchscreen tap arrives as emulated buttons 1: kept (Tela has no
	// XI2 touch input yet).
	got = feed(
		xiCrossing(xiEnter, 20, 20, 0, xproto.NotifyDetailNonlinear, nil, 0),
		xiEv{evtype: xiButtonPress, source: 10, detail: 1, x: 20, y: 20, flags: xiPointerEmulated}.bytes(),
		xiEv{evtype: xiButtonRelease, source: 10, detail: 1, x: 20, y: 20, buttons: []int{1}, flags: xiPointerEmulated}.bytes(),
	)
	want = lines(
		"pointer enter 10,10 btn=0 held=0 mods=0",
		"pointer down 10,10 btn=1 held=1 mods=0",
		"pointer up 10,10 btn=1 held=0 mods=0")
	if got != want {
		t.Errorf("emulated touch buttons:\n got %s\nwant %s", got, want)
	}
	// Malformed events are ignored.
	short := ev(xiMotion, 0, 30, nil, 0)[:60]
	bad := ev(xiMotion, 0, 30, nil, 0)
	le.PutUint16(bad[50:], 200) // valuator mask longer than the event
	if got := feed(short, bad, xiCrossing(xiEnter, 1, 1, 0, 0, nil, 0)[:50]); got != "" {
		t.Errorf("malformed events delivered %q", got)
	}
}
