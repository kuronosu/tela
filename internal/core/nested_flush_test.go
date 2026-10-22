package core

import (
	"fmt"
	"testing"
)

// nestedProbe is a page whose overlay clicks a counter button
// (ElementRef.Click) from each of its callbacks, and records the builds
// of the counter, to tell whether the click's updates were applied and
// built inside the callback (synchronously, mid-event) or by the event
// that is running.
type nestedProbe struct {
	r   recorder
	btn ElementRef
}

func (o *nestedProbe) app() Node {
	counter := func() Node {
		n, set := UseState(0)
		if n > 0 {
			o.r.add("build")
		}
		return button(Key("count"), AttachRef(o.btn), Class("block h-10 w-20"),
			On("click", func(MouseEvent) { o.r.add("click"); set.Update(func(n int) int { return n + 1 }) }),
			Text("count"))
	}
	clickIn := func(name string) {
		o.btn.Click()
		o.r.add("after:" + name)
	}
	comp := func() Node {
		o.btn = UseElementRef()
		return div(Key("page"), Class("h-full"),
			Component(counter),
			button(Key("other"), Class("block h-10 w-20"), o.r.pointer("pointerdown", "other"), Text("other")),
			Portal(div(Key("pop"), Class("fixed top-[100px] left-[200px] size-20"),
				InteractOutside(OutsideOptions{
					OnStart: func(PointerEvent) { clickIn("start") },
					OnEnd:   func(PointerEvent) { clickIn("end") },
				}),
				Overlay(OverlayOptions{
					OnInteractOutsideStart: func(PointerEvent) { clickIn("overlay-start") },
					OnInteractOutside:      func(PointerEvent) { clickIn("overlay-outside") },
					OnEscape:               func(KeyboardEvent) { clickIn("escape") },
					OnFocusOutside:         func() { clickIn("focus-outside") },
				}),
				button(Key("inside"), Class("block size-10"), Text("in")))))
	}
	return Component(comp)
}

// TestOverlayCallbacksDeferNestedFlush: the interact-outside, Escape and
// focus-outside callbacks run like handlers, so a click they make
// (ElementRef.Click) leaves applying its updates and building to the
// event that is running: the pointerdown reaches its target before the
// build, and each callback runs to its end first.
func TestOverlayCallbacksDeferNestedFlush(t *testing.T) {
	o := &nestedProbe{}
	x := newKeys(t, o.app(), 400, 300)
	// A press outside the popover, on "other" (y 40..80).
	x.send(move(10, 50), down(10, 50))
	if g, want := o.r.String(), "click after:start click after:overlay-start pointerdown:other build"; g != want {
		t.Errorf("pointerdown outside:\n got %q\nwant %q", g, want)
	}
	o.r.reset()
	x.send(up(10, 50))
	if g, want := o.r.String(), "click after:end click after:overlay-outside build"; g != want {
		t.Errorf("pointerup outside:\n got %q\nwant %q", g, want)
	}
	o.r.reset()
	x.escape()
	if g, want := o.r.String(), "click after:escape build"; g != want {
		t.Errorf("Escape:\n got %q\nwant %q", g, want)
	}
	// The focus moves from inside the popover to an element outside it.
	x.frame()
	inX, inY := x.center("inside")
	x.click(inX, inY)
	if f := focusedName(x.tree()); f != "inside" {
		t.Fatalf("focused %q, want inside", f)
	}
	o.r.reset()
	x.tab() // wraps to the first button, outside the popover
	if g, want := o.r.String(), "click after:focus-outside build"; g != want {
		t.Errorf("focus leaving the overlay:\n got %q\nwant %q", g, want)
	}
}

// TestEffectClickDefersFlush: a passive effect that clicks
// (ElementRef.Click), run by the host after the frame, leaves applying
// the click's updates and building to the host's next step instead of
// building inside the commit of the effects.
func TestEffectClickDefersFlush(t *testing.T) {
	var r recorder
	var btn ElementRef
	counter := func() Node {
		n, set := UseState(0)
		r.add(fmt.Sprintf("build:%d", n))
		return button(Key("count"), AttachRef(btn), Class("block h-10 w-20"),
			On("click", func(MouseEvent) { r.add("click"); set.Update(func(n int) int { return n + 1 }) }),
			Textf("%d", n))
	}
	clicker := func() Node {
		started, setStarted := UseState(false)
		UseEffect(func() func() {
			if started {
				btn.Click()
				r.add("after-click")
			}
			return nil
		}, started)
		return button(Key("go"), Class("block h-10 w-20"), On("click", func(MouseEvent) { setStarted.Set(true) }), Text("go"))
	}
	root := Component(func() Node {
		btn = UseElementRef()
		return div(Component(counter), Component(clicker))
	})
	x := newInput(t, root, 300, 200)
	r.reset()
	x.click(10, 50) // "go": its effect runs after the next frame
	x.frame()
	x.frame()
	if g, want := r.String(), "click after-click build:1"; g != want {
		t.Errorf("an effect that clicks:\n got %q\nwant %q", g, want)
	}
}
