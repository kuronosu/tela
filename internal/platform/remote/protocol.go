package remote

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Wire format
//
// The socket carries frames: a little-endian uint32 length, then that many
// bytes, the first of which is the message type. Fields are fixed-size
// little-endian integers and IEEE floats, bools as one byte, strings and
// byte slices as a uint32 length plus the bytes. There is no padding and no
// self-description: both ends are built from this file, and the hello /
// welcome exchange checks the version before anything else is read.

// ProtocolVersion is the version of the wire format. A host refuses a
// child of another version (the child gets an error from Dial).
const ProtocolVersion = 1

// magic opens the hello message.
const magic = "TELA"

// maxFrame bounds a frame's length: a corrupt length fails the connection
// instead of allocating gigabytes. Control payloads (saved state) are the
// largest messages.
const maxFrame = 256 << 20

// maxSide bounds a framebuffer side in pixels (shared buffers and damage).
const maxSide = 1 << 15

// msgType is the first byte of a frame.
type msgType uint8

const (
	// Handshake.
	msgHello   msgType = iota + 1 // child→host: magic, version, pid, tag
	msgWelcome                    // host→child: version, host clock, settings, backend name
	msgReject                     // host→child: reason

	// Window operations, child→host. Every one starts with the window ID.
	msgNewWindow      // id, title, app ID, size, min size, fixed
	msgCloseWindow    // id
	msgSetTitle       // id, title
	msgSetCursor      // id, cursor
	msgSetSize        // id, size
	msgSetTextInput   // id, state (or none)
	msgResetTextInput // id
	msgRequestFrame   // id
	msgBuffers        // id, generation, width, height, stride, slots, name (+ fd on Unix)
	msgPresent        // id, generation, slot, metrics, damage

	// Host→child.
	msgEvent      // window ID (0: app-scoped), event
	msgRelease    // id, generation, slot: the host finished reading the slot
	msgBuffersAck // id, generation: the host mapped the buffers of that generation

	// Clipboard.
	msgClipRead  // child→host: request ID
	msgClipReply // host→child: request ID, text, error text
	msgClipWrite // child→host: text

	// Control channel, both directions.
	msgCtlRequest // request ID, kind, payload
	msgCtlReply   // request ID, error text, payload
	msgCtlNotify  // kind, payload
)

var errShort = errors.New("remote: truncated message")

// enc builds one frame.
type enc struct{ b []byte }

// newMsg starts a frame of type t, with room for the length prefix.
func newMsg(t msgType) *enc {
	e := &enc{b: make([]byte, 5, 64)}
	e.b[4] = byte(t)
	return e
}

// bytes finishes the frame (writes the length) and returns it.
func (e *enc) bytes() []byte {
	binary.LittleEndian.PutUint32(e.b, uint32(len(e.b)-4))
	return e.b
}

func (e *enc) u8(v uint8)          { e.b = append(e.b, v) }
func (e *enc) u16(v uint16)        { e.b = binary.LittleEndian.AppendUint16(e.b, v) }
func (e *enc) u32(v uint32)        { e.b = binary.LittleEndian.AppendUint32(e.b, v) }
func (e *enc) u64(v uint64)        { e.b = binary.LittleEndian.AppendUint64(e.b, v) }
func (e *enc) i64(v int64)         { e.u64(uint64(v)) }
func (e *enc) f32(v float32)       { e.u32(math.Float32bits(v)) }
func (e *enc) f64(v float64)       { e.u64(math.Float64bits(v)) }
func (e *enc) dur(v time.Duration) { e.i64(int64(v)) }
func (e *enc) str(s string) {
	e.u32(uint32(len(s)))
	e.b = append(e.b, s...)
}
func (e *enc) blob(p []byte) {
	e.u32(uint32(len(p)))
	e.b = append(e.b, p...)
}
func (e *enc) boolean(v bool) {
	if v {
		e.u8(1)
	} else {
		e.u8(0)
	}
}
func (e *enc) point(p platform.Point) { e.f32(p.X); e.f32(p.Y) }
func (e *enc) size(s platform.Size)   { e.f32(s.W); e.f32(s.H) }
func (e *enc) rect(r platform.Rect)   { e.f32(r.X); e.f32(r.Y); e.f32(r.W); e.f32(r.H) }
func (e *enc) metrics(m platform.Metrics) {
	e.size(m.Size)
	e.f64(m.Scale)
}
func (e *enc) settings(s platform.SystemSettings) {
	e.boolean(s.DarkMode)
	e.boolean(s.ReducedMotion)
	e.dur(s.DoubleClickInterval)
}
func (e *enc) rects(rs []image.Rectangle) {
	e.u32(uint32(len(rs)))
	for _, r := range rs {
		e.u32(uint32(r.Min.X))
		e.u32(uint32(r.Min.Y))
		e.u32(uint32(r.Max.X))
		e.u32(uint32(r.Max.Y))
	}
}

// dec reads the payload of one frame. The first error sticks; later reads
// return zero values, and end reports it.
type dec struct {
	b   []byte
	err error
}

func (d *dec) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || len(d.b) < n {
		d.err = errShort
		return nil
	}
	p := d.b[:n]
	d.b = d.b[n:]
	return p
}

func (d *dec) u8() uint8 {
	if p := d.take(1); p != nil {
		return p[0]
	}
	return 0
}
func (d *dec) u16() uint16 {
	if p := d.take(2); p != nil {
		return binary.LittleEndian.Uint16(p)
	}
	return 0
}
func (d *dec) u32() uint32 {
	if p := d.take(4); p != nil {
		return binary.LittleEndian.Uint32(p)
	}
	return 0
}
func (d *dec) u64() uint64 {
	if p := d.take(8); p != nil {
		return binary.LittleEndian.Uint64(p)
	}
	return 0
}
func (d *dec) i64() int64            { return int64(d.u64()) }
func (d *dec) f32() float32          { return math.Float32frombits(d.u32()) }
func (d *dec) f64() float64          { return math.Float64frombits(d.u64()) }
func (d *dec) dur() time.Duration    { return time.Duration(d.i64()) }
func (d *dec) boolean() bool         { return d.u8() != 0 }
func (d *dec) point() platform.Point { return platform.Point{X: d.f32(), Y: d.f32()} }
func (d *dec) size() platform.Size   { return platform.Size{W: d.f32(), H: d.f32()} }
func (d *dec) rect() platform.Rect {
	return platform.Rect{X: d.f32(), Y: d.f32(), W: d.f32(), H: d.f32()}
}
func (d *dec) metrics() platform.Metrics {
	return platform.Metrics{Size: d.size(), Scale: d.f64()}
}
func (d *dec) settings() platform.SystemSettings {
	return platform.SystemSettings{DarkMode: d.boolean(), ReducedMotion: d.boolean(), DoubleClickInterval: d.dur()}
}
func (d *dec) str() string {
	n := d.u32()
	return string(d.take(int(n)))
}

// blob returns a copy of a byte slice field (the frame buffer is reused).
func (d *dec) blob() []byte {
	n := d.u32()
	p := d.take(int(n))
	if p == nil {
		return nil
	}
	return append([]byte(nil), p...)
}

// rects reads damage rectangles; each must lie inside bounds and be
// non-empty.
func (d *dec) rects(bounds image.Rectangle) []image.Rectangle {
	n := d.u32()
	if d.err != nil {
		return nil
	}
	if int64(n)*16 > int64(len(d.b)) {
		d.err = errShort
		return nil
	}
	rs := make([]image.Rectangle, n)
	for i := range rs {
		r := image.Rect(int(int32(d.u32())), int(int32(d.u32())), int(int32(d.u32())), int(int32(d.u32())))
		if r.Empty() || !r.In(bounds) || r.Min.X < 0 || r.Min.Y < 0 {
			if d.err == nil {
				d.err = fmt.Errorf("remote: damage rectangle %v outside the frame %v", r, bounds)
			}
			return nil
		}
		rs[i] = r
	}
	return rs
}

// end reports the first error, or an error if bytes are left over.
func (d *dec) end() error {
	if d.err == nil && len(d.b) != 0 {
		d.err = fmt.Errorf("remote: %d unexpected trailing bytes", len(d.b))
	}
	return d.err
}

// Event tags. Every type of platform's sealed event set has one; the test
// TestEveryEventHasAnEncoding fails when platform gains an event type that
// is missing here.
const (
	evConfigure uint8 = iota + 1
	evFrame
	evCloseRequested
	evFocus
	evWake
	evSettings
	evPointer
	evScroll
	evKey
	evIME
	evIMESession
)

// event encodes ev. It panics on an event type it does not know, which
// can only be a platform event added without an encoding (a test catches
// it).
func (e *enc) event(ev platform.Event) {
	switch ev := ev.(type) {
	case platform.ConfigureEvent:
		e.u8(evConfigure)
		e.metrics(ev.Metrics)
	case platform.FrameEvent:
		e.u8(evFrame)
		e.dur(ev.Time)
	case platform.CloseRequestedEvent:
		e.u8(evCloseRequested)
	case platform.FocusEvent:
		e.u8(evFocus)
		e.boolean(ev.Focused)
	case platform.WakeEvent:
		e.u8(evWake)
	case platform.SettingsEvent:
		e.u8(evSettings)
		e.settings(ev.Settings)
	case platform.PointerEvent:
		e.u8(evPointer)
		e.u8(uint8(ev.Action))
		e.u8(uint8(ev.Kind))
		e.u32(uint32(ev.ID))
		e.point(ev.Pos)
		e.u8(uint8(ev.Button))
		e.u8(uint8(ev.Buttons))
		e.u8(uint8(ev.Mods))
		e.dur(ev.Time)
	case platform.ScrollEvent:
		e.u8(evScroll)
		e.u8(uint8(ev.Kind))
		e.point(ev.Pos)
		e.point(ev.Delta)
		e.point(ev.Steps)
		e.u8(uint8(ev.Phase))
		e.u8(uint8(ev.Mods))
		e.dur(ev.Time)
	case platform.KeyEvent:
		e.u8(evKey)
		e.u8(uint8(ev.Action))
		e.u32(uint32(ev.Code))
		e.str(string(ev.Key))
		e.str(ev.Text)
		e.u8(uint8(ev.Location))
		e.boolean(ev.Repeat)
		e.boolean(ev.Synthetic)
		e.u8(uint8(ev.Mods))
		e.dur(ev.Time)
	case platform.IMEEvent:
		e.u8(evIME)
		e.i64(int64(ev.DeleteBefore))
		e.i64(int64(ev.DeleteAfter))
		e.str(ev.Commit)
		e.str(ev.Preedit)
		e.i64(int64(ev.CursorStart))
		e.i64(int64(ev.CursorEnd))
	case platform.IMESessionEvent:
		e.u8(evIMESession)
		e.boolean(ev.Active)
	default:
		panic(fmt.Sprintf("remote: no wire encoding for event %T; add one to protocol.go", ev))
	}
}

// event decodes an event, or sets an error for an unknown tag.
func (d *dec) event() platform.Event {
	switch tag := d.u8(); tag {
	case evConfigure:
		return platform.ConfigureEvent{Metrics: d.metrics()}
	case evFrame:
		return platform.FrameEvent{Time: d.dur()}
	case evCloseRequested:
		return platform.CloseRequestedEvent{}
	case evFocus:
		return platform.FocusEvent{Focused: d.boolean()}
	case evWake:
		return platform.WakeEvent{}
	case evSettings:
		return platform.SettingsEvent{Settings: d.settings()}
	case evPointer:
		return platform.PointerEvent{
			Action:  platform.PointerAction(d.u8()),
			Kind:    platform.PointerKind(d.u8()),
			ID:      platform.PointerID(d.u32()),
			Pos:     d.point(),
			Button:  platform.PointerButton(d.u8()),
			Buttons: platform.Buttons(d.u8()),
			Mods:    platform.Modifiers(d.u8()),
			Time:    d.dur(),
		}
	case evScroll:
		return platform.ScrollEvent{
			Kind:  platform.PointerKind(d.u8()),
			Pos:   d.point(),
			Delta: d.point(),
			Steps: d.point(),
			Phase: platform.ScrollPhase(d.u8()),
			Mods:  platform.Modifiers(d.u8()),
			Time:  d.dur(),
		}
	case evKey:
		return platform.KeyEvent{
			Action:    platform.KeyAction(d.u8()),
			Code:      platform.KeyCode(d.u32()),
			Key:       platform.Key(d.str()),
			Text:      d.str(),
			Location:  platform.KeyLocation(d.u8()),
			Repeat:    d.boolean(),
			Synthetic: d.boolean(),
			Mods:      platform.Modifiers(d.u8()),
			Time:      d.dur(),
		}
	case evIME:
		return platform.IMEEvent{
			DeleteBefore: int(d.i64()),
			DeleteAfter:  int(d.i64()),
			Commit:       d.str(),
			Preedit:      d.str(),
			CursorStart:  int(d.i64()),
			CursorEnd:    int(d.i64()),
		}
	case evIMESession:
		return platform.IMESessionEvent{Active: d.boolean()}
	default:
		if d.err == nil {
			d.err = fmt.Errorf("remote: unknown event tag %d", tag)
		}
		return nil
	}
}

// textInput encodes an optional text input state.
func (e *enc) textInput(s *platform.TextInputState) {
	e.boolean(s != nil)
	if s == nil {
		return
	}
	e.rect(s.CursorArea)
	e.u8(uint8(s.Purpose))
	e.boolean(s.Multiline)
	e.boolean(s.Surrounding != nil)
	if st := s.Surrounding; st != nil {
		e.str(st.Text)
		e.i64(int64(st.Cursor))
		e.i64(int64(st.Anchor))
	}
}

func (d *dec) textInput() *platform.TextInputState {
	if !d.boolean() {
		return nil
	}
	s := &platform.TextInputState{CursorArea: d.rect(), Purpose: platform.TextPurpose(d.u8()), Multiline: d.boolean()}
	if d.boolean() {
		s.Surrounding = &platform.SurroundingText{Text: d.str(), Cursor: int(d.i64()), Anchor: int(d.i64())}
	}
	return s
}

// windowOptions encodes the options of a new window.
func (e *enc) windowOptions(o platform.WindowOptions) {
	e.str(o.Title)
	e.str(o.AppID)
	e.size(o.Size)
	e.size(o.MinSize)
	e.boolean(o.FixedSize)
}

func (d *dec) windowOptions() platform.WindowOptions {
	return platform.WindowOptions{Title: d.str(), AppID: d.str(), Size: d.size(), MinSize: d.size(), FixedSize: d.boolean()}
}
