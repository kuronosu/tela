package main

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// This file measures what a frame of the dashboard does besides its
// time: how many boxes it styles, lays out, places and shapes (ADR-280).

// workOf is the work counters a frame added, from before to after.
func workOf(before, after core.Stats) core.Stats {
	return core.Stats{Styles: after.Styles - before.Styles, Shapes: after.Shapes - before.Shapes,
		Breaks: after.Breaks - before.Breaks, LaidOut: after.LaidOut - before.LaidOut,
		Remapped: after.Remapped - before.Remapped, Placed: after.Placed - before.Placed}
}

// themeSwitch clicks the theme button of the settled page r and returns
// the pipeline time and the work of the frame that applies the switch;
// the frames after it (the button's hover, its tooltip) run unmeasured.
func themeSwitch(r *rig) (time.Duration, core.Stats) {
	var st []frameStat
	before := r.tree.Stats()
	r.tick(&st, r.click("button", "Switch theme")...)
	work := workOf(before, r.tree.Stats())
	r.settle(200)
	r.rest()
	if len(st) != 1 {
		r.tb.Fatalf("the click ran %d frames", len(st))
	}
	return st[0].phase[phPipeline], work
}

// BenchmarkThemeSwitch is the frame of a theme switch at 1080p x1 (one
// frame since ADR-276): the median pipeline time (build, style, layout,
// paint) and its work. A color change lays out nothing and shapes no
// text again.
func BenchmarkThemeSwitch(b *testing.B) {
	r := newRig(b, 1920, 1080, 1, false)
	r.settle(600)
	r.rest()
	var pipe []time.Duration
	var work core.Stats
	for b.Loop() {
		d, w := themeSwitch(r)
		pipe, work = append(pipe, d), w
	}
	slices.Sort(pipe)
	b.ReportMetric(float64(pipe[len(pipe)/2].Microseconds())/1000, "pipeline-ms")
	b.ReportMetric(float64(work.Styles), "styles")
	b.ReportMetric(float64(work.Shapes), "shapes")
	b.ReportMetric(float64(work.LaidOut), "laidout")
	b.ReportMetric(float64(work.Placed), "placed")
}
