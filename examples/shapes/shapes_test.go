package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// windowW and windowH are the size of the window of main (and of the goldens).
const windowW, windowH = 960, 640

// mount renders Preview like `tela snapshot` does, and returns the tree and
// the dev messages (class errors, missing keys) it logged.
func mount(w, h, scale float32) (*core.Tree, []string) {
	var logs []string
	tree := core.NewTree(core.Config{Dev: true, Log: func(m string) { logs = append(logs, m) }})
	tree.SetViewport(gfx.Size{W: w, H: h})
	tree.SetScale(scale)
	tree.Mount(core.Component(Preview))
	tree.Frame()
	return tree, logs
}

// TestNoClassErrors fails on an unknown or unsupported class, or any other
// dev warning, at several window sizes: core reports them through its log
// once per distinct class string.
func TestNoClassErrors(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {700, 500}, {1280, 800}} {
		_, logs := mount(s[0], s[1], 1)
		for _, m := range logs {
			t.Errorf("%vx%v: %s", s[0], s[1], m)
		}
	}
}

// cards returns the 12 cards of the layout: the page has the two glows and
// the grid, whose three rows hold four cells with one card each.
func cards(t testing.TB, tree *core.Tree) [3][4]core.InspectNode {
	t.Helper()
	root := tree.Inspect()
	if len(root.Children) != 1 || len(root.Children[0].Children) != 3 {
		t.Fatalf("unexpected page: %d root children", len(root.Children))
	}
	grid := root.Children[0].Children[2]
	if len(grid.Children) != 3 {
		t.Fatalf("grid has %d rows, want 3", len(grid.Children))
	}
	var out [3][4]core.InspectNode
	for i, r := range grid.Children {
		if len(r.Children) != 4 {
			t.Fatalf("row %d has %d cells, want 4", i, len(r.Children))
		}
		for j, c := range r.Children {
			if len(c.Children) != 1 {
				t.Fatalf("cell %d.%d has %d children, want its card", i, j, len(c.Children))
			}
			out[i][j] = c.Children[0]
		}
	}
	return out
}

// TestGridLayout checks that the 4x3 grid of flex rows gives every card the
// same size, as the grid of the phase-1 scene did: 202 x 170.67 at 960x640
// (p-10 and gap-6 around and between the cards).
func TestGridLayout(t *testing.T) {
	tree, _ := mount(windowW, windowH, 1)
	const wantW, wantH = 202, 512.0 / 3
	for i, row := range cards(t, tree) {
		for j, c := range row {
			w, h := float64(c.Rect.Width()), float64(c.Rect.Height())
			if math.Abs(w-wantW) > 0.02 || math.Abs(h-wantH) > 0.02 {
				t.Errorf("card %d.%d is %.3f x %.3f, want %d x %.3f", i, j, w, h, wantW, wantH)
			}
			if x, y := float64(c.Rect.X0), float64(c.Rect.Y0); math.Abs(x-(40+float64(j)*226)) > 0.02 || math.Abs(y-(40+float64(i)*(wantH+24))) > 0.02 {
				t.Errorf("card %d.%d is at (%.3f, %.3f)", i, j, x, y)
			}
		}
	}
}

// TestCardsStayInsideTheWindow checks the grid at other window sizes: the
// cards stretch with the window and never leave it.
func TestCardsStayInsideTheWindow(t *testing.T) {
	for _, s := range [][2]float32{{700, 500}, {1280, 800}, {1600, 500}} {
		tree, _ := mount(s[0], s[1], 1)
		for i, row := range cards(t, tree) {
			for j, c := range row {
				r := c.Rect
				if r.X0 < 0 || r.Y0 < 0 || r.X1 > s[0] || r.Y1 > s[1] || r.Width() <= 0 || r.Height() <= 0 {
					t.Errorf("%vx%v: card %d.%d is at %v", s[0], s[1], i, j, r)
				}
			}
		}
	}
}

// TestGolden is the render of Preview at the size of the window, at scale 1
// and 2, bit by bit.
func TestGolden(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			tree, _ := mount(windowW, windowH, scale)
			dl, _ := tree.Frame()
			golden.Image(t, fmt.Sprintf("shapes_x%g", scale), tree.Rasterize(dl, scale))
		})
	}
}
