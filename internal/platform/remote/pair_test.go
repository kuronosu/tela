package remote

import (
	"context"
	"errors"
	"image"
	"image/color"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

func TestFramesAndDamage(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "frames", Size: platform.Size{W: 120, H: 80}})
	ce := a.configured()
	if want := (platform.Metrics{Size: platform.Size{W: 120, H: 80}, Scale: 1}); ce.Metrics != want {
		t.Fatalf("ConfigureEvent %v, want %v", ce.Metrics, want)
	}
	a.waitEvent("FrameEvent after configure", isType[platform.FrameEvent])
	hw := p.window(0)
	if hw.Title() != "frames" {
		t.Errorf("host title %q", hw.Title())
	}

	want := a.presentNow(w, 1, nil)
	waitFrame(t, hw, "full present", want)
	if d := hw.Damage(); !slices.Equal(d, []image.Rectangle{want.Rect}) {
		t.Errorf("damage of a full present: %v", d)
	}

	// A partial present: only the damage is read, everything else keeps
	// the previous frame (the new framebuffer differs everywhere).
	r := image.Rect(10, 20, 50, 33)
	next := a.presentNow(w, 2, []image.Rectangle{r})
	platform.CopyRGBA(want, next, r)
	waitFrame(t, hw, "partial present", want)
	if d := hw.Damage(); !slices.Equal(d, []image.Rectangle{r}) {
		t.Errorf("damage %v, want %v", d, []image.Rectangle{r})
	}

	// Several presents in a row (more than the two slots) all arrive.
	for i := range 10 {
		r := image.Rect(i*10, 0, i*10+10, 80)
		next := a.presentNow(w, byte(10+i), []image.Rectangle{r})
		platform.CopyRGBA(want, next, r)
	}
	waitFrame(t, hw, "burst of partial presents", want)
	if f := p.hostFrame(hw); f == nil || !reflect.DeepEqual(f.Pix, want.Pix) {
		t.Errorf("Host.Frame differs from the presented frame")
	}
}

func (p *pair) hostFrame(hw *headless.Window) *image.RGBA {
	var f *image.RGBA
	p.ui(func() { f = p.host.Frame(hw) })
	return f
}

func TestResizeReallocates(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{Scale: 1.5})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 100, H: 60}})
	if m := a.configured().Metrics; m.Physical() != image.Pt(150, 90) {
		t.Fatalf("metrics %v", m)
	}
	hw := p.window(0)
	waitFrame(t, hw, "first frame", a.presentNow(w, 1, nil))
	gen := func() uint32 {
		var g uint32
		a.ui(func() { g = w.(*Window).gen })
		return g
	}
	if g := gen(); g != 1 {
		t.Errorf("generation %d after the first present", g)
	}

	hw.Resize(platform.Size{W: 130, H: 70})
	ce := a.waitEvent("ConfigureEvent after resize", isType[platform.ConfigureEvent]).(platform.ConfigureEvent)
	if ce.Physical() != image.Pt(195, 105) {
		t.Fatalf("resized metrics %v", ce.Metrics)
	}
	// The child presents with damage, but the first present after a
	// configure is full.
	want := a.presentNow(w, 2, []image.Rectangle{image.Rect(0, 0, 1, 1)})
	waitFrame(t, hw, "frame after resize", want)
	if g := gen(); g != 2 {
		t.Errorf("generation %d after a resize, want 2", g)
	}
	// The old segment is unmapped once the host acknowledged the new one.
	waitFor(t, "retired segment released", func() bool {
		var n int
		a.ui(func() { n = len(w.(*Window).retired) })
		return n == 0
	})

	hw.SetScale(2)
	a.waitEvent("ConfigureEvent after scale change", func(e platform.Event) bool {
		ce, ok := e.(platform.ConfigureEvent)
		return ok && ce.Scale == 2
	})
	waitFrame(t, hw, "frame at scale 2", a.presentNow(w, 3, nil))
}

// TestEveryEventForwarded injects every event of the sealed set into the
// host's window (or backend) and checks the child receives it unchanged,
// timestamps included.
func TestEveryEventForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 100, H: 100}})
	a.configured()
	hw := p.window(0)
	waitFrame(t, hw, "first frame", a.presentNow(w, 1, nil))
	a.take()

	expect := func(what string, want platform.Event) {
		t.Helper()
		got := a.waitEvent(what, func(e platform.Event) bool { return reflect.TypeOf(e) == reflect.TypeOf(want) })
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", what, got, want)
		}
	}
	ts := func(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }

	hw.SetFocused(true)
	expect("focus", platform.FocusEvent{Focused: true})
	for _, e := range []platform.PointerEvent{
		{Action: platform.PointerEnter, Pos: platform.Point{X: 5, Y: 6}, Time: ts(1)},
		{Action: platform.PointerMove, Pos: platform.Point{X: 7.5, Y: 8.25}, Mods: platform.ModShift, Time: ts(2)},
		{Action: platform.PointerDown, Pos: platform.Point{X: 7.5, Y: 8.25}, Button: platform.ButtonPrimary, Buttons: 1, Time: ts(3)},
		{Action: platform.PointerUp, Kind: platform.PointerTouch, ID: 3, Pos: platform.Point{X: 9, Y: 9}, Button: platform.ButtonPrimary, Time: ts(4)},
		{Action: platform.PointerCancel, Kind: platform.PointerPen, Time: ts(5)},
		{Action: platform.PointerLeave, Pos: platform.Point{X: -1, Y: 2}, Time: ts(6)},
	} {
		hw.Inject(e)
		expect("pointer "+e.Action.String(), e)
	}
	sc := platform.ScrollEvent{Pos: platform.Point{X: 1, Y: 2}, Delta: platform.Point{X: 0.5, Y: 3}, Steps: platform.Point{Y: 1},
		Phase: platform.ScrollPhaseUpdate, Mods: platform.ModCtrl, Time: ts(7)}
	hw.Inject(sc)
	expect("scroll", sc)
	press := platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyA, Key: "a", Text: "a", Time: ts(8)}
	hw.Inject(press)
	expect("key press", press)
	rep := press
	rep.Repeat, rep.Time = true, ts(9)
	hw.Inject(rep)
	expect("key repeat", rep)
	rel := platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyA, Key: "a", Time: ts(10)}
	hw.Inject(rel)
	expect("key release", rel)

	// IME: a session starts once the child sets a text input state.
	a.ui(func() { w.SetTextInput(&platform.TextInputState{CursorArea: platform.Rect{X: 1, Y: 2, H: 10}}) })
	expect("IME session start", platform.IMESessionEvent{Active: true})
	ime := platform.IMEEvent{Preedit: "にほ", CursorStart: 3, CursorEnd: 6}
	hw.Inject(ime)
	expect("IME preedit", ime)
	ime = platform.IMEEvent{DeleteBefore: 1, Commit: "日本"}
	hw.Inject(ime)
	expect("IME commit", ime)

	hw.Inject(platform.CloseRequestedEvent{})
	expect("close request", platform.CloseRequestedEvent{})

	s := platform.SystemSettings{DarkMode: true, DoubleClickInterval: 300 * time.Millisecond}
	p.hb.SetSettings(s)
	expect("settings", platform.SettingsEvent{Settings: s})
	if got := a.b.Settings(); got != s {
		t.Errorf("Backend.Settings() = %+v after the event, want %+v", got, s)
	}

	hw.Resize(platform.Size{W: 90, H: 70})
	expect("configure", platform.ConfigureEvent{Metrics: platform.Metrics{Size: platform.Size{W: 90, H: 70}, Scale: 1}})
	fe := a.waitEvent("frame after configure", isType[platform.FrameEvent]).(platform.FrameEvent)
	if fe.Time <= 0 || fe.Time > p.hb.Now() {
		t.Errorf("FrameEvent.Time %v is not in the host's time base (now %v)", fe.Time, p.hb.Now())
	}

	// A requested frame is the host window's.
	a.take()
	a.ui(func() { w.RequestFrame(); w.RequestFrame() })
	a.waitEvent("requested frame", isType[platform.FrameEvent])
	time.Sleep(50 * time.Millisecond)
	for _, e := range a.take() {
		if isType[platform.FrameEvent](e.e) {
			t.Errorf("two RequestFrame calls produced more than one FrameEvent")
		}
	}

	hw.SetFocused(false)
	expect("IME session end", platform.IMESessionEvent{Active: false})
	expect("focus lost", platform.FocusEvent{Focused: false})
}

func TestWindowOperations(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "one", AppID: "com.example.ops", Size: platform.Size{W: 100, H: 100}})
	a.configured()
	hw := p.window(0)
	if o := hw.Options(); o.AppID != "com.example.ops" || o.Size != (platform.Size{W: 100, H: 100}) {
		t.Errorf("host window options %+v", o)
	}
	st := &platform.TextInputState{CursorArea: platform.Rect{X: 3, Y: 4, W: 0, H: 12}, Purpose: platform.TextPurposeEmail,
		Surrounding: &platform.SurroundingText{Text: "ab", Cursor: 1, Anchor: 1}}
	a.ui(func() {
		w.SetTitle("two")
		w.SetCursor(platform.CursorText)
		w.SetTextInput(st)
		w.ResetTextInput()
		w.SetSize(platform.Size{W: 150, H: 90})
	})
	waitFor(t, "window operations", func() bool {
		return hw.Title() == "two" && hw.Cursor() == platform.CursorText && hw.TextInput().Equal(st) && hw.TextInputResets() == 1
	})
	a.waitEvent("configure after SetSize", func(e platform.Event) bool {
		ce, ok := e.(platform.ConfigureEvent)
		return ok && ce.Size == platform.Size{W: 150, H: 90}
	})
	a.ui(func() { w.SetTextInput(nil) })
	waitFor(t, "text input cleared", func() bool { return hw.TextInput() == nil })
	a.ui(func() { w.Close() })
	waitFor(t, "host window closed", hw.Closed)
}

// TestCloseRightAfterPresent closes a window whose buffers the host may
// not have acknowledged yet: they are kept until it does, then released.
func TestCloseRightAfterPresent(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	for i := range 5 {
		w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: float32(20 + i), H: 20}})
		waitFor(t, "configured", func() bool { return w.Metrics().Scale > 0 })
		a.ui(func() {
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			if err := w.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
			}
			w.Close()
		})
	}
	waitFor(t, "closed windows' buffers released", func() bool {
		var n int
		a.ui(func() {
			a.b.mu.Lock()
			n = len(a.b.closing)
			a.b.mu.Unlock()
		})
		return n == 0
	})
}

func TestClipboard(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	p.ui(func() { p.hb.Clipboard().WriteText("from host") })
	read := func() string {
		ch := make(chan string, 1)
		inside := false
		a.ui(func() {
			inside = true
			a.b.Clipboard().ReadText(func(s string, err error) {
				if inside {
					t.Errorf("ReadText called back synchronously")
				}
				if err != nil {
					t.Errorf("ReadText: %v", err)
				}
				ch <- s
			})
			inside = false
		})
		select {
		case s := <-ch:
			return s
		case <-time.After(waitTimeout):
			t.Fatal("ReadText never called back")
			return ""
		}
	}
	if s := read(); s != "from host" {
		t.Errorf("read %q", s)
	}
	a.ui(func() {
		if err := a.b.Clipboard().WriteText("from child ✓"); err != nil {
			t.Errorf("WriteText: %v", err)
		}
	})
	if s := read(); s != "from child ✓" {
		t.Errorf("read %q after WriteText", s)
	}
}

func TestClockMatchesHost(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a")
	defer a.b.Close()
	const tolerance = 2 * time.Millisecond
	for range 20 {
		h1 := p.hb.Now()
		c := a.b.Now()
		h2 := p.hb.Now()
		if c < h1-tolerance || c > h2+tolerance {
			t.Fatalf("child clock %v outside the host's [%v, %v] (± %v)", c, h1, h2, tolerance)
		}
		time.Sleep(time.Millisecond)
	}
	if a.b.HostBackend() != "headless" {
		t.Errorf("HostBackend %q", a.b.HostBackend())
	}
}

// TestSwap is tela dev's reload: child B renders offscreen while A is
// shown, input is frozen, B is promoted and shows its frame and gets the
// queued input in order; A gets nothing more.
func TestSwap(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("gen-1").start()
	wa := a.newWindow(platform.WindowOptions{Title: "A", Size: platform.Size{W: 80, H: 60}})
	ca := p.child(0)
	a.configured()
	hw := p.window(0)
	frameA := a.presentNow(wa, 1, nil)
	waitFrame(t, hw, "A's frame", frameA)
	p.ui(func() {
		if !ca.Active() || p.host.Active() != ca {
			t.Errorf("the first child is not active")
		}
	})
	if ca.Tag() != "gen-1" || ca.PID() == 0 {
		t.Errorf("child tag %q pid %d", ca.Tag(), ca.PID())
	}
	hw.SetFocused(true)
	hw.Inject(platform.PointerEvent{Action: platform.PointerEnter, Pos: platform.Point{X: 3, Y: 4}})
	held := platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeShiftLeft, Key: "Shift", Mods: platform.ModShift}
	hw.Inject(held) // held across the swap: B must not get its release
	a.waitEvent("A got the key", isType[platform.KeyEvent])

	b := p.dial("gen-2").start()
	wb := b.newWindow(platform.WindowOptions{Title: "B", Size: platform.Size{W: 999, H: 999}})
	cb := p.child(1)
	p.ui(func() {
		if !cb.Pending() {
			t.Errorf("the second child is not pending")
		}
	})
	if ce := b.configured(); ce.Metrics != (platform.Metrics{Size: platform.Size{W: 80, H: 60}, Scale: 1}) {
		t.Errorf("pending child configured with %v, want the real window's metrics", ce.Metrics)
	}
	b.waitEvent("pending child's focus", func(e platform.Event) bool { return e == platform.FocusEvent{Focused: true} })
	b.waitEvent("pending child's frame", isType[platform.FrameEvent])
	frameB := b.presentNow(wb, 2, nil)
	select {
	case <-cb.FirstFrame():
	case <-time.After(waitTimeout):
		t.Fatal("FirstFrame not closed")
	}
	time.Sleep(30 * time.Millisecond)
	if f := hw.Frame(); !reflect.DeepEqual(f.Pix, frameA.Pix) {
		t.Fatalf("a pending child's frame was shown")
	}
	if hw.Title() != "A" {
		t.Errorf("a pending child changed the title to %q", hw.Title())
	}

	// Freeze, then input arrives: queued, not delivered to A.
	p.ui(p.host.Freeze)
	a.take()
	queued := []platform.Event{
		platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 10, Y: 11}, Time: 1},
		platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: 10, Y: 11}, Button: platform.ButtonPrimary, Buttons: 1, Time: 2},
		platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeShiftLeft, Key: "Shift", Time: 3},
		platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyB, Key: "b", Text: "b", Time: 4},
		platform.ScrollEvent{Steps: platform.Point{Y: 1}, Time: 5},
		platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 10, Y: 11}, Button: platform.ButtonPrimary, Time: 6},
	}
	for _, e := range queued {
		hw.Inject(e)
	}
	time.Sleep(50 * time.Millisecond)
	for _, e := range a.take() {
		switch e.e.(type) {
		case platform.PointerEvent, platform.KeyEvent, platform.ScrollEvent:
			t.Errorf("A got %T while input was frozen", e.e)
		}
	}
	b.take()

	var err error
	p.ui(func() { err = p.host.Promote(cb) })
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	waitFrame(t, hw, "B's frame after promotion", frameB)
	if d := hw.Damage(); !slices.Equal(d, []image.Rectangle{frameB.Rect}) {
		t.Errorf("promotion present damage %v, want full", d)
	}
	if hw.Title() != "B" {
		t.Errorf("title %q after promotion", hw.Title())
	}
	p.ui(func() {
		if !cb.Active() || ca.Active() || ca.Pending() || p.host.Active() != cb {
			t.Errorf("states after promotion: A active %v pending %v, B active %v", ca.Active(), ca.Pending(), cb.Active())
		}
	})
	// B gets a synthetic enter (the pointer is over the window), then the
	// queued input in order, minus the release of a key it never saw
	// pressed.
	want := []platform.Event{queued[0], queued[1], queued[3], queued[4], queued[5]}
	waitFor(t, "B's input", func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		n := 0
		for _, e := range b.events {
			switch e.e.(type) {
			case platform.PointerEvent, platform.KeyEvent, platform.ScrollEvent:
				n++
			}
		}
		return n >= len(want)+1
	})
	var got []platform.Event
	for _, e := range b.take() {
		switch e.e.(type) {
		case platform.PointerEvent, platform.KeyEvent, platform.ScrollEvent:
			got = append(got, e.e)
		}
	}
	if enter, ok := got[0].(platform.PointerEvent); !ok || enter.Action != platform.PointerEnter || enter.Pos != (platform.Point{X: 3, Y: 4}) {
		t.Errorf("B's first input event %+v, want a pointer enter at the last position", got[0])
	}
	if !reflect.DeepEqual(got[1:], want) {
		t.Errorf("B's queued input:\n got %+v\nwant %+v", got[1:], want)
	}

	// After the swap, input goes to B only, and A's presents are ignored.
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 1, Y: 1}, Time: 9})
	b.waitEvent("input after promotion", isType[platform.PointerEvent])
	a.presentNow(wa, 3, nil)
	time.Sleep(30 * time.Millisecond)
	for _, e := range a.take() {
		if isType[platform.PointerEvent](e.e) {
			t.Errorf("the detached child got input")
		}
	}
	if f := hw.Frame(); !reflect.DeepEqual(f.Pix, frameB.Pix) {
		t.Errorf("a detached child's present was shown")
	}
	if p.hb.Windows()[0] != hw || len(p.hb.Windows()) != 1 {
		t.Errorf("the real window was recreated")
	}
}

func TestUnfreezeResumesActive(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	a.newWindow(platform.WindowOptions{})
	a.configured()
	hw := p.window(0)
	p.ui(p.host.Freeze)
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Time: 1})
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Time: 2})
	time.Sleep(30 * time.Millisecond)
	for _, e := range a.take() {
		if isType[platform.PointerEvent](e.e) {
			t.Fatalf("input delivered while frozen")
		}
	}
	p.ui(func() {
		if !p.host.Frozen() {
			t.Errorf("not frozen")
		}
		p.host.Unfreeze()
	})
	e1 := a.waitEvent("first queued event", isType[platform.PointerEvent]).(platform.PointerEvent)
	e2 := a.waitEvent("second queued event", isType[platform.PointerEvent]).(platform.PointerEvent)
	if e1.Time != 1 || e2.Time != 2 {
		t.Errorf("queued events out of order: %v, %v", e1.Time, e2.Time)
	}
}

func TestForcedScale(t *testing.T) {
	t.Run("down", func(t *testing.T) {
		// The child renders at 2, the window is at 1.
		p := newPair(t, HostOptions{Scale: 2}, headless.Options{Scale: 1})
		a := p.dial("a").start()
		w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 50, H: 30}})
		if m := a.configured().Metrics; m != (platform.Metrics{Size: platform.Size{W: 50, H: 30}, Scale: 2}) {
			t.Fatalf("child metrics %v, want the forced scale", m)
		}
		hw := p.window(0)
		src := a.presentNow(w, 1, nil)
		want := image.NewRGBA(image.Rect(0, 0, 50, 30))
		for y := range 30 {
			for x := range 50 {
				want.SetRGBA(x, y, src.RGBAAt(2*x+1, 2*y+1))
			}
		}
		waitFrame(t, hw, "downsampled frame", want)
		// Damage maps to the output pixels that sample it.
		r := image.Rect(10, 10, 21, 20)
		next := a.presentNow(w, 2, []image.Rectangle{r})
		for y := range 30 {
			for x := range 50 {
				if image.Pt(2*x+1, 2*y+1).In(r) {
					want.SetRGBA(x, y, next.RGBAAt(2*x+1, 2*y+1))
				}
			}
		}
		waitFrame(t, hw, "downsampled partial frame", want)
		if d := hw.Damage(); !slices.Equal(d, []image.Rectangle{image.Rect(5, 5, 10, 10)}) {
			t.Errorf("mapped damage %v, want [(5,5)-(10,10)]", d)
		}
	})
	t.Run("up", func(t *testing.T) {
		// The child renders at 1, the window is at 2; a scale change of
		// the window re-presents without a new child frame.
		p := newPair(t, HostOptions{Scale: 1}, headless.Options{Scale: 2})
		a := p.dial("a").start()
		w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 40, H: 20}})
		if m := a.configured().Metrics; m.Scale != 1 {
			t.Fatalf("child metrics %v", m)
		}
		hw := p.window(0)
		src := a.presentNow(w, 1, nil)
		up := func(k int) *image.RGBA {
			out := image.NewRGBA(image.Rect(0, 0, 40*k, 20*k))
			for y := range 20 * k {
				for x := range 40 * k {
					out.SetRGBA(x, y, src.RGBAAt(x/k, y/k))
				}
			}
			return out
		}
		waitFrame(t, hw, "upsampled frame", up(2))
		r := image.Rect(3, 4, 9, 7)
		next := a.presentNow(w, 2, []image.Rectangle{r})
		platform.CopyRGBA(src, next, r)
		waitFrame(t, hw, "upsampled partial frame", up(2))
		if d := hw.Damage(); !slices.Equal(d, []image.Rectangle{image.Rect(6, 8, 18, 14)}) {
			t.Errorf("mapped damage %v", d)
		}
		a.take()
		hw.SetScale(3)
		waitFrame(t, hw, "frame after the window's scale changed", up(3))
		for _, e := range a.take() {
			if isType[platform.ConfigureEvent](e.e) {
				t.Errorf("the child was reconfigured although its metrics did not change")
			}
		}
	})
}

func TestOverlay(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 30, H: 20}})
	a.configured()
	hw := p.window(0)
	frame := a.presentNow(w, 1, nil)
	waitFrame(t, hw, "frame", frame)
	red := color.RGBA{255, 0, 0, 255}
	p.ui(func() {
		p.host.SetOverlay(func(pw platform.Window, fb *image.RGBA) {
			if pw != hw {
				t.Errorf("overlay for another window")
			}
			for x := range 30 {
				fb.SetRGBA(x, 0, red)
			}
		})
	})
	want := image.NewRGBA(frame.Rect)
	copy(want.Pix, frame.Pix)
	for x := range 30 {
		want.SetRGBA(x, 0, red)
	}
	waitFrame(t, hw, "overlay", want)
	// The overlay is drawn over every new frame.
	next := a.presentNow(w, 2, []image.Rectangle{image.Rect(0, 0, 30, 5)})
	platform.CopyRGBA(want, next, image.Rect(0, 1, 30, 5))
	waitFrame(t, hw, "overlay over a new frame", want)
	p.ui(func() { p.host.SetOverlay(nil) })
	platform.CopyRGBA(want, next, image.Rect(0, 0, 30, 1))
	waitFrame(t, hw, "overlay removed", want)
}

func TestDisconnectKeepsLastFrame(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 40, H: 30}})
	c := p.child(0)
	a.configured()
	hw := p.window(0)
	frame := a.presentNow(w, 1, nil)
	waitFrame(t, hw, "frame", frame)
	a.b.Quit()
	select {
	case err := <-a.done:
		if err != nil {
			t.Errorf("child Run after Quit: %v", err)
		}
		a.done <- nil // for the cleanup
	case <-time.After(waitTimeout):
		t.Fatal("child Run did not return")
	}
	select {
	case <-c.Done():
	case <-time.After(waitTimeout):
		t.Fatal("Done not closed")
	}
	if err := c.Err(); err != nil {
		t.Errorf("Err after a clean disconnect: %v", err)
	}
	p.ui(func() {
		if p.host.Active() != nil {
			t.Errorf("a disconnected child is still active")
		}
	})
	// The window stays, with the last frame; input is now unhandled.
	if hw.Closed() {
		t.Fatalf("the real window closed")
	}
	if f := p.hostFrame(hw); f == nil || !reflect.DeepEqual(f.Pix, frame.Pix) {
		t.Errorf("last frame lost")
	}
	hw.Inject(platform.CloseRequestedEvent{})
	waitFor(t, "unhandled close request", func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return slices.ContainsFunc(p.unhandled, isType[platform.CloseRequestedEvent])
	})
	// A resize re-presents the last frame (top-left) under an overlay.
	hw.Resize(platform.Size{W: 50, H: 30})
	p.ui(func() { p.host.SetOverlay(func(platform.Window, *image.RGBA) {}) })
	want := image.NewRGBA(image.Rect(0, 0, 50, 30))
	fillBlack(want, want.Rect)
	platform.CopyRGBA(want, frame, frame.Rect)
	waitFrame(t, hw, "last frame after resize", want)

	// The next child becomes active by itself and reuses the window.
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{})
	b.configured()
	p.ui(func() { p.host.SetOverlay(nil) })
	waitFrame(t, hw, "the next child's frame", b.presentNow(wb, 2, nil))
}

func TestConnectionLoss(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{})
	c := p.child(0)
	a.configured()
	c.Close()
	select {
	case err := <-a.done:
		if err == nil {
			t.Errorf("child Run returned nil after the host closed the connection")
		}
		a.done <- err
	case <-time.After(waitTimeout):
		t.Fatal("child Run did not return after the connection was lost")
	}
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	if err := w.Present(fb, nil); err == nil {
		t.Errorf("Present succeeded without a connection")
	}
	if _, err := a.b.Request(context.Background(), "x", nil); err == nil {
		t.Errorf("Request succeeded without a connection")
	}
}

func TestHostCloseEndsChildren(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	p.child(0)
	p.host.Close()
	select {
	case err := <-a.done:
		if err == nil {
			t.Errorf("child Run returned nil after Host.Close")
		}
		a.done <- err
	case <-time.After(waitTimeout):
		t.Fatal("child Run did not return after Host.Close")
	}
	if _, err := Dial(p.host.Addr(), DialOptions{Timeout: time.Second}); err == nil {
		t.Errorf("Dial succeeded after Host.Close")
	}
}

func TestVersionMismatch(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	tr, err := dialUnix(p.host.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	e := newMsg(msgHello)
	e.b = append(e.b, magic...)
	e.u16(ProtocolVersion + 1)
	e.u32(1)
	e.str("")
	tr.write(e.bytes(), nil)
	typ, payload, err := newFrameReader(tr).next()
	if err != nil || typ != msgReject {
		t.Fatalf("got message %d (%v), want a rejection", typ, err)
	}
	d := &dec{b: payload}
	if reason := d.str(); reason == "" {
		t.Errorf("empty rejection reason")
	}
}

func TestControlChannel(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a")
	var inRun atomic.Bool
	a.b.Handle("save", func(payload []byte) ([]byte, error) {
		inRun.Store(true) // runs on the child's UI thread, inside Run
		return append([]byte("state:"), payload...), nil
	})
	a.b.Handle("fail", func([]byte) ([]byte, error) { return nil, errors.New("cannot") })
	a.start()
	c := p.child(0)
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	got, err := c.Request(ctx, "save", []byte("x"))
	if err != nil || string(got) != "state:x" || !inRun.Load() {
		t.Errorf("save: %q, %v", got, err)
	}
	big := make([]byte, 8<<20)
	for i := range big {
		big[i] = byte(i)
	}
	got, err = c.Request(ctx, "save", big)
	if err != nil || len(got) != len(big)+6 || !slices.Equal(got[6:], big) {
		t.Errorf("large payload: %d bytes, %v", len(got), err)
	}
	_, err = c.Request(ctx, "fail", nil)
	var re *RemoteError
	if !errors.As(err, &re) || re.Msg != "cannot" || re.Kind != "fail" {
		t.Errorf("failing handler: %v", err)
	}
	if _, err := c.Request(ctx, "unknown", nil); !errors.Is(err, ErrNoHandler) {
		t.Errorf("unknown kind: %v, want ErrNoHandler", err)
	}

	// Child to host.
	notes := make(chan string, 1)
	p.host.Handle("panic", func(from *Child, payload []byte) ([]byte, error) {
		if from != c {
			t.Errorf("notification from the wrong child")
		}
		notes <- string(payload)
		return nil, nil
	})
	p.host.Handle("roots", func(from *Child, payload []byte) ([]byte, error) {
		return []byte("ok " + string(payload)), nil
	})
	if err := a.b.Notify("panic", []byte("boom")); err != nil {
		t.Errorf("Notify: %v", err)
	}
	select {
	case s := <-notes:
		if s != "boom" {
			t.Errorf("notification payload %q", s)
		}
	case <-time.After(waitTimeout):
		t.Fatal("notification not handled")
	}
	got, err = a.b.Request(ctx, "roots", []byte("assets"))
	if err != nil || string(got) != "ok assets" {
		t.Errorf("child request: %q, %v", got, err)
	}
	if _, err := a.b.Request(ctx, "nothing", nil); !errors.Is(err, ErrNoHandler) {
		t.Errorf("unknown host kind: %v", err)
	}
	// A request the other side never answers ends with the context.
	a.b.Handle("slow", func([]byte) ([]byte, error) { time.Sleep(200 * time.Millisecond); return nil, nil })
	short, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if _, err := c.Request(short, "slow", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timed-out request: %v", err)
	}
}

func TestTwoWindows(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w1 := a.newWindow(platform.WindowOptions{Title: "1", Size: platform.Size{W: 20, H: 10}})
	w2 := a.newWindow(platform.WindowOptions{Title: "2", Size: platform.Size{W: 30, H: 15}})
	h1, h2 := p.window(0), p.window(1)
	waitFor(t, "both configured", func() bool { return w1.Metrics().Scale > 0 && w2.Metrics().Scale > 0 })
	waitFrame(t, h1, "window 1", a.presentNow(w1, 1, nil))
	waitFrame(t, h2, "window 2", a.presentNow(w2, 2, nil))
	h2.Inject(platform.PointerEvent{Action: platform.PointerMove, Time: 5})
	a.waitEvent("pointer on window 2", isType[platform.PointerEvent])
}
