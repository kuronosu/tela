package main

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// recordedCommands sums the commands of the layers the last paint
// recorded.
func recordedCommands(tr *core.Tree) int {
	n := 0
	for _, l := range tr.Layers() {
		if l.Recorded {
			n += l.Commands
		}
	}
	return n
}

// TestRailHoverRecordsLittle: a hover over a button of the rail (the pane
// of icons on the left, painted in the root list with the other panes)
// fades the button's background; each frame of the fade records the
// button's group, a few commands, not the window's root list of hundreds
// (ADR-274).
func TestRailHoverRecordsLittle(t *testing.T) {
	r := newRig(t, 1920, 1080, 1, true)
	r.settle(400)
	r.rest()
	r.tick(nil, r.move(center(r.find("button", "Orders"))))
	frames := 0
	for i := 0; i < 60 && r.asked; i++ {
		r.tick(nil)
		frames++
		if n := recordedCommands(r.tree); n > 20 {
			t.Errorf("frame %d of the hover recorded %d commands, want at most 20", i, n)
		}
	}
	if frames < 3 {
		t.Errorf("the hover ran %d frames: no fade", frames)
	}
}

// BenchmarkRailHover moves the pointer between two buttons of the rail
// and plays their fades: the commands recorded per frame and the median
// pipeline time of a frame (1080p x1).
func BenchmarkRailHover(b *testing.B) {
	r := newRig(b, 1920, 1080, 1, true)
	r.settle(400)
	r.rest()
	at := []string{"Orders", "Tasks"}
	var pipe []time.Duration
	cmds, frames := 0, 0
	i := 0
	for b.Loop() {
		var st []frameStat
		r.tick(&st, r.move(center(r.find("button", at[i%2]))))
		for k := 0; k < 60 && r.asked; k++ {
			r.tick(&st)
			cmds += recordedCommands(r.tree)
			frames++
		}
		for _, f := range st {
			pipe = append(pipe, f.phase[phPipeline])
		}
		i++
	}
	slices.Sort(pipe)
	b.ReportMetric(float64(cmds)/float64(max(frames, 1)), "cmds/frame")
	if len(pipe) > 0 {
		b.ReportMetric(float64(pipe[len(pipe)/2].Nanoseconds())/1000, "pipe-µs/frame")
	}
}

// BenchmarkFullRecording is a frame that records every list (a canvas
// that turns transparent and back: what a new scale or viewport records,
// without the layout), at 1080p x1: the cost of the groups.
func BenchmarkFullRecording(b *testing.B) {
	r := newRig(b, 1920, 1080, 1, true)
	r.settle(400)
	r.rest()
	on := false
	b.ReportAllocs()
	for b.Loop() {
		on = !on
		r.tree.SetTransparentCanvas(on)
		r.tree.Frame()
	}
}
