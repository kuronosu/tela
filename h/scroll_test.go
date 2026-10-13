package h_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// openWith is open with tela.App's scrolling options.
func openWith(t *testing.T, root func() tela.Node, cfg core.Config) *app {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}})
	h, err := core.NewHost(b, core.Options{Config: cfg, Root: tela.C(root)})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(h)
	return &app{t: t, b: b, h: h, w: h.Window().(*headless.Window)}
}

func (a *app) settle() {
	a.b.Drain(a.h)
	for i := 0; i < 200 && a.w.FrameRequested(); i++ {
		a.frame()
	}
}

func (a *app) wheel(x, y, steps float32) {
	a.w.Inject(platform.ScrollEvent{Pos: platform.Point{X: x, Y: y}, Steps: platform.Point{Y: steps}})
	a.b.Drain(a.h)
}

// TestScrollingAPI drives the public scrolling API: OnWheel, OnScroll,
// UseScroll in a paint closure, ElementRef's scroll methods, UseCallback
// with the new event types, and App.InstantScroll.
func TestScrollingAPI(t *testing.T) {
	var (
		list      tela.ElementRef
		wheels    []tela.WheelEvent
		scrolls   []tela.ScrollEvent
		opacities []float32
		builds    int
	)
	root := func() tela.Node {
		builds++
		list = tela.UseElementRef()
		_, y := tela.UseScroll(list)
		onWheel := tela.UseCallback(func(e tela.WheelEvent) { wheels = append(wheels, e) })
		onScroll := tela.UseCallback(func(e tela.ScrollEvent) { scrolls = append(scrolls, e) })
		rows := make([]tela.Child, 20)
		for i := range rows {
			rows[i] = Div(Key(i), Class("h-[50px]"), Textf("Row %d", i))
		}
		return Div(Class("flex h-screen flex-col"),
			Div(Class("h-[20px] bg-sky-500"), Style(tela.OpacityFn(func() float32 {
				o := 1 - min(y.Get()/200, 1)
				opacities = append(opacities, o)
				return o
			}))),
			Div(Ref(list), Class("h-[200px] overflow-y-auto"), OnWheel(onWheel.Fn()), OnScroll(onScroll.Fn()),
				Fragment(rows...)),
		)
	}
	a := openWith(t, root, core.Config{InstantScroll: true})
	b0 := builds
	a.wheel(50, 100, 1)
	a.settle()
	if len(wheels) != 1 || wheels[0].DeltaY != 53 {
		t.Fatalf("wheel events %+v", wheels)
	}
	if _, y := list.ScrollOffset(); y != 53 {
		t.Errorf("offset %g after a notch", y)
	}
	if len(scrolls) != 1 || scrolls[0].ScrollTop != 53 || scrolls[0].ScrollHeight != 1000 || scrolls[0].ClientHeight != 200 {
		t.Errorf("scroll events %+v", scrolls)
	}
	if o := opacities[len(opacities)-1]; o != 1-53.0/200 {
		t.Errorf("opacity %g", o)
	}
	if builds != b0 {
		t.Errorf("%d builds while scrolling", builds-b0)
	}
	list.ScrollTo(0, 1e6)
	a.settle()
	if _, y := list.ScrollOffset(); y != 800 {
		t.Errorf("ScrollTo clamps: %g", y)
	}
	list.ScrollBy(0, -100)
	if _, y := list.ScrollOffset(); y != 700 {
		t.Errorf("ScrollBy: %g", y)
	}
	a.settle()
	if o := opacities[len(opacities)-1]; o != 0 {
		t.Errorf("opacity %g at the end", o)
	}
	if got := fmt.Sprint(scrolls[len(scrolls)-1].ScrollTop); got != "700" {
		t.Errorf("last scroll event at %s", got)
	}
}

// TestScrollIntoViewAndClassic checks ScrollIntoView through h.Ref and
// classic scrollbars from App.ClassicScrollbars.
func TestScrollIntoViewAndClassic(t *testing.T) {
	var item, list tela.ElementRef
	root := func() tela.Node {
		item, list = tela.UseElementRef(), tela.UseElementRef()
		return Div(Ref(list), Class("h-[200px] w-[200px] overflow-y-auto"),
			Div(Class("h-[500px]")),
			Div(Ref(item), Class("h-[20px]")),
		)
	}
	a := openWith(t, root, core.Config{ClassicScrollbars: true})
	item.ScrollIntoView()
	a.settle()
	in := a.h.Tree().Inspect()
	box := in.Children[0]
	if w := box.Children[0].Rect.Width(); w != 200-15 {
		t.Errorf("content width %g with a classic gutter, want 185", w)
	}
	// The item (500..520) is aligned with the bottom edge.
	if _, y := list.ScrollOffset(); y != 320 {
		t.Errorf("ScrollIntoView: offset %g, want 320", y)
	}
	before := a.w.Presents()
	item.ScrollIntoView()
	a.settle()
	if a.w.Presents() != before {
		t.Error("ScrollIntoView of a visible element scrolled again")
	}
}
