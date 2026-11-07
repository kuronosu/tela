package platform

import (
	"errors"
	"image"
	"slices"
	"strings"
	"time"
)

// DropEffect is what a drop does with the data: copy it, move it (the
// source deletes its copy) or link to it. As a set it is the effects a
// drag source allows ([DragOptions].Allowed, [DragEvent].Allowed); as a
// single value, the effect a target accepts ([Window.SetDropEffect]) or the
// outcome of a drag ([DragDoneEvent]). The bits are Win32's DROPEFFECT_COPY,
// MOVE and LINK, and Wayland's copy and move dnd actions.
type DropEffect uint8

const (
	// DropNone refuses the drop, or reports that a drag was refused or
	// cancelled.
	DropNone DropEffect = 0
	DropCopy DropEffect = 1
	DropMove DropEffect = 2
	DropLink DropEffect = 4
	// DropAll is every effect.
	DropAll = DropCopy | DropMove | DropLink
)

func (e DropEffect) String() string {
	if e == DropNone {
		return "none"
	}
	if e&^DropAll != 0 {
		return "DropEffect(invalid)"
	}
	var parts []string
	for _, x := range [...]struct {
		e DropEffect
		n string
	}{{DropCopy, "copy"}, {DropMove, "move"}, {DropLink, "link"}} {
		if e&x.e != 0 {
			parts = append(parts, x.n)
		}
	}
	return strings.Join(parts, "|")
}

// Has reports whether the set e contains the single effect x.
func (e DropEffect) Has(x DropEffect) bool { return x != DropNone && e&x == x }

// single reports whether e is DropNone or exactly one effect.
func (e DropEffect) single() bool {
	return e == DropNone || e == DropCopy || e == DropMove || e == DropLink
}

// DefaultDropEffect returns the effect a target picks for a drag that
// allows the set allowed while the modifiers mods are held, following the
// convention of Windows file managers and browsers: Ctrl+Shift links,
// Ctrl copies, Shift moves, and no modifier copies (move when copying is
// not allowed, then link). A modifier whose effect is not allowed falls
// back to the unmodified choice. DropNone when allowed is empty.
func DefaultDropEffect(allowed DropEffect, mods Modifiers) DropEffect {
	var want DropEffect
	switch {
	case mods&(ModCtrl|ModShift) == ModCtrl|ModShift:
		want = DropLink
	case mods&ModCtrl != 0:
		want = DropCopy
	case mods&ModShift != 0:
		want = DropMove
	}
	if allowed.Has(want) {
		return want
	}
	for _, e := range [...]DropEffect{DropCopy, DropMove, DropLink} {
		if allowed.Has(e) {
			return e
		}
	}
	return DropNone
}

// DragAction is what happened in a [DragEvent].
type DragAction uint8

const (
	// DragEnter: a drag entered the window's content area. It starts the
	// drag's session over the window.
	DragEnter DragAction = iota
	// DragOver: the drag moved over the window, or its modifiers or
	// allowed effects changed.
	DragOver
	// DragLeave: the drag left the window, was cancelled, or was released
	// while the window refused it. It ends the session.
	DragLeave
	// DragDrop: the user released the drag over the window while the
	// window accepted it. Data holds what the app asked for. It ends the
	// session (no DragLeave follows).
	DragDrop
)

var dragActionNames = [...]string{"enter", "over", "leave", "drop"}

func (a DragAction) String() string {
	if int(a) < len(dragActionNames) {
		return dragActionNames[a]
	}
	return "DragAction(invalid)"
}

// DragEvent is a drag of another application, or of this one, over the
// window: the target side of drag and drop (see "Drag and drop and the
// clipboard" in the package documentation). A session over a window is a
// DragEnter, any number of DragOver, then one DragLeave or DragDrop. The
// window answers with [Window.SetDropEffect].
type DragEvent struct {
	Action DragAction
	// Pos is the pointer position in logical pixels relative to the
	// content area (the last known one for DragLeave).
	Pos Point
	// Types are the MIME types the source offers (normalized: Linux
	// targets mapped with SelectionType, Win32 formats mapped to the
	// well-known types), in the source's order; the same for every event
	// of the session. Only the types are known before the drop (HTML's
	// protected mode).
	Types []string
	// Allowed is the set of effects the source allows.
	Allowed DropEffect
	// Mods are the keyboard modifiers held (the app picks the effect from
	// them: DefaultDropEffect).
	Mods Modifiers
	// FromApp reports that the drag was started by this app
	// (Window.StartDrag, in any of its windows).
	FromApp bool
	// Data, for DragDrop only, is the content of the types the app asked
	// for in its latest SetDropEffect (or the well-known types offered,
	// when it named none), in the source's order, read by the backend
	// before the event is delivered. A type the source failed to deliver
	// is missing from Data but listed in Types.
	Data Data
	// Time is when the event happened, in the time base of [Backend.Now].
	Time time.Duration
}

// DragDoneEvent is delivered to the window that started a drag with
// [Window.StartDrag] when the drag is over: Effect is the effect the target
// performed, DropNone when the drag was cancelled, refused or dropped
// where nothing accepts it. After DropMove the source deletes its copy of
// the data (unless the target was this app and did it already). One
// follows every StartDrag that returned nil, unless the window closes or
// Run returns first.
type DragDoneEvent struct {
	Effect DropEffect
}

// MaxDragImageSide bounds the side, in pixels, of a drag image.
const MaxDragImageSide = 1024

// DragOptions configures [Window.StartDrag].
type DragOptions struct {
	// Data is what the drag carries; at least one entry (CheckData).
	Data Data
	// Allowed is the set of effects the source allows; at least one.
	Allowed DropEffect
	// Image, if not nil, is shown under the pointer during the drag:
	// premultiplied RGBA (like Present's framebuffer) in physical pixels
	// of the window's scale. nil: the OS's default feedback. It is a hint:
	// a backend that cannot show it (an X server without input shapes)
	// shows the cursor only, and X11 without a compositing manager shows
	// its pixels at least half opaque, opaque. The backend copies it
	// before StartDrag returns.
	Image *image.RGBA
	// Hotspot is the point of Image under the pointer, in Image's
	// coordinates (physical pixels; it may lie outside the image).
	Hotspot image.Point
}

// ErrNoGrab is returned by [Window.StartDrag] when the window holds no
// pointer button pressed in it (the OS starts a drag from that press:
// Wayland needs its serial), or when a drag is already in progress.
var ErrNoGrab = errors.New("platform: StartDrag needs a pointer button pressed in the window and still held, and no other drag in progress")

// CheckDropEffect panics with an actionable message if e is not DropNone
// or one effect (single) or a set of effects. what names the call.
func CheckDropEffect(what string, e DropEffect, single bool) {
	if e&^DropAll != 0 {
		panicf("platform: %s: invalid DropEffect %d; use DropNone, DropCopy, DropMove or DropLink", what, uint8(e))
	}
	if single && !e.single() {
		panicf("platform: %s: %v is a set of effects; give one effect (DefaultDropEffect picks one from the allowed set)", what, e)
	}
}

// CheckDragOptions panics with an actionable message if opts has no data,
// invalid data, no allowed effect, or an empty or oversized image.
func CheckDragOptions(opts DragOptions) {
	if len(opts.Data) == 0 {
		panic("platform: StartDrag: DragOptions.Data is empty; a drag carries at least one entry (TextData, FileData)")
	}
	CheckData("StartDrag", opts.Data)
	CheckDropEffect("StartDrag Allowed", opts.Allowed, false)
	if opts.Allowed == DropNone {
		panic("platform: StartDrag: DragOptions.Allowed is DropNone; allow at least one effect (DropCopy, DropMove, DropLink)")
	}
	if img := opts.Image; img != nil {
		b := img.Rect
		if b.Empty() {
			panicf("platform: StartDrag: DragOptions.Image is empty (%v); pass nil for the OS's default feedback", b)
		}
		if b.Dx() > MaxDragImageSide || b.Dy() > MaxDragImageSide {
			panicf("platform: StartDrag: DragOptions.Image is %dx%d; drag images must be at most %dx%d", b.Dx(), b.Dy(), MaxDragImageSide, MaxDragImageSide)
		}
	}
}

// DropTypes returns the types a backend reads at a drop: those of want
// that the source offers (in the source's order), or, when want is empty,
// the well-known types the source offers.
func DropTypes(offered, want []string) []string {
	var out []string
	for _, t := range offered {
		if len(want) == 0 {
			switch t {
			case MIMEText, MIMEHTML, MIMEPNG, MIMEURIList:
				out = append(out, t)
			}
		} else if slices.Contains(want, t) {
			out = append(out, t)
		}
	}
	return out
}

// CheckSetDropEffect panics with an actionable message if the arguments
// of Window.SetDropEffect are invalid: not one effect, or a type that is
// not a MIME type.
func CheckSetDropEffect(effect DropEffect, types []string) {
	CheckDropEffect("SetDropEffect", effect, true)
	for _, t := range types {
		CheckMIME("SetDropEffect", t)
	}
}
