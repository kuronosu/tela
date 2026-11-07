package cocoa

import (
	"fmt"
	"image"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

func TestDropEffectsOfMask(t *testing.T) {
	const every = ^uint64(0)
	for _, c := range []struct {
		mask uint64
		want platform.DropEffect
	}{
		{0, platform.DropNone},
		{opCopy, platform.DropCopy},
		{opLink, platform.DropLink},
		{opGeneric, platform.DropMove}, // generic counts as move, as AppKit does
		{opMove, platform.DropMove},
		{opMove | opGeneric, platform.DropMove},
		{opPrivate | opDelete, platform.DropNone},
		{opCopy | opMove | opLink, platform.DropAll},
		{every, platform.DropAll},
	} {
		if got := dropEffects(c.mask); got != c.want {
			t.Errorf("dropEffects(%#x) = %v, want %v", c.mask, got, c.want)
		}
	}
}

func TestTargetOperation(t *testing.T) {
	for _, c := range []struct {
		e    platform.DropEffect
		mask uint64
		want uint64
	}{
		{platform.DropNone, ^uint64(0), 0},
		{platform.DropCopy, opCopy | opMove, opCopy},
		{platform.DropCopy, opMove, 0},
		{platform.DropLink, opLink | opCopy, opLink},
		{platform.DropMove, opMove | opGeneric, opMove},
		{platform.DropMove, opGeneric, opGeneric}, // the mask's own bit
		{platform.DropMove, opCopy, 0},
		{platform.DropMove, ^uint64(0), opMove},
	} {
		if got := targetOperation(c.e, c.mask); got != c.want {
			t.Errorf("targetOperation(%v, %#x) = %#x, want %#x", c.e, c.mask, got, c.want)
		}
	}
}

func TestSourceMaskAndDoneEffect(t *testing.T) {
	if got := sourceMask(platform.DropCopy); got != opCopy {
		t.Errorf("sourceMask(copy) = %#x", got)
	}
	if got := sourceMask(platform.DropMove); got != opMove|opGeneric {
		t.Errorf("sourceMask(move) = %#x, want move|generic", got)
	}
	if got := sourceMask(platform.DropAll); got != opCopy|opMove|opGeneric|opLink {
		t.Errorf("sourceMask(all) = %#x", got)
	}
	// Every allowed set round-trips through AppKit's mask.
	for e := platform.DropEffect(1); e <= platform.DropAll; e++ {
		if got := dropEffects(sourceMask(e)); got != e {
			t.Errorf("dropEffects(sourceMask(%v)) = %v", e, got)
		}
	}
	for _, c := range []struct {
		op      uint64
		allowed platform.DropEffect
		want    platform.DropEffect
	}{
		{0, platform.DropAll, platform.DropNone},
		{opCopy, platform.DropAll, platform.DropCopy},
		{opMove, platform.DropAll, platform.DropMove},
		{opGeneric, platform.DropMove, platform.DropMove},
		{opDelete, platform.DropMove, platform.DropMove}, // the Trash
		{opLink, platform.DropLink | platform.DropCopy, platform.DropLink},
		{opCopy, platform.DropMove, platform.DropNone}, // not allowed
		{opPrivate, platform.DropAll, platform.DropNone},
	} {
		if got := doneEffect(c.op, c.allowed); got != c.want {
			t.Errorf("doneEffect(%#x, %v) = %v, want %v", c.op, c.allowed, got, c.want)
		}
	}
}

func TestDraggedTypes(t *testing.T) {
	got := draggedTypes([]string{"com.apple.NSFilePromiseItemMetaData", "promise.extra"}, []string{"dev.telaui.mime.application-2fx-a", utiPNG})
	for _, want := range []string{utiUTF8Text, "NSStringPboardType", utiHTML, utiPNG, utiTIFF, utiFileURL, utiURL, "dev.telaui.mime.application-2fx-a"} {
		if !slices.Contains(got, want) {
			t.Errorf("draggedTypes lacks %s: %q", want, got)
		}
	}
	seen := map[string]bool{}
	for _, t2 := range got {
		if seen[t2] {
			t.Errorf("draggedTypes lists %s twice: %q", t2, got)
		}
		seen[t2] = true
	}
	if got[len(got)-1] != "dev.telaui.mime.application-2fx-a" {
		t.Errorf("the extra types come last: %q", got)
	}
}

func TestCustomDragTypes(t *testing.T) {
	d := platform.Data{
		{Type: platform.MIMEText, Bytes: []byte("t")},
		{Type: "application/json", Bytes: []byte("{}")},
		{Type: platform.MIMEURIList, Bytes: []byte("file:///a\r\n")},
		{Type: "application/x-a;v=1", Bytes: []byte("a")},
		{Type: platform.MIMEPNG, Bytes: []byte{1}},
	}
	got := customDragTypes(d, func(m string) string {
		if m == "application/json" {
			return "public.json"
		}
		return customType(m)
	})
	want := []string{"public.json", customType("application/x-a;v=1")}
	if !slices.Equal(got, want) {
		t.Errorf("customDragTypes = %q, want %q", got, want)
	}
}

func TestImageFrame(t *testing.T) {
	// A 40x20 image at scale 2 is 20x10 points; its hotspot (10, 4) px is
	// (5, 2) points under the pointer.
	f := imageFrame(platform.Point{X: 100, Y: 50}, image.Pt(40, 20), image.Pt(10, 4), 2)
	if want := (dragFrame{X: 95, Y: 48, W: 20, H: 10}); f != want {
		t.Errorf("imageFrame = %+v, want %+v", f, want)
	}
	// A hotspot outside the image, and a bad scale (taken as 1).
	f = imageFrame(platform.Point{X: 10, Y: 10}, image.Pt(8, 8), image.Pt(-4, 12), 0)
	if want := (dragFrame{X: 14, Y: -2, W: 8, H: 8}); f != want {
		t.Errorf("imageFrame = %+v, want %+v", f, want)
	}
}

func TestPointerCancel(t *testing.T) {
	var got []platform.PointerEvent
	emit := func(e platform.PointerEvent) { got = append(got, e) }
	p := pointerState{}
	p.button(platform.ButtonPrimary, true, platform.Point{X: 5, Y: 5}, emit)
	p.move(platform.Point{X: 300, Y: 5}, false, emit) // out while held: the leave waits
	got = nil
	p.cancel(emit)
	if len(got) != 2 || got[0].Action != platform.PointerCancel || got[1].Action != platform.PointerLeave {
		t.Fatalf("cancel = %+v, want a cancel and the leave that waited", got)
	}
	if p.held != 0 || p.inside {
		t.Errorf("after cancel: held %v inside %v", p.held, p.inside)
	}
	// The release of the cancelled press is dropped; a second cancel is
	// nothing.
	got = nil
	p.button(platform.ButtonPrimary, false, platform.Point{X: 5, Y: 5}, emit)
	p.cancel(emit)
	if len(got) != 0 {
		t.Errorf("after the cancel: %+v, want nothing", got)
	}
}

// fakeDropHost delivers drag events to a handler at once, as the
// backend's synchronous pass does, unless busy (a pass is running): then
// they wait in pending.
type fakeDropHost struct {
	handle  func(platform.DragEvent)
	busy    bool
	got     []platform.DragEvent
	pending []platform.DragEvent
}

func (h *fakeDropHost) deliverDrag(e platform.DragEvent) bool {
	if h.busy {
		h.pending = append(h.pending, e)
		return false
	}
	h.got = append(h.got, e)
	if h.handle != nil {
		h.handle(e)
	}
	return true
}

func (h *fakeDropHost) now() time.Duration { return time.Second }

func dragActions(events []platform.DragEvent) []platform.DragAction {
	var out []platform.DragAction
	for _, e := range events {
		out = append(out, e.Action)
	}
	return out
}

func TestDropSessionAccepts(t *testing.T) {
	var s dropSession
	types := []string{platform.MIMEHTML, platform.MIMEText, "application/x-a"}
	h := &fakeDropHost{}
	h.handle = func(e platform.DragEvent) {
		switch e.Action {
		case platform.DragEnter, platform.DragOver:
			if e.Mods&platform.ModShift != 0 {
				s.setAnswer(platform.DropMove, []string{platform.MIMEText})
			} else {
				s.setAnswer(platform.DropCopy, []string{"application/x-a", platform.MIMEText})
			}
		}
	}
	at := platform.Point{X: 10, Y: 20}
	if got := s.enter(h, types, platform.DropCopy|platform.DropMove, 0, at, true); got != platform.DropCopy {
		t.Fatalf("enter = %v, want copy", got)
	}
	e := h.got[0]
	if e.Action != platform.DragEnter || e.Pos != at || !slices.Equal(e.Types, types) || e.Allowed != platform.DropCopy|platform.DropMove || !e.FromApp || e.Time != time.Second {
		t.Errorf("enter event = %+v", e)
	}
	// draggingUpdated: with nothing changed: no event.
	for range 3 {
		if got := s.over(h, platform.DropCopy|platform.DropMove, 0, at); got != platform.DropCopy {
			t.Errorf("unchanged over = %v", got)
		}
	}
	if len(h.got) != 1 {
		t.Errorf("unchanged draggingUpdated: calls delivered %d events", len(h.got)-1)
	}
	if got := s.over(h, platform.DropCopy|platform.DropMove, platform.ModShift, at); got != platform.DropMove {
		t.Errorf("over with Shift = %v, want move", got)
	}
	// The source stops allowing move (the user let go of a modifier that
	// filtered the mask): the answer counts as none.
	if got := s.over(h, platform.DropCopy, platform.ModShift, at); got != platform.DropNone {
		t.Errorf("over with move not allowed = %v, want none", got)
	}
	s.over(h, platform.DropCopy|platform.DropMove, 0, platform.Point{X: 11, Y: 20})
	var asked []string
	got := s.drop(h, platform.DropCopy|platform.DropMove, 0, platform.Point{X: 12, Y: 21}, func(t []string) platform.Data {
		asked = t
		return platform.Data{{Type: "application/x-a", Bytes: []byte("a")}}
	})
	if got != platform.DropCopy {
		t.Errorf("drop = %v, want copy", got)
	}
	if !slices.Equal(asked, []string{platform.MIMEText, "application/x-a"}) {
		t.Errorf("drop read %q, want the wanted types in the source's order", asked)
	}
	last := h.got[len(h.got)-1]
	if last.Action != platform.DragDrop || len(last.Data) != 1 || last.Pos != (platform.Point{X: 12, Y: 21}) {
		t.Errorf("drop event = %+v", last)
	}
	if want := []platform.DragAction{platform.DragEnter, platform.DragOver, platform.DragOver, platform.DragOver, platform.DragDrop}; !slices.Equal(dragActions(h.got), want) {
		t.Errorf("events = %v, want %v", dragActions(h.got), want)
	}
	if s.active {
		t.Errorf("the session is still active after the drop")
	}
	// draggingExited: or draggingEnded: after the drop: no DragLeave.
	s.leave(h)
	if n := len(h.got); h.got[n-1].Action != platform.DragDrop {
		t.Errorf("a leave after the drop delivered %v", h.got[n-1].Action)
	}
	// Answers outside a session do nothing.
	s.setAnswer(platform.DropCopy, nil)
	if s.effect != platform.DropNone || s.over(h, platform.DropCopy, 0, at) != platform.DropNone {
		t.Errorf("an answer without a session was kept")
	}
}

func TestDropSessionDefaultTypesAndFinalEffect(t *testing.T) {
	var s dropSession
	h := &fakeDropHost{}
	h.handle = func(e platform.DragEvent) {
		switch e.Action {
		case platform.DragEnter:
			s.setAnswer(platform.DropMove, nil) // no types: the well-known ones
		case platform.DragDrop:
			s.setAnswer(platform.DropNone, nil) // the app did not take the data
		}
	}
	types := []string{"application/x-a", platform.MIMEPNG, platform.MIMEText}
	s.enter(h, types, platform.DropAll, 0, platform.Point{}, false)
	var asked []string
	got := s.drop(h, platform.DropAll, 0, platform.Point{}, func(t []string) platform.Data { asked = t; return nil })
	if got != platform.DropNone {
		t.Errorf("drop after SetDropEffect(DropNone) in the handler = %v, want none", got)
	}
	if !slices.Equal(asked, []string{platform.MIMEPNG, platform.MIMEText}) {
		t.Errorf("drop read %q, want the well-known types offered", asked)
	}
}

func TestDropSessionRefusedDropIsLeave(t *testing.T) {
	var s dropSession
	h := &fakeDropHost{}
	s.enter(h, []string{platform.MIMEText}, platform.DropCopy, 0, platform.Point{}, false)
	read := false
	if got := s.drop(h, platform.DropCopy, 0, platform.Point{}, func([]string) platform.Data { read = true; return nil }); got != platform.DropNone {
		t.Errorf("refused drop = %v", got)
	}
	if read {
		t.Errorf("a refused drop read the data")
	}
	if want := []platform.DragAction{platform.DragEnter, platform.DragLeave}; !slices.Equal(dragActions(h.got), want) {
		t.Errorf("events = %v, want %v", dragActions(h.got), want)
	}
	s.leave(h)
	if len(h.got) != 2 {
		t.Errorf("leave without a session delivered an event")
	}
}

func TestDropSessionBusyPass(t *testing.T) {
	// A destination method called during a pass (from the handler): the
	// event waits and the previous answer stands.
	var s dropSession
	h := &fakeDropHost{}
	h.handle = func(platform.DragEvent) { s.setAnswer(platform.DropLink, nil) }
	s.enter(h, []string{platform.MIMEText}, platform.DropAll, 0, platform.Point{}, false)
	h.busy = true
	if got := s.over(h, platform.DropAll, platform.ModCtrl, platform.Point{X: 1}); got != platform.DropLink {
		t.Errorf("over during a pass = %v, want the previous answer", got)
	}
	if len(h.pending) != 1 || h.pending[0].Action != platform.DragOver {
		t.Errorf("pending = %v", dragActions(h.pending))
	}
	// A second enter ends the first session first.
	h.busy = false
	s.enter(h, []string{platform.MIMEText}, platform.DropCopy, 0, platform.Point{}, false)
	if want := []platform.DragAction{platform.DragEnter, platform.DragLeave, platform.DragEnter}; !slices.Equal(dragActions(h.got), want) {
		t.Errorf("events = %v, want %v", dragActions(h.got), want)
	}
}

func TestDragWatch(t *testing.T) {
	var w dragWatch
	t0 := time.Unix(100, 0)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	var looks []int
	for ms := 0; ms <= 200; ms += 8 { // a 125 Hz text selection: no drag
		if w.dragged(at(ms)) {
			looks = append(looks, ms)
		}
	}
	if fmt.Sprint(looks) != "[0 56 112 168]" {
		t.Errorf("looks without a drag = %v, want the first event and one per %v", looks, dragLookGap)
	}
	w.up()
	if !w.dragged(at(300)) {
		t.Error("a new sequence's first event was not looked at")
	}
	w.changed()
	if w.dragged(at(400)) || w.dragged(at(1000)) {
		t.Error("looked again after the sequence showed its drag")
	}
	w.up()
	if !w.dragged(at(1001)) {
		t.Error("no look after the mouse went up and a new sequence started")
	}
}
