package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform/headless"
)

// effectLog records effect events in order.
type effectLog struct{ events []string }

func (l *effectLog) add(format string, args ...any) {
	l.events = append(l.events, fmt.Sprintf(format, args...))
}

// take returns the events since the last take.
func (l *effectLog) take() []string {
	e := l.events
	l.events = nil
	return e
}

// effect returns an effect that logs "name" when it runs and "~name"
// when cleaned up.
func (l *effectLog) effect(name string) func() func() {
	return func() func() {
		l.add("%s", name)
		return func() { l.add("~%s", name) }
	}
}

func expectEvents(t *testing.T, l *effectLog, what string, want ...string) {
	t.Helper()
	if got := l.take(); !slices.Equal(got, want) {
		t.Errorf("%s: events %q, want %q", what, got, want)
	}
}

func TestEffectTiming(t *testing.T) {
	var l effectLog
	var tr *Tree
	paintsAt := map[string]int{}
	comp := func() Node {
		UseLayoutEffect(func() func() {
			paintsAt["layout"] = tr.Stats().Paints
			l.add("layout")
			return nil
		})
		UseEffect(func() func() {
			paintsAt["passive"] = tr.Stats().Paints
			l.add("passive")
			return nil
		})
		return Text("x")
	}
	tr = newTestTree(t, 100, 100, Component(comp), nil)
	expectEvents(t, &l, "after Mount (built, not committed)")
	tr.Frame()
	expectEvents(t, &l, "after Frame", "layout")
	if paintsAt["layout"] != 0 {
		t.Errorf("the layout effect ran after %d paints, want 0 (before paint)", paintsAt["layout"])
	}
	tr.FlushEffects()
	expectEvents(t, &l, "after FlushEffects", "passive")
	if paintsAt["passive"] != 1 {
		t.Errorf("the passive effect ran after %d paints, want 1", paintsAt["passive"])
	}
	tr.FlushEffects()
	expectEvents(t, &l, "a second flush")

	// A frame whose passive effects were not flushed runs them first.
	tr.Mount(Component(comp))
	tr.Frame()
	expectEvents(t, &l, "rebuilt", "layout")
	tr.Mount(Component(comp))
	expectEvents(t, &l, "Mount flushes the previous frame", "passive")
	tr.Frame()
	expectEvents(t, &l, "frame", "layout")
	tr.Frame()
	expectEvents(t, &l, "Frame flushes the previous frame", "passive")
}

func TestEffectAfterPresent(t *testing.T) {
	var h *Host
	var presentsAt []int
	var set Setter[int]
	comp := func() Node {
		v, s := UseState(0)
		set = s
		UseEffect(func() func() {
			presentsAt = append(presentsAt, h.Presents())
			if v == 1 {
				s.Set(2) // state set by an effect schedules another frame
			}
			return nil
		}, v)
		return Textf("%d", v)
	}
	b := headless.New(headless.Options{})
	var err error
	h, err = NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: Component(comp)})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(h)
	if !slices.Equal(presentsAt, []int{1}) {
		t.Fatalf("effect ran at presents %v, want [1] (after the first present)", presentsAt)
	}
	set.Set(1)
	b.Drain(h)
	b.Advance(b.FrameInterval())
	b.Drain(h)
	b.Advance(b.FrameInterval())
	b.Drain(h)
	if !slices.Equal(presentsAt, []int{1, 2, 3}) {
		t.Errorf("effect ran at presents %v, want [1 2 3]", presentsAt)
	}
	if got := texts(h.Tree()); got != "2" {
		t.Errorf("text %q, want 2 (set by the effect)", got)
	}
	if h.Window().(*headless.Window).FrameRequested() {
		t.Error("a frame is still requested")
	}
}

func TestEffectDeps(t *testing.T) {
	var l effectLog
	type props struct {
		a    int
		f    func()
		deps []any
	}
	var p props
	var logs []string
	comp := func() Node {
		UseEffect(l.effect("every"))
		UseEffect(l.effect("mount"), Once)
		UseEffect(l.effect("a"), p.a)
		UseEffect(l.effect("f"), p.f)
		UseEffect(l.effect("var"), p.deps...)
		return Text("x")
	}
	p = props{a: 1, f: func() {}, deps: []any{1, "x"}}
	tr := devTree(t, Component(comp), &logs)
	build := func() {
		tr.Mount(Component(comp))
		tr.Frame()
		tr.FlushEffects()
	}
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "mount", "every", "mount", "a", "f", "var")
	if len(logs) != 1 ||
		!strings.Contains(logs[0], "dependency 1 of UseEffect at core.TestEffectDeps.func1 (effects_test.go:") ||
		!strings.Contains(logs[0], "is a func()") || !strings.Contains(logs[0], "tela.UseCallback") {
		t.Errorf("logs %q, want one warning about the func dependency", logs)
	}
	logs = nil

	// Same values: only the effects without dependencies and with a
	// non-comparable one run (the warning is not repeated).
	build()
	expectEvents(t, &l, "same deps", "~every", "~f", "every", "f")
	if len(logs) != 0 {
		t.Errorf("logs %q, want none", logs)
	}

	// A changed value runs its effect.
	p.a = 2
	build()
	expectEvents(t, &l, "a changed", "~every", "~a", "~f", "every", "a", "f")

	// A changed count runs it too, with a dev warning.
	p.deps = []any{1}
	build()
	expectEvents(t, &l, "count changed", "~every", "~f", "~var", "every", "f", "var")
	if len(logs) != 1 || !strings.Contains(logs[0], "got 1 dependencies in this build and 2 in the previous one") {
		t.Errorf("logs %q, want a count warning", logs)
	}

	// The dependency slice is copied: changing it after the build does
	// not change what the next build compares against.
	p.deps[0] = 5
	build()
	expectEvents(t, &l, "mutated deps", "~every", "~f", "~var", "every", "f", "var")

	// Unmount runs every cleanup.
	tr.Mount(Text("gone"))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "unmount", "~every", "~mount", "~a", "~f", "~var")
}

func TestEffectDepsNeverPanic(t *testing.T) {
	type holder struct{ v any }
	var dep any
	runs := 0
	var logs []string
	comp := func() Node {
		UseEffect(func() func() { runs++; return nil }, dep)
		return Text("x")
	}
	dep = holder{func() {}} // comparable type, == panics at run time
	tr := devTree(t, Component(comp), &logs)
	for i := 0; i < 3; i++ {
		tr.Frame()
		tr.FlushEffects()
		tr.Mount(Component(comp))
	}
	if runs != 3 {
		t.Errorf("%d runs, want 3 (a dependency whose == panics is always changed)", runs)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "cannot be compared") {
		t.Errorf("logs %q, want one warning", logs)
	}
}

// TestEffectOrder checks React's commit order: children's effects before
// their parent's, every cleanup before the effects that follow it, layout
// effects before passive ones, unmount cleanups parents first.
func TestEffectOrder(t *testing.T) {
	var l effectLog
	type childProps struct {
		name string
		gen  int
	}
	child := func(p childProps) Node {
		UseLayoutEffect(l.effect("L" + p.name))
		UseEffect(l.effect(p.name))
		return Text(p.name)
	}
	gen := 0
	parent := func() Node {
		UseLayoutEffect(l.effect("Lp"))
		UseEffect(l.effect("p"))
		return div(ComponentProps(child, childProps{"a", gen}), div(ComponentProps(child, childProps{"b", gen})))
	}
	tr := newTestTree(t, 200, 100, div(Component(parent)), nil)
	tr.Frame()
	expectEvents(t, &l, "mount layout", "La", "Lb", "Lp")
	tr.FlushEffects()
	expectEvents(t, &l, "mount passive", "a", "b", "p")

	// The parent builds again; the children skip (equal props).
	tr.Mount(div(Component(parent)))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "parent rebuilt", "~Lp", "Lp", "~p", "p")

	// Everything builds: every cleanup of a commit runs before its
	// effects, children before parents.
	gen++
	tr.Mount(div(Component(parent)))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "all rebuilt",
		"~La", "~Lb", "~Lp", "La", "Lb", "Lp",
		"~a", "~b", "~p", "a", "b", "p")

	// Another component at the same position: the old subtree unmounts
	// (cleanups parents first) before the new one's effects run.
	tr.Mount(div(Component(func() Node { return parent() })))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "remounted",
		"~Lp", "~La", "~Lb", "La", "Lb", "Lp",
		"~p", "~a", "~b", "a", "b", "p")
}

func TestEffectsNotRunForUncommittedOrSkipped(t *testing.T) {
	var l effectLog
	comp := func(name string) Node {
		UseLayoutEffect(l.effect("L" + name))
		UseEffect(l.effect(name))
		return Text(name)
	}
	tr := newTestTree(t, 200, 100, div(ComponentProps(comp, "a")), nil)
	// Unmounted before its first commit: its effects never run.
	tr.Mount(div())
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "mounted and unmounted before a frame")

	tr.Mount(div(ComponentProps(comp, "b")))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "mount", "Lb", "b")
	// Skipped: no effects, even without dependencies.
	tr.Mount(div(Class("p-1"), ComponentProps(comp, "b")))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "skipped")
	if tr.Stats().Skips != 1 {
		t.Errorf("%d skips, want 1", tr.Stats().Skips)
	}
}

func TestTreeUnmountRunsCleanups(t *testing.T) {
	var l effectLog
	child := func() Node {
		UseLayoutEffect(l.effect("Lc"))
		UseEffect(l.effect("c"))
		return Text("c")
	}
	parent := func() Node {
		UseEffect(l.effect("p"))
		return div(Component(child))
	}
	tr := newTestTree(t, 200, 100, Component(parent), nil)
	tr.Frame()
	l.take()
	// The passive effects of the last frame were not flushed: they run
	// first, then every cleanup (layout ones first, parents first).
	tr.Unmount()
	expectEvents(t, &l, "Unmount", "c", "p", "~Lc", "~p", "~c")
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "after Unmount")
	if got := texts(tr); got != "" {
		t.Errorf("text %q after Unmount", got)
	}
	// The tree can be mounted again.
	tr.Mount(Component(parent))
	tr.Frame()
	tr.FlushEffects()
	expectEvents(t, &l, "mounted again", "Lc", "c", "p")
}

func TestRenderRunsEffects(t *testing.T) {
	var l effectLog
	comp := func() Node {
		UseLayoutEffect(l.effect("layout"))
		UseEffect(l.effect("passive"))
		return Text("x")
	}
	Render(Component(comp), RenderOptions{Config: Config{Fonts: testFonts}, Width: 50, Height: 20})
	expectEvents(t, &l, "Render", "layout", "passive", "~layout", "~passive")
}

func TestUseMemo(t *testing.T) {
	computes := map[string]int{}
	var a, b int
	var got []int
	comp := func() Node {
		x := UseMemo(func() int { computes["a"]++; return a * 10 }, a)
		y := UseMemo(func() int { computes["every"]++; return b })
		// Outside dev mode a nil list still means "only on mount" (ADR-097).
		z := UseMemo(func() int { computes["once"]++; return 7 }, nil)
		got = []int{x, y, z}
		return Text("x")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	a, b = 1, 2
	tr.Mount(Component(comp))
	if want := map[string]int{"a": 2, "every": 2, "once": 1}; fmt.Sprint(computes) != fmt.Sprint(want) {
		t.Errorf("computes %v, want %v", computes, want)
	}
	tr.Mount(Component(comp))
	if want := map[string]int{"a": 2, "every": 3, "once": 1}; fmt.Sprint(computes) != fmt.Sprint(want) {
		t.Errorf("computes %v, want %v", computes, want)
	}
	if !slices.Equal(got, []int{10, 2, 7}) {
		t.Errorf("values %v", got)
	}
}
