package core

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// newHeadlessHost mounts root in a 400x300 headless window and delivers
// the first configure and frame.
func newHeadlessHost(t *testing.T, root Node) (*headless.Backend, *Host) {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}})
	h, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(h)
	if h.Presents() != 1 {
		t.Fatalf("after the first configure: %d presents, want 1", h.Presents())
	}
	return b, h
}

// panicMessage runs f and returns what it panicked with ("" if nothing).
func panicMessage(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	f()
	return ""
}

// funcID returns the identity of a func value (its closure pointer).
func funcID[F any](f F) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&f)) }

func TestSetStateFromGoroutine(t *testing.T) {
	var set Setter[int]
	builds := 0
	counter := func() Node {
		builds++
		n, s := UseState(0)
		set = s
		return div(Textf("count %d", n))
	}
	b, h := newHeadlessHost(t, Component(counter))
	win := h.Window().(*headless.Window)
	tr := h.Tree()
	if builds != 1 {
		t.Fatalf("builds = %d after mount, want 1", builds)
	}
	frames := tr.Stats().Frames

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		set.Set(1)
	}()
	wg.Wait()

	// The setter woke the loop; the update is applied on the WakeEvent,
	// which requests a frame. The window drew at time 0, so the frame is
	// paced to the next refresh.
	b.Drain(h)
	if !win.FrameRequested() {
		t.Fatal("the state update did not request a frame")
	}
	if builds != 1 {
		t.Errorf("the component built before the frame (%d builds)", builds)
	}
	b.Advance(b.FrameInterval())
	b.Drain(h)
	if builds != 2 {
		t.Errorf("builds = %d, want 2 (exactly one rebuild)", builds)
	}
	if got := tr.Stats().Frames - frames; got != 1 {
		t.Errorf("%d frames ran, want 1", got)
	}
	if h.Presents() != 2 {
		t.Errorf("presents = %d, want 2", h.Presents())
	}
	if got := texts(tr); got != "count 1" {
		t.Errorf("text = %q, want count 1", got)
	}

	// Setting the same value does not rebuild.
	set.Set(1)
	b.Drain(h)
	b.Advance(time.Second)
	b.Drain(h)
	if builds != 2 || win.FrameRequested() {
		t.Errorf("setting an equal value rebuilt (builds %d) or requested a frame", builds)
	}
}

func TestIdleNoFrames(t *testing.T) {
	b, h := newHeadlessHost(t, div(Class("p-4"), Text("idle")))
	win := h.Window().(*headless.Window)
	frames := h.Tree().Stats().Frames
	for i := 0; i < 5; i++ {
		b.Advance(time.Second)
		b.Drain(h)
	}
	if win.FrameRequested() {
		t.Error("an idle tree requested a frame")
	}
	if got := h.Tree().Stats().Frames - frames; got != 0 {
		t.Errorf("%d frames while idle, want 0", got)
	}
	if h.Presents() != 1 {
		t.Errorf("presents = %d while idle, want 1", h.Presents())
	}
	// A Wake with no updates does not request a frame either.
	b.Wake()
	b.Drain(h)
	if win.FrameRequested() {
		t.Error("an empty wake requested a frame")
	}
}

func TestRootSizedOnConfigure(t *testing.T) {
	b, h := newHeadlessHost(t, div(Key("page"), Class("h-full bg-white"), Text("x")))
	win := h.Window().(*headless.Window)
	check := func(w, hh float32) {
		t.Helper()
		root := h.Tree().Inspect()
		if root.Rect != (gfx.Rect{X1: w, Y1: hh}) {
			t.Errorf("root rect %v, want 0,0 %gx%g", root.Rect, w, hh)
		}
		page := find(&root, "page")
		if page == nil || page.Rect.Width() != w || page.Rect.Height() != hh {
			t.Errorf("h-full page %+v, want %gx%g", page, w, hh)
		}
	}
	check(400, 300)
	win.Resize(platform.Size{W: 640, H: 480})
	b.Drain(h)
	check(640, 480)
	if f := win.Frame(); f == nil || f.Rect.Dx() != 640 || f.Rect.Dy() != 480 {
		t.Errorf("presented frame %v, want 640x480", f.Bounds())
	}
	// A scale change keeps the logical layout and renders more pixels.
	win.SetScale(2)
	b.Drain(h)
	check(640, 480)
	if f := win.Frame(); f == nil || f.Rect.Dx() != 1280 || f.Rect.Dy() != 960 {
		t.Errorf("presented frame %v at scale 2, want 1280x960", f.Bounds())
	}
}

func TestCloseQuits(t *testing.T) {
	b, h := newHeadlessHost(t, Text("bye"))
	h.Window().(*headless.Window).RequestClose()
	b.Drain(h)
	if !b.Quitting() {
		t.Error("closing the window did not quit")
	}
}

func TestHookMisuse(t *testing.T) {
	for name, f := range map[string]func(){
		"UseState":        func() { UseState(1) },
		"UseEffect":       func() { UseEffect(func() func() { return nil }) },
		"UseLayoutEffect": func() { UseLayoutEffect(func() func() { return nil }) },
		"UseMemo":         func() { UseMemo(func() int { return 1 }) },
		"UseRef":          func() { UseRef(0) },
		"UseId":           func() { UseId() },
		"UseCallback":     func() { UseCallback(func() {}) },
		"UseContext":      func() { UseContext(CreateContext(0)) },
		"UseReducer":      func() { UseReducer(func(s, a int) int { return s + a }, 0) },
	} {
		msg := panicMessage(f)
		if !strings.Contains(msg, name+" called outside a component build") {
			t.Errorf("%s outside a build: panic %q", name, msg)
		}
	}
}

func TestHookCountChange(t *testing.T) {
	extra := false
	comp := func() Node {
		UseState(0)
		if extra {
			UseState("x")
		}
		return Text("c")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	extra = true
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "called 2 hooks in this build and 1 in the previous one") {
		t.Errorf("panic %q, want a hook-count panic", msg)
	}
}

func TestHookTypeChange(t *testing.T) {
	str := false
	comp := func() Node {
		if str {
			UseState("x")
		} else {
			UseState(0)
		}
		return Text("c")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	str = true
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "hook 1 of") || !strings.Contains(msg, "UseState[int]") || !strings.Contains(msg, "UseState[string]") {
		t.Errorf("panic %q, want the old and new hook types", msg)
	}
}

// devTree returns a dev-mode tree with root mounted and its messages
// collected in *logs.
func devTree(t testing.TB, root Node, logs *[]string) *Tree {
	t.Helper()
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) {
		if logs != nil {
			*logs = append(*logs, m)
		}
	}})
	tr.SetViewport(gfx.Size{W: 200, H: 100})
	tr.Mount(root)
	return tr
}

// countIn counts the occurrences of sub in s.
func countIn(s, sub string) int { return strings.Count(s, sub) }

func TestDevHookOrderKindChange(t *testing.T) {
	cond := true
	comp := func() Node {
		if cond {
			UseState(0)
		}
		UseEffect(func() func() { return nil })
		return Text("c")
	}
	tr := devTree(t, Component(comp), nil)
	cond = false
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	for _, want := range []string{
		"the hook order of <github.com/kuronosu/tela/internal/core.TestDevHookOrderKindChange.func1> changed",
		"hook 1 is UseEffect at core.TestDevHookOrderKindChange.func1 (hooks_test.go:",
		"but it was UseState at core.TestDevHookOrderKindChange.func1 (hooks_test.go:",
		"tela.C",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic %q\ndoes not contain %q", msg, want)
		}
	}
	if countIn(msg, "hooks_test.go:") != 2 {
		t.Errorf("panic %q should name both call sites", msg)
	}
}

func TestDevHookOrderExtraAndMissing(t *testing.T) {
	n := 1
	comp := func() Node {
		for i := 0; i < n; i++ {
			UseRef(i)
		}
		return Text("c")
	}
	tr := devTree(t, Component(comp), nil)
	n = 2
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "called UseRef at core.TestDevHookOrderExtraAndMissing.func1 (hooks_test.go:") ||
		!strings.Contains(msg, "a hook its previous builds did not call (they called 1 hooks)") {
		t.Errorf("extra hook: panic %q", msg)
	}

	n = 2
	tr = devTree(t, Component(comp), nil)
	n = 1
	msg = panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "called 1 hooks in this build and 2 in the previous one: UseRef at core.TestDevHookOrderExtraAndMissing.func1 (hooks_test.go:") ||
		!strings.Contains(msg, "was not called") {
		t.Errorf("missing hook: panic %q", msg)
	}
}

func TestDevCustomHookSite(t *testing.T) {
	// A hook inside a function called by the component (a custom hook)
	// is reported where the custom hook calls it.
	cond := true
	counterHook := func() int {
		v, _ := UseState(0)
		return v
	}
	comp := func() Node {
		if cond {
			counterHook()
		}
		UseId()
		return Text("c")
	}
	tr := devTree(t, Component(comp), nil)
	cond = false
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "it was UseState at core.TestDevCustomHookSite.func1 (hooks_test.go:") ||
		!strings.Contains(msg, "hook 1 is UseId at core.TestDevCustomHookSite.func2 (hooks_test.go:") {
		t.Errorf("panic %q", msg)
	}
}

func TestCustomHookBelongsToCaller(t *testing.T) {
	// A component function called directly is a custom hook: its state
	// lives in the caller's Element.
	var setInner func(int)
	inner := func() Node {
		v, s := UseStateSet(10)
		setInner = s
		return Textf("inner %d", v)
	}
	outerBuilds := 0
	outer := func() Node {
		outerBuilds++
		return div(inner())
	}
	tr := newTestTree(t, 200, 100, Component(outer), nil)
	tr.Frame()
	setInner(11)
	tr.Frame()
	if outerBuilds != 2 {
		t.Errorf("outer built %d times, want 2 (the inlined hook rebuilds its caller)", outerBuilds)
	}
	if got := texts(tr); got != "inner 11" {
		t.Errorf("got %q", got)
	}
}

func TestUnmountedUpdateIgnored(t *testing.T) {
	var set Setter[int]
	var dispatch Dispatcher[int]
	child := func() Node {
		v, s := UseState(0)
		_, d := UseReducer(func(s, a int) int { return s + a }, 0)
		set, dispatch = s, d
		return Textf("%d", v)
	}
	tr := newTestTree(t, 100, 100, div(Component(child)), nil)
	tr.Frame()
	rerender(tr, div())
	builds := tr.Stats().Builds
	set.Set(3)
	set.Update(func(v int) int { t.Error("an updater of an unmounted component ran"); return v })
	dispatch.Dispatch(1)
	if tr.ApplyUpdates() {
		t.Error("an update of an unmounted component reported work")
	}
	tr.Frame()
	if tr.Stats().Builds != builds {
		t.Error("an unmounted component was built")
	}
}

func TestSetterIdentity(t *testing.T) {
	var setters []Setter[int]
	var dispatchers []Dispatcher[string]
	var sets []func(int)
	var updates []func(func(int) int)
	comp := func() Node {
		_, s := UseState(0)
		_, d := UseReducer(func(s int, a string) int { return s + len(a) }, 0)
		_, set := UseStateSet(0)
		_, upd := UseStateUpdate(0)
		setters, dispatchers = append(setters, s), append(dispatchers, d)
		sets, updates = append(sets, set), append(updates, upd)
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Mount(Component(comp))
	if len(setters) != 2 || setters[0] != setters[1] || dispatchers[0] != dispatchers[1] {
		t.Error("the Setter or Dispatcher of a slot changed between builds")
	}
	if setters[0] == (Setter[int]{}) || dispatchers[0] == (Dispatcher[string]{}) {
		t.Error("a Setter or Dispatcher is zero")
	}
	if funcID(sets[0]) != funcID(sets[1]) || funcID(updates[0]) != funcID(updates[1]) {
		t.Error("UseStateSet or UseStateUpdate made a new function on the second build")
	}
	// Setters of different slots differ.
	other := func() Node {
		_, s := UseState(0)
		setters = append(setters, s)
		return Text("y")
	}
	newTestTree(t, 100, 100, Component(other), nil)
	if setters[2] == setters[0] {
		t.Error("two slots share a Setter")
	}
}

func TestZeroHandlesPanic(t *testing.T) {
	for name, f := range map[string]func(){
		"zero Setter":     func() { Setter[int]{}.Set(1) },
		"zero Setter.":    func() { Setter[int]{}.Update(func(v int) int { return v }) },
		"zero Dispatcher": func() { Dispatcher[int]{}.Dispatch(1) },
		"zero Callback":   func() { Callback[func()]{}.Fn() },
	} {
		msg := panicMessage(f)
		if !strings.Contains(msg, strings.TrimSuffix(name, ".")) || !strings.Contains(msg, "returned by tela.Use") {
			t.Errorf("%s: panic %q", name, msg)
		}
	}
}

func TestQueuedUpdatesApplyInOrder(t *testing.T) {
	var set Setter[int]
	var d Dispatcher[string]
	var seen []string
	builds := 0
	comp := func() Node {
		builds++
		v, s := UseState(0)
		w, dd := UseReducer(func(s int, a string) int {
			switch a {
			case "inc":
				return s + 1
			case "double":
				return s * 2
			}
			return s
		}, 1)
		set, d = s, dd
		seen = append(seen, fmt.Sprintf("%d/%d", v, w))
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	inc := func(v int) int { return v + 1 }

	// Two queued Update(inc) add 2; Set in between resets the base.
	set.Update(inc)
	set.Update(inc)
	tr.Frame()
	set.Update(inc)
	set.Set(10)
	set.Update(func(v int) int { return v * 3 })
	d.Dispatch("inc")
	d.Dispatch("double")
	d.Dispatch("inc")
	tr.Frame()
	if want := []string{"0/1", "2/1", "30/5"}; !slices.Equal(seen, want) {
		t.Errorf("builds saw %q, want %q", seen, want)
	}
	if builds != 3 {
		t.Errorf("%d builds, want 3 (updates are batched per frame)", builds)
	}

	// An update that leaves a comparable state == to the current one
	// does not rebuild, even through Update or Dispatch.
	set.Update(func(v int) int { return v })
	d.Dispatch("noop")
	if tr.ApplyUpdates() {
		t.Error("an update to an equal value marked the component")
	}
}

func TestReducerUsesLatestBuild(t *testing.T) {
	step := 1
	var d Dispatcher[struct{}]
	var got int
	comp := func() Node {
		k := step
		v, dd := UseReducer(func(s int, _ struct{}) int { return s + k }, 0)
		d, got = dd, v
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	step = 10
	tr.Mount(Component(comp)) // the reducer of this build adds 10
	d.Dispatch(struct{}{})
	tr.Frame()
	if got != 10 {
		t.Errorf("state %d, want 10 (the latest build's reducer)", got)
	}
}

func TestNonComparableStateAlwaysRebuilds(t *testing.T) {
	var setSlice Setter[[]int]
	var setAny Setter[any]
	builds := 0
	comp := func() Node {
		builds++
		_, s := UseState([]int{1})
		_, a := UseState[any](nil)
		setSlice, setAny = s, a
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	setSlice.Set([]int{1})
	tr.Frame()
	if builds != 2 {
		t.Errorf("a slice state set again did not rebuild (%d builds)", builds)
	}
	// An interface holding a func cannot be compared: no panic, a rebuild.
	setAny.Set(func() {})
	tr.Frame()
	setAny.Set(func() {})
	tr.Frame()
	if builds != 4 {
		t.Errorf("%d builds, want 4", builds)
	}
	// An interface holding an equal comparable value bails out.
	setAny.Set(3)
	tr.Frame()
	setAny.Set(3)
	tr.Frame()
	if builds != 5 {
		t.Errorf("%d builds, want 5", builds)
	}
}

func TestUpdaterNilPanics(t *testing.T) {
	var set Setter[int]
	comp := func() Node {
		_, set = UseState(0)
		return Text("x")
	}
	newTestTree(t, 100, 100, Component(comp), nil)
	if msg := panicMessage(func() { set.Update(nil) }); !strings.Contains(msg, "Update(nil)") {
		t.Errorf("panic %q", msg)
	}
	if msg := panicMessage(func() { UseReducer[int, int](nil, 0) }); !strings.Contains(msg, "UseReducer(nil") {
		t.Errorf("panic %q", msg)
	}
}

// TestSettersFromGoroutines updates state from many goroutines at once
// while the UI thread applies and builds (run with -race).
func TestSettersFromGoroutines(t *testing.T) {
	var set Setter[int]
	var d Dispatcher[int]
	var last, lastR int
	comp := func() Node {
		v, s := UseState(0)
		w, dd := UseReducer(func(s, a int) int { return s + a }, 0)
		set, d, last, lastR = s, dd, v, w
		return Textf("%d %d", v, w)
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	wakes := make(chan struct{}, 1)
	tr.SetWake(func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	})
	const workers, per = 8, 200
	s0, d0 := set, d
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				s0.Update(func(v int) int { return v + 1 })
				d0.Dispatch(2)
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for running := true; running; {
		select {
		case <-wakes:
			tr.Frame()
		case <-done:
			running = false
		}
	}
	tr.Frame()
	if last != workers*per || lastR != 2*workers*per {
		t.Errorf("state %d and %d, want %d and %d", last, lastR, workers*per, 2*workers*per)
	}
}

func TestPost(t *testing.T) {
	var log []string
	var set Setter[int]
	comp := func() Node {
		v, s := UseState(0)
		set = s
		log = append(log, fmt.Sprint("build ", v))
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()

	// Before any tree receives posts, they wait, in order.
	Post(func() { log = append(log, "early 1") })
	Post(func() { log = append(log, "early 2") })
	tr.ApplyUpdates()
	if slices.Contains(log, "early 1") {
		t.Fatal("a post ran before a tree received posts")
	}
	stop := tr.ReceivePosts()
	defer stop()

	// Posts run in order with the state updates queued around them.
	set.Update(func(v int) int { log = append(log, "update 1"); return v + 1 })
	Post(func() { log = append(log, "post") })
	set.Update(func(v int) int { log = append(log, "update 2"); return v + 1 })
	// A post that sets state: the update is applied in the same call.
	Post(func() { set.Update(func(v int) int { log = append(log, "from post"); return v + 1 }) })
	tr.Frame()
	want := []string{"build 0", "early 1", "early 2", "update 1", "post", "update 2", "from post", "build 3"}
	if !slices.Equal(log, want) {
		t.Errorf("log %q, want %q", log, want)
	}

	// After stop, posts wait for the next receiver.
	stop()
	ran := false
	Post(func() { ran = true })
	tr.ApplyUpdates()
	if ran {
		t.Error("a post ran on a tree that stopped receiving")
	}
	tr2 := newTestTree(t, 100, 100, Text("y"), nil)
	stop2 := tr2.ReceivePosts()
	defer stop2()
	tr2.ApplyUpdates()
	if !ran {
		t.Error("a post did not run on the next receiver")
	}
	if msg := panicMessage(func() { Post(nil) }); !strings.Contains(msg, "Post(nil)") {
		t.Errorf("panic %q", msg)
	}
}

// TestPostFromGoroutines posts from several goroutines to a host (run
// with -race): each goroutine's posts run in its order.
func TestPostFromGoroutines(t *testing.T) {
	b, h := newHeadlessHost(t, Text("x"))
	stop := h.Tree().ReceivePosts()
	defer stop()
	const workers, per = 4, 100
	got := make([][]int, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				Post(func() { got[w] = append(got[w], i) })
			}
		}()
	}
	wg.Wait()
	b.Drain(h)
	for w := range got {
		if len(got[w]) != per || !slices.IsSorted(got[w]) {
			t.Errorf("worker %d: %d posts ran, in order %v", w, len(got[w]), slices.IsSorted(got[w]))
		}
	}
}

func TestIsAnonymous(t *testing.T) {
	named := reflect.ValueOf(TestIsAnonymous).Pointer()
	closure := reflect.ValueOf(func() {}).Pointer()
	var pr counterProbe
	method := reflect.ValueOf(pr.render).Pointer()
	for _, c := range []struct {
		pc   uintptr
		want bool
	}{{named, false}, {closure, true}, {method, true}} {
		if got := noteAnon(c.pc, "CP") != nil; got != c.want {
			t.Errorf("%s: anonymous %v, want %v", nameOf(c.pc), got, c.want)
		}
	}
	for name, want := range map[string]bool{
		"main.Counter":                        false,
		"main.Counter.func1":                  true,
		"main.Counter.func1.2":                true,
		"github.com/x/app.(*Page).Render":     false,
		"github.com/x/app.(*Page).Render-fm":  true,
		"github.com/x/app.Card[...]":          false,
		"github.com/x/app.Card[...].func3":    true,
		"github.com/x/app.init.func1":         true,
		"github.com/x/app.glob..func2":        true,
		"github.com/x/function.funcsAreFun":   false,
		"github.com/x/app.functionalCard":     false,
		"github.com/x/app.Card[go.shape.int]": false,
	} {
		if got := isAnonymous(name); got != want {
			t.Errorf("isAnonymous(%q) = %v, want %v", name, got, want)
		}
	}
}

func nameOf(pc uintptr) string { return (&compDesc{typ: pc}).name() }

// TestPostReceiversStack covers two trees receiving posts at once (a test
// that mounts an app while another is alive): the newest runs them, and
// when it stops the older one receives them again instead of the posts
// waiting forever.
func TestPostReceiversStack(t *testing.T) {
	a, b := NewTree(Config{}), NewTree(Config{})
	stopA := a.ReceivePosts()
	defer stopA()
	stopB := b.ReceivePosts()
	var ran []string
	Post(func() { ran = append(ran, "first") })
	b.ApplyUpdates()
	a.ApplyUpdates()
	if len(ran) != 1 {
		t.Fatalf("ran %v, want the first post run by the newest receiver", ran)
	}
	stopB()
	stopB() // stopping twice is harmless
	Post(func() { ran = append(ran, "second") })
	a.ApplyUpdates()
	if len(ran) != 2 || ran[1] != "second" {
		t.Errorf("ran %v after the newest receiver stopped, want the older one to run it", ran)
	}
}
