package core

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// AriaDescribedBy describes an element with the text of another.
func TestAriaDescribedBy(t *testing.T) {
	root := Component(func() Node {
		tip := UseElementRef()
		return div(
			button(AriaDescribedBy(tip), Text("Save")),
			button(AriaDescribedBy(tip), AriaDescription("own"), Text("Print")),
			div(AttachRef(tip), Role("tooltip"), Text("Saves  the file")))
	})
	tr := newTestTree(t, 300, 200, root, nil)
	tr.Frame()
	if n := mustNode(t, tr, "button", "Save"); n.Description != "Saves the file" {
		t.Errorf("description %q, want the tooltip's text", n.Description)
	}
	if n := mustNode(t, tr, "button", "Print"); n.Description != "own" {
		t.Errorf("AriaDescription must win: %q", n.Description)
	}
	if msg := panicMessage(func() { AriaDescribedBy(ElementRef{}) }); !strings.Contains(msg, "zero ElementRef") {
		t.Errorf("panic %q", msg)
	}
}

// Two different refs on one element panic in dev mode, naming both sites.
func TestTwoRefsOnOneElement(t *testing.T) {
	a, b := new(ElementRef), new(ElementRef)
	root := Component(func() Node {
		*a, *b = UseElementRef(), UseElementRef()
		return div(AttachRef(*a), AttachRef(*b))
	})
	mount := func(dev bool) string {
		return panicMessage(func() {
			tr := NewTree(Config{Fonts: testFonts, Dev: dev})
			tr.SetViewport(gfx.Size{W: 100, H: 100})
			tr.Mount(root)
			tr.Frame()
		})
	}
	msg := mount(true)
	if msg == "" || !strings.Contains(msg, "two different refs") || !strings.Contains(msg, "engine_overlay_test.go") {
		t.Errorf("panic %q", msg)
	}
	if msg := mount(false); msg != "" {
		t.Errorf("outside dev mode the last ref wins, but it panicked: %q", msg)
	}
}

// A Timeout runs its function once on the frame clock, replaces a pending
// run when started again, and stops with Stop.
func TestTimeout(t *testing.T) {
	var to Timeout
	var ran []string
	root := Component(func() Node {
		to = UseTimeout()
		return div(button(Text("x")))
	})
	tr := newTestTree(t, 100, 100, root, nil)
	tr.Frame()
	if _, ok := tr.NextDeadline(); ok {
		t.Fatal("an idle timeout has a deadline")
	}
	start := tr.clock()
	to.Start(100*time.Millisecond, func() { ran = append(ran, "first") })
	to.Start(300*time.Millisecond, func() { ran = append(ran, "second") })
	if at, ok := tr.NextDeadline(); !ok || at != start+300*time.Millisecond || !to.Pending() {
		t.Errorf("deadline %v %v pending %v, want %v", at, ok, to.Pending(), start+300*time.Millisecond)
	}
	tr.TickGestures(start + 200*time.Millisecond)
	if len(ran) != 0 {
		t.Errorf("ran %v early", ran)
	}
	tr.TickGestures(start + 400*time.Millisecond)
	tr.TickGestures(start + 900*time.Millisecond)
	if len(ran) != 1 || ran[0] != "second" || to.Pending() {
		t.Errorf("ran %v, pending %v; want [second], false", ran, to.Pending())
	}
	if _, ok := tr.NextDeadline(); ok {
		t.Error("a finished timeout has a deadline")
	}
	to.Start(time.Second, func() { ran = append(ran, "stopped") })
	to.Stop()
	tr.TickGestures(start + 5*time.Second)
	if len(ran) != 1 {
		t.Errorf("a stopped timeout ran: %v", ran)
	}
}

// TreeLocal is one value per tree, made on first use.
func TestTreeLocal(t *testing.T) {
	var key byte
	made := 0
	var got []*int
	root := Component(func() Node {
		v := TreeLocal(&key, func() *int { made++; return new(int) })
		got = append(got, v)
		return div()
	})
	a := newTestTree(t, 100, 100, root, nil)
	a.Frame()
	b := newTestTree(t, 100, 100, root, nil)
	b.Frame()
	if made != 2 || len(got) != 2 || got[0] == got[1] {
		t.Errorf("made %d values for 2 trees: %v", made, got)
	}
}
