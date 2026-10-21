package tela_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// mount mounts root in a new tree (dev mode with logs when logs is not
// nil) and runs a frame.
func mount(t *testing.T, root tela.Node, logs *[]string) *core.Tree {
	t.Helper()
	cfg := core.Config{}
	if logs != nil {
		cfg.Dev = true
		cfg.Log = func(m string) { *logs = append(*logs, m) }
	}
	tr := core.NewTree(cfg)
	tr.SetViewport(gfx.Size{W: 200, H: 100})
	tr.Mount(root)
	tr.Frame()
	return tr
}

func panicOf(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	f()
	return ""
}

func TestPublicStateHooks(t *testing.T) {
	var got []string
	var set func(string)
	var update func(func(int) int)
	var d tela.Dispatcher[int]
	comp := func() tela.Node {
		s, st := tela.UseStateSet("a")
		n, up := tela.UseStateUpdate(1)
		r, dd := tela.UseReducer(func(s, a int) int { return s*10 + a }, 0)
		set, update, d = st, up, dd
		got = append(got, fmt.Sprintf("%s %d %d", s, n, r))
		return div()
	}
	tr := mount(t, tela.C(comp), nil)
	set("b")
	update(func(n int) int { return n + 1 })
	update(func(n int) int { return n + 1 })
	d.Dispatch(4)
	d.Dispatch(2)
	tr.Frame()
	if want := []string{"a 1 0", "b 3 42"}; !slices.Equal(got, want) {
		t.Errorf("builds %q, want %q", got, want)
	}
}

func TestPublicEffects(t *testing.T) {
	var log []string
	comp := func() tela.Node {
		tela.UseLayoutEffect(func() func() { log = append(log, "layout"); return nil }, nil)
		tela.UseEffect(func() func() {
			log = append(log, "effect")
			return func() { log = append(log, "cleanup") }
		}, nil)
		return div()
	}
	tr := mount(t, tela.C(comp), nil)
	tr.FlushEffects()
	tr.Mount(tela.C(comp))
	tr.Frame()
	tr.FlushEffects()
	tr.Unmount()
	if want := []string{"layout", "effect", "cleanup"}; !slices.Equal(log, want) {
		t.Errorf("log %q, want %q (nil deps: mount only)", log, want)
	}
}

func TestPublicMemoRefIdCallback(t *testing.T) {
	computes := 0
	var refs []*tela.Ref[int]
	var ids []string
	var cbs []tela.Callback[func(tela.MouseEvent)]
	var clicks []float32
	gen := 0
	comp := func() tela.Node {
		g := gen
		tela.UseMemo(func() int { computes++; return 1 }, "k")
		refs = append(refs, tela.UseRef(0))
		ids = append(ids, tela.UseID())
		cbs = append(cbs, tela.UseCallback(func(e tela.MouseEvent) { clicks = append(clicks, e.X+float32(g)) }))
		return div()
	}
	tr := mount(t, tela.C(comp), nil)
	gen = 100
	tr.Mount(tela.C(comp))
	if computes != 1 || refs[0] != refs[1] || ids[0] != ids[1] || ids[0] == "" || cbs[0] != cbs[1] {
		t.Errorf("computes %d, refs %p %p, ids %q, callbacks equal %v", computes, refs[0], refs[1], ids, cbs[0] == cbs[1])
	}
	cbs[0].Fn()(tela.MouseEvent{X: 1})
	if !slices.Equal(clicks, []float32{101}) {
		t.Errorf("clicks %v, want [101] (the latest function)", clicks)
	}
}

// TestEventCallbacksDoNotAllocate checks that the UseCallback forwarders
// of the event handler types are registered closures, not
// reflect.MakeFunc (which allocates on every call).
func TestEventCallbacksDoNotAllocate(t *testing.T) {
	var m tela.Callback[func(tela.MouseEvent)]
	var p tela.Callback[func(tela.PointerEvent)]
	var d tela.Callback[func(tela.DragEvent)]
	var k tela.Callback[func(tela.KeyboardEvent)]
	var f tela.Callback[func(tela.FocusEvent)]
	sum := float32(0)
	comp := func() tela.Node {
		m = tela.UseCallback(func(e tela.MouseEvent) { sum += e.X })
		p = tela.UseCallback(func(e tela.PointerEvent) { sum += e.X })
		d = tela.UseCallback(func(e tela.DragEvent) { sum += e.DeltaX })
		k = tela.UseCallback(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyEnter {
				sum++
			}
		})
		f = tela.UseCallback(func(tela.FocusEvent) { sum++ })
		return div()
	}
	mount(t, tela.C(comp), nil)
	fm, fp, fd, fk, ff := m.Fn(), p.Fn(), d.Fn(), k.Fn(), f.Fn()
	allocs := testing.AllocsPerRun(100, func() {
		fm(tela.MouseEvent{X: 1})
		fp(tela.PointerEvent{X: 1})
		fd(tela.DragEvent{DeltaX: 1})
		fk(tela.KeyboardEvent{Key: "Enter"})
		ff(tela.FocusEvent{})
	})
	if allocs != 0 {
		t.Errorf("%v allocations per call of the forwarders", allocs)
	}
	if sum == 0 {
		t.Error("the forwarders did not call the functions")
	}
}

var themeCtx = tela.CreateContext("light")

type labelProps struct{ Text string }

var labelBuilds int

func label(p labelProps) tela.Node {
	labelBuilds++
	return div(core.Text(p.Text + "/" + tela.UseContext(themeCtx)))
}

func TestPublicComponentsAndContext(t *testing.T) {
	labelBuilds = 0
	var setTheme tela.Setter[string]
	app := func() tela.Node {
		theme, st := tela.UseState("dark")
		setTheme = st
		return themeCtx.Provider(theme, tela.CP(label, labelProps{"x"}))
	}
	tr := mount(t, tela.C(app), nil)
	tr.Mount(tela.C(app)) // equal props, same value: skipped
	if labelBuilds != 1 {
		t.Errorf("%d label builds, want 1", labelBuilds)
	}
	setTheme.Set("light")
	tr.Frame()
	if labelBuilds != 2 {
		t.Errorf("%d label builds after a context change, want 2", labelBuilds)
	}
	memoBuilds := 0
	chart := func(p []int) tela.Node { memoBuilds++; return div() }
	eq := func(a, b []int) bool { return slices.Equal(a, b) }
	tr2 := mount(t, tela.Memo(chart, []int{1}, eq), nil)
	tr2.Mount(tela.Memo(chart, []int{1}, eq))
	if memoBuilds != 1 {
		t.Errorf("%d Memo builds, want 1", memoBuilds)
	}
}

func TestPublicHookOrderSites(t *testing.T) {
	// Dev-mode call sites skip the wrappers of package tela: they name
	// the user's line.
	cond := true
	comp := func() tela.Node {
		if cond {
			tela.UseState(0)
		}
		tela.UseMemo(func() int { return 0 })
		return div()
	}
	var logs []string
	tr := mount(t, tela.C(comp), &logs)
	cond = false
	msg := panicOf(func() { tr.Mount(tela.C(comp)) })
	if strings.Count(msg, "(hooks_test.go:") != 2 || strings.Contains(msg, "tela.UseState") || strings.Contains(msg, "tela.UseMemo") {
		t.Errorf("panic %q should name two lines of hooks_test.go", msg)
	}

	// Anonymous CP functions are reported at the user's call.
	logs = nil
	page := func() tela.Node {
		return div(tela.CP(func(n int) tela.Node { return div() }, 1))
	}
	mount(t, tela.C(page), &logs)
	if len(logs) != 1 || !strings.Contains(logs[0], "tela: CP(...) at tela_test.TestPublicHookOrderSites.func3 (hooks_test.go:") {
		t.Errorf("logs %q", logs)
	}
}

func TestPublicPost(t *testing.T) {
	tr := mount(t, div(), nil)
	ran := false
	tela.Post(func() { ran = true })
	stop := tr.ReceivePosts()
	defer stop()
	tr.ApplyUpdates()
	if !ran {
		t.Error("the posted function did not run")
	}
}

func TestPublicUseValue(t *testing.T) {
	var pos *tela.Value[float64]
	var width *tela.Value[float32]
	builds := 0
	comp := func() tela.Node {
		builds++
		pos = tela.UseValue(0.0)
		width = tela.UseValue[float32](10)
		return div(core.Class("h-4 bg-sky-500"), core.Style(
			tela.TranslateFn(func() (x, y float32) { return float32(pos.Get()), 0 }),
			tela.WidthFn(func() float32 { return width.Get() }),
		))
	}
	tr := mount(t, tela.C(comp), nil)
	first := pos

	// From a goroutine: the element is repainted, nothing else.
	s0 := tr.Stats()
	done := make(chan struct{})
	go func() {
		pos.Set(12)
		pos.Update(func(v float64) float64 { return v + 1 })
		close(done)
	}()
	<-done
	tr.Frame()
	s1 := tr.Stats()
	if builds != 1 || s1.Paints-s0.Paints != 1 || s1.Layouts != s0.Layouts || s1.Styles != s0.Styles {
		t.Errorf("paint read: %d builds, stats %+v -> %+v; want a paint only", builds, s0, s1)
	}
	if pos.Get() != 13 {
		t.Errorf("Get = %g, want 13", pos.Get())
	}

	// A layout read: laid out again, not built.
	width.Set(40)
	tr.Frame()
	s2 := tr.Stats()
	if builds != 1 || s2.Layouts-s1.Layouts != 1 || s2.Styles != s1.Styles {
		t.Errorf("layout read: %d builds, stats %+v -> %+v; want a layout without building", builds, s1, s2)
	}
	if w := tr.Inspect().Children[0].Rect.Width(); w != 40 {
		t.Errorf("width %g, want 40", w)
	}

	// The *Value is stable across builds.
	tr.Mount(tela.C(comp))
	if pos != first {
		t.Error("UseValue returned another *Value on the next build")
	}
	if msg := panicOf(func() { tela.UseValue(1) }); !strings.Contains(msg, "UseValue called outside a component build") {
		t.Errorf("panic %q", msg)
	}
}
