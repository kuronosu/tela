package remote

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
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
//
//   - 2: the window state, notification and tray events (phase 12).
//   - 3: every WindowOptions field, window keys, window control
//     requests, window info and desktop services (phase 12, ADR-184).
//   - 4: WindowOptions.TitleBar and Backdrop, and their window control
//     requests (ADR-187).
//   - 5: phase 13 (ADR-214): settings v2, clipboard data, drag and drop,
//     native menus (and menu accelerators), global shortcuts, single
//     instance and launches, secrets, associations, ActivateWithToken,
//     MoveToTrash (ADR-216).
//   - 6: platform contract v4 (ADR-225; appmenu.go): QuitRequestedEvent,
//     the app menu bar (SetAppMenu, AppMenuEvent), menu roles,
//     WindowOptions.TitleBarOverlay and WindowState.TitleBarControls.
//   - 7: TrayOptions.Template (ADR-237) and
//     SystemSettings.TitleBarButtons (ADR-238).
//   - 8: SystemSettings.UIFontFamily and MonoFontFamily (ADR-244).
//   - 9: platform contract v5 (ADR-246; access.go): SetAccessibility,
//     AccessibilityEvent and AccessActionEvent.
//   - 10: platform contract v6 (ADR-249): WindowOptions.Transparent and
//     WindowState.Transparent.
//   - 11: platform contract v7 (ADR-252): CaptionClickEvent.
//   - 12: platform contract v8 (ADR-251): Desktop.SetStrings.
//   - 13: platform contract v9 (ADR-253): LaunchEvent.Source and
//     Association.Role.
//   - 14: platform contract v10 (ADR-257): the resize hit kinds
//     (HitResizeTop to HitResizeBottomRight) in SetHitRegions; the
//     encoding is unchanged, but a host of 13 refuses the kinds.
//   - 15: platform contract v11 (ADR-259): LaunchEvent.AtStart,
//     MenuItem.AccessKey and AssociationStatus.Role.
const ProtocolVersion = 15

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
	msgNewWindow      // id, options, parent ID (0: none), key
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

	// Window control (version 3, ADR-184).
	msgWindowCtl  // child→host: id, op (ctl…), arguments
	msgWindowInfo // host→child: id, hidden, restore token of the real window
	msgRunning    // child→host: Run started (the windows of its setup are sent)

	// Drag and drop (version 5, ADR-214).
	msgSetDropEffect // child→host: id, drag session, effect, types

	// Accessibility (version 9, ADR-246; access.go).
	msgSetAccessibility // child→host: id, update
)

// Window control operations of msgWindowCtl.
const (
	ctlMinimize      uint8 = iota + 1
	ctlMaximized           // bool
	ctlFullscreen          // bool
	ctlVisible             // bool
	ctlOnTop               // bool
	ctlMinSize             // size
	ctlMaxSize             // size
	ctlPosition            // i64 x, i64 y
	ctlActivate            //
	ctlIcon                // images
	ctlHitRegions          // regions
	ctlTitleBar            // three colors (version 4, ADR-187)
	ctlBackdrop            // u8
	ctlActivateToken       // token (version 5, ADR-214)
)

// Limits of decoded lists: a corrupt count fails the message instead of
// allocating without bound.
const (
	maxImages  = 32
	maxRegions = 4096
	maxItems   = 4096 // menu items, filters, patterns, actions, paths
	maxDepth   = 16   // submenu nesting
)

var errShort = errors.New("remote: truncated message")

// fail records err as the decoding error unless one is already set.
func (d *dec) fail(err error) {
	if d.err == nil {
		d.err = err
	}
}

// count reads a list length and fails when it is above limit.
func (d *dec) count(limit int, what string) int {
	n := d.u32()
	if int64(n) > int64(limit) {
		d.fail(fmt.Errorf("remote: %d %s, at most %d", n, what, limit))
		return 0
	}
	return int(n)
}

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
	c := s.AccentColor
	e.u8(c.R)
	e.u8(c.G)
	e.u8(c.B)
	e.u8(c.A)
	e.boolean(s.HasAccentColor)
	e.boolean(s.HighContrast)
	e.f64(s.TextScale)
	e.str(s.TitleBarButtons) // protocol 7 (ADR-238)
	e.str(s.UIFontFamily)    // protocol 8 (ADR-244)
	e.str(s.MonoFontFamily)
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
	return platform.SystemSettings{
		DarkMode:            d.boolean(),
		ReducedMotion:       d.boolean(),
		DoubleClickInterval: d.dur(),
		AccentColor:         color.NRGBA{R: d.u8(), G: d.u8(), B: d.u8(), A: d.u8()},
		HasAccentColor:      d.boolean(),
		HighContrast:        d.boolean(),
		TextScale:           d.f64(),
		TitleBarButtons:     d.str(),
		UIFontFamily:        d.str(),
		MonoFontFamily:      d.str(),
	}
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
	evWindowState
	evNotification
	evTray
	// Version 5 (ADR-214).
	evDrag
	evDragDone
	evMenu
	evShortcut
	evShortcutStatus
	evLaunch
	// Version 6 (ADR-225).
	evQuitRequested
	evAppMenu
	// Version 9 (ADR-246).
	evAccessibility
	evAccessAction
	// Version 11 (ADR-252).
	evCaptionClick
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
	case platform.WindowStateEvent:
		e.u8(evWindowState)
		st := ev.State
		e.boolean(st.Maximized)
		e.boolean(st.Fullscreen)
		e.boolean(st.Minimized)
		e.boolean(st.Suspended)
		e.boolean(st.ClientSideDecorations)
		e.boolean(st.HasPosition)
		e.i64(int64(st.Position.X))
		e.i64(int64(st.Position.Y))
		e.rect(st.TitleBarControls)
		e.boolean(st.Transparent)
	case platform.NotificationEvent:
		e.u8(evNotification)
		e.u32(uint32(ev.ID))
		e.u8(uint8(ev.Kind))
		e.str(ev.Action)
	case platform.TrayEvent:
		e.u8(evTray)
		e.u32(uint32(ev.Tray))
		e.u8(uint8(ev.Kind))
		e.u32(uint32(ev.Item))
	case platform.DragEvent:
		e.u8(evDrag)
		e.u8(uint8(ev.Action))
		e.point(ev.Pos)
		e.strs(ev.Types)
		e.u8(uint8(ev.Allowed))
		e.u8(uint8(ev.Mods))
		e.boolean(ev.FromApp)
		e.data(ev.Data)
		e.dur(ev.Time)
	case platform.DragDoneEvent:
		e.u8(evDragDone)
		e.u8(uint8(ev.Effect))
	case platform.MenuEvent:
		e.u8(evMenu)
		e.u32(uint32(ev.Item))
	case platform.ShortcutEvent:
		e.u8(evShortcut)
		e.str(ev.ID)
		e.u8(uint8(ev.Action))
		e.dur(ev.Time)
	case platform.ShortcutStatusEvent:
		e.u8(evShortcutStatus)
		e.str(ev.ID)
		e.u8(uint8(ev.Status))
		e.str(ev.Trigger)
		e.str(ev.Reason)
	case platform.LaunchEvent:
		e.u8(evLaunch)
		e.launch(ev.Launch)
		e.u8(uint8(ev.Source)) // version 13
		e.boolean(ev.AtStart)  // version 15
	case platform.QuitRequestedEvent:
		e.u8(evQuitRequested)
	case platform.AppMenuEvent:
		e.u8(evAppMenu)
		e.u32(uint32(ev.Item))
	case platform.AccessibilityEvent:
		e.u8(evAccessibility)
		e.boolean(ev.Active)
	case platform.AccessActionEvent:
		e.u8(evAccessAction)
		e.u64(uint64(ev.Node))
		e.u8(uint8(ev.Action))
		e.str(ev.Value)
		e.f32(ev.DX)
		e.f32(ev.DY)
		e.i64(int64(ev.Selection.Anchor))
		e.i64(int64(ev.Selection.Focus))
	case platform.CaptionClickEvent:
		e.u8(evCaptionClick)
		e.point(ev.Pos)
		e.dur(ev.Time)
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
	case evWindowState:
		return platform.WindowStateEvent{State: platform.WindowState{
			Maximized:             d.boolean(),
			Fullscreen:            d.boolean(),
			Minimized:             d.boolean(),
			Suspended:             d.boolean(),
			ClientSideDecorations: d.boolean(),
			HasPosition:           d.boolean(),
			Position:              image.Point{X: int(d.i64()), Y: int(d.i64())},
			TitleBarControls:      d.rect(),
			Transparent:           d.boolean(),
		}}
	case evNotification:
		return platform.NotificationEvent{
			ID:     platform.NotificationID(d.u32()),
			Kind:   platform.NotificationEventKind(d.u8()),
			Action: d.str(),
		}
	case evTray:
		return platform.TrayEvent{
			Tray: platform.TrayID(d.u32()),
			Kind: platform.TrayEventKind(d.u8()),
			Item: platform.MenuItemID(d.u32()),
		}
	case evDrag:
		return platform.DragEvent{
			Action:  platform.DragAction(d.u8()),
			Pos:     d.point(),
			Types:   d.strs(),
			Allowed: platform.DropEffect(d.u8()),
			Mods:    platform.Modifiers(d.u8()),
			FromApp: d.boolean(),
			Data:    d.data(),
			Time:    d.dur(),
		}
	case evDragDone:
		return platform.DragDoneEvent{Effect: platform.DropEffect(d.u8())}
	case evMenu:
		return platform.MenuEvent{Item: platform.MenuItemID(d.u32())}
	case evShortcut:
		return platform.ShortcutEvent{ID: d.str(), Action: platform.ShortcutAction(d.u8()), Time: d.dur()}
	case evShortcutStatus:
		return platform.ShortcutStatusEvent{ID: d.str(), Status: platform.ShortcutStatus(d.u8()), Trigger: d.str(), Reason: d.str()}
	case evLaunch:
		return platform.LaunchEvent{Launch: d.launch(), Source: platform.LaunchSource(d.u8()), AtStart: d.boolean()}
	case evQuitRequested:
		return platform.QuitRequestedEvent{}
	case evAppMenu:
		return platform.AppMenuEvent{Item: platform.MenuItemID(d.u32())}
	case evAccessibility:
		return platform.AccessibilityEvent{Active: d.boolean()}
	case evAccessAction:
		return platform.AccessActionEvent{
			Node:      platform.AccessID(d.u64()),
			Action:    platform.AccessAction(d.u8()),
			Value:     d.str(),
			DX:        d.f32(),
			DY:        d.f32(),
			Selection: platform.AccessSelection{Anchor: int(d.i64()), Focus: int(d.i64())},
		}
	case evCaptionClick:
		return platform.CaptionClickEvent{Pos: d.point(), Time: d.dur()}
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

// windowOptions encodes the options of a new window; parent is the ID of
// opts.Parent (0: none), which the caller resolved.
func (e *enc) windowOptions(o platform.WindowOptions, parent uint32) {
	e.str(o.Title)
	e.str(o.AppID)
	e.size(o.Size)
	e.size(o.MinSize)
	e.size(o.MaxSize)
	e.boolean(o.FixedSize)
	e.i64(int64(o.Position.X))
	e.i64(int64(o.Position.Y))
	e.boolean(o.HasPosition)
	e.boolean(o.Maximized)
	e.boolean(o.Fullscreen)
	e.boolean(o.Hidden)
	e.boolean(o.AlwaysOnTop)
	e.boolean(o.Frameless)
	e.u32(parent)
	e.boolean(o.Modal)
	e.str(o.RestoreToken)
	e.titleBar(o.TitleBar)
	e.u8(uint8(o.Backdrop))
	e.boolean(o.TitleBarOverlay)
	e.boolean(o.Transparent)
}

// windowOptions decodes the options of a new window. Parent is left nil:
// the host resolves the parent ID to its own window.
func (d *dec) windowOptions() (platform.WindowOptions, uint32) {
	o := platform.WindowOptions{
		Title:       d.str(),
		AppID:       d.str(),
		Size:        d.size(),
		MinSize:     d.size(),
		MaxSize:     d.size(),
		FixedSize:   d.boolean(),
		Position:    image.Point{X: int(d.i64()), Y: int(d.i64())},
		HasPosition: d.boolean(),
		Maximized:   d.boolean(),
		Fullscreen:  d.boolean(),
		Hidden:      d.boolean(),
		AlwaysOnTop: d.boolean(),
		Frameless:   d.boolean(),
	}
	parent := d.u32()
	o.Modal = d.boolean()
	o.RestoreToken = d.str()
	o.TitleBar = d.titleBar()
	o.Backdrop = d.backdrop()
	o.TitleBarOverlay = d.boolean()
	o.Transparent = d.boolean()
	return o, parent
}

// titleBar encodes the colors of a window's title bar.
func (e *enc) titleBar(c platform.TitleBarColors) {
	for _, w := range [...]platform.WindowColor{c.Background, c.Text, c.Border} {
		e.boolean(w.Set)
		e.u8(w.R)
		e.u8(w.G)
		e.u8(w.B)
	}
}

func (d *dec) titleBar() platform.TitleBarColors {
	var c platform.TitleBarColors
	for _, w := range [...]*platform.WindowColor{&c.Background, &c.Text, &c.Border} {
		w.Set = d.boolean()
		w.R, w.G, w.B = d.u8(), d.u8(), d.u8()
	}
	return c
}

// backdrop decodes a Backdrop; an invalid value fails the message.
func (d *dec) backdrop() platform.Backdrop {
	b := platform.Backdrop(d.u8())
	if !b.Valid() {
		d.fail(fmt.Errorf("remote: invalid backdrop %d", uint8(b)))
		return platform.BackdropDefault
	}
	return b
}

// images encodes an icon (images at several sizes) as non-premultiplied
// RGBA pixels.
func (e *enc) images(imgs []image.Image) {
	e.u32(uint32(len(imgs)))
	for _, img := range imgs {
		n := platform.IconNRGBA(img)
		e.u32(uint32(n.Rect.Dx()))
		e.u32(uint32(n.Rect.Dy()))
		e.b = append(e.b, n.Pix...)
	}
}

func (d *dec) images() []image.Image {
	n := d.count(maxImages, "icon images")
	if n == 0 || d.err != nil {
		return nil
	}
	out := make([]image.Image, 0, n)
	for range n {
		w, h := int(d.u32()), int(d.u32())
		if d.err != nil {
			return nil
		}
		if w <= 0 || h <= 0 || w > platform.MaxIconSide || h > platform.MaxIconSide {
			d.fail(fmt.Errorf("remote: icon image of %dx%d", w, h))
			return nil
		}
		p := d.take(4 * w * h)
		if p == nil {
			return nil
		}
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		copy(img.Pix, p)
		out = append(out, img)
	}
	return out
}

// regions encodes hit regions.
func (e *enc) regions(rs []platform.HitRegion) {
	e.u32(uint32(len(rs)))
	for _, r := range rs {
		e.u8(uint8(r.Kind))
		e.rect(r.Rect)
	}
}

func (d *dec) regions() []platform.HitRegion {
	n := d.count(maxRegions, "hit regions")
	if n == 0 || d.err != nil {
		return nil
	}
	out := make([]platform.HitRegion, n)
	for i := range out {
		out[i] = platform.HitRegion{Kind: platform.HitKind(d.u8()), Rect: d.rect()}
	}
	return out
}

// strs encodes a list of strings.
func (e *enc) strs(ss []string) {
	e.u32(uint32(len(ss)))
	for _, s := range ss {
		e.str(s)
	}
}

func (d *dec) strs() []string {
	n := d.count(maxItems, "strings")
	if n == 0 || d.err != nil {
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = d.str()
	}
	return out
}

// stringMap encodes platform.Strings, sorted by ID so equal sets give equal
// bytes.
func (e *enc) stringMap(m platform.Strings) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	e.u32(uint32(len(keys)))
	for _, k := range keys {
		e.str(k)
		e.str(m[k])
	}
}

func (d *dec) stringMap() platform.Strings {
	n := d.count(maxItems, "strings")
	if n == 0 || d.err != nil {
		return nil
	}
	out := make(platform.Strings, n)
	for i := 0; i < n && d.err == nil; i++ {
		k := d.str()
		out[k] = d.str()
	}
	return out
}

// fileDialog encodes the options of a file dialog; parent is the ID of
// opts.Parent (0: none).
func (e *enc) fileDialog(o platform.FileDialogOptions, parent uint32) {
	e.u32(parent)
	e.str(o.Title)
	e.str(o.Directory)
	e.str(o.Name)
	e.u32(uint32(len(o.Filters)))
	for _, f := range o.Filters {
		e.str(f.Name)
		e.strs(f.Patterns)
	}
	e.boolean(o.Multiple)
}

func (d *dec) fileDialog() (platform.FileDialogOptions, uint32) {
	parent := d.u32()
	o := platform.FileDialogOptions{Title: d.str(), Directory: d.str(), Name: d.str()}
	n := d.count(maxItems, "file filters")
	for range n {
		o.Filters = append(o.Filters, platform.FileFilter{Name: d.str(), Patterns: d.strs()})
	}
	o.Multiple = d.boolean()
	return o, parent
}

// messageBox encodes the options of a message box; parent is the ID of
// opts.Parent (0: none).
func (e *enc) messageBox(o platform.MessageBoxOptions, parent uint32) {
	e.u32(parent)
	e.u8(uint8(o.Kind))
	e.str(o.Title)
	e.str(o.Text)
	e.u8(uint8(o.Buttons))
}

func (d *dec) messageBox() (platform.MessageBoxOptions, uint32) {
	parent := d.u32()
	o := platform.MessageBoxOptions{Kind: platform.MessageKind(d.u8()), Title: d.str(), Text: d.str(), Buttons: platform.MessageButtons(d.u8())}
	return o, parent
}

// notification encodes a notification.
func (e *enc) notification(n platform.Notification) {
	e.str(n.Title)
	e.str(n.Body)
	e.images(n.Icon)
	e.u32(uint32(len(n.Actions)))
	for _, a := range n.Actions {
		e.str(a.ID)
		e.str(a.Label)
	}
}

func (d *dec) notification() platform.Notification {
	n := platform.Notification{Title: d.str(), Body: d.str(), Icon: d.images()}
	c := d.count(maxItems, "notification actions")
	for range c {
		n.Actions = append(n.Actions, platform.NotificationAction{ID: d.str(), Label: d.str()})
	}
	return n
}

// tray encodes the options of a tray icon.
func (e *enc) tray(o platform.TrayOptions) {
	e.images(o.Icon)
	e.str(o.Tooltip)
	e.menu(o.Menu)
	e.boolean(o.Template)
}

func (d *dec) tray() platform.TrayOptions {
	return platform.TrayOptions{Icon: d.images(), Tooltip: d.str(), Menu: d.menu(0), Template: d.boolean()}
}

func (e *enc) menu(items []platform.MenuItem) {
	e.u32(uint32(len(items)))
	for _, it := range items {
		e.u32(uint32(it.ID))
		e.str(it.Label)
		e.str(it.Accelerator)
		e.boolean(it.Separator)
		e.boolean(it.Checkable)
		e.boolean(it.Checked)
		e.boolean(it.Disabled)
		e.u8(uint8(it.Role))
		e.u32(uint32(it.AccessKey)) // version 15
		e.menu(it.Submenu)
	}
}

func (d *dec) menu(depth int) []platform.MenuItem {
	n := d.count(maxItems, "menu items")
	if n == 0 || d.err != nil {
		return nil
	}
	if depth >= maxDepth {
		d.fail(fmt.Errorf("remote: menu nested deeper than %d", maxDepth))
		return nil
	}
	out := make([]platform.MenuItem, 0, n)
	for range n {
		it := platform.MenuItem{ID: platform.MenuItemID(d.u32()), Label: d.str(), Accelerator: d.str(),
			Separator: d.boolean(), Checkable: d.boolean(), Checked: d.boolean(), Disabled: d.boolean(),
			Role: platform.MenuRole(d.u8()), AccessKey: rune(d.u32())}
		if !it.Role.Valid() {
			d.fail(fmt.Errorf("remote: invalid menu role %d", uint8(it.Role)))
			return nil
		}
		it.Submenu = d.menu(depth + 1)
		if d.err != nil {
			return nil
		}
		out = append(out, it)
	}
	return out
}

// --- version 5 (phase 13, ADR-214) ---

// data encodes clipboard or drag data: (MIME type, bytes) entries in
// order.
func (e *enc) data(d platform.Data) {
	e.u32(uint32(len(d)))
	for _, x := range d {
		e.str(x.Type)
		e.blob(x.Bytes)
	}
}

func (d *dec) data() platform.Data {
	n := d.count(maxItems, "data entries")
	if n == 0 || d.err != nil {
		return nil
	}
	out := make(platform.Data, n)
	for i := range out {
		out[i] = platform.DataEntry{Type: d.str(), Bytes: d.blob()}
	}
	if d.err != nil {
		return nil
	}
	return out
}

// optBytes encodes a byte slice that may be nil (absent) as opposed to
// empty.
func (e *enc) optBytes(p []byte) {
	e.boolean(p != nil)
	if p != nil {
		e.blob(p)
	}
}

func (d *dec) optBytes() []byte {
	if !d.boolean() {
		return nil
	}
	p := d.blob()
	if p == nil && d.err == nil {
		p = []byte{}
	}
	return p
}

// rgba encodes an optional premultiplied image (a drag image), rows
// packed.
func (e *enc) rgba(img *image.RGBA) {
	e.boolean(img != nil)
	if img == nil {
		return
	}
	r := img.Rect
	e.u32(uint32(r.Dx()))
	e.u32(uint32(r.Dy()))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		o := img.PixOffset(r.Min.X, y)
		e.b = append(e.b, img.Pix[o:o+4*r.Dx()]...)
	}
}

func (d *dec) rgba() *image.RGBA {
	if !d.boolean() {
		return nil
	}
	w, h := int(d.u32()), int(d.u32())
	if d.err != nil {
		return nil
	}
	if w <= 0 || h <= 0 || w > platform.MaxDragImageSide || h > platform.MaxDragImageSide {
		d.fail(fmt.Errorf("remote: drag image of %dx%d", w, h))
		return nil
	}
	p := d.take(4 * w * h)
	if p == nil {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, p)
	return img
}

// dragOptions encodes the options of a StartDrag.
func (e *enc) dragOptions(o platform.DragOptions) {
	e.data(o.Data)
	e.u8(uint8(o.Allowed))
	e.rgba(o.Image)
	e.i64(int64(o.Hotspot.X))
	e.i64(int64(o.Hotspot.Y))
}

func (d *dec) dragOptions() platform.DragOptions {
	return platform.DragOptions{Data: d.data(), Allowed: platform.DropEffect(d.u8()), Image: d.rgba(),
		Hotspot: image.Point{X: int(d.i64()), Y: int(d.i64())}}
}

// launch encodes how a process was started.
func (e *enc) launch(l platform.Launch) {
	e.strs(l.Args)
	e.str(l.Dir)
	e.str(l.Token)
}

func (d *dec) launch() platform.Launch {
	return platform.Launch{Args: d.strs(), Dir: d.str(), Token: d.str()}
}

// shortcut encodes a global shortcut.
func (e *enc) shortcut(s platform.Shortcut) {
	e.str(s.ID)
	e.u8(uint8(s.Accelerator.Mods))
	e.str(string(s.Accelerator.Key))
	e.str(s.Description)
}

func (d *dec) shortcut() platform.Shortcut {
	return platform.Shortcut{ID: d.str(), Accelerator: platform.Accelerator{Mods: platform.Modifiers(d.u8()), Key: platform.Key(d.str())},
		Description: d.str()}
}

// handlerApp encodes the app of an association request.
func (e *enc) handlerApp(a platform.HandlerApp) {
	e.str(a.ID)
	e.str(a.Name)
	e.str(a.Exec)
	e.strs(a.Args)
	e.str(a.Icon)
}

func (d *dec) handlerApp() platform.HandlerApp {
	return platform.HandlerApp{ID: d.str(), Name: d.str(), Exec: d.str(), Args: d.strs(), Icon: d.str()}
}

// association encodes a scheme or file type.
func (e *enc) association(a platform.Association) {
	e.str(a.Scheme)
	e.str(a.Extension)
	e.str(a.MIMEType)
	e.str(a.Description)
	e.u8(uint8(a.Role))
}

func (d *dec) association() platform.Association {
	return platform.Association{Scheme: d.str(), Extension: d.str(), MIMEType: d.str(), Description: d.str(),
		Role: platform.HandlerRole(d.u8())}
}
