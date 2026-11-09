package core

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

// Drag and drop (dnd.go, ADR-211) on the headless backend: its fake OS
// moves a drag with the pointer events injected into a window.

// newBackendHarness is newWinHarness with the backend's options.
func newBackendHarness(t *testing.T, bo headless.Options, opts Options) *winHarness {
	t.Helper()
	b := headless.New(bo)
	if opts.Fonts == nil {
		opts.Fonts = testFonts
	}
	h, err := NewHost(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	x := &winHarness{t: t, b: b, h: h}
	x.drain()
	return x
}

// dndLog records the drop target events of named elements as
// "kind:name" strings.
type dndLog struct{ log []string }

// target returns the four drop target handlers of an element named name,
// then more.
func (l *dndLog) target(name string, more ...Child) []Child {
	rec := func(kind string) func(DragEvent) {
		return func(DragEvent) { l.log = append(l.log, kind+":"+name) }
	}
	return append([]Child{On("dragenter", rec("enter")), On("dragover", rec("over")),
		On("dragleave", rec("leave")), On("drop", rec("drop"))}, more...)
}

func (l *dndLog) take() string {
	s := strings.Join(l.log, " ")
	l.log = nil
	return s
}

func withMods(e platform.PointerEvent, m platform.Modifiers) platform.PointerEvent {
	e.Mods = m
	return e
}

// The element under the drag gets HTML's events in HTML's order:
// dragenter on the new element, dragleave on the old one, then dragover,
// all bubbling; leaving the window is a dragleave. Every enter has its
// leave on each ancestor.
func TestDropTargetEnterLeaveOrder(t *testing.T) {
	var l dndLog
	root := div(l.target("outer", Key("outer"), Class("flex w-full h-full"),
		div(l.target("a", Key("a"), Class("w-[200px] h-full p-[20px]"),
			div(l.target("a1", Key("a1"), Class("size-[100px]"))...))...),
		div(l.target("b", Key("b"), Class("w-[200px] h-full"))...))...)
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	x.b.StartExternalDrag(platform.TextData("hi"), platform.DropCopy)
	for _, step := range []struct {
		e    platform.PointerEvent
		want string
	}{
		{move(10, 10), "enter:a enter:outer over:a over:outer"},
		{move(12, 10), "over:a over:outer"},
		{move(50, 50), "enter:a1 enter:a enter:outer leave:a leave:outer over:a1 over:a over:outer"},
		{move(300, 10), "enter:b enter:outer leave:a1 leave:a leave:outer over:b over:outer"},
		{ptr(platform.PointerLeave, 300, 10, 0, 0), "leave:b leave:outer"},
		{move(300, 20), "enter:b enter:outer over:b over:outer"},
	} {
		w.Inject(step.e)
		x.drain()
		if got := l.take(); got != step.want {
			t.Errorf("after %v at %v: events %q, want %q", step.e.Action, step.e.Pos, got, step.want)
		}
	}
	if e, _ := w.DropAnswer(); e != platform.DropNone {
		t.Errorf("answer %v without PreventDefault, want none", e)
	}
	x.b.CancelDrag()
	x.drain()
	if got := l.take(); got != "leave:b leave:outer" {
		t.Errorf("a cancel fires %q, want the leaves", got)
	}
	if len(w.Drops()) != 0 {
		t.Errorf("drops %v after a cancel", w.Drops())
	}
}

// A drop is accepted when the latest dragover called PreventDefault:
// the answer names the well-known types offered and the custom types the
// handlers looked up, the drop gets that data (nothing before), and the
// accepted effect is the outcome; without PreventDefault the release is
// a dragleave and no drop.
func TestDropAcceptsAfterDragOverPreventDefault(t *testing.T) {
	data := platform.Data{
		{Type: "application/x-card", Bytes: []byte("card-7")},
		{Type: platform.MIMEText, Bytes: []byte("hello")},
		{Type: "application/x-other", Bytes: []byte("big")},
	}
	for _, tc := range []struct {
		name               string
		accept, lookCustom bool
		wantTypes          []string
		wantDrop           string
	}{
		{"accepts the well-known types", true, false, []string{platform.MIMEText}, "drop text=hello card="},
		{"reads a custom type it looked up", true, true, []string{"application/x-card", platform.MIMEText}, "drop text=hello card=card-7"},
		{"refuses", false, false, []string{platform.MIMEText}, "leave"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			root := div(Key("zone"), Class("w-full h-full"),
				On("dragover", func(e DragEvent) {
					if e.DataTransfer.Text() != "" {
						t.Error("the data is known before the drop")
					}
					if !slices.Equal(e.DataTransfer.Types(), data.Types()) {
						t.Errorf("types %v, want %v", e.DataTransfer.Types(), data.Types())
					}
					// Looking a custom type up, even for nothing
					// before the drop, makes the drop read it.
					if tc.lookCustom && (!e.DataTransfer.Has("application/x-card") || e.DataTransfer.GetData("application/x-card") != "") {
						t.Error("Has(custom) is false, or its data is known before the drop")
					}
					if tc.accept && e.DataTransfer.Has("text/plain") {
						e.PreventDefault()
					}
				}),
				On("dragleave", func(DragEvent) { log = append(log, "leave") }),
				On("drop", func(e DragEvent) {
					log = append(log, "drop text="+e.DataTransfer.Text()+" card="+e.DataTransfer.GetData("application/x-card"))
				}))
			x := newWinHarness(t, Options{Root: root})
			w := x.windows()[0]
			x.b.StartExternalDrag(data, platform.DropCopy|platform.DropMove)
			w.Inject(move(100, 100))
			x.drain()
			eff, types := w.DropAnswer()
			wantEff := platform.DropCopy
			if !tc.accept {
				wantEff = platform.DropNone
			}
			if eff != wantEff || !slices.Equal(types, tc.wantTypes) {
				t.Errorf("answer %v %v, want %v %v", eff, types, wantEff, tc.wantTypes)
			}
			w.Inject(up(100, 100))
			x.drain()
			if got := strings.Join(log, ", "); got != tc.wantDrop {
				t.Errorf("events %q, want %q", got, tc.wantDrop)
			}
			wantDrops := []platform.DropEffect{platform.DropCopy}
			if !tc.accept {
				wantDrops = nil
			}
			if !slices.Equal(w.Drops(), wantDrops) {
				t.Errorf("drops %v, want %v", w.Drops(), wantDrops)
			}
			if _, ok := x.b.Drag(); ok {
				t.Error("the drag is still in progress")
			}
		})
	}
}

// DropEffect starts at the platform's convention for the allowed effects
// and the modifiers; a handler's choice outside the allowed set refuses,
// a set is narrowed, and the drop handler's choice is the outcome.
func TestDropEffectsAndModifiers(t *testing.T) {
	var choose DropEffect
	var seen []DropEffect
	var atDrop DropEffect
	root := div(Class("w-full h-full"),
		On("dragover", func(e DragEvent) {
			seen = append(seen, e.DataTransfer.DropEffect)
			if e.DataTransfer.EffectAllowed != platform.DropCopy|platform.DropMove {
				t.Errorf("EffectAllowed %v", e.DataTransfer.EffectAllowed)
			}
			e.PreventDefault()
			if choose != 255 {
				e.DataTransfer.DropEffect = choose
			}
		}),
		On("drop", func(e DragEvent) {
			if e.DataTransfer.DropEffect != platform.DropMove {
				t.Errorf("the drop starts at %v, want the accepted move", e.DataTransfer.DropEffect)
			}
			e.DataTransfer.DropEffect = atDrop
		}))
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	x.b.StartExternalDrag(platform.TextData("x"), platform.DropCopy|platform.DropMove)
	at := float32(10)
	for _, tc := range []struct {
		name         string
		mods         platform.Modifiers
		choose       DropEffect
		seen, answer platform.DropEffect
	}{
		{"no modifier copies", 0, 255, platform.DropCopy, platform.DropCopy},
		{"shift moves", platform.ModShift, 255, platform.DropMove, platform.DropMove},
		{"ctrl copies", platform.ModCtrl, 255, platform.DropCopy, platform.DropCopy},
		{"ctrl+shift: link not allowed", platform.ModCtrl | platform.ModShift, 255, platform.DropCopy, platform.DropCopy},
		{"a link the source refuses", 0, platform.DropLink, platform.DropCopy, platform.DropNone},
		{"a set narrowed by shift", platform.ModShift, platform.DropAll, platform.DropMove, platform.DropMove},
		{"move chosen", 0, platform.DropMove, platform.DropCopy, platform.DropMove},
	} {
		seen, choose = nil, tc.choose
		at++
		w.Inject(withMods(move(at, 10), tc.mods))
		x.drain()
		eff, _ := w.DropAnswer()
		if len(seen) != 1 || seen[0] != tc.seen || eff != tc.answer {
			t.Errorf("%s: dragover saw %v, answer %v; want [%v], %v", tc.name, seen, eff, tc.seen, tc.answer)
		}
	}
	atDrop = platform.DropNone // the target did not take the data
	w.Inject(up(at, 10))
	x.drain()
	if d := w.Drops(); !slices.Equal(d, []platform.DropEffect{platform.DropNone}) {
		t.Errorf("drops %v, want the drop handler's none", d)
	}
}

// The element under a drag is hit-tested like a pointer's: a
// pointer-events-none child lets its parent take the drag, a disabled
// element runs none of its handlers (its ancestors do), and a window
// blocked by a modal child window refuses every drag.
func TestDropTargetHitTest(t *testing.T) {
	var l dndLog
	root := div(l.target("outer", Key("outer"), Class("flex w-full h-full"),
		div(l.target("ghost", Key("ghost"), Class("w-[200px] h-full pointer-events-none"))...),
		div(l.target("off", Key("off"), Disabled(true), Class("w-[200px] h-full"))...))...)
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	x.b.StartExternalDrag(platform.TextData("x"), platform.DropCopy)
	w.Inject(move(50, 50))
	x.drain()
	if got := l.take(); got != "enter:outer over:outer" {
		t.Errorf("over a pointer-events-none child: %q, want the parent's events", got)
	}
	w.Inject(move(300, 50))
	x.drain()
	if got := l.take(); got != "enter:outer leave:outer over:outer" {
		t.Errorf("over a disabled child: %q, want only its ancestor's events", got)
	}
	x.b.CancelDrag()
	x.drain()
	l.take()

	var setModal Setter[bool]
	accepted := 0
	blocked := Component(func() Node {
		modal, set := UseState(false)
		setModal = set
		return div(Class("w-full h-full"), On("dragover", func(e DragEvent) { accepted++; e.PreventDefault() }),
			If(modal, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Modal"}, Modal: true, OnClose: func() {}, Root: div(Text("modal"))})))
	})
	y := newWinHarness(t, Options{Root: blocked})
	main := y.windows()[0]
	y.b.StartExternalDrag(platform.TextData("x"), platform.DropCopy)
	main.Inject(move(20, 20))
	y.drain()
	if e, _ := main.DropAnswer(); e != platform.DropCopy || accepted != 1 {
		t.Fatalf("without the modal: answer %v after %d dragovers", e, accepted)
	}
	setModal.Set(true)
	y.drain()
	main.Inject(move(30, 20))
	y.drain()
	if e, _ := main.DropAnswer(); e != platform.DropNone || accepted != 1 {
		t.Errorf("under a modal child window: answer %v after %d dragovers; want none and no handler", e, accepted)
	}
	main.Inject(up(30, 20))
	y.drain()
	if len(main.Drops()) != 0 {
		t.Errorf("a blocked window took a drop: %v", main.Drops())
	}
}

// The OS owns the pointer while a drag is over the window: hover ends
// when the drag enters, and comes back with the next pointer event after
// it.
func TestDragEndsHoverUntilThePointerReturns(t *testing.T) {
	root := div(Class("w-full h-full"), div(Key("card"), Class("size-[100px] bg-white hover:bg-gray-100")))
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	tr := x.h.Tree()
	hovered := func() bool { return keyed(tr, "card").istate&style.StateHover != 0 }
	w.Inject(move(50, 50))
	x.drain()
	if !hovered() {
		t.Fatal("the card is not hovered under the pointer")
	}
	x.b.StartExternalDrag(platform.TextData("x"), platform.DropCopy)
	w.Inject(move(50, 50))
	x.drain()
	if hovered() {
		t.Error("the card stays hovered while a drag is over the window")
	}
	x.b.CancelDrag()
	w.Inject(move(51, 50))
	x.drain()
	if !hovered() {
		t.Error("the card is not hovered again after the drag")
	}
}

// dndSource is a Draggable element at (10, 10), 50 px square, that drags
// "payload" and an application/x-item, and a target at (200, 10) that
// accepts an x-item as a move; their events go to log.
func dndSource(log *[]string, fill func(e DragEvent)) (src, dst Node) {
	add := func(s string) { *log = append(*log, s) }
	src = div(Key("src"), Class("absolute left-[10px] top-[10px] size-[50px] bg-red-500"), Draggable(true),
		On("dragstart", func(e DragEvent) {
			if e.DataTransfer == nil {
				add("start:nil")
				return
			}
			add("start")
			fill(e)
		}),
		On("drag", func(e DragEvent) {
			if e.DataTransfer != nil {
				add("drag:data")
				return
			}
			add("drag")
		}),
		On("dragend", func(e DragEvent) {
			if e.DataTransfer == nil {
				add("end")
				return
			}
			add("end:" + e.DataTransfer.DropEffect.String())
		}))
	dst = dndTarget(log)
	return src, dst
}

func dndTarget(log *[]string) Node {
	return div(Key("dst"), Class("absolute left-[200px] top-[10px] size-[100px]"),
		On("dragover", func(e DragEvent) {
			if e.DataTransfer.Has("application/x-item") {
				e.PreventDefault()
				e.DataTransfer.DropEffect = platform.DropMove
			}
		}),
		On("drop", func(e DragEvent) {
			*log = append(*log, "drop:"+e.DataTransfer.GetData("application/x-item")+":"+e.DataTransfer.Text())
		}))
}

func fillItem(e DragEvent) {
	e.DataTransfer.SetText("payload")
	e.DataTransfer.SetData("application/x-item", "7")
	e.DataTransfer.EffectAllowed = platform.DropCopy | platform.DropMove
}

// startDrag presses the source of dndSource and moves past the slop.
func (x *winHarness) startDrag(w *headless.Window) {
	w.Inject(move(20, 20))
	w.Inject(down(20, 20))
	x.drain()
	w.Inject(moveHeld(30, 20))
	x.drain()
}

// A Draggable element's drag fills the DataTransfer and becomes an OS
// drag through the window: the press is cancelled (no OnDrag, :active
// ends), the drag shows the element, a target in the same window gets the
// data, and the source's OnDragEnd reports the effect.
func TestDraggableToTargetInTheSameWindow(t *testing.T) {
	var log []string
	src, dst := dndSource(&log, fillItem)
	x := newWinHarness(t, Options{Root: div(Class("relative w-full h-full"), src, dst)})
	w := x.windows()[0]
	tr := x.h.Tree()
	x.startDrag(w)
	info, ok := x.b.Drag()
	if !ok {
		t.Fatalf("no OS drag after the source's drag started; events %v", log)
	}
	if info.Source != w || info.Allowed != platform.DropCopy|platform.DropMove ||
		!slices.Equal(info.Data.Types(), []string{platform.MIMEText, "application/x-item"}) {
		t.Errorf("drag %+v", info)
	}
	if info.Image == nil || info.Image.Rect.Dx() != 50 || info.Image.Rect.Dy() != 50 || info.Hotspot != image.Pt(10, 10) {
		t.Errorf("drag image %v hotspot %v; want the 50x50 element with the press at (10, 10)", info.Image.Bounds(), info.Hotspot)
	} else if c := info.Image.RGBAAt(25, 25); c.A != 255 || c.R < 200 || c.G > 100 {
		t.Errorf("drag image center %v, want the element's red", c)
	}
	if el := keyed(tr, "src"); el.istate&style.StateActive != 0 {
		t.Error("the source stays :active after the OS took the pointer")
	}
	w.Inject(moveHeld(250, 50))
	x.drain()
	w.Inject(moveHeld(260, 50)) // OnDrag would run here for the gesture
	x.drain()
	w.Inject(up(260, 50))
	x.drain()
	if got := strings.Join(log, " "); got != "start drop:7:payload end:move" {
		t.Errorf("events %q", got)
	}
	if _, ok := x.b.Drag(); ok || !slices.Equal(w.Drops(), []platform.DropEffect{platform.DropMove}) {
		t.Errorf("drag in progress %v, drops %v", ok, w.Drops())
	}
	// The next press is a new one: a click on the source works again.
	log = nil
	w.Inject(move(20, 20))
	w.Inject(down(20, 20))
	w.Inject(up(20, 20))
	x.drain()
	if len(log) != 0 {
		t.Errorf("a click after the drag fired %v", log)
	}
	if el := keyed(tr, "src"); el.istate&style.StateHover == 0 {
		t.Error("the source is not hovered again under the pointer")
	}
}

// A drag between the app's windows goes through the OS too: the drag
// leaves the source window and enters a tela.Window's.
func TestDraggableToASecondWindow(t *testing.T) {
	var log []string
	src, _ := dndSource(&log, fillItem)
	root := Component(func() Node {
		return div(Class("relative w-full h-full"), src,
			win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Target", Width: 400, Height: 200}, OnClose: func() {},
				Root: div(Class("relative w-full h-full"), dndTarget(&log))}))
	})
	x := newWinHarness(t, Options{Root: root})
	main, c := x.windows()[0], x.child(0)
	x.startDrag(main)
	c.Inject(moveHeld(250, 50))
	x.drain()
	if info, ok := x.b.Drag(); !ok || info.Over != c {
		t.Fatalf("the drag is not over the second window: %+v %v", info, ok)
	}
	c.Inject(up(250, 50))
	x.drain()
	if got := strings.Join(log, " "); got != "start drop:7:payload end:move" {
		t.Errorf("events %q", got)
	}
	if !slices.Equal(c.Drops(), []platform.DropEffect{platform.DropMove}) || len(main.Drops()) != 0 {
		t.Errorf("drops: target %v, source %v", c.Drops(), main.Drops())
	}
}

// A drag that ends over another application, or is cancelled, ends the
// source's drag with that application's effect, or none.
func TestDraggableOutsideAndCancel(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(b *headless.Backend)
		want string
	}{
		{"dropped in another application", func(b *headless.Backend) { b.DropOutside(platform.DropCopy) }, "start end:copy"},
		{"refused by another application", func(b *headless.Backend) { b.DropOutside(platform.DropNone) }, "start end:none"},
		{"cancelled", func(b *headless.Backend) { b.CancelDrag() }, "start end:none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			src, dst := dndSource(&log, fillItem)
			x := newWinHarness(t, Options{Root: div(Class("relative w-full h-full"), src, dst)})
			w := x.windows()[0]
			x.startDrag(w)
			w.Inject(ptr(platform.PointerLeave, 30, 20, 0, 0)) // the drag leaves the window
			x.drain()
			tc.end(x.b)
			x.drain()
			if got := strings.Join(log, " "); got != tc.want {
				t.Errorf("events %q, want %q", got, tc.want)
			}
		})
	}
}

// An element without Draggable keeps the drag gesture exactly; a
// Draggable one whose OnDragStart puts no data, or calls PreventDefault,
// keeps it too.
func TestDragGestureWithoutDragAndDrop(t *testing.T) {
	for _, tc := range []struct {
		name      string
		draggable bool
		fill      func(e DragEvent)
		want      string
	}{
		{"not draggable", false, nil, "start:nil drag drag end"},
		{"draggable without data", true, func(DragEvent) {}, "start drag drag end"},
		{"draggable, prevented", true, func(e DragEvent) { e.DataTransfer.SetText("x"); e.PreventDefault() }, "start drag drag end"},
		{"draggable, no effect allowed", true, func(e DragEvent) { e.DataTransfer.SetText("x"); e.DataTransfer.EffectAllowed = platform.DropNone }, "start drag drag end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			src, _ := dndSource(&log, func(e DragEvent) {
				if tc.fill != nil {
					tc.fill(e)
				}
			})
			if !tc.draggable {
				src = div(Key("src"), Class("absolute left-[10px] top-[10px] size-[50px]"),
					On("dragstart", func(e DragEvent) {
						if e.DataTransfer != nil {
							t.Error("a DataTransfer without Draggable")
						}
						log = append(log, "start:nil")
					}),
					On("drag", func(DragEvent) { log = append(log, "drag") }),
					On("dragend", func(DragEvent) { log = append(log, "end") }))
			}
			x := newWinHarness(t, Options{Root: div(Class("relative w-full h-full"), src)})
			w := x.windows()[0]
			x.startDrag(w)
			if _, ok := x.b.Drag(); ok {
				t.Error("an OS drag started")
			}
			w.Inject(moveHeld(40, 20))
			w.Inject(moveHeld(50, 20))
			w.Inject(up(50, 20))
			x.drain()
			if got := strings.Join(log, " "); got != tc.want {
				t.Errorf("events %q, want %q", got, tc.want)
			}
		})
	}
}

// When the platform cannot start the drag, the source's OnDragEnd runs at
// once with none, a dev warning says why, and the rest of the press is
// not dispatched.
func TestDraggableUnsupported(t *testing.T) {
	var log, warns []string
	src, dst := dndSource(&log, fillItem)
	x := newWinHarness(t, Options{Config: Config{Dev: true, Warn: func(m string) { warns = append(warns, m) }},
		Root: div(Class("relative w-full h-full"), src, dst)})
	x.b.SetUnsupported(headless.ServiceDragDrop)
	w := x.windows()[0]
	x.startDrag(w)
	w.Inject(moveHeld(40, 20))
	w.Inject(up(40, 20))
	x.drain()
	if got := strings.Join(log, " "); got != "start end:none" {
		t.Errorf("events %q, want the start and an end with none", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "could not start") {
		t.Errorf("warnings %q", warns)
	}
}

// TestTextDropIntoField (ADR-253): a text field takes a drag of text
// without handlers, as HTML's do: the drag is accepted, and the drop
// focuses the field, puts the caret under the drop point and inserts the
// text as one insertFromDrop edit (beforeinput, change). A drop handler's
// PreventDefault cancels the insertion, a read-only field takes nothing,
// and a drag without text is not accepted.
func TestTextDropIntoField(t *testing.T) {
	text := platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}
	files := platform.Data{{Type: platform.MIMEURIList, Bytes: []byte("file:///tmp/a.txt\r\n")}}
	for _, tc := range []struct {
		name     string
		data     platform.Data
		readOnly bool
		cancel   bool
		accept   bool
		want     string
		events   string
	}{
		{"inserts", text, false, false, true, "abXY", "beforeinput:insertFromDrop:XY change:insertFromDrop"},
		{"canceled by a drop handler", text, false, true, true, "ab", ""},
		{"read-only", text, true, false, false, "ab", ""},
		{"no text", files, false, false, false, "ab", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			root := div(Class("p-4"),
				On("drop", func(e DragEvent) {
					if tc.cancel {
						e.PreventDefault()
					}
				}),
				input(Key("in"), Class("w-60"), DefaultValue("ab"), ReadOnly(tc.readOnly),
					On("beforeinput", func(e InputEvent) { log = append(log, "beforeinput:"+e.InputType+":"+e.Data) }),
					On("change", func(e ChangeEvent) { log = append(log, "change:"+e.InputType) })))
			x := newWinHarness(t, Options{Root: root})
			w := x.windows()[0]
			x.b.StartExternalDrag(tc.data, platform.DropCopy|platform.DropMove)
			w.Inject(move(200, 26)) // right of the text: the caret goes to its end
			x.drain()
			eff, _ := w.DropAnswer()
			if (eff != platform.DropNone) != tc.accept {
				t.Fatalf("answer %v, want accepted %v", eff, tc.accept)
			}
			w.Inject(up(200, 26))
			x.drain()
			f := fieldOf(t, x.h.Tree(), "in")
			if v := f.ed.RawValue(); v != tc.want {
				t.Errorf("value %q, want %q", v, tc.want)
			}
			if got := strings.Join(log, " "); got != tc.events {
				t.Errorf("events %q, want %q", got, tc.events)
			}
			if focused := x.h.Tree().foc.el == f.el; focused != (tc.events != "") {
				t.Errorf("field focused %v after the drop", focused)
			}
		})
	}
}

// The DataTransfer's modes: setters panic outside OnDragStart, the HTML
// names of the types work, and the zero value is writable.
func TestDataTransfer(t *testing.T) {
	var d DataTransfer
	d.SetData("text", "hi")
	d.SetData("url", "https://example.com/")
	d.SetHTML("<b>hi</b>")
	if err := d.SetFiles("relative"); err == nil {
		t.Error("SetFiles with a relative path")
	}
	d.Set("application/x-n", []byte{1})
	if !slices.Equal(d.Types(), []string{platform.MIMEText, platform.MIMEURIList, platform.MIMEHTML, "application/x-n"}) {
		t.Errorf("types %v", d.Types())
	}
	if d.Text() != "hi" || d.GetData("text/plain") != "hi" || d.GetData("URL") != "https://example.com/" || !d.Has("Files") || d.HTML() != "<b>hi</b>" {
		t.Errorf("getters: text %q url %q html %q", d.Text(), d.GetData("url"), d.HTML())
	}
	d.ClearData("url")
	if d.Has("text/uri-list") || len(d.Files()) != 0 {
		t.Error("ClearData(url) kept the URI list")
	}
	var nilDT *DataTransfer
	if nilDT.Has("text") || nilDT.Text() != "" || nilDT.Types() != nil {
		t.Error("a nil DataTransfer is not empty")
	}
	for name, f := range map[string]func(){
		"nil":       func() { nilDT.SetText("x") },
		"protected": func() { (&DataTransfer{mode: dtProtected}).SetText("x") },
		"read-only": func() { (&DataTransfer{mode: dtReadOnly}).SetData("text", "x") },
		"bad type":  func() { d.Set("not a type", nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
	if got := TransferData(NewDataTransfer(platform.TextData("c"))); !slices.Equal(got.Types(), []string{platform.MIMEText}) {
		t.Errorf("round trip %v", got)
	}
}
