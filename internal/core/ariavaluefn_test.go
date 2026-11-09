package core

import (
	"fmt"
	"math"
	"testing"
)

// TestAriaValueFns: AriaValueNowFn and AriaValueTextFn (ADR-250) give a
// range widget its value when the semantics tree is built; a change of a
// Value they read builds nothing and asks for no frame, it makes the tree
// stale; an AriaLabelFn on the same element is tracked with them; a NaN is
// no value; the static props replace them.
func TestAriaValueFns(t *testing.T) {
	var pct *Value[float64]
	var name *Value[string]
	builds := 0
	tr, _ := snapshotOf(t, Component(func() Node {
		builds++
		pct = UseValue(10.0)
		name = UseValue("Upload")
		return div(div(Role("progressbar"), AriaValueMin(0), AriaValueMax(100),
			AriaLabelFn(func() string { return name.Get() }),
			AriaValueNowFn(pct.Get),
			AriaValueTextFn(func() string { return fmt.Sprintf("%.0f percent", pct.Get()) })))
	}))
	check := func(label string, now float64, text string) {
		t.Helper()
		n := mustNode(t, tr, "progressbar", label)
		if !n.Range.HasNow || n.Range.Now != now || n.Value != text {
			t.Errorf("range %+v, value %q; want now %v and %q", n.Range, n.Value, now, text)
		}
	}
	check("Upload", 10, "10 percent")
	told := 0
	tr.OnSemanticsStale(func() { told++ })
	pct.Set(42)
	if tr.ApplyUpdates() {
		t.Error("a Value read only by AriaValueNowFn and AriaValueTextFn requested a frame")
	}
	if !tr.SemanticsStale() || told != 1 {
		t.Errorf("stale %v, told %d; want true, 1", tr.SemanticsStale(), told)
	}
	check("Upload", 42, "42 percent")
	// The label's Value is tracked with the value's: both make it stale.
	name.Set("Download")
	tr.ApplyUpdates()
	if told != 2 {
		t.Errorf("told %d times after the label's Value changed, want 2", told)
	}
	check("Download", 42, "42 percent")
	pct.Set(43)
	tr.ApplyUpdates()
	if told != 3 {
		t.Errorf("told %d times after the value changed again, want 3", told)
	}
	if builds != 1 {
		t.Errorf("%d builds, want 1", builds)
	}

	// Without a text, the value is the number; NaN is no value.
	var v *Value[float64]
	tr.Mount(Component(func() Node {
		v = UseValue(7.5)
		return div(div(Role("slider"), AriaLabel("Gain"), AriaValueNowFn(v.Get)))
	}))
	tr.Frame()
	if n := mustNode(t, tr, "slider", "Gain"); n.Range.Now != 7.5 || n.Value != "7.5" {
		t.Errorf("slider: range %+v, value %q", n.Range, n.Value)
	}
	v.Set(math.NaN())
	tr.ApplyUpdates()
	if n := mustNode(t, tr, "slider", "Gain"); n.Range.HasNow || n.Value != "" {
		t.Errorf("NaN: range %+v, value %q; want no value", n.Range, n.Value)
	}

	// The static props replace the Fns, and the other way around.
	tr.Mount(div(
		div(Role("slider"), AriaLabel("A"), AriaValueNowFn(func() float64 { return 1 }), AriaValueNow(2)),
		div(Role("slider"), AriaLabel("B"), AriaValueNow(2), AriaValueNowFn(func() float64 { return 3 })),
		div(Role("slider"), AriaLabel("C"), AriaValueTextFn(func() string { return "fn" }), AriaValueText("fixed")),
		div(Role("slider"), AriaLabel("D"), AriaValueText("fixed"), AriaValueTextFn(func() string { return "fn" }))))
	tr.Frame()
	for _, c := range []struct {
		name, value string
	}{{"A", "2"}, {"B", "3"}, {"C", "fixed"}, {"D", "fn"}} {
		if n := mustNode(t, tr, "slider", c.name); n.Value != c.value {
			t.Errorf("slider %s: value %q, want %q", c.name, n.Value, c.value)
		}
	}
}
