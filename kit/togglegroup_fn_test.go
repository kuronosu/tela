package kit_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The deferred forms of ToggleGroup (ADR-250): ValueFn and SelectedFn,
// read when the group builds, and the Indicator slot.

// fnGroup holds the choice in a tela.Value that only the group and the
// viewContent read: builds counts its own builds.
type fnGroup struct {
	builds    *int
	view      **tela.Value[string]
	indicator tela.Component
	itemClass string
}

func (p fnGroup) Build() tela.Node {
	*p.builds++
	view := tela.UseValue("grid")
	*p.view = view
	return h.Div(
		kit.ToggleGroup[string]{Items: tgViews, AriaLabel: "View", ValueFn: view.Get, OnValueChange: view.Set,
			DisallowEmptySelection: true, Indicator: p.indicator, ItemClass: p.itemClass},
		viewContent{View: view})
}

// viewContent shows the view: it reads the Value in its build.
type viewContent struct{ View *tela.Value[string] }

func (c viewContent) Build() tela.Node {
	return h.Span(h.Class("sr-only"), h.Text("view is "+c.View.Get()))
}

// TestToggleGroupValueFn: the group shows the Value's key, a click sets
// the Value through OnValueChange, and a change of the Value (from the
// click, or from elsewhere) rebuilds the group and the content that reads
// it, not the component that holds it.
func TestToggleGroupValueFn(t *testing.T) {
	builds := 0
	var view *tela.Value[string]
	app := telatest.Mount(t, fnGroup{builds: &builds, view: &view})
	app.Expect(tgRadio("Grid"), telatest.Checked(true))
	app.ExpectText("view is grid")
	b0, s0 := builds, app.Stats()
	app.Click(tgRadio("Table"))
	app.Expect(tgRadio("Table"), telatest.Checked(true))
	app.Expect(tgRadio("Grid"), telatest.Checked(false))
	app.ExpectText("view is table")
	view.Set("list")
	app.Expect(tgRadio("List"), telatest.Checked(true))
	app.ExpectText("view is list")
	s1 := app.Stats()
	if builds != b0 {
		t.Errorf("the component that holds the Value built %d times, want 0", builds-b0)
	}
	// The group and the content for each change (a toggle group is one
	// component: its items are elements), and the group once more for the
	// press.
	if d := s1.Builds - s0.Builds; d < 4 || d > 5 {
		t.Errorf("%d builds for two changes, want the group and the content each time", d)
	}
	// A key that is not an item's turns none on.
	view.Set("none")
	app.Expect(tgRadio("List"), telatest.Checked(false))
	app.Expect(tgRadio("Grid"), telatest.Checked(false))
}

// TestToggleGroupIndicator: the Indicator is mounted inside the item that
// is on only, follows the choice, and with a LayoutID slides there with
// frames that only composite.
func TestToggleGroupIndicator(t *testing.T) {
	builds := 0
	var view *tela.Value[string]
	pill := h.Div(h.Class("absolute inset-0 -z-10 rounded-md bg-sky-500"), h.LayoutID("view-pill"))
	// The items' color transition is off, so that the frames of the slide
	// are the indicator's alone.
	app := telatest.Mount(t, fnGroup{builds: &builds, view: &view, indicator: pill, itemClass: "transition-none"}, telatest.NoSettle(), telatest.Size(240, 48))
	loc := telatest.ByClass("bg-sky-500")
	app.ExpectCount(loc, 1)
	inside := func(item string) {
		t.Helper()
		p, r := app.Bounds(loc), app.Bounds(tgRadio(item))
		if p != r {
			t.Errorf("the indicator is at %v, want the %s item's box %v", p, item, r)
		}
	}
	inside("Grid")
	// Behind the label: the label's text is drawn over the pill.
	if n := app.Count(telatest.ByClass("relative", "isolate")); n != len(tgViews) {
		t.Errorf("%d items have the recipe's ItemWithIndicator, want %d", n, len(tgViews))
	}
	app.Click(tgRadio("Table"))
	app.ExpectCount(loc, 1)
	// The slide: from Grid toward Table, with no build or layout after its
	// first frame.
	b0 := builds
	app.Advance(16 * time.Millisecond)
	s0 := app.Stats()
	app.Advance(100 * time.Millisecond)
	grid, table := app.Bounds(tgRadio("Grid")), app.Bounds(tgRadio("Table"))
	mid := app.Bounds(loc)
	if !(mid.X > grid.X && mid.X < table.X) {
		t.Errorf("100 ms into the slide the indicator is at x %g, want between Grid (%g) and Table (%g)", mid.X, grid.X, table.X)
	}
	app.Golden("togglegroup_indicator_slide.png")
	app.Advance(time.Second)
	if builds != b0 {
		t.Errorf("the holder of the Value built %d times", builds-b0)
	}
	if s1 := app.Stats(); s1.Builds != s0.Builds || s1.Layouts != s0.Layouts || s1.Paints == s0.Paints {
		t.Errorf("the slide: %d builds, %d layouts, %d paints; want only paints",
			s1.Builds-s0.Builds, s1.Layouts-s0.Layouts, s1.Paints-s0.Paints)
	}
	inside("Table")
	app.Golden("togglegroup_indicator.png")
}

// mountCounter is an item's content that counts its mounts.
type mountCounter struct {
	label  string
	mounts *int
}

func (m mountCounter) Build() tela.Node {
	n := m.mounts
	tela.UseMount(func() func() { *n++; return nil })
	return h.Span(h.Text(m.label))
}

// The Indicator comes and goes without remounting the items' content.
func TestToggleGroupIndicatorKeepsContent(t *testing.T) {
	mounts := 0
	items := []kit.ToggleGroupItem[string]{
		{Value: "a", Children: mountCounter{"A", &mounts}, AriaLabel: "A"},
		{Value: "b", Children: mountCounter{"B", &mounts}, AriaLabel: "B"},
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return kit.ToggleGroup[string]{Items: items, AriaLabel: "Pick", DefaultValue: new("a"), DisallowEmptySelection: true,
			Indicator: h.Div(h.Class("absolute inset-0 -z-10 bg-sky-500"))}
	})
	app.Click(tgRadio("B"))
	app.Expect(tgRadio("B"), telatest.Checked(true))
	app.Click(tgRadio("A"))
	app.Expect(tgRadio("A"), telatest.Checked(true))
	if mounts != 2 {
		t.Errorf("the items' content mounted %d times, want 2 (once each)", mounts)
	}
}

// TestToggleGroupSelectedFn: the deferred selection of a group with
// Multiple; the zero Selection is the empty one.
func TestToggleGroupSelectedFn(t *testing.T) {
	var sel *tela.Value[aria.Selection[string]]
	app := telatest.MountFunc(t, func() tela.Node {
		sel = tela.UseValue(aria.SelectionOf("bold"))
		return kit.ToggleGroup[string]{Multiple: true, AriaLabel: "Format", SelectedFn: sel.Get, OnSelectionChange: sel.Set,
			Indicator: h.Span(h.Class("absolute inset-x-1 bottom-0 h-0.5 bg-sky-500")),
			Items: []kit.ToggleGroupItem[string]{
				{Value: "bold", Icon: iconBold, AriaLabel: "Bold"},
				{Value: "italic", Icon: iconItalic, AriaLabel: "Italic"}}}
	})
	app.Expect(toggleButton("Bold"), telatest.Pressed(true))
	app.Click(toggleButton("Italic"))
	app.Expect(toggleButton("Italic"), telatest.Pressed(true))
	if got := fmt.Sprint(sel.Get().Keys()); got != "[bold italic]" {
		t.Errorf("selection %s", got)
	}
	app.ExpectCount(telatest.ByClass("bg-sky-500"), 2) // every item that is on
	sel.Set(aria.Selection[string]{})
	app.Expect(toggleButton("Bold"), telatest.Pressed(false))
	app.Expect(toggleButton("Italic"), telatest.Pressed(false))
	app.ExpectCount(telatest.ByClass("bg-sky-500"), 0)
}

func TestToggleGroupTwoControlledFormsPanic(t *testing.T) {
	_, err := telatest.AriaSnapshot(kit.ToggleGroup[string]{Items: tgViews, Value: new("list"), ValueFn: func() string { return "grid" }})
	if err == nil || !strings.Contains(err.Error(), "one of Value, ValueFn") {
		t.Errorf("error %v, want the Value and ValueFn conflict reported", err)
	}
	_, err = telatest.AriaSnapshot(kit.ToggleGroup[string]{Items: tgViews, ValueFn: func() string { return "grid" },
		SelectedFn: func() aria.Selection[string] { return aria.SelectionOf("list") }})
	if err == nil || !strings.Contains(err.Error(), "one of Value, ValueFn") {
		t.Errorf("error %v, want the ValueFn and SelectedFn conflict reported", err)
	}
}
