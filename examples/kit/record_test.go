package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// TestCalendarHoverRecordsLittle: the Calendar page has more than 300
// layers (each positioned day cell is a chunk of its own); a hover over a
// day restyles the cell and records its chunk and the scrollbars, not the
// page (ADR-204).
func TestCalendarHoverRecordsLittle(t *testing.T) {
	tr := core.NewTree(core.Config{Theme: (*core.Theme)(App.Theme), RootClass: App.RootClass, Dev: core.ResolveDev(nil)})
	tr.SetViewport(gfx.Size{W: 1280, H: 800})
	tr.Mount(core.NodeOf(Gallery{Page: "Calendar"}))
	t.Cleanup(tr.Unmount)
	now := time.Duration(0)
	frame := func() {
		now += 16 * time.Millisecond
		tr.Animate(now)
		tr.Frame()
		tr.FlushEffects()
	}
	frame()
	if n := len(tr.Layers()); n < 300 {
		t.Fatalf("%d layers on the Calendar page, want the day cells' chunks", n)
	}
	var days []platform.Point
	var find func(n core.InspectNode)
	find = func(n core.InspectNode) {
		switch n.Text {
		case "12", "13", "14", "20", "21":
			c := n.Rect.Center()
			days = append(days, platform.Point{X: c.X, Y: c.Y})
		}
		for _, k := range n.Children {
			find(k)
		}
	}
	find(tr.Inspect())
	if len(days) < 5 {
		t.Fatalf("found %d day cells", len(days))
	}
	painted := 0
	for i, p := range days {
		s0 := tr.Stats()
		tr.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: p, Time: now})
		frame()
		if tr.Stats().Paints == s0.Paints {
			continue // the same cell (a day's number is found twice)
		}
		painted++
		n := 0
		for _, l := range tr.Layers() {
			if l.Recorded && l.Kind != "scrollbars" {
				n++
			}
		}
		if n == 0 || n > 2 {
			t.Errorf("hover %d recorded %d layers besides the scrollbars, want the cells' chunks", i, n)
		}
	}
	if painted < 3 {
		t.Errorf("%d hovers painted", painted)
	}
}
