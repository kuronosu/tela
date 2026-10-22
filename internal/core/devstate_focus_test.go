//go:build teladev

package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// Under tela dev the window's focus event reaches the new build before its
// first frame (the host sends it right after the configure event), when the
// restored focus has not been styled yet: the focus fix-up that comes with
// the window gaining focus must not take it away (ADR-113, ADR-114).
func TestDevStateFocusSurvivesWindowFocusBeforeFirstFrame(t *testing.T) {
	root := Component(devPage)
	a := newKeys(t, root, 400, 300)
	a.frame()
	a.click(a.center("in"))
	devRest(a)
	if got := focusedName(a.tree()); got != "in" {
		t.Fatalf("setup: focus %q, want in", got)
	}
	data, _ := devSnap(t, a.tree())

	tr := NewTree(Config{Fonts: testFonts, Dev: true})
	tr.SetWindowFocused(false) // NewHost: the window has no focus until its first event
	tr.SeedDevState(data)
	tr.Mount(root)
	// As Host does: the configure event, then the window's focus, before
	// any frame.
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.SetScale(1)
	tr.SetWindowFocused(true)
	devFrames(tr)
	if got := focusedName(tr); got != "in" {
		t.Errorf("focus %q after the window focus and the first frames, want in", got)
	}
	if !tr.foc.window {
		t.Error("the window is not focused")
	}
}
