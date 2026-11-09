package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/telatest"
)

// TestOverlayOverTransparentWindow (ADR-257): over a transparent window's
// frame (a card floating on transparent pixels) the error panel stays
// opaque and readable, the transparent pixels away from the panel stay
// transparent (the docked panel's), and the full panel's dimming is
// translucent, not an opaque black sheet.
func TestOverlayOverTransparentWindow(t *testing.T) {
	size := platform.Size{W: 480, H: 360}
	root := h.Div(h.Class("flex h-full items-center justify-center p-10"),
		h.Div(h.Class("w-64 h-40 rounded-2xl bg-white shadow-xl")))
	for _, scale := range []float64{1, 2} {
		frame, err := telatest.Render(root, telatest.Size(size.W, size.H), telatest.Scale(scale), telatest.Transparent())
		if err != nil {
			t.Fatal(err)
		}
		if a := frame.RGBAAt(1, 1).A; a != 0 {
			t.Fatalf("the frame's corner has alpha %d, want a transparent frame", a)
		}
		for _, c := range []struct {
			name string
			p    problem
		}{
			{"docked", problem{kind: problemBuild, summary: "build failed", text: compilerOutput, live: true}},
			{"full", problem{kind: problemPanic, summary: "the app panicked", text: panicText}},
		} {
			t.Run(fmt.Sprintf("%s_x%g", c.name, scale), func(t *testing.T) {
				img, g := drawOverlay(t, c.p, frame, size, scale)
				px := func(x, y float32) image.Point { return image.Pt(int(x*float32(scale)), int(y*float32(scale))) }
				mid := px((g.panel.X0+g.panel.X1)/2, (g.panel.Y0+g.panel.Y1)/2)
				if a := img.RGBAAt(mid.X, mid.Y).A; a != 255 {
					t.Errorf("the panel's middle has alpha %d, want an opaque panel", a)
				}
				corner := img.RGBAAt(1, 1).A
				if c.name == "docked" && corner != 0 {
					t.Errorf("the window's top corner has alpha %d under a docked panel, want 0", corner)
				}
				if c.name == "full" && (corner == 0 || corner == 255) {
					t.Errorf("the dimmed corner has alpha %d, want translucent", corner)
				}
				golden.Image(t, fmt.Sprintf("overlay/transparent_%s_x%g", c.name, scale), img)
			})
		}
	}
}
