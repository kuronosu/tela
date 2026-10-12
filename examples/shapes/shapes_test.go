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
	for _, s := range [][2]float32{{windowW, windowH}, {701, 846}, {700, 500}, {1280, 800}} {
		_, logs := mount(s[0], s[1], 1)
		for _, m := range logs {
			t.Errorf("%vx%v: %s", s[0], s[1], m)
		}
	}
}

// cards returns the 12 cards: the page holds the two glows and the wrapping
// container, whose cells hold one card each.
func cards(t testing.TB, tree *core.Tree) []core.InspectNode {
	t.Helper()
	root := tree.Inspect()
	if len(root.Children) != 1 || len(root.Children[0].Children) != 3 {
		t.Fatalf("unexpected page: %d root children", len(root.Children))
	}
	grid := root.Children[0].Children[2]
	if len(grid.Children) != 12 {
		t.Fatalf("the page has %d cells, want 12", len(grid.Children))
	}
	out := make([]core.InspectNode, 12)
	for i, c := range grid.Children {
		if len(c.Children) != 1 {
			t.Fatalf("cell %d has %d children, want its card", i, len(c.Children))
		}
		out[i] = c.Children[0]
	}
	return out
}

// TestGridLayout checks the wrapping grid: every card is 202 x 170, four
// columns at 960x640 (as the grid of the phase-1 scene) and three in a
// tiled window of 701x846 (Hyprland's default tile on a 1440x900 screen).
func TestGridLayout(t *testing.T) {
	for _, tc := range []struct {
		w, h, cols float32
	}{{windowW, windowH, 4}, {701, 846, 3}} {
		tree, _ := mount(tc.w, tc.h, 1)
		cs := cards(t, tree)
		x0 := cs[0].Rect.X0
		for i, c := range cs {
			if w, h := c.Rect.Width(), c.Rect.Height(); w != 202 || h != 170 {
				t.Errorf("%vx%v: card %d is %v x %v, want 202 x 170", tc.w, tc.h, i, w, h)
			}
			col := float32(i % int(tc.cols))
			if x := c.Rect.X0; math.Abs(float64(x-(x0+col*226))) > 0.01 {
				t.Errorf("%vx%v: card %d at x = %v, want column %v", tc.w, tc.h, i, x, col)
			}
		}
	}
}

// TestContentStaysInsideCards checks, at several window sizes, that the
// cards stay in the window and that no text or button sticks out of its
// card (decorations may: the cards clip them).
func TestContentStaysInsideCards(t *testing.T) {
	const eps = 0.01
	inside := func(r, in gfx.Rect) bool {
		return r.X0 >= in.X0-eps && r.Y0 >= in.Y0-eps && r.X1 <= in.X1+eps && r.Y1 <= in.Y1+eps
	}
	var walk func(t *testing.T, n core.InspectNode, card gfx.Rect, path string)
	walk = func(t *testing.T, n core.InspectNode, card gfx.Rect, path string) {
		if (n.Text != "" || n.Kind == "button") && !inside(n.Rect, card) {
			t.Errorf("%s (%q) at %v sticks out of its card %v", path, n.Text, n.Rect, card)
		}
		for i, k := range n.Children {
			walk(t, k, card, fmt.Sprintf("%s/%s[%d]", path, k.Kind, i))
		}
	}
	for _, s := range [][2]float32{{windowW, windowH}, {701, 846}, {1280, 800}, {1600, 500}} {
		t.Run(fmt.Sprintf("%vx%v", s[0], s[1]), func(t *testing.T) {
			tree, _ := mount(s[0], s[1], 1)
			for i, c := range cards(t, tree) {
				if !inside(c.Rect, gfx.Rect{X1: s[0], Y1: s[1]}) {
					t.Errorf("card %d at %v leaves the window", i, c.Rect)
				}
				walk(t, c, c.Rect, fmt.Sprintf("card%d", i))
			}
		})
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
