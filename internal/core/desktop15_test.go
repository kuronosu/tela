package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Windows lists the open windows in order, WatchWindows hears each one
// open and close, and KeyWindow is the focused one.
func TestDesktopWindowsAndWatch(t *testing.T) {
	var setOpen Setter[bool]
	root := Component(func() Node {
		open, set := UseState(false)
		setOpen = set
		return div(If(open, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() { set.Set(false) }, Root: div(Text("c"))})))
	})
	x := newWinHarness(t, Options{Root: root})
	d := Desktop{x.h}
	if got := d.Windows(); len(got) != 1 {
		t.Fatalf("Windows = %d, want the main window only", len(got))
	}
	changes := 0
	stop := d.WatchWindows(func() { changes++ })
	setOpen.Set(true)
	x.drain()
	if changes != 1 || len(d.Windows()) != 2 {
		t.Fatalf("after opening: %d notices, %d windows; want 1 and 2", changes, len(d.Windows()))
	}
	if kw := d.KeyWindow(); kw != d.Windows()[0] {
		t.Errorf("KeyWindow is not the main window while no window is focused")
	}
	x.child(0).SetFocused(true)
	x.drain()
	if kw := d.KeyWindow(); kw != d.Windows()[1] {
		t.Errorf("KeyWindow is not the focused child window")
	}
	setOpen.Set(false)
	x.drain()
	if changes != 2 || len(d.Windows()) != 1 {
		t.Errorf("after closing: %d notices, %d windows; want 2 and 1", changes, len(d.Windows()))
	}
	stop()
	stop() // twice is fine
	setOpen.Set(true)
	x.drain()
	if changes != 2 {
		t.Errorf("a stopped watcher heard %d notices", changes)
	}
}

// PressKey delivers a press and a release through the window's event
// path, with the flavor's primary modifier.
func TestDesktopPressKey(t *testing.T) {
	var got []string
	root := Component(func() Node {
		return div(
			On("keydown", func(e KeyboardEvent) {
				got = append(got, "down:"+string(e.Key)+map[bool]string{true: "+cmd", false: ""}[e.CmdOrCtrl()])
			}),
			On("keyup", func(e KeyboardEvent) { got = append(got, "up:"+string(e.Key)) }),
			Text("b"))
	})
	for _, fl := range []platform.Flavor{platform.FlavorMac, platform.FlavorWindows} {
		got = nil
		x := newWinHarness(t, Options{Root: root, Config: Config{Flavor: fl}})
		d := Desktop{x.h}
		w := d.Windows()[0]
		code, _ := platform.ParseKeyCode("KeyC")
		if !d.PressKey(w, code, "c", d.PrimaryModifier(w)) {
			t.Fatal("PressKey reported the window closed")
		}
		if want := "[down:c+cmd up:c]"; sprint(got) != want {
			t.Errorf("flavor %v: events %v, want %v", fl, got, want)
		}
		wantMod := platform.ModCtrl
		if fl == platform.FlavorMac {
			wantMod = platform.ModSuper
		}
		if d.PrimaryModifier(w) != wantMod {
			t.Errorf("flavor %v: primary modifier %v, want %v", fl, d.PrimaryModifier(w), wantMod)
		}
	}
}

func sprint(v []string) string {
	s := "["
	for i, e := range v {
		if i > 0 {
			s += " "
		}
		s += e
	}
	return s + "]"
}
