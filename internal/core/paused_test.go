package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Windows without frames (paused.go, ADR-185).

// expectFullRender fails unless window w shows exactly a full rendering
// of the tree's current frame: no stale pixels, no stale damage.
func expectFullRender(t *testing.T, w *headless.Window, tr *Tree) {
	t.Helper()
	if tr.Dirty() {
		t.Fatal("the tree still has work: the window shows an older state")
	}
	got := w.Frame()
	want := tr.Rasterize(tr.ReferenceFrame(false), tr.Scale())
	if got == nil || got.Rect != want.Rect {
		t.Fatalf("the window holds %v, a full rendering %v", got, want.Rect)
	}
	for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
		for x := got.Rect.Min.X; x < got.Rect.Max.X; x++ {
			if g, e := got.RGBAAt(x, y), want.RGBAAt(x, y); g != e {
				t.Fatalf("pixel (%d, %d) is %v in the window, %v in a full rendering", x, y, g, e)
			}
		}
	}
}

// counterApp is a window content that shows n in its text and color and
// records its effects and layout effects.
type counterApp struct {
	w                      WindowHandle
	setN                   Setter[int]
	builds                 int
	effects, layoutEffects []int
}

func (c *counterApp) root() Node {
	return Component(func() Node {
		c.builds++
		c.w = UseWindow()
		n, set := UseState(0)
		c.setN = set
		UseEffect(func() func() { c.effects = append(c.effects, n); return nil }, n)
		UseLayoutEffect(func() func() { c.layoutEffects = append(c.layoutEffects, n); return nil }, n)
		bg := "bg-red-500"
		if n%2 == 1 {
			bg = "bg-blue-500"
		}
		return div(Class("w-40 h-20 "+bg), Textf("n=%d", n))
	})
}

// A hidden window applies its state updates, builds, lays out and runs
// its effects at once, without presenting; shown again, it presents the
// current state in full.
func TestHiddenWindowCommits(t *testing.T) {
	var c counterApp
	x := newWinHarness(t, Options{Window: WindowSpec{Title: "Main"}, Root: c.root()})
	main := x.windows()[0]
	c.w.Hide()
	x.drain()
	if main.Visible() || !c.w.State().Hidden {
		t.Fatal("Hide did not hide the window")
	}
	presents, frames := main.Presents(), x.h.Presents()

	c.setN.Set(1)
	x.b.Drain(x.h) // no clock moves: the commit runs after the update's wake
	if s := x.snapshot(main); !strings.Contains(s, "n=1") {
		t.Errorf("a hidden window did not build its state update: %q", s)
	}
	if !slices.Equal(c.effects, []int{0, 1}) || !slices.Equal(c.layoutEffects, []int{0, 1}) {
		t.Errorf("effects %v, layout effects %v; want [0 1] for both", c.effects, c.layoutEffects)
	}
	c.setN.Set(2)
	x.drain()
	if !slices.Equal(c.effects, []int{0, 1, 2}) {
		t.Errorf("effects %v after a second update, want [0 1 2]", c.effects)
	}
	if main.Presents() != presents || x.h.Presents() != frames {
		t.Errorf("a hidden window presented: %d presents, want %d", main.Presents(), presents)
	}
	if _, ok := x.h.PausedWork(); ok {
		t.Error("PausedWork reports work after the commits")
	}

	c.setN.Set(3) // blue again: the pixels differ from the last presented frame
	x.drain()
	c.w.Show()
	x.drain()
	if !main.Visible() || main.Presents() == presents {
		t.Fatalf("shown again: visible %v, %d presents (was %d)", main.Visible(), main.Presents(), presents)
	}
	if s := x.snapshot(main); !strings.Contains(s, "n=3") {
		t.Errorf("shown again, the window shows %q", s)
	}
	expectFullRender(t, main, x.h.Tree())

	// Damage tracking goes on from the full frame.
	c.setN.Set(4)
	x.drain()
	expectFullRender(t, main, x.h.Tree())
}

// A minimized window commits like a hidden one; restored, it presents
// the current state.
func TestMinimizedWindowCommits(t *testing.T) {
	var c counterApp
	x := newWinHarness(t, Options{Window: WindowSpec{Title: "Main"}, Root: c.root()})
	main := x.windows()[0]
	c.w.Minimize()
	x.drain()
	if !main.OSState().Minimized || !c.w.State().Minimized {
		t.Fatal("Minimize did not minimize the window")
	}
	presents := main.Presents()
	c.setN.Set(1)
	x.drain()
	if s := x.snapshot(main); !strings.Contains(s, "n=1") || !slices.Equal(c.effects, []int{0, 1}) {
		t.Errorf("a minimized window: snapshot %q, effects %v", s, c.effects)
	}
	if main.Presents() != presents {
		t.Errorf("a minimized window presented (%d presents, want %d)", main.Presents(), presents)
	}
	c.w.Focus() // the fake window manager restores it
	x.drain()
	if main.OSState().Minimized || main.Presents() == presents {
		t.Fatalf("restored: minimized %v, %d presents", main.OSState().Minimized, main.Presents())
	}
	expectFullRender(t, main, x.h.Tree())
}

// A window whose parent is minimized counts as one without frames
// (Windows hides owned windows with their owner).
func TestChildOfMinimizedWindowIsPaused(t *testing.T) {
	var w WindowHandle
	root := Component(func() Node {
		w = UseWindow()
		return div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {}, Root: div(Text("child"))}))
	})
	x := newWinHarness(t, Options{Root: root})
	cv := x.h.viewOf(x.child(0))
	if cv.paused() {
		t.Fatal("a visible child of a visible window is paused")
	}
	w.Minimize()
	x.drain()
	if !cv.paused() {
		t.Error("a child of a minimized window is not paused")
	}
	w.Focus()
	x.drain()
	if cv.paused() {
		t.Error("a child of a restored window is still paused")
	}
}

// A hidden window opens the Windows its builds declare (a tray menu's
// Preferences while the main window is hidden), and closes them.
func TestHiddenWindowOpensChildWindow(t *testing.T) {
	var w WindowHandle
	var setOpen Setter[bool]
	mounts, cleanups := 0, 0
	content := Component(func() Node {
		UseMount(func() func() {
			mounts++
			return func() { cleanups++ }
		})
		return div(Text("preferences"))
	})
	root := Component(func() Node {
		w = UseWindow()
		open, set := UseState(false)
		setOpen = set
		return div(Text("main"), If(open, win(ChildWindowSpec{
			WindowSpec: WindowSpec{Title: "Prefs", Width: 200, Height: 100},
			OnClose:    func() { set.Set(false) }, Root: content,
		})))
	})
	x := newWinHarness(t, Options{Window: WindowSpec{Title: "Main"}, Root: root})
	main := x.windows()[0]
	w.Hide()
	x.drain()
	presents := main.Presents()

	setOpen.Set(true)
	x.drain()
	if len(x.windows()) != 2 {
		t.Fatalf("%d windows open, want the main window and Prefs", len(x.windows()))
	}
	c := x.child(0)
	if c.Title() != "Prefs" || !c.Visible() || c.Presents() == 0 || mounts != 1 {
		t.Errorf("Prefs: title %q, visible %v, %d presents, %d mounts", c.Title(), c.Visible(), c.Presents(), mounts)
	}
	if s := x.snapshot(c); !strings.Contains(s, "preferences") {
		t.Errorf("Prefs shows %q", s)
	}

	// Its close button runs OnClose, which sets the hidden window's state.
	c.RequestClose()
	x.drain()
	if !c.Closed() || cleanups != 1 || len(x.h.Windows()) != 1 {
		t.Errorf("after the close request: closed %v, %d cleanups, %d windows", c.Closed(), cleanups, len(x.h.Windows()))
	}
	if main.Presents() != presents || main.Visible() {
		t.Errorf("the hidden main window presented or showed (%d presents, visible %v)", main.Presents(), main.Visible())
	}
}

// State set in a visible window reaches a hidden window's tree: its
// components build and its effects run (a context and a Value of the
// main window read by a hidden child window).
func TestHiddenChildWindowFollowsParent(t *testing.T) {
	ctx := CreateContext("light")
	var setTheme, setHidden Setter[string]
	var val *Value[int]
	var seen []string
	content := Component(func() Node {
		th, n := UseContext(ctx), val.Get()
		label := th + "/" + string(rune('0'+n))
		UseEffect(func() func() { seen = append(seen, label); return nil }, label)
		bg := "bg-white"
		if th == "dark" {
			bg = "bg-black text-white"
		}
		return div(Class("w-full h-full "+bg), Text("child "+label))
	})
	root := Component(func() Node {
		th, set := UseState("light")
		setTheme = set
		hid, setH := UseState("hidden")
		setHidden = setH
		val = UseValue(1)
		return ctx.Provider(th, div(win(ChildWindowSpec{
			WindowSpec: WindowSpec{Title: "C", Width: 200, Height: 100, Hidden: hid == "hidden"},
			OnClose:    func() {}, Root: content,
		})))
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	if c.Visible() || c.Presents() != 0 {
		t.Fatalf("hidden child: visible %v, %d presents", c.Visible(), c.Presents())
	}
	if !slices.Equal(seen, []string{"light/1"}) {
		t.Fatalf("effects of the hidden child at open: %q, want [light/1]", seen)
	}
	setTheme.Set("dark")
	x.drain()
	val.Set(2)
	x.drain()
	if s := x.snapshot(c); !strings.Contains(s, "child dark/2") {
		t.Errorf("hidden child shows %q, want dark/2", s)
	}
	if !slices.Equal(seen, []string{"light/1", "dark/1", "dark/2"}) {
		t.Errorf("effects of the hidden child %q", seen)
	}
	if c.Presents() != 0 {
		t.Errorf("the hidden child presented %d frames", c.Presents())
	}
	setHidden.Set("shown")
	x.drain()
	if !c.Visible() || c.Presents() == 0 {
		t.Fatalf("shown child: visible %v, %d presents", c.Visible(), c.Presents())
	}
	expectFullRender(t, c, x.h.TreeOf(c))
}

// A window hidden from the start (App.Hidden) runs its effects: a tray
// icon it shows appears before the window does.
func TestHiddenAtStartRunsEffects(t *testing.T) {
	mounts := 0
	root := Component(func() Node {
		UseMount(func() func() {
			mounts++
			return nil
		})
		return div(Text("x"))
	})
	x := newWinHarness(t, Options{Window: WindowSpec{Hidden: true}, Root: root})
	main := x.windows()[0]
	if main.Visible() || main.Presents() != 0 {
		t.Fatalf("hidden at start: visible %v, %d presents", main.Visible(), main.Presents())
	}
	if mounts != 1 {
		t.Errorf("%d mount effects in a window hidden from the start, want 1", mounts)
	}
}

// The commits of a window without frames are paced like frames: the
// first runs at once, the next one a pausedInterval later (the Host's
// timer wakes the loop then), so a tree that sets state in a loop does
// not spin while hidden.
func TestPausedCommitsArePaced(t *testing.T) {
	var w WindowHandle
	var setN Setter[int]
	var built []int
	root := Component(func() Node {
		w = UseWindow()
		n, set := UseState(0)
		setN = set
		built = append(built, n)
		UseEffect(func() func() {
			if n > 0 && n < 4 {
				set.Set(n + 1)
			}
			return nil
		}, n)
		return div(Textf("n=%d", n))
	})
	x := newWinHarness(t, Options{Root: root})
	w.Hide()
	x.drain()
	built = nil

	setN.Set(1)
	x.b.Drain(x.h)
	if !slices.Equal(built, []int{1}) {
		t.Fatalf("builds %v at once, want [1]", built)
	}
	now := x.b.Now()
	if at, ok := x.h.PausedWork(); !ok || at != now+pausedInterval {
		t.Fatalf("PausedWork = %v, %v; want %v (one pace later)", at, ok, now+pausedInterval)
	}
	if at, ok := x.h.TimerArmed(); !ok || at != now+pausedInterval {
		t.Errorf("timer armed %v at %v, want at %v", ok, at, now+pausedInterval)
	}
	for want := 2; want <= 4; want++ {
		x.b.Advance(pausedInterval / 2)
		x.b.Wake()
		x.b.Drain(x.h)
		if len(built) != want-1 {
			t.Fatalf("builds %v half a pace later, want %d", built, want-1)
		}
		x.b.Advance(pausedInterval - pausedInterval/2)
		x.b.Wake()
		x.b.Drain(x.h)
		if built[len(built)-1] != want {
			t.Fatalf("builds %v a pace later, want %d last", built, want)
		}
	}
	if _, ok := x.h.PausedWork(); ok {
		t.Error("PausedWork reports work after the loop ended")
	}
	if !slices.Equal(built, []int{1, 2, 3, 4}) {
		t.Errorf("builds %v, want [1 2 3 4]", built)
	}
}

// A window the OS reports with an empty size (Win32 minimizes to a 0x0
// client area) keeps its tree's size.
func TestEmptyConfigureKeepsViewport(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x"))})
	main := x.windows()[0]
	x.h.HandleEvent(main, platform.ConfigureEvent{Metrics: platform.Metrics{Scale: 1}})
	if v := x.h.Tree().Viewport(); v != (gfx.Size{W: 400, H: 300}) {
		t.Errorf("viewport %v after an empty configure, want 400x300", v)
	}
}
