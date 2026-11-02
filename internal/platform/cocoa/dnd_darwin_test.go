//go:build darwin

package cocoa

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Drag and drop tests. A real drag needs the window server and a person
// holding the mouse, so the target side calls TelaView's
// NSDraggingDestination methods directly with a fake NSDraggingInfo (a
// small class registered here, answering from fakeDrag) over a named
// pasteboard the test fills, from run loop blocks outside any delivery
// pass, where AppKit calls them; one step calls them from inside the
// handler. The source side starts a real NSDraggingSession from a
// synthetic press and ends it with a synthetic release posted to the
// event queue, logging what AppKit does.

// fakeDrag is what the fake dragging info answers (one drag at a time;
// main thread only).
var fakeDrag struct {
	once   sync.Once
	class  darwin.Class
	err    error
	pb     darwin.ID      // draggingPasteboard
	loc    darwin.CGPoint // draggingLocation: window coordinates
	mask   uint64         // draggingSourceOperationMask
	source darwin.ID      // draggingSource
	window darwin.ID      // draggingDestinationWindow
	calls  map[string]int // the methods called, for the log
}

// fakeInfoClass registers TelaTestDraggingInfo once.
func fakeInfoClass() (darwin.Class, error) {
	fakeDrag.once.Do(func() {
		count := func(name string) { fakeDrag.calls[name]++ }
		fakeDrag.calls = map[string]int{}
		fakeDrag.class, fakeDrag.err = darwin.RegisterClass("TelaTestDraggingInfo", darwin.GetClass("NSObject"), nil, []darwin.Method{
			{Sel: darwin.Sel("draggingPasteboard"), Fn: func(darwin.ID, darwin.SEL) darwin.ID { count("draggingPasteboard"); return fakeDrag.pb }},
			{Sel: darwin.Sel("draggingLocation"), Fn: func(darwin.ID, darwin.SEL) darwin.CGPoint { count("draggingLocation"); return fakeDrag.loc }},
			{Sel: darwin.Sel("draggingSourceOperationMask"), Fn: func(darwin.ID, darwin.SEL) uint {
				count("draggingSourceOperationMask")
				return uint(fakeDrag.mask)
			}},
			{Sel: darwin.Sel("draggingSource"), Fn: func(darwin.ID, darwin.SEL) darwin.ID { count("draggingSource"); return fakeDrag.source }},
			{Sel: darwin.Sel("draggingDestinationWindow"), Fn: func(darwin.ID, darwin.SEL) darwin.ID {
				count("draggingDestinationWindow")
				return fakeDrag.window
			}},
			{Sel: darwin.Sel("draggingSequenceNumber"), Fn: func(darwin.ID, darwin.SEL) int { count("draggingSequenceNumber"); return 7 }},
			{Sel: darwin.Sel("draggedImage"), Fn: func(darwin.ID, darwin.SEL) darwin.ID { count("draggedImage"); return 0 }},
			{Sel: darwin.Sel("draggedImageLocation"), Fn: func(darwin.ID, darwin.SEL) darwin.CGPoint {
				count("draggedImageLocation")
				return fakeDrag.loc
			}},
			{Sel: darwin.Sel("numberOfValidItemsForDrop"), Fn: func(darwin.ID, darwin.SEL) int { count("numberOfValidItemsForDrop"); return 1 }},
			{Sel: darwin.Sel("setNumberOfValidItemsForDrop:"), Fn: func(darwin.ID, darwin.SEL, int) { count("setNumberOfValidItemsForDrop:") }},
			{Sel: darwin.Sel("animatesToDestination"), Fn: func(darwin.ID, darwin.SEL) bool { count("animatesToDestination"); return false }},
			{Sel: darwin.Sel("setAnimatesToDestination:"), Fn: func(darwin.ID, darwin.SEL, bool) { count("setAnimatesToDestination:") }},
		})
	})
	return fakeDrag.class, fakeDrag.err
}

// dndStep is one step of a drag test: run from a run loop block outside
// any delivery pass, or (inPass) from the handler's WakeEvent.
type dndStep struct {
	name   string
	inPass bool
	fn     func(r *dndRig)
}

// dndRig runs a backend with one shown window and the steps, and records
// the window's events (frames excluded) and the answers of the
// destination methods.
type dndRig struct {
	t       *testing.T
	b       *Backend
	w       *Window
	info    darwin.ID // the fake NSDraggingInfo
	events  []platform.Event
	answers []string
	onEvent func(r *dndRig, e platform.Event) // called from the handler
	done    bool                              // every step ran
	ts      float64
}

func runDnD(t *testing.T, title string, onEvent func(*dndRig, platform.Event), steps ...dndStep) *dndRig {
	t.Helper()
	b := newBackend(t)
	r := &dndRig{t: t, b: b, onEvent: onEvent}
	const pause = 200 * time.Millisecond
	onMain(func() {
		cls, err := fakeInfoClass()
		if err != nil {
			t.Errorf("registering the fake dragging info: %v", err)
			return
		}
		pw, err := b.NewWindow(platform.WindowOptions{Title: title, Size: platform.Size{W: 200, H: 150}})
		if err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		r.w = pw.(*Window)
		r.info = darwin.Send(darwin.SendClass(cls, sel.alloc), sel.init)
		defer darwin.Release(r.info)
		fakeDrag.window = r.w.ns
		timer := time.AfterFunc(20*time.Second+time.Duration(len(steps))*pause, b.Quit) // a failure, not a hang
		defer timer.Stop()
		stage, scheduled := 0, false
		run := func(s dndStep) {
			pool := darwin.PoolPush()
			defer darwin.PoolPop(pool)
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("step %q panicked: %v", s.name, p)
				}
			}()
			t.Logf("step %q", s.name)
			s.fn(r)
		}
		next := func() { time.AfterFunc(pause, b.Wake) }
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e.(type) {
			case platform.FrameEvent:
				presentSolid(t, ew, color.RGBA{0x50, 0x70, 0x90, 0xff})
				if !scheduled && r.w.shown {
					scheduled = true
					r.ts = uptime()
					next()
				}
				return
			case platform.WakeEvent:
				if stage >= len(steps) {
					r.done = true
					b.Quit()
					return
				}
				s := steps[stage]
				stage++
				if s.inPass {
					run(s)
					next()
					return
				}
				darwin.RunLoopPerformBlock(b.runLoop, darwin.CFRunLoopCommonModes, func() {
					run(s)
					next()
				})
				return
			}
			if ew == platform.Window(r.w) && stage > 0 {
				r.events = append(r.events, e)
				if r.onEvent != nil {
					r.onEvent(r, e)
				}
			}
		}))
	})
	if r.w == nil {
		return nil
	}
	if !r.done {
		t.Errorf("the steps did not all run before the timeout")
	}
	return r
}

// windowPoint returns the window coordinates (bottom-left origin) of the
// view point p.
func (r *dndRig) windowPoint(p platform.Point) darwin.CGPoint {
	return darwin.SendPoint(r.w.view, isel.convertPointToView, darwin.CGPoint{X: float64(p.X), Y: float64(p.Y)}, darwin.ID(0))
}

// drag sets the fake drag's location (view point) and mask.
func (r *dndRig) drag(p platform.Point, mask uint64) {
	fakeDrag.loc, fakeDrag.mask = r.windowPoint(p), mask
}

// call sends a destination method with the fake info and records its
// answer.
func (r *dndRig) call(name string) {
	s := darwin.Sel(name)
	switch name {
	case "draggingEntered:", "draggingUpdated:":
		op := darwin.SendUint(r.w.view, s, r.info)
		r.answers = append(r.answers, fmt.Sprintf("%s %#x", name, op))
	case "performDragOperation:":
		ok := darwin.SendBool(r.w.view, s, r.info)
		r.answers = append(r.answers, fmt.Sprintf("%s %v", name, ok))
	default:
		darwin.Send(r.w.view, s, r.info)
	}
}

// mouse builds a mouse NSEvent of the window at the view point p.
func (r *dndRig) mouse(typ int, p platform.Point, ms, clicks int) darwin.ID {
	return darwin.SendClass(app.event, tsel.mouseEvent, uint(typ), r.windowPoint(p), uint(0), r.ts+float64(ms)/1000,
		darwin.SendInt(r.w.ns, sel.windowNumber), darwin.ID(0), 0, clicks, float32(1))
}

// newDragPasteboard returns a new named pasteboard (+1, released globally
// by releaseDragPasteboard) holding items.
func newDragPasteboard(t *testing.T, items ...pbItem) darwin.ID {
	pb := darwin.Retain(darwin.SendClass(darwin.GetClass("NSPasteboard"), darwin.Sel("pasteboardWithUniqueName")))
	if pb == 0 {
		t.Errorf("+[NSPasteboard pasteboardWithUniqueName] returned nil")
		return 0
	}
	darwin.Send(pb, pbSel.clearContents)
	array := darwin.SendClass(darwin.GetClass("NSMutableArray"), pbSel.array)
	for _, it := range items {
		item, err := newPasteboardItem(it)
		if err != nil {
			t.Errorf("newPasteboardItem: %v", err)
			continue
		}
		darwin.Send(array, pbSel.addObject, item)
		darwin.Release(item)
	}
	if !darwin.SendBool(pb, pbSel.writeObjects, array) {
		t.Errorf("writeObjects: on the drag pasteboard failed")
	}
	return pb
}

func releaseDragPasteboard(pb darwin.ID) {
	if pb != 0 {
		darwin.Send(pb, darwin.Sel("releaseGlobally"))
		darwin.Release(pb)
	}
}

func dragString(e platform.DragEvent) string {
	s := fmt.Sprintf("drag %v %.0f,%.0f allowed=%v", e.Action, e.Pos.X, e.Pos.Y, e.Allowed)
	if e.FromApp {
		s += " fromApp"
	}
	if e.Action == platform.DragDrop {
		s += fmt.Sprintf(" data=%d", len(e.Data))
	}
	return s
}

// eventLines describes the drag and pointer events of r, in order.
func (r *dndRig) eventLines() []string {
	var out []string
	for _, e := range r.events {
		switch e := e.(type) {
		case platform.DragEvent:
			out = append(out, dragString(e))
		case platform.PointerEvent:
			out = append(out, "pointer "+pointerString(e))
		case platform.DragDoneEvent:
			out = append(out, fmt.Sprintf("done %v", e.Effect))
		}
	}
	return out
}

func (r *dndRig) dragEvents() []platform.DragEvent {
	var out []platform.DragEvent
	for _, e := range r.events {
		if d, ok := e.(platform.DragEvent); ok {
			out = append(out, d)
		}
	}
	return out
}

// TestDropTarget drives TelaView's NSDraggingDestination methods through
// four sessions: refused by default (with the pointer leaving first and
// no pointer events during the drag), an accepted drop of files, text and
// a custom type (no DragLeave after it), a call from inside a pass (the
// previous answer, the event delivered after), and a release whose answer
// the drop's mask no longer allows (DragLeave, no drop).
func TestDropTarget(t *testing.T) {
	const (
		moved   = darwin.NSEventTypeMouseMoved
		allMask = opCopy | opMove | opGeneric | opLink
	)
	custom := "application/x-tela"
	file1, file2 := "file:///tmp/tela%20a.txt", "file:///tmp/b.txt"
	pt := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	var registered []string
	var pbs []darwin.ID
	session := 0
	onEvent := func(r *dndRig, e platform.Event) {
		d, ok := e.(platform.DragEvent)
		if !ok {
			return
		}
		switch {
		case session == 2 && d.Action == platform.DragEnter:
			r.w.SetDropEffect(platform.DropCopy, platform.MIMEURIList, platform.MIMEText, custom)
		case session == 3 && d.Action == platform.DragEnter:
			r.w.SetDropEffect(platform.DropLink)
		case session == 3 && d.Action == platform.DragOver:
			r.w.SetDropEffect(platform.DropCopy)
		case session == 4 && d.Action == platform.DragEnter:
			r.w.SetDropEffect(platform.DropMove)
		}
	}
	steps := []dndStep{
		{name: "pointer over the window, then a refused drag", fn: func(r *dndRig) {
			registered = stringsOf(darwin.Send(r.w.view, darwin.Sel("registeredDraggedTypes")))
			r.w.ptr = pointerState{}
			darwin.Send(r.w.view, darwin.Sel("mouseMoved:"), r.mouse(moved, pt(20, 20), 0, 0))
			session = 1
			pb := newDragPasteboard(t, pbItem{{uti: utiUTF8Text, data: []byte("refused"), text: true}})
			pbs = append(pbs, pb)
			fakeDrag.pb, fakeDrag.source = pb, 0
			r.drag(pt(30, 30), opCopy)
			r.call("draggingEntered:")
			r.call("draggingUpdated:") // unchanged: no event
			// No pointer events while the drag is over the window.
			darwin.Send(r.w.view, darwin.Sel("mouseMoved:"), r.mouse(moved, pt(31, 31), 10, 0))
			r.drag(pt(40, 30), opCopy)
			r.call("draggingUpdated:")
			r.call("draggingExited:")
		}},
		{name: "an accepted drop of files, text and a custom type", fn: func(r *dndRig) {
			session = 2
			pb := newDragPasteboard(t,
				pbItem{
					{uti: utiFileURL, data: []byte(file1), text: true},
					{uti: utiUTF8Text, data: []byte("hello"), text: true},
					{uti: customType(custom), data: []byte("custom bytes")},
				},
				pbItem{{uti: utiFileURL, data: []byte(file2), text: true}},
			)
			pbs = append(pbs, pb)
			fakeDrag.pb = pb
			r.drag(pt(50, 60), allMask)
			r.call("draggingEntered:")
			r.drag(pt(55, 60), allMask)
			r.call("draggingUpdated:")
			r.call("performDragOperation:")
			// AppKit's later calls find no session: no DragLeave.
			r.call("draggingExited:")
			r.call("draggingEnded:")
		}},
		{name: "a session entered outside a pass", fn: func(r *dndRig) {
			session = 3
			r.drag(pt(10, 10), allMask)
			r.call("draggingEntered:")
		}},
		{name: "draggingUpdated: from inside a pass", inPass: true, fn: func(r *dndRig) {
			r.drag(pt(12, 10), allMask)
			r.call("draggingUpdated:") // the previous answer (link); the event waits
		}},
		{name: "the answer the waiting DragOver gave, then exit", fn: func(r *dndRig) {
			r.drag(pt(12, 10), allMask)
			r.call("draggingUpdated:") // unchanged: the handler's new answer (copy)
			r.call("draggingExited:")
		}},
		{name: "a release whose mask no longer allows the answer", fn: func(r *dndRig) {
			session = 4
			r.drag(pt(70, 70), opMove|opCopy)
			r.call("draggingEntered:")
			r.drag(pt(70, 70), opCopy) // Option held: copy only
			r.call("performDragOperation:")
			r.call("draggingEnded:")
		}},
	}
	r := runDnD(t, "tela drop target", onEvent, steps...)
	onMain(func() {
		for _, pb := range pbs {
			releaseDragPasteboard(pb)
		}
		fakeDrag.pb = 0
	})
	if r == nil {
		return
	}
	t.Logf("registered dragged types: %q", registered)
	t.Logf("fake info calls: %v", fakeDrag.calls)
	for _, want := range []string{utiFileURL, utiURL, utiUTF8Text, utiHTML, utiPNG, utiTIFF} {
		if !slices.Contains(registered, want) {
			t.Errorf("the view is not registered for %s: %q", want, registered)
		}
	}
	lines := r.eventLines()
	for _, l := range lines {
		t.Logf("got: %s", l)
	}
	for _, a := range r.answers {
		t.Logf("answer: %s", a)
	}

	wantAnswers := []string{
		// 1: refused by default.
		"draggingEntered: 0x0", "draggingUpdated: 0x0", "draggingUpdated: 0x0",
		// 2: copy, then the drop.
		"draggingEntered: 0x1", "draggingUpdated: 0x1", "performDragOperation: true",
		// 3: link; inside a pass the previous answer; then the copy the
		// waiting DragOver's handler gave.
		"draggingEntered: 0x2", "draggingUpdated: 0x2", "draggingUpdated: 0x1",
		// 4: move, then a drop whose mask lacks it.
		"draggingEntered: 0x10", "performDragOperation: false",
	}
	if strings.Join(r.answers, "\n") != strings.Join(wantAnswers, "\n") {
		t.Errorf("answers differ:\n got:\n  %s\n want:\n  %s", strings.Join(r.answers, "\n  "), strings.Join(wantAnswers, "\n  "))
	}
	wantLines := []string{
		"pointer enter 20,20",
		"pointer move 20,20",
		"pointer leave 20,20",
		"drag enter 30,30 allowed=copy",
		"drag over 40,30 allowed=copy",
		"drag leave 40,30 allowed=copy",
		"drag enter 50,60 allowed=copy|move|link",
		"drag over 55,60 allowed=copy|move|link",
		"drag drop 55,60 allowed=copy|move|link data=3",
		"drag enter 10,10 allowed=copy|move|link",
		"drag over 12,10 allowed=copy|move|link",
		"drag leave 12,10 allowed=copy|move|link",
		"drag enter 70,70 allowed=copy|move",
		"drag leave 70,70 allowed=copy",
	}
	if strings.Join(lines, "\n") != strings.Join(wantLines, "\n") {
		t.Errorf("events differ:\n got:\n  %s\n want:\n  %s", strings.Join(lines, "\n  "), strings.Join(wantLines, "\n  "))
	}

	drags := r.dragEvents()
	for _, d := range drags {
		if d.FromApp {
			t.Errorf("%s: FromApp for a drag of no Tela window", dragString(d))
		}
		if dt := d.Time - r.b.Now(); dt > time.Second || dt < -30*time.Second {
			t.Errorf("%s: time %v is far from the backend clock (now %v)", dragString(d), d.Time, r.b.Now())
		}
	}
	for _, d := range drags {
		if d.Action != platform.DragEnter {
			continue
		}
		t.Logf("types at %s: %q", dragString(d), d.Types)
		want := []string{platform.MIMEURIList, platform.MIMEText, custom}
		if d.Pos == pt(30, 30) {
			want = []string{platform.MIMEText} // the first session's pasteboard
		}
		if len(d.Types) != len(want) {
			t.Errorf("%s: Types %q, want %q in any order", dragString(d), d.Types, want)
		}
		for _, w := range want {
			if !slices.Contains(d.Types, w) {
				t.Errorf("%s: Types %q lack %s", dragString(d), d.Types, w)
			}
		}
	}
	var drop *platform.DragEvent
	for i := range drags {
		if drags[i].Action == platform.DragDrop {
			drop = &drags[i]
		}
	}
	if drop == nil {
		t.Fatalf("no DragDrop was delivered")
	}
	for _, e := range drop.Data {
		t.Logf("dropped %s: %q", e.Type, e.Bytes)
	}
	if got, _ := drop.Data.Get(platform.MIMEURIList); string(got) != string(platform.URIList([]string{file1, file2})) {
		t.Errorf("dropped URI list %q, want %q (one pasteboard item per file)", got, platform.URIList([]string{file1, file2}))
	}
	if got, _ := drop.Data.Text(); got != "hello" {
		t.Errorf("dropped text %q, want hello", got)
	}
	if got, _ := drop.Data.Get(custom); string(got) != "custom bytes" {
		t.Errorf("dropped %s %q, want the custom bytes", custom, got)
	}
}

// TestStartDrag: StartDrag without a held press is ErrNoGrab; from a
// synthetic press it starts a real NSDraggingSession, and a synthetic
// release posted to the event queue should end it. What AppKit does with
// a session the window server never saw a press for is logged; the hard
// checks are the contract's: ErrNoGrab, a nil error from the press, a
// PointerCancel before any drag event and nothing else of the sequence,
// FromApp on the window's own target events, at most one DragDoneEvent.
func TestStartDrag(t *testing.T) {
	const (
		lDown, lUp = darwin.NSEventTypeLeftMouseDown, darwin.NSEventTypeLeftMouseUp
	)
	at := platform.Point{X: 40, Y: 40}
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	opts := platform.DragOptions{
		Data:    platform.Data{{Type: platform.MIMEText, Bytes: []byte("dragged")}, {Type: "application/x-tela-drag", Bytes: []byte("x")}},
		Allowed: platform.DropCopy | platform.DropMove,
		Image:   img,
		Hotspot: image.Pt(8, 6),
	}
	var noGrab, startErr, secondErr error
	started, posted := false, 0
	var doneAt time.Duration
	onEvent := func(r *dndRig, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			if e.Action == platform.PointerDown && !started {
				started = true
				startErr = r.w.StartDrag(opts)
				secondErr = r.w.StartDrag(opts)
				t.Logf("StartDrag from the PointerDown: %v; a second one: %v; session drag %v", startErr, secondErr, r.b.dnd.drag != nil)
			}
		case platform.DragEvent:
			// The app's own window accepts its drag.
			r.w.SetDropEffect(platform.DropCopy, platform.MIMEText)
		case platform.DragDoneEvent:
			doneAt = r.b.Now()
		}
	}
	release := func(r *dndRig) {
		if doneAt != 0 || r.b.dnd.drag == nil {
			return
		}
		up := r.mouse(lUp, at, 600+100*posted, 1)
		posted++
		darwin.Send(app.ns, sel.postEventAtStart, up, false)
		t.Logf("posted a synthetic left mouse up (%d)", posted)
	}
	watchdog := time.AfterFunc(75*time.Second, func() {
		panic("TestStartDrag: Run has not returned 75 s after it started; the NSDraggingSession probably holds the main thread in its tracking loop, and the synthetic mouse up did not end it")
	})
	defer watchdog.Stop()
	steps := []dndStep{
		{name: "StartDrag without a press", inPass: true, fn: func(r *dndRig) {
			noGrab = r.w.StartDrag(opts)
		}},
		{name: "a synthetic press starts the drag", fn: func(r *dndRig) {
			r.w.ptr = pointerState{}
			darwin.Send(app.ns, isel.sendEvent, r.mouse(lDown, at, 0, 1))
			t.Logf("after the press: drag in progress %v", r.b.dnd.drag != nil)
			release(r)
		}},
		{name: "wait", fn: release},
		{name: "wait", fn: release},
		{name: "wait", fn: func(r *dndRig) {}},
		{name: "wait", fn: release},
		{name: "wait", fn: func(r *dndRig) {}},
		{name: "wait", fn: func(r *dndRig) {}},
	}
	r := runDnD(t, "tela drag source", onEvent, steps...)
	if r == nil {
		return
	}
	lines := r.eventLines()
	for _, l := range lines {
		t.Logf("got: %s", l)
	}
	t.Logf("drag still in progress after the run: %v; DragDoneEvent at %v", r.b.dnd.drag != nil, doneAt)
	if !errors.Is(noGrab, platform.ErrNoGrab) {
		t.Errorf("StartDrag without a held press = %v, want ErrNoGrab", noGrab)
	}
	if !started {
		t.Fatalf("the synthetic press delivered no PointerDown")
	}
	if startErr != nil {
		t.Fatalf("StartDrag from the PointerDown of a held press = %v, want nil", startErr)
	}
	if !errors.Is(secondErr, platform.ErrNoGrab) {
		t.Errorf("a second StartDrag during the drag = %v, want ErrNoGrab", secondErr)
	}
	// The sequence: the press, its cancel, then only drag events (and the
	// pointer coming back after the drag ended).
	down, cancel, firstDrag, done := -1, -1, -1, -1
	dones := 0
	for i, e := range r.events {
		switch e := e.(type) {
		case platform.PointerEvent:
			switch {
			case e.Action == platform.PointerDown && down < 0:
				down = i
			case e.Action == platform.PointerCancel && cancel < 0:
				cancel = i
			case down >= 0 && done < 0 && e.Action != platform.PointerLeave && e.Action != platform.PointerCancel:
				t.Errorf("event %d: %s during the drag; the sequence ends at its PointerCancel", i, pointerString(e))
			}
		case platform.DragEvent:
			if firstDrag < 0 {
				firstDrag = i
			}
			if !e.FromApp {
				t.Errorf("%s over the source window without FromApp", dragString(e))
			}
		case platform.DragDoneEvent:
			dones++
			if done < 0 {
				done = i
			}
		}
	}
	if cancel < 0 || cancel < down {
		t.Errorf("no PointerCancel after the PointerDown the drag started from")
	}
	if firstDrag >= 0 && firstDrag < cancel {
		t.Errorf("a drag event (%d) came before the PointerCancel (%d)", firstDrag, cancel)
	}
	if dones > 1 {
		t.Errorf("%d DragDoneEvents, want one", dones)
	}
	if done < 0 {
		t.Logf("note: no DragDoneEvent: AppKit did not end the session from the synthetic release within the run (the window server saw no press)")
	}
}
