package remote

import (
	"image"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TestTransparentForwarded (protocol 10): the option reaches the real
// window, its Transparent state reaches the child, the real window's input
// region follows the frames the child presents, and a compositing manager
// that stops reaches the child too.
func TestTransparentForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Title: "glass", Frameless: true, Transparent: true})
	hw := p.hostWindow("glass")
	if o := hw.Options(); !o.Transparent || !o.Frameless {
		t.Errorf("the real window's options %+v, want Frameless and Transparent", o)
	}
	want := platform.WindowState{ClientSideDecorations: true, Transparent: true}
	a.waitEvent("the transparent state", func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State == want
	})
	a.waitEvent("a frame", isType[platform.FrameEvent])
	fb := a.presentNow(w, 1, nil)
	waitFrame(t, hw, "the child's frame", fb)
	if got, want := hw.InputRects(), []image.Rectangle{fb.Rect}; !slices.Equal(got, want) {
		t.Errorf("the real window's input region %v, want %v (an opaque frame)", got, want)
	}

	hw.SetCompositing(false)
	a.waitEvent("the opaque state", func(e platform.Event) bool {
		se, ok := e.(platform.WindowStateEvent)
		return ok && se.State == platform.WindowState{ClientSideDecorations: true}
	})
	var st platform.WindowState
	a.ui(func() { st = w.State() })
	if st.Transparent {
		t.Errorf("the child window's state %v after compositing stopped", st)
	}
}
