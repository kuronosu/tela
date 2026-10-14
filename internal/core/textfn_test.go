package core

import (
	"fmt"
	"strings"
	"testing"
)

func paraText(t *testing.T, tr *Tree, key string) string {
	t.Helper()
	n := mustFind(t, tr, key)
	return n.Text
}

func TestTextFnRelaysOutWithoutBuilding(t *testing.T) {
	builds := 0
	var v *Value[int]
	comp := func() Node {
		builds++
		v = UseValue(5)
		return div(Class("w-[300px]"),
			p(Key("label"), Class("w-fit"), TextFn(func() string { return fmt.Sprintf("%d", v.Get()) })))
	}
	tr := newTestTree(t, 400, 200, Component(comp), nil)
	tr.Frame()
	if got := paraText(t, tr, "label"); got != "5" {
		t.Fatalf("text %q, want 5", got)
	}
	w0 := mustFind(t, tr, "label").Rect.Width()
	s0 := tr.Stats()
	v.Set(1234)
	tr.Frame()
	d := work(s0, tr.Stats())
	if got := paraText(t, tr, "label"); got != "1234" {
		t.Errorf("text %q, want 1234", got)
	}
	if w := mustFind(t, tr, "label").Rect.Width(); !(w > w0) {
		t.Errorf("width %g did not grow from %g", w, w0)
	}
	if d.Builds != 0 || d.Styles != 0 || d.Shapes != 1 {
		t.Errorf("work %+v, want one shaping and no build or style", d)
	}
	if builds != 1 {
		t.Errorf("the component built %d times", builds)
	}
}

func TestTextFnUnchangedTextDoesNotLayOut(t *testing.T) {
	var v *Value[int]
	comp := func() Node {
		v = UseValue(5)
		return div(p(Key("label"), TextFn(func() string { return fmt.Sprint(v.Get() / 10) })))
	}
	tr := newTestTree(t, 400, 200, Component(comp), nil)
	tr.Frame()
	s0 := tr.Stats()
	v.Set(7) // 0 again
	tr.Frame()
	if d := work(s0, tr.Stats()); d.Shapes != 0 || d.Breaks != 0 || d.Builds != 0 {
		t.Errorf("an unchanged text did work: %+v", d)
	}
	if tr.Dirty() {
		t.Error("an unchanged text left the tree dirty")
	}
}

func TestTextFnInsideParagraphs(t *testing.T) {
	var v *Value[string]
	comp := func() Node {
		v = UseValue("1:05")
		return div(
			p(Key("para"), Text("Time: "), strong(Class("tabular-nums"), TextFn(func() string { return v.Get() })), Text(" left, "),
				span(Class("px-1"), Text("of "), TextFn(func() string { return "3:00" })), Text(".")),
			// Next to a flex container's other children: its own anonymous item.
			div(Key("row"), Class("flex gap-2"), TextFn(func() string { return "a " + v.Get() }), span(Text("b"))),
		)
	}
	tr := newTestTree(t, 400, 200, Component(comp), nil)
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "Time: 1:05 left, of 3:00." {
		t.Errorf("paragraph %q", got)
	}
	s0 := tr.Stats()
	v.Set("12:34")
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "Time: 12:34 left, of 3:00." {
		t.Errorf("paragraph after the change %q", got)
	}
	row := mustFind(t, tr, "row")
	if len(row.Children) != 2 || row.Children[0].Text != "a 12:34" {
		t.Errorf("flex row:\n%s", dump(row))
	}
	if d := work(s0, tr.Stats()); d.Builds != 0 || d.Styles != 0 {
		t.Errorf("work %+v", d)
	}
}

func TestTextFnFollowsTheNewestClosure(t *testing.T) {
	// A rebuild replaces the function: a closure over the component's
	// state reads the new state.
	var set func(string)
	comp := func() Node {
		s, st := UseStateSet("one")
		set = st
		return p(Key("para"), TextFn(func() string { return s }))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "one" {
		t.Fatalf("text %q", got)
	}
	set("two")
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "two" {
		t.Errorf("text %q after the rebuild", got)
	}
}

func TestTextFnAndTextSwitch(t *testing.T) {
	var v *Value[int]
	var set func(bool)
	comp := func() Node {
		v = UseValue(1)
		useFn, st := UseStateSet(true)
		set = st
		if useFn {
			return p(Key("para"), TextFn(func() string { return fmt.Sprint("n", v.Get()) }))
		}
		return p(Key("para"), Text("static"))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	set(false)
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "static" {
		t.Fatalf("text %q", got)
	}
	// The old function no longer reads: a change of its Value does nothing.
	s0 := tr.Stats()
	v.Set(2)
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "static" {
		t.Errorf("text %q after the Value changed", got)
	}
	if d := work(s0, tr.Stats()); d.Shapes != 0 || d.Breaks != 0 {
		t.Errorf("a dropped TextFn still did work: %+v", d)
	}
	set(true)
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "n2" {
		t.Errorf("text %q after switching back", got)
	}
}

func TestTextFnHiddenAndUnmounted(t *testing.T) {
	var v *Value[int]
	var set func(bool)
	comp := func() Node {
		v = UseValue(1)
		hidden, st := UseStateSet(true)
		set = st
		cls := "w-fit"
		if hidden {
			cls += " hidden"
		}
		return div(p(Key("para"), Class(cls), TextFn(func() string { return fmt.Sprint("n", v.Get()) })))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	v.Set(9) // while display: none
	tr.Frame()
	set(false)
	tr.Frame()
	if got := paraText(t, tr, "para"); got != "n9" {
		t.Errorf("text %q once shown", got)
	}
	tr.Unmount()
	v.Set(10)
	tr.Frame() // nothing mounted: the update is dropped
}

func TestTextFnNil(t *testing.T) {
	msg := panicMessage(func() { TextFn(nil) })
	if !strings.Contains(msg, "TextFn(nil)") {
		t.Errorf("panic %q", msg)
	}
}
