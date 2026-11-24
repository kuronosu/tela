//go:build linux

package x11

import (
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

const (
	hz60  = 16666667 * time.Nanosecond
	hz144 = 6944444 * time.Nanosecond
)

// randrMode is a MODEINFO of a reply.
type randrMode struct {
	id, dotClock   uint32
	htotal, vtotal uint16
	flags          uint32
}

// resourcesReply encodes an RRGetScreenResourcesCurrent reply.
func resourcesReply(configTime uint32, crtcs, outputs []uint32, modes ...randrMode) []byte {
	const names = "1920x1080i60" // padded to 4 bytes, as a server does
	rep := make([]byte, 32)
	rep[0] = 1
	le.PutUint32(rep[8:], 1) // timestamp
	le.PutUint32(rep[12:], configTime)
	le.PutUint16(rep[16:], uint16(len(crtcs)))
	le.PutUint16(rep[18:], uint16(len(outputs)))
	le.PutUint16(rep[20:], uint16(len(modes)))
	le.PutUint16(rep[22:], uint16(len(names)))
	for _, id := range append(append([]uint32(nil), crtcs...), outputs...) {
		rep = le.AppendUint32(rep, id)
	}
	for _, m := range modes {
		info := make([]byte, 32)
		le.PutUint32(info[0:], m.id)
		le.PutUint16(info[4:], 1920)
		le.PutUint16(info[6:], 1080)
		le.PutUint32(info[8:], m.dotClock)
		le.PutUint16(info[12:], 2008) // hsync start
		le.PutUint16(info[14:], 2052)
		le.PutUint16(info[16:], m.htotal)
		le.PutUint16(info[20:], 1084) // vsync start
		le.PutUint16(info[22:], 1089)
		le.PutUint16(info[24:], m.vtotal)
		le.PutUint16(info[26:], uint16(len(names)))
		le.PutUint32(info[28:], m.flags)
		rep = append(rep, info...)
	}
	return append(rep, names...)
}

func TestParseResources(t *testing.T) {
	modes := []randrMode{
		{70, 148500000, 2200, 1125, 5},                  // 1080p60
		{71, 148351648, 2200, 1125, 5},                  // 59.94
		{72, 74250000, 2200, 1125, 5 | 1<<4},            // 1080i: 60 fields
		{73, 12588000, 400, 262, 1<<5 | 1<<1 | 1<<3},    // double scan, 60.05
		{74, 0, 0, 0, 0},                                // a virtual mode
		{75, 0xffffffff, 1, 1, 0},                       // absurd
		{76, 596500000, 2720, 1523, 1<<0 | 1<<3 | 1<<8}, // 1440p144
	}
	rep := resourcesReply(99, []uint32{50, 51, 52}, []uint32{60, 61}, modes...)
	want := resources{
		configTime: 99,
		crtcs:      []uint32{50, 51, 52},
		modes: map[uint32]time.Duration{70: hz60, 71: 16683333, 72: hz60, 73: 16650779, 74: 0, 75: 0,
			76: platform.RefreshInterval(596500000.0 / (2720 * 1523))},
	}
	if got := parseResources(rep); !reflect.DeepEqual(got, want) {
		t.Errorf("parseResources = %+v\nwant %+v", got, want)
	}
	// A truncated reply gives what is whole, and nothing panics.
	for n := range rep {
		parseResources(rep[:n])
	}
	if got := parseResources(rep[:32+20+32+10]); len(got.crtcs) != 3 || len(got.modes) != 1 {
		t.Errorf("truncated in the second mode: %+v", got)
	}
}

// crtcReply encodes an RRGetCrtcInfo reply (with one output).
func crtcReply(status byte, r image.Rectangle, mode uint32) []byte {
	rep := make([]byte, 36)
	rep[0], rep[1] = 1, status
	le.PutUint16(rep[12:], uint16(int16(r.Min.X)))
	le.PutUint16(rep[14:], uint16(int16(r.Min.Y)))
	le.PutUint16(rep[16:], uint16(r.Dx()))
	le.PutUint16(rep[18:], uint16(r.Dy()))
	le.PutUint32(rep[20:], mode)
	le.PutUint16(rep[24:], 1) // rotation
	le.PutUint16(rep[28:], 1) // outputs
	return rep
}

func TestParseCRTC(t *testing.T) {
	modes := map[uint32]time.Duration{70: hz60, 74: 0}
	left := image.Rect(-1920, -100, 0, 980)
	for _, tt := range []struct {
		name string
		rep  []byte
		want crtc
		ok   bool
	}{
		{"active", crtcReply(0, left, 70), crtc{left, hz60}, true},
		{"rate unknown", crtcReply(0, left, 74), crtc{left, 0}, true},
		{"mode not listed", crtcReply(0, left, 80), crtc{left, 0}, true},
		{"off", crtcReply(0, image.Rectangle{}, 0), crtc{}, false},
		{"failed", crtcReply(1, left, 70), crtc{}, false},
		{"short", crtcReply(0, left, 70)[:31], crtc{}, false},
	} {
		if got, ok := parseCRTC(tt.rep, modes); got != tt.want || ok != tt.ok {
			t.Errorf("%s: parseCRTC = %v, %v; want %v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRefreshOn(t *testing.T) {
	a := crtc{image.Rect(0, 0, 1920, 1080), hz60}
	b := crtc{image.Rect(1920, 0, 4480, 1440), hz144}
	unknown := func(c crtc) crtc { return crtc{rect: c.rect} }
	at := func(x, y int) image.Rectangle { return image.Rect(x, y, x+400, y+300) }
	for _, tt := range []struct {
		name  string
		crtcs []crtc
		win   image.Rectangle
		want  time.Duration
	}{
		{"none", nil, at(0, 0), 0},
		{"on one", []crtc{a, b}, at(10, 10), hz60},
		{"on the other", []crtc{a, b}, at(2000, 10), hz144},
		{"most on the first", []crtc{a, b}, at(1920-201, 10), hz60},
		{"most on the second", []crtc{a, b}, at(1920-199, 10), hz144},
		{"half and half", []crtc{a, b}, at(1920-200, 10), hz144},
		{"half and half, listed the other way", []crtc{b, a}, at(1920-200, 10), hz144},
		{"mirrored", []crtc{a, {a.rect, hz144}}, at(10, 10), hz144},
		{"mirrored, one unknown", []crtc{unknown(a), a}, at(10, 10), hz60},
		{"mirrored, the other unknown", []crtc{a, unknown(a)}, at(10, 10), hz60},
		{"most on one that says nothing", []crtc{unknown(a), b}, at(1920-201, 10), 0},
		{"off all", []crtc{a, b}, at(-1000, 0), 0},
		{"no size", []crtc{a, b}, image.Rectangle{Min: image.Pt(5, 5), Max: image.Pt(5, 5)}, 0},
	} {
		if got := refreshOn(tt.crtcs, tt.win); got != tt.want {
			t.Errorf("%s: refreshOn = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// refreshes records FrameEvent.Refresh.
type refreshes []time.Duration

func (r *refreshes) HandleEvent(_ platform.Window, e platform.Event) {
	if f, ok := e.(platform.FrameEvent); ok {
		*r = append(*r, f.Refresh)
	}
}

// TestFrameRefresh: frames carry the rate of the CRTC under the window;
// the CRTCs are read at the first frame on screen and after a notification.
func TestFrameRefresh(t *testing.T) {
	g := newRig(t)
	g.b.randr = randrState{checked: true, ok: true, opcode: 139, firstEvent: 89, current: true}
	g.f.crtcs = []crtc{{image.Rect(0, 0, 1920, 1080), hz60}, {image.Rect(1920, 0, 3840, 1080), hz144}}
	w := fakeWindow(g.b, 7, image.Pt(300, 200), platform.WindowOptions{})
	frame := func(what string, want time.Duration, wantReads int, evs ...xgb.Event) {
		t.Helper()
		for _, e := range evs {
			g.b.translate(xevent{ev: e})
		}
		g.b.endBatch()
		w.VsyncDone() // due without waiting
		w.RequestFrame()
		var got refreshes
		g.b.dispatch(&got)
		if len(got) != 1 || got[0] != want || g.f.reads != wantReads {
			t.Errorf("%s: frames with %v and %d reads, want one with %v and %d", what, got, g.f.reads, want, wantReads)
		}
	}
	notify := func(code byte) xgb.RawEvent {
		raw := make(xgb.RawEvent, 32)
		raw[0] = code
		return raw
	}
	at := func(x int16) xproto.ConfigureNotifyEvent {
		return xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: x, Y: 10, Width: 300, Height: 200}
	}
	frame("not on screen yet", 0, 0)
	g.present(w)
	frame("position unknown", 0, 0)
	frame("on the first", hz60, 1, at(100))
	frame("again", hz60, 1)
	frame("moved", hz144, 1, at(2000))
	g.f.crtcs = []crtc{{image.Rect(0, 0, 3840, 1080), hz60}}
	frame("changed, nothing reported", hz144, 1)
	frame("RRNotify", hz60, 2, notify(90))
	g.f.crtcs = nil
	frame("RRScreenChangeNotify", 0, 3, notify(89))
	frame("an event of another extension", 0, 3, notify(91))
}

// TestCRTCsWire: the requests and replies over a connection, and the
// notification that has them read again.
func TestCRTCsWire(t *testing.T) {
	left, right := image.Rect(0, 0, 1920, 1080), image.Rect(1920, 0, 3840, 1080)
	x, b, _ := newWireX(t, func(x *wireX) {
		x.randrMinor = 6
		x.resources = resourcesReply(99, []uint32{50, 51, 52, 53}, []uint32{60, 61},
			randrMode{70, 148500000, 2200, 1125, 5}, randrMode{71, 356400000, 2200, 1125, 5})
		x.crtcs = map[uint32][]byte{50: crtcReply(0, left, 70), 51: crtcReply(0, image.Rectangle{}, 0), 53: crtcReply(0, right, 71)}
	})
	want := []crtc{{left, hz60}, {right, hz144}}
	if got := (connServer{b}).activeCRTCs(); !reflect.DeepEqual(got, want) { // 52 is an error
		t.Errorf("activeCRTCs = %v, want %v", got, want)
	}
	const read = "RRGetScreenResourcesCurrent 0x1; RRGetCrtcInfo 50 at 99; RRGetCrtcInfo 51 at 99; RRGetCrtcInfo 52 at 99; RRGetCrtcInfo 53 at 99"
	if got, want := x.take(), "QueryVersion 139: client 1.5; RRSelectInput 0x1 mask 3; "+read; got != want {
		t.Errorf("requests %q\nwant %q", got, want)
	}
	if got := (connServer{b}).activeCRTCs(); len(got) != 2 {
		t.Errorf("read again: %v", got)
	}
	if got := x.take(); got != read {
		t.Errorf("requests %q\nwant %q", got, read)
	}

	for _, code := range []byte{wireRandREvent, wireRandREvent + 1} {
		b.randr.fresh = true
		ev := make([]byte, 32)
		ev[0] = code
		x.send(b, ev)
		if b.randr.fresh {
			t.Errorf("event %d: the CRTCs are not read again", code)
		}
	}
}

// A server without RandR 1.3 states no rate and is not asked for changes.
func TestNoCRTCsWire(t *testing.T) {
	for _, minor := range []uint32{0, 2} {
		x, b, _ := newWireX(t, func(x *wireX) { x.randrMinor = minor })
		if got := (connServer{b}).activeCRTCs(); got != nil {
			t.Errorf("RandR 1.%d: activeCRTCs = %v", minor, got)
		}
		want := "QueryVersion 139: client 1.5"
		if minor == 0 {
			want = ""
		}
		if got := x.take(); got != want {
			t.Errorf("RandR 1.%d: requests %q, want %q", minor, got, want)
		}
	}
}
