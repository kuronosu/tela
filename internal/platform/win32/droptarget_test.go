package win32

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

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

func actions(events []platform.DragEvent) []platform.DragAction {
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
	// OLE's timer calls DragOver with nothing changed: no event.
	for range 3 {
		if got := s.over(h, platform.DropCopy|platform.DropMove, 0, at); got != platform.DropCopy {
			t.Errorf("unchanged over = %v", got)
		}
	}
	if len(h.got) != 1 {
		t.Errorf("unchanged DragOver calls delivered %d events", len(h.got)-1)
	}
	// Shift picks move.
	if got := s.over(h, platform.DropCopy|platform.DropMove, platform.ModShift, at); got != platform.DropMove {
		t.Errorf("over with Shift = %v, want move", got)
	}
	// The source stops allowing move: the answer counts as none.
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
	if want := []platform.DragAction{platform.DragEnter, platform.DragOver, platform.DragOver, platform.DragOver, platform.DragDrop}; !slices.Equal(actions(h.got), want) {
		t.Errorf("events = %v, want %v", actions(h.got), want)
	}
	if s.active {
		t.Errorf("the session is still active after the drop")
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
	if want := []platform.DragAction{platform.DragEnter, platform.DragLeave}; !slices.Equal(actions(h.got), want) {
		t.Errorf("events = %v, want %v", actions(h.got), want)
	}
	// Leave without a session, and a second leave, do nothing.
	s.leave(h)
	if len(h.got) != 2 {
		t.Errorf("leave without a session delivered an event")
	}
}

func TestDropSessionBusyPass(t *testing.T) {
	// A pass is running when OLE calls (something in the handler ran a
	// message loop): the event waits and the previous answer stands.
	var s dropSession
	h := &fakeDropHost{}
	h.handle = func(platform.DragEvent) { s.setAnswer(platform.DropLink, nil) }
	s.enter(h, []string{platform.MIMEText}, platform.DropAll, 0, platform.Point{}, false)
	h.busy = true
	if got := s.over(h, platform.DropAll, platform.ModCtrl, platform.Point{X: 1}); got != platform.DropLink {
		t.Errorf("over during a pass = %v, want the previous answer", got)
	}
	if len(h.pending) != 1 || h.pending[0].Action != platform.DragOver {
		t.Errorf("pending = %v", actions(h.pending))
	}
	// A second enter ends the first session first.
	h.busy = false
	s.enter(h, []string{platform.MIMEText}, platform.DropCopy, 0, platform.Point{}, false)
	if want := []platform.DragAction{platform.DragEnter, platform.DragLeave, platform.DragEnter}; !slices.Equal(actions(h.got), want) {
		t.Errorf("events = %v, want %v", actions(h.got), want)
	}
}
