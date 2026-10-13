package headless

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// keyLog records key and focus events as compact strings.
type keyLog struct{ events []string }

func (k *keyLog) HandleEvent(_ platform.Window, e platform.Event) {
	switch e := e.(type) {
	case platform.KeyEvent:
		s := fmt.Sprintf("%v %v %q", e.Action, e.Code, e.Key)
		if e.Text != "" {
			s += fmt.Sprintf(" text=%q", e.Text)
		}
		if e.Repeat {
			s += " repeat"
		}
		if e.Synthetic {
			s += " synthetic"
		}
		if e.Location != platform.KeyLocationStandard {
			s += " " + e.Location.String()
		}
		if e.Mods != 0 {
			s += fmt.Sprintf(" mods=%b", e.Mods)
		}
		s += fmt.Sprintf(" @%v", e.Time)
		k.events = append(k.events, s)
	case platform.FocusEvent:
		k.events = append(k.events, fmt.Sprintf("focus %v", e.Focused))
	}
}

func (k *keyLog) take() string {
	s := strings.Join(k.events, "; ")
	k.events = nil
	return s
}

func key(a platform.KeyAction, c platform.KeyCode, k platform.Key, text string) platform.KeyEvent {
	return platform.KeyEvent{Action: a, Code: c, Key: k, Text: text, Location: c.Location()}
}

func TestInjectKeys(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	b.Drain(&recorder{})
	log := &keyLog{}

	w.SetFocused(true)
	b.Advance(5 * time.Millisecond)
	shift := key(platform.KeyPress, platform.CodeShiftLeft, platform.KeyShift, "")
	shift.Mods = platform.ModShift
	w.Inject(shift)
	a := key(platform.KeyPress, platform.CodeKeyA, "A", "A")
	a.Mods = platform.ModShift
	w.Inject(a)
	a.Repeat, a.Time = true, 9*time.Millisecond
	w.Inject(a)
	w.Inject(key(platform.KeyRelease, platform.CodeKeyA, "A", ""))
	w.Inject(key(platform.KeyPress, platform.CodeNumpad1, "1", "1"))
	b.Drain(log)
	want := strings.Join([]string{
		"focus true",
		`press ShiftLeft "Shift" left mods=1 @5ms`,
		`press KeyA "A" text="A" mods=1 @5ms`,
		`press KeyA "A" text="A" repeat mods=1 @9ms`,
		`release KeyA "A" @5ms`,
		`press Numpad1 "1" text="1" numpad @5ms`,
	}, "; ")
	if got := log.take(); got != want {
		t.Errorf("key sequence:\n got %s\nwant %s", got, want)
	}

	// Losing focus releases the held keys (Shift, Numpad1) synthetically,
	// in press order, before the FocusEvent.
	b.Advance(time.Millisecond)
	w.SetFocused(false)
	b.Drain(log)
	want = `release ShiftLeft "Shift" synthetic left @6ms; release Numpad1 "1" synthetic numpad @6ms; focus false`
	if got := log.take(); got != want {
		t.Errorf("focus loss:\n got %s\nwant %s", got, want)
	}

	// Nothing is held any more; refocusing delivers no key events.
	w.SetFocused(true)
	w.Inject(platform.FocusEvent{Focused: false})
	b.Drain(log)
	if got := log.take(); got != "focus true; focus false" {
		t.Errorf("refocus = %q", got)
	}
}

func TestInjectKeyContract(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	b.Drain(&recorder{})
	press := key(platform.KeyPress, platform.CodeKeyQ, "q", "q")

	expectPanic(t, "keyboard focus", func() { w.Inject(press) })
	w.SetFocused(true)
	release := key(platform.KeyRelease, platform.CodeKeyQ, "q", "")
	expectPanic(t, "not held", func() { w.Inject(release) })
	rep := press
	rep.Repeat = true
	expectPanic(t, "not held", func() { w.Inject(rep) })

	w.Inject(press)
	expectPanic(t, "already held", func() { w.Inject(press) })
	bad := release
	bad.Text = "q"
	expectPanic(t, "produce no text", func() { w.Inject(bad) })
	bad = release
	bad.Repeat = true
	expectPanic(t, "never repeat", func() { w.Inject(bad) })
	bad = release
	bad.Synthetic = true
	expectPanic(t, "Synthetic", func() { w.Inject(bad) })
	bad = press
	bad.Action = 7
	expectPanic(t, "invalid Action", func() { w.Inject(bad) })
	w.Inject(release)
	log := &keyLog{}
	b.Drain(log)
	if got, want := log.take(), `focus true; press KeyQ "q" text="q" @0s; release KeyQ "q" @0s`; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}

	// Closed windows drop injected keys silently, like other events.
	w.Close()
	w.Inject(press)
	if n := b.Drain(log); n != 0 {
		t.Errorf("a closed window delivered %d events: %s", n, log.take())
	}
}
