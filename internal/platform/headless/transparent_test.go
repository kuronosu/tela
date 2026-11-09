package headless

import (
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestTransparentWindowPassesInput: pointer events over the pixels a
// Transparent window presents (nearly) transparent are not delivered;
// crossing the mask's edge is a leave and an enter; a held button keeps
// the sequence.
func TestTransparentWindowPassesInput(t *testing.T) {
	b := New(Options{Size: platform.Size{W: 100, H: 80}, Scale: 2})
	w := newWindow(t, b, platform.WindowOptions{Title: "w", Frameless: true, Transparent: true})
	r := &recorder{}
	expectEvents(t, b, r, "w:configure(100x80@2x (200x160 px)) w:state{csd transparent} w:frame(0s)")
	if !w.State().Transparent {
		t.Fatal("State().Transparent is false")
	}
	fb := image.NewRGBA(image.Rect(0, 0, 200, 160))
	fill(fb, image.Rect(40, 40, 160, 120), color.RGBA{R: 200, G: 200, B: 200, A: 255}) // the card: 20,20 to 80,60
	fill(fb, image.Rect(40, 120, 160, 140), color.RGBA{A: 30})                         // its shadow
	if err := w.Present(fb, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := w.InputRects(), []image.Rectangle{image.Rect(40, 40, 160, 120)}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("InputRects = %v, want %v", got, want)
	}
	if !w.TakesInput(platform.Point{X: 50, Y: 30}) || w.TakesInput(platform.Point{X: 50, Y: 65}) {
		t.Fatal("TakesInput disagrees with the frame")
	}
	at := func(action platform.PointerAction, button platform.PointerButton, x, y float32) {
		w.Inject(platform.PointerEvent{Action: action, Button: button, Pos: platform.Point{X: x, Y: y}})
	}
	at(platform.PointerMove, platform.ButtonNone, 50, 30)
	at(platform.PointerMove, platform.ButtonNone, 50, 65) // over the shadow: below
	at(platform.PointerDown, platform.ButtonPrimary, 50, 65)
	at(platform.PointerUp, platform.ButtonPrimary, 50, 65)
	at(platform.PointerMove, platform.ButtonNone, 50, 40) // back on the card
	expectEvents(t, b, r, "w:pointer(move 50,30) w:pointer(leave 50,65) w:pointer(enter 50,40) w:pointer(move 50,40)")
	// A drag that starts on the card follows the pointer off it.
	at(platform.PointerDown, platform.ButtonPrimary, 50, 40)
	at(platform.PointerMove, platform.ButtonNone, 90, 75)
	at(platform.PointerUp, platform.ButtonPrimary, 90, 75)
	expectEvents(t, b, r, "w:pointer(down 50,40) w:pointer(move 90,75) w:pointer(up 90,75)")

	// Without compositing the window is opaque and takes every event.
	w.SetCompositing(false)
	expectEvents(t, b, r, "w:state{csd}")
	if !w.TakesInput(platform.Point{X: 1, Y: 1}) {
		t.Fatal("an opaque window lets input through")
	}
}

func TestNoCompositingTransparentWindow(t *testing.T) {
	b := New(Options{NoCompositing: true})
	w := newWindow(t, b, platform.WindowOptions{Frameless: true, Transparent: true})
	if w.State().Transparent {
		t.Fatal("a window without compositing reports Transparent")
	}
	w.SetCompositing(true)
	b.Drain(&recorder{})
	if !w.State().Transparent {
		t.Fatal("SetCompositing(true) left the window opaque")
	}
}
